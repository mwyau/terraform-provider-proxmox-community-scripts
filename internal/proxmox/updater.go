package proxmox

import (
 "fmt"
 "regexp"
 "strings"
)
var updaterSlug=regexp.MustCompile(`(?m)^\s*(?:export\s+)?SCRIPT_SLUG=["']?([a-z0-9][a-z0-9-]*)["']?\s*$`)
var updaterLegacy=regexp.MustCompile(`/ct/([a-z0-9][a-z0-9-]*)\.sh(?:[^a-z0-9-]|$)`)
// DetectUpdaterSlug uses the explicit upstream generated slug. Legacy paths
// provide weaker but usable evidence only when no SCRIPT_SLUG exists.
func DetectUpdaterSlug(text string)(string,error){
 if len(text)>64*1024{return "",fmt.Errorf("updater entrypoint exceeds limit")}
 matches:=updaterSlug.FindAllStringSubmatch(text,-1)
 if len(matches)==0{matches=updaterLegacy.FindAllStringSubmatch(text,-1)}
 unique:=map[string]bool{}
 for _,m:=range matches{unique[strings.ToLower(m[1])]=true}
 if len(unique)!=1{return "",fmt.Errorf("updater slug unknown or conflicting")}
 for k:=range unique{return k,nil}
 return "",fmt.Errorf("unreachable updater state")
}
