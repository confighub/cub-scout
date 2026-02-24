# Testing Best Practices

> **Role:** Cookbook for contributors. Answers "what tests do I write for feature X?"
>
> **Supplements:** [docs/testing/README.md](README.md) (authoritative reference).
> Do not duplicate content from README.md — cross-reference it.

---

## 1. The Test Recipe

Before writing code, check this table. Every change type has a minimum test expectation.

| I am adding... | Required tests | Optional |
|---|---|---|
| **Ownership detection logic** | Unit test in `pkg/agent/ownership_test.go` + pattern fixture in `test/fixtures/patterns/` | ASCII golden for trace output |
| **New CLI subcommand** | Golden test in `test/golden/<cmd>/` + integration test (`//go:build integration`) | Demo scenario in `prove-it-works.sh` |
| **New TUI view or keybinding** | teatest snapshot in `cmd/cub-scout/*_test.go` | E2E test with real cluster data (`//go:build e2e`) |
| **New risk pattern** | Pattern fixture YAML in `test/fixtures/patterns/` + scale smoke gate | Entry in risk catalog |
| **CLI output format change** | ASCII golden in `test/ascii/<cmd>/` | Contract test if struct is exported |
| **Connected mode feature** | Integration test with `skipIfNotConnected(t)` gate | Dry-run test (no ConfigHub writes) |
| **Schema change to locked struct** | Contract test in `test/contract/` | Golden hash comparison |

**Minimum bar (from CLAUDE.md):**
- Tests exist and pass
- Examples demonstrate expected behavior
- User-facing output is correct and explainable

---

## 2. Build Tags

Three tiers, each gated by a build tag (or lack of one):

| Tag | Meaning | CI command | When to use |
|---|---|---|---|
| *(none)* | Offline — no cluster, no network | `go test ./...` | Pure logic, TUI snapshots, query parsing, pattern ownership |
| `integration` | Needs a live K8s cluster | `go test -tags=integration ./...` | CLI commands against a real cluster, golden tests needing real data |
| `e2e` | Needs cluster + internal model access | `go test -tags=e2e ./cmd/cub-scout/...` | Tests that import `main` package types and need real cluster data |

**Rules:**

1. **Default to no tag.** Most tests should run with `go test ./...`.
2. **Use `integration`** when the test invokes the compiled binary or public APIs against a real cluster. Tests in `test/integration/` always carry this tag.
3. **Use `e2e`** when the test needs both a real cluster and access to internal types (e.g., `LocalClusterModel`). These live in `cmd/cub-scout/` and cannot run via the binary alone.
4. **Never mix tags** in the same file. A file is either untagged, `integration`, or `e2e`.

```go
// File header for an integration test:
//go:build integration

package integration

// File header for an E2E test:
//go:build e2e

package main
```

---

## 3. Fixture Organization

| Fixture type | Location | Convention |
|---|---|---|
| Offline logic fixtures (JSON/YAML) | `test/ascii/<cmd>/testdata/` or `<pkg>/testdata/` | Injected via `CUB_SCOUT_TEST_*` env vars |
| Pattern ownership YAML | `test/fixtures/patterns/<pattern-name>/` | Multi-doc YAML loaded by `loadObjects()` |
| Cluster-apply fixtures | `test/fixtures/<feature>/` | Applied via `kubectl apply` in test setup |
| Real example apps | `examples/` | **Read-only** — never modified by tests |
| Package-level unit test data | `<pkg>/testdata/` | Standard Go convention |
| Golden output files | Co-located with test or in `testdata/` subdir | `.golden.txt` or `.golden.json` extension |
| Bridge pattern fixtures | `test/fixtures/patterns/bridge-<name>/` | YAML with full controller → workload chain |

**Key rule:** `examples/` is read-only test input. Tests should never write to or modify files in `examples/`. If a test needs to deploy example resources, `kubectl apply` them into a temporary namespace and clean up via `t.Cleanup()`.

**Environment variable convention:** Offline golden tests inject fixture data into the CLI via environment variables prefixed with `CUB_SCOUT_TEST_`. Example: `CUB_SCOUT_TEST_MAP_ENTRIES_JSON` overrides the map command's data source with a JSON fixture file.

---

## 4. Golden Test Patterns

cub-scout has two golden test harnesses. Use the right one for the job.

### `test/golden/` — CLI golden harness

Runs the **compiled binary** (`./cub-scout`). Tests CLI output as the user sees it.

```go
import "github.com/confighub/cub-scout/test/golden"

func TestMyCommand(t *testing.T) {
    result := golden.RunCubScout(t, "mycommand", "--flag", "value")
    golden.AssertExitCode(t, 0, result)
    normalized := golden.Normalize(result.Stdout)
    golden.AssertGolden(t, "mycommand/scenario", normalized)
    //                      ^ path first, actual second
}
```

- Requires `go build ./cmd/cub-scout` before running tests
- Binary path overridable via `CUB_SCOUT_BINARY` env var
- Sets `NO_COLOR=1`, `TERM=dumb`, `CI=true` automatically
- Normalize scrubs: ANSI codes, timestamps, temp paths, UUIDs, long SHAs, elapsed times, home dirs

### `test/ascii/` — ASCII golden harness

Runs via **`go run ./cmd/cub-scout`**. Tests ASCII output format contracts.

```go
import (
    agolden "github.com/confighub/cub-scout/test/ascii/golden"
    "github.com/confighub/cub-scout/test/ascii/runner"
)

func TestMyOutput(t *testing.T) {
    out := runner.Run(t, runner.RepoRoot(t), "mycommand", "--flag")
    agolden.AssertGolden(t, out, goldenPath)
    //                       ^ got first, path second
}
```

- No pre-built binary needed (uses `go run`)
- Normalize scrubs: ANSI codes, pod UIDs, timestamps, relative times
- Trims trailing whitespace and ensures single trailing newline

### Which to use?

| Scenario | Use |
|---|---|
| Testing a CLI command's full output | `test/golden/` (closer to user experience) |
| Testing ASCII format contracts (column alignment, section layout) | `test/ascii/` |
| New golden tests (default choice) | `test/golden/` |

### Updating golden files

Both harnesses support the same two mechanisms:

```bash
# Via flag
go test ./test/golden/... -update
go test ./test/ascii/... -update

# Via environment variable (CI-friendly)
UPDATE_GOLDEN=1 go test ./test/golden/... -count=1
UPDATE_GOLDEN=1 go test ./test/ascii/... -count=1
```

**Important:** Always review golden file diffs before committing. Golden changes are UX changes.

---

## 5. Cluster Lifecycle Patterns

### Gating: when to skip

Use the lightest gate that fits:

| Pattern | When | Example files |
|---|---|---|
| No tag, no skip | Pure offline logic | `ownership_test.go`, `query_test.go` |
| Build tag only | All tests in the file need a cluster | `test/integration/connected_test.go` |
| Build tag + `skipIfNoCluster(t)` | Mixed file where some subtests need a cluster | Integration suites with conditional tests |
| Build tag + `skipIfNotConnected(t)` | Needs ConfigHub auth | Connected mode tests |
| `requireGolden(t, name)` | Cluster-dependent golden tests with optional regeneration | `test/golden/trace/` |

### Namespace management

Always use unique namespaces with automatic cleanup:

```go
func createTestNamespace(t *testing.T) string {
    t.Helper()
    ns := fmt.Sprintf("e2e-%s-%d", strings.ToLower(t.Name()), time.Now().Unix())
    cmd := exec.Command("kubectl", "create", "namespace", ns)
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("create namespace %s: %v\n%s", ns, err, out)
    }
    t.Cleanup(func() {
        exec.Command("kubectl", "delete", "namespace", ns,
            "--ignore-not-found", "--wait=false").Run()
    })
    return ns
}
```

### Deploying fixtures

```go
func deployFixtures(t *testing.T, ns, fixtureDir string) {
    t.Helper()
    files, _ := filepath.Glob(filepath.Join(fixtureDir, "*.yaml"))
    for _, f := range files {
        cmd := exec.Command("kubectl", "apply", "-f", f, "-n", ns)
        if out, err := cmd.CombinedOutput(); err != nil {
            t.Fatalf("apply %s: %v\n%s", f, err, out)
        }
    }
}
```

### Waiting for readiness

```go
cmd := exec.Command("kubectl", "wait", "--for=condition=available",
    "deployment", "--all", "-n", ns, "--timeout=60s")
if out, err := cmd.CombinedOutput(); err != nil {
    t.Fatalf("wait for deployments: %v\n%s", err, out)
}
```

---

## 6. Example-Driven Testing

The `examples/` directory serves three roles: documentation, regression protection, and demo artifacts.

### Using examples in tests

1. **Apply** example YAML to a kind cluster:
   ```bash
   kubectl apply -f examples/demo-data/manifests.yaml
   ```

2. **Run** cub-scout against the live cluster:
   ```bash
   ./cub-scout map app-hierarchy
   ./cub-scout map list --json
   ```

3. **Assert** ownership detection and output format.

4. **Clean up** (always):
   ```bash
   kubectl delete -f examples/demo-data/manifests.yaml --ignore-not-found
   ```

### Adding a new example

1. Create YAML manifests in `examples/<name>/`
2. Add a `README.md` following the problem-first pattern:
   - "The Problem" → "cub-scout answers this" → "How it works"
3. Add entry to `test/examples/real-examples-catalog.yaml`
4. Add a step to `test/prove-it-works.sh` at the appropriate level
5. If the example demonstrates ownership detection, add a matching pattern fixture in `test/fixtures/patterns/` for offline regression

### prove-it-works.sh levels for examples

| Level | What it does with examples |
|---|---|
| `gitops` (level 3) | Applies `flux-boutique`, asserts Flux ownership |
| `examples` (level 5) | Applies additional examples, runs `TestRealExamplesCatalog` |
| `full` (level 7) | Everything above + connected mode examples |

---

## 7. GitOps Validation Patterns

### Ownership assertion table

Every ownership type has a detection rule and a standard assertion:

| Owner | Detection label/annotation | Assert |
|---|---|---|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` | `owner == "Flux"` |
| **ArgoCD** | `argocd.argoproj.io/instance` or tracking-id annotation | `owner == "ArgoCD"` |
| **Helm** | `app.kubernetes.io/managed-by: Helm` | `owner == "Helm"` |
| **Terraform** | `app.terraform.io/run-id` annotation or managed label | `owner == "Terraform"` |
| **Crossplane** | `crossplane.io/claim-name` label | `owner == "Crossplane"` |
| **ConfigHub** | `confighub.com/UnitSlug` label | `owner == "ConfigHub"` |
| **Native** | None of the above | `owner == "Native"` |

### Standard fixture pattern

Pattern fixtures in `test/fixtures/patterns/` use multi-document YAML:

```yaml
# Controller resource (e.g., Flux Kustomization)
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: my-app
  namespace: flux-system
---
# Workload with matching labels
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
  labels:
    kustomize.toolkit.fluxcd.io/name: my-app
    kustomize.toolkit.fluxcd.io/namespace: flux-system
```

Bridge pattern fixtures include the full chain: source → deployer → workload.

### Trace validation

For each owner type, validate that `trace` produces the expected chain:

```bash
# Flux: GitRepository → Kustomization → Deployment
./cub-scout trace deploy/cart -n boutique

# ArgoCD: Application → Deployment
./cub-scout trace --app guestbook

# Native: warns about unmanaged resource
./cub-scout trace deploy/legacy-auth -n legacy-apps
```

---

## 8. Connected Mode Testing

### The skip-chain pattern

Connected tests progressively check prerequisites and skip gracefully:

```go
func TestMyConnectedFeature(t *testing.T) {
    skipIfNotConnected(t)                // skips if no cub CLI or no auth
    space := getCurrentSpace(t)          // skips if no active space
    workerSlug := requireWorker(t, space) // skips if no worker running
    targetSlug := requireTarget(t, space) // skips if no target configured

    // ... test logic using real ConfigHub state
}
```

### Dry-run testing (preferred for CI)

Dry-run tests exercise the full discovery and proposal flow without creating ConfigHub resources:

```go
func TestImportDryRun(t *testing.T) {
    skipIfNoCluster(t)
    ns := createTestNamespace(t)
    deployFixtures(t, ns, "test/fixtures/import-e2e/")

    output := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")
    // Assert JSON structure, workload discovery, ownership detection
    // No ConfigHub resources created or modified
}
```

### Full round-trip testing (manual trigger)

For tests that create real ConfigHub resources, guard with an extra env var:

```go
if os.Getenv("CUB_E2E_FULL") != "1" {
    t.Skip("full round-trip tests require CUB_E2E_FULL=1")
}
```

Always clean up: `t.Cleanup(func() { deleteCubSpace(t, slug) })`.

---

## 9. Cross-Reference Index

| Topic | Document |
|---|---|
| Authoritative test reference | [docs/testing/README.md](README.md) |
| Complete test inventory | [test/TEST-INVENTORY.md](../../test/TEST-INVENTORY.md) |
| Test level definitions | [test/test-levels.yaml](../../test/test-levels.yaml) |
| Multi-level test orchestrator | [test/prove-it-works.sh](../../test/prove-it-works.sh) |
| Pre-coding test requirements | [CLAUDE.md](../../CLAUDE.md) — "Pre-Coding Test & Success Proof Requirements" |
| CI workflow | [.github/workflows/ci.yaml](../../.github/workflows/ci.yaml) |
| Semantic contract (JSON/ASCII) | [docs/semantic-contract.md](../semantic-contract.md) |
| CLI contract (exit codes, output) | [docs/reference/cli-contract.md](../reference/cli-contract.md) |
| Contributing guide | [CONTRIBUTING.md](../../CONTRIBUTING.md) |
| Real examples catalog | [test/examples/real-examples-catalog.yaml](../../test/examples/real-examples-catalog.yaml) |
