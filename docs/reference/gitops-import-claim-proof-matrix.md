# GitOps Import Claim-vs-Proof Matrix

Status snapshot taken from the repo, published docs, and targeted live demo
checks on 2026-03-22.

This matrix is intentionally conservative. It records what each surface appears
to claim, what it actually proves today, and what still needs a follow-on slice.
Where called out explicitly below, items were rechecked against real kept-alive
demo clusters rather than docs alone.

## Proof Layers

Use these layers consistently:

1. Connected readiness
   Workers, targets, and imported dry/wet units are present; connected-workload
   preview counts are a bounded secondary signal.
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
| [`examples/argo-import-confighub-demo/README.md`](../../examples/argo-import-confighub-demo/README.md) + local AI-first files | CLI-first / local demo | Three-path Argo story: `cub gitops import`, `cub-scout import-argocd`, `cub-scout import` | Partial | Partial | Yes, as cluster and cub-scout evidence | Yes | Rechecked live on 2026-03-22: the Argo harness reaches `cub gitops discover`/`import`; connected readiness now gates on worker-backed targets plus dry/wet units, while the scout connected-workload preview is bounded and may skip on timeout in the `argocd` namespace; the Arnie fixture Applications still degrade in the renderer because their Git sources are not fetchable |
| [`examples/flux-import-confighub-demo/README.md`](../../examples/flux-import-confighub-demo/README.md) + local AI-first files | CLI-first / local demo | Flux import plus D2 ownership/discovery story: `cub gitops import`, `cub-scout import`, `tree`, `trace` | Partial | Partial | Yes, as cluster and cub-scout evidence | Yes | Live checks show workers, targets, units, and scan output; connected readiness now treats the scout connected-workload preview as a bounded secondary signal instead of a hard gate, and exact scan outcomes remain environment-specific |
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
- The live demo checks now show where the remaining proof gap is: both demos
  can produce workers, targets, units, and scan evidence, while the scout
  connected-workload preview can still stall or diverge from the imported demo
  space.

### What is still only partial or implied

- The published doc is still GUI-first and does not share the local AI-first
  structure.
- The scout connected-workload preview remains noisy in live checks,
  especially around `argocd`, so it is now bounded and treated as a secondary
  signal rather than the primary connected-readiness gate.
- The Argo demo now reaches live import, but the Arnie fixture Applications are
  still degraded in the render path because the renderer cannot fetch their
  non-real Git sources.
- The two import how-tos now point readers at the local proof harness examples,
  but they still do not function as proof harnesses themselves.
- The demos still rely on `demo.sh` as the narrated human walkthrough, with
  `setup.sh` and `verify.sh` layered alongside it rather than replacing it.

### What is not yet proved

- A kept-alive Argo or Flux run that satisfies the connected-readiness
  threshold in `verify-connected-demo.sh`.
- The published doc does not yet expose the same scan/finding proof paths as
  the local Argo and Flux demos.
- The published doc does not yet expose the current demo drift explicitly:
  import/render evidence can exist without a passing connected-readiness proof.

## Recommended Next Slice

Close the remaining proof gap before broadening claims:

1. keep the bounded connected-preview gate and the Argo retrying HTTPS token path landed in the demos
2. treat controller auth as its own readiness phase in the Argo demo, because fresh-cluster `argocd-server` reachability is now the main blocker before `cub gitops import`
3. reduce verifier dependence on local worker PID files so `verify.sh` can discover ConfigHub evidence from live state, not only from local breadcrumbs
4. then revisit the remaining fixture questions: whether to filter the non-renderable Arnie Applications and whether any stronger connected-readiness threshold is still worth enforcing
5. only after that, update the published doc and local how-tos to describe the final proved path

## AI-First Lessons From Live Runs

- Bound broad discovery previews. A verifier should not hang forever on
  `cub-scout import --dry-run --json`; treat it as a bounded secondary signal,
  not the primary readiness gate.
- Separate bootstrap from proof. Fresh-cluster controller install and image
  pull behavior are noisy enough that setup needs its own degraded-but-explicit
  branch before the proof harness starts making stronger claims.
- Treat controller auth as its own readiness phase. In Argo, a cluster can
  exist, workloads can exist, and `cub-scout` can inspect state while the
  ArgoCD session endpoint is still not usable for `cub gitops import`.
- Avoid proof paths that depend on ephemeral local breadcrumbs. The worker PID
  file is useful for cleanup, but it should not be the only way a verifier
  discovers whether ConfigHub evidence is available.
- Show progress during long waits. Sequential `kubectl wait` calls and
  controller-auth retries are much easier for humans and agents to reason
  about when each bounded wait has visible progress and a named purpose.
