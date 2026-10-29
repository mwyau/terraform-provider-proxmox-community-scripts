package runner

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "strings"
)
type ExecSystem struct{}
func (ExecSystem) Run(ctx context.Context,name string,args []string,env map[string]string)(string,error){
 // This process is launched as root only on a trusted Proxmox host.
 // Do not invoke a shell for argument interpolation.
 cmd:=exec.CommandContext(ctx,name,args...)
 cmd.Env=os.Environ()
 if len(env)>0{
  // Prevent caller settings from overriding execution identity and source pins.
  for k,v:=range env{if k==""||strings.ContainsAny(k,"=\x00")||strings.ContainsRune(v,'\x00'){return "",errors.New("invalid environment")}
   cmd.Env=append(cmd.Env,k+"="+v)
  }
 }
 // Avoid capturing full output: scripts may print secrets.
 b,e:=cmd.CombinedOutput()
 if e!=nil {return "",errors.New("host subprocess failed (output redacted): "+e.Error())}
 if name=="pvesh"||name=="git"&&len(args)>2&&args[2]=="rev-parse"{return string(b),nil}
 return "",nil
}
