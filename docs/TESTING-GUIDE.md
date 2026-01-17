# ConfigHub Agent Testing Guide

A step-by-step guide to testing the ConfigHub Agent locally.

## Prerequisites

```bash
# macOS
brew install kind kubectl fluxcd/tap/flux

# Verify
kind --version
kubectl version --client
flux --version
```

## Step 1: Set Up Test Cluster

Creates a Kind cluster with Flux CD and Argo CD pre-installed.

```bash
./test/atk/setup-cluster
```

**Expected output:**

```
Creating Kind cluster 'atk'...
Cluster 'atk' already exists
Switched to context "kind-atk".
Installing Flux...
Flux already installed
Installing Argo CD...
Argo CD already installed

=== Cluster Status ===
Context: kind-atk

Namespaces:
argocd                  Active   47h
flux-system             Active   47h

Flux controllers:
NAME                      READY
helm-controller           1
kustomize-controller      1
notification-controller   1
source-controller         1
Argo CD controllers:
NAME                               READY
argocd-applicationset-controller   1
argocd-dex-server                  1
argocd-notifications-controller    1
argocd-redis                       1
argocd-repo-server                 1
argocd-server                      1

✓ Cluster ready for ATK tests
Run: ./test/atk/verify --your-cluster
```

## Step 2: Build the Agent

```bash
go build ./cmd/cub-scout
```

**Expected output:** (none on success)

Verify with:

```bash
./cub-scout --help
```

**Expected output:**

```
ConfigHub Agent - Kubernetes resource visibility and ownership detection

The cub-scout observes Kubernetes clusters and detects resource ownership.
It provides commands for:

  - Mapping resources and their ownership (Flux, Argo CD, Helm, ConfigHub, Native)
  - Scanning for CCVEs (configuration anti-patterns)
  - Tracing ownership chains
  - Importing resources into ConfigHub

Interacts with ConfigHub via the cub CLI (like kubectl, flux, argocd).

Environment Variables:
  CLUSTER_NAME            Name for this cluster (default: default)
  KUBECONFIG              Path to kubeconfig file (default: ~/.kube/config)

Usage:
  cub-scout [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  import      Import resources into ConfigHub
  map         Interactive map of resources and ownership
  scan        Scan for CCVEs
  trace       Trace resource ownership chain
  version     Print version information

Flags:
  -h, --help   help for cub-scout

Use "cub-scout [command] --help" for more information about a command.
```

## Step 3: Run Ownership Detection Tests

Tests that the agent correctly identifies who manages each resource (Flux, Argo CD, ConfigHub, Helm, or native K8s).

```bash
./test/atk/verify
```

**Expected output:**

```
=== Testing: argo-basic ===
Applying fixture...
namespace/atk-argo-basic created
application.argoproj.io/guestbook created
Waiting for reconciliation...
Detecting resources in atk-argo-basic...
Found 5 resources
Detected:
  ArgoCD  deployment/guestbook-ui             app=
  ArgoCD  service/guestbook-ui                app=
  Native  configmap/kube-root-ca.crt
  Native  pod/guestbook-ui-84774bdc6f-mpwz5
  ArgoCD  replicaset/guestbook-ui-84774bdc6f  app=

Cleaning up...
namespace "atk-argo-basic" deleted
application.argoproj.io "guestbook" deleted
✓ argo-basic (5 resources detected)

=== Testing: confighub-basic ===
Applying fixture...
namespace/atk-confighub-basic created
deployment.apps/backend created
service/backend created
configmap/backend-config created
Waiting for reconciliation...
Detecting resources in atk-confighub-basic...
Found 6 resources
Detected:
  ConfigHub  deployment/backend             unit=backend rev=42
  ConfigHub  service/backend                unit=backend rev=42
  ConfigHub  configmap/backend-config       unit=backend rev=42
  Native     configmap/kube-root-ca.crt
  Native     pod/backend-6ddd6cbbcb-2hrkp
  ConfigHub  replicaset/backend-6ddd6cbbcb  unit=backend rev=42

Cleaning up...
namespace "atk-confighub-basic" deleted
✓ confighub-basic (6 resources detected)

=== Testing: confighub-variant ===
Applying fixture...
namespace/atk-confighub-variant created
deployment.apps/payment-service-dev created
service/payment-service-dev created
configmap/payment-service-dev-config created
Waiting for reconciliation...
Detecting resources in atk-confighub-variant...
Found 6 resources
Detected:
  ConfigHub  deployment/payment-service-dev             unit=payment-service-dev rev=42
  ConfigHub  service/payment-service-dev                unit=payment-service-dev rev=42
  Native     configmap/kube-root-ca.crt
  ConfigHub  configmap/payment-service-dev-config       unit=payment-service-dev rev=42
  Native     pod/payment-service-dev-866f96fd88-tvpkh
  ConfigHub  replicaset/payment-service-dev-866f96fd88  unit=payment-service-dev rev=42

Cleaning up...
namespace "atk-confighub-variant" deleted
✓ confighub-variant (6 resources detected)

=== Testing: flux-basic ===
Applying fixture...
namespace/atk-flux-basic created
gitrepository.source.toolkit.fluxcd.io/podinfo created
kustomization.kustomize.toolkit.fluxcd.io/podinfo created
Waiting for reconciliation...
Detecting resources in atk-flux-basic...
Found 8 resources
Detected:
  Flux    deployment/podinfo             kustomization=podinfo namespace=atk-flux-basic
  Flux    service/podinfo                kustomization=podinfo namespace=atk-flux-basic
  Native  configmap/kube-root-ca.crt
  Native  pod/podinfo-69c97645d7-n97rk
  Native  pod/podinfo-69c97645d7-tfndg
  Native  replicaset/podinfo-69c97645d7
  Flux    gitrepository/podinfo          url=https://github.com/stefanprodan/podinfo
  Flux    kustomization/podinfo          sourceRef=podinfo

Cleaning up...
namespace "atk-flux-basic" deleted
✓ flux-basic (8 resources detected)

=== Testing: flux-helm ===
Applying fixture...
namespace/atk-flux-helm created
helmrepository.source.toolkit.fluxcd.io/podinfo created
helmrelease.helm.toolkit.fluxcd.io/podinfo created
Waiting for reconciliation...
Detecting resources in atk-flux-helm...
Found 7 resources
Detected:
  Flux    deployment/podinfo            helmRelease=podinfo namespace=atk-flux-helm
  Flux    service/podinfo               helmRelease=podinfo namespace=atk-flux-helm
  Native  configmap/kube-root-ca.crt
  Native  pod/podinfo-8bf94758f-kjr7c
  Native  replicaset/podinfo-8bf94758f
  Flux    helmrepository/podinfo        url=https://stefanprodan.github.io/podinfo
  Flux    helmrelease/podinfo           chart=podinfo

Cleaning up...
namespace "atk-flux-helm" deleted
✓ flux-helm (7 resources detected)

=== Testing: native-basic ===
Applying fixture...
namespace/atk-native-basic created
deployment.apps/nginx created
service/nginx created
configmap/nginx-config created
Waiting for reconciliation...
Detecting resources in atk-native-basic...
Found 6 resources
Detected:
  Native  deployment/nginx
  Native  service/nginx
  Native  configmap/kube-root-ca.crt
  Native  configmap/nginx-config
  Native  pod/nginx-77bf8679f9-qvhh8
  Native  replicaset/nginx-77bf8679f9

Cleaning up...
namespace "atk-native-basic" deleted
✓ native-basic (6 resources detected)

================================
Results: 6 passed, 0 failed
```

## Step 4: Try the Map Dashboard

First, apply some test fixtures so there's something to see:

```bash
kubectl apply -f test/atk/fixtures/flux-basic.yaml
kubectl apply -f test/atk/fixtures/confighub-basic.yaml
kubectl wait --for=condition=Ready pods -l app=podinfo -n atk-flux-basic --timeout=60s
```

Then run the map:

```bash
cub-scout map
```

**Expected output:**

```
  ✓ ALL HEALTHY   atk

  Deployers  1/1 ✓
  Workloads  12/12 ✓

  PIPELINES
  ────────────────────────────────────────────────
  ✓ stefanprodan/podinfo@6.5.0  →  podinfo  →  3 resources

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(1) ConfigHub(1) Native(10)
  ██░░░░░░░░░░

  ConfigHub Hierarchy:
  Org → Space → Unit (with Resources, Targets, Workers)

  Cluster Resources with ConfigHub Labels:
  demo-prod / backend @ rev 42  [atk-confighub-basic/backend]
```

> **Note:** Use `cub-scout map --mode=hub` for experimental Hub → App Space → Application → Variant hierarchy.

### Map Subcommands

#### Status (one-liner)

```bash
cub-scout map status
```

**Expected output:**

```
✓ atk: 1 deployers, 12 workloads — all healthy
```

#### Workloads by Owner

```bash
cub-scout map workloads
```

**Expected output:**

```
STATUS  NAMESPACE                NAME                      OWNER       MANAGED-BY           IMAGE
────────────────────────────────────────────────────────────────────────────────────────────────────
✓       atk-confighub-basic     backend                   ConfigHub   backend             nginx:alpine
✓       atk-flux-basic          podinfo                   Flux        podinfo             podinfo:6.5.0
✓       argocd                  argocd-applicationset-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-notifications-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-repo-server        Native      -                   argocd:v3.2.3
✓       argocd                  argocd-server             Native      -                   argocd:v3.2.3
✓       argocd                  argocd-dex-server         Native      -                   dex:v2.43.0
✓       flux-system             helm-controller           Native      -                   helm-controller:v1.3.0
✓       flux-system             kustomize-controller      Native      -                   kustomize-controller:v1.6.1
✓       flux-system             notification-controller   Native      -                   notification-controller:v1.6.0
✓       argocd                  argocd-redis              Native      -                   redis:8.2.2-alpine
✓       flux-system             source-controller         Native      -                   source-controller:v1.6.2
```

#### Pipelines

```bash
cub-scout map pipelines
```

**Expected output:**

```
SOURCE                                      DEPLOYER                 TARGET
────────────────────────────────────────────────────────────────────────────────
✓ stefanprodan/podinfo@6.5.0              → podinfo              → 3 resources
```

#### Deployers

```bash
cub-scout map deployers
```

**Expected output:**

```
STATUS  KIND            NAME                      NAMESPACE            REVISION   RESOURCES
─────────────────────────────────────────────────────────────────────────────────────────────
✓       Kustomization   podinfo                   atk-flux-basic       abc1234    3
```

#### Sources

```bash
cub-scout map sources
```

**Expected output:**

```
STATUS  TYPE           URL                                       REF          REVISION
────────────────────────────────────────────────────────────────────────────────────────────
✓       Git           stefanprodan/podinfo                       6.5.0        abc1234
```

#### JSON Output

```bash
cub-scout map --json
```

**Expected output:**

```json
{
  "cluster": "kind-atk",
  "scannedAt": "2025-12-31T09:10:00Z",
  "gitops": [
    {
      "kind": "Kustomization",
      "name": "podinfo",
      "namespace": "atk-flux-basic",
      "owner": "Flux",
      "ready": true,
      "suspended": false,
      "revision": "6.5.0@sha1:abc1234",
      "shortRevision": "abc1234",
      "inventoryCount": 3
    },
    {
      "kind": "GitRepository",
      "name": "podinfo",
      "namespace": "atk-flux-basic",
      "owner": "Flux",
      "url": "https://github.com/stefanprodan/podinfo",
      "shortUrl": "stefanprodan/podinfo",
      "ref": "6.5.0",
      "ready": true
    }
  ],
  "workloads": [
    {
      "name": "podinfo",
      "namespace": "atk-flux-basic",
      "owner": "Flux",
      "ownerRef": "podinfo",
      "ready": true,
      "desired": 2,
      "available": 2,
      "image": "podinfo:6.5.0"
    },
    {
      "name": "backend",
      "namespace": "atk-confighub-basic",
      "owner": "ConfigHub",
      "ownerRef": "backend",
      "confighub": {
        "unit": "backend",
        "space": "demo-prod",
        "spaceId": "550e8400-e29b-41d4-a716-446655440000",
        "revision": "42"
      },
      "ready": true,
      "desired": 1,
      "available": 1,
      "image": "nginx:alpine"
    }
  ]
}
```

## Step 5: Scan for Config CVEs

```bash
cub-scout scan
```

**Expected output (healthy cluster):**

```
CONFIG CVE SCAN: kind-atk
════════════════════════════════════════════════════════════════════

✓ No Config CVEs detected
```

### List Available CCVEs

```bash
cub-scout scan --list
```

**Expected output:**

```
Config CVE Catalog:

ID                 CAT      Name                                       Severity
--                 ---      ----                                       --------
CCVE-2025-0001     SOURCE   GitRepository not ready                    critical
CCVE-2025-0002     RENDER   Kustomization build failed                 critical
CCVE-2025-0003     SOURCE   HelmRelease chart not ready                critical
CCVE-2025-0004     APPLY    Application sync failed                    critical
CCVE-2025-0005     DRIFT    Application out of sync                    warning
...
CCVE-2025-0027     CONFIG   Grafana sidecar namespace whitespace err   critical
CCVE-2025-0028     DEPEND   IngressRoute service not found             critical
...
```

**Categories:**
- **SOURCE** — Git/Helm repository issues
- **RENDER** — Kustomization/HelmRelease build failures
- **APPLY** — Sync/deploy failures
- **DRIFT** — Live state differs from desired
- **CONFIG** — Configuration anti-patterns (like CCVE-2025-0027)
- **DEPEND** — Missing dependencies (services, secrets, issuers)
- **STATE** — Health/status issues
- **ORPHAN** — Unmanaged resources

### JSON Output

```bash
cub-scout scan --json
```

**Expected output:**

```json
{
  "cluster": "kind-atk",
  "scannedAt": "2025-12-31T09:11:39Z",
  "summary": {
    "critical": 0,
    "warning": 0,
    "info": 0
  },
  "findings": []
}
```

### Example with Problems

If there were issues, the scan would show:

```
CONFIG CVE SCAN: prod-east
════════════════════════════════════════════════════════════════════

CRITICAL (2)
────────────────────────────────────────────────────────────────────
[CCVE-FLUX-001] monitoring/prometheus-stack
[CCVE-ARGO-001] argocd/payments-api

WARNING (3)
────────────────────────────────────────────────────────────────────
[CCVE-ARGO-003] argocd/frontend
[CCVE-CH-001] production/Deployment/orders-api
[CCVE-CH-005] production/Deployment/users-service

INFO (1)
────────────────────────────────────────────────────────────────────
[CCVE-FLUX-005] staging/feature-flag-service

════════════════════════════════════════════════════════════════════
Summary: 2 critical, 3 warning, 1 info

⚠ Run './scan <CCVE-ID>' for remediation steps
```

## Step 6: Try the Demos

Interactive demos with narrative walkthroughs:

```bash
# DEPRECATED: ./test/atk/demo --list
```

**Expected output:**

```
Available Demos

  NAME                 TIME         DESCRIPTION
  ────────────────────────────────────────────────────────────────
  quick                ~30 sec      Fastest path to WOW (--no-pods mode)
  ccve                 ~2 min       CCVE-2025-0027: The BIGBANK Grafana bug
  healthy              ~2 min       Enterprise healthy (IITS hub-and-spoke)
  unhealthy            ~2 min       Enterprise unhealthy (common problems)

Scenarios (Narrative Demos)

  scenario bigbank-incident ~3 min       Walk through the BIGBANK 4-hour outage
  scenario orphan-hunt ~2 min       Find and fix orphan resources
  scenario monday-morning ~1 min       Weekly health check ritual
```

### Quick Demo (~30 sec)

Fastest path to see the Map in action:

```bash
# DEPRECATED: ./test/atk/demo quick
```

### CCVE-2025-0027 Demo (~2 min)

The headline story — this exact bug caused a 4-hour outage at BIGBANK:

```bash
# DEPRECATED: ./test/atk/demo ccve
```

### Narrative Scenarios

Walk through real incidents with storytelling:

```bash
# DEPRECATED: ./test/atk/demo scenario bigbank-incident   # The BIGBANK 4-hour outage story
# DEPRECATED: ./test/atk/demo scenario orphan-hunt    # "What's this mystery-app?"
# DEPRECATED: ./test/atk/demo scenario monday-morning # Weekly health check ritual
# DEPRECATED: ./test/atk/demo scenario clobber        # Platform vs app config protection
```

### Other Demos

```bash
# DEPRECATED: ./test/atk/demo query                   # Query language syntax
# DEPRECATED: ./test/atk/demo connected               # ConfigHub connected mode
```

### TUI Demo Scripts

Interactive demo scripts in `examples/demos/`:

```bash
examples/demos/kyverno-scan-demo.sh     # KPOL database (460 patterns)
examples/demos/tui-trace-demo.sh        # Resource tracing
examples/demos/tui-queries-demo.sh      # TUI query interface
examples/demos/tui-import-demo.sh       # Import wizard
examples/demos/fleet-queries-demo.sh    # Fleet queries
```

### Cleanup Demos

```bash
# DEPRECATED: ./test/atk/demo quick --cleanup
# DEPRECATED: ./test/atk/demo ccve --cleanup
```

## Want More?

Connect to ConfigHub for fleet-wide capabilities:

| Feature | Standalone | Connected |
|---------|------------|-----------|
| Ownership detection | ✓ | ✓ |
| Drift detection | ✓ | ✓ |
| CCVE scanning | ✓ | ✓ |
| Fleet-wide queries | — | ✓ |
| Cross-cluster map | — | ✓ |
| Drift merge | — | ✓ |
| ConfigHub UI | — | ✓ |

See: https://confighub.com/docs/getting-started

For larger scale demos (312 units, 3 clusters), see Brian's KubeCon 2025 demo:
https://github.com/confighub-kubecon-2025

## Full Test Suite

Run all tests (3 phases + connected mode):

```bash
./test/run-all.sh --connected
```

**What it tests:**
- Phase 1: Preflight, build, unit tests, integration tests, ATK verify/map/scan
- Phase 2: All demos (quick, ccve, healthy, unhealthy)
- Phase 3: Examples validation (integrations, configs)
- Connected mode: Worker status, targets, ConfigHub API

**Expected output:**
```
Phase 1 (Standard):  8 passed
Phase 2 (Demos):     4 passed
Phase 3 (Examples):  5 passed

All tests passed
```

**Test logs:** `docs/planning/sessions/test-runs/test-run-YYYY-MM-DD_HH-MM-SS.log`

## Cleanup

Remove test fixtures:

```bash
kubectl delete -f test/atk/fixtures/flux-basic.yaml
kubectl delete -f test/atk/fixtures/confighub-basic.yaml
```

Tear down the cluster entirely:

```bash
./test/atk/teardown-cluster
```

Or keep the cluster but remove ATK namespaces:

```bash
./test/atk/teardown-cluster --keep
```

## Testing Principles

### No Untested Code

Every feature must have tests. In Jan 2026, we deleted ~500 lines of dead code that was never tested:
- An "agent daemon" that would sync to a non-existent HTTP API
- Environment variables (`CONFIGHUB_AGENT_TOKEN`) that don't exist in the real system
- API endpoints (`app.confighub.com/api/...`) that were never built

**Lesson:** Code without tests is invisible dead code. If you can't test it, don't write it.

### ConfigHub Integration

The only integration with ConfigHub is via the `cub` CLI. Tests that need ConfigHub should:
1. Use `cub auth status` to check authentication
2. Shell to `cub` commands (e.g., `exec.Command("cub", "unit", "list", "--json")`)
3. Never make direct HTTP calls to ConfigHub

## Quick Reference

| Command | Description |
|---------|-------------|
| `./test/atk/setup-cluster` | Create Kind + Flux + Argo CD |
| `./test/atk/verify` | Run all ownership tests |
| `./test/atk/verify flux-basic` | Run single test |
| `./test/atk/verify --list` | List available fixtures |
| `cub-scout map` | Full dashboard |
| `cub-scout map status` | One-line health check |
| `cub-scout map workloads` | List workloads by owner |
| `cub-scout map pipelines` | List delivery pipelines |
| `cub-scout map confighub` | ConfigHub hierarchy (requires cub auth) |
| `cub-scout map --json` | JSON output |
| `cub-scout map --mode=hub` | Experimental hub hierarchy mode |
| `cub-scout scan` | Scan for CCVEs |
| `cub-scout scan --list` | List all CCVEs |
| `cub-scout scan --json` | JSON output |
| `cub-scout scan` | Kyverno policy scan |
| `cub-scout scan --list` | List KPOL policies |
| `cub-scout scan -n <ns>` | Scan specific namespace |
| `./test/atk/teardown-cluster` | Delete cluster |

### Hierarchy Display Modes

| Mode | Flag | Hierarchy |
|------|------|-----------|
| **Standard** (default) | `--mode=standard` | Org → Space → Unit |
| **Hub** (experimental) | `--mode=hub` | Hub → App Space → Application → Variant |
