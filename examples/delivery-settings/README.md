# Delivery settings: who self-heals, who prunes, who applies differently

Every GitOps deployer carries settings that decide what it does without being
asked: whether it syncs on its own, whether it reverts a manual change, whether
it deletes what leaves the source, and which apply options it uses. They are
written per object, so on a cluster with many deployers the only way to see
them together is to read every object.

`gitops settings` reads them all and inverts them: for each setting, which
deployers have which value.

```bash
cub scout gitops settings                         # as a cub plugin
./cub-scout gitops settings                       # standalone
./cub-scout gitops settings --setting self-heal=off
./cub-scout gitops settings --project payments --format md
```

It covers Argo CD Applications and Flux Kustomizations and HelmReleases. See
the [command reference](../../docs/reference/commands.md#gitops-settings) for
flags and how to read the output.

## What it does not tell you

- It reports what each `spec` declares. It does not show that a controller
  acted on a setting, and it does not say whether a setting is acceptable.
  Judging a combination against a policy is not this command's job.
- It is not a new capability over `kubectl get applications -o json` and a
  filter; it is the same facts, read once, across both controllers, with the
  cases that are easy to get wrong handled the same way every time (below).
- Three agent eval cases ask its question
  ([report](../../evals/reports/2026-10-09-gitops-settings.md)). An agent that
  already had the raw export did not use the MCP tool and gained nothing; an
  agent with cub-scout and no export answered a 300-deployer question in four
  turns. That is not a general claim about agent time or cost.

## The cases the check pins

`verify-live.py` creates one owned kind cluster with real Argo CD v3.5.3 and
Flux, and these objects:

| Object | What its spec declares |
|---|---|
| Application `argocd/guestbook-auto` | automated, `selfHeal` and `prune` true, two sync options |
| Application `argocd/guestbook-manual` | no `syncPolicy` at all |
| Application `argocd/guestbook-partial` | `automated: {}`, four sync options, one `ignoreDifferences` rule |
| Application `team-b/outside` | automated with `selfHeal: false`, outside the Argo CD namespace |
| Kustomization `flux-system/podinfo` | `prune: true` only |
| Kustomization `flux-system/podinfo-held` | `suspend`, `force` and `wait` true, `prune: false` |
| HelmRelease `team-h/plain` | nothing beyond the chart |
| HelmRelease `team-h/tuned` | drift detection enabled with an ignore rule, install and upgrade fields |

It then checks, through the built binary with `--kube-context`:

1. **Absent is unset, not off.** `guestbook-partial` does not declare
   `selfHeal`; it is `unset` with default `off`, and still matches
   `--setting self-heal=off`. `guestbook-manual` has no automated sync, so its
   `self-heal` and `prune` are `n/a` and do not match.
2. **Not read is not absent.** As a service account that may list only Flux
   kinds, Applications are reported `NOT READ (forbidden)` and the output is
   marked incomplete. The four Applications exist; nothing says they do not.
3. **No invented links.** Before `argocd-cm` declares a `url`, no Application
   has a link. After, only Applications in the `argocd` namespace do;
   `team-b/outside` has no `argocd-cm` beside it and gets none. As a service
   account that may not read ConfigMaps, no link is shown at all.
4. **Options keep their values.** `Validate=false`,
   `PrunePropagationPolicy=foreground` and the HelmRelease's
   `upgrade.remediation.strategy=rollback` are reported as declared.
5. **One model, every rendering.** ASCII, Markdown, JSON, each `--group-by`,
   and the Delivery Settings section of `map deep-dive` agree.

One thing the real cluster showed that an authored fixture would not: the Flux
Kustomization CRD defaults `spec.force` to `false`, so a Kustomization that
never mentions `force` reads back as a declared `off`, while `suspend` and
`wait` read back absent. The recorded objects are in
[`pkg/agent/testdata/delivery-settings/`](../../pkg/agent/testdata/delivery-settings/)
and a unit test parses them.

```bash
python3 examples/delivery-settings/verify-live.py
```

Needs `kind`, `kubectl`, `curl` and the `flux` CLI, and network access. It uses
a private kubeconfig, never writes the shared one, and deletes its cluster.

## Retained runs

- [`live-proof.json`](live-proof.json): the passing run.
- [`live-attempt-1.json`](live-attempt-1.json): the first run, which failed in
  the harness, not the product. After making the ambient context unusable, the
  harness ran one more `kubectl` without naming a context.

## Limits

One kind cluster, one Argo CD version, one Flux version. Only
`guestbook-auto` and the `podinfo` Kustomization are waited on to sync; the
other objects exist to be read. The run does not cover ApplicationSet-generated
Applications, AppProject sync windows, per-resource sync-option annotations,
the `--tui` viewport, older Flux API versions (unit-tested only), or Sveltos.
