package proxmox

import(
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestFindGuest(t *testing.T){
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="PVEAPIToken=alice@pve!test=secret"{http.Error(w,"denied",401);return}
  switch r.URL.Path{
  case "/api2/json/cluster/resources":
   if r.URL.Query().Get("type")!="vm"{t.Errorf("missing type filter")}
   fmt.Fprint(w,`{"data":[{"vmid":120,"node":"pve2","type":"lxc"}]}`)
  case "/api2/json/nodes/pve2/lxc/120/config":fmt.Fprint(w,`{"data":{"description":"https://community-scripts.org/scripts/adguard"}}`)
  case "/api2/json/nodes/pve2/lxc/120/status/current":fmt.Fprint(w,`{"data":{"status":"running"}}`)
  default:http.Error(w,"missing",404)
  }
 }))
 defer srv.Close()
 c,e:=NewClient(srv.URL,"alice@pve!test","secret",true);if e!=nil{t.Fatal(e)}
 g,ok,e:=c.FindGuest(context.Background(),120)
 if e!=nil||!ok||g.Node!="pve2"||g.Status!="running"{t.Fatalf("guest %+v found=%v err=%v",g,ok,e)}
 _,ok,e=c.FindGuest(context.Background(),121);if e!=nil||ok{t.Fatalf("missing guest error %v, %v",ok,e)}
}
func TestFailedSubresourceReadRetainsIdentity(t *testing.T){
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if strings.Contains(r.URL.Path,"cluster/resources"){fmt.Fprint(w,`{"data":[{"vmid":120,"node":"pve1","type":"qemu"}]}`)}else{http.Error(w,"fail",503)}
 }))
 defer srv.Close()
 c,_:=NewClient(srv.URL,"u@pve!t","secret",true)
 _,ok,e:=c.FindGuest(context.Background(),120);if e==nil||ok{t.Fatal("incomplete read treated as absent")}
}
func TestDuplicateVMIDFails(t *testing.T){
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,`{"data":[{"vmid":120,"node":"pve1","type":"lxc"},{"vmid":120,"node":"pve2","type":"qemu"}]}`)}))
 defer srv.Close()
 c,_:=NewClient(srv.URL,"u@pve!t","secret",true)
 _,ok,e:=c.FindGuest(context.Background(),120);if e==nil||ok{t.Fatal("duplicate VMID accepted")}
}
func TestEndpointChecks(t *testing.T) {
 for _,s:=range []string{"http://pve:8006","https://name:pass@pve:8006","https://pve/path","https://pve?x=1"}{
  if _,e:=NewClient(s,"id","secret",false);e==nil{t.Errorf("accepted %q",s)}
 }
}
