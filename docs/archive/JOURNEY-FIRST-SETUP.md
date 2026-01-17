# Journey: First-Time Setup

**Time:** 5 minutes
**Goal:** Install tools, connect to ConfigHub, verify everything works

---

## Prerequisites

- Kubernetes cluster (any: kind, minikube, EKS, GKE, AKS)
- kubectl configured and working

Verify:
```bash
kubectl cluster-info
# Kubernetes control plane is running at https://...
```

---

## Step 1: Install cub CLI

```bash
# macOS
brew install confighub/tap/cub

# Linux
curl -sSL https://get.confighub.com | sh

# Verify
cub version
# cub version 0.x.x
```

---

## Step 2: Get confighub-agent

```bash
git clone https://github.com/confighubai/confighub-agent.git
cd confighub-agent

# Build
go build ./cmd/cub-agent

# Verify
./cub-agent version
# cub-agent version 0.x.x
```

---

## Step 3: Test Standalone Mode (No Account Needed)

See what's running in your cluster:

```bash
./cub-agent map
```

**Expected output:**

```
┌─ CLUSTER: kind-my-cluster ────────────────────────────────────┐
│                                                                │
│  ████████████████████░░░░  85% healthy (17/20 pods)           │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌─ RESOURCES (by owner) ─────────────────────────────────────────┐
│                                                                │
│  Flux          8 resources   ████████                          │
│  ArgoCD        5 resources   █████                             │
│  Native        7 resources   ███████                           │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

This works without a ConfigHub account. You can see:
- Cluster health
- Resource ownership (Flux, Argo CD, Helm, Native)
- Deployments, StatefulSets, etc.

---

## Step 4: Login to ConfigHub

```bash
cub auth login
```

This opens your browser. After login:

```bash
cub auth whoami
# Logged in as: you@example.com
# Organization: your-org
```

---

## Step 5: Verify Connection

```bash
# Check context
cub context get
```

**Expected output:**

```json
{
  "coordinate": {
    "user": "you@example.com",
    "organization": "your-org"
  },
  "metadata": {
    "organizationName": "Your Organization"
  }
}
```

---

## Step 6: Test Connected Mode

Run the import TUI to see your cluster connected to ConfigHub:

```bash
./test/atk/map-import
```

**Expected output (connected):**

```
┌─ CONFIGHUB ────────────────────────────────────────────────────┐
│                                                                │
│  ✓ Authenticated as: you@example.com                          │
│  ✓ Organization: your-org                                      │
│  ○ Units: 0 (none imported yet)                                │
│  ○ Worker: Not started                                         │
│                                                                │
└────────────────────────────────────────────────────────────────┘

Press [i] to import workloads
Press [w] to start worker
Press [q] to quit
```

---

## Step 7: Quick Import Test

Import a namespace to create your first Unit:

```bash
./cub-agent import -n default --dry-run
```

**Expected output:**

```
┌─ DISCOVERED ───────────────────────────────────────────────────┐
│                                                                │
│  default (3 workloads)                                         │
│    • my-app (Deployment)                                       │
│    • my-db (StatefulSet)                                       │
│    • my-worker (Deployment)                                    │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌─ WILL CREATE ──────────────────────────────────────────────────┐
│                                                                │
│  App Space: default-team                                       │
│                                                                │
│  • my-app                                                      │
│    labels: app=my-app, variant=default                         │
│                                                                │
└────────────────────────────────────────────────────────────────┘

Dry run: no changes made
Remove --dry-run to create these Units in ConfigHub
```

---

## You're Ready!

| What You Can Do | Command |
|-----------------|---------|
| See what's running | `./cub-agent map` |
| Scan for issues | `./test/atk/scan` |
| Import workloads | `./cub-agent import` |
| Trace ownership | Press `t` in map TUI |
| Query fleet | `./cub-agent map list -q "owner=Flux"` |

---

## Next Steps

| Journey | What You'll Learn |
|---------|-------------------|
| [**JOURNEY-IMPORT.md**](JOURNEY-IMPORT.md) | Import your cluster into ConfigHub |
| [**JOURNEY-MAP.md**](JOURNEY-MAP.md) | Navigate the map TUI |
| [**JOURNEY-SCAN.md**](JOURNEY-SCAN.md) | Find configuration issues |

---

## Troubleshooting

### "cub: command not found"

Check installation:
```bash
which cub
# Should show /usr/local/bin/cub or similar

# If using brew
brew info confighub/tap/cub
```

### "Not authenticated"

Login again:
```bash
cub auth login
cub context get  # Verify
```

### "No cluster access"

Check kubectl:
```bash
kubectl cluster-info
kubectl get nodes
```

### "Permission denied"

cub-agent needs read access. Check your kubeconfig:
```bash
kubectl auth can-i get deployments --all-namespaces
# yes
```

---

**Next Journey:** [JOURNEY-IMPORT.md](JOURNEY-IMPORT.md) — Import workloads into ConfigHub

---

## See Also

- [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) — Glossary of terms
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Detailed import guide
- [README.md](../README.md) — Full documentation index
