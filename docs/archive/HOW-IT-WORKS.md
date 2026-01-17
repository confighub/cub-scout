# How cub-agent Works

**Last Updated:** 2026-01-12

A practical guide to cub-agent: what it does, how it connects, and how to use it.

> **Looking for the protocol spec?** See [ARCHITECTURE.md](ARCHITECTURE.md) for GSF schema and API contracts.

---

## Table of Contents

- [What is cub-agent?](#what-is-cub-agent)
- [Two Operating Modes](#two-operating-modes)
- [System Diagram](#system-diagram)
- [Command Reference](#command-reference)
- [Authentication & Permissions](#authentication--permissions)
- [Relationship to ConfigHub](#relationship-to-confighub)
- [Deployment Options](#deployment-options)
- [Quick Reference](#quick-reference)
- [Related Documentation](#related-documentation)

---

## What is cub-agent?

**cub-agent** is a read-only Kubernetes observer that answers three questions:

1. **What's running?** — Discover all resources in your cluster
2. **Who owns it?** — Detect if Flux, ArgoCD, Helm, or native kubectl manages each resource
3. **Is it configured correctly?** — Scan for CCVEs (Configuration Common Vulnerabilities and Exposures)

It's a single Go binary that runs on your laptop, in CI, or as a Pod in your cluster.

```bash
# Quick examples
cub-agent map list          # What's running + who owns it
cub-agent scan                           # Find misconfigurations (CCVEs)
cub-agent trace deploy/nginx -n default  # Follow ownership chain
```

---

## Two Operating Modes

cub-agent has two distinct operating modes:

| Mode | Flag | Data Source | Auth Required |
|------|------|-------------|---------------|
| **Standalone** | (default) | Kubernetes API directly | kubeconfig only |
| **Connected** | (default) | ConfigHub API | ConfigHub token + kubeconfig |

### Standalone Mode

Works without any ConfigHub account. Reads directly from your Kubernetes cluster.

```bash
# List all resources and their owners
cub-agent map list

# Scan for CCVEs
cub-agent scan

# Trace ownership chain
cub-agent trace deploy/my-app -n production

# Export cluster state as JSON
cub-agent snapshot -n default
```

**Use cases:**
- Local development and debugging
- CI/CD pipeline checks
- Quick cluster inspection
- Teams not using ConfigHub

### ConfigHub Integration

Uses the `cub` CLI for fleet-wide visibility across multiple clusters.

```bash
# Authenticate to ConfigHub
cub auth login

# Import workloads into ConfigHub Units
cub-agent import -n my-namespace

# View fleet across spaces
cub-agent map fleet

# Interactive TUI for ConfigHub hierarchy
cub-agent map confighub
```

**Use cases:**
- Multi-cluster visibility
- Team collaboration via ConfigHub
- Import workloads into ConfigHub Units

---

## System Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                           cub-agent                                  │
│                        (single binary)                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   STANDALONE MODE                    CONFIGHUB MODE                  │
│   ───────────────                    ─────────────                   │
│                                                                      │
│   ┌─────────────────┐                ┌─────────────────┐            │
│   │ map list        │                │ map fleet       │            │
│   │ (default)       │                │ (uses cub CLI)  │            │
│   │                 │                │                 │            │
│   │ scan            │                │ hierarchy (TUI) │            │
│   │ trace           │                │ import          │            │
│   │ snapshot        │                │                 │            │
│   └────────┬────────┘                └────────┬────────┘            │
│            │                                  │                      │
│            ▼                                  ▼                      │
│   ┌─────────────────┐                ┌─────────────────┐            │
│   │ Kubernetes API  │                │ ConfigHub API   │            │
│   │                 │                │ confighub.com│           │
│   │ (via kubeconfig)│                └────────┬────────┘            │
│   └─────────────────┘                         │                      │
│            │                                  │ also uses            │
│            │                                  ▼                      │
│            │                         ┌─────────────────┐            │
│            │                         │ cub CLI         │            │
│            │                         │ (separate tool) │            │
│            │                         └────────┬────────┘            │
│            │                                  │                      │
│            ▼                                  ▼                      │
│   ┌─────────────────────────────────────────────────────┐           │
│   │                  Kubernetes Cluster                  │           │
│   │                                                      │           │
│   │   Flux CRDs    ArgoCD CRDs    Helm Secrets          │           │
│   │   Deployments  StatefulSets   Services   etc.       │           │
│   └─────────────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow: Standalone Mode

```
Your Terminal
     │
     ▼
cub-agent map list
     │
     ▼
~/.kube/config (your kubectl context)
     │
     ▼
Kubernetes API Server
     │
     ▼
Returns: Deployments, Services, Flux/Argo CRDs, etc.
```

### Data Flow: ConfigHub Mode (via cub CLI)

```
cub-agent import -n my-namespace
     │
     ├──► Kubernetes API (discover workloads)
     │         │
     │         ▼
     │    List Deployments, StatefulSets, etc.
     │         │
     │         ▼
     └──► cub CLI (create units)
              │
              ▼
         cub unit create <name>
              │
              ▼
         ConfigHub stores unit metadata
```

---

## Command Reference

### Standalone Commands (No ConfigHub Required)

| Command | Description | Example |
|---------|-------------|---------|
| `map list --standalone` | List resources + owners from cluster | `cub-agent map list -n prod` |
| `scan` | Detect CCVEs and stuck states | `cub-agent scan --namespace default` |
| `trace` | Follow GitOps ownership chain | `cub-agent trace deploy/nginx -n default` |
| `snapshot` | Export cluster state as GSF JSON | `cub-agent snapshot -n default -o state.json` |
| `parse-repo` | Analyze GitOps repo structure | `cub-agent parse-repo ./my-repo` |

### ConfigHub Commands (Require cub CLI)

| Command | Description | Requires |
|---------|-------------|----------|
| `map confighub` | Interactive TUI for ConfigHub | `cub` CLI authenticated |
| `import` | Import workloads as ConfigHub Units | `cub` CLI authenticated |
| `map fleet` | Aggregate view across clusters | `cub` CLI authenticated |

### Query Syntax (map list)

```bash
# Filter by field
cub-agent map list -q "kind=Deployment"
cub-agent map list -q "namespace=prod*"
cub-agent map list -q "owner=Flux"

# Combine with AND/OR
cub-agent map list -q "kind=Deployment AND owner!=Native"
cub-agent map list -q "owner=Flux OR owner=ArgoCD"

# Filter by label
cub-agent map list -q "labels[app]=nginx"
```

### Scan Modes

```bash
# Full scan (Kyverno + state + timing bombs)
cub-agent scan

# Kyverno PolicyReports only
cub-agent scan --kyverno

# Stuck reconciliation states only
cub-agent scan --state

# Timing bombs (expiring certs, quota limits)
cub-agent scan --timing-bombs

# Dangling/orphan resources
cub-agent scan --dangling

# JSON output
cub-agent scan --json
```

---

## Authentication & Permissions

### Standalone Mode: Uses Your Existing Credentials

cub-agent piggybacks on credentials you already have configured:

| What | How | Where Credentials Live |
|------|-----|------------------------|
| **Kubernetes API** | client-go library | `~/.kube/config` or `$KUBECONFIG` |
| **Flux tracing** | Shells to `flux` CLI | Uses kubeconfig (Flux reads K8s CRDs) |
| **ArgoCD tracing** | Shells to `argocd` CLI | `~/.argocd/config` |

**No special setup needed.** If you can run these commands, cub-agent works:

```bash
kubectl get deployments -A    # K8s access
flux get kustomizations -A    # Flux access (optional)
argocd app list               # ArgoCD access (optional)
```

### How cub-agent Reads from Each Tool

```go
// Kubernetes - uses client-go with your kubeconfig
loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, ...)

// Flux - shells out to flux CLI
cmd := exec.Command("flux", "trace", kind, name, "-n", namespace)

// ArgoCD - shells out to argocd CLI
cmd := exec.Command("argocd", "app", "get", appName, "-o", "json")
```

### Minimum RBAC for Read-Only Access

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cub-agent-readonly
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
```

For most users, your existing `cluster-admin` or dev context works fine.

### ConfigHub Integration

When using ConfigHub features (import, fleet view), authenticate via the cub CLI:

```bash
# Authenticate to ConfigHub
cub auth login

# Then use ConfigHub features in cub-agent
cub-agent import -n my-namespace    # Import to ConfigHub
cub-agent map fleet                 # View fleet across spaces
```

---

## Relationship to ConfigHub

### Three Related Tools

| Tool | Repository | Purpose |
|------|------------|---------|
| **cub** | confighub.com (closed source) | Official ConfigHub CLI - manages orgs, spaces, units, workers |
| **cub-agent** | this repo | K8s observer + TUI wrapper |
| **ConfigHub** | confighub.com | SaaS platform for fleet management |

### How They Interact

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   cub-agent     │     │    cub CLI      │     │   ConfigHub     │
│   (this repo)   │────►│  (separate)     │────►│   (SaaS)        │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │                       │
        │ K8s watcher           │ Unit/Space mgmt       │ Fleet data
        │ CCVE scanner          │ Worker mgmt           │ UI dashboard
        │ Trace/ownership       │ Auth (login)          │ API
        │                       │                       │
        ▼                       ▼                       ▼
   Standalone OK          Requires login         Requires account
```

### The TUI Shells to cub CLI

The `hierarchy` TUI in cub-agent calls `cub` CLI commands:

```go
// From hierarchy.go
exec.Command("cub", "space", "list", "--json")
exec.Command("cub", "unit", "apply", unitSlug, "--space", space)
exec.Command("cub", "worker", "run", workerName, "--space", space)
```

**Prerequisite for TUI:** Install `cub` CLI from https://docs.confighub.com/cli

---

## Deployment Options

### Option 1: Local CLI (Recommended for Getting Started)

Run on your laptop against any cluster you have kubectl access to:

```bash
# Install
go install github.com/confighub/confighub-agent/cmd/cub-agent@latest

# Or build from source
git clone https://github.com/confighubai/confighub-agent
cd confighub-agent
go build ./cmd/cub-agent

# Use
./cub-agent scan
./cub-agent map list
```

### Option 2: CI/CD Integration

Run in pipelines for pre-deploy checks:

```yaml
# GitHub Actions example
- name: Scan for CCVEs
  run: |
    cub-agent scan --json > ccve-report.json
    if jq -e '.findings | length > 0' ccve-report.json; then
      echo "CCVEs found!"
      exit 1
    fi
```

---

## Quick Reference

### What Works Without ConfigHub?

| Command | Standalone? | Notes |
|---------|-------------|-------|
| `map list --standalone` | ✅ Yes | Queries K8s directly |
| `scan` | ✅ Yes | CCVE detection from K8s |
| `trace` | ✅ Yes | Follows ownerRefs in K8s |
| `snapshot` | ✅ Yes | Dumps cluster as JSON |
| `parse-repo` | ✅ Yes | Analyzes Git repo locally |
| `map confighub` | ❌ No | Requires `cub` CLI authenticated |
| `map fleet` | ❌ No | Requires `cub` CLI authenticated |
| `import` | ❌ No | Creates Units via `cub` CLI |

### Where Does cub-agent Run?

| Location | Auth Method | Use Case |
|----------|-------------|----------|
| **Your laptop** | `~/.kube/config` | Development, debugging |
| **CI runner** | `$KUBECONFIG` or service account | Pre-deploy checks |
| **In-cluster Pod** | ServiceAccount token | Continuous monitoring |

### Summary

| Question | Answer |
|----------|--------|
| What is cub-agent? | Read-only K8s observer + CCVE scanner |
| Does it modify my cluster? | No, read-only (`get`, `list`, `watch` only) |
| Do I need ConfigHub? | No, standalone mode works without it |
| Where does it run? | Laptop, CI, or as a Pod in-cluster |
| How does it authenticate? | Uses your existing kubeconfig |
| What about Flux/ArgoCD? | Shells to their CLIs if installed |
| What's the TUI? | `map confighub` command - requires `cub` CLI |

---

## Related Documentation

### Conceptual

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | GSF protocol spec, API contracts |
| [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) | Terminology and concepts |
| [INTRODUCTION.md](INTRODUCTION.md) | High-level overview |

### User Guides

| Document | Description |
|----------|-------------|
| [README.md](../README.md) | Quick start and installation |
| [CCVE-GUIDE.md](CCVE-GUIDE.md) | Understanding CCVEs and the scanner |
| [CLI-REFERENCE.md](CLI-REFERENCE.md) | Full CLI documentation |
| [TESTING-GUIDE.md](TESTING-GUIDE.md) | Running tests |
| [EXAMPLES-OVERVIEW.md](EXAMPLES-OVERVIEW.md) | Central examples overview |

### TUI Documentation

| Document | Description |
|----------|-------------|
| [TUI-GUI-notes.md](TUI-GUI-notes.md) | TUI vs GUI: scope and capabilities |
| [TUI-TRACE.md](TUI-TRACE.md) | Trace ownership chains interactively |
| [TUI-SCAN.md](TUI-SCAN.md) | Kyverno policy scanning TUI |
| [TUI-SAVED-QUERIES.md](TUI-SAVED-QUERIES.md) | Save and reuse fleet queries |

### Journeys (Step-by-Step Guides)

| Document | Description |
|----------|-------------|
| [JOURNEY-FIRST-SETUP.md](JOURNEY-FIRST-SETUP.md) | Initial setup walkthrough |
| [JOURNEY-MAP.md](JOURNEY-MAP.md) | Using the map command |
| [JOURNEY-SCAN.md](JOURNEY-SCAN.md) | CCVE scanning walkthrough |
| [JOURNEY-IMPORT.md](JOURNEY-IMPORT.md) | Importing workloads |
| [JOURNEY-QUERY.md](JOURNEY-QUERY.md) | Query syntax and examples |

### Import Documentation

| Document | Description |
|----------|-------------|
| [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) | Import workloads overview |
| [IMPORT-FROM-LIVE.md](IMPORT-FROM-LIVE.md) | Import from running clusters |
| [IMPORT-FROM-SOURCES.md](IMPORT-FROM-SOURCES.md) | Import from Git sources |
| [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](IMPORT-GIT-REFERENCE-ARCHITECTURES.md) | Common GitOps patterns |

### Technical References

| Document | Description |
|----------|-------------|
| [cve/ccve/README.md](../cve/ccve/README.md) | CCVE database (1,700+ patterns) |
| [GSF-SCHEMA.md](GSF-SCHEMA.md) | GitOps State Format specification |
| [EXTENDING.md](EXTENDING.md) | Adding custom detectors |

### Examples & Test Kit

```bash
# See what's in the cluster
./test/atk/map

# Run CCVE scan
./test/atk/scan

# Verify ownership detection
./test/atk/verify

# Quick demo
./test/atk/demo quick
```

---

*For questions or issues: https://github.com/confighubai/confighub-agent/issues*
