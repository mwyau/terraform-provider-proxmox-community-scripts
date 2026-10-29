// Package runner is the privileged, separately installed PVE host-side coordinator.
// It uses one coordinator host per cluster; a local flock is not distributed.
package runner

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "strings"
 "syscall"
 "time"

 "github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/proxmox"
)
var shaRE=regexp.MustCompile(`^[a-f0-9]{40}$`)
var slugRE=regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var simpleRE=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
type Request struct {
 Action string `json:"action"`
 VMID int64 `json:"vmid"`
 Node string `json:"node"`
 Kind string `json:"kind"`
 Script string `json:"script"`
 ScriptsRef string `json:"scripts_ref"`
 CoreRef string `json:"core_ref"`
 Settings map[string]string `json:"settings,omitempty"`
 UpdateRequestID string `json:"update_request_id,omitempty"`
 BackupStorage string `json:"backup_storage,omitempty"`
 AcceptUnverified bool `json:"accept_unverified,omitempty"`
 ConfirmDestroy bool `json:"confirm_destroy,omitempty"`
 AllowImportedDestroy bool `json:"allow_imported_destroy,omitempty"`
}
type Result struct { Status string `json:"status"`; Journal string `json:"journal,omitempty"` }
type Journal struct {
 Attempt string `json:"attempt"`
 Phase string `json:"phase"`
 Request Request `json:"request"`
 Time string `json:"time"`
 Error string `json:"error,omitempty"`
}
// System abstracts only the process boundary; no shell string interpolation.
type System interface {Run(ctx context.Context, name string, args []string, env map[string]string) (string,error)}
type Runner struct {Dir string; Node string; System System}
func valid(r Request) error {
 if r.VMID<100||r.VMID>999999999||!simpleRE.MatchString(r.Node)||!slugRE.MatchString(r.Script)|| (r.Kind!="lxc"&&r.Kind!="vm") {return errors.New("invalid node, VMID, kind, or script")}
 if r.Action!="create"&&r.Action!="update"&&r.Action!="delete"{return errors.New("invalid action")}
 if (r.Action=="create"||r.Action=="update") && (!shaRE.MatchString(r.ScriptsRef)|| (r.Action=="create"&&!shaRE.MatchString(r.CoreRef))) {return errors.New("immutable full source SHA required")}
 for k,v:=range r.Settings{
  if !simpleRE.MatchString(k)||len(v)>1024||strings.ContainsAny(v,"\x00\r\n"){return fmt.Errorf("unsafe setting %q",k)}
 }
 if r.Action=="create" && !r.AcceptUnverified {return errors.New("creation requires explicit accept_unverified approval")}
 if r.Action=="update" && (r.Kind!="lxc"||!simpleRE.MatchString(r.UpdateRequestID)||!simpleRE.MatchString(r.BackupStorage)) {return errors.New("LXC update requires request ID and backup storage")}
 if r.Action=="delete"&&!r.ConfirmDestroy {return errors.New("explicit destruction approval required")}
 return nil
}
func (r *Runner) withLock(fn func() (Result,error)) (Result,error) {
 if err:=os.MkdirAll(r.Dir,0700);err!=nil{return Result{},err}
 if err:=os.Chmod(r.Dir,0700);err!=nil{return Result{},err}
 f,err:=os.OpenFile(filepath.Join(r.Dir,".lock"),os.O_CREATE|os.O_RDWR,0600);if err!=nil{return Result{},err}
 defer f.Close()
 if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX);err!=nil{return Result{},err}
 defer syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
 return fn()
}
func (r *Runner) journalPath(req Request) string {
 return filepath.Join(r.Dir,req.Kind,fmt.Sprintf("%d",req.VMID),"journal.json")
}
func (r *Runner) updatePath(req Request) string {
 return filepath.Join(r.Dir,req.Kind,fmt.Sprintf("%d",req.VMID),"updates",req.UpdateRequestID+".json")
}
func readJournal(path string) (Journal,bool,error){
 b,e:=os.ReadFile(path)
 if errors.Is(e,os.ErrNotExist){return Journal{},false,nil}
 if e!=nil{return Journal{},false,e}
 var j Journal
 if e=json.Unmarshal(b,&j);e!=nil{return Journal{},true,fmt.Errorf("corrupt journal; refuse operation: %w",e)}
 return j,true,nil
}
func atomicJournal(path string,j Journal) error {
 dir:=filepath.Dir(path)
 if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 if err:=os.Chmod(dir,0700);err!=nil{return err}
 b,err:=json.MarshalIndent(j,"","  ");if err!=nil{return err}
 f,err:=os.CreateTemp(dir,".journal-");if err!=nil{return err}
 tmp:=f.Name()
 defer os.Remove(tmp)
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(b);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(tmp,path);err!=nil{return err}
 d,err:=os.Open(dir);if err!=nil{return err};defer d.Close()
 return d.Sync()
}
func newJournal(req Request) (Journal,error){
 var data [16]byte
 if _,err:=rand.Read(data[:]);err!=nil{return Journal{},err}
 return Journal{Attempt:hex.EncodeToString(data[:]),Phase:"prepared",Request:req,Time:time.Now().UTC().Format(time.RFC3339)},nil
}
func (r *Runner) exec(ctx context.Context,name string,args ...string) (string,error){
 return r.System.Run(ctx,name,args,nil)
}
func (r *Runner) list(ctx context.Context) ([]struct{VMID int64 `json:"vmid"`; Node string `json:"node"`; Type string `json:"type"`},error) {
 var list []struct{VMID int64 `json:"vmid"`; Node string `json:"node"`; Type string `json:"type"`}
 out,e:=r.exec(ctx,"pvesh","get","/cluster/resources","--type","vm","--output-format","json")
 if e!=nil{return nil,e}
 if e=json.Unmarshal([]byte(out),&list);e!=nil{return nil,fmt.Errorf("unexpected cluster inventory: %w",e)}
 return list,nil
}
func (r *Runner) check(ctx context.Context,req Request) (proxmox.Guest,bool,error) {
 list,e:=r.list(ctx);if e!=nil{return proxmox.Guest{},false,e}
 var result proxmox.Guest; found:=false
 for _,g:=range list{if g.VMID!=req.VMID{continue}
  if found{return proxmox.Guest{},false,errors.New("duplicate VMID")}
  found=true;kind:=g.Type;if kind=="qemu"{kind="vm"}
  result=proxmox.Guest{VMID:g.VMID,Kind:kind,Node:g.Node}
 }
 if !found{return result,false,nil}
 path:=fmt.Sprintf("/nodes/%s/%s/%d/config",result.Node,result.Kind,result.VMID)
 if result.Kind=="vm"{path=fmt.Sprintf("/nodes/%s/qemu/%d/config",result.Node,result.VMID)}
 out,e:=r.exec(ctx,"pvesh","get",path,"--output-format","json");if e!=nil{return result,true,e}
 var conf struct{Description string `json:"description"`; Tags string `json:"tags"`}
 if e=json.Unmarshal([]byte(out),&conf);e!=nil{return result,true,e}
 result.Description=conf.Description;result.Tags=conf.Tags
 statusPath:=strings.TrimSuffix(path,"/config")+"/status/current"
 out,e=r.exec(ctx,"pvesh","get",statusPath,"--output-format","json");if e!=nil{return result,true,e}
 var st struct{Status string `json:"status"`}
 if e=json.Unmarshal([]byte(out),&st);e!=nil{return result,true,e}
 result.Status=st.Status
 return result,true,nil
}
func (r *Runner) requireGuest(ctx context.Context,req Request)(proxmox.Guest,error){
 g,found,e:=r.check(ctx,req);if e!=nil{return g,e}
 if !found{return g,errors.New("guest not found")}
 if g.Node!=req.Node{return g,fmt.Errorf("guest moved to %s; refusing operation",g.Node)}
 if e=proxmox.ValidateExpected(g,req.Kind,req.Script);e!=nil{return g,e}
 return g,nil
}
func (r *Runner) Handle(ctx context.Context,req Request)(Result,error) {
 if r.System==nil{return Result{},errors.New("missing system executor")}
 if err:=valid(req);err!=nil{return Result{},err}
 if r.Node=="" || r.Node!=req.Node{return Result{},errors.New("nonlocal guest denied: use single designated coordinator node")}
 return r.withLock(func()(Result,error){
  switch req.Action{
  case "create":return r.create(ctx,req)
  case "update":return r.update(ctx,req)
  default:return r.destroy(ctx,req)
  }
 })
}
func (r *Runner) sources(ctx context.Context,req Request)(string,string,error) {
 dir:=filepath.Join(r.Dir,"sources")
 if e:=os.MkdirAll(dir,0700);e!=nil{return "","",e}
 clone:=func(name,remote,sha string)(string,error){
  target:=filepath.Join(dir,name+"-"+sha)
  if _,e:=os.Stat(filepath.Join(target,".git"));errors.Is(e,os.ErrNotExist){
   if e=os.MkdirAll(target,0700);e!=nil{return "",e}
   if _,e=r.exec(ctx,"git","-C",target,"init","-q");e!=nil{return "",e}
   if _,e=r.exec(ctx,"git","-C",target,"remote","add","origin",remote);e!=nil{return "",e}
  }
  if _,e:=r.exec(ctx,"git","-C",target,"fetch","--depth","1","origin",sha);e!=nil{return "",e}
  if _,e:=r.exec(ctx,"git","-C",target,"checkout","-q","--detach",sha);e!=nil{return "",e}
  got,e:=r.exec(ctx,"git","-C",target,"rev-parse","HEAD")
  if e!=nil||strings.TrimSpace(got)!=sha{return "",errors.New("checkout did not resolve to requested immutable SHA")}
  return target,nil
 }
 scripts,e:=clone("scripts","https://github.com/community-scripts/ProxmoxVE.git",req.ScriptsRef);if e!=nil{return "","",e}
 if req.Action=="update"{return scripts,"",nil}
 core,e:=clone("core","https://github.com/community-scripts/core.git",req.CoreRef)
 return scripts,core,e
}
func (r *Runner) create(ctx context.Context,req Request)(Result,error) {
 path:=r.journalPath(req)
 _,existing,e:=readJournal(path);if e!=nil{return Result{},e}
 if existing{return Result{},fmt.Errorf("prior journal for %s/%d exists; inspect and import, never repeat installation",req.Kind,req.VMID)}
 _,found,e:=r.check(ctx,req);if e!=nil{return Result{},e}
 if found{return Result{},errors.New("VMID already exists; explicit import required")}
 j,e:=newJournal(req);if e!=nil{return Result{},e}
 if e=atomicJournal(path,j);e!=nil{return Result{},e}
 fail:=func(err error)(Result,error){
  j.Phase="uncertain";j.Error="operator inspection required";_ = atomicJournal(path,j)
  return Result{Journal:path,Status:j.Phase},fmt.Errorf("installation requires manual recovery: %w",err)
 }
 scripts,core,e:=r.sources(ctx,req);if e!=nil{return fail(e)}
 _,found,e=r.check(ctx,req);if e!=nil{return fail(e)}
 if found{return fail(errors.New("VMID collision before installer invocation"))}
 if _,e=os.Stat(filepath.Join(scripts,map[bool]string{true:"ct",false:"vm"}[req.Kind=="lxc"],req.Script+".sh"));e!=nil{return fail(e)}
 j.Phase="running";j.Error=""
 if e=atomicJournal(path,j);e!=nil{return Result{},e}
 env:=map[string]string{
  "COMMUNITY_SCRIPTS_URL":"https://raw.githubusercontent.com/community-scripts/ProxmoxVE/"+req.ScriptsRef,
  "COMMUNITY_SCRIPTS_CORE_URL":"https://raw.githubusercontent.com/community-scripts/core/"+req.CoreRef,
  "COMMUNITY_SCRIPTS_CORE_DIR":core, "COMMUNITY_SCRIPTS_ROOT":scripts,
  "PHS_SILENT":"1","TERM":"dumb",
 }
 argv:=[]string{filepath.Join(scripts,"ct",req.Script+".sh"),"generated"}
 if req.Kind=="lxc"{
  env["mode"]="generated";env["MODE"]="generated";env["var_ctid"]=fmt.Sprint(req.VMID);env["var_unattended"]="yes"
 }else{
  argv=[]string{filepath.Join(scripts,"vm",req.Script+".sh")}
  env["VM_VMID"]=fmt.Sprint(req.VMID);env["VM_UNATTENDED"]="1"
 }
 for k,v:=range req.Settings {env[k]=v}
 if _,e=r.System.Run(ctx,"bash",argv,env);e!=nil{return fail(e)}
 g,found,e:=r.check(ctx,req);if e!=nil{return fail(e)}
 if !found||g.Node!=req.Node{return fail(errors.New("installer did not create expected VMID on expected node"))}
 if e=proxmox.ValidateExpected(g,req.Kind,req.Script);e!=nil{return fail(e)}
 j.Phase="verified"
 if e=atomicJournal(path,j);e!=nil{return Result{Journal:path,Status:"uncertain"},e}
 return Result{Journal:path,Status:"verified"},nil
}
func (r *Runner) update(ctx context.Context,req Request)(Result,error){
 path:=r.updatePath(req)
 j,exists,e:=readJournal(path);if e!=nil{return Result{},e}
 if exists{
  if j.Phase=="verified" && j.Request.VMID==req.VMID && j.Request.Script==req.Script {return Result{Status:"already-applied",Journal:path},nil}
  return Result{},errors.New("prior uncertain update journal; manual recovery required")
 }
 g,e:=r.requireGuest(ctx,req);if e!=nil{return Result{},e}
 if g.Status!="running"{return Result{},errors.New("LXC must be running for updater")}
 // Run a positive LXC updater entrypoint inspection before any updater is invoked.
 _,e=r.exec(ctx,"pct","exec",fmt.Sprint(req.VMID),"--","test","-r","/usr/bin/update");if e!=nil{return Result{},e}
 j,e=newJournal(req);if e!=nil{return Result{},e}
 if e=atomicJournal(path,j);e!=nil{return Result{},e}
 fail:=func(err error)(Result,error){j.Phase="uncertain";j.Error="operator inspection required";_ = atomicJournal(path,j);return Result{Status:"uncertain",Journal:path},err}
 scripts,_,e:=r.sources(ctx,req);if e!=nil{return fail(e)}
 j.Phase="running";if e=atomicJournal(path,j);e!=nil{return Result{},e}
 env:=map[string]string{"PHS_SILENT":"1","TERM":"dumb","var_container":fmt.Sprint(req.VMID),"var_unattended":"yes","var_skip_confirm":"yes","var_continue_on_error":"no","var_auto_reboot":"no","var_dry_run":"no","var_backup":"yes","var_backup_storage":req.BackupStorage}
 if _,e=r.System.Run(ctx,"bash",[]string{filepath.Join(scripts,"tools/pve/update-apps.sh")},env);e!=nil{return fail(e)}
 if _,e=r.requireGuest(ctx,req);e!=nil{return fail(e)}
 j.Phase="verified";if e=atomicJournal(path,j);e!=nil{return Result{},e}
 return Result{Status:"verified",Journal:path},nil
}
func (r *Runner) destroy(ctx context.Context,req Request)(Result,error){
 path:=r.journalPath(req)
 j,exists,e:=readJournal(path);if e!=nil{return Result{},e}
 if exists && j.Phase!="verified" && j.Phase!="deleted" {return Result{},errors.New("partial/uncertain installation: do not destroy automatically")}
 if !exists && !req.AllowImportedDestroy{return Result{},errors.New("imported guest has no verified creation journal; allow_imported_destroy required")}
 g,found,e:=r.check(ctx,req);if e!=nil{return Result{},e}
 if !found{
  if exists && j.Phase=="deleted"{return Result{Status:"already-deleted",Journal:path},nil}
  return Result{},errors.New("guest absent without confirmed deletion journal; manual review required")
 }
 if g.Node!=req.Node{return Result{},errors.New("guest moved; refusing deletion")}
 if e=proxmox.ValidateExpected(g,req.Kind,req.Script);e!=nil{return Result{},e}
 if g.Status!="stopped"{return Result{},errors.New("guest must be stopped explicitly before deletion")}
 if !exists {j,e=newJournal(req);if e!=nil{return Result{},e}}
 j.Request=req;j.Phase="deleting"
 if e=atomicJournal(path,j);e!=nil{return Result{},e}
 tool:="pct";if req.Kind=="vm"{tool="qm"}
 if _,e=r.exec(ctx,tool,"destroy",fmt.Sprint(req.VMID));e!=nil{return Result{Status:"uncertain",Journal:path},e}
 _,found,e=r.check(ctx,req);if e!=nil{return Result{Status:"uncertain",Journal:path},e}
 if found{return Result{Status:"uncertain",Journal:path},errors.New("guest remains after destruction request")}
 j.Phase="deleted"
 if e=atomicJournal(path,j);e!=nil{return Result{Status:"uncertain",Journal:path},e}
 return Result{Status:"deleted",Journal:path},nil
}
