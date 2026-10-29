package compat

import(
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
const A="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const B="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
func fake(t *testing.T,script string,core string)*Inspector{
 t.Helper()
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if strings.Contains(r.URL.Path,"/ProxmoxVE/"){fmt.Fprint(w,script);return}
  if strings.Contains(r.URL.Path,"/core/"){fmt.Fprint(w,core);return}
  http.NotFound(w,r)
 }))
 t.Cleanup(srv.Close)
 return &Inspector{HTTP:srv.Client(),Base:srv.URL}
}
func TestPinnedLXCPlanInspection(t *testing.T){
 inspector:=fake(t,"source core/build.func\nstart\nbuild_container\nmsg_menu", "var_cpu var_ram var_disk")
 r,e:=inspector.Verify(context.Background(),"lxc","adguard",A,B,map[string]string{"cores":"2","memory":"2048","disk_size":"8"})
 if e!=nil{t.Fatal(e)}
 if r.ScriptPath!="ct/adguard.sh"||len(r.Notes)<2{t.Fatalf("result %+v",r)}
}
func TestRetiredStubDenied(t *testing.T){
 inspector:=fake(t,"msg_warn 'This script was removed'\nexit 1\n", "")
 if _,e:=inspector.Verify(context.Background(),"lxc","old",A,B,nil);e==nil{t.Fatal("retired stub accepted")}
}
func TestMissingSharedCoreVariable(t *testing.T){
 inspector:=fake(t,"source core/build.func\nbuild_container\n","var_cpu")
 if _,e:=inspector.Verify(context.Background(),"lxc","test",A,B,map[string]string{"memory":"2048"});e==nil{t.Fatal("unsupported Core override accepted")}
}
func TestVMWarnings(t *testing.T){
 inspector:=fake(t,"source pve/vm-core.func\nvm_start_script\n","vm_apply_env_overrides() { } VM_UNATTENDED VM_RAM")
 r,e:=inspector.Verify(context.Background(),"vm","ubuntu-vm",A,B,map[string]string{"memory":"2048"})
 if e!=nil{t.Fatal(e)}
 if len(r.Notes)<2{t.Fatal("missing per-script field usage warning")}
}
func TestHTTPFailureStopsPlan(t *testing.T){
 srv:=httptest.NewServer(http.NotFoundHandler());defer srv.Close()
 c:=Inspector{HTTP:srv.Client(),Base:srv.URL}
 if _,e:=c.Verify(context.Background(),"lxc","adguard",A,B,nil);e==nil{t.Fatal("missing source accepted")}
 if _,e:=c.Verify(context.Background(),"lxc","adguard","main",B,nil);e==nil{t.Fatal("moving branch accepted")}
}
