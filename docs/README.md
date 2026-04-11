# cub-scout Documentation

**Read-only Kubernetes observer. See what's really happening in your cluster.**

---

## Start Here

| Step | Guide | Time |
|------|-------|------|
| Install | [getting-started/install.md](getting-started/install.md) | 2 min |
| First Map | [getting-started/first-map.md](getting-started/first-map.md) | 5 min |
| CLI workflow guide | [CLI-GUIDE.md](../CLI-GUIDE.md) | reference |

---

## Core Features (Standalone — No Account Needed)

### Discover

| What | Command | Guide |
|------|---------|-------|
| See ownership (Flux, Argo, Helm, Native) | `cub-scout map list` | [howto/ownership-detection.md](howto/ownership-detection.md) |
| Find orphan resources | `cub-scout map orphans` | [howto/find-orphans.md](howto/find-orphans.md) |
| Trace provenance chains | `cub-scout trace deploy/NAME -n NS` | [howto/trace-ownership.md](howto/trace-ownership.md) |
| Explain a resource | `cub-scout explain deploy/NAME -n NS` | [reference/commands.md](reference/commands.md#explain) |

### Inspect

| What | Command | Guide |
|------|---------|-------|
| GitOps pipeline health | `cub-scout gitops status` | [reference/commands.md#gitops-v014](reference/commands.md#gitops-v014) |
| Scan for risks | `cub-scout scan --state` | [howto/scan-for-risks.md](howto/scan-for-risks.md) |
| Scan a manifest file | `cub-scout scan --file FILE` | [howto/scan-for-risks.md](howto/scan-for-risks.md) |
| Drift detection | `cub-scout drift deploy/NAME -n NS` | [howto/drift.md](howto/drift.md) |

### Analyze

| What | Command | Guide |
|------|---------|-------|
| Tree hierarchies | `cub-scout tree ownership` | [howto/tree-hierarchies.md](howto/tree-hierarchies.md) |
| Git repo patterns | `cub-scout patterns detect --git-root PATH` | [reference/commands.md#patterns-v07](reference/commands.md#patterns-v07) |
| Git + cluster alignment | `cub-scout combined --git-path PATH` | [combined-git-live example](../examples/combined-git-live/) |
| Export resource graph | `cub-scout graph export` | [reference/commands.md#graph-v06](reference/commands.md#graph-v06) |
| Debug bundles | `cub-scout bundle inspect FILE` | [howto/debug-bundle.md](howto/debug-bundle.md) |

---

## Connected Mode (Requires ConfigHub Account)

Connected mode adds: change history, fleet comparison, import/promotion workflows.

> **Ownership:** The `cub` CLI comes from the [ConfigHub SDK](https://github.com/confighub/sdk).
> cub-scout discovers and explains; `cub` handles connected lifecycle.

| Step | Guide | Time |
|------|-------|------|
| Why connect? | [concepts/why-connected-mode.md](concepts/why-connected-mode.md) | 5 min read |
| First import | [getting-started/first-import.md](getting-started/first-import.md) | 10 min |
| Canonical import path | [howto/import-to-confighub.md](howto/import-to-confighub.md) | reference |
| Import from live cluster | [howto/import-from-live.md](howto/import-from-live.md) | reference |
| Migration playbook | [howto/migration-playbook.md](howto/migration-playbook.md) | reference |
| Fleet queries | [howto/fleet-queries.md](howto/fleet-queries.md) | reference |
| Break-glass to managed | [howto/break-glass-to-managed.md](howto/break-glass-to-managed.md) | reference |

### Connected Demos (AI-First)

| Demo | What it shows |
|------|---------------|
| [Argo import demo](../examples/argo-import-confighub-demo/) | Three Argo import lenses on one cluster |
| [Flux import demo](../examples/flux-import-confighub-demo/) | Flux D2-pattern import with ConfigHub |

---

## AI Integration

| Guide | For |
|-------|-----|
| [AI skill bundle](../skills/cub-scout/SKILL.md) | Canonical Claude/Codex operating profile |
| [Using cub-scout from AI tools](howto/using-cub-scout-from-ai-tool.md) | Claude Code, Cursor, Copilot setup |
| [Capability assistant playbook](howto/claude-capability-assistant.md) | "Can cub-scout do X?" workflow |
| [Kubara + Argo debugging](howto/kubara-argo-debugging.md) | ApplicationSet platform debugging |

---

## Reference

| Topic | Link |
|-------|------|
| CLI workflow guide | [CLI-GUIDE.md](../CLI-GUIDE.md) |
| Complete CLI reference (A-Z) | [reference/cli-reference.md](reference/cli-reference.md) |
| Command usage examples | [reference/commands.md](reference/commands.md) |
| JSON contracts | [reference/json-contracts.md](reference/json-contracts.md) |
| Semantic contract (JSON vs ASCII) | [semantic-contract.md](semantic-contract.md) |
| Command matrix | [reference/command-matrix.md](reference/command-matrix.md) |
| CLI contract | [reference/cli-contract.md](reference/cli-contract.md) |
| Ownership & precedence | [reference/ownership-precedence.md](reference/ownership-precedence.md) |
| Health & failure states | [reference/health-failure-states.md](reference/health-failure-states.md) |
| Glossary | [reference/glossary.md](reference/glossary.md) |
| All examples | [reference/examples-overview.md](reference/examples-overview.md) |
| Visual diagrams | [diagrams/README.md](diagrams/README.md) |

---

## Concepts

| Concept | Link |
|---------|------|
| GitOps overview | [concepts/gitops-overview.md](concepts/gitops-overview.md) |
| Architecture | [concepts/architecture.md](concepts/architecture.md) |
| The clobbering problem | [concepts/clobbering-problem.md](concepts/clobbering-problem.md) |
| Mental model | [concepts/mental-model.md](concepts/mental-model.md) |
| Why connected mode | [concepts/why-connected-mode.md](concepts/why-connected-mode.md) |

---

## Internal

| File | Purpose |
|------|---------|
| [HANDOVER.md](../HANDOVER.md) | AI coder handover document |
| [roadmap.md](roadmap.md) | Canonical roadmap |
| [RELEASE-PROCESS.md](RELEASE-PROCESS.md) | Release checklist |
| [reference/testing.md](reference/testing.md) | Testing guide |
| `archive/` | Historical documentation |
