# gitops-settings-no-self-heal

Asks which deployers would leave a manual change in place: Argo CD Applications
that sync automatically without self-heal, and suspended Flux Kustomizations and
HelmReleases. It is the question `gitops settings` and the MCP `gitops_settings`
tool exist to answer (#839).

## Why this question

The answer is spread over thirty objects and is easy to get wrong from the
export:

- three of the six Applications never mention `selfHeal`, and one has
  `automated: {}`. A search for `selfHeal: false` finds two of the six;
- `argocd/dns-migration` has `selfHeal: false` and must be left out, because
  `automated.enabled: false` means it does not sync automatically;
- three Applications have no `automated` block at all and belong in neither
  list;
- every object's `managedFields` repeats the field names (`f:selfHeal: {}`),
  whatever the values are.

Expected answer:

```
NO_SELF_HEAL: argocd/etl-nightly, argocd/feature-store, argocd/fx-rates, argocd/ingress-nginx, argocd/playground, argocd/settlement-worker
SUSPENDED: HelmRelease/team-h/redis, Kustomization/flux-system/legacy-migration, Kustomization/flux-system/monitoring
```

It was derived three ways that agree: by hand from `scenario.yaml`, from the
recorded tool answers, and by a separate script over the raw export.

## What each arm gets

Both arms get the same export in `./cluster/`: `kubectl get -o yaml
--show-managed-fields` of the namespaces, Argo CD Applications and AppProjects,
Flux GitRepositories, HelmRepositories, Kustomizations and HelmReleases, and the
ConfigMaps in `argocd` (about 42 KB).

The cub-scout arm also gets the plugin's skills and recorded MCP answers from
the same cluster:

| Tool | Recording |
|---|---|
| `gitops_settings` | no arguments; `setting ["self-heal=off"]`; `setting ["suspend=on"]`; each in the default `summary` view and the `deployers` view |
| `doctor`, `map`, `scan`, `gitops_status` | one answer each, no arguments |
| `explain`, `trace`, `release_check` | none: the scenario has no workloads, and the mock says so |

`_tools.json` is the real `tools/list` from the recording binary, so the agent
sees the real descriptions and schemas.

## Limits, stated before any result

- **No controller is installed.** The cluster has the real Argo CD v3.5.3 and
  Flux CRDs and the objects in `scenario.yaml`; nothing reconciles and no object
  has a status. The case is about declared settings, which is all the tool
  reports. Values a CRD defaults are real: `spec.force: false` appears on
  Kustomizations that never set it.
- **The `gitops_settings` mock is an agent mock.** A small model picks the
  recording that matches the call's arguments, as the suite's `explain` and
  `trace` mocks do. It answers only the calls listed above and says so for any
  other; the real tool accepts any filter. That costs the cub-scout arm a turn
  if it tries a filter that was not recorded, and the mock's own model spend is
  counted in that arm's cost.
- **One scenario, thirty objects.** It says nothing about a large fleet, a
  cluster with running controllers, or any other question.
- **Every Application is in the Argo CD namespace.** The first wiring run had
  one in another namespace. Both arms left it out, reasoning that this Argo CD
  was not configured to manage Applications there. `gitops_settings` lists such
  an Application by what its spec declares and cannot tell whether an Argo CD
  instance manages it; that is a limit of the tool, recorded in #839. The object
  was moved because a case with a contested answer measures nothing, not to
  hide the limit.
- **A result here is not a general claim.** It is one question on one recording.
  Do not pool it with `benchmark-v1`, whose inputs are frozen and unchanged by
  this case.

## Result (2026-10-09)

Three runs per arm; see the [report](../reports/2026-10-09-gitops-settings.md)
for all three cases, the transcripts' behaviour and the limits.

| Arm | Both lists correct | $/run | Turns | Called `gitops_settings` |
|---|---:|---:|---:|---|
| with cub-scout | 3/3 | $0.32 | 7 | never |
| without | 3/3 | $0.24 | 6 to 7 | n/a |

**cub-scout added nothing here and cost more.** Both arms read the export; the
cub-scout arm never called the tool, and paid for the plugin's tool
descriptions and skill list in context.

## Re-recording

```bash
go build ./cmd/cub-scout
python3 evals/gitops-settings-no-self-heal/record.py
```

`record.py` creates and deletes its own kind cluster with a private kubeconfig.
It rewrites `fixtures/`, `mocks/`, `scaffold.sh` and `recording.json`. Re-record
when `gitops settings` output or the MCP tool list changes.

## Running

```bash
claude plugin eval . --scaffold --case gitops-settings-no-self-heal --runs 3 --max-cost-usd 10 --json evals/results/gitops-settings-no-self-heal.json
```
