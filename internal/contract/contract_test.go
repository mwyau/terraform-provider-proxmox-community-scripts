package contract

import (
 "reflect"
 "strings"
 "testing"
)
func TestFieldCounts(t *testing.T){
 if len(LXCFields)!=34 || len(VMFields)!=14 {t.Fatalf("drift: %d LXC, %d VM fields",len(LXCFields),len(VMFields))}
}
func TestMappingsAndConversions(t *testing.T){
 env,e:=Environment("lxc",map[string]string{"cores":"2","memory":"2048","privileged":"true","ipv6_method":"auto","nesting":"false","tUN":"yes"},nil)
 if e==nil{t.Fatalf("unknown variable accepted: %v",env)}
 env,e=Environment("lxc",map[string]string{"cores":"2","memory":"2048","privileged":"true","nesting":"false","tun":"yes"},map[string]string{"APP_OPTION":"x"})
 if e!=nil{t.Fatal(e)}
 want:=map[string]string{"var_cpu":"2","var_ram":"2048","var_unprivileged":"0","var_nesting":"0","var_tun":"yes","APP_OPTION":"x"}
 if !reflect.DeepEqual(env,want){t.Fatalf("got %v, want %v",env,want)}
 vm,e:=Environment("vm",map[string]string{"disk_size":"8","disk_cache":"none","start":"false","machine":"q35"},nil)
 if e!=nil{t.Fatal(e)}
 if vm["VM_DISK_SIZE"]!="8G"||vm["VM_DISK_CACHE"]!="cache=none,"||vm["VM_START"]!="no"{t.Fatalf("bad VM normalization %v",vm)}
}
func TestRejectOverrideAndInjection(t *testing.T){
 for _,k:=range []string{"PATH","BASH_ENV","LD_PRELOAD","VM_RAM","var_ctid","var_cpu","COMMUNITY_SCRIPTS_CORE_DIR","APP-INVALID"}{
  if err:=ScriptVars(map[string]string{k:"x"});err==nil{t.Errorf("accepted %s",k)}
 }
 for _,tc:=range []map[string]string{{"gpu":"undefined"},{"vlan":"4095"},{"ipv6_method":"static"},{"cpu_type":"qemu64"},{"memory":"64"}} {
  kind:="lxc";if _,ok:=tc["cpu_type"];ok{kind="vm"}
  if err:=ValidateSettings(kind,tc);err==nil{t.Errorf("accepted %v",tc)}
 }
 if err:=ScriptVars(map[string]string{"APP_TOKEN":strings.Repeat("a",32770)});err==nil{t.Fatal("script_vars length accepted")}
}
func TestDefaultsPrecedence(t *testing.T){
 d:=map[string]map[string]string{"all":{"cores":"2","memory":"1024"},"lxc":{"memory":"2048"}}
 got,e:=ResolveDefaults("lxc",d,map[string]string{"cores":"4"});if e!=nil{t.Fatal(e)}
 want:=map[string]string{"cores":"4","memory":"2048"}
 if !reflect.DeepEqual(got,want){t.Fatalf("defaults=%v",got)}
}
