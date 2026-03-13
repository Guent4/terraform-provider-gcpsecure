package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*gcpSecureProvider)(nil)

type gcpSecureProvider struct {
	version string
}

// ProviderData is passed to resources that implement ResourceWithConfigure.
type ProviderData struct {
	Project types.String
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &gcpSecureProvider{version: version}
	}
}

func (p *gcpSecureProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "gcpsecure"
	resp.Version = p.version
}

func (p *gcpSecureProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default GCP project ID. Can be overridden per resource.",
			},
		},
	}
}

func (p *gcpSecureProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config struct {
		Project types.String `tfsdk:"project"`
	}
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.ResourceData = &ProviderData{Project: config.Project}
}

func (p *gcpSecureProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *gcpSecureProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServiceAccountKeyResource,
		NewApiKeysKeyResource,
	}
}
