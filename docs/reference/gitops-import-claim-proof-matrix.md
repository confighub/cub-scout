# GitOps Import Claim-vs-Proof Matrix

Status snapshot taken from the repo, published docs, and targeted live demo
checks on 2026-03-21.

This matrix is intentionally conservative. It records what each surface appears
to claim, what it actually proves today, and what still needs a follow-on slice.
Where called out explicitly below, items were rechecked against real kept-alive
demo clusters rather than docs alone.

## Proof Layers

Use these layers consistently:

1. Connected readiness
   Workers, targets, and connected workloads are present.
2. Import/render evidence
   Discover, dry, wet, and renderer results are visible in ConfigHub.
3. Post-import scan/finding evidence
   A concrete `cub-scout scan` result, policy result, or explicit "no finding"
   contract is shown immediately after import.

## Evidence Surfaces

Use these surfaces consistently:

- Cluster evidence
- ConfigHub evidence
- cub-scout evidence

Import/render evidence does not, by itself, prove live workload reconciliation.

## Matrix

| Surface | Audience | Import story claimed | Connected readiness proved | Import/render evidence proved | Post-import scan/finding evidence proved | AI-first structure | Notes |
|---|---|---|---|---|---|---|---|
| Published doc: [GitOps Import](https://docs.confighub.com/get-started/examples/gitops-import/) | GUI-first / official docs | Argo GitOps import into ConfigHub with worker-driven discovery/import; `cub gitops discover` and `cub gitops import` shown as CLI equivalents | Partial | Partial | No | No | Strong on import narrative, GUI-first, not a local AI-first walkthrough |
| [`examples/argo-import-confighub-demo/README.md`](../../examples/argo-import-confighub-demo/README.md) + local AI-first files | CLI-first / local demo | Three-path Argo story: `cub gitops import`, `cub-scout import-argocd`, `cub-scout import` | Yes, when `--with-worker` is used | Yes, when `--with-worker` is used | Yes, as cluster and cub-scout evidence | Yes | Slice 2 landed for Argo: `verify.sh` now includes `cub-scout scan` summary plus sample finding without collapsing it into import/render proof |
| [`examples/flux-import-confighub-demo/README.md`](../../examples/flux-import-confighub-demo/README.md) + local AI-first files | CLI-first / local demo | Flux import plus D2 ownership/discovery story: `cub gitops import`, `cub-scout import`, `tree`, `trace` | Yes, when `--with-worker` is used | Yes, when `--with-worker` is used | Yes, as cluster and cub-scout evidence | Yes | Slice 2 is now live-proved for Flux: `verify.sh` includes `cub-scout scan` summary and a kept-alive live run on 2026-03-21 returned `scan_contract=no-findings-observed` |
| [`docs/howto/import-to-confighub.md`](../../docs/howto/import-to-confighub.md) | Canonical how-to | Canonical import path; `cub-scout import` may delegate Argo/Flux to `cub gitops import` | Implied | Implied | No | No | Strong migration framing, and now points readers at the local AI-first proof harnesses, but is not itself a demo proof path |
| [`docs/howto/import-from-live.md`](../../docs/howto/import-from-live.md) | Canonical how-to | Cluster-first import; Argo/Flux workloads may delegate to `cub gitops import` when targets exist | Implied | Implied | No | No | Useful for discovery framing; now points to the AI-first demos for proof, but is not evidence-rich by itself |

## Current Conclusions

### What is genuinely proved today

- The local Argo and Flux demos now have AI-first entry files plus `setup.sh`,
  `verify.sh`, and `cleanup.sh`.
- Connected readiness is scriptable through
  [`examples/scripts/verify-connected-demo.sh`](../../examples/scripts/verify-connected-demo.sh).
- Import/render evidence is scriptable through `cub target list`, `cub unit list`,
  and the live `cub gitops import` path in both demos.
- The demos explicitly separate cluster, ConfigHub, and cub-scout evidence.
- The Argo AI-first verify path now includes real `cub-scout scan` evidence with
  summary output and a sample finding.
- The Flux AI-first verify path now includes a `cub-scout scan` contract with
  explicit findings-or-no-findings reporting.
- The Flux AI-first verify path has now been live-checked on a kept-alive
  cluster and returned `scan_contract=no-findings-observed`.

### What is still only partial or implied

- The published doc is still GUI-first and does not share the local AI-first
  structure.
- The two import how-tos now point readers at the local proof harness examples,
  but they still do not function as proof harnesses themselves.
- The demos still rely on `demo.sh` as the narrated human walkthrough, with
  `setup.sh` and `verify.sh` layered alongside it rather than replacing it.

### What is not yet proved

- The published doc does not yet expose the same scan/finding proof paths as the
  local Argo and Flux demos.
- The published doc does not yet expose the explicit no-finding contract used by
  the live-checked Flux demo.

## Recommended Next Slice

Decide what should change in the published doc while keeping the current local
docs honest:

1. decide whether the published doc should stay GUI-first or link out to the AI-first local proof harnesses
2. keep the evidence model explicit: cluster, ConfigHub, and cub-scout remain separate
3. preserve the difference between a sample finding contract (Argo) and a no-findings contract (Flux)
4. avoid collapsing scan evidence into import/render proof when updating external docs
