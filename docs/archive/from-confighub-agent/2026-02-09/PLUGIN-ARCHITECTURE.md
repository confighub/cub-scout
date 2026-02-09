# Modular Architecture for ConfigHub Agent

> **Status:** Planning
> **Authors:** Alexis, Claude
> **Date:** 2026-01-12

## Executive Summary

ConfigHub Agent will be refactored into a **modular monolith**: clean package boundaries with well-defined interfaces, compiled into a single binary with unified monetization.

This approach gives us 90% of the benefits of a plugin system (maintainability, testability, clear ownership) with 30% of the complexity. Full plugin infrastructure (runtime loading, gRPC protocol, third-party SDK) is deferred until we have evidence of real demand.

## Why Not Full Plugin Architecture (Yet)

We considered a full plugin system with runtime loading, per-plugin monetization, and third-party SDK. We're deferring this because:

| Concern | Reality |
|---------|---------|
| Third-party demand | No one has asked. GSF export already enables integration. |
| Per-plugin monetization | Fragments the product. Users want "understand my cluster," not "buy scanner separately." |
| gRPC TUI rendering | Unproven territory. Likely months of debugging for marginal benefit. |
| Event bus complexity | Distributed systems problems (ordering, delivery, backpressure) we don't need yet. |
| Plugin versioning | Support nightmare. Terraform has a team for this; we don't. |

**Revisit full plugin architecture when:**
- A third party literally asks to integrate (not hypothetically)
- Data shows users want à la carte features
- We have resources to maintain plugin ecosystem

Until then: modular monolith.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      cub-agent binary                            │
├─────────────────────────────────────────────────────────────────┤
│  cmd/cub-agent/                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  main.go          CLI entry, Cobra setup                  │   │
│  │  tui.go           Bubble Tea host, tab management         │   │
│  └──────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│  pkg/                                                            │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐   │
│  │   core/    │ │    map/    │ │ ownership/ │ │   trace/   │   │
│  │            │ │            │ │            │ │            │   │
│  │ - Context  │ │ - Discover │ │ - Detect   │ │ - Resolve  │   │
│  │ - Auth     │ │ - Filter   │ │ - Label    │ │ - Chain    │   │
│  │ - State    │ │ - Render   │ │ - Render   │ │ - Render   │   │
│  └────────────┘ └────────────┘ └────────────┘ └────────────┘   │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐                   │
│  │  scanner/  │ │ confighub/ │ │    gsf/    │                   │
│  │            │ │            │ │            │                   │
│  │ - Risk DB  │ │ - Connect  │ │ - Export   │                   │
│  │ - Scan     │ │ - Sync     │ │ - Import   │                   │
│  │ - Render   │ │ - Render   │ │            │                   │
│  └────────────┘ └────────────┘ └────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

## Package Boundaries

### Core (`pkg/core/`)

Shared infrastructure used by all modules.

```go
// pkg/core/interfaces.go

// State holds shared application state
type State struct {
    mu         sync.RWMutex
    Resources  []Resource
    Ownership  map[string]Owner
    Chains     map[string]Chain
    Findings   []Finding
}

func (s *State) GetResources() []Resource
func (s *State) SetResources(r []Resource)
func (s *State) GetOwnership() map[string]Owner
func (s *State) SetOwnership(o map[string]Owner)
// ... etc

// Context manages k8s and ConfigHub context
type Context struct {
    KubeConfig    string
    KubeContext   string
    Namespace     string
    AllNamespaces bool
    CubSpace      string
    CubOrg        string
}

// Services provides access to external systems
type Services struct {
    KubeClient kubernetes.Interface
    CubClient  *cub.Client  // nil if not connected
    Logger     *slog.Logger
}
```

### Map (`pkg/map/`)

Resource discovery and display.

```go
// pkg/map/interfaces.go

// Discoverer finds resources in the cluster
type Discoverer interface {
    Discover(ctx context.Context, opts DiscoverOptions) ([]core.Resource, error)
}

// Filterer applies query filters
type Filterer interface {
    Filter(resources []core.Resource, query string) ([]core.Resource, error)
}

// pkg/map/module.go

type Module struct {
    discoverer Discoverer
    filterer   Filterer
    state      *core.State
    services   *core.Services
}

// CLI commands this module provides
func (m *Module) Commands() []*cobra.Command

// TUI tab this module provides
func (m *Module) Tab() tea.Model

// Called when resources should be refreshed
func (m *Module) Refresh(ctx context.Context) error
```

### Ownership (`pkg/ownership/`)

Owner detection for resources.

```go
// pkg/ownership/interfaces.go

// Detector identifies who manages a resource
type Detector interface {
    Detect(r core.Resource) (core.Owner, error)
}

// pkg/ownership/module.go

type Module struct {
    detectors []Detector  // Flux, ArgoCD, Helm, ConfigHub, Native
    state     *core.State
}

// Resolve ownership for all resources in state
func (m *Module) ResolveAll(ctx context.Context) error

// TUI components (may not have own tab, extends Map)
func (m *Module) OwnerColumn() ColumnRenderer
```

### Trace (`pkg/trace/`)

Ownership chain resolution.

```go
// pkg/trace/interfaces.go

// Resolver builds ownership chains
type Resolver interface {
    Resolve(r core.Resource) (core.Chain, error)
}

// pkg/trace/module.go

type Module struct {
    resolver Resolver
    state    *core.State
    services *core.Services
}

func (m *Module) Commands() []*cobra.Command
func (m *Module) Tab() tea.Model
func (m *Module) TraceResource(uid string) (core.Chain, error)
```

### Scanner (`pkg/scanner/`)

risk detection.

```go
// pkg/scanner/interfaces.go

// Scanner checks resources against Risk patterns
type Scanner interface {
    Scan(resources []core.Resource) ([]core.Finding, error)
}

// Database provides Risk pattern lookup
type Database interface {
    Load() error
    Lookup(id string) (*Risk, error)
    All() []*Risk
}

// pkg/scanner/module.go

type Module struct {
    scanner  Scanner
    database Database
    state    *core.State
}

func (m *Module) Commands() []*cobra.Command
func (m *Module) Tab() tea.Model
func (m *Module) ScanAll(ctx context.Context) error
```

### ConfigHub (`pkg/confighub/`)

ConfigHub.com integration.

```go
// pkg/confighub/module.go

type Module struct {
    client   *cub.Client
    state    *core.State
    services *core.Services
}

func (m *Module) Commands() []*cobra.Command  // connect, sync, etc.
func (m *Module) Tab() tea.Model              // ConfigHub view
func (m *Module) IsConnected() bool
func (m *Module) Sync(ctx context.Context) error
```

### GSF (`pkg/gsf/`)

GitOps State Format export/import.

```go
// pkg/gsf/export.go

type Exporter struct {
    state *core.State
}

func (e *Exporter) Export(w io.Writer, opts ExportOptions) error
func (e *Exporter) ExportToFile(path string, opts ExportOptions) error
```

## Module Communication

Modules communicate through **shared state** and **direct calls**, not events.

### Shared State Pattern

```go
// Map discovers resources, writes to state
mapModule.Refresh(ctx)  // writes state.Resources

// Ownership reads resources, writes ownership
ownershipModule.ResolveAll(ctx)  // reads state.Resources, writes state.Ownership

// Scanner reads both, writes findings
scannerModule.ScanAll(ctx)  // reads state.Resources + state.Ownership, writes state.Findings
```

### Direct Calls Pattern

When one module needs another's functionality:

```go
// Trace module needs ownership detection
type TraceModule struct {
    ownership *ownership.Module  // Direct reference
    // ...
}

func (t *TraceModule) TraceResource(uid string) (Chain, error) {
    resource := t.state.GetResource(uid)
    owner, _ := t.ownership.Detect(resource)  // Direct call
    // ... build chain
}
```

### No Event Bus (For Now)

We explicitly avoid pub/sub because:
- Direct calls are easier to trace and debug
- No ordering/delivery guarantees to worry about
- Compile-time type safety
- Call stack in error traces

If we later need async communication, we can add it for specific cases.

## TUI Structure

Single Bubble Tea application with tabs:

```go
// cmd/cub-agent/tui.go

type TUI struct {
    tabs       []Tab
    activeTab  int
    state      *core.State

    // Module references for direct calls
    mapModule       *mapPkg.Module
    ownershipModule *ownership.Module
    traceModule     *trace.Module
    scannerModule   *scanner.Module
    confighubModule *confighub.Module
}

type Tab struct {
    Name    string
    Icon    string
    Model   tea.Model
    KeyMap  help.KeyMap
}

func (t *TUI) Init() tea.Cmd
func (t *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (t *TUI) View() string
```

### Tab Order

| Order | Tab | Module |
|-------|-----|--------|
| 1 | Map | `pkg/map/` |
| 2 | Trace | `pkg/trace/` |
| 3 | Scan | `pkg/scanner/` |
| 4 | ConfigHub | `pkg/confighub/` |

### Cross-Module TUI Integration

Modules can contribute to other modules' views:

```go
// Map tab shows ownership column
type MapTab struct {
    ownershipModule *ownership.Module
}

func (m *MapTab) View() string {
    // ...
    for _, r := range resources {
        owner := m.ownershipModule.GetOwner(r.UID)
        row := fmt.Sprintf("%s %s %s",
            r.Name,
            r.Kind,
            m.ownershipModule.RenderOwnerBadge(owner),
        )
        // ...
    }
}
```

## CLI Structure

Cobra-based CLI with module-contributed commands:

```go
// cmd/cub-agent/main.go

func main() {
    // Initialize shared state and services
    state := core.NewState()
    services := core.NewServices()

    // Initialize modules
    mapMod := mapPkg.New(state, services)
    ownershipMod := ownership.New(state, services)
    traceMod := trace.New(state, services, ownershipMod)
    scannerMod := scanner.New(state, services)
    confighubMod := confighub.New(state, services)

    // Build CLI
    root := &cobra.Command{Use: "cub-agent"}

    // Add module commands
    root.AddCommand(mapMod.Commands()...)
    root.AddCommand(traceMod.Commands()...)
    root.AddCommand(scannerMod.Commands()...)
    root.AddCommand(confighubMod.Commands()...)

    // Global flags
    root.PersistentFlags().StringVarP(&ctx.Namespace, "namespace", "n", "", "Namespace")
    root.PersistentFlags().BoolVarP(&ctx.AllNamespaces, "all-namespaces", "A", false, "All namespaces")

    root.Execute()
}
```

## Directory Structure

```
confighub-agent/
├── cmd/
│   └── cub-agent/
│       ├── main.go           # Entry point, wire up modules
│       ├── tui.go            # Bubble Tea host
│       └── tui_tabs.go       # Tab management
├── pkg/
│   ├── core/
│   │   ├── state.go          # Shared state
│   │   ├── context.go        # K8s/ConfigHub context
│   │   ├── services.go       # External service access
│   │   └── types.go          # Resource, Owner, Chain, Finding
│   ├── map/
│   │   ├── module.go         # Module struct, initialization
│   │   ├── discover.go       # Resource discovery
│   │   ├── filter.go         # Query filtering
│   │   ├── commands.go       # CLI commands
│   │   ├── tab.go            # TUI tab
│   │   └── map_test.go
│   ├── ownership/
│   │   ├── module.go
│   │   ├── detect.go         # Owner detection logic
│   │   ├── detectors/        # Flux, ArgoCD, Helm, etc.
│   │   │   ├── flux.go
│   │   │   ├── argocd.go
│   │   │   ├── helm.go
│   │   │   └── native.go
│   │   ├── render.go         # Owner badges, colors
│   │   └── ownership_test.go
│   ├── trace/
│   │   ├── module.go
│   │   ├── resolve.go        # Chain resolution
│   │   ├── commands.go
│   │   ├── tab.go
│   │   └── trace_test.go
│   ├── scanner/
│   │   ├── module.go
│   │   ├── scan.go           # Scanning logic
│   │   ├── database.go       # Risk database
│   │   ├── commands.go
│   │   ├── tab.go
│   │   └── scanner_test.go
│   ├── confighub/
│   │   ├── module.go
│   │   ├── client.go         # ConfigHub API client
│   │   ├── sync.go           # State synchronization
│   │   ├── commands.go
│   │   ├── tab.go
│   │   └── confighub_test.go
│   └── gsf/
│       ├── types.go          # GSF schema
│       ├── export.go
│       ├── import.go
│       └── gsf_test.go
├── cve/
│   └── ccve/                 # Risk database (unchanged)
└── docs/
    └── planning/
        └── PLUGIN-ARCHITECTURE.md  # This document
```

## Testing Strategy

### Unit Tests

Each module is independently testable:

```go
// pkg/scanner/scanner_test.go

func TestScanner_Scan(t *testing.T) {
    db := NewMockDatabase()
    scanner := NewScanner(db)

    resources := []core.Resource{
        {Kind: "Deployment", Name: "test"},
    }

    findings, err := scanner.Scan(resources)
    assert.NoError(t, err)
    assert.Len(t, findings, 1)
}
```

### Integration Tests

Test module interactions:

```go
// test/integration/map_ownership_test.go

func TestMapWithOwnership(t *testing.T) {
    state := core.NewState()
    services := testServices(t)

    mapMod := mapPkg.New(state, services)
    ownershipMod := ownership.New(state, services)

    // Map discovers resources
    mapMod.Refresh(context.Background())

    // Ownership resolves
    ownershipMod.ResolveAll(context.Background())

    // Verify ownership is populated
    for _, r := range state.GetResources() {
        owner := state.GetOwnership()[r.UID]
        assert.NotEmpty(t, owner.Type)
    }
}
```

### TUI Tests

teatest for TUI components:

```go
// pkg/map/tab_test.go

func TestMapTab_Navigation(t *testing.T) {
    state := testState()
    tab := NewTab(state)

    tm := teatest.NewTestModel(t, tab)
    tm.Send(tea.KeyMsg{Type: tea.KeyDown})
    tm.Send(tea.KeyMsg{Type: tea.KeyDown})

    final := tm.FinalModel(t).(Tab)
    assert.Equal(t, 2, final.cursor)
}
```

## Migration Plan

### Phase 1: Extract Core (Week 1)

1. Create `pkg/core/` with State, Context, Services
2. Define core types (Resource, Owner, Chain, Finding)
3. All existing code continues to work

### Phase 2: Extract Map (Week 2)

1. Move resource discovery to `pkg/map/`
2. Implement Module interface
3. Wire up in main.go
4. Verify TUI and CLI unchanged

### Phase 3: Extract Ownership (Week 2)

1. Move ownership detection to `pkg/ownership/`
2. Move individual detectors to `pkg/ownership/detectors/`
3. Wire up, verify

### Phase 4: Extract Remaining Modules (Week 3)

1. Trace → `pkg/trace/`
2. Scanner → `pkg/scanner/`
3. ConfigHub → `pkg/confighub/`
4. GSF → `pkg/gsf/`

### Phase 5: Clean Up (Week 4)

1. Remove dead code from `cmd/cub-agent/`
2. Ensure all tests pass
3. Update documentation

## Monetization

**Single unified tier structure.** Users buy cub-agent access, not individual modules.

| Tier | Features |
|------|----------|
| Free | Map, Ownership, Trace (local only) |
| Pro | + Scanner (full Risk database), + GSF export |
| Enterprise | + ConfigHub.com integration, + Team features |

This keeps pricing simple and avoids "why doesn't scanner work, I'm on Pro" confusion.

## Future: Full Plugin Architecture

If in 12+ months we have evidence of demand, we can evolve to full plugins:

1. **Evidence needed:**
   - Third party asks to integrate (not hypothetically)
   - Users request à la carte features
   - Resources to maintain plugin ecosystem

2. **Evolution path:**
   - Module interface becomes Plugin interface
   - Add plugin manifest (YAML)
   - Add runtime loading (gRPC)
   - Add per-plugin monetization

3. **What we preserve:**
   - Clean module boundaries (already done)
   - Shared state pattern (already done)
   - Interface-based design (already done)

The modular monolith is the foundation that makes future plugin architecture possible without building speculative infrastructure today.

---

## Appendix: Original Plugin Architecture

The full plugin architecture design (gRPC runtime plugins, per-plugin monetization, third-party SDK) is preserved below for future reference.

<details>
<summary>Click to expand full plugin architecture (deferred)</summary>

### Runtime Plugin Protocol

```protobuf
service CubPlugin {
    rpc Init(InitRequest) returns (InitResponse);
    rpc Shutdown(Empty) returns (Empty);
    rpc HandleCommand(CommandRequest) returns (CommandResponse);
    rpc RenderTab(RenderRequest) returns (RenderResponse);
    rpc HandleInput(InputEvent) returns (UpdateResponse);
    rpc Subscribe(SubscribeRequest) returns (stream Event);
    rpc Publish(Event) returns (Empty);
}
```

### Event Bus Topics

```go
const (
    TopicResourceDiscovered = "k8s.resource.discovered"
    TopicResourceUpdated    = "k8s.resource.updated"
    TopicOwnershipResolved  = "ownership.resolved"
    TopicCCVEFound          = "scan.ccve.found"
    TopicChainResolved      = "trace.chain.resolved"
)
```

### Third-Party Plugin Manifest

```yaml
name: flux9s
version: 2.1.0
vendor: Weaveworks
requires:
  plugin_api: ">=1.0.0, <2.0.0"
cli:
  verbs:
    - name: flux
      subcommands: [reconcile, suspend, resume]
tui:
  tabs:
    - name: Flux9s
      icon: "⚡"
      order: 50
monetization:
  upsell:
    url: "https://flux9s.io/upgrade?source=cub-agent"
```

### Why Deferred

- gRPC TUI rendering is unproven, likely months of debugging
- No third-party has asked to integrate
- Per-plugin monetization fragments the product
- Event bus introduces distributed systems complexity
- Plugin versioning requires dedicated support resources

</details>

---

*This document captures the modular architecture discussion from 2026-01-12.*
