# cub-scout Documentation

**Demystify GitOps. See what's really happening in your cluster.**

---

## Getting Started

New to cub-scout? Start here:

| Step | Guide |
|------|-------|
| **1. Install** | [getting-started/install.md](getting-started/install.md) |
| **2. First Map** | [getting-started/first-map.md](getting-started/first-map.md) |
| **3. Understand GitOps** | [concepts/gitops-overview.md](concepts/gitops-overview.md) |
| **4. App + AI GitOps (Plain English)** | [reference/app-and-ai-gitops-plain-english.md](reference/app-and-ai-gitops-plain-english.md) |
| **5. Scale Demo** | [getting-started/scale-demo.md](getting-started/scale-demo.md) |

---

## Common Entry Points

| I need... | Start here |
|-----------|------------|
| JSON contract docs | [reference/json-contracts.md](reference/json-contracts.md) |
| JSON vs ASCII semantics | [semantic-contract.md](semantic-contract.md) |
| Full command/flag reference | [reference/commands.md](reference/commands.md) |
| End-to-end CLI guide | [../CLI-GUIDE.md](../CLI-GUIDE.md) |

---

## How-To Guides

### v1.0 Standalone

| Task | Guide |
|------|-------|
| Find orphan resources | [howto/find-orphans.md](howto/find-orphans.md) |
| Trace ownership chains | [howto/trace-ownership.md](howto/trace-ownership.md) |
| Query resources | [howto/query-resources.md](howto/query-resources.md) |
| Ownership detection | [howto/ownership-detection.md](howto/ownership-detection.md) |
| Scan for risk issues | [howto/scan-for-risks.md](howto/scan-for-risks.md) |
| Tree hierarchies | [howto/tree-hierarchies.md](howto/tree-hierarchies.md) |
| Advanced queries | [howto/advanced-queries.md](howto/advanced-queries.md) |
| Crossplane walkthrough | [howto/crossplane-walkthrough.md](howto/crossplane-walkthrough.md) |
| Run demos | [howto/running-demos.md](howto/running-demos.md) |
| Extending cub-scout | [howto/extending.md](howto/extending.md) |

### 1.x Connected *(requires ConfigHub)*

| Task | Guide |
|------|-------|
| Import to ConfigHub | [howto/import-to-confighub.md](howto/import-to-confighub.md) |
| Import from live cluster | [howto/import-from-live.md](howto/import-from-live.md) |
| Migration playbook | [howto/migration-playbook.md](howto/migration-playbook.md) |
| Fleet queries | [howto/fleet-queries.md](howto/fleet-queries.md) |

---

## Reference

### v1.0 Standalone

| Topic | Reference |
|-------|-----------|
| **JSON Contracts (Start Here)** | [reference/json-contracts.md](reference/json-contracts.md) |
| **Semantic Contract (JSON vs ASCII)** | [semantic-contract.md](semantic-contract.md) |
| **Commands** | [reference/commands.md](reference/commands.md) |
| **Debug Bundles** | [debug-bundle.md](debug-bundle.md) |
| **Drift Detection** | [drift.md](drift.md) |
| **Ownership & Precedence** | [reference/ownership-precedence.md](reference/ownership-precedence.md) |
| **Health & Failure States** | [reference/health-failure-states.md](reference/health-failure-states.md) |
| **CLI Contract** | [reference/cli-contract.md](reference/cli-contract.md) |
| Query syntax | [reference/query-syntax.md](reference/query-syntax.md) |
| Query library | [reference/query-library.md](reference/query-library.md) |
| GSF schema | [reference/gsf-schema.md](reference/gsf-schema.md) |
| TUI views | [reference/views.md](reference/views.md) |
| Keybindings | [reference/keybindings.md](reference/keybindings.md) |
| GitOps patterns | [reference/gitops-patterns.md](reference/gitops-patterns.md) |
| GitOps repo structures | [reference/gitops-repo-structures.md](reference/gitops-repo-structures.md) |
| Map PRD | [reference/map-prd.md](reference/map-prd.md) |
| Command matrix | [reference/command-matrix.md](reference/command-matrix.md) |
| Glossary | [reference/glossary.md](reference/glossary.md) |
| Testing guide | [reference/testing.md](reference/testing.md) |
| CLI guide | [../CLI-GUIDE.md](../CLI-GUIDE.md) |

### 1.x Connected *(requires ConfigHub)*

| Topic | Reference |
|-------|-----------|
| Import docs crosswalk | [reference/import-docs-crosswalk.md](reference/import-docs-crosswalk.md) |
| Connected tiers + views guide | [reference/connected-tiers-and-views-product-guide.md](reference/connected-tiers-and-views-product-guide.md) |
| App model examples | [reference/hub-appspace-examples.md](reference/hub-appspace-examples.md) |
| Rendered Manifest + Argo guide | [reference/rendered-manifest-and-argo-product-guide.md](reference/rendered-manifest-and-argo-product-guide.md) |
| Stored in Git vs ConfigHub | [reference/stored-in-git-vs-confighub.md](reference/stored-in-git-vs-confighub.md) |
| Resolver pattern | [reference/resolver-pattern.md](reference/resolver-pattern.md) |
| GitOps checkpoint PRD (proposal) | [reference/gitops-checkpoint-prd.md](reference/gitops-checkpoint-prd.md) |
| GitOps checkpoint schemas | [reference/gitops-checkpoint-schemas.md](reference/gitops-checkpoint-schemas.md) |

---

## Concepts

Understand the "why":

| Concept | Explanation |
|---------|-------------|
| Concepts index (start here) | [concepts/README.md](concepts/README.md) |
| GitOps Overview | [concepts/gitops-overview.md](concepts/gitops-overview.md) |
| The Clobbering Problem | [concepts/clobbering-problem.md](concepts/clobbering-problem.md) |
| Architecture | [concepts/architecture.md](concepts/architecture.md) |
| Live Cluster Inference | [concepts/live-cluster-inference.md](concepts/live-cluster-inference.md) |
| TUI vs GUI | [concepts/tui-vs-gui.md](concepts/tui-vs-gui.md) |
| Alternatives | [concepts/alternatives.md](concepts/alternatives.md) |

---

## Visual Guides

See [diagrams/](diagrams/) for visual explanations using [D2](https://d2lang.com):

| Diagram | What it shows |
|---------|---------------|
| [Flux Architecture](diagrams/flux-architecture.svg) | How Flux GitOps works |
| [Ownership Detection](diagrams/ownership-detection.svg) | How ownership is detected |
| [Ownership Trace](diagrams/ownership-trace.svg) | What cub-scout reveals |
| [Kustomize Overlays](diagrams/kustomize-overlays.svg) | Multi-environment pattern |
| [Clobbering Problem](diagrams/clobbering-problem.svg) | Hidden layer dangers |
| [Upgrade Tracing](diagrams/upgrade-tracing.svg) | Finding what changed |

> **Note:** "D2 pattern" in `tree patterns` refers to a GitOps repository pattern (Flux CD "Control Plane" style), not the D2 diagram language.

Diagram source/render index:
- [diagrams/README.md](diagrams/README.md)

Terminal screenshots and GIF capture scripts:
- [images/README.md](images/README.md)

---

## Examples

| Example | What you'll learn |
|---------|-------------------|
| [platform-example](../examples/platform-example/) | Full GitOps environment with base/overlays pattern |
| [flux-boutique](../examples/flux-boutique/) | Simple Flux demo |
| [orphans](../examples/orphans/) | Detecting orphan resources |
| [impressive-demo](../examples/impressive-demo/) | Comprehensive demo with risk scanning |

See [EXAMPLES-OVERVIEW.md](EXAMPLES-OVERVIEW.md) for all examples.

---

## Outcomes

Real-world use cases:

| Outcome | Description |
|---------|-------------|
| [Enterprise Case Studies](outcomes/enterprise-case-studies.md) | Real-world enterprise GitOps challenges |

---

## Internal Docs

| File/Folder | Purpose |
|-------------|---------|
| [roadmap.md](roadmap.md) | Canonical roadmap and current execution scope |
| [releases/v1.0.0.md](releases/v1.0.0.md) | Latest release notes (v1.0.0) |
| [releases/v0.20.0-slice-plan.md](releases/v0.20.0-slice-plan.md) | Historical implementation plan for shipped v0.20.0 slice |
| [roadmap-rendered-manifest-and-argo.md](roadmap-rendered-manifest-and-argo.md) | Backlog split from RM/App-of-Apps planning docs |
| [roadmap-connected-views-and-launch.md](roadmap-connected-views-and-launch.md) | Backlog split from view-tier/mockup/launch planning docs |
| `archive/` | Historical documentation |
