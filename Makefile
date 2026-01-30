GO ?= go
NPM ?= npm
CURL ?= curl
MANIFEST_FILE ?= plugin.json
BUNDLE_NAME ?= com.mattermost.google-drive

# Build version from plugin.json
PLUGIN_VERSION := $(shell node -p "require('./plugin.json').version")

# Plugin bundle output
DIST_DIR := dist
BUNDLE_FILE := $(DIST_DIR)/$(BUNDLE_NAME)-$(PLUGIN_VERSION).tar.gz

# Go parameters
GO_BUILD_FLAGS := -ldflags '-s -w'
SERVER_DIR := server

# Supported platforms
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

.PHONY: all build build-server dist clean help check-style test

## all: builds the plugin for all platforms
all: dist

## build: builds the plugin server for the current platform
build: build-server

## build-server: builds the Go server component
build-server:
	cd $(SERVER_DIR) && $(GO) mod tidy
	cd $(SERVER_DIR) && $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-$(shell $(GO) env GOOS)-$(shell $(GO) env GOARCH) .

## build-all-platforms: builds for all supported platforms
build-all-platforms:
	@mkdir -p $(SERVER_DIR)/dist
	cd $(SERVER_DIR) && GOOS=linux GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-linux-amd64 .
	cd $(SERVER_DIR) && GOOS=linux GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-linux-arm64 .
	cd $(SERVER_DIR) && GOOS=darwin GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-darwin-amd64 .
	cd $(SERVER_DIR) && GOOS=darwin GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-darwin-arm64 .
	cd $(SERVER_DIR) && GOOS=windows GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) -o dist/plugin-windows-amd64.exe .

## dist: creates the plugin bundle
dist: build-all-platforms
	@mkdir -p $(DIST_DIR)
	@rm -rf $(DIST_DIR)/plugin
	@mkdir -p $(DIST_DIR)/plugin/server/dist
	@mkdir -p $(DIST_DIR)/plugin/assets
	@cp plugin.json $(DIST_DIR)/plugin/
	@cp -r $(SERVER_DIR)/dist/* $(DIST_DIR)/plugin/server/dist/
	@cp assets/icon.svg $(DIST_DIR)/plugin/assets/ 2>/dev/null || echo "Warning: icon.svg not found, skipping"
	cd $(DIST_DIR)/plugin && tar -czvf ../$(BUNDLE_NAME)-$(PLUGIN_VERSION).tar.gz .
	@rm -rf $(DIST_DIR)/plugin
	@echo "Plugin bundle created: $(BUNDLE_FILE)"

## check-style: runs Go linting
check-style:
	cd $(SERVER_DIR) && $(GO) vet ./...

## test: runs Go tests
test:
	cd $(SERVER_DIR) && $(GO) test -v ./...

## clean: removes build artifacts
clean:
	rm -rf $(DIST_DIR)
	rm -rf $(SERVER_DIR)/dist

## deploy: deploys the plugin to a local Mattermost instance (requires MM_SERVICESETTINGS_SITEURL and MM_ADMIN_TOKEN)
deploy: dist
ifndef MM_SERVICESETTINGS_SITEURL
	$(error MM_SERVICESETTINGS_SITEURL is not set)
endif
ifndef MM_ADMIN_TOKEN
	$(error MM_ADMIN_TOKEN is not set)
endif
	$(CURL) -i -X POST \
		$(MM_SERVICESETTINGS_SITEURL)/api/v4/plugins \
		-H "Authorization: Bearer $(MM_ADMIN_TOKEN)" \
		-F "plugin=@$(BUNDLE_FILE)" \
		-F "force=true"

## help: prints this help message
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
