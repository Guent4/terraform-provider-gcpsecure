.PHONY: build install test generate fmt lint

# Build the provider binary
build:
	go build -o terraform-provider-gcpsecure .

# Install the provider (for local development)
install: build
	go install .

# Run tests
test:
	go test -v ./...

# Run acceptance tests (requires GCP credentials and TF_ACC=1)
testacc:
	TF_ACC=1 go test -v ./...

# Generate provider documentation for the Terraform Registry.
# Requires: go get github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
# Then: go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name gcpsecure
# Docs in docs/ are maintained by hand for this provider.
generate:
	@echo "Docs are in docs/. To regenerate: go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name gcpsecure"

# Format code
fmt:
	go fmt ./...

# Run linters
lint:
	golangci-lint run ./...
