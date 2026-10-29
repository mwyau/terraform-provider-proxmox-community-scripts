package provider

import (
 "context"
 "github.com/hashicorp/terraform-plugin-framework/datasource"
 "github.com/hashicorp/terraform-plugin-framework/provider"
 "github.com/hashicorp/terraform-plugin-framework/provider/schema"
 "github.com/hashicorp/terraform-plugin-framework/resource"
 "github.com/hashicorp/terraform-plugin-framework/types"
 "github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/proxmox"
)
var _ provider.Provider=(*CommunityScriptsProvider)(nil)
type CommunityScriptsProvider struct{}
func New() provider.Provider {return &CommunityScriptsProvider{}}
func (p *CommunityScriptsProvider) Metadata(_ context.Context,_ provider.MetadataRequest,r *provider.MetadataResponse) {r.TypeName="communityscripts"}
func (p *CommunityScriptsProvider) Schema(_ context.Context,_ provider.SchemaRequest,r *provider.SchemaResponse) {
 r.Schema=schema.Schema{MarkdownDescription:"Experimental read-only Community Scripts adoption; installer not enabled.",Attributes:map[string]schema.Attribute{
 "endpoint":schema.StringAttribute{Required:true,MarkdownDescription:"Proxmox HTTPS origin."},
 "api_token_id":schema.StringAttribute{Required:true,Sensitive:true},
 "api_token_secret":schema.StringAttribute{Required:true,Sensitive:true},
 "insecure":schema.BoolAttribute{Optional:true,MarkdownDescription:"Test only: disable TLS certificate validation."},
 }}
}
type config struct {Endpoint types.String `tfsdk:"endpoint"`;ID types.String `tfsdk:"api_token_id"`;Secret types.String `tfsdk:"api_token_secret"`;Insecure types.Bool `tfsdk:"insecure"`}
func (p *CommunityScriptsProvider) Configure(ctx context.Context,req provider.ConfigureRequest,r *provider.ConfigureResponse) {
 var c config
 r.Diagnostics.Append(req.Config.Get(ctx,&c)...);if r.Diagnostics.HasError(){return}
 if c.Endpoint.IsUnknown()||c.ID.IsUnknown()||c.Secret.IsUnknown()||c.Insecure.IsUnknown(){r.Diagnostics.AddError("Unknown provider settings","Provider settings must be known at apply time.");return}
 client,e:=proxmox.NewClient(c.Endpoint.ValueString(),c.ID.ValueString(),c.Secret.ValueString(),c.Insecure.ValueBool())
 if e!=nil {r.Diagnostics.AddError("Invalid Proxmox configuration",e.Error());return}
 r.ResourceData=client
}
func (p *CommunityScriptsProvider) Resources(_ context.Context) []func() resource.Resource{return []func()resource.Resource{NewGuestResource}}
func (p *CommunityScriptsProvider) DataSources(_ context.Context) []func() datasource.DataSource{return nil}
