package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// addKeyToSecretManagerImpl decodes the base64 PrivateKeyData from the IAM API
// and adds it as a new version to the given Secret Manager secret.
// Returns the full resource name of the created secret version (e.g. projects/.../secrets/.../versions/N).
func addKeyToSecretManagerImpl(ctx context.Context, base64KeyData, secretID, resourceProject string, providerData *ProviderData) (versionName string, err error) {
	payload, err := base64.StdEncoding.DecodeString(base64KeyData)
	if err != nil {
		return "", fmt.Errorf("decode key data: %w", err)
	}

	project := resourceProject
	if project == "" && providerData != nil && !providerData.Project.IsNull() {
		project = providerData.Project.ValueString()
	}
	if project == "" {
		return "", fmt.Errorf("project is required to store key in Secret Manager")
	}

	parent := secretID
	if !strings.HasPrefix(secretID, "projects/") {
		parent = fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()

	ver, err := client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: parent,
		Payload: &secretmanagerpb.SecretPayload{
			Data: payload,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add secret version: %w", err)
	}
	return ver.Name, nil
}

// secretVersionAliasExists returns true if the secret has the given alias pointing to the given version.
func secretVersionAliasExists(ctx context.Context, parentSecretName, versionName, alias string) (bool, error) {
	if alias == "" {
		return false, nil
	}
	prefix := "/versions/"
	idx := strings.LastIndex(versionName, prefix)
	if idx == -1 {
		return false, nil
	}
	versionNumStr := versionName[idx+len(prefix):]
	versionNum, err := strconv.ParseInt(versionNumStr, 10, 64)
	if err != nil {
		return false, err
	}
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return false, fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	secret, err := client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: parentSecretName})
	if err != nil {
		return false, fmt.Errorf("get secret: %w", err)
	}
	return secret.VersionAliases[alias] == versionNum, nil
}

// setSecretVersionAlias sets the Secret Manager version alias for the given version.
func setSecretVersionAlias(ctx context.Context, parentSecretName, versionName, alias string) error {
	prefix := "/versions/"
	idx := strings.LastIndex(versionName, prefix)
	if idx == -1 {
		return fmt.Errorf("invalid version name %q", versionName)
	}
	versionNumStr := versionName[idx+len(prefix):]
	versionNum, err := strconv.ParseInt(versionNumStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse version number from %q: %w", versionName, err)
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	secret, err := client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: parentSecretName})
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	versionAliases := make(map[string]int64)
	for k, v := range secret.VersionAliases {
		if v != versionNum {
			versionAliases[k] = v
		}
	}
	if alias != "" {
		versionAliases[alias] = versionNum
	}
	_, err = client.UpdateSecret(ctx, &secretmanagerpb.UpdateSecretRequest{
		Secret: &secretmanagerpb.Secret{
			Name:           parentSecretName,
			VersionAliases: versionAliases,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"version_aliases"}},
	})
	if err != nil {
		return fmt.Errorf("update secret version_aliases: %w", err)
	}
	return nil
}

// getSecretVersionExists returns true if the secret version exists and is not destroyed.
func getSecretVersionExists(ctx context.Context, versionName string) (bool, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return false, fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	ver, err := client.GetSecretVersion(ctx, &secretmanagerpb.GetSecretVersionRequest{Name: versionName})
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, fmt.Errorf("get secret version: %w", err)
	}
	if ver.State == secretmanagerpb.SecretVersion_DESTROYED {
		return false, nil
	}
	return true, nil
}

// disableSecretVersion disables the given Secret Manager version.
func disableSecretVersion(ctx context.Context, versionName string) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	_, err = client.DisableSecretVersion(ctx, &secretmanagerpb.DisableSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return fmt.Errorf("disable secret version: %w", err)
	}
	return nil
}

// enableSecretVersion re-enables the given Secret Manager version.
func enableSecretVersion(ctx context.Context, versionName string) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	_, err = client.EnableSecretVersion(ctx, &secretmanagerpb.EnableSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return fmt.Errorf("enable secret version: %w", err)
	}
	return nil
}

// destroySecretVersion permanently destroys the given Secret Manager version (irreversible).
func destroySecretVersion(ctx context.Context, versionName string) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create Secret Manager client: %w", err)
	}
	defer client.Close()
	_, err = client.DestroySecretVersion(ctx, &secretmanagerpb.DestroySecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return fmt.Errorf("destroy secret version: %w", err)
	}
	return nil
}
