# cub-scout Makefile
# Canonical verification commands for development and CI

.PHONY: build test test-race fmt lint clean

# Default target
all: build test

# Build the binary
build:
	go build ./cmd/cub-scout

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
	go clean ./...

# Run lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Quick verification (build + test)
verify: build test

# Full verification (format check + build + test + race)
verify-full: fmt-check build test test-race
