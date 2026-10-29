package runner

import (
 "bytes"
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "strings"
)
type ExecSystem struct{}
type boundedWriter struct{ b *bytes.Buffer; max int }
func (w boundedWriter) Write(p []byte)(int,error){
 if len(p)+w.b.Len()>w.max {return 0,errors.New("bounded host response exceeded")}
 return w.b.Write(p)
}
func (ExecSystem) Run(ctx context.Context,name string,args []string,env map[string]string)(string,error){
 cmd:=exec.CommandContext(ctx,name,args...)
 cmd.Env=os.Environ()
 for k,v:=range env {
  if k==""||strings.ContainsAny(k,"=\x00")||strings.ContainsRune(v,'\x00'){return "",errors.New("invalid environment")}
  cmd.Env=append(cmd.Env,k+"="+v)
 }
 var out bytes.Buffer
 // Capture only the small structured inspection outputs, never installer logs.
 capture:=name=="pvesh" || name=="git"&&len(args)>2&&args[2]=="rev-parse" ||
  name=="pct"&&len(args)>4&&args[0]=="exec"&&args[3]=="cat"
 if capture {cmd.Stdout=boundedWriter{b:&out,max:2<<20}}else{cmd.Stdout=io.Discard}
 cmd.Stderr=io.Discard
 if e:=cmd.Run();e!=nil{return "",errors.New("host subprocess failed (output redacted): "+e.Error())}
 if capture{return out.String(),nil}
 return "",nil
}
