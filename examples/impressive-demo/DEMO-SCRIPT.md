# The Demo: Map. Merge. Scan.

**Duration:** 60 seconds
**Tagline:** Three commands. Complete fleet control.

---

## What Already Exists

| Component | Status | Location |
|-----------|--------|----------|
| `./map` | ✅ Ready | `test/atk/map` |
| `./scan` | ✅ Ready | `test/atk/scan` |
| CCVE-2025-0027 detection | ✅ Ready | Built into `test/atk/scan` |
| `drift merge` | ❌ Not built | Needs implementation |
| CCVE-2025-0027 fixture | ✅ Ready | `examples/impressive-demo/bad-configs/monitoring-bad.yaml` |
| Mixed ownership fixture | ✅ Ready | `test/atk/fixtures/mixed.yaml` |
| Native/orphan fixture | ✅ Ready | `test/atk/fixtures/native-basic.yaml` |
| Demo runner | ✅ Ready | `test/atk/demo` |

---

## Demo Cluster Setup

### Prerequisites
```bash
# Kind cluster
kind create cluster --name demo

# Install Flux (for ownership detection)
flux install

# Install Argo CD (optional, for mixed ownership)
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

### Apply Fixtures
```bash
cd /Users/alexis/Public/github-repos/confighub-agent

# 1. Mixed ownership (Flux + Argo + Native)
kubectl apply -f test/atk/fixtures/mixed.yaml

# 2. Orphan resources (Native, no owner)
kubectl apply -f test/atk/fixtures/native-basic.yaml

# 3. Grafana CCVE (the BIGBANK bug)
kubectl apply -f examples/impressive-demo/bad-configs/monitoring-bad.yaml

# 4. Create some "drift" by editing a deployment
kubectl set replicas deployment/nginx -n atk-native-basic --replicas=5
```

---

## The Three Scenes (60 seconds)

### Scene 1: Map (20 seconds)
**Question:** "Where's redis running across all our clusters?"

```bash
# Full fleet view
./test/atk/map

# Query (simulated - needs enhancement)
./test/atk/map workloads | grep redis
```

**Magic:** Instant answer. One command. All clusters, all owners.

**Screenshot moment:** The ownership bar showing Flux/Argo/Native distribution.

---

### Scene 2: Merge (20 seconds)
**Question:** "I kubectl edited prod last night. Now what?"

```bash
# See the drift
./test/atk/map workloads | grep -E "(Native|drift)"

# Merge it (THIS NEEDS TO BE BUILT)
# cub drift merge nginx --namespace atk-native-basic
# Output: "MR created: !1847 - Merge hotfix: nginx replicas 1→5"
```

**Magic:** One command. MR created. Audit trail preserved.

**Screenshot moment:** The MR being created automatically.

**NOTE:** `drift merge` is not implemented yet. For the demo, we can:
1. Mock the output
2. Show the `./map` detecting the drift
3. Explain what `merge` would do

---

### Scene 3: Scan (20 seconds)
**Question:** "Is this config safe?"

```bash
# Scan for CCVEs
./test/atk/scan
```

**Expected output:**
```
CONFIG CVE SCAN: kind-demo
════════════════════════════════════════════════════════════════════

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[CCVE-2025-0027] monitoring/grafana - Namespace whitespace in sidecar
  Impact: BIGBANK Capital Markets - 3-day outage (FluxCon 2025)
  Fix: Remove spaces from namespace list

════════════════════════════════════════════════════════════════════
Summary: 1 critical, 0 warning, 0 info
```

**Magic:** 10 seconds to find a bug that took BIGBANK 3 days.

**Screenshot moment:** The CCVE with real-world incident citation.

---

## What Needs Building

### 1. `drift merge` Command
Needs to:
1. Detect drift (compare live state to last-applied)
2. Generate a Git commit/MR with the live state
3. Update ConfigHub annotations

### 2. Enhanced Map Query
The map tool needs `--query` support:
```bash
./map --query "image contains redis"
./map --query "owner = Native"  # Find orphans
./map --drifted                 # Find drift
```

---

## Quick Demo (Using Demo Runner)

The demo runner provides the fastest path to a demo:

```bash
# Quick demo (~30 sec) - fastest path to WOW
./test/atk/demo quick

# CCVE-2025-0027 demo (~2 min) - the BIGBANK 4-hour outage story
./test/atk/demo ccve

# Narrative scenario (~3 min) - walk through the incident
./test/atk/demo scenario bigbank-incident

# List all available demos with timing
./test/atk/demo --list
```

For Brian's larger scale demos (312 units, 3 clusters), see:
https://github.com/confighub-kubecon-2025

---

## Recording Tips

1. **Use a clean terminal** — black background, large font
2. **Pre-type commands** — paste them for clean execution
3. **Pause after each output** — let it sink in
4. **No narration needed** — the commands speak for themselves

---

## The Ritual to Teach

After the demo, show the rituals:

```bash
# Monday morning
./map status

# Before every deploy
./map --pending

# When paged at 2am
./map --drifted
./scan
```

---

## Metrics

| Before | After |
|--------|-------|
| "Where is redis?" → 1 hour | 5 seconds |
| "Fix this hotfix" → 30 minutes | 10 seconds |
| "Is this safe?" → 4 hours | 10 seconds |
