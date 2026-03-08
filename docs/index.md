---
page_title: "gcpsecure Provider"
description: |-
  Provider for managing GCP resources with security-focused behavior. Sensitive
  material (e.g. service account private keys) is never stored in Terraform state.
---

# gcpsecure Provider

The gcpsecure provider manages Google Cloud Platform (GCP) resources in a way that avoids storing sensitive data in Terraform state. The provider offers multiple resources (with more planned); each is designed so that secrets are never persisted in state.

## Example Usage

```hcl
terraform {
  required_providers {
    gcpsecure = {
      source  = "registry.terraform.io/Guent4/gcpsecure"
      version = "~> 0.1"
    }
  }
}

provider "gcpsecure" {
  project = "my-gcp-project-id"
}
```

## Authentication

The provider uses [Google Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials). Set them via `gcloud auth application-default login` or `GOOGLE_APPLICATION_CREDENTIALS`.

## Schema

### Optional

- `project` (String) Default GCP project ID. Can be overridden per resource.
