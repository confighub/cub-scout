# cub-scout -- explore and map GitOps clusters

**Offline-first. Deterministic. Cluster read-only.**

cub-scout is an open-source cluster explorer for Kubernetes and GitOps. It helps
you answer:
- what owns this resource?
- where did it come from?
- what is broken right now?
- how does governed state compare to live state?

It works standalone with your current kube context, or connected to
[ConfigHub](https://confighub.com) for comparison, import, history, fleet, and
AI-friendly read-only workflows.

**New here?** Start with the [Fast Path](#fast-path-2-minutes), then use:
- [CLI Guide](CLI-GUIDE.md) for the workflow-first tour
- [docs/reference/cli-reference.md](docs/reference/cli-reference.md) for the full command catalog
- [docs/reference/commands.md](docs/reference/commands.md) for detailed usage examples
- [docs/reference/cli-contract.md](docs/reference/cli-contract.md) for stable flags and schemas

Please send feedback by [opening an issue](https://github.com/confighub/cub-scout/issues)
or joining [Discord](https://discord-auth.confighub.net/discord/join) via ConfigHub signup.

---

## Fast Path (2 Minutes)

```bash
brew install confighub/tap/cub-scout
cub-scout quickstart
cub-scout doctor
cub-scout explain deploy/app -n ns
cub-scout trace deploy/app -n ns
cub-scout map
```

What you get in the first two minutes:
- ownership detection across Flux, ArgoCD, Helm, ConfigHub, and native resources
- one-command health summary with next steps
- trace from workload to Git source
- recent Kubernetes events in `explain` and `trace`
- deterministic JSON for scripts, AI agents, and MCP clients

---

## Install

```bash
# Homebrew
brew install confighub/tap/cub-scout

# Go install
go install github.com/confighub/cub-scout/cmd/cub-scout@latest

# Binary download
curl -sL https://github.com/confighub/cub-scout/releases/latest

# Container
docker run ghcr.io/confighub/cub-scout:latest version
```

If you want the `kubectl` plugin wrapper too:

```bash
make build-kubectl-plugin
kubectl cub-scout map list --json
```

---

## Choose Your Goal

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
fleet workflows.

### AI And Automation

```bash
cub-scout mcp serve
cub-scout doctor --format json
cub-scout explain deploy/api -n prod --format json
cub-scout compare three-way --scope namespace/prod --format json
cub-scout context-pack --format json --max-bytes 16384
```

Use this path when you want stable read-only JSON or an MCP gateway for Claude,
Codex, or other agent tooling.

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

cub-scout works fully offline. Connected mode is optional.

| Capability | Standalone | Connected |
|------------|:----------:|:---------:|
| Map cluster resources | ✓ | ✓ |
| Explain ownership and lineage | ✓ | ✓ |
| Trace to Git source | ✓ | ✓ |
| Scan for risk issues | ✓ | ✓ |
| Deterministic JSON output | ✓ | ✓ |
| MCP gateway | ✓ | ✓ |
| Import workloads into ConfigHub | — | ✓ |
| Connected change history | — | ✓ |
| DRY vs WET vs LIVE comparison | — | ✓ |
| Fleet-level queries | — | ✓ |

**Standalone:** Works from your current kube context with no signup required.

**Connected:** Run `cub auth login` to enable ConfigHub-backed comparison,
history, import, and fleet features.

Connected import writes ConfigHub records, not cluster manifests.

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
- a Deployment exists, but what actually owns it?
- Argo or Flux says "synced", but is the cluster converged?
- which events explain the failure right now?
- is this resource governed, orphaned, or manually changed?

cub-scout gives you those answers through one read-only observation layer
instead of making you stitch them together by hand with `kubectl`, controller
CRDs, labels, and UI tabs.

---

## AI First-Read

For Claude, Codex, and other AI agents:
- start with [AI-README-FIRST.md](AI-README-FIRST.md)
- then load [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md) if repo-local skills are supported
- for AI tool setup, see [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md)

What `cub-scout mcp serve` is:
- a read-only MCP server over stdio
- backed by the existing CLI JSON surfaces
- strongest as a troubleshooting and exploration layer

What it is not:
- not a Kubernetes write tool
- not a multi-gateway router
- not the full ConfigHub authority layer

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

- **Read-only by default**: cluster observation uses `Get`, `List`, and `Watch`
- **Deterministic**: same inputs, same outputs; no AI/ML inside core ownership logic
- **Parse, don't guess**: ownership comes from real labels, annotations, owner refs, and controller facts
- **Complement GitOps**: cub-scout helps you understand Flux, ArgoCD, Helm, and ConfigHub state; it does not try to replace them
- **Graceful degradation**: works without ConfigHub, without internet, and many flows work without a live cluster

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
