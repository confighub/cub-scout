# Mental Model: cub-scout in 3 Minutes

> Status: Current (Primary)
> Last reviewed: 2026-02-12
> Concepts index: [README.md](README.md)

cub-scout helps you understand what's running in your Kubernetes cluster and where it came from.

---

## The Core Question

**"Who owns this resource?"**

Every resource in Kubernetes was created by something:
- **Flux** (Kustomization, HelmRelease)
- **ArgoCD** (Application)
- **Helm** (standalone release)
- **kubectl** (manual / Native)

cub-scout reads labels and annotations to answer this question definitively.

---

## Three Interfaces, One Tool

| Interface | Command | Use When |
|-----------|---------|----------|
| **TUI** | `cub-scout map` | Exploring, debugging, learning |
| **CLI** | `cub-scout map list` | Scripts, one-liners, CI |
| **JSON** | `--json` flag | Automation, downstream tools |

All three interfaces produce the same information. Choose based on your workflow.

**TUI example:**
```bash
cub-scout map          # Interactive dashboard
# Press 's' for status, 'w' for workloads, '?' for help
```

**CLI example:**
```bash
cub-scout map list -q "owner=Native"   # Find unmanaged resources
cub-scout map status                    # One-line health check
```

**JSON example:**
```bash
cub-scout map list --json | jq '.[] | select(.owner=="Native")'
```

---

## Standalone vs Connected

### Standalone (Default)

- **No network required** — works offline, air-gapped, restricted
- **Reads kubectl context** — uses your existing cluster access
- **Deterministic output** — same input = same output, always
- **No signup needed** — fully functional immediately

Everything documented in this repo works standalone.

### Connected (Optional)

Connected mode adds ConfigHub features:
- Fleet queries across multiple clusters
- Import workloads for tracking
- DRY↔WET↔LIVE comparison
- Revision history

To enable: `cub auth login`

Connected mode is **optional and additive**. All standalone features continue to work.

---

## Maps & Trees

cub-scout provides multiple views into your cluster:

### Runtime Tree
```bash
cub-scout tree runtime
```
Shows: Deployment → ReplicaSet → Pod

### Ownership Tree
```bash
cub-scout tree ownership
```
Shows: Resources grouped by GitOps owner (Flux, ArgoCD, Helm, Native)

### Git Tree
```bash
cub-scout tree git
```
Shows: Detected Git repository structure

### Composition Tree (Crossplane)
```bash
cub-scout tree composition
```
Shows: XR → Composed resources

---

## Outputs Are Deterministic

cub-scout follows a strict contract:

1. **Same input = same output** — no randomness, no ML inference
2. **JSON is the source of truth** — ASCII is just a rendering
3. **Schemas are versioned** — `attribution-graph.v1`, `attribution-report.v1`

This makes cub-scout safe for:
- CI pipelines
- GitOps automation
- Audit trails
- Reproducible debugging

---

## Quick Reference

| Task | Command |
|------|---------|
| Explore cluster | `cub-scout map` |
| List resources | `cub-scout map list` |
| Health check | `cub-scout map status` |
| Find orphans | `cub-scout map orphans` |
| Trace to Git | `cub-scout trace deploy/x -n ns` |
| View trees | `cub-scout tree ownership` |
| Scan for issues | `cub-scout scan` |
| Export as JSON | Add `--json` to any command |

---

## Learn More

- [CLI Guide](../../CLI-GUIDE.md) — Workflow-first CLI tour
- [Query Syntax](../reference/query-syntax.md) — Filtering resources
- [Tree Hierarchies](../howto/tree-hierarchies.md) — Using tree views
- [Why Connected Mode](why-connected-mode.md) — ConfigHub features
