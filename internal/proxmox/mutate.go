package proxmox

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "time"
)
func (c *Client) postConfig(ctx context.Context,path string,params url.Values)(string,error){
 req,e:=http.NewRequestWithContext(ctx,http.MethodPut,c.base+"/api2/json"+path,strings.NewReader(params.Encode()))
 if e!=nil{return "",e}
 req.Header.Set("Authorization",c.token)
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 res,e:=c.http.Do(req);if e!=nil{return "",e}
 defer res.Body.Close()
 if res.StatusCode!=200{return "",fmt.Errorf("Proxmox write returned HTTP %d",res.StatusCode)}
 var d struct{Data json.RawMessage `json:"data"`}
 if e=json.NewDecoder(io.LimitReader(res.Body,2<<20)).Decode(&d);e!=nil{return "",e}
 var upid string
 if len(d.Data)>0&&string(d.Data)!="null"{_ = json.Unmarshal(d.Data,&upid)}
 return upid,nil
}
// ApplyCompute writes only fields explicitly declared, never resets other config.
// Storage shrink, disk moves, network and cloud-init are excluded.
func (c *Client) ApplyCompute(ctx context.Context,g Guest,values map[string]string) error{
 params:=url.Values{}
 for k,v:=range values {
  if k!="cores"&&k!="memory"{return fmt.Errorf("unsupported mutable property %q",k)}
  n,e:=strconv.ParseInt(v,10,64)
  if e!=nil||n<1 || k=="memory"&&n<256 || k=="cores"&&n>128 {return fmt.Errorf("invalid value for %s",k)}
  params.Set(k,strconv.FormatInt(n,10))
 }
 if len(params)==0{return nil}
 kind:=g.Kind;if kind=="vm"{kind="qemu"}
 path:="/nodes/"+url.PathEscape(g.Node)+"/"+kind+"/"+strconv.FormatInt(g.VMID,10)
 upid,e:=c.postConfig(ctx,path+"/config",params);if e!=nil{return e}
 if upid!=""{return c.WaitTask(ctx,g.Node,upid)}
 return nil
}
func (c *Client) WaitTask(ctx context.Context,node,upid string)error{
 if node==""||upid==""{return errors.New("missing Proxmox task")}
 ctx,cancel:=context.WithTimeout(ctx,5*time.Minute);defer cancel()
 path:="/nodes/"+url.PathEscape(node)+"/tasks/"+url.PathEscape(upid)+"/status"
 ticker:=time.NewTicker(1500*time.Millisecond);defer ticker.Stop()
 for {
  var st struct{Status string `json:"status"`; Exit string `json:"exitstatus"`}
  if e:=c.get(ctx,path,&st);e!=nil{return e}
  if st.Status=="stopped"{if st.Exit=="OK"{return nil};return fmt.Errorf("Proxmox task failed with status %s",st.Exit)}
  select{case <-ctx.Done():return ctx.Err();case <-ticker.C:}
 }
}
