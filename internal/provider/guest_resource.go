package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/proxmox"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/recovery"
)

var _ resource.Resource = (*GuestResource)(nil)
var _ resource.ResourceWithConfigure = (*GuestResource)(nil)
var _ resource.ResourceWithImportState = (*GuestResource)(nil)

type GuestResource struct{ client *proxmox.Client }

func NewGuestResource() resource.Resource { return &GuestResource{} }
func (r *GuestResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest"
}
func (r *GuestResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Import and inspect existing Community Scripts guests. Create and Update are disabled. Destroy retains the real guest.", Attributes: map[string]schema.Attribute{
		"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster-scoped identity, e.g. lxc/120.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"node":   schema.StringAttribute{Required: true, MarkdownDescription: "Observed Proxmox node."},
		"kind":   schema.StringAttribute{Required: true, MarkdownDescription: "lxc or vm."},
		"vmid":   schema.Int64Attribute{Required: true},
		"script": schema.StringAttribute{Required: true, MarkdownDescription: "Exact verified Community Scripts slug."},
		"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status."},
	}}
}

type guestModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	Kind   types.String `tfsdk:"kind"`
	VMID   types.Int64  `tfsdk:"vmid"`
	Script types.String `tfsdk:"script"`
	Status types.String `tfsdk:"status"`
}

func (r *GuestResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*proxmox.Client)
	if !ok {
		resp.Diagnostics.AddError("Provider configuration error", "Expected Proxmox API client.")
		return
	}
	r.client = c
}
func (r *GuestResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError("Guest creation deliberately disabled", "This provider does not execute Community Scripts yet. A crash-safe host-side write-ahead journal, cluster lock and interruption tests are required before enabling Create. Import a verified existing guest instead.")
}
func (r *GuestResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing Proxmox API client.")
		return
	}
	var state guestModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, e := recovery.ParseID(state.ID.ValueString())
	if e != nil {
		resp.Diagnostics.AddError("Invalid guest identity in state", e.Error())
		return
	}
	g, exists, e := r.client.FindGuest(ctx, id.VMID)
	if e != nil {
		resp.Diagnostics.AddError("Proxmox guest inspection failed", fmt.Sprintf("Retained state for %s; API error: %s", id, e))
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	if g.Kind != id.Kind {
		resp.Diagnostics.AddError("Guest VMID collision", fmt.Sprintf("VMID %d now belongs to type %s, expected %s. No state change.", id.VMID, g.Kind, id.Kind))
		return
	}
	script, e := proxmox.DetectScript(g)
	if e != nil {
		resp.Diagnostics.AddError("Guest script identity not verified", fmt.Sprintf("VMID %d found; retaining prior state: %s", id.VMID, e))
		return
	}
	if !state.Script.IsNull() && !state.Script.IsUnknown() && state.Script.ValueString() != script {
		resp.Diagnostics.AddError("Conflicting guest identity", fmt.Sprintf("State expects %q, Proxmox reports %q. Retaining state.", state.Script.ValueString(), script))
		return
	}
	state.ID = types.StringValue(id.String())
	state.Kind = types.StringValue(g.Kind)
	state.Node = types.StringValue(g.Node)
	state.VMID = types.Int64Value(g.VMID)
	state.Script = types.StringValue(script)
	state.Status = types.StringValue(g.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *GuestResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Guest update deliberately disabled", "This version only supports inspecting/importing existing guests.")
}
func (r *GuestResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Proxmox guest retained", "Terraform untracked this guest; no host resource was deleted.")
}
func (r *GuestResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, e := recovery.ParseID(req.ID)
	if e != nil {
		resp.Diagnostics.AddError("Invalid import ID", e.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id.String())...)
}
