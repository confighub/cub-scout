# ConfigHub Agent Examples

All examples, demos, and integration code in one place.

> **Maintainer note:** When updating this file, also update [docs/EXAMPLES-OVERVIEW.md](../docs/EXAMPLES-OVERVIEW.md).

## Status Legend

| Status | Meaning |
|--------|---------|
| **Working** | Tested code that runs on your cluster |
| **Mockup** | UI designs/mockups for discussion |
| **Proposal** | Architecture proposals, not yet implemented |

## Quick Start

```bash
# Clone (requires GitHub access)
git clone https://github.com/confighubai/confighub-agent.git
cd confighub-agent

# Try it on your cluster
./run.sh

# Or run a demo
./test/atk/demo quick
```

---

## Real-World Examples

Clone and deploy these repos to see the agent in action with real GitOps setups.

### Apptique Examples (NEW)

Multiple GitOps patterns using Google's Online Boutique app — **included in this repo**:

| Pattern | Tool | Shows |
|---------|------|-------|
| [apptique-examples/flux-monorepo](apptique-examples/flux-monorepo/) | **Flux** | Monorepo with Kustomize overlays |
| [apptique-examples/argo-applicationset](apptique-examples/argo-applicationset/) | **Argo CD** | ApplicationSet with directory generator |
| [apptique-examples/argo-app-of-apps](apptique-examples/argo-app-of-apps/) | **Argo CD** | Parent Application managing children |

```bash
# Deploy Flux pattern
kubectl apply -f examples/apptique-examples/flux-monorepo/infrastructure/gitrepository.yaml
kubectl apply -f examples/apptique-examples/flux-monorepo/clusters/dev/kustomization.yaml

# Deploy Argo ApplicationSet
kubectl apply -f examples/apptique-examples/argo-applicationset/bootstrap/applicationset.yaml

# Verify ownership detection
./test/atk/map workloads | grep apptique
```

### External Examples (GitHub)

| Example | Repo | Shows |
|---------|------|-------|
| **ArgoCD Setup** | [confighubai/examples-internal/argocd](https://github.com/confighubai/examples-internal/tree/main/argocd) | Argo CD Applications, failing deployers |
| **FluxCD Setup** | [confighubai/examples-internal/fluxcd](https://github.com/confighubai/examples-internal/tree/main/fluxcd) | Flux Kustomizations, HelmReleases |
| **Global App** | [confighub/examples/global-app](https://github.com/confighub/examples/tree/main/global-app) | ConfigHub multi-region deployment |
| **Helm Platform** | [confighub/examples/helm-platform-components](https://github.com/confighub/examples/tree/main/helm-platform-components) | Helm ownership detection |
| **VM Fleet** | [confighub/examples/vm-fleet](https://github.com/confighub/examples/tree/main/vm-fleet) | Non-Kubernetes fleet management |
| **Flux Bridge** | [confighubai/flux-bridge](https://github.com/confighubai/flux-bridge) | Flux-to-ConfigHub integration |

### Try an example

```bash
# Deploy an example to your cluster and see the map output
./test/atk/examples --capture jesper_argocd

# Or test that all examples are accessible
./test/atk/examples

# Filter by type
./test/atk/examples jesper    # Jesper's internal examples
./test/atk/examples public    # Public confighub/examples
```

Expected output for each example is in `test/fixtures/expected-output/examples/`.

---

## What's Here

| File/Folder | Type | What | Use When |
|-------------|------|------|----------|
| [apptique-examples/](apptique-examples/) | **Working** | Real GitOps patterns (Flux, Argo) | Learning GitOps ownership |
| [demos/](demos/) | **Test Fixtures** | YAML with GitOps labels + nginx:alpine | Learning ownership detection |
| [impressive-demo/](impressive-demo/) | **Test Fixtures** | Conference demo with CCVE scenarios | Presentations, videos |
| [scripts/](scripts/) | **Integration Code** | k9s, Slack, CI/CD scripts | Adding to your workflow |
| [integrations/](integrations/) | **Plugins** | ArgoCD extension, Flux operator | Building on the agent |
| [rm-demos-argocd/](rm-demos-argocd/) | **Concept Demo** | Rendered Manifest simulations | Sales presentations |
| [app-config-rtmsg/](app-config-rtmsg/) | **Concept Demo** | Non-K8s config management TUI | Understanding Hub/Space model |

> **Note:** For **real GitOps applications** you can deploy, see [Real-World Examples](#real-world-examples) below.
> The demos in this folder are test fixtures that demonstrate the agent's detection capabilities.

---

## Concept Demos (Future Features)

These demos show **future ConfigHub capabilities** that are not yet implemented. They're for presentations, sales, and understanding the vision — not for running against real clusters.

> **⚠️ Important:** Concept demos print simulated output or show TUI mockups. They do NOT connect to real clusters or ConfigHub. Use [Test Fixtures](#demos-test-fixtures) or [Real-World Examples](#real-world-examples) for actual functionality.

| Demo | Type | Status | Shows |
|------|------|--------|-------|
| [rm-demos-argocd/](rm-demos-argocd/) | **Simulation** | Future feature | Rendered Manifest pattern — fleet-wide queries, drift detection, bulk patching |
| [app-config-rtmsg/](app-config-rtmsg/) | **TUI Mockup** | Future feature | Non-K8s config management — DynamoDB/Consul style with Hub/Space model |

### Rendered Manifest Demos (rm-demos-argocd)

Simulation scripts showing what ConfigHub WILL do when Rendered Manifest features are implemented:

```bash
./examples/rm-demos-argocd/scenarios/monday-panic/demo.sh    # Find problem across 47 clusters
./examples/rm-demos-argocd/scenarios/2am-kubectl/demo.sh     # Catch and fix drift
./examples/rm-demos-argocd/scenarios/security-patch/demo.sh  # Patch 847 services in one command
```

**These print simulated output** — they don't connect to clusters. Use for storytelling: "I literally did this last week. It took me 2 hours."

### App Config Demo (app-config-rtmsg)

TUI mockup showing how ConfigHub can manage non-Kubernetes configuration (based on real architecture from a messaging platform):

```bash
./examples/app-config-rtmsg/demo.sh    # TUI mockup with terminal colors
```

**Key concepts demonstrated:**
- **Hub** — Catalog of config templates + constraints
- **Spaces** — Team boundaries + customer self-serve
- **Units** — Config entities with labels, inheritance
- **Queries** — Cross-cutting visibility ("all production configs")

**This is a mockup** — it shows what the UI will look like, not working code. See [app-config-rtmsg/README.md](app-config-rtmsg/README.md) for details.

---

## Demos (Test Fixtures)

> **Important:** Demos are **test fixtures**, not real applications. They create Kubernetes
> resources with the correct GitOps labels (Flux, Argo, Helm, ConfigHub) to demonstrate
> ownership detection, but they run `nginx:alpine` as a placeholder — not actual app code.

**What demos are for:**
- Learning how the agent detects ownership
- Demonstrating the map/scan tools
- Presentations and videos

**What demos are NOT:**
- Production-ready applications
- Examples of how to structure real GitOps repos

```bash
./test/atk/demo --list           # List all demos
./test/atk/demo quick            # 30-second demo
./test/atk/demo ccve             # CCVE-2025-0027 detection
./test/atk/demo healthy          # Enterprise healthy pattern
./test/atk/demo unhealthy        # Common GitOps problems
./test/atk/demo <name> --cleanup # Remove demo resources
```

| Demo | Time | Shows |
|------|------|-------|
| `quick` | ~30 sec | Ownership detection, map dashboard |
| `ccve` | ~2 min | CCVE-2025-0027 (BIGBANK Grafana bug) |
| `healthy` | ~2 min | IITS hub-and-spoke pattern |
| `unhealthy` | ~2 min | Suspended resources, broken deployers |
| `scenario clobber` | ~2 min | Platform updates vs app overlays |

See [demos/README.md](demos/README.md) for detailed walkthroughs.

### Converting Demos to Real Apps

To turn a demo fixture into a real application:

1. **Replace the placeholder image:**
   ```yaml
   # Demo uses:
   image: nginx:alpine

   # Replace with your real app:
   image: your-registry/your-app:v1.0.0
   ```

2. **Add your actual configuration:**
   ```yaml
   # Demo has minimal config for ownership labels
   # Add your real env vars, volumes, secrets, etc.
   env:
     - name: DATABASE_URL
       valueFrom:
         secretKeyRef:
           name: db-credentials
           key: url
   ```

3. **Move to a GitOps repo:** The demo YAML is applied directly with `kubectl`. For real GitOps:
   - **Flux:** Add to a Kustomization source directory
   - **Argo CD:** Create an Application pointing to your repo
   - **Helm:** Convert to a Helm chart with values files

4. **Keep the ownership labels** — these are what the agent uses:
   ```yaml
   # Flux ownership (keep these)
   labels:
     kustomize.toolkit.fluxcd.io/name: my-app
     kustomize.toolkit.fluxcd.io/namespace: flux-system

   # Or Argo ownership
   labels:
     app.kubernetes.io/instance: my-app
     argocd.argoproj.io/instance: my-app
   ```

**For real GitOps examples,** see the [Real-World Examples](#real-world-examples) section — these are actual repos you can clone and deploy.

---

## Scripts

Copy-paste scripts for common integrations.

| Script | What It Does |
|--------|--------------|
| [k9s-plugin.yaml](scripts/k9s-plugin.yaml) | Add map/scan commands to k9s |
| [slack-alerting.sh](scripts/slack-alerting.sh) | Alert on drift/CCVEs |
| [github-workflow.yaml](scripts/github-workflow.yaml) | CI/CD gate for CCVEs |
| [prometheus-metrics.sh](scripts/prometheus-metrics.sh) | Export metrics |
| [audit-images.sh](scripts/audit-images.sh) | Find all image versions |
| [find-orphans.sh](scripts/find-orphans.sh) | Find unmanaged resources |

See [scripts/README.md](scripts/README.md) for usage.

---

## Integrations

Third-party tool integrations with working code.

| Integration | Type | Status |
|-------------|------|--------|
| [argocd-extension/](integrations/argocd-extension/) | UI Extension | Working |
| [flux-operator/](integrations/flux-operator/) | Metrics Exporter | Working |
| [flux9s/](integrations/flux9s/) | TUI Enhancement | Proposal |

See [integrations/README.md](integrations/README.md) for architecture.

---

## Fleet Queries

Questions the agent answers in seconds. See [JOURNEY-QUERY.md](../docs/JOURNEY-QUERY.md) for complete examples.

```bash
# Interactive demo
./examples/demos/fleet-queries-demo.sh

# Live queries against cluster
./test/atk/demo query
```

### "What's running and who owns it?"

```bash
$ ./test/atk/map workloads

STATUS  NAMESPACE        NAME              OWNER      MANAGED-BY            IMAGE
────────────────────────────────────────────────────────────────────────────────────
✓     demo-payments    payment-api       ConfigHub  payment-api-prod      api:2.4.1
✓     atk-flux-basic   podinfo           Flux       podinfo               podinfo:6.5.0
✓     demo-orders      postgresql        Helm       orders-db             postgres:15
✓     argocd           argocd-server     Native     -                     argocd:v3.2.3
```

### "Which clusters are behind?"

```bash
$ ./test/atk/map   # With ConfigHub auth

ConfigHub Fleet View
  order-processor
  ├── variant: prod
  │   └── ✓ cluster-east @ rev 89
  │   └── ✓ cluster-west @ rev 89
  │   └── ⚠ cluster-eu @ rev 87    ← behind!
```

### "What config bugs exist?"

```bash
$ ./test/atk/scan

CONFIG CVE SCAN
════════════════════════════════════════════════════════════════════
CRITICAL (1)
[CCVE-2025-0027] demo-monitoring/grafana    ← BIGBANK 4-hour outage bug
════════════════════════════════════════════════════════════════════
```

### "What's broken right now?"

```bash
$ ./test/atk/map problems

✗ HelmRelease/redis-cache in flux-system: SourceNotReady
⏸ Kustomization/monitoring-stack in flux-system: suspended
✗ Deployment/order-processor in demo-orders: 0/2 ready
```

---

## JSON Output

All commands support `--json` for tooling:

```bash
$ ./test/atk/map --json | jq '.workloads[] | select(.owner == "ConfigHub")'

{
  "name": "payment-api",
  "namespace": "demo-payments",
  "owner": "ConfigHub",
  "confighub": {
    "unit": "payment-api-prod",
    "space": "payments-prod",
    "revision": "127"
  }
}
```

---

## See Also

| Doc | What's in it |
|-----|--------------|
| [docs/EXAMPLES-OVERVIEW.md](../docs/EXAMPLES-OVERVIEW.md) | Central examples overview |
| [docs/IMPORTING-WORKLOADS.md](../docs/IMPORTING-WORKLOADS.md) | Import workloads into ConfigHub |
| [README.md](../README.md) | Project overview |
| [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md) | How it works, GSF protocol |
| [docs/CCVE-GUIDE.md](../docs/CCVE-GUIDE.md) | Config CVE scanning |
| [docs/CLI-REFERENCE.md](../docs/CLI-REFERENCE.md) | Full CLI reference |
| [test/README.md](../test/README.md) | Testing documentation |
