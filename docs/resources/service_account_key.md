---
page_title: "gcpsecure_service_account_key Resource - gcpsecure"
description: |-
  Creates a GCP service account key. The private key JSON is never stored in
  Terraform state. Optionally store the key in Secret Manager at creation time.
---

# gcpsecure_service_account_key (Resource)

Creates a GCP service account key. The private key JSON is **never** stored in Terraform state. You can optionally store the key in [Secret Manager](https://cloud.google.com/secret-manager) at creation time via `secret_manager_secret_id`.

## Example Usage

### Basic (no key in state)

```hcl
resource "gcpsecure_service_account_key" "example" {
  service_account_id = google_service_account.example.email
  project            = var.project_id
  enabled            = true
}
```

### With Secret Manager

```hcl
resource "gcpsecure_service_account_key" "example" {
  service_account_id       = google_service_account.example.email
  project                  = var.project_id
  secret_manager_secret_id = google_secret_manager_secret.sa_key.secret_id
  alias                    = "current"
  enabled                  = true
}
```

## Schema

### Required

- `service_account_id` (String) The service account to create the key for. Full name (`projects/.../serviceAccounts/...`) or email.

### Optional

- `project` (String) GCP project ID. Defaults to the provider `project`.
- `secret_manager_secret_id` (String) If set, the key JSON is stored as a new version in this Secret Manager secret at creation time.
- `alias` (String) Optional alias set on the Secret Manager version. Must start with a letter; cannot be `latest` or `NEW`.
- `enabled` (Boolean) Whether the key is enabled. Set to `false` to disable (key is not deleted). Defaults to `true`.

### Read-Only

- `id` (String) Full resource name of the key (e.g. `projects/.../keys/...`).
- `name` (String) Same as `id`.
- `key_id` (String) The key ID (last segment of the resource name).
- `secret_manager_secret_version_name` (String) Full resource name of the Secret Manager version storing the key (if `secret_manager_secret_id` was set).
- `alias_present` (Boolean) Whether the alias currently exists on the Secret Manager secret (used to detect drift).
