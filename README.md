# cub-scout — explore and map GitOps clusters

**Read-only. Deterministic. Offline-capable. Optional `cub` plugin.**

cub-scout is an open-source observer for Kubernetes and GitOps.
**It observes and explains; it never decides.** It diagnoses, explains,
traces, maps, and scans Kubernetes resources and their GitOps origins — but
never modifies cluster state and never makes authority calls about what
*should* be true. It is safe to run against production.

It helps you answer:
- what owns this resource, really?
- where did it come from in Git?
- what is broken right now, and what should I do next?
- how does **intended** (governed) state compare to **live** (cluster) state?

It works standalone with your current kube context, or connected to
[ConfigHub](https://confighub.com) for governed comparison, change history,
import, fleet queries, and AI-friendly read-only workflows.

### Who reaches for cub-scout?

- **SREs and on-call** — when a workload is unhealthy and you need to know
  *why* and *who owns it* in seconds, not by clicking through Argo or Flux UIs.
- **Platform engineers** — when you inherit a cluster and need a
  trustworthy ownership map across Flux, ArgoCD, Helm, Crossplane, ConfigHub,
  and naked YAML.
- **AI agents and automation** — when you need stable, deterministic JSON or
  a Model Context Protocol (MCP) gateway for read-only cluster facts.

### `cub-scout` vs `cub`: the boundary

cub-scout and [`cub`](https://confighub.com) are designed to be complementary,
not interchangeable:

| | `cub-scout` | `cub` |
|---|---|---|
| Role | **Observe and explain** | **Act and govern** |
| Cluster state | Read-only | Read + reconcile via workers |
| ConfigHub state | Read-only queries | Authoring, import, promotion |
| Best for | Diagnosis, ownership, drift, audit, AI tools | Intended-state authoring, GitOps pipelines |

cub-scout is the **read-only witness**. ConfigHub (driven by `cub`) is the
**authority**. The two ship as separate binaries and never overlap on writes.

> **New in v2.0:** cub-scout also runs as a `cub` plugin (`cub scout ...`)
> with inherited auth. Standalone `cub-scout` is unchanged. See
> [How To Run It](#how-to-run-it) and the
> [migration guide](docs/releases/v2.0.0-migration-guide.md).

**New here?** Start with the [Fast Path](#fast-path-2-minutes), then use:
- [CLI Guide](CLI-GUIDE.md) for the workflow-first tour
- [docs/reference/cli-reference.md](docs/reference/cli-reference.md) for the full command catalog
- [docs/reference/commands.md](docs/reference/commands.md) for detailed usage examples
- [docs/reference/cli-contract.md](docs/reference/cli-contract.md) for stable flags and schemas

Please send feedback by [opening an issue](https://github.com/confighub/cub-scout/issues)
or joining [Discord](https://discord-auth.confighub.net/discord/join) via ConfigHub signup.

---

## What cub-scout does, at a glance

| Capability | Command | What you get |
|---|---|---|
| **Diagnose** cluster health | `doctor` | One-screen summary of ownership, health, drift, and risk, with concrete next steps. |
| **Explain** any resource | `explain` | Plain-English ownership and lineage for one resource — auto-detects Flux, ArgoCD, Helm, Crossplane, or native. |
| **Trace** workloads to Git | `trace` | The full ownership chain from a Kubernetes object back to its Git source, ApplicationSet, or OCI registry. |
| **Map** the cluster interactively | `map` | TUI for browsing ownership, health, and dependencies across a cluster. |
| **Scan** for risk | `scan` | Audits live clusters or manifest files against 46 built-in risk patterns. Optionally delegates to the separate [confighub-scan](https://github.com/confighubai/confighub-scan) tool for its broader catalog. |
| **Compare** intent vs reality | `compare three-way` | Three-way diff between **DRY** (what ConfigHub *intends* to deploy), **WET** (what the renderer *produced*), and **LIVE** (what is *running* in the cluster). Surfaces drift the Kubernetes API alone cannot. *Connected only.* |
| **Serve** as an AI gateway | `mcp serve` | Read-only Model Context Protocol (MCP) server over stdio for Claude, Codex, and other agents. |

All commands emit deterministic JSON via `--format json` for scripts and
agents. Same input, same output — no AI/ML inference inside the core
ownership logic.

---

## Fast Path (2 Minutes)

**No ConfigHub signup required.** cub-scout reads from your current kube
context and works fully offline.

```bash
# Install (Homebrew — see Install section below for other options)
brew install confighub/tap/cub-scout

# Triage flow: doctor → explain → trace
cub-scout doctor                                  # cluster health summary
cub-scout explain deploy/frontend -n boutique     # who owns this resource?
cub-scout trace   deploy/frontend -n boutique     # where did it come from?
cub-scout map                                     # interactive TUI
```

If `cub-scout: command not found` but you have `cub` installed, try
`cub scout doctor` instead — see [How To Run It](#how-to-run-it) for the
plugin form.

What you get in the first two minutes:
- ownership detection across Flux, ArgoCD, Helm, Crossplane, ConfigHub, and native resources
- one-command health summary with next steps
- trace from workload to Git source (Application, ApplicationSet, GitRepository, OCI registry)
- recent Kubernetes events surfaced in `explain` and `trace`
- deterministic JSON for scripts, AI agents, and MCP clients

---

## Install

```bash
# cub plugin (preferred for ConfigHub users — one install, shared auth)
cub plugin install confighub/cub-scout
cub scout version

# Homebrew
brew install confighub/tap/cub-scout

# Go install
go install github.com/confighub/cub-scout/cmd/cub-scout@latest

# Binary download
curl -sL https://github.com/confighub/cub-scout/releases/latest

# Container
docker run ghcr.io/confighub/cub-scout:latest version

# kubectl krew
kubectl krew install cub-scout
```

If you want the `kubectl` plugin wrapper too:

```bash
make build-kubectl-plugin
kubectl cub-scout map list --json
```

See [docs/howto/plugin-install.md](docs/howto/plugin-install.md) for pinned
version install, direct-URL install, offline install, and plugin-mode
troubleshooting.

---

## How To Run It

Same binary, three ways to invoke it. Commands, flags, JSON, exit codes, and
MCP tool names are identical across all three; only the invocation prefix
differs.

### 1. Standalone — "if you can kubectl, you can cub-scout"

No ConfigHub signup required. Reads from your current kube context.

```bash
brew install confighub/tap/cub-scout
cub-scout doctor
cub-scout explain deploy/api -n prod
cub-scout trace deploy/api -n prod
```

### 2. Connected — ConfigHub-backed comparison, history, import

Adds governed read paths once you've authenticated with `cub`. cub-scout
still never modifies the cluster — `import` writes records into ConfigHub,
not manifests into your cluster.

```bash
cub auth login
cub-scout compare three-way --scope namespace/prod
cub-scout history deploy/api -n prod
cub-scout import --dry-run -n prod
```

### 3. Plugin mode (new in v2.0) — `cub scout ...`

A **`cub` plugin** is an installable extension to the `cub` CLI that lets you
invoke another tool as a `cub` subcommand. cub-scout was the first tool we
shipped this way: after `cub plugin install confighub/cub-scout`, the plugin
form `cub scout ...` runs the same binary as standalone `cub-scout ...` but
inherits `cub`'s authentication, context, and ConfigHub session
automatically — no separate login step.

Use plugin form when:
- you already have `cub` installed and authenticated (one auth surface)
- you are invoking cub-scout from AI tools (MCP tool descriptions and `nextSteps`
  hints render `cub scout ...`, so agent output stays consistent)

Otherwise, standalone form is the path of least resistance: `brew install`
and you're done.

```bash
cub plugin install confighub/cub-scout
cub scout doctor
cub scout compare three-way --scope cluster
cub scout mcp serve
```

Under the hood, `cub` exec's the plugin binary with `CUB_PLUGIN=1`,
`CUB_TOKEN`, and `CUB_CONTEXT` set. The plugin form inherits auth without a
separate login step and never recursively shells out to `cub auth get-token`.

Standalone and plugin forms are held to byte-equivalence by a release-gate
test (`TestPluginParity_StandaloneMatchesPlugin`).

---

## Choose Your Goal

Each example below uses standalone form (`cub-scout ...`). If you installed
the plugin (`cub plugin install confighub/cub-scout`), substitute `cub scout`
for `cub-scout` in any command — behavior is identical.

### Standalone Value Now

```bash
cub-scout quickstart --yes
cub-scout doctor
cub-scout explain deploy/api -n prod
cub-scout trace deploy/api -n prod
cub-scout scan
```

Use this path when you want immediate cluster visibility without signing into
anything else.

### Connected Value Now

```bash
cub auth login
cub-scout status
cub-scout compare three-way --scope namespace/prod
cub-scout history deploy/api -n prod
cub-scout impact payments-api
cub-scout import --dry-run -n prod
```

Use this path when you want ConfigHub-backed history, comparison, import, and
fleet workflows. In plugin form, `cub auth login` is all you need — the
plugin inherits the token automatically, no separate cub-scout auth step.

### AI And Automation

```bash
cub scout mcp serve                                 # or: cub-scout mcp serve
cub scout doctor --format json
cub scout explain deploy/api -n prod --format json
cub scout compare three-way --scope namespace/prod --format json
cub scout context-pack --format json --max-bytes 16384
```

Use this path when you want stable read-only JSON or an MCP gateway for
Claude, Codex, or other agent tooling. The plugin form is preferred here
because MCP tool descriptions, structured `nextSteps` hints, and canonical
trust URLs all render the `cub scout ...` invocation form so downstream
agents see a consistent command shape.

---

## Signature Example

`trace` is the headline feature: it answers "what created this?" without making
you manually jump across Deployments, Applications, Helm releases, labels,
annotations, and controller state.

```text
$ cub-scout trace deploy/frontend -n boutique

TRACE: Deployment/frontend in boutique
Owner: Flux
Source: GitRepository/flux-system/platform-config
Path: clusters/prod/apps/boutique
Status: Ready

Recent events:
  Warning BackOff (3x, 5m) kubelet: Back-off restarting failed container

Next:
  cub-scout explain deploy/frontend -n boutique
```

For more detailed examples, see [docs/reference/commands.md#trace](docs/reference/commands.md#trace).

---

## Choose Your Interface

| Interface | Best for | Start here |
|-----------|----------|------------|
| TUI | Interactive exploration and keyboard-driven debugging | `cub-scout map` |
| CLI | One-off troubleshooting, shell use, pipelines | `doctor`, `explain`, `trace` |
| JSON | Automation, AI, MCP, downstream tooling | `--format json` or `--json` |

Press `?` inside the TUI for shortcuts and panel navigation.

**Ownership at a glance:**

![cub-scout map dashboard](docs/images/map-dashboard.png)

### Scan Variants

`scan` is the fastest way to audit a cluster or manifest set for known risk patterns.
The built-in scanner covers **46 patterns**, and the broader
[confighub-scan](https://github.com/confighubai/confighub-scan) catalog tracks
**3,513 risk patterns** for deeper policy and detection work.

Common entrypoints:

- `cub-scout scan --state`
- `cub-scout scan --kyverno`
- `cub-scout scan --lifecycle-hazards`
- `cub-scout scan --timing-bombs`
- `cub-scout scan --dangling`
- `cub-scout scan --file manifest.yaml`
- `cub-scout scan --json`
- `cub-scout scan --normalized-json`

```bash
cub-scout scan --state
cub-scout scan --kyverno
cub-scout scan --lifecycle-hazards
cub-scout scan --timing-bombs
cub-scout scan --dangling
cub-scout scan --file manifest.yaml
cub-scout scan --json
cub-scout scan --normalized-json
```

For detailed scanner behavior and output shape, use
[docs/reference/commands.md#scan](docs/reference/commands.md#scan) and
[docs/reference/json-contracts.md](docs/reference/json-contracts.md).

---

## Standalone vs Connected

cub-scout works fully offline. Connected mode is optional but unlocks the
questions a live cluster alone cannot answer.

**Standalone (no signup):** Works from your current kube context. You get
ownership detection, ownership tracing, health diagnosis, drift hints,
risk scanning, JSON, and an MCP gateway — everything you need to triage
*right now* on the cluster in front of you.

**Connected (`cub auth login`):** Adds the historical and cross-cluster
context that the Kubernetes API does not have. Specifically:

- **DRY vs WET vs LIVE** — compare what ConfigHub *intended* to deploy
  (DRY), what the renderer *produced* (WET), and what is actually
  *running* (LIVE). Catches drift the live cluster can't tell you about.
- **Change history** — ask "when did this change, who changed it, and
  why?" against ConfigHub's governed timeline, not just `kubectl events`.
- **Fleet queries** — answer "is this version running everywhere it
  should?" across many clusters at once.
- **Import preview** — propose how a live workload should be modeled in
  ConfigHub before writing anything.

The capability axis is orthogonal to the [invocation form](#how-to-run-it):
every row below works identically in `cub-scout ...` standalone and `cub
scout ...` plugin form. Parity is enforced by a release-gate test
(`TestPluginParity_StandaloneMatchesPlugin`).

| Capability | Standalone | Connected |
|------------|:----------:|:---------:|
| Map cluster resources | ✓ | ✓ |
| Explain ownership and lineage | ✓ | ✓ |
| Trace to Git source | ✓ | ✓ |
| Scan for risk issues | ✓ | ✓ |
| Deterministic JSON output | ✓ | ✓ |
| Model Context Protocol (MCP) gateway | ✓ | ✓ |
| DRY vs WET vs LIVE comparison | — | ✓ |
| Connected change history | — | ✓ |
| Fleet-level queries | — | ✓ |
| Import workloads into ConfigHub | — | ✓ |

In plugin form (`cub scout ...`) the token from `cub auth login` is inherited
automatically. In standalone form (`cub-scout ...`) it is picked up from
`cub`'s local token store.

**Important:** connected import writes ConfigHub records, not cluster
manifests. cub-scout never modifies the cluster.

---

## Quick Connect

Use `setup connect` when you want "just connect and go" with minimal kubeconfig
setup:

```bash
# Import an existing kubeconfig context and launch the TUI
cub-scout setup connect --from-kubeconfig ./artem.yaml --from-context ske-vcl-pro --map

# Or connect directly to an API endpoint with a token
cub-scout setup connect https://api.example.com:6443 \
  --token "$K8S_BEARER_TOKEN" \
  --context prod \
  --map
```

If you prefer CLI-only, omit `--map`.

---

## Why People Reach For It

GitOps stacks are powerful, but the day-two questions are still painful:

- A Deployment exists — but what actually owns it?
- Argo or Flux says "Synced" — but is the cluster actually converged?
- Which events explain *this* failure, right now?
- Is this resource governed, orphaned, or manually changed?
- Did someone hot-patch prod, or did the renderer produce this?

cub-scout answers these through **one read-only observation layer** instead
of making you stitch them together by hand with `kubectl`, controller CRDs,
labels, annotations, and UI tabs across Argo, Flux, and Helm.

It is built to be the tool you can keep running on a cluster you don't fully
trust yet — including production.

---

## AI First-Read

For Claude, Codex, and other AI agents:
- start with [AI-README-FIRST.md](AI-README-FIRST.md)
- then load [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md) if repo-local skills are supported
- for AI tool setup, see [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md)

What `cub-scout mcp serve` is:
- a read-only **Model Context Protocol (MCP)** server over stdio
- backed by the existing CLI JSON surfaces — same answers, agent-shaped
- strongest as a troubleshooting and exploration layer for AI agents

What it is not:
- not a Kubernetes write tool — every tool call goes through the read-only CLI
- not a multi-gateway router — it speaks one protocol, MCP
- not the ConfigHub authority layer — that role belongs to `cub`

---

## Documentation Map

Start here based on what you need:

| Need | Doc |
|------|-----|
| Workflow-first CLI tour | [CLI-GUIDE.md](CLI-GUIDE.md) |
| Full command catalog | [docs/reference/cli-reference.md](docs/reference/cli-reference.md) |
| Command usage and examples | [docs/reference/commands.md](docs/reference/commands.md) |
| Stable flags and schemas | [docs/reference/cli-contract.md](docs/reference/cli-contract.md) |
| JSON fields and output model | [docs/reference/json-contracts.md](docs/reference/json-contracts.md) |
| Getting started checklist | [docs/getting-started/checklist.md](docs/getting-started/checklist.md) |
| Import and migration path | [docs/howto/import-to-confighub.md](docs/howto/import-to-confighub.md) |
| AI tool integration | [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md) |
| Examples and demos | [examples/README.md](examples/README.md) |
| Security model | [SECURITY.md](SECURITY.md) |

---

## Build From Source

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
./cub-scout version
```

---

## Principles

- **Read-only, by design** — cluster observation uses only `Get`, `List`, and
  `Watch`. cub-scout has no `apply`, no `delete`, no admission webhook, no
  cluster-mutating code path. Even `suggest-remedy` is read-only — it
  describes a fix that *would* resolve a finding; it does not run it. Apply
  via ConfigHub or kubectl, governed. Run cub-scout against production
  without an approval ticket.
- **Deterministic** — same inputs, same outputs. No AI/ML inference inside the
  core ownership logic. Ownership decisions are reproducible and explainable.
- **Parse, don't guess** — ownership comes from real labels, annotations,
  owner references, and controller facts. We refuse to invent provenance.
- **Complement GitOps, don't replace it** — cub-scout helps you understand
  Flux, ArgoCD, Helm, Crossplane, and ConfigHub state. It is not another
  reconciler.
- **Graceful degradation** — works without ConfigHub, without internet, and
  many flows work without a live cluster (debug bundles, manifest scans,
  Git-path import preview).
- **Evidence, not authority** — cub-scout is the read-only **witness**.
  ConfigHub (driven by `cub`) is the **authority** for intended state. The two
  never overlap on writes.

For more on the security and operating model, see [SECURITY.md](SECURITY.md).

---

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

- **Found a bug?** [Open an issue](https://github.com/confighub/cub-scout/issues)
- **Have an idea?** Start a discussion
- **Want to contribute?** PRs welcome

---

## Community

- **Discord:** [discord.gg/confighub](https://discord-auth.confighub.net/discord/join)
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues)
- **Website:** [confighub.com](https://confighub.com)
