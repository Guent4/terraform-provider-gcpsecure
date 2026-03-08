# Example: Create a GCP service account key without storing the key JSON in state.
#
# Prerequisites:
# - Build the provider: go build -o terraform-provider-gcpsecure ..
# - Configure provider (e.g. dev_overrides in ~/.terraformrc)
# - gcloud auth application-default login (or set GOOGLE_APPLICATION_CREDENTIALS)

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0"
    }
    gcpsecure = {
      source  = "registry.terraform.io/your-org/gcpsecure"
      version = "0.1.0"
    }
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

# Create a service account (using the official Google provider)
resource "google_service_account" "example" {
  project      = var.project_id
  account_id   = "gcpsecure-example-sa"
  display_name = "Example SA for gcpsecure provider"
}

# Create a key with gcpsecure provider - the private key is NEVER stored in state
resource "gcpsecure_service_account_key" "example" {
  service_account_id = google_service_account.example.email
  project            = var.project_id
  enabled            = true # set to false to disable the key (not delete it)
}

output "key_id" {
  description = "The key ID (not the private key material)"
  value       = gcpsecure_service_account_key.example.key_id
}

output "key_name" {
  description = "Full resource name of the key"
  value       = gcpsecure_service_account_key.example.name
}
