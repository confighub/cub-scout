# First Map: Your Cluster in 5 Minutes

**Time:** 5 minutes
**Goal:** See your cluster, understand ownership, trace resource chains

---

## Launch the Map

```bash
cub-scout map
```

**What you'll see:**

```
┌─ ⚡ CUB-SCOUT MAP ───────────────────────────────────────────────────────────┐
│                                                                              │
│  Context: kind-demo                                                          │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ CLUSTER HEALTH ─────────────────────────────────────────────────────────────┐
│                                                                              │
│  ████████████████████░░░░  85%  (17/20 ready)                               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ RESOURCES ────────────────┬─ PIPELINES ─────────────────────────────────────┐
│                            │                                                 │
│  Flux        8  ████████   │  ✓ GitRepo → Kustomization → Deployment        │
│  ArgoCD      5  █████      │  ✓ GitRepo → Application → Deployment          │
│  Helm        4  ████       │  ⚠ HelmRelease pending                         │
│  Native      3  ███        │                                                 │
│                            │                                                 │
└────────────────────────────┴─────────────────────────────────────────────────┘

Keys: [s]tatus [w]orkloads [o]rphans [4]deep-dive  [↑↓] navigate  [T]race  [q]uit
```

---

## Navigate Resources

Press **w** for Workloads view:

```
┌─ WORKLOADS ──────────────────────────────────────────────────────────────────┐
│                                                                              │
│  NAMESPACE        NAME                    KIND           OWNER    STATUS     │
│  ─────────────────────────────────────────────────────────────────────────   │
│  flux-system      source-controller       Deployment     Flux     ✓ Ready   │
│  flux-system      kustomize-controller    Deployment     Flux     ✓ Ready   │
│> podinfo          podinfo                 Deployment     Flux     ✓ Ready   │
│  monitoring       prometheus              StatefulSet    Helm     ✓ Ready   │
│  default          debug-nginx             Deployment     Native   ✓ Ready   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Status icons:**
- **✓ Green** — Healthy, all pods ready
- **⚠ Amber** — Partial (some pods not ready)
- **✗ Red** — Failed or missing

---

## Trace Ownership

Press **t** on any resource to trace where it came from:

```
┌─ TRACE: podinfo ─────────────────────────────────────────────────────────────┐
│                                                                              │
│  ┌─────────────────────────┐                                                │
│  │ GitRepository           │                                                │
│  │ flux-system/flux-system │                                                │
│  │ https://github.com/...  │                                                │
│  │ Revision: main@abc123   │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Kustomization           │                                                │
│  │ flux-system/apps        │                                                │
│  │ Path: ./apps/podinfo    │                                                │
│  │ Status: Applied         │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Deployment              │                                                │
│  │ podinfo/podinfo         │                                                │
│  │ Status: 2/2 Ready       │                                                │
│  └─────────────────────────┘                                                │
│                                                                              │
│  ✓ Full chain traced: Git → Flux → Kubernetes                               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Trace shows:**
- Where the config came from (Git)
- How it was deployed (Flux/Argo)
- Current state in cluster

---

## Find Orphans

Press **o** for Orphans view — resources NOT in Git:

```
┌─ ORPHANS (Not in Git) ───────────────────────────────────────────────────────┐
│                                                                              │
│  ⚠ These resources will be LOST on cluster rebuild                          │
│                                                                              │
│  NAMESPACE     NAME                KIND          SCENARIO                    │
│  ─────────────────────────────────────────────────────────────────────────   │
│  default       debug-nginx         Deployment    Left from debugging        │
│  default       manual-config       ConfigMap     Applied during incident    │
│  default       hotfix-credentials  Secret        Emergency rotation         │
│  kube-system   legacy-monitor      DaemonSet     Pre-GitOps installation    │
│                                                                              │
│  Found: 4 orphans across 2 namespaces                                       │
│                                                                              │
│  Next: Run 'cub-scout trace' on each to investigate                         │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Quick Commands

| Command | What it does |
|---------|-------------|
| `cub-scout map` | Interactive TUI |
| `cub-scout map workloads` | List all workloads |
| `cub-scout map orphans` | Find shadow IT |
| `cub-scout map status` | Health dashboard |
| `cub-scout trace deploy/X -n Y` | Trace to Git source |

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `s` | Status view |
| `w` | Workloads view |
| `o` | Orphans view |
| `4` | Deep-dive (Cluster Data) |
| `5`/`A` | App hierarchy |
| `↑`/`↓` | Navigate |
| `Enter` | View details |
| `T` | Trace ownership |
| `q` | Quit |
| `?` | Help |

---

## Next Steps

- **[Find orphans](../howto/find-orphans.md)** — Resources not in Git
- **[Trace ownership](../howto/trace-ownership.md)** — Full ownership chains
- **[Query resources](../howto/query-resources.md)** — Filter and search
