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
| **4. Scale Demo** | [getting-started/scale-demo.md](getting-started/scale-demo.md) |

---

## How-To Guides

Task-based guides:

| Task | Guide |
|------|-------|
| Find orphan resources | [howto/find-orphans.md](howto/find-orphans.md) |
| Trace ownership chains | [howto/trace-ownership.md](howto/trace-ownership.md) |
| Query resources | [howto/query-resources.md](howto/query-resources.md) |
| Fleet queries | [howto/fleet-queries.md](howto/fleet-queries.md) |
| Tree hierarchies | [howto/tree-hierarchies.md](howto/tree-hierarchies.md) |
| Scan for CCVEs | [howto/scan-for-ccves.md](howto/scan-for-ccves.md) |
| Ownership detection | [howto/ownership-detection.md](howto/ownership-detection.md) |
| Import to ConfigHub | [howto/import-to-confighub.md](howto/import-to-confighub.md) |
| Scan for risks | [howto/scan-for-risks.md](howto/scan-for-risks.md) |
| Advanced queries | [howto/advanced-queries.md](howto/advanced-queries.md) |
| Extending cub-scout | [howto/extending.md](howto/extending.md) |

---

## Reference

Complete reference documentation:

| Topic | Reference |
|-------|-----------|
| **Commands** | [reference/commands.md](reference/commands.md) |
| Query syntax | [reference/query-syntax.md](reference/query-syntax.md) |
| GSF schema | [reference/gsf-schema.md](reference/gsf-schema.md) |
| TUI views | [reference/views.md](reference/views.md) |
| Keybindings | [reference/keybindings.md](reference/keybindings.md) |
| GitOps repo patterns | [reference/gitops-repo-structures.md](reference/gitops-repo-structures.md) |
| Hub/AppSpace examples | [reference/hub-appspace-examples.md](reference/hub-appspace-examples.md) |
| Map PRD | [reference/map-prd.md](reference/map-prd.md) |
| Command matrix | [reference/command-matrix.md](reference/command-matrix.md) |
| Glossary | [reference/glossary.md](reference/glossary.md) |
| Testing guide | [reference/testing.md](reference/testing.md) |
| CLI guide | [../CLI-GUIDE.md](../CLI-GUIDE.md) |

---

## Concepts

Understand the "why":

| Concept | Explanation |
|---------|-------------|
| GitOps Overview | [concepts/gitops-overview.md](concepts/gitops-overview.md) |
| The Clobbering Problem | [concepts/clobbering-problem.md](concepts/clobbering-problem.md) |
| Architecture | [concepts/architecture.md](concepts/architecture.md) |
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

---

## Examples

| Example | What you'll learn |
|---------|-------------------|
| [platform-example](../examples/platform-example/) | Full GitOps environment with base/overlays pattern |
| [flux-boutique](../examples/flux-boutique/) | Simple Flux demo |
| [orphans](../examples/orphans/) | Detecting orphan resources |
| [impressive-demo](../examples/impressive-demo/) | Comprehensive demo with CCVE scanning |

See [EXAMPLES-OVERVIEW.md](EXAMPLES-OVERVIEW.md) for all examples.

---

## Outcomes

Real-world use cases:

| Outcome | Description |
|---------|-------------|
| [Enterprise Case Studies](outcomes/enterprise-case-studies.md) | IITS research findings |

---

## Internal Docs

| File/Folder | Purpose |
|-------------|---------|
| [roadmap.md](roadmap.md) | Future features (P2-P3) |
| `archive/` | Historical documentation |
