.PHONY: help proto test lint fmt clean examples install-tools version-major version-minor version-patch version-current version-help

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

install-tools: ## Install required tools
	@echo "Installing protoc plugins..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Installing linting tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

proto: ## Generate gRPC code from proto files
	@echo "Generating gRPC code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/ensync.proto
	@echo "gRPC code generated successfully!"

test: ## Run tests
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "Coverage report:"
	go tool cover -func=coverage.out

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run linters
	@echo "Running linters..."
	golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy
	go mod verify

build-examples: ## Build example programs
	@echo "Building examples..."
	@mkdir -p bin
	go build -o bin/grpc_publisher ./examples/grpc_publisher
	go build -o bin/grpc_subscriber ./examples/grpc_subscriber
	go build -o bin/websocket_example ./examples/websocket_example
	@echo "Examples built in bin/"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf dist/
	rm -f coverage.out coverage.html
	@echo "Clean complete!"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download

check: fmt vet lint test ## Run all checks (format, vet, lint, test)

all: clean deps proto check build-examples ## Run all tasks

# Version Management
version-help: ## Show version management help
	@echo "Version Management Commands:"
	@echo "  make version-current    - Show current version"
	@echo "  make version-patch      - Bump patch version (x.y.Z)"
	@echo "  make version-minor      - Bump minor version (x.Y.0)"
	@echo "  make version-major      - Bump major version (X.0.0)"
	@echo ""
	@echo "After bumping version, commit changes and push to trigger automated tagging."

version-current: ## Show current version
	@./scripts/version.sh current

version-patch: ## Bump patch version (x.y.Z)
	@./scripts/version.sh patch
	@echo "📝 Don't forget to commit and push the changes to trigger automated tagging!"

version-minor: ## Bump minor version (x.Y.0)
	@./scripts/version.sh minor
	@echo "📝 Don't forget to commit and push the changes to trigger automated tagging!"

version-major: ## Bump major version (X.0.0)
	@./scripts/version.sh major
	@echo "📝 Don't forget to commit and push the changes to trigger automated tagging!"

.DEFAULT_GOAL := help
