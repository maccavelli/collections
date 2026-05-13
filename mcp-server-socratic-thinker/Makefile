MOD_VERSION := 1.26.2
BINARY_NAME=mcp-server-socratic-thinker
DIST_DIR=dist
GIT_VERSION=$(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
VERSION?=$(GIT_VERSION)

.PHONY: all build clean test run install version build-all linux darwin-amd64 darwin-arm64 windows-amd64 windows-arm64 help fmt vet lint vendor

all: help build-all

build: ## Compiles the Go application for the local OS/Arch
	@mkdir -p $(DIST_DIR)
	@CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "-extldflags '-static' -s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) .

build-all: linux darwin-amd64 darwin-arm64 windows-amd64 windows-arm64 ## Compiles for multiple platforms

linux: ## Compiles for Linux AMD64
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags netgo -ldflags "-extldflags '-static' -s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 .

darwin-amd64: ## Compiles for macOS AMD64
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 .

darwin-arm64: ## Compiles for macOS ARM64
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 .

windows-amd64: ## Compiles for Windows AMD64
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe .

windows-arm64: ## Compiles for Windows ARM64
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe .

clean: ## Removes all build artifacts
	rm -rf $(DIST_DIR)

test: ## Runs all tests with verbose output
	go test -v ./...

fmt: ## Formats all Go source files
	go fmt ./...

vet: ## Runs go vet on the project
	go vet ./...

lint: ## Runs golangci-lint
	golangci-lint run

vendor: ## Re-vendor dependencies
	go mod vendor
	go mod tidy

run: build ## Builds and executes the local binary
	@BIN_NAME=$(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) ; \
	$$BIN_NAME

install: build ## Copies the local binary to ~/.local/bin/
	@BIN_NAME=$(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) ; \
	INSTALL_PATH=$(HOME)/.local/bin/$(BINARY_NAME) ; \
	if [ -f "$$INSTALL_PATH" ]; then mv "$$INSTALL_PATH" "$$INSTALL_PATH.old"; fi ; \
	cp $$BIN_NAME $$INSTALL_PATH ; \
	rm -f "$$INSTALL_PATH.old" ; \
	echo "Installed $(BINARY_NAME) to ~/.local/bin/"

version: build ## Displays the version of the local binary
	@BIN_NAME=$(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) ; \
	$$BIN_NAME --version

help: ## Displays this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
