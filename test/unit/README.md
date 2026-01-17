# Unit Tests

Go unit tests that run **without a Kubernetes cluster**.

## Running

```bash
# Run all unit tests
go test ./test/unit/...

# Run with verbose output
go test -v ./test/unit/...

# Run specific test
go test -v ./test/unit/... -run TestOwnershipDetection

# Run with coverage
go test -cover ./test/unit/...
```

## Test Files

| File | What It Tests |
|------|---------------|
| `ownership_test.go` | Ownership detection for all 6 types (Flux, Argo, Helm, Terraform, ConfigHub, K8s) |
| `cub_cli_test.go` | cub CLI JSON output structure (prevents issue #1) |
| `helpers.go` | Test utilities: precondition helpers, fixture loaders, assertions |

## Adding Tests

### Ownership Detection Test

```go
func TestMyOwnershipPattern(t *testing.T) {
    resource := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "apps/v1",
            "kind":       "Deployment",
            "metadata": map[string]interface{}{
                "name":      "test",
                "namespace": "default",
            },
        },
    }
    resource.SetLabels(map[string]string{
        "my-label": "my-value",
    })

    ownership := agent.DetectOwnership(resource)

    require.Equal(t, agent.OwnerXxx, ownership.Type)
}
```

### cub CLI Output Test

```go
func TestCubOutputField(t *testing.T) {
    RequireCubAuth(t) // Skip if not authenticated

    var items []map[string]interface{}
    RunCubJSON(t, &items, "unit", "list")

    for _, item := range items {
        // Check flat structure (not nested)
        _, hasField := item["FieldName"]
        assert.True(t, hasField)
    }
}
```

## Helpers

### Precondition Helpers

These fail the test immediately if the environment isn't ready:

```go
RequireCluster(t)     // kubectl works
RequireFlux(t)        // Flux CRDs installed
RequireArgo(t)        // Argo CD CRDs installed
RequireCubAuth(t)     // cub CLI authenticated
RequireSpace(t)       // Active space set
RequireWorker(t, space)  // Worker connected
RequireTarget(t, space)  // Target exists
RequireUnits(t, space, n) // At least n units
```

### Assertion Helpers

```go
AssertNoNullValues(t, output)     // No "null - unknown" patterns
AssertUnitsHaveTargets(t, output) // No "→ no target"
AssertWorkersHealthy(t, output)   // Workers connected
AssertOwnerDetected(t, output, kind, name, owner)
```

### Command Helpers

```go
RunCubAgent(t, "map", "--json")   // Run cub-agent
RunCub(t, "unit", "list")         // Run cub CLI
RunCubJSON(t, &result, "unit", "list")  // Run cub and parse JSON
```

## Common Issues

| Issue | Test That Catches It | Fix |
|-------|---------------------|-----|
| `.Unit.Slug` returns null | `TestCubCLIOutputStructure` | Use `.Slug` (flat object) |
| Wrong owner detected | `TestOwnershipDetection` | Check label detection logic |
| Priority conflict | `TestOwnershipPriority` | Check detection order |
