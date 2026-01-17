# Plan: GitOps Trace Integration (Flux + Argo CD)

**Status:** Proposed
**Created:** 2026-01-08
**Author:** ConfigHub Team

---

## Problem Statement

Current ownership detection (`pkg/agent/ownership.go`) only identifies the **immediate** owner of a resource via labels:

```
Deployment/nginx → owner: Flux Kustomization "apps"
```

But users need the **full ownership chain**:

```
GitRepository/infra → Kustomization/apps → Deployment/nginx
```

The `flux trace` command provides exactly this — the complete delivery pipeline from source to deployed resource.

---

## What `flux trace` Provides

```bash
$ flux trace deployment nginx -n demo
Object:        Deployment/nginx
Namespace:     demo
Status:        Managed by Flux
---
Kustomization: apps
Namespace:     flux-system
Path:          ./clusters/prod/apps
Revision:      main@sha1:abc123
Status:        Applied revision main@sha1:abc123
---
GitRepository: infra-repo
Namespace:     flux-system
URL:           https://github.com/company/infra.git
Revision:      main@sha1:abc123
Status:        Artifact is up to date
```

Key information we don't currently capture:
1. **Full chain** — GitRepo → Kustomization → Resource (or GitRepo → HelmRepo → HelmRelease → Resource)
2. **Path in repo** — `./clusters/prod/apps`
3. **Revision at each level** — Know exactly which commit deployed this
4. **Status at each level** — Where in the chain did something break?

---

## Use Cases

### 1. Enhanced Pipelines View

**Current:**
```
PIPELINES
✓ infra-repo@main → apps → 5 resources
```

**With flux trace:**
```
PIPELINES
✓ github.com/company/infra@sha1:abc123
  └─▶ Kustomization/apps (./clusters/prod/apps)
      └─▶ 5 Deployments, 3 Services, 2 ConfigMaps
```

### 2. Debugging "Why isn't my change deployed?"

```bash
./test/atk/map trace deployment/nginx -n demo
```

Output:
```
TRACE: deployment/nginx

GitRepository/infra-repo     ✓ main@sha1:abc123  (2m ago)
    └─▶ Kustomization/apps   ✓ Applied sha1:abc123
        └─▶ Deployment/nginx ✓ 3/3 ready

All levels in sync. Change will deploy when pushed.
```

Or when broken:
```
TRACE: deployment/nginx

GitRepository/infra-repo     ✓ main@sha1:def456  (2m ago)
    └─▶ Kustomization/apps   ✗ Reconciliation failed
        │   Error: path './clusters/prod/apps' not found
        └─▶ Deployment/nginx ⚠ Running stale sha1:abc123
```

### 3. Orphan Detection Enhancement

Currently we detect orphans via missing labels. With trace:
- Confirm resource truly has no Flux/Argo owner
- Distinguish "never managed" from "was managed, now orphaned"

### 4. CCVE Enhancement

New CCVEs possible:
- `CCVE-FLUX-TRACE-001`: Resource not in any Flux trace (orphan)
- `CCVE-FLUX-TRACE-002`: Trace shows stale revision (drift)
- `CCVE-FLUX-TRACE-003`: Trace chain broken at intermediate level

---

## Implementation Phases

### Phase 1: `flux trace` CLI Integration

**Goal:** Call `flux trace` and parse its output

**Files to modify:**
- `pkg/agent/flux_trace.go` (new)
- `pkg/agent/ownership.go` (enhance)

**Implementation:**

```go
// pkg/agent/flux_trace.go

type FluxTraceResult struct {
    Object      ResourceRef
    Chain       []ChainLink
    FullyManaged bool
    Error       error
}

type ChainLink struct {
    Kind      string  // GitRepository, Kustomization, HelmRelease
    Name      string
    Namespace string
    Path      string  // for Kustomization
    Revision  string
    Status    string
    Ready     bool
}

func FluxTrace(ctx context.Context, kind, name, namespace string) (*FluxTraceResult, error) {
    cmd := exec.CommandContext(ctx, "flux", "trace", kind, name, "-n", namespace)
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("flux trace failed: %w", err)
    }
    return parseFluxTraceOutput(output)
}
```

**Deliverable:** `./cub-scout trace <resource>` command that calls flux trace

### Phase 2: Integrate into Map

**Goal:** Enhance pipelines view with trace data

**Files to modify:**
- `test/atk/map` (enhance pipelines view)
- `cmd/cub-scout/map.go` (add trace subcommand)

**Implementation:**
- Add `map trace` subcommand
- Enhance `map pipelines` to show trace chains
- Cache trace results (expensive operation)

### Phase 3: Batch Tracing for Overview

**Goal:** Trace all Flux-managed resources efficiently

**Challenge:** `flux trace` is slow (~500ms per resource)

**Solution:**
1. Only trace deployers (Kustomizations, HelmReleases) not every resource
2. Build chain from deployer data, not individual traces
3. Use trace only for debugging specific resources

**Files to modify:**
- `pkg/agent/agent.go` (add batch trace option)
- `test/atk/map` (optional --trace flag)

### Phase 4: CCVE Trace Checks

**Goal:** New CCVE patterns using trace data

**New CCVEs:**
```yaml
# CCVE-FLUX-TRACE-001
name: flux-trace-orphan
title: "Resource not in any Flux trace"
category: ORPHAN
detection:
  method: flux_trace_missing
```

### Phase 5: TUI Trace Tab

**Goal:** Interactive trace view in the TUI dashboard

**Files to modify:**
- `test/atk/map` (add trace tab/view)
- `test/atk/lib/ui.sh` (add trace tree rendering)

**User flow:**
1. User runs `./test/atk/map` (TUI mode)
2. Navigate to workloads or deployers view
3. Press `t` on any resource to trace it
4. Trace tab opens showing full ownership chain

**Mockup:**

```
┌─ TRACE: deployment/nginx ────────────────────────────────────────┐
│                                                                   │
│  ✓ GitRepository/infra-repo                                       │
│  │ URL: https://github.com/company/infra.git                      │
│  │ Revision: main@sha1:abc123                                     │
│  │ Status: Artifact is up to date (2m ago)                        │
│  │                                                                │
│  └─▶ ✓ Kustomization/apps                                         │
│      │ Path: ./clusters/prod/apps                                 │
│      │ Revision: main@sha1:abc123                                 │
│      │ Status: Applied successfully                               │
│      │                                                            │
│      └─▶ ✓ Deployment/nginx                                       │
│          │ Namespace: demo                                        │
│          │ Replicas: 3/3 ready                                    │
│          │ Image: nginx:1.25                                      │
│          └─▶ Pod/nginx-abc12 ✓                                    │
│          └─▶ Pod/nginx-def34 ✓                                    │
│          └─▶ Pod/nginx-ghi56 ✓                                    │
│                                                                   │
├───────────────────────────────────────────────────────────────────┤
│ [↑↓] Navigate  [Enter] Expand  [d] Diff  [l] Logs  [q] Back       │
└───────────────────────────────────────────────────────────────────┘
```

**When broken:**

```
┌─ TRACE: deployment/nginx ────────────────────────────────────────┐
│                                                                   │
│  ✓ GitRepository/infra-repo                                       │
│  │ Revision: main@sha1:def456 (2m ago)                            │
│  │                                                                │
│  └─▶ ✗ Kustomization/apps                    ← PROBLEM HERE       │
│      │ Status: Reconciliation failed                              │
│      │ Error: path './clusters/prod/apps' not found               │
│      │                                                            │
│      └─▶ ⚠ Deployment/nginx (stale)                               │
│          │ Running: sha1:abc123 (old)                             │
│          │ Expected: sha1:def456 (new)                            │
│                                                                   │
├───────────────────────────────────────────────────────────────────┤
│ ⚠ Chain broken at Kustomization/apps - check path exists in repo  │
└───────────────────────────────────────────────────────────────────┘
```

**Key bindings in trace view:**

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate chain |
| `Enter` | Expand/collapse node details |
| `d` | Show diff (live vs desired) |
| `l` | Show logs (for pods) |
| `h` | Show history (revisions) |
| `r` | Refresh trace |
| `q` | Back to previous view |

**Integration with existing TUI:**

Current tabs: `[o]verview  [w]orkloads  [d]eployers  [s]ources  p[i]pelines`

New: `[o]verview  [w]orkloads  [d]eployers  [s]ources  p[i]pelines  [t]race`

Or: Keep trace as a drill-down from any view (press `t` on selected resource)

---

## Technical Considerations

### 1. Performance

`flux trace` is slow (~500ms per resource). Strategies:
- Only trace on-demand (not in overview)
- Cache results with TTL
- Batch by inferring chains from Kustomization/HelmRelease data

### 2. Flux CLI Dependency

Requires `flux` CLI installed. Options:
- Check for flux CLI, graceful fallback
- Use Flux's Go packages directly (more complex)
- Document as optional enhancement

### 2b. flux-operator FluxReport CRD (Alternative)

If the [ControlPlane flux-operator](https://github.com/controlplaneio-fluxcd/flux-operator) is installed, we can query the `FluxReport` CRD directly for structured status info:

```yaml
apiVersion: fluxcd.controlplane.io/v1
kind: FluxReport
# Provides: deployment readiness, reconciler statistics, CRD versions, cluster sync status
```

**Benefits:**
- No CLI dependency (direct K8s API)
- Batch-friendly (one resource, all status)
- Includes Prometheus metrics

**Detection:** Check if `fluxreports.fluxcd.controlplane.io` CRD exists

### 3. Argo CD Equivalent

Argo CD has no single `argocd trace` command, but equivalent data via:

| Command | What It Shows |
|---------|---------------|
| `argocd app get <app>` | Sync status, health, all managed resources |
| `argocd app history <app>` | Deployment history with revisions |
| `argocd app diff <app>` | Live state vs Git state (drift) |
| `argocd app logs <app>` | Pod logs for debugging |

**Implementation approach:**
- Use `argocd app get --output json` to get full resource tree
- Parse `status.resources[]` for managed resources with sync status
- Parse `status.history[]` for revision chain
- Combine into same `TraceResult` structure as Flux

```bash
# Argo equivalent trace output we'll generate:
Application/frontend-app     ✓ Synced (sha1:abc123)
    └─▶ Deployment/frontend  ✓ 3/3 ready
    └─▶ Service/frontend     ✓ healthy
    └─▶ ConfigMap/frontend   ✓ synced
```

---

## Milestones

| Phase | Deliverable | Effort |
|-------|-------------|--------|
| **1a** | `cub-scout trace` for Flux resources | 2-3 hours |
| **1b** | `cub-scout trace` for Argo resources | 2-3 hours |
| **2** | `map trace` and enhanced pipelines | 3-4 hours |
| **3** | Efficient batch tracing for overview | 4-6 hours |
| **4** | CCVE patterns using trace | 2-3 hours |
| **5** | TUI trace tab with interactive navigation | 4-6 hours |

**Total:** ~18-25 hours of work

### Phase 1b Detail: Argo CD Trace

```go
// pkg/agent/argo_trace.go

func ArgoTrace(ctx context.Context, appName string) (*TraceResult, error) {
    // Get full app status with managed resources
    cmd := exec.CommandContext(ctx, "argocd", "app", "get", appName, "-o", "json")
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    var app ArgoApp
    json.Unmarshal(output, &app)

    // Build chain: Application → managed resources
    result := &TraceResult{
        Chain: []ChainLink{{
            Kind:     "Application",
            Name:     app.Metadata.Name,
            Revision: app.Status.Sync.Revision,
            Status:   app.Status.Sync.Status,
            Ready:    app.Status.Health.Status == "Healthy",
        }},
    }

    // Add managed resources
    for _, res := range app.Status.Resources {
        result.Chain = append(result.Chain, ChainLink{
            Kind:      res.Kind,
            Name:      res.Name,
            Namespace: res.Namespace,
            Status:    res.Status,
            Ready:     res.Health.Status == "Healthy",
        })
    }

    return result, nil
}
```

---

## Commands After Implementation

```bash
# CLI: Trace a Flux-managed resource (uses flux trace)
./cub-scout trace deployment/nginx -n demo

# CLI: Trace an Argo-managed resource (uses argocd app get)
./cub-scout trace deployment/frontend -n demo
# Auto-detects owner and uses appropriate tool

# CLI: Trace by Argo application name directly
./cub-scout trace --app frontend-app

# CLI: Map with trace chains
./test/atk/map pipelines --trace

# CLI: Debug why change isn't deployed
./test/atk/map trace deployment/broken-app

# TUI: Interactive trace
./test/atk/map                    # Launch TUI
# Press 'w' for workloads, select resource, press 't' to trace
# Or press 't' directly for trace tab (shows recent/pinned traces)
```

**Auto-detection logic:**
1. Check resource labels for Flux ownership → use `flux trace`
2. Check resource labels for Argo ownership → use `argocd app get`
3. Neither → report "not managed by GitOps"

---

## Open Questions

1. **Should trace be opt-in or default?**
   - Recommendation: Opt-in for overview, always-on for `map trace`

2. **CLI availability detection?**
   - What if `flux` CLI installed but not `argocd`? (or vice versa)
   - Recommendation: Graceful per-tool fallback with warning

3. **Cache strategy?**
   - In-memory with 5min TTL?
   - Persist to disk for multi-command sessions?

4. **Unified output format?**
   - Same `TraceResult` struct for both Flux and Argo
   - Allows consistent UI regardless of underlying tool

5. **TUI trace tab vs drill-down?**
   - Option A: Dedicated `[t]race` tab showing recent/pinned traces
   - Option B: Trace as drill-down only (press `t` on any resource)
   - Option C: Both — tab shows history, `t` opens trace for selected
   - Recommendation: Option C for maximum flexibility

---

## Future Work (TODO)

### ConfigHub Source Tracing

**GitHub Issue:** https://github.com/confighubai/confighub-agent/issues/3

Currently trace only covers Flux and Argo CD. Future enhancement should trace resources through **all** sources:

1. **GitOps (Flux/Argo)** — Already implemented ✓
2. **ConfigHub Managed** — Trace through ConfigHub's Unit/Target/Cluster hierarchy
3. **Raw kubectl apply** — Identify resources created manually (orphans)
4. **Helm (standalone)** — Trace Helm releases not managed by Flux/Argo
5. **Kustomize (standalone)** — Trace kustomize builds applied directly

**Full trace vision:**

```
┌─ TRACE: Deployment/payment-api ─────────────────────────────────────┐
│                                                                      │
│  ConfigHub Unit/payment-service                                      │
│    │ Organization: acme-corp                                         │
│    │ Space: production                                               │
│    │ Target: cluster-east                                            │
│    │                                                                 │
│    └─▶ GitRepository/payment-charts                                  │
│        │ URL: https://github.com/acme/payment-charts.git             │
│        │ Revision: v2.1.0                                            │
│        │                                                             │
│        └─▶ HelmRelease/payment-api                                   │
│            │ Chart: payment-api                                      │
│            │ Status: Synced                                          │
│            │                                                         │
│            └─▶ Deployment/payment-api                                │
│                  Status: 3/3 ready                                   │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│ ✓ Full chain traced: ConfigHub → Git → Flux → Kubernetes            │
└──────────────────────────────────────────────────────────────────────┘
```

**For orphan resources (raw kubectl apply):**

```
TRACE: Deployment/mystery-app

  ⚠ No GitOps owner detected
    │ Labels: app=mystery-app
    │ Created: 2025-12-15 by: kubectl
    │ Last modified: 2026-01-05
    │
    └─▶ Deployment/mystery-app
          Status: Running (no sync tracking)

⚠ Resource not managed by GitOps - consider adding to a Kustomization
```

**Implementation approach:**
- Query ConfigHub API for source metadata
- Check Helm release secrets for standalone Helm
- Detect "last-applied-configuration" annotation for kubectl apply
- Integrate with existing `TraceResult` structure
- Show full provenance chain regardless of management method

---

## See Also

- [Flux trace documentation](https://fluxcd.io/flux/cmd/flux_trace/)
- [Argo CD app get documentation](https://argo-cd.readthedocs.io/en/stable/user-guide/commands/argocd_app_get/)
- [pkg/agent/ownership.go](../../pkg/agent/ownership.go) — Current ownership detection
- [test/atk/map](../../test/atk/map) — Current pipelines implementation
