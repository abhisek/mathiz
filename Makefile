GO := go
BIN_DIR := bin
BIN := $(BIN_DIR)/mathiz
SHELL := /bin/bash
GITCOMMIT := $(shell git rev-parse HEAD)
VERSION := "$(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)-$(shell git rev-parse --short HEAD)"

.PHONY: all deps generate mathiz clean test

all: mathiz

# Install dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Generate ent code
generate:
	$(GO) generate ./storage/ent/...

# Generate Event JSON Schema
generate-schema:
	$(GO) run ./cmd/jsonschema-gen

# Verify Event JSON Schema is up-to-date
verify-schema: generate-schema
	@git diff --exit-code schema/event.schema.json || (echo "ERROR: schema/event.schema.json is out of date. Run 'make generate-schema' and commit the result." && exit 1)

# Build mathiz binary
mathiz: create_bin
	$(GO) build ${GO_LDFLAGS} -o $(BIN) .

create_bin:
	mkdir -p $(BIN_DIR)

clean:
	rm -rf $(BIN_DIR)

test:
	$(GO) test ./...

# Format code
fmt:
	$(GO) fmt ./...

# Run linter
lint:
	golangci-lint run

# Build for all platforms
build-all: create_bin
	GOOS=darwin GOARCH=amd64 $(GO) build ${GO_LDFLAGS} -o $(BIN_DIR)/mathiz-darwin-amd64 ./cmd/mathiz
	GOOS=darwin GOARCH=arm64 $(GO) build ${GO_LDFLAGS} -o $(BIN_DIR)/mathiz-darwin-arm64 ./cmd/mathiz
	GOOS=linux GOARCH=amd64 $(GO) build ${GO_LDFLAGS} -o $(BIN_DIR)/mathiz-linux-amd64 ./cmd/mathiz
	GOOS=windows GOARCH=amd64 $(GO) build ${GO_LDFLAGS} -o $(BIN_DIR)/mathiz-windows-amd64.exe ./cmd/mathiz
