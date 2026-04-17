# llm-manager Makefile
#
# Usage:
#   make build          Build the binary
#   make test           Run tests
#   make lint           Run linter
#   make fmt            Format code
#   make clean          Remove build artifacts
#   make install        Install the binary
#   make version        Show version info
#   make help           Show this help message

# Project settings
APP_NAME       := llm-manager
MODULE_PATH    := github.com/user/llm-manager
VERSION        ?= dev
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE           ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS        := -s -w
BUILD_LDFLAGS  := -ldflags "$(LDFLAGS) -X $(MODULE_PATH)/internal/version.version=$(VERSION) -X $(MODULE_PATH)/internal/version.commit=$(COMMIT) -X $(MODULE_PATH)/internal/version.date=$(DATE)"

# Go settings
GO             := go
GOLANGCI_LINT  := golangci-lint
BUILD_TARGET   := ./cmd/$(APP_NAME)

# Default target
.PHONY: help
help: ## Show this help message
	@echo "$(APP_NAME) - A CLI tool for managing LLM resources"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[0-9a-zA-Z_-]+:.*?## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build the binary
.PHONY: build
build: ## Build the binary
	@echo "Building $(APP_NAME) $(VERSION)..."
	$(GO) build $(BUILD_LDFLAGS) -o bin/$(APP_NAME) $(BUILD_TARGET)
	@echo "Build complete: bin/$(APP_NAME)"

# Run tests with race detection
.PHONY: test
test: ## Run tests with race detection
	@echo "Running tests..."
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic ./...
	@echo ""
	@echo "Coverage report:"
	@$(GO) tool cover -func=coverage.txt | tail -1

# Run tests and open HTML coverage report
.PHONY: test-html
test-html: test ## Run tests and open HTML coverage report
	@echo "Opening coverage report..."
	@$(GO) tool cover -html=coverage.txt

# Run tests with short mode
.PHONY: test-short
test-short: ## Run tests in short mode
	@echo "Running tests (short mode)..."
	$(GO) test -short -race ./...

# Run linter
.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running linter..."
	@if command -v $(GOLANGCI_LINT) > /dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Falling back to go vet..."; \
		$(GO) vet ./...; \
	fi

# Format code
.PHONY: fmt
fmt: ## Format code with gofmt
	@echo "Formatting code..."
	$(GO) fmt ./...

# Clean build artifacts
.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.txt
	rm -f coverage.html
	@echo "Clean complete."

# Install the binary
.PHONY: install
install: ## Install the binary via go install
	@echo "Installing $(APP_NAME)..."
	$(GO) install -ldflags "$(LDFLAGS) -X $(MODULE_PATH)/internal/version.version=$(VERSION) -X $(MODULE_PATH)/internal/version.commit=$(COMMIT) -X $(MODULE_PATH)/internal/version.date=$(DATE)" $(BUILD_TARGET)
	@echo "Install complete."

# Show version info
.PHONY: version
version: ## Show version info
	@echo "$(APP_NAME) version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Date: $(DATE)"
	@echo "Go: $($(GO) version 2>/dev/null || echo 'unknown')"

# Run all verification steps
.PHONY: verify
verify: fmt lint test ## Format, lint, and test
	@echo "All checks passed!"

# Build and run
.PHONY: run
run: build ## Build and run the binary
	@./bin/$(APP_NAME) $(ARGS)

# Generate documentation
.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	@$(GO) doc -all . 2>/dev/null || echo "Documentation generation requires goimports"

# Download and tidy dependencies
.PHONY: deps
deps: ## Download and tidy dependencies
	@echo "Tidying dependencies..."
	$(GO) mod tidy
	$(GO) mod verify

# Pre-commit hook setup
.PHONY: pre-commit
pre-commit: ## Set up pre-commit hook
	@mkdir -p .git/hooks
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make verify' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."
