package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/proxmox"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/transport"
)

var _ provider.Provider = (*CommunityScriptsProvider)(nil)

type Runtime struct {
	API *proxmox.Client
	SSH *transport.SSH
}
type CommunityScriptsProvider struct{}

func New() provider.Provider { return &CommunityScriptsProvider{} }
func (p *CommunityScriptsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, r *provider.MetadataResponse) {
	r.TypeName = "communityscripts"
}
func (p *CommunityScriptsProvider) Schema(_ context.Context, _ provider.SchemaRequest, r *provider.SchemaResponse) {
	r.Schema = schema.Schema{MarkdownDescription: "Community Scripts guest lifecycle. Privileged operations use a separately installed audited Proxmox runner.", Attributes: map[string]schema.Attribute{
		"api_host":         schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox HTTPS origin or hostname; matches Ansible."},
		"api_token_id":     schema.StringAttribute{Required: true, Sensitive: true},
		"api_token_secret": schema.StringAttribute{Required: true, Sensitive: true},
		"validate_certs":   schema.BoolAttribute{Optional: true, MarkdownDescription: "Verify Proxmox TLS certificates (default true), matching the Ansible api_host/validate_certs convention."},
		"ssh_target":       schema.StringAttribute{Optional: true, MarkdownDescription: "SSH endpoint of the *single designated PVE coordinator*, including user if necessary. Requires root runner installed separately."},
	}}
}

type config struct {
	Host     types.String `tfsdk:"api_host"`
	ID       types.String `tfsdk:"api_token_id"`
	Secret   types.String `tfsdk:"api_token_secret"`
	Validate types.Bool   `tfsdk:"validate_certs"`
	SSH      types.String `tfsdk:"ssh_target"`
}

func (p *CommunityScriptsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, r *provider.ConfigureResponse) {
	var c config
	r.Diagnostics.Append(req.Config.Get(ctx, &c)...)
	if r.Diagnostics.HasError() {
		return
	}
	if c.Host.IsUnknown() || c.ID.IsUnknown() || c.Secret.IsUnknown() || c.SSH.IsUnknown() || c.Validate.IsUnknown() {
		r.Diagnostics.AddError("Unknown provider configuration", "Configuration must be known before apply.")
		return
	}
	api, e := proxmox.NewClient(c.Host.ValueString(), c.ID.ValueString(), c.Secret.ValueString(), !c.Validate.IsNull() && !c.Validate.ValueBool())
	if e != nil {
		r.Diagnostics.AddError("Invalid Proxmox endpoint", e.Error())
		return
	}
	ssh, e := transport.NewSSH(c.SSH.ValueString())
	if e != nil {
		r.Diagnostics.AddError("Invalid SSH target", e.Error())
		return
	}
	r.ResourceData = &Runtime{API: api, SSH: ssh}
}
func (p *CommunityScriptsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewGuestResource}
}
func (p *CommunityScriptsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
