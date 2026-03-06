# cub-scout Makefile
# Canonical verification commands for development and CI

.PHONY: build build-kubectl-plugin test test-race fmt lint clean regression-argo test-import-delegation

# Default target
all: build test

# Build the binary
build:
	go build ./cmd/cub-scout

# Build kubectl plugin-compatible binary alias (kubectl-cub_scout)
build-kubectl-plugin: build
	cp cub-scout kubectl-cub_scout
	chmod +x kubectl-cub_scout

# Run all tests
test:
	go test ./... -v

# Run tests with race detector
test-race:
	go test -race ./...

# Format code
fmt:
	gofmt -w .

# Check formatting (fails if changes needed)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' to fix formatting" && gofmt -l . && exit 1)

# Clean build artifacts
clean:
	rm -f cub-scout
	rm -f kubectl-cub_scout
	go clean ./...

# Run lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Quick verification (build + test)
verify: build test

# Full verification (format check + build + test + race)
verify-full: fmt-check build test test-race

# Run Argo regression audit (v0.4.0 vs v0.19.6)
regression-argo:
	./test/regression/argo-version-audit.sh

# Repeatable checks for connected import delegation flow
test-import-delegation:
	./scripts/test-import-delegation.sh
