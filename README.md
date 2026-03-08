# terraform-provider-gcpsecure

A Terraform provider for managing Google Cloud Platform (GCP) resources with security-focused behavior. Sensitive material (such as service account private keys) is **never** stored in Terraform state.

The provider offers (and will grow to include) multiple resources designed to avoid persisting secrets in Terraform state. This README documents the provider configuration and the resources currently available.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://go.dev/doc/install) 1.21+ (for building from source)
- Google Cloud credentials (Application Default Credentials or `GOOGLE_APPLICATION_CREDENTIALS`)

## Installation

### From the Terraform Registry

Add the provider to your Terraform configuration:

```hcl
terraform {
  required_providers {
    gcpsecure = {
      source  = "registry.terraform.io/Guent4/gcpsecure"
      version = "~> 0.1"
    }
  }
}
```

Then run `terraform init`.

### Local development

Use a local build of the provider for development or testing. Replace `<VERSION>` with your provider version (e.g. `0.1.0`).

**One-time setup** — create the local plugin directory for your OS and architecture:

```bash
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/Guent4/gcpsecure/<VERSION>/darwin_arm64/
```

**Build and install** the provider:

```bash
cd terraform-provider-gcpsecure && go build -o terraform-provider-gcpsecure .
cp terraform-provider-gcpsecure ~/.terraform.d/plugins/registry.terraform.io/Guent4/gcpsecure/<VERSION>/darwin_arm64/
```

In your Terraform configuration, require the provider with the same source and version so Terraform finds the local binary:

```hcl
terraform {
  required_providers {
    gcpsecure = {
      source  = "registry.terraform.io/Guent4/gcpsecure"
      version = "<VERSION>"
    }
  }
}
```

Then run `terraform init`.

For other OS/architectures, use the appropriate subdirectory under `~/.terraform.d/plugins/registry.terraform.io/Guent4/gcpsecure/<VERSION>/` (e.g. `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`).

## Authentication

The provider uses [Google Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials):

- **User:** `gcloud auth application-default login`
- **Service account:** set `GOOGLE_APPLICATION_CREDENTIALS` to the path of a JSON key file

Ensure the credentials have the IAM and (where applicable) Secret Manager permissions required by the resources you use.

## Provider configuration

```hcl
provider "gcpsecure" {
  project = "my-gcp-project-id"  # optional; can be set per resource
}
```

| Argument   | Type     | Required | Description                                                                 |
|------------|----------|----------|-----------------------------------------------------------------------------|
| `project`  | `string` | No       | Default GCP project ID. Can be overridden per resource.                     |

## Resources

The provider includes resources that avoid storing sensitive data in Terraform state. Additional resources may be added over time.

### `gcpsecure_service_account_key`

Creates a GCP service account key. The private key JSON is **never** stored in Terraform state. You can optionally store the key in [Secret Manager](https://cloud.google.com/secret-manager) at creation time via `secret_manager_secret_id`.

| Argument                       | Type     | Required | Description                                                                 |
|--------------------------------|----------|----------|-----------------------------------------------------------------------------|
| `service_account_id`           | `string` | **Yes**  | The service account to create the key for. Full name (`projects/.../serviceAccounts/...`) or email. |
| `project`                      | `string` | No       | GCP project ID. Defaults to the provider `project`.                         |
| `secret_manager_secret_id`     | `string` | No       | If set, the key JSON is stored as a new version in this Secret Manager secret at creation time. |
| `alias`                        | `string` | No       | Optional alias set on the Secret Manager version. Must start with a letter; cannot be `latest` or `NEW`. |
| `enabled`                      | `bool`   | No       | Whether the key is enabled. Set to `false` to disable (key is not deleted). Defaults to `true`. |

| Attribute                             | Type     | Description                                                                 |
|--------------------------------------|----------|-----------------------------------------------------------------------------|
| `id`                                 | `string` | Full resource name of the key (e.g. `projects/.../keys/...`).               |
| `name`                               | `string` | Same as `id`.                                                               |
| `key_id`                             | `string` | The key ID (last segment of the resource name).                             |
| `secret_manager_secret_version_name` | `string` | Full resource name of the Secret Manager version storing the key (if `secret_manager_secret_id` was set). |
| `alias_present`                      | `bool`   | Whether the alias currently exists on the Secret Manager secret (used to detect drift). |

## Example usage

### Service account key (no key in state)

```hcl
terraform {
  required_providers {
    google   = { source = "hashicorp/google", version = ">= 4.0" }
    gcpsecure = { source = "registry.terraform.io/Guent4/gcpsecure", version = "~> 0.1" }
  }
}

variable "project_id" {
  type        = string
  description = "GCP project ID"
}

provider "google" {
  project = var.project_id
}

provider "gcpsecure" {
  project = var.project_id
}

resource "google_service_account" "example" {
  project      = var.project_id
  account_id   = "my-app-sa"
  display_name = "Example service account"
}

resource "gcpsecure_service_account_key" "example" {
  service_account_id = google_service_account.example.email
  project            = var.project_id
  enabled            = true
}

output "key_id" {
  value = gcpsecure_service_account_key.example.key_id
}

output "key_name" {
  value = gcpsecure_service_account_key.example.name
}
```

### Service account key with Secret Manager

Store the key in Secret Manager at creation and optionally set a version alias:

```hcl
resource "google_secret_manager_secret" "sa_key" {
  project   = var.project_id
  secret_id = "my-sa-key"
  replication { auto {} }
}

resource "gcpsecure_service_account_key" "example" {
  service_account_id       = google_service_account.example.email
  project                  = var.project_id
  secret_manager_secret_id = google_secret_manager_secret.sa_key.secret_id
  alias                    = "current"  # e.g. for stable reference
  enabled                  = true
}

output "secret_version" {
  value = gcpsecure_service_account_key.example.secret_manager_secret_version_name
}
```

## Security notes

- Private key material is never written to Terraform state. Only metadata (key ID, name, Secret Manager version, etc.) is stored.
- When using `secret_manager_secret_id`, the key is written once to Secret Manager at creation; Terraform does not retain the key in memory beyond that.
- Use IAM and Secret Manager permissions to restrict who can create keys and access secrets.

## Development

- **Build:** `make build` or `go build -o terraform-provider-gcpsecure .`
- **Tests:** `make test` or `go test ./...`
- **Lint:** `make lint` (requires [golangci-lint](https://golangci-lint.run/))
- **Docs:** Provider docs live in `docs/`. See `make generate` for optional doc generation.

For local installation of the built binary, see [Local development](#local-development) above.

### Releasing

Releases are built and published via GitHub Actions when you push a version tag (e.g. `v0.1.0`). Prerequisites:

1. **GPG key:** Create a key for signing, add the **private** key to repo secrets as `GPG_PRIVATE_KEY`, and (if used) the passphrase as `PASSPHRASE`.
2. **Terraform Registry:** Add your GPG **public** key at [registry.terraform.io/settings/gpg-keys](https://registry.terraform.io/settings/gpg-keys), then use **Publish > Providers** to link this repository.

After that, pushing a tag (e.g. `git push origin v0.1.0`) will run [hashicorp/ghaction-terraform-provider-release](https://github.com/hashicorp/ghaction-terraform-provider-release) (community workflow), build binaries with GoReleaser, create a GitHub Release, and the Registry will use the release assets to publish the provider version.
