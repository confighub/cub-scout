# Integration Tests

Go integration tests that require a **Kubernetes cluster** and optionally **ConfigHub API access**.

## Running

```bash
# Run all integration tests (requires cluster)
go test -tags=integration ./test/integration/...

# Run with verbose output
go test -tags=integration -v ./test/integration/...

# Skip if no cluster available
SKIP_INTEGRATION=true go test -tags=integration ./test/integration/...
```

## Test Files

| File | What It Tests | Requirements |
|------|---------------|--------------|
| `standalone_test.go` | Cluster-only mode, no ConfigHub | Kubernetes cluster |
| `connected_test.go` | ConfigHub connected mode | Kubernetes + ConfigHub auth |

## Build Tag

All integration tests use the `integration` build tag:

```go
//go:build integration
// +build integration

package integration
```

This ensures they don't run during normal `go test ./...`.

## Preconditions

Integration tests use precondition helpers to fail fast:

```go
func TestConnectedMode(t *testing.T) {
    // Skip if not authenticated
    space := unit.RequireSpace(t)
    unit.RequireWorker(t, space)
    unit.RequireTarget(t, space)
    units := unit.RequireUnits(t, space, 1)

    // Now run actual test...
}
```

## Adding Tests

### Standalone Mode Test

```go
//go:build integration
// +build integration

package integration

import (
    "testing"
    "github.com/confighub/agent/test/unit"
)

func TestStandaloneOwnership(t *testing.T) {
    unit.RequireCluster(t)
    unit.RequireFlux(t)

    // Deploy fixture
    // Run cub-agent
    // Verify output
}
```

### Connected Mode Test

```go
//go:build integration
// +build integration

package integration

import (
    "testing"
    "github.com/confighub/agent/test/unit"
)

func TestConnectedModeMap(t *testing.T) {
    space := unit.RequireSpace(t)
    unit.RequireWorker(t, space)
    unit.RequireTarget(t, space)

    output := unit.RunCubAgent(t, "map", "--space", space.Slug)

    unit.AssertNoNullValues(t, output)
    unit.AssertWorkersHealthy(t, output)
}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SKIP_INTEGRATION` | Skip all integration tests | false |
| `KUBECONFIG` | Kubernetes config | `~/.kube/config` |
| `CUB_AGENT` | Path to cub-agent binary | `./cub-agent` |
