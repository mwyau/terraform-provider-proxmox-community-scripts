package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/contract"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/proxmox"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/recovery"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/runner"
)

var _ resource.Resource = (*GuestResource)(nil)
var _ resource.ResourceWithConfigure = (*GuestResource)(nil)
var _ resource.ResourceWithImportState = (*GuestResource)(nil)

type GuestResource struct{ runtime *Runtime }

func NewGuestResource() resource.Resource { return &GuestResource{} }
func (r *GuestResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest"
}
func (r *GuestResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Managed Community Scripts guest. Creation uses an audited root runner and durable journal. Destroy retains the guest by default. Only CPU/memory and explicit LXC update requests are mutable.",
		Attributes: map[string]schema.Attribute{
			"id":                        schema.StringAttribute{Computed: true, MarkdownDescription: "kind/vmid identity."},
			"node":                      schema.StringAttribute{Required: true},
			"kind":                      schema.StringAttribute{Required: true, MarkdownDescription: "lxc or vm"},
			"vmid":                      schema.Int64Attribute{Required: true},
			"script":                    schema.StringAttribute{Required: true},
			"status":                    schema.StringAttribute{Computed: true},
			"scripts_ref":               schema.StringAttribute{Optional: true, MarkdownDescription: "Immutable 40-char Community Scripts commit used only at creation."},
			"core_ref":                  schema.StringAttribute{Optional: true, MarkdownDescription: "Immutable 40-char Core commit used only at creation."},
			"disk_key":                  schema.StringAttribute{Optional: true, MarkdownDescription: "Existing disk to grow: rootfs for LXC; explicit scsiN/virtioN/sataN/ideN for VM. Never creates or moves disks."},
			"settings":                  schema.MapAttribute{ElementType: types.StringType, Optional: true, MarkdownDescription: "Creation settings (cores,memory,disk_size,hostname,bridge,vlan,storage) mapped to the upstream Core variables. Only cores/memory can be changed in-place."},
			"guest_defaults": schema.MapAttribute{ElementType: types.MapType{ElemType:types.StringType},Optional:true,MarkdownDescription:"Creation-only layered defaults (all, lxc, vm). Precedence: settings > matching kind > all > upstream script defaults."},
			"script_vars": schema.MapAttribute{ElementType:types.StringType,Optional:true,Sensitive:true,MarkdownDescription:"Creation-only app-specific environment variables. Protected names and normalized Core fields cannot be overridden; values remain in Terraform state."},
			"accept_unverified_install": schema.BoolAttribute{Optional: true, MarkdownDescription: "Explicit high-risk approval for running unverified root installation code."},
			"allow_hardware_updates":    schema.BoolAttribute{Optional: true, MarkdownDescription: "Allow explicitly requested in-place CPU/memory changes."},
			"update_request_id":         schema.StringAttribute{Optional: true, MarkdownDescription: "An explicit unique request ID for a single LXC software update. Change it to request the next update."},
			"update_scripts_ref":        schema.StringAttribute{Optional: true, MarkdownDescription: "Pinned Community Scripts revision containing tools/pve/update-apps.sh."},
			"backup_storage":            schema.StringAttribute{Optional: true, MarkdownDescription: "Required backup storage for LXC software updates."},
			"update_backup_storage": schema.StringAttribute{Optional:true,MarkdownDescription:"Ansible-compatible update backup storage; required for LXC updates. Do not set alongside backup_storage."},
			"deletion_policy":           schema.StringAttribute{Optional: true, MarkdownDescription: "retain (default) or delete. Delete additionally requires confirm_destroy."},
			"confirm_destroy":           schema.BoolAttribute{Optional: true, MarkdownDescription: "Explicit approval to delete an existing guest in the stopped state."},
			"allow_imported_destroy":    schema.BoolAttribute{Optional: true, MarkdownDescription: "Dangerous: allow deletion of an imported guest without a verified host creation journal."},
		}}
}

type guestModel struct {
	ID                   types.String `tfsdk:"id"`
	Node                 types.String `tfsdk:"node"`
	Kind                 types.String `tfsdk:"kind"`
	VMID                 types.Int64  `tfsdk:"vmid"`
	Script               types.String `tfsdk:"script"`
	Status               types.String `tfsdk:"status"`
	ScriptsRef           types.String `tfsdk:"scripts_ref"`
	CoreRef              types.String `tfsdk:"core_ref"`
	Settings             types.Map    `tfsdk:"settings"`
	GuestDefaults        types.Map    `tfsdk:"guest_defaults"`
	ScriptVars           types.Map    `tfsdk:"script_vars"`
	DiskKey              types.String `tfsdk:"disk_key"`
	Accept               types.Bool   `tfsdk:"accept_unverified_install"`
	Hardware             types.Bool   `tfsdk:"allow_hardware_updates"`
	UpdateID             types.String `tfsdk:"update_request_id"`
	UpdateRef            types.String `tfsdk:"update_scripts_ref"`
	Backup               types.String `tfsdk:"backup_storage"`
	UpdateBackup types.String `tfsdk:"update_backup_storage"`
	DeletionPolicy       types.String `tfsdk:"deletion_policy"`
	ConfirmDestroy       types.Bool   `tfsdk:"confirm_destroy"`
	AllowImportedDestroy types.Bool   `tfsdk:"allow_imported_destroy"`
}

func (m guestModel) id() recovery.ID {
	return recovery.ID{Kind: m.Kind.ValueString(), VMID: m.VMID.ValueInt64()}
}
func (r *GuestResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	v, ok := req.ProviderData.(*Runtime)
	if !ok {
		resp.Diagnostics.AddError("Provider configuration error", "Expected runtime configuration.")
		return
	}
	r.runtime = v
}
func (m guestModel) options(ctx context.Context) (map[string]string, error) {
	if m.Settings.IsNull() {
		return map[string]string{}, nil
	}
	if m.Settings.IsUnknown() {
		return nil, fmt.Errorf("unknown settings")
	}
	var values map[string]string
	if ds := m.Settings.ElementsAs(ctx, &values, false); ds.HasError() {
		return nil, fmt.Errorf("invalid settings: %s", ds.Errors()[0].Summary())
	}
	return values, nil
}
func (m guestModel) backupStorage()(string,error){
 if !m.UpdateBackup.IsNull()&&!m.Backup.IsNull(){return "",fmt.Errorf("update_backup_storage and backup_storage cannot both be set")}
 if !m.UpdateBackup.IsNull(){return m.UpdateBackup.ValueString(),nil}
 return m.Backup.ValueString(),nil
}
func (m guestModel) creationOptions(ctx context.Context)(map[string]string,map[string]string,error){
 settings,e:=m.options(ctx);if e!=nil{return nil,nil,e}
 defaults:=map[string]map[string]string{}
 if !m.GuestDefaults.IsNull(){
  if m.GuestDefaults.IsUnknown(){return nil,nil,fmt.Errorf("guest_defaults must be known for creation")}
  diags:=m.GuestDefaults.ElementsAs(ctx,&defaults,false)
  if diags.HasError(){return nil,nil,fmt.Errorf("invalid guest_defaults: %s",diags.Errors()[0].Summary())}
 }
 effective,e:=contract.ResolveDefaults(m.Kind.ValueString(),defaults,settings)
 if e!=nil{return nil,nil,e}
 vars:=map[string]string{}
 if !m.ScriptVars.IsNull(){
  if m.ScriptVars.IsUnknown(){return nil,nil,fmt.Errorf("script_vars must be known for creation")}
  diags:=m.ScriptVars.ElementsAs(ctx,&vars,false)
  if diags.HasError(){return nil,nil,fmt.Errorf("invalid script_vars: %s",diags.Errors()[0].Summary())}
 }
 if e=contract.ScriptVars(vars);e!=nil{return nil,nil,e}
 return effective,vars,nil
}
func (m guestModel) validate() error {
	if m.Node.IsNull() || m.Kind.IsNull() || m.Script.IsNull() || m.VMID.IsNull() {
		return fmt.Errorf("node, kind, VMID and script must be set")
	}
	if _, e := recovery.ParseID(m.id().String()); e != nil {
		return e
	}
	if m.Node.ValueString() == "" || m.Script.ValueString() == "" {
		return fmt.Errorf("node and script cannot be empty")
	}
	p := m.DeletionPolicy.ValueString()
	if p != "" && p != "retain" && p != "delete" {
		return fmt.Errorf("deletion_policy must be retain or delete")
	}
	return nil
}
func (r *GuestResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m guestModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if e := m.validate(); e != nil {
		resp.Diagnostics.AddError("Invalid guest", e.Error())
		return
	}
	if r.runtime == nil || r.runtime.SSH == nil {
		resp.Diagnostics.AddError("Missing host runner", "Configure ssh_target and install the audited runner on one designated coordinator.")
		return
	}
	if !m.Accept.ValueBool() {
		resp.Diagnostics.AddError("Installer approval required", "Set accept_unverified_install=true after reviewing immutable upstream commits.")
		return
	}
	if m.ScriptsRef.IsNull() || m.CoreRef.IsNull() {
		resp.Diagnostics.AddError("Source pins required", "Creating a guest requires scripts_ref and core_ref with full immutable SHAs.")
		return
	}
	if !m.UpdateID.IsNull() {
		resp.Diagnostics.AddError("Software updates are separate", "Do not set update_request_id when initially creating a guest.")
		return
	}
	_, found, e := r.runtime.API.FindGuest(ctx, m.VMID.ValueInt64())
	if e != nil {
		resp.Diagnostics.AddError("Create preflight failed", e.Error())
		return
	}
	if found {
		resp.Diagnostics.AddError("VMID collision", "A guest already exists under this VMID; explicitly import it instead of rerunning the installer.")
		return
	}
	settings, vars, e := m.creationOptions(ctx)
	if e != nil {
		resp.Diagnostics.AddError("Invalid settings", e.Error())
		return
	}
	if m.Kind.ValueString() == "vm" {
		if _, ok := settings["disk_size"]; ok && (m.DiskKey.IsNull() || m.DiskKey.ValueString() == "") {
			resp.Diagnostics.AddError("VM disk identity required", "Set disk_key to the exact existing VM disk slot if managing disk_size; the provider never guesses which disk to grow.")
			return
		}
	}
	_, e = r.runtime.SSH.Run(ctx, runner.Request{Action: "create", Node: m.Node.ValueString(), Kind: m.Kind.ValueString(), VMID: m.VMID.ValueInt64(), Script: m.Script.ValueString(), ScriptsRef: m.ScriptsRef.ValueString(), CoreRef: m.CoreRef.ValueString(), Settings: settings, ScriptVars: vars, AcceptUnverified: true})
	if e != nil {
		resp.Diagnostics.AddError("Creation uncertain", e.Error()+"; "+recovery.RecoveryHint(m.id()))
		return
	}
	g, found, e := r.runtime.API.FindGuest(ctx, m.VMID.ValueInt64())
	if e != nil || !found {
		resp.Diagnostics.AddError("Creation verification uncertain", "Host runner returned success, but API postflight failed. Inspect journal and import; never repeat installer.")
		return
	}
	if e = proxmox.ValidateExpected(g, m.Kind.ValueString(), m.Script.ValueString()); e != nil || g.Node != m.Node.ValueString() {
		resp.Diagnostics.AddError("Creation identity conflict", "Created guest failed identity verification. Inspect manually and import only if safe.")
		return
	}
	m.ID = types.StringValue(m.id().String())
	m.Status = types.StringValue(g.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *GuestResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.runtime == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing Proxmox client.")
		return
	}
	var m guestModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, e := recovery.ParseID(m.ID.ValueString())
	if e != nil {
		resp.Diagnostics.AddError("Invalid state ID", e.Error())
		return
	}
	g, found, e := r.runtime.API.FindGuest(ctx, id.VMID)
	if e != nil {
		resp.Diagnostics.AddError("Proxmox inspection failed", "Retaining state: "+e.Error())
		return
	}
	if !found {
		// A permission-filtered cluster listing can look like absence. Do not remove state automatically.
		resp.Diagnostics.AddError("Guest absent or not visible", "Cannot prove VMID absence from a permissions-filtered API response. Inspect inventory and use terraform state rm only after verification.")
		return
	}
	if g.Kind != id.Kind {
		resp.Diagnostics.AddError("VMID collision", fmt.Sprintf("VMID %d is %s, not %s; retaining state.", id.VMID, g.Kind, id.Kind))
		return
	}
	script, e := proxmox.DetectScript(g)
	if e != nil {
		resp.Diagnostics.AddError("Unverified identity", e.Error()+"; retaining state.")
		return
	}
	if !m.Script.IsNull() && m.Script.ValueString() != script {
		resp.Diagnostics.AddError("Conflicting script", fmt.Sprintf("Expected %s, found %s", m.Script.ValueString(), script))
		return
	}
	if !m.Node.IsNull() && m.Node.ValueString() != g.Node {
		resp.Diagnostics.AddError("Guest moved nodes", fmt.Sprintf("Actual node is %s; update the configured node only after verifying migration.", g.Node))
		return
	}
	m.ID = types.StringValue(id.String())
	m.Node = types.StringValue(g.Node)
	m.Kind = types.StringValue(g.Kind)
	m.VMID = types.Int64Value(g.VMID)
	m.Script = types.StringValue(script)
	m.Status = types.StringValue(g.Status)
	// Refresh observed managed compute/disk values; do not synthesize creation-only defaults.
	if !m.Settings.IsNull() {
		vals, e := m.options(ctx)
		if e != nil {
			resp.Diagnostics.AddError("Invalid saved settings", e.Error())
			return
		}
		if _, ok := vals["cores"]; ok && g.Cores > 0 {
			vals["cores"] = strconv.FormatInt(g.Cores, 10)
		}
		if _, ok := vals["memory"]; ok && g.Memory > 0 {
			vals["memory"] = strconv.FormatInt(g.Memory, 10)
		}
		if _, ok := vals["disk_size"]; ok {
			key := m.DiskKey.ValueString()
			if key == "" && g.Kind == "lxc" {
				key = "rootfs"
			}
			cfg, found := g.Disks[key]
			if !found {
				resp.Diagnostics.AddError("Managed disk missing", fmt.Sprintf("Disk %q absent; retaining state.", key))
				return
			}
			size, e := proxmox.DiskGiB(cfg)
			if e != nil {
				resp.Diagnostics.AddError("Disk size unavailable", e.Error())
				return
			}
			vals["disk_size"] = strconv.FormatFloat(size, 'f', -1, 64)
		}
		nv, ds := types.MapValueFrom(ctx, types.StringType, vals)
		resp.Diagnostics.Append(ds...)
		if resp.Diagnostics.HasError() {
			return
		}
		m.Settings = nv
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *GuestResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.runtime == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing runtime.")
		return
	}
	var old, plan guestModel
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if e := plan.validate(); e != nil {
		resp.Diagnostics.AddError("Invalid guest", e.Error())
		return
	}
	if old.Kind != plan.Kind || old.VMID != plan.VMID || old.Node != plan.Node || old.Script != plan.Script || old.ScriptsRef != plan.ScriptsRef || old.CoreRef != plan.CoreRef || old.DiskKey != plan.DiskKey || !old.ScriptVars.Equal(plan.ScriptVars) {
		resp.Diagnostics.AddError("Immutable guest identity/source", "Changing node, VMID, kind, script_vars, disk key, or installation refs is not supported. Import or use an explicit migration plan.")
		return
	}
	g, found, e := r.runtime.API.FindGuest(ctx, plan.VMID.ValueInt64())
	if e != nil || !found {
		resp.Diagnostics.AddError("Cannot inspect guest", "Refusing update without positive guest inspection.")
		return
	}
	if e = proxmox.ValidateExpected(g, plan.Kind.ValueString(), plan.Script.ValueString()); e != nil || g.Node != plan.Node.ValueString() {
		resp.Diagnostics.AddError("Conflicting identity", "Refusing to update a different guest.")
		return
	}
	oldOpts, e := old.options(ctx)
	if e != nil {
		resp.Diagnostics.AddError("Invalid previous settings", e.Error())
		return
	}
	newOpts, e := plan.options(ctx)
	if e != nil {
		resp.Diagnostics.AddError("Invalid desired settings", e.Error())
		return
	}
	delta := map[string]string{}
	if !old.Settings.Equal(plan.Settings) {
		for k, v := range newOpts {
			if oldOpts[k] != v {
				if k != "cores" && k != "memory" && k != "disk_size" {
					resp.Diagnostics.AddError("Creation-only setting", fmt.Sprintf("%s cannot be changed in place", k))
					return
				}
				delta[k] = v
			}
		}
		for k := range oldOpts {
			if _, ok := newOpts[k]; !ok {
				resp.Diagnostics.AddError("Settings removal unsupported", "Removing a previously managed setting is not an approved mutation.")
				return
			}
		}
		if len(delta) > 0 && !plan.Hardware.ValueBool() {
			resp.Diagnostics.AddError("Hardware update approval required", "Set allow_hardware_updates=true to reconcile cores or memory.")
			return
		}
	}
	planBackup,backupErr:=plan.backupStorage()
 if backupErr!=nil{resp.Diagnostics.AddError("Conflicting update options",backupErr.Error());return}
 oldBackup,oldErr:=old.backupStorage()
 if oldErr!=nil{resp.Diagnostics.AddError("Conflicting prior update options",oldErr.Error());return}
 if plan.UpdateID.Equal(old.UpdateID) && (!plan.UpdateRef.Equal(old.UpdateRef) || planBackup!=oldBackup) {
		resp.Diagnostics.AddError("Update request identity is immutable", "Changing the pinned updater or backup storage requires a new update_request_id; never change the meaning of a completed request.")
		return
	}
	newUpdate := !plan.UpdateID.IsNull() && plan.UpdateID.ValueString() != "" && !plan.UpdateID.Equal(old.UpdateID)
	if newUpdate && (plan.UpdateRef.IsNull() || planBackup=="" || r.runtime.SSH == nil) {
		resp.Diagnostics.AddError("Missing update prerequisites", "Set update_scripts_ref, update_backup_storage, and provider ssh_target.")
		return
	}
	if len(delta) > 0 {
		if disk, ok := delta["disk_size"]; ok {
			delete(delta, "disk_size")
			key := plan.DiskKey.ValueString()
			if key == "" && g.Kind == "lxc" {
				key = "rootfs"
			}
			if e = r.runtime.API.GrowDisk(ctx, g, key, disk); e != nil {
				resp.Diagnostics.AddError("Disk growth failed or refused", "Inspect disk and state before retry: "+e.Error())
				return
			}
		}
		if len(delta) > 0 {
			// A previous resize can change the Proxmox config digest: re-read before writes.
			g, found, e = r.runtime.API.FindGuest(ctx, plan.VMID.ValueInt64())
			if e != nil || !found {
				resp.Diagnostics.AddError("Cannot re-inspect guest", "Disk may have grown; inspect manually before retry.")
				return
			}
			if e = r.runtime.API.ApplyCompute(ctx, g, delta); e != nil {
				resp.Diagnostics.AddError("Hardware update failed or uncertain", "Inspect actual Proxmox config before retry: "+e.Error())
				return
			}
		}
	}
	if newUpdate {
		_, e = r.runtime.SSH.Run(ctx, runner.Request{Action: "update", Node: plan.Node.ValueString(), Kind: plan.Kind.ValueString(), VMID: plan.VMID.ValueInt64(), Script: plan.Script.ValueString(), ScriptsRef: plan.UpdateRef.ValueString(), UpdateRequestID: plan.UpdateID.ValueString(), BackupStorage: planBackup})
		if e != nil {
			resp.Diagnostics.AddError("Application update uncertain", "Inspect the host request journal before retry: "+e.Error())
			return
		}
	}
	g, found, e = r.runtime.API.FindGuest(ctx, plan.VMID.ValueInt64())
	if e != nil || !found {
		resp.Diagnostics.AddError("Post-update verification failed", "State not advanced; inspect before retrying.")
		return
	}
	if e = proxmox.ValidateExpected(g, plan.Kind.ValueString(), plan.Script.ValueString()); e != nil {
		resp.Diagnostics.AddError("Post-update identity conflict", e.Error())
		return
	}
	plan.ID = old.ID
	plan.Status = types.StringValue(g.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guestModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.DeletionPolicy.IsNull() || state.DeletionPolicy.ValueString() == "" || state.DeletionPolicy.ValueString() == "retain" {
		resp.Diagnostics.AddWarning("Guest retained", "Only Terraform tracking was removed; Proxmox guest remains.")
		return
	}
	if state.DeletionPolicy.ValueString() != "delete" || !state.ConfirmDestroy.ValueBool() {
		resp.Diagnostics.AddError("Destroy not approved", "Set deletion_policy=delete and confirm_destroy=true before planning destroy.")
		return
	}
	if r.runtime == nil || r.runtime.SSH == nil {
		resp.Diagnostics.AddError("Runner unavailable", "ssh_target required to destroy a guest.")
		return
	}
	id, e := recovery.ParseID(state.ID.ValueString())
	if e != nil {
		resp.Diagnostics.AddError("Invalid state", e.Error())
		return
	}
	g, found, e := r.runtime.API.FindGuest(ctx, id.VMID)
	if e != nil || !found {
		resp.Diagnostics.AddError("Cannot verify guest", "Refusing destructive deletion when inventory is inaccessible or ambiguous.")
		return
	}
	if e = proxmox.ValidateExpected(g, id.Kind, state.Script.ValueString()); e != nil || g.Node != state.Node.ValueString() || g.Status != "stopped" {
		resp.Diagnostics.AddError("Destroy safety check failed", "Require exact VMID/type/node/script and a stopped guest.")
		return
	}
	_, e = r.runtime.SSH.Run(ctx, runner.Request{Action: "delete", Node: state.Node.ValueString(), Kind: id.Kind, VMID: id.VMID, Script: state.Script.ValueString(), ConfirmDestroy: true, AllowImportedDestroy: state.AllowImportedDestroy.ValueBool()})
	if e != nil {
		resp.Diagnostics.AddError("Destroy uncertain", "Do not repeat automatically; inspect host-side deletion journal: "+e.Error())
		return
	}
}
func (r *GuestResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, e := recovery.ParseID(req.ID)
	if e != nil {
		resp.Diagnostics.AddError("Invalid import ID", e.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id.String())...)
}

var _ = strings.TrimSpace
