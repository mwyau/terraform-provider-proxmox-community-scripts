package runner

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "sync"
)

type fakePVE struct{
 exists bool
 kind string
 node string
 script string
 status string
 creates int
 deletes int
 updates int
 failScript bool
 failFetch bool
 extraID bool
 badPostflight bool
 updaterMutatesHardware bool
 lastEnv map[string]string
}
func(f *fakePVE) Run(_ context.Context,name string,args []string,env map[string]string)(string,error){
 if name=="pvesh" {
  if len(args)<3 {return "",errors.New("bad pvesh args")}
  if args[1]=="/cluster/resources" {
   if !f.exists{return "[]",nil}
   kind:=f.kind;if kind=="vm"{kind="qemu"}
   list:=[]map[string]any{{"vmid":120,"node":f.node,"type":kind}}
   if f.extraID&&f.creates>0{list=append(list,map[string]any{"vmid":121,"node":f.node,"type":"lxc"})}
   b,_:=json.Marshal(list);return string(b),nil
  }
  if strings.HasSuffix(args[1],"/config"){
   memory:=2048
   if f.updaterMutatesHardware && f.updates>0{memory=4096}
   return fmt.Sprintf(`{"description":"https://community-scripts.org/scripts/%s","memory":%d}`,f.script,memory),nil
  }
  if strings.HasSuffix(args[1],"/status/current"){
   return fmt.Sprintf(`{"status":"%s"}`,f.status),nil
  }
  return "",fmt.Errorf("unexpected pvesh request: %v",args)
 }
 if name=="git"{
  if f.failFetch&&len(args)>2&&args[2]=="fetch" {return "",errors.New("network unavailable")}
  dir:=args[1]
  if len(args)>2&&args[2]=="init" {return "",os.MkdirAll(filepath.Join(dir,".git"),0700)}
  if len(args)>2&&args[2]=="checkout" && strings.Contains(dir,"scripts-") {
    p:=filepath.Join(dir,"ct")
    if e:=os.MkdirAll(p,0700);e!=nil{return "",e}
    if e:=os.WriteFile(filepath.Join(p,"adguard.sh"),[]byte("#!/bin/bash\n"),0600);e!=nil{return "",e}
    if e:=os.MkdirAll(filepath.Join(dir,"vm"),0700);e!=nil{return "",e}
    if e:=os.WriteFile(filepath.Join(dir,"vm","adguard.sh"),[]byte("#!/bin/bash\n"),0600);e!=nil{return "",e}
  }
  if len(args)>2&&args[2]=="rev-parse"{return filepath.Base(dir)[strings.LastIndex(filepath.Base(dir),"-")+1:]+ "\n",nil}
  return "",nil
 }
 if name=="bash" {
  f.lastEnv=env
  if f.failScript {f.creates++;f.exists=true;return "",errors.New("interrupted installer")}
  if strings.HasSuffix(args[0],"update-apps.sh"){f.updates++;return "",nil}
  f.creates++;f.exists=true;f.status="running";if f.badPostflight {f.script="unexpected"};return "",nil
 }
 if name=="pct" {
  if args[0]=="exec"{return "export SCRIPT_SLUG=adguard\n",nil}
  if args[0]=="destroy"{f.deletes++;f.exists=false;return "",nil}
 }
 if name=="qm"&&args[0]=="destroy"{f.deletes++;f.exists=false;return "",nil}
 return "",fmt.Errorf("unexpected process %s %v",name,args)
}
const testSHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const coreSHA="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
func request(action string)Request{
 return Request{Action:action,VMID:120,Node:"pve1",Kind:"lxc",Script:"adguard",ScriptsRef:testSHA,CoreRef:coreSHA,AcceptUnverified:true}
}
func newFake(t *testing.T)(*Runner,*fakePVE){
 t.Helper()
 f:=&fakePVE{node:"pve1",kind:"lxc",script:"adguard",status:"running"}
 return &Runner{Dir:t.TempDir(),Node:"pve1",System:f},f
}
func TestCreateNoReplay(t *testing.T){
 r,f:=newFake(t)
 req:=request("create")
 req.Settings=map[string]string{"cores":"2","memory":"2048","storage":"local"}
 got,e:=r.Handle(context.Background(),req)
 if e!=nil||got.Status!="verified"||f.creates!=1{t.Fatalf("first create = %+v, %v; count %d",got,e,f.creates)}
 if f.lastEnv["var_ctid"]!="120"||f.lastEnv["var_cpu"]!="2"||f.lastEnv["var_container_storage"]!="local"{t.Fatalf("wrong Core mapping: %v",f.lastEnv)}
 if _,e=r.Handle(context.Background(),req);e==nil{t.Fatal("journal did not block automatic retry")}
 if f.creates!=1{t.Fatal("duplicate script execution")}
 j,ok,e:=readJournal(r.journalPath(req));if e!=nil||!ok||j.Phase!="verified"{t.Fatalf("journal %+v %v",j,e)}
}
func TestInterruptedCreateBlocked(t *testing.T){
 r,f:=newFake(t);f.failScript=true
 req:=request("create")
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("expected uncertain error")}
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("retried uncertain install")}
 if f.creates!=1{t.Fatalf("unexpected %d attempts",f.creates)}
 j,_,_:=readJournal(r.journalPath(req));if j.Phase!="uncertain"{t.Fatalf("phase %s",j.Phase)}
}
func TestPreparedOrFailedSourceBlocksReplay(t *testing.T){
 r,f:=newFake(t);f.failFetch=true;req:=request("create")
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("expected source failure")}
 f.failFetch=false
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("source failure incorrectly retryable")}
 if f.creates!=0{t.Fatal("script executed despite source failure")}
}
func TestOccupiedIDNeverExecutes(t *testing.T){
 r,f:=newFake(t);f.exists=true
 if _,e:=r.Handle(context.Background(),request("create"));e==nil{t.Fatal("occupied ID accepted")}
 if f.creates!=0{t.Fatal("ran script on occupied ID")}
}
func TestUpdateIdempotencyAndIdentity(t *testing.T){
 r,f:=newFake(t);f.exists=true
 q:=request("update");q.UpdateRequestID="patch-20261029";q.BackupStorage="backup"
 got,e:=r.Handle(context.Background(),q);if e!=nil||got.Status!="verified"{t.Fatalf("update %v %+v",e,got)}
 got,e=r.Handle(context.Background(),q);if e!=nil||got.Status!="already-applied"||f.updates!=1{t.Fatal("duplicate update executed")}
 q.ScriptsRef=coreSHA
 if _,e=r.Handle(context.Background(),q);e==nil{t.Fatal("same request ID different pinned source accepted")}
}
func TestDestroyRetainAndJournalGuards(t *testing.T){
 r,f:=newFake(t);f.exists=true;f.status="stopped";req:=request("delete");req.ConfirmDestroy=true
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("imported deletion without override accepted")}
 if f.deletes!=0{t.Fatal("deleted without approval")}
 req.AllowImportedDestroy=true
 got,e:=r.Handle(context.Background(),req)
 if e!=nil||got.Status!="deleted"||f.deletes!=1{t.Fatalf("destroy=%+v, %v",got,e)}
 again,e:=r.Handle(context.Background(),req)
 if e!=nil||again.Status!="already-deleted"||f.deletes!=1{t.Fatal("destroy not idempotent")}
}
func TestDestroyRefusesRunningAndWrongSlug(t *testing.T){
 r,f:=newFake(t);f.exists=true
 req:=request("delete");req.ConfirmDestroy=true;req.AllowImportedDestroy=true
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("deleted running guest")}
 f.status="stopped";f.script="different"
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("deleted different script")}
 if f.deletes!=0{t.Fatal("destructive request reached system")}
}
func TestRejectRawEnvironmentOverrides(t *testing.T){
 for _,settings:=range []map[string]string{
  {"var_ctid":"999"},{"COMMUNITY_SCRIPTS_ROOT":"/tmp"}, {"cores":"2\nEVIL=yes"},
 }{
  q:=request("create");q.Settings=settings
  if err:=valid(q);err==nil{t.Fatalf("accepted %v",settings)}
 }
}
func TestForeignNodeRefused(t *testing.T){
 r,f:=newFake(t);req:=request("create");req.Node="pve2"
 if _,e:=r.Handle(context.Background(),req);e==nil{t.Fatal("other node accepted")}
 if f.creates!=0{t.Fatal("ran installer on other node")}
}

func TestUnexpectedAdditionalVMIDUncertain(t *testing.T){
 r,f:=newFake(t);f.extraID=true
 q:=request("create")
 if _,e:=r.Handle(context.Background(),q);e==nil{t.Fatal("unexpected extra VMID was accepted")}
 j,_,_:=readJournal(r.journalPath(q))
 if j.Phase!="uncertain"{t.Fatalf("phase %q",j.Phase)}
 if _,e:=r.Handle(context.Background(),q);e==nil{t.Fatal("unsafe replay")}
}
func TestMalformedNormalizedSettings(t *testing.T){
 for _,opts:=range []map[string]string{
  {"cores":"0"},{"cores":"256"},{"memory":"1"},{"disk_size":"-1"},
  {"vlan":"4095"},{"bridge":"vmbr0;reboot"},{"storage":"/tmp/x"},
  {"memory":"2048x"},{"hostname":"foo bar"},
 }{
  q:=request("create");q.Settings=opts
  if e:=valid(q);e==nil{t.Errorf("accepted settings %v",opts)}
 }
}

func TestConcurrentCreatesSerialized(t *testing.T){
 r,f:=newFake(t)
 q:=request("create")
 var wg sync.WaitGroup
 errorsCh:=make(chan error,2)
 for i:=0;i<2;i++ {
  wg.Add(1)
  go func(){defer wg.Done();_,e:=r.Handle(context.Background(),q);errorsCh<-e}()
 }
 wg.Wait();close(errorsCh)
 success,fail:=0,0
 for e:=range errorsCh {if e==nil{success++}else{fail++}}
 if success!=1||fail!=1||f.creates!=1{t.Fatalf("success=%d fail=%d creates=%d",success,fail,f.creates)}
}
func TestInstallerSuccessWithoutMatchingProvenanceIsUncertain(t *testing.T){
 r,f:=newFake(t);f.badPostflight=true
 q:=request("create")
 if _,e:=r.Handle(context.Background(),q);e==nil{t.Fatal("wrong postflight identity accepted")}
 j,ok,e:=readJournal(r.journalPath(q))
 if e!=nil||!ok||j.Phase!="uncertain"{t.Fatalf("journal %+v %v",j,e)}
 if f.creates!=1{t.Fatalf("script count %d",f.creates)}
}

func TestUpdateDetectsUnexpectedHardwareMutation(t *testing.T){
 r,f:=newFake(t);f.exists=true;f.updaterMutatesHardware=true
 q:=request("update");q.UpdateRequestID="next";q.BackupStorage="backup"
 if _,e:=r.Handle(context.Background(),q);e==nil{t.Fatal("updater changed memory undetected")}
 j,ok,e:=readJournal(r.updatePath(q))
 if e!=nil||!ok||j.Phase!="uncertain"{t.Fatalf("journal %+v, err %v",j,e)}
 if _,e:=r.Handle(context.Background(),q);e==nil{t.Fatal("unsafe replay after hardware change")}
}
