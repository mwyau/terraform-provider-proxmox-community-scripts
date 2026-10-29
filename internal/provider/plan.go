package provider

import (
 "context"
 "fmt"

 "github.com/hashicorp/terraform-plugin-framework/resource"
 "github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/compat"
)
var _ resource.ResourceWithModifyPlan=(*GuestResource)(nil)

// ModifyPlan verifies selected pinned upstream sources before an installer can
// be invoked. A successful plan demonstrates source-level compatibility only.
// No installer, Proxmox mutation, or host SSH operation is performed here.
func (r *GuestResource) ModifyPlan(ctx context.Context,req resource.ModifyPlanRequest,resp *resource.ModifyPlanResponse){
 if req.Plan.Raw.IsNull() || !req.State.Raw.IsNull() {return}
 var planned guestModel
 resp.Diagnostics.Append(req.Plan.Get(ctx,&planned)...)
 if resp.Diagnostics.HasError(){return}
 if planned.Kind.IsUnknown()||planned.Script.IsUnknown()||planned.ScriptsRef.IsUnknown()||planned.CoreRef.IsUnknown()||planned.Settings.IsUnknown()||planned.GuestDefaults.IsUnknown()||planned.ScriptVars.IsUnknown(){
  resp.Diagnostics.AddWarning("Deferred compatibility inspection","One or more creation values are unknown at plan time. Source inspection must complete when Terraform resolves the values at apply; this plan does not establish compatibility.")
  return
 }
 if planned.ScriptsRef.IsNull()||planned.CoreRef.IsNull(){
  resp.Diagnostics.AddError("Immutable source pins required","New Community Scripts guests require scripts_ref and core_ref pinned to full 40-character commits.");return
 }
 if e:=planned.validate();e!=nil{resp.Diagnostics.AddError("Invalid guest",e.Error());return}
 options,vars,e:=planned.creationOptions(ctx)
 if e!=nil{resp.Diagnostics.AddError("Invalid creation settings",e.Error());return}
 _=vars
 report,e:=(compat.Inspector{}).Verify(ctx,planned.Kind.ValueString(),planned.Script.ValueString(),planned.ScriptsRef.ValueString(),planned.CoreRef.ValueString(),options)
 if e!=nil{resp.Diagnostics.AddError("Pinned script plan verification failed",fmt.Sprintf("%v. No remote changes were made.",e));return}
 for _,note:=range report.Notes{
  resp.Diagnostics.AddWarning("Pinned script source inspection",fmt.Sprintf("%s @ %s: %s",report.ScriptPath,report.ScriptsRef[:12],note))
 }
}
