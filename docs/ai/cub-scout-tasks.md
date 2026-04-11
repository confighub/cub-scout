# cub-scout Task Skill (For AI Agents Using cub-scout)

> **Audience:** AI agents (Claude, Codex, etc.) helping a human operator
> investigate, troubleshoot, or understand a Kubernetes cluster.
>
> **Difference from `skills/cub-scout/SKILL.md`:** That doc is about working *on*
> the cub-scout repo. This doc is about *using* cub-scout to answer real
> operator questions.

## Core mental model

`cub-scout` is a **read-only Kubernetes and GitOps observer**. It answers four
questions faster than `kubectl` + GitOps GUIs:

1. **What is here?** — what resources exist, grouped by owner
2. **Who owns it?** — Flux, ArgoCD, Helm, Terraform, Crossplane, native
3. **Where did it come from?** — Git source, controller chain
4. **What is broken?** — health, scan findings, secret references, events

cub-scout never writes to the cluster. It uses `Get`, `List`, `Watch` only.

## Task → command map

When the operator asks... | Run this | What you get
---|---|---
"What's running in my cluster?" | `cub-scout map list --json` | Every resource with owner classification
"What's broken?" | `cub-scout doctor --format json` | One-shot health summary
"Why is this resource broken?" | `cub-scout explain <kind>/<name> -n <ns> --presentation ai` | Owner, status, conditions, recent events, next-step hints
"Where did this come from?" | `cub-scout trace <kind>/<name> -n <ns>` | Full ownership chain to Git source
"Find unmanaged resources" | `cub-scout map list --json \| jq '.[] \| select(.owner=="Native")'` | Resources with no GitOps owner
"Are there config issues?" | `cub-scout scan --json` | 46-pattern misconfiguration scan
"What's the GitOps pipeline doing?" | `cub-scout gitops status` | Reconciliation state across Flux/Argo/Helm
"Show me the resource hierarchy" | `cub-scout tree ownership` | Resources grouped by GitOps owner
"Trace history of a resource" | `cub-scout trace <kind>/<name> -n <ns> --history` | Deployment history from controller storage

## First commands for any new cluster

```bash
./cub-scout version                    # Verify cub-scout is available
kubectl config current-context         # Verify cluster context
./cub-scout doctor                     # One-command health summary
./cub-scout map list --json | jq 'group_by(.owner) | map({owner: .[0].owner, count: length})'
```

That gives you: tool version, cluster context, overall health, and ownership
breakdown — enough to answer most "what's going on here" questions.

## Detailed task flows

### Task: Investigate a broken Deployment

```bash
# 1. Get plain-English explanation with hints
./cub-scout explain deploy/<name> -n <namespace> --presentation ai

# 2. If it has GitOps ownership, trace to source
./cub-scout trace deploy/<name> -n <namespace>

# 3. Check pipeline health for related controllers
./cub-scout gitops status

# 4. Scan namespace for known config issues
./cub-scout scan -n <namespace>
```

The `explain` command with `--presentation ai` is designed for AI consumption:
it includes structured fields, recent K8s events, and action-typed next-step
hints (v1.10+).

### Task: Find orphaned / unmanaged resources

```bash
./cub-scout map list --json | jq '.[] | select(.owner=="Native") | {namespace, kind, name}'
```

"Native" means: no GitOps ownership labels detected. These are typically
`kubectl apply`-ed resources or things created manually. Confirm before
suggesting cleanup — some Native resources are intentional (e.g.,
`kube-system/coredns`).

### Task: Understand a multi-controller cluster

```bash
./cub-scout tree ownership                  # Group by Flux/Argo/Helm/Native
./cub-scout map list --json | jq 'group_by(.owner) | map({owner: .[0].owner, count: length})'
```

This is cub-scout's biggest advantage over single-controller GUIs (Argo CD UI,
Flux CLI): one view of *all* GitOps controllers at once.

### Task: Cross-controller dependency check

When a resource managed by Argo depends on a CRD installed by Helm:

```bash
./cub-scout trace <kind>/<name> -n <namespace>     # Shows controller chain
./cub-scout graph export --format json             # Full resource graph
```

Only cub-scout sees across controller boundaries. The Argo UI cannot show
"this Application depends on a CRD installed by Helm".

### Task: Connected mode (ConfigHub) — drift and history

```bash
./cub-scout status                                                    # Verify connected
./cub-scout compare three-way --scope namespace/<ns>                   # DRY/WET/LIVE
./cub-scout history deploy/<name> -n <ns> --since 7d                  # Change timeline
./cub-scout impact <unit-slug>                                        # Blast radius
```

Connected mode requires `cub auth login` first. If not authenticated,
`./cub-scout status` will report standalone mode.

## Output interpretation

### `cub-scout doctor` exit codes

- `0` — cluster healthy
- non-zero — issues found; details in stdout

### `cub-scout scan` severity

- `CRITICAL` — actionable now (e.g., known outage patterns)
- `WARNING` — likely problem
- `INFO` — best-practice violation

### `cub-scout explain --presentation ai` JSON shape

The AI presentation mode emits structured JSON suitable for LLM parsing:
status, conditions, events, ownership chain, and next-step hints. Prefer
this over scraping plain-text output.

### Owner values returned by `map list --json`

- `Flux` — kustomize.toolkit.fluxcd.io or helm.toolkit.fluxcd.io labels
- `ArgoCD` — argocd.argoproj.io labels or tracking-id annotations
- `Helm` — app.kubernetes.io/managed-by=Helm (and not Flux)
- `Terraform` — app.terraform.io annotations
- `Crossplane` — crossplane.io labels (experimental)
- `kro` — kro.run labels (experimental)
- `ConfigHub` — confighub.com/UnitSlug label
- `Native` — none of the above

## When NOT to use cub-scout

cub-scout is read-only and metadata-focused. It cannot:

| Task | Use this instead |
|------|------------------|
| Read pod logs | `kubectl logs` |
| Exec into a pod | `kubectl exec` |
| Roll back a deployment | `kubectl rollout undo`, Argo UI, Flux suspend/resume |
| Edit resources | `kubectl edit` (and check the GitOps source first) |
| Apply manifests | `kubectl apply`, Argo sync, Flux reconcile |
| View metrics | Prometheus, Grafana |
| Scan vulnerabilities | Trivy, Falco, etc. |
| Manage secrets | Vault, External Secrets, SOPS |

## Verification rule

Before claiming a command exists, verify with `--help`:

```bash
./cub-scout --help
./cub-scout <command> --help
```

Command surface evolves between releases. Use `--help` as source of truth, not
prior knowledge.

## Connected vs standalone — quick check

```bash
./cub-scout status
```

If output shows `Mode: standalone`, only standalone commands work. If
`Mode: connected`, ConfigHub-backed commands (`import`, `fleet`, `history`,
`compare three-way`, `impact`) are also available.

## JSON-first for automation

Every cub-scout command that produces output supports `--json` or
`--format json`. Prefer JSON output when the result will be processed
programmatically:

```bash
./cub-scout doctor --format json
./cub-scout map list --json
./cub-scout scan --json
./cub-scout explain deploy/x -n y --presentation ai
./cub-scout graph export --format json
```

Schema is documented in `docs/reference/json-contracts.md` and is treated as
a stable contract.

## MCP mode

`cub-scout` exposes a Model Context Protocol gateway:

```bash
./cub-scout mcp serve
```

Standalone MCP tools: `doctor`, `explain`, `map`, `scan`, `trace`.
Connected mode adds read-only ConfigHub query tools.

When operating via MCP, prefer the structured tool calls over shell
invocations — the MCP layer handles JSON parsing and schema validation.

## Honest limitations to report to operators

If asked about something cub-scout cannot do today, do not invent. Be honest:

- **No pod logs / events streaming** — read-only metadata only (events are
  fetched on-demand in `explain`/`trace`, not streamed)
- **No write operations** — never modifies cluster state
- **No ML / inference** — deterministic label parsing
- **Single cluster in standalone mode** — multi-cluster only via connected mode
- **Known scale boundary** — tested to ~500 resources; larger clusters
  unprofiled

When a capability is missing, offer to file an issue with evidence using
`./scripts/create-ai-capability-issue.sh` or the GitHub issue template
"AI capability gap".
