# TUI Refactor Plan: Preparing for cub Merge

> **Status:** Active
> **Date:** 2026-01-12
> **Context:** cub-agent will merge into cub CLI soon

## Goal

Clean up the TUI code in `cmd/cub-agent/` before merging into `cub`. Focus on:
1. Split the god model into composable sub-models
2. Add test coverage for TUI components
3. Prepare shared types for the merge

**Non-goals:**
- Modular monolith architecture (over-engineering)
- Replacing `runCubCommand` (goes away with merge)
- Refactoring `pkg/` (already clean)

---

## Current State

### The Problem: God Model

`hierarchy_types.go` defines a `Model` struct with **40+ fields** handling 6 concerns:

```go
type Model struct {
    // Tree navigation (~10 fields)
    nodes, flatList, cursor, width, height, ready, loading, err, ...

    // Search (~6 fields)
    searchMode, searchQuery, searchMatches, ...

    // Auth (~4 fields)
    authPrompt, authOrgName, authOrgID, statusMsg

    // Import wizard (~15 fields)  ← EXTRACT THIS
    importMode, importStep, importNamespaces, importSpace, ...

    // Create wizard (~10 fields)  ← EXTRACT THIS
    createMode, createType, createStep, createName, ...

    // Delete wizard (~5 fields)   ← EXTRACT THIS
    deleteMode, deleteConfirm, deleteTarget, ...
}
```

### File Sizes

| File | Lines | After Refactor |
|------|-------|----------------|
| `hierarchy.go` | 5,116 | ~2,500 (tree + search + auth) |
| `hierarchy_types.go` | 802 | ~400 (tree types only) |
| `import_wizard.go` | 3,354 | Standalone model |
| **New:** `create_wizard.go` | — | ~500 (extracted) |
| **New:** `delete_wizard.go` | — | ~300 (extracted) |

---

## Phase 1: Extract Import Wizard

**Goal:** Make `ImportWizardModel` a standalone Bubble Tea model.

### 1.1 Create Import Wizard Model

```go
// import_wizard_model.go (new file)

package main

type ImportWizardModel struct {
    // All import-related fields from Model
    step         int
    namespaces   []namespaceInfo
    showAllNS    bool
    namespace    string
    space        string
    workloads    []WorkloadInfo
    selected     []bool
    cursor       int
    loading      bool
    err          error
    applyError   error
    progress     int
    total        int
    extractDone  bool
    // ... etc

    // Dependencies
    width, height int
    spinner       spinner.Model
}

func NewImportWizard(width, height int) ImportWizardModel {
    return ImportWizardModel{
        step:    0,
        spinner: spinner.New(),
        width:   width,
        height:  height,
    }
}

func (m ImportWizardModel) Init() tea.Cmd {
    return tea.Batch(m.spinner.Tick, loadNamespacesCmd())
}

func (m ImportWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Move updateImportWizard logic here
}

func (m ImportWizardModel) View() string {
    // Move renderImportWizard logic here
}

// Result returns the import result when wizard completes
func (m ImportWizardModel) Result() *ImportResult {
    // Return what was imported
}

func (m ImportWizardModel) Done() bool {
    return m.step == importStepDone
}

func (m ImportWizardModel) Cancelled() bool {
    return m.step == importStepCancelled
}
```

### 1.2 Update Main Model

```go
// hierarchy_types.go

type Model struct {
    // Tree navigation (keep)
    nodes, flatList, cursor, ...

    // Search (keep)
    searchMode, searchQuery, ...

    // Auth (keep)
    authPrompt, authOrgName, ...

    // Wizards (now sub-models)
    importWizard *ImportWizardModel  // nil when not active
    createWizard *CreateWizardModel
    deleteWizard *DeleteWizardModel
}
```

### 1.3 Update Main Update()

```go
// hierarchy.go

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Delegate to active wizard
    if m.importWizard != nil {
        wizard, cmd := m.importWizard.Update(msg)
        m.importWizard = wizard.(*ImportWizardModel)

        if m.importWizard.Done() {
            result := m.importWizard.Result()
            m.importWizard = nil
            return m, m.handleImportResult(result)
        }
        if m.importWizard.Cancelled() {
            m.importWizard = nil
            return m, nil
        }
        return m, cmd
    }

    // ... rest of Update
}
```

### 1.4 Files Changed

| File | Change |
|------|--------|
| `import_wizard_model.go` | **New** — standalone model |
| `import_wizard.go` | Delete or rename to `import_wizard_views.go` (just View helpers) |
| `hierarchy_types.go` | Remove import fields, add `importWizard *ImportWizardModel` |
| `hierarchy.go` | Simplify Update() to delegate to wizard |

---

## Phase 2: Extract Create Wizard

Same pattern as import wizard.

### 2.1 Create Wizard Model

```go
// create_wizard_model.go

type CreateWizardModel struct {
    resourceType string  // "space", "unit", "target"
    step         int
    name         string
    space        string
    cloneFrom    string
    target       string
    worker       string
    provider     string

    // Lists for selection
    spaces   []string
    units    []string
    targets  []string
    workers  []string

    cursor   int
    loading  bool
    err      error

    width, height int
    textInput     textinput.Model
}

func NewCreateWizard(resourceType string, w, h int) CreateWizardModel
func (m CreateWizardModel) Init() tea.Cmd
func (m CreateWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m CreateWizardModel) View() string
func (m CreateWizardModel) Result() *CreateResult
func (m CreateWizardModel) Done() bool
func (m CreateWizardModel) Cancelled() bool
```

### 2.2 Files Changed

| File | Change |
|------|--------|
| `create_wizard_model.go` | **New** |
| `hierarchy_types.go` | Remove create fields |
| `hierarchy.go` | Remove `updateCreateWizard`, delegate instead |

---

## Phase 3: Extract Delete Wizard

Simpler than the others — just confirmation dialog.

### 3.1 Delete Wizard Model

```go
// delete_wizard_model.go

type DeleteWizardModel struct {
    resourceType string  // "space", "unit", "target"
    resourceName string
    space        string

    confirmed bool
    loading   bool
    err       error

    width, height int
}

func NewDeleteWizard(resourceType, name, space string, w, h int) DeleteWizardModel
func (m DeleteWizardModel) Init() tea.Cmd
func (m DeleteWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m DeleteWizardModel) View() string
func (m DeleteWizardModel) Confirmed() bool
func (m DeleteWizardModel) Cancelled() bool
```

---

## Phase 4: Add Tests

### 4.1 Test Structure

```
cmd/cub-agent/
├── import_wizard_model.go
├── import_wizard_model_test.go    ← New
├── create_wizard_model.go
├── create_wizard_model_test.go    ← New
├── delete_wizard_model.go
├── delete_wizard_model_test.go    ← New
├── hierarchy_test.go              ← Existing (expand)
└── testdata/
    ├── import_wizard_*.golden     ← New golden files
    ├── create_wizard_*.golden
    └── delete_wizard_*.golden
```

### 4.2 Test Patterns

Use teatest for interaction testing:

```go
// import_wizard_model_test.go

func TestImportWizard_NamespaceSelection(t *testing.T) {
    m := NewImportWizard(80, 24)

    // Simulate namespaces loaded
    m, _ = m.Update(namespacesLoadedMsg{
        namespaces: []namespaceInfo{
            {name: "default", workloadCount: 5},
            {name: "kube-system", workloadCount: 10, isSystem: true},
            {name: "app", workloadCount: 3},
        },
    }).(ImportWizardModel)

    tm := teatest.NewTestModel(t, m)

    // Navigate down
    tm.Send(tea.KeyMsg{Type: tea.KeyDown})
    tm.Send(tea.KeyMsg{Type: tea.KeyDown})

    // Select namespace
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

    final := tm.FinalModel(t).(ImportWizardModel)
    assert.Equal(t, "app", final.namespace)
    assert.Equal(t, 1, final.step)  // Moved to workload selection
}

func TestImportWizard_Cancel(t *testing.T) {
    m := NewImportWizard(80, 24)

    tm := teatest.NewTestModel(t, m)
    tm.Send(tea.KeyMsg{Type: tea.KeyEsc})

    final := tm.FinalModel(t).(ImportWizardModel)
    assert.True(t, final.Cancelled())
}

func TestImportWizard_GoldenView(t *testing.T) {
    m := NewImportWizard(80, 24)
    m, _ = m.Update(namespacesLoadedMsg{...}).(ImportWizardModel)

    tm := teatest.NewTestModel(t, m)
    out, _ := io.ReadAll(tm.FinalOutput(t))
    teatest.RequireEqualOutput(t, out)  // Compares to testdata/*.golden
}
```

### 4.3 Mock Data Fetching

For tests, mock the command execution:

```go
// test_helpers.go

type mockDataFetcher struct {
    namespaces []namespaceInfo
    workloads  []WorkloadInfo
    err        error
}

func (f *mockDataFetcher) loadNamespacesCmd() tea.Cmd {
    return func() tea.Msg {
        if f.err != nil {
            return namespacesLoadedMsg{err: f.err}
        }
        return namespacesLoadedMsg{namespaces: f.namespaces}
    }
}
```

### 4.4 Test Coverage Goals

| Component | Current | Target |
|-----------|---------|--------|
| Import Wizard | ~30% | 80% |
| Create Wizard | 0% | 80% |
| Delete Wizard | 0% | 80% |
| Tree Navigation | ~50% | 70% |
| Search | 0% | 60% |

---

## Phase 5: Prepare for Merge

### 5.1 Identify Shared Types

Types that will be shared between TUI and CLI after merge:

```go
// These are currently defined in hierarchy_types.go
// They should move to a shared location in cub

type CubContext struct { ... }
type CubOrganization struct { ... }
type CubSpaceData struct { ... }
type CubUnitData struct { ... }
type CubTargetData struct { ... }
type CubWorkerData struct { ... }
```

### 5.2 Document runCubCommand Calls

Create a list of all CLI calls that will become direct functions:

| Current Call | After Merge |
|--------------|-------------|
| `runCubCommand("context", "get", "--json")` | `context.Get()` |
| `runCubCommand("organization", "list", "--json")` | `organization.List()` |
| `runCubCommand("space", "list", "--json")` | `space.List()` |
| `runCubCommand("space", "create", name)` | `space.Create(name)` |
| `runCubCommand("unit", "list", "--space", s)` | `unit.List(space)` |
| `runCubCommand("unit", "create", ...)` | `unit.Create(...)` |
| `runCubCommand("target", "list", "--space", s)` | `target.List(space)` |
| `runCubCommand("worker", "list", "--space", s)` | `worker.List(space)` |

### 5.3 Don't Refactor runCubCommand

Leave `runCubCommand` as-is. It will be replaced wholesale during merge. Refactoring it now is wasted effort.

---

## Execution Order

### Week 1: Import Wizard Extraction

- [ ] Create `import_wizard_model.go` with `ImportWizardModel` struct
- [ ] Move `Init()`, `Update()`, `View()` methods
- [ ] Update `hierarchy.go` to delegate to wizard
- [ ] Remove import fields from main `Model`
- [ ] Verify TUI still works identically
- [ ] Add `import_wizard_model_test.go` with basic tests

### Week 2: Create/Delete Wizard Extraction

- [ ] Create `create_wizard_model.go`
- [ ] Create `delete_wizard_model.go`
- [ ] Update main `Model` and `Update()`
- [ ] Add tests for both wizards
- [ ] Update golden files

### Week 3: Test Coverage

- [ ] Add comprehensive teatest coverage for all wizards
- [ ] Add golden file tests for view regression
- [ ] Add error case tests
- [ ] Reach 80% coverage on wizard code

### Week 4: Merge Prep

- [ ] Document all `runCubCommand` calls
- [ ] Identify shared types
- [ ] Create migration checklist for merge
- [ ] Final cleanup and documentation

---

## Success Criteria

### Before Merge

- [ ] `Model` struct has < 20 fields (down from 40+)
- [ ] Each wizard is a standalone `tea.Model`
- [ ] Test coverage > 70% on TUI code
- [ ] `hierarchy.go` < 3,000 lines (down from 5,116)
- [ ] All existing functionality preserved

### After Merge

- [ ] `runCubCommand` deleted entirely
- [ ] Types imported from `cub` packages
- [ ] Single binary build
- [ ] Tests still pass

---

## What We're NOT Doing

| Temptation | Why Not |
|------------|---------|
| Modular monolith architecture | Over-engineering; `pkg/` is already clean |
| `pkg/confighub/client.go` | Merge makes this unnecessary |
| Event bus | Not needed |
| Plugin system | Deferred until real demand |
| Refactoring `pkg/agent/` | Already well-structured |

**Focus:** Clean TUI code. That's it.

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking existing TUI | Golden file tests catch regressions |
| Wizard extraction changes behavior | Test each wizard in isolation before integration |
| Merge timeline slips | Refactored code is still better even without merge |
| Scope creep | Strictly limit to wizard extraction + tests |

---

## References

- `docs/planning/CODE-REVIEW-2026-01-12.md` — Full code review
- `docs/planning/PLUGIN-ARCHITECTURE.md` — Deferred architecture (for future reference)
- `cmd/cub-agent/hierarchy_test.go` — Existing teatest patterns
- `cmd/cub-agent/import_wizard_test.go` — Existing wizard tests

---

## Code Review: 2026-01-13 (Brian-style Review)

### Issues Found

#### 1. ~~DUPLICATE COMMAND NAMES: `fleet`~~ ✓ FIXED

**Problem:** Two different commands had the same name.

**Fix applied (2026-01-13):** Renamed `cub-agent fleet` → `cub-agent import-cluster-aggregator`

Now:
- `cub-agent import-cluster-aggregator` (fleet.go) — Aggregates multi-cluster imports from JSON files
- `cub-agent map fleet` (map.go) — Shows fleet view grouped by app/variant

#### 2. ~~IGNORED ERRORS~~ ✓ FIXED

**Fix applied (2026-01-13):**

- **Dead code removed:** `map.go` — removed unused `list` variable and suppression
- **Comments added:** All best-effort operations now have `//nolint:errcheck // <reason>` comments:
  - `hierarchy.go:511-519` — kubectl cleanup commands (best-effort cleanup)
  - `hierarchy.go:671` — labelWorkload (best-effort labeling)
  - `hierarchy.go:894` — cmd.Wait() in goroutine (background process)
  - `hierarchy.go:1439, 2313` — open command (best-effort browser open)
  - `import_wizard.go:1741` — cmd.Wait() in goroutine (background process)
- **Error now handled:** `import.go:1209` — context set failure now returns error

#### 3. ~~INCONSISTENT ERROR PATTERNS~~ ✓ DOCUMENTED

**Note (2026-01-13):** The patterns are actually consistent within their domains:

| Domain | Pattern | Reason |
|--------|---------|--------|
| CLI commands | `return fmt.Errorf(...)` | Standard Go error handling |
| Bubble Tea TUI | `return msg{err: ...}` | TUI uses message passing |
| Best-effort ops | `//nolint:errcheck` | Non-critical, documented |

The apparent inconsistency is intentional: TUI code uses message-based error propagation (Elm architecture), while CLI code uses standard Go errors. No changes needed.

#### 4. 105 STRUCT TYPES IN cmd/cub-agent/

Excessive type proliferation in package main:
- `FleetResult`, `FleetUnit`, `FleetSummary` (fleet.go + map.go — overlap?)
- `ImportResult`, `WorkloadInfo`, `SuggestionJSON`, `UnitJSON` (import.go)
- `RemedyOutput`, `RemedyFindingOut`, `CCVEDefinition` (remedy.go)
- Many more...

**Fix:** Consider moving shared types to `pkg/types/` before merge.

#### 5. NEW FILE: remedy.go

**Status:** Just added (2026-01-13), looks clean.

**Observations:**
- Imports `pkg/remedy` which is new
- Good separation of concerns
- Uses color constants from `trace.go` (shared correctly)
- Has dry-run by default (safe)

**No issues found** — this is a good addition.

### Issues NOT Found (Good News)

| Pattern | Status |
|---------|--------|
| Fake HTTP APIs | ✓ None found (cleaned up previously) |
| CONFIGHUB_*_TOKEN env vars | ✓ None found |
| Speculative scaffolding | ✓ None found |
| Dead imports | ✓ go vet passes |
| Build errors | ✓ go build passes |

### Priority Fixes

| Issue | Effort | Priority | Status |
|-------|--------|----------|--------|
| ~~Rename duplicate `fleet` command~~ | Low | **High** | ✓ Done |
| ~~Document ignored errors~~ | Low | Medium | ✓ Done |
| ~~Consolidate error patterns~~ | Medium | Low | ✓ N/A (already consistent) |
| Move types to pkg/types/ | Medium | Low | Deferred |

---

*This plan supersedes the modular monolith architecture for near-term work.*
