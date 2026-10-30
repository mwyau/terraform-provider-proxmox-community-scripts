package provider

import (
	"context"
 "strings"

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
		"endpoint": schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox HTTPS endpoint, bpg/proxmox-style."},
        "api_token": schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "API token user@realm!tokenid=secret."},
        "insecure": schema.BoolAttribute{Optional: true, MarkdownDescription: "Disable TLS validation; default false."},
		"ssh_target":       schema.StringAttribute{Optional: true, MarkdownDescription: "SSH endpoint of the *single designated PVE coordinator*, including user if necessary. Requires root runner installed separately."},
	}}
}

type config struct {
	Endpoint types.String `tfsdk:"endpoint"`
 Token types.String `tfsdk:"api_token"`
 Insecure types.Bool `tfsdk:"insecure"`
	SSH      types.String `tfsdk:"ssh_target"`
}

func (p *CommunityScriptsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, r *provider.ConfigureResponse) {
	var c config
	r.Diagnostics.Append(req.Config.Get(ctx, &c)...)
	if r.Diagnostics.HasError() {
		return
	}
	if c.Endpoint.IsUnknown() || c.Token.IsUnknown() || c.SSH.IsUnknown() || c.Insecure.IsUnknown() {
		r.Diagnostics.AddError("Unknown provider configuration", "Configuration must be known before apply.")
		return
	}
	parts := strings.SplitN(c.Token.ValueString(), "=", 2)
 if len(parts)!=2 || parts[0]=="" || parts[1]=="" {
   r.Diagnostics.AddError("Invalid API token", "api_token must be user@realm!tokenid=secret")
   return
 }
 api, e := proxmox.NewClient(c.Endpoint.ValueString(), parts[0], parts[1], c.Insecure.ValueBool())
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
