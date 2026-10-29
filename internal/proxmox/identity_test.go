package proxmox

import "testing"
func TestDetectScript(t *testing.T) {
 cases:=[]struct{name string;g Guest;want string;fail bool}{
 {"lxc description",Guest{Kind:"lxc",Description:"https://community-scripts.org/scripts/adguard"},"adguard",false},
 {"vm strong tags",Guest{Kind:"vm",Tags:"community-script;app-deployer;script-ubuntu"},"ubuntu",false},
 {"weak tag",Guest{Kind:"vm",Tags:"community-script;app-deployer;ubuntu"},"",true},
 {"lxc tag insufficient",Guest{Kind:"lxc",Tags:"community-script;script-adguard"},"",true},
 {"unknown",Guest{Kind:"lxc"},"",true},
 {"conflict",Guest{Kind:"vm",Tags:"community-script;app-deployer;script-ubuntu",Description:"community-scripts.org/scripts/adguard"},"",true},
 }
 for _,c:=range cases{t.Run(c.name,func(t *testing.T){got,e:=DetectScript(c.g);if got!=c.want||(e!=nil)!=c.fail{t.Fatalf("got %q %v",got,e)}})}
}
func TestValidateExpected(t *testing.T){
 g:=Guest{Kind:"lxc",VMID:120,Description:"https://community-scripts.org/scripts/adguard"}
 if e:=ValidateExpected(g,"lxc","adguard");e!=nil{t.Fatal(e)}
 if e:=ValidateExpected(g,"vm","adguard");e==nil{t.Fatal("type conflict accepted")}
 if e:=ValidateExpected(g,"lxc","ubuntu");e==nil{t.Fatal("script conflict accepted")}
}
