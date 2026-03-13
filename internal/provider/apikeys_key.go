package provider

import (
	"context"
	"fmt"
	"strings"

	apikeys "cloud.google.com/go/apikeys/apiv2"
	apikeyspb "cloud.google.com/go/apikeys/apiv2/apikeyspb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*apikeysKeyResource)(nil)
var _ resource.ResourceWithConfigure = (*apikeysKeyResource)(nil)

type apikeysKeyResource struct {
	providerData *ProviderData
}

// ApiKeysKeyResourceModel maps the resource schema.
type ApiKeysKeyResourceModel struct {
	ID                             types.String `tfsdk:"id"`
	Name                           types.String `tfsdk:"name"`
	UID                            types.String `tfsdk:"uid"`
	DisplayName                    types.String `tfsdk:"display_name"`
	Project                        types.String `tfsdk:"project"`
	SecretManagerSecretID          types.String `tfsdk:"secret_manager_secret_id"`
	SecretManagerSecretVersionName types.String `tfsdk:"secret_manager_secret_version_name"`
	Restrictions                   types.List   `tfsdk:"restrictions"`
}

// ApiTargetModel is a single api_targets entry.
type ApiTargetModel struct {
	Service types.String `tfsdk:"service"`
	Methods types.List   `tfsdk:"methods"`
}

func NewApiKeysKeyResource() resource.Resource {
	return &apikeysKeyResource{}
}

func (r *apikeysKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apikeys_key"
}

func (r *apikeysKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates a GCP API key (equivalent to `google_apikeys_key`). The key string is **never** stored in Terraform state. It is written directly into the Secret Manager secret specified by `secret_manager_secret_id` at creation time.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "The full resource name of the key (e.g. projects/PROJECT_NUMBER/locations/global/keys/KEY_ID).",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The resource name of the key. Must be unique within the project, conform to RFC-1034, use only lower-cased letters, and have a maximum length of 63 characters. Pattern: `[a-z]([a-z0-9-]{0,61}[a-z0-9])?`",
			},
			"uid": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Output only. Unique id in UUID4 format.",
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable display name of the API key.",
			},
			"project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "GCP project ID. Defaults to the provider's project.",
			},
			"secret_manager_secret_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Secret Manager secret (ID or full name) where the API key string will be stored as a new version at creation time.",
			},
			"secret_manager_secret_version_name": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "The full resource name of the Secret Manager version storing the API key.",
			},
			"restrictions": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Key restrictions. Only one block is supported; use api_targets to restrict which APIs can be called.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"api_targets": schema.ListNestedAttribute{
							Optional: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"service": schema.StringAttribute{
										Required: true,
									},
									"methods": schema.ListAttribute{
										ElementType: types.StringType,
										Optional:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *apikeysKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if data, ok := req.ProviderData.(*ProviderData); ok {
			r.providerData = data
		}
	}
}

func apikeysKeyProject(plan *ApiKeysKeyResourceModel, providerData *ProviderData) string {
	if plan.Project.ValueString() != "" {
		return plan.Project.ValueString()
	}
	if providerData != nil && !providerData.Project.IsNull() {
		return providerData.Project.ValueString()
	}
	return ""
}

func (r *apikeysKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ApiKeysKeyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	project := apikeysKeyProject(&plan, r.providerData)
	if project == "" {
		resp.Diagnostics.AddAttributeError(path.Root("project"), "Missing project", "Set project on this resource or on the provider.")
		return
	}

	if plan.SecretManagerSecretID.IsNull() || plan.SecretManagerSecretID.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("secret_manager_secret_id"), "Required", "secret_manager_secret_id is required.")
		return
	}

	client, err := apikeys.NewClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API Keys client", err.Error())
		return
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/global", project)
	keyReq := &apikeyspb.CreateKeyRequest{
		Parent: parent,
		KeyId:  plan.Name.ValueString(),
		Key: &apikeyspb.Key{
			DisplayName: plan.DisplayName.ValueString(),
		},
	}

	restrictions, err := planRestrictionsToProto(ctx, plan.Restrictions)
	if err != nil {
		resp.Diagnostics.AddError("Invalid restrictions", err.Error())
		return
	}
	if restrictions != nil {
		keyReq.Key.Restrictions = restrictions
	}

	op, err := client.CreateKey(ctx, keyReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API key", err.Error())
		return
	}

	key, err := op.Wait(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed waiting for API key creation", err.Error())
		return
	}

	// Key string is only available via GetKeyString; it is not in the Key from CreateKey.
	keyStrResp, err := client.GetKeyString(ctx, &apikeyspb.GetKeyStringRequest{Name: key.GetName()})
	if err != nil {
		resp.Diagnostics.AddError("Failed to get API key string", err.Error())
		return
	}
	keyString := keyStrResp.GetKeyString()
	if keyString == "" {
		resp.Diagnostics.AddError("Empty key string", "GetKeyString returned an empty key string")
		return
	}

	secretVersionName, err := addPayloadToSecretManager(ctx, []byte(keyString), plan.SecretManagerSecretID.ValueString(), project, r.providerData)
	if err != nil {
		resp.Diagnostics.AddError("Failed to store API key in Secret Manager", err.Error())
		return
	}
	tflog.Info(ctx, "Stored API key in Secret Manager", map[string]any{"secret_id": plan.SecretManagerSecretID.ValueString()})

	plan.ID = types.StringValue(key.GetName())
	plan.Name = types.StringValue(plan.Name.ValueString()) // keep user-specified key id
	plan.UID = types.StringValue(key.GetUid())
	plan.SecretManagerSecretVersionName = types.StringValue(secretVersionName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apikeysKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ApiKeysKeyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := apikeys.NewClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API Keys client", err.Error())
		return
	}
	defer client.Close()

	key, err := client.GetKey(ctx, &apikeyspb.GetKeyRequest{Name: state.ID.ValueString()})
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			tflog.Info(ctx, "API key no longer exists, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read API key", err.Error())
		return
	}

	// Keep name as key id (last segment of resource name) for consistency with user input
	if parts := strings.Split(key.GetName(), "/"); len(parts) > 0 {
		state.Name = types.StringValue(parts[len(parts)-1])
	} else {
		state.Name = types.StringValue(key.GetName())
	}
	state.UID = types.StringValue(key.GetUid())
	state.DisplayName = types.StringValue(key.GetDisplayName())

	if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		exists, err := getSecretVersionExists(ctx, state.SecretManagerSecretVersionName.ValueString())
		if err != nil {
			tflog.Warn(ctx, "Failed to check Secret Manager version", map[string]any{"error": err.Error()})
		} else if !exists {
			tflog.Info(ctx, "Secret Manager version no longer exists", map[string]any{"version": state.SecretManagerSecretVersionName.ValueString()})
			state.SecretManagerSecretVersionName = types.StringNull()
		}
	}

	restrictions, err := protoRestrictionsToPlan(ctx, key.GetRestrictions())
	if err != nil {
		resp.Diagnostics.AddError("Failed to convert restrictions to plan", err.Error())
		return
	}
	state.Restrictions = restrictions

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apikeysKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ApiKeysKeyResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := apikeys.NewClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API Keys client", err.Error())
		return
	}
	defer client.Close()

	key, err := client.GetKey(ctx, &apikeyspb.GetKeyRequest{Name: state.ID.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Failed to read API key for update", err.Error())
		return
	}

	updateMask := []string{}
	if plan.DisplayName.ValueString() != state.DisplayName.ValueString() {
		updateMask = append(updateMask, "display_name")
	}

	restrictions, err := planRestrictionsToProto(ctx, plan.Restrictions)
	if err != nil {
		resp.Diagnostics.AddError("Invalid restrictions", err.Error())
		return
	}
	// Compare restrictions; if different, add to update mask.
	if !restrictionsEqual(key.GetRestrictions(), restrictions) {
		updateMask = append(updateMask, "restrictions")
	}

	if len(updateMask) > 0 {
		op, err := client.UpdateKey(ctx, &apikeyspb.UpdateKeyRequest{
			Key: &apikeyspb.Key{
				Name:         state.ID.ValueString(),
				DisplayName:  plan.DisplayName.ValueString(),
				Restrictions: restrictions,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: updateMask},
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to update API key", err.Error())
			return
		}
		_, err = op.Wait(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Failed waiting for API key update", err.Error())
			return
		}
	}

	plan.ID = state.ID
	plan.Name = state.Name
	plan.UID = state.UID
	plan.SecretManagerSecretVersionName = state.SecretManagerSecretVersionName
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apikeysKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ApiKeysKeyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.SecretManagerSecretVersionName.IsNull() && state.SecretManagerSecretVersionName.ValueString() != "" {
		if err := destroySecretVersion(ctx, state.SecretManagerSecretVersionName.ValueString()); err != nil {
			if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "DESTROYED") {
				resp.Diagnostics.AddError("Failed to destroy Secret Manager version", err.Error())
				return
			}
			tflog.Info(ctx, "Secret version already gone or destroyed", map[string]any{"version": state.SecretManagerSecretVersionName.ValueString()})
		}
	}

	client, err := apikeys.NewClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API Keys client", err.Error())
		return
	}
	defer client.Close()

	_, err = client.DeleteKey(ctx, &apikeyspb.DeleteKeyRequest{Name: state.ID.ValueString()})
	if err != nil && !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "NotFound") {
		resp.Diagnostics.AddError("Failed to delete API key", err.Error())
	}
}

// planRestrictionsToProto converts the plan restrictions list to apikeyspb.Restrictions.
func planRestrictionsToProto(ctx context.Context, list types.List) (*apikeyspb.Restrictions, error) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	var entries []struct {
		ApiTargets types.List `tfsdk:"api_targets"`
	}
	diag := list.ElementsAs(ctx, &entries, false)
	if diag.HasError() {
		return nil, fmt.Errorf("reading restrictions: %s", diag.Errors()[0].Detail())
	}
	if len(entries) == 0 {
		return nil, nil
	}
	first := entries[0]
	if first.ApiTargets.IsNull() || first.ApiTargets.IsUnknown() {
		return nil, nil
	}
	var targets []struct {
		Service types.String `tfsdk:"service"`
		Methods types.List   `tfsdk:"methods"`
	}
	if d := first.ApiTargets.ElementsAs(ctx, &targets, false); d.HasError() {
		return nil, fmt.Errorf("reading api_targets: %s", d.Errors()[0].Detail())
	}
	if len(targets) == 0 {
		return nil, nil
	}
	apiTargets := make([]*apikeyspb.ApiTarget, 0, len(targets))
	for _, t := range targets {
		var methods []string
		if !t.Methods.IsNull() && !t.Methods.IsUnknown() {
			_ = t.Methods.ElementsAs(ctx, &methods, false)
		}
		apiTargets = append(apiTargets, &apikeyspb.ApiTarget{
			Service: t.Service.ValueString(),
			Methods: methods,
		})
	}
	return &apikeyspb.Restrictions{ApiTargets: apiTargets}, nil
}

func protoRestrictionsToPlan(ctx context.Context, r *apikeyspb.Restrictions) (types.List, error) {
	restrictionObjType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"api_targets": types.ListType{ElemType: types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"service": types.StringType,
					"methods": types.ListType{ElemType: types.StringType},
				},
			}},
		},
	}
	emptyList, diags := types.ListValueFrom(ctx, restrictionObjType, []any{})
	if diags.HasError() {
		return types.ListNull(restrictionObjType), fmt.Errorf("restrictions: %s", diags.Errors()[0].Summary())
	}
	if r == nil || len(r.GetApiTargets()) == 0 {
		return emptyList, nil
	}
	targets := r.GetApiTargets()
	objs := make([]attr.Value, 0, len(targets))
	targetObjType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"service": types.StringType,
			"methods": types.ListType{ElemType: types.StringType},
		},
	}
	for _, t := range targets {
		methods, _ := types.ListValueFrom(ctx, types.StringType, t.GetMethods())
		objs = append(objs, types.ObjectValueMust(
			targetObjType.AttrTypes,
			map[string]attr.Value{
				"service": types.StringValue(t.GetService()),
				"methods": methods,
			},
		))
	}
	apiTargetsList, _ := types.ListValueFrom(ctx, targetObjType, objs)
	restrictionObj := types.ObjectValueMust(
		restrictionObjType.AttrTypes,
		map[string]attr.Value{"api_targets": apiTargetsList},
	)
	restrictionsList, diags := types.ListValueFrom(ctx, restrictionObjType, []attr.Value{restrictionObj})
	if diags.HasError() {
		return types.ListNull(restrictionObjType), fmt.Errorf("restrictions: %s", diags.Errors()[0].Summary())
	}
	return restrictionsList, nil
}

func restrictionsEqual(a, b *apikeyspb.Restrictions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	at, bt := a.GetApiTargets(), b.GetApiTargets()
	if len(at) != len(bt) {
		return false
	}
	for i := range at {
		if at[i].GetService() != bt[i].GetService() {
			return false
		}
		ma, mb := at[i].GetMethods(), bt[i].GetMethods()
		if len(ma) != len(mb) {
			return false
		}
		for j := range ma {
			if ma[j] != mb[j] {
				return false
			}
		}
	}
	return true
}
