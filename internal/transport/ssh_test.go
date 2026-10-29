package transport

import "testing"

func TestSSHTargetValidation(t *testing.T){
 good:=[]string{"pve1","admin@pve1","pve1.example.internal"}
 for _,v:=range good{c,e:=NewSSH(v);if e!=nil||c==nil{t.Fatalf("valid %q: %v",v,e)}}
 for _,v:=range []string{"-oProxyCommand=bad","user@host;reboot","user@host other","user@host\nmalicious","admin@[::1]"}{
  if _,e:=NewSSH(v);e==nil{t.Errorf("unsafe target %q accepted",v)}
 }
 c,e:=NewSSH("");if e!=nil||c!=nil{t.Fatalf("optional SSH absent: %v",e)}
}
