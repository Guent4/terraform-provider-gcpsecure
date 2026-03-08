package provider

import (
	"context"
	"fmt"
	"strings"

	iam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*serviceAccountKeyResource)(nil)
var _ resource.ResourceWithConfigure = (*serviceAccountKeyResource)(nil)

type serviceAccountKeyResource struct {
	providerData *ProviderData
}

type ServiceAccountKeyResourceModel struct {
	ID                             types.String `tfsdk:"id"`
	Name                           types.String `tfsdk:"name"`
	KeyID                          types.String `tfsdk:"key_id"`
	ServiceAccountID               types.String `tfsdk:"service_account_id"`
	Project                        types.String `tfsdk:"project"`
	SecretManagerSecret            types.String `tfsdk:"secret_manager_secret_id"`
	SecretManagerSecretVersionName types.String `tfsdk:"secret_manager_secret_version_name"`
	Alias                          types.String `tfsdk:"alias"`
	AliasPresent                   types.Bool   `tfsdk:"alias_present"`
	Enabled                        types.Bool   `tfsdk:"enabled"`
}

func NewServiceAccountKeyResource() resource.Resource {
	return &serviceAccountKeyResource{}
}

func (r *serviceAccountKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account_key"
}

func (r *serviceAccountKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates a GCP service account key. The private key JSON is **never** stored in Terraform state. Use `secret_manager_secret_id` to store the key in Secret Manager at creation time.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key_id": schema.StringAttribute{
				Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The service account to create the key for. Can be the full name or just the email.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "GCP project ID. Defaults to the provider's project.",
			},
			"secret_manager_secret_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "If set, the key JSON will be stored as a new version in this Secret Manager secret at creation time.",
			},
			"alias": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional alias set as a Secret Manager version alias. Must start with a letter; cannot be 'latest' or 'NEW'.",
			},
			"alias_present": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "True if the alias currently exists on the Secret Manager secret (used to detect drift). Aliases are only created or restored during Update.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"secret_manager_secret_version_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The full resource name of the Secret Manager version storing the key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the key is enabled. Set to `false` to disable the key (it will not be deleted). Defaults to `true`.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *serviceAccountKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if data, ok := req.ProviderData.(*ProviderData); ok {
			r.providerData = data
		}
	}
}

func normalizeServiceAccountName(serviceAccountID, resourceProject string, providerData *ProviderData) string {
	if strings.HasPrefix(serviceAccountID, "projects/") {
		return serviceAccountID
	}
	project := resourceProject
	if project == "" && providerData != nil && !providerData.Project.IsNull() {
		project = providerData.Project.ValueString()
	}
	if project == "" {
		return ""
	}
	if strings.Contains(serviceAccountID, "@") {
		return fmt.Sprintf("projects/%s/serviceAccounts/%s", project, serviceAccountID)
	}
	return fmt.Sprintf("projects/%s/serviceAccounts/%s@%s.iam.gserviceaccount.com", project, serviceAccountID, project)
}

func (r *serviceAccountKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountKeyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceAccountName := normalizeServiceAccountName(plan.ServiceAccountID.ValueString(), plan.Project.ValueString(), r.providerData)
	if serviceAccountName == "" {
		resp.Diagnostics.AddAttributeError(path.Root("service_account_id"), "Missing project", "Either provide project on this resource or on the provider when using service account email.")
		return
	}

	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create IAM client", err.Error())
		return
	}

	key, err := iamService.Projects.ServiceAccounts.Keys.Create(serviceAccountName, &iam.CreateServiceAccountKeyRequest{}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create service account key", err.Error())
		return
	}

	var secretVersionName string
	if !plan.SecretManagerSecret.IsNull() && plan.SecretManagerSecret.ValueString() != "" {
		secretVersionName, err = addKeyToSecretManagerImpl(ctx, key.PrivateKeyData, plan.SecretManagerSecret.ValueString(), plan.Project.ValueString(), r.providerData)
		if err != nil {
			resp.Diagnostics.AddError("Failed to store key in Secret Manager", err.Error())
			return
		}
		plan.SecretManagerSecretVersionName = types.StringValue(secretVersionName)
		tflog.Info(ctx, "Stored service account key in Secret Manager", map[string]any{"secret_id": plan.SecretManagerSecret.ValueString()})
	} else {
		plan.SecretManagerSecretVersionName = types.StringNull()
		plan.AliasPresent = types.BoolValue(false)
	}

	enabled := true
	if !plan.Enabled.IsNull() {
		enabled = plan.Enabled.ValueBool()
	}
	if !enabled {
		_, err = iamService.Projects.ServiceAccounts.Keys.Disable(key.Name, &iam.DisableServiceAccountKeyRequest{}).Context(ctx).Do()
		if err != nil {
			resp.Diagnostics.AddError("Failed to disable service account key", err.Error())
			return
		}
		if secretVersionName != "" {
			if disableErr := disableSecretVersion(ctx, secretVersionName); disableErr != nil {
				resp.Diagnostics.AddError("Failed to disable Secret Manager version", disableErr.Error())
				return
			}
		}
	}
	plan.Enabled = types.BoolValue(enabled)

	if secretVersionName != "" && !plan.Alias.IsNull() && plan.Alias.ValueString() != "" {
		parentSecretName := plan.SecretManagerSecret.ValueString()
		if !strings.HasPrefix(parentSecretName, "projects/") {
			project := plan.Project.ValueString()
			if project == "" && r.providerData != nil && !r.providerData.Project.IsNull() {
				project = r.providerData.Project.ValueString()
			}
			parentSecretName = fmt.Sprintf("projects/%s/secrets/%s", project, parentSecretName)
		}
		if aliasErr := setSecretVersionAlias(ctx, parentSecretName, secretVersionName, plan.Alias.ValueString()); aliasErr != nil {
			resp.Diagnostics.AddError("Failed to set version alias on secret", aliasErr.Error())
			return
		}
		plan.AliasPresent = types.BoolValue(true)
		tflog.Info(ctx, "Set version alias on secret", map[string]any{"alias": plan.Alias.ValueString()})
	} else if secretVersionName != "" {
		plan.AliasPresent = types.BoolValue(false)
	}

	plan.ID = types.StringValue(key.Name)
	plan.Name = types.StringValue(key.Name)
	if parts := strings.Split(key.Name, "/"); len(parts) >= 2 {
		plan.KeyID = types.StringValue(parts[len(parts)-1])
	} else {
		plan.KeyID = types.StringValue(key.Name)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceAccountKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountKeyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create IAM client", err.Error())
		return
	}

	key, err := iamService.Projects.ServiceAccounts.Keys.Get(state.Name.ValueString()).Context(ctx).Do()
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			tflog.Info(ctx, "Service account key no longer exists, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read service account key", err.Error())
		return
	}

	if parts := strings.Split(key.Name, "/"); len(parts) >= 2 {
		state.KeyID = types.StringValue(parts[len(parts)-1])
	} else {
		state.KeyID = types.StringValue(key.Name)
	}
	state.Enabled = types.BoolValue(!key.Disabled)

	if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		exists, err := getSecretVersionExists(ctx, state.SecretManagerSecretVersionName.ValueString())
		if err != nil {
			tflog.Warn(ctx, "Failed to check Secret Manager version, keeping state", map[string]any{"error": err.Error()})
		} else if !exists {
			tflog.Info(ctx, "Secret Manager version no longer exists or was destroyed, clearing from state to show drift", map[string]any{"version": state.SecretManagerSecretVersionName.ValueString()})
			state.SecretManagerSecretVersionName = types.StringNull()
		} else if !state.Alias.IsNull() && state.Alias.ValueString() != "" {
			versionName := state.SecretManagerSecretVersionName.ValueString()
			idx := strings.LastIndex(versionName, "/versions/")
			if idx != -1 {
				parentSecretName := versionName[:idx]
				aliasExists, aliasErr := secretVersionAliasExists(ctx, parentSecretName, versionName, state.Alias.ValueString())
				if aliasErr != nil {
					tflog.Warn(ctx, "Failed to check version alias", map[string]any{"error": aliasErr.Error()})
				} else {
					state.AliasPresent = types.BoolValue(aliasExists)
				}
			}
		} else {
			state.AliasPresent = types.BoolValue(false)
		}
	} else {
		state.AliasPresent = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serviceAccountKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ServiceAccountKeyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Restore alias if missing (only in Update).
	if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" &&
		!plan.Alias.IsNull() && plan.Alias.ValueString() != "" {
		versionName := state.SecretManagerSecretVersionName.ValueString()
		idx := strings.LastIndex(versionName, "/versions/")
		if idx != -1 {
			parentSecretName := versionName[:idx]
			exists, err := secretVersionAliasExists(ctx, parentSecretName, versionName, plan.Alias.ValueString())
			if err != nil {
				tflog.Warn(ctx, "Failed to check version alias, skipping restore", map[string]any{"error": err.Error()})
			} else if !exists {
				if aliasErr := setSecretVersionAlias(ctx, parentSecretName, versionName, plan.Alias.ValueString()); aliasErr != nil {
					resp.Diagnostics.AddError("Failed to restore version alias on secret", aliasErr.Error())
					return
				}
				tflog.Info(ctx, "Restored version alias on secret", map[string]any{"alias": plan.Alias.ValueString()})
			}
		}
	}

	// Sync alias when alias changed.
	planAlias := ""
	if !plan.Alias.IsNull() {
		planAlias = plan.Alias.ValueString()
	}
	stateAlias := ""
	if !state.Alias.IsNull() {
		stateAlias = state.Alias.ValueString()
	}
	if planAlias != stateAlias && !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		versionName := state.SecretManagerSecretVersionName.ValueString()
		idx := strings.LastIndex(versionName, "/versions/")
		if idx != -1 {
			parentSecretName := versionName[:idx]
			if aliasErr := setSecretVersionAlias(ctx, parentSecretName, versionName, planAlias); aliasErr != nil {
				resp.Diagnostics.AddError("Failed to update version alias on secret", aliasErr.Error())
				return
			}
		}
	}

	// Sync enabled.
	planEnabled := true
	if !plan.Enabled.IsNull() {
		planEnabled = plan.Enabled.ValueBool()
	}
	stateEnabled := true
	if !state.Enabled.IsNull() {
		stateEnabled = state.Enabled.ValueBool()
	}
	if planEnabled != stateEnabled {
		iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
		if err != nil {
			resp.Diagnostics.AddError("Failed to create IAM client", err.Error())
			return
		}
		if planEnabled {
			_, err = iamService.Projects.ServiceAccounts.Keys.Enable(state.Name.ValueString(), &iam.EnableServiceAccountKeyRequest{}).Context(ctx).Do()
			if err != nil {
				resp.Diagnostics.AddError("Failed to update key enabled state", err.Error())
				return
			}
			if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
				if enableErr := enableSecretVersion(ctx, state.SecretManagerSecretVersionName.ValueString()); enableErr != nil {
					resp.Diagnostics.AddError("Failed to enable Secret Manager version", enableErr.Error())
					return
				}
			}
		} else {
			_, err = iamService.Projects.ServiceAccounts.Keys.Disable(state.Name.ValueString(), &iam.DisableServiceAccountKeyRequest{}).Context(ctx).Do()
			if err != nil {
				resp.Diagnostics.AddError("Failed to update key enabled state", err.Error())
				return
			}
			if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
				if disableErr := disableSecretVersion(ctx, state.SecretManagerSecretVersionName.ValueString()); disableErr != nil {
					resp.Diagnostics.AddError("Failed to disable Secret Manager version", disableErr.Error())
					return
				}
			}
		}
	}

	plan.SecretManagerSecretVersionName = state.SecretManagerSecretVersionName
	if !plan.Alias.IsNull() && plan.Alias.ValueString() != "" && !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		plan.AliasPresent = types.BoolValue(true)
	} else {
		plan.AliasPresent = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceAccountKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountKeyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Destroy the Secret Manager version first (if we stored the key there).
	if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		versionName := state.SecretManagerSecretVersionName.ValueString()
		if destroyErr := destroySecretVersion(ctx, versionName); destroyErr != nil {
			if !strings.Contains(destroyErr.Error(), "404") && !strings.Contains(destroyErr.Error(), "NotFound") && !strings.Contains(destroyErr.Error(), "DESTROYED") {
				resp.Diagnostics.AddError("Failed to destroy Secret Manager version", destroyErr.Error())
				return
			}
			tflog.Info(ctx, "Secret version already gone or destroyed, continuing", map[string]any{"version": versionName})
		} else {
			tflog.Info(ctx, "Destroyed Secret Manager version", map[string]any{"version": versionName})
		}
	}

	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create IAM client", err.Error())
		return
	}

	_, err = iamService.Projects.ServiceAccounts.Keys.Delete(state.Name.ValueString()).Context(ctx).Do()
	if err != nil && !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "NotFound") {
		resp.Diagnostics.AddError("Failed to delete service account key", err.Error())
	}
}
