BINARY_NAME=mcp-server-magicskills
DIST_DIR=dist
GIT_VERSION=$(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
VERSION?=$(GIT_VERSION)

.PHONY: all build clean test run install version build-all linux darwin-amd64 darwin-arm64 windows help fmt vet lint

all: help build-all

build: ## Compiles the Go application for the local OS/Arch
	@mkdir -p $(DIST_DIR)
	@CGO_ENABLED=0 go build -tags netgo -ldflags "-extldflags '-static' -w -s -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) .

build-all: linux darwin-amd64 darwin-arm64 windows ## Compiles for multiple platforms

linux: ## Compiles for Linux AMD64
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags netgo -ldflags "-extldflags '-static' -w -s -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 .

darwin-amd64: ## Compiles for macOS AMD64
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 .

darwin-arm64: ## Compiles for macOS Apple Silicon (arm64)
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 .

windows: ## Compiles for Windows AMD64
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe .

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
