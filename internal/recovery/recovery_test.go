package recovery

import (
 "strings"
 "testing"
)
func TestParseID(t *testing.T){
 for _,s:=range []string{"lxc/120","vm/999999999"}{id,e:=ParseID(s);if e!=nil||id.String()!=s{t.Fatalf("unexpected %s: %+v %v",s,id,e)}}
 for _,s:=range []string{"","120","qemu/120","lxc/99","vm/1000000000","lxc/0120","lxc/-120","vm/+120","vm/120/extra"}{
  if _,e:=ParseID(s);e==nil{t.Errorf("accepted invalid ID %q",s)}
 }
}
func TestRecoveryGuidance(t *testing.T){
 s:=RecoveryHint(ID{Kind:"lxc",VMID:120})
 for _,word:=range []string{"Do not re-run","VMID 120","journal","lxc/120"}{if !strings.Contains(s,word){t.Fatalf("missing %q",word)}}
}
