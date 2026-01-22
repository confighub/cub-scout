# cub-scout Documentation

**Demystify GitOps. See what's really happening in your cluster.**

---

## Getting Started

New to cub-scout? Start here:

1. **[Install](../README.md#installation)** — Get cub-scout running
2. **[First Map](getting-started/first-map.md)** — See your cluster in 5 minutes

---

## How-To Guides

Task-based guides:

| Task | Guide |
|------|-------|
| Find orphan resources | [howto/find-orphans.md](howto/find-orphans.md) |
| Trace ownership chains | [howto/trace-ownership.md](howto/trace-ownership.md) |
| Query resources | [howto/query-resources.md](howto/query-resources.md) |
| Scan for risks | [SCAN-GUIDE.md](SCAN-GUIDE.md) |

---

## Reference

Complete reference:

| Topic | Reference |
|-------|-----------|
| Query syntax | [reference/query-syntax.md](reference/query-syntax.md) |
| CLI commands | [CLI-GUIDE.md](../CLI-GUIDE.md) |
| GSF schema | [GSF-SCHEMA.md](GSF-SCHEMA.md) |
| Ownership labels | [ARCHITECTURE.md](ARCHITECTURE.md) |

---

## Concepts

Understand the "why":

| Concept | Explanation |
|---------|-------------|
| The Clobbering Problem | [concepts/clobbering-problem.md](concepts/clobbering-problem.md) |
| Ownership Detection | [ARCHITECTURE.md](ARCHITECTURE.md) |
| GitOps Overview | [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) |

---

## Visual Guides

See the [diagrams/](diagrams/) for visual explanations. These use [D2](https://d2lang.com), a modern diagram scripting language:

| Diagram | What it shows |
|---------|---------------|
| [Flux Architecture](diagrams/flux-architecture.d2) | How Flux GitOps works |
| [Ownership Trace](diagrams/ownership-trace.d2) | What cub-scout reveals |
| [Kustomize Overlays](diagrams/kustomize-overlays.d2) | Multi-environment pattern |
| [Clobbering Problem](diagrams/clobbering-problem.d2) | Hidden layer dangers |
| [Upgrade Tracing](diagrams/upgrade-tracing.d2) | Finding what changed |

> **Note:** The "D2" pattern mentioned in `tree patterns` and `tree suggest` refers to a GitOps repository pattern (Flux CD "Control Plane" style with clusters/infrastructure/apps structure), not the D2 diagram language.

---

## Examples

| Example | What you'll learn |
|---------|-------------------|
| [platform-example](../examples/platform-example/) | Full GitOps environment with orphans |
| [flux-boutique](../examples/flux-boutique/) | Simple Flux demo |
| [impressive-demo](../examples/impressive-demo/) | Comprehensive demo with CCVE scanning |
| [orphans](../examples/orphans/) | Detecting and managing orphan resources |
| [rm-demos-argocd](../examples/rm-demos-argocd/) | ArgoCD integration patterns |
| [apptique-examples](../examples/apptique-examples/) | E-commerce microservices patterns |
| [app-config-rtmsg](../examples/app-config-rtmsg/) | Real-time messaging configuration |
| [demos](../examples/demos/) | Interactive demo scripts |
| [integrations](../examples/integrations/) | Third-party tool integrations |

---

## Internal Docs

| Folder | Purpose |
|--------|---------|
| `planning/` | Product planning, specs |
| `archive/` | Historical documentation (gold content preserved) |
