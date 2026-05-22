# Reference: source-truth strategies

The closed enum of nine **delivery strategies** cub-scout's `compare source-truth` contract knows about. Each strategy declares an expected ConfigHub / controller / runtime chain and the canonical authority for cross-surface equality.

The principle, locked in via `#393` and `#418`: **cub-scout never infers a strategy**. The operator (or an acceptance kernel like Pilot) declares the strategy via `--strategy <name>`; cub-scout then produces a *strategy-relative* verdict. Identical observations can produce opposite verdicts under different declared strategies — see [`confighub-source-truth`](../confighub-source-truth/SKILL.md) for the rule.

Source of truth: `pkg/agent/source_truth.go` — the constants below are the verbatim enum values, and `AllStrategies()` returns the closed list. Adding a strategy is a `#393`-class issue + a code change with full Phase-1/Phase-2 evaluator coverage.

## The nine strategies

| Strategy (CLI) | Human form (`DeclaredStrategy`) | ConfigHub | Controller | Runtime | Anchor (cross-surface equality) |
|---|---|---|---|---|---|
| `confighub-oci-argo` | ConfigHub -> OCI -> Argo -> Kubernetes | required (unit + space + revision) | required (Argo, observing an OCI source) | required (live image + health) | (none in v0.2 — ConfigHub does not yet expose a per-revision rendered digest; tracked) |
| `confighub-oci-flux` | ConfigHub -> OCI -> Flux -> Kubernetes | required | required (Flux Kustomization, OCIRepository source) | required | (same gap as above) |
| `git-argo` | Git -> Argo -> Kubernetes | required (unit pointed at a Git path) | required (Argo, git source) | required | **Git SHA** (controller `revision_or_digest` vs runtime image-tag-derived) |
| `git-flux` | Git -> Flux -> Kubernetes | required | required (Flux Kustomization, GitRepository source) | required | **Git SHA** |
| `helm-argo` | Helm -> Argo -> Kubernetes | required | required (Argo Helm renderer) | required | **Helm chart anchor** — runtime extractor partial (`runtime.helm_chart_anchor` proof gap until chart-version plumbed in) |
| `helm-flux` | Helm -> Flux -> Kubernetes | required | required (Flux HelmRelease, HelmRepository source) | required | **Helm chart anchor** (same gap) |
| `kustomize-flux` | Kustomize -> Flux -> Kubernetes | required | required (Flux Kustomization, any source) | required | **Git SHA** |
| `oci-argo` | OCI -> Argo -> Kubernetes | required | required (Argo, OCI source — NOT ConfigHub-rendered) | required | **OCI digest** |
| `oci-flux` | OCI -> Flux -> Kubernetes | required | required (Flux, OCIRepository source — NOT ConfigHub-rendered) | required | **OCI digest** |

CLI parsing accepts the lowercase machine name (`git-argo`); the `Human()` rendering on `SourceTruthStrategy` is what populates the `declared_strategy` field on the JSON evidence body.

## Strategy-relative correctness

This is the central rule of the source-truth contract. The same three-surface observation gets opposite verdicts under different strategies.

Example: an Argo CD Application reading from a Git URL.

| Strategy | Verdict | Why |
|---|---|---|
| `git-argo` | `status: PASS`, `source_truth: AGREED` (or BLOCK/MISMATCH on actual divergence) | Git is the declared source; Argo reading Git matches the strategy. |
| `confighub-oci-argo` | `status: BLOCK`, `source_truth: MISMATCH`, `outlier: controller` | Strategy declares Argo should pull a ConfigHub-rendered OCI artifact; reading Git is the strategy violation. |

cub-scout grades the deployment **against the declared strategy**, not in the abstract. That's why `--strategy` is mandatory: there is no "default" verdict.

Locked by tests:
- `TestDerive_StrategyMismatch_ControllerOutlier` — Argo reading Git under `confighub-oci-argo` returns BLOCK with controller as the outlier
- `TestDerive_VanillaGitOps_PASS` — same Argo reading Git under `git-argo` returns PASS

## Four statuses + five verdicts

Two separate axes on the same evidence body. See [`scout-compare`](../scout-compare/SKILL.md) and [`confighub-source-truth`](../confighub-source-truth/SKILL.md) for usage.

| Axis | Type | Values | Meaning |
|---|---|---|---|
| `status` | `SourceTruthStatus` (`pkg/agent/source_truth.go`) | `PASS` / `WATCH` / `BLOCK` / `ASK` | cub-scout's evidence-quality verdict |
| `source_truth` | `SourceTruthVerdict` | `AGREED` / `MISMATCH` / `INCOMPLETE` / `BLOCKED` / `UNKNOWN` | Cross-surface agreement under the declared strategy |

The pair (status, source_truth) can combine in non-obvious ways:

| Pair | Example |
|---|---|
| `(PASS, AGREED)` | Happy path — all three surfaces line up, no proof gaps. |
| `(WATCH, AGREED)` | Surfaces agree; soft proof gap (e.g., missing controller revision_or_digest). |
| `(WATCH, MISMATCH)` | Surfaces disagree, BUT one of the divergent fields is itself a proof gap so cub-scout doesn't escalate to BLOCK. |
| `(BLOCK, MISMATCH)` | Hard rule failure — strategy declares one thing, evidence shows another. |
| `(BLOCK, BLOCKED)` | A required surface couldn't be fetched at all (kubeconfig unreachable, controller CLI missing). |
| `(ASK, UNKNOWN)` | cub-scout cannot classify deterministically. Strategy is missing or surfaces fundamentally can't be compared. |

The strict missing-proof rule, locked by `TestDerive_NeverPASS_OnAnyMissingProof`: **any missing critical field forces at least `WATCH` / `INCOMPLETE`**. PASS+AGREED requires every required surface present and every required equality holding.

## The outlier

`SourceTruthOutlier` names which of the three surfaces diverges from the strategy-implied authority. Values:

- `confighub` — ConfigHub side is the outlier (e.g., the unit's declared image doesn't match what Argo actually pulled)
- `controller` — controller is the outlier (e.g., Argo is reading from a different source than ConfigHub said)
- `runtime` — runtime is the outlier (e.g., live cluster has a different image than what Argo applied — `kubectl-edit` territory)
- `import_render` — the rendered OCI artifact disagrees with the unit's declared data (caught by `#395` producer fixtures; v0.1 emits as `unknown`)
- `unknown` — verdict is AGREED, BLOCKED, or UNKNOWN, no specific outlier to name

The outlier is **derived** from the strategy-relative comparison, not declared by the caller.

## Proof gaps (`proof_gaps[]`)

Stable-string keys naming what cub-scout couldn't fully verify, even on PASS / WATCH outcomes. The list is the contract surface Pilot pattern-matches on. Common gaps:

| Gap key | Strategy | Meaning |
|---|---|---|
| `declared_strategy` | (ASK/UNKNOWN only) | Empty / unknown strategy was passed |
| `confighub` | any | ConfigHub surface absent (no `confighub.com/UnitSlug` label, or `cub-scout` not in connected mode) |
| `controller` | any | Controller surface absent (no Argo/Flux tracer evidence) |
| `runtime` | any | Runtime surface absent (live read failed) |
| `controller.revision_or_digest` | any | Controller reported the source URL but not the revision/digest it observed |
| `controller.multi_source` | argo strategies | Argo `spec.sources[]` has len > 1; cub-scout uses index 0 — equality across other sources unverifiable |
| `runtime.helm_chart_anchor` | `helm-argo`, `helm-flux` | Helm chart version not extractable from runtime; runtime extractor is partial (Phase 2) |
| `confighub.rendered_digest` | `confighub-oci-*` | ConfigHub does not yet expose a per-revision rendered digest; per-field digest equality unshipped |

Proof gaps are explicit non-claims. They're the contract's equivalent of receipt `omissions[]`: known gaps that don't fail the verdict but are surfaced so consumers know what's missing.

## Helper functions in `pkg/agent/source_truth.go`

| Function | Use |
|---|---|
| `AllStrategies()` | Returns the closed list of nine strategies |
| `ParseStrategy(s string) (SourceTruthStrategy, bool)` | Lowercases + trims input; second return `false` for empty/unknown |
| `s.Human() string` | Returns the chain rendering for the `declared_strategy` JSON field |
| `s.expectsConfigHubOCISource() bool` | True for `confighub-oci-*` only; used by the controller-source rule |
| `s.ExpectsArgoController() bool` | True for the four `*-argo` strategies; used by the tracer-pick step |

These functions are the API surface for any caller building source-truth evidence (the source-truth CLI, the source-truth-pass predicate, the `compare source-truth` MCP tool, and Pilot).

## CLI surface

```bash
$ cub-scout compare source-truth --strategy
Usage: cub-scout compare source-truth <kind>/<name> -n <ns> --strategy <name> [--format json]

  --strategy supports:
    confighub-oci-argo   ConfigHub -> OCI -> Argo -> Kubernetes
    confighub-oci-flux   ConfigHub -> OCI -> Flux -> Kubernetes
    git-argo             Git -> Argo -> Kubernetes
    git-flux             Git -> Flux -> Kubernetes
    helm-argo            Helm -> Argo -> Kubernetes
    helm-flux            Helm -> Flux -> Kubernetes
    kustomize-flux       Kustomize -> Flux -> Kubernetes
    oci-argo             OCI -> Argo -> Kubernetes
    oci-flux             OCI -> Flux -> Kubernetes
```

The help text is dynamically built from `AllStrategies()` (verified by drift test in `cmd/cub-scout/source_truth_help_test.go`, the fix from `#450` / `#453`). If a strategy is added in the Go code, the help text updates automatically; if the help text is hand-edited to remove one, the test fails.

## Connected mode required

`compare source-truth` refuses to run without `cub auth login`. Standalone-mode verdicts are meaningless: the ConfigHub surface is unfetchable, and the strategy-relative rule has nothing to compare against. The error message points at auth, not at the verb.

The receipt-side complement, `cub-scout receipt verify --strategy <s>`, inherits the same requirement — see [`scout-verify`](../scout-verify/SKILL.md).

## Skills that consume this reference

- [`scout-compare`](../scout-compare/SKILL.md) — the four Compare verbs, including `compare source-truth`
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — the strategy-typed workflow
- [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) — fleet-wide composition over per-resource source-truth
- [`scout-verify`](../scout-verify/SKILL.md) — `source-truth-pass` predicate wraps the verdict into a receipt
- [`observe-flux`](../observe-flux/SKILL.md) — the `helm-flux` / `kustomize-flux` runtime proof gaps
- [`observe-helm`](../observe-helm/SKILL.md) — the three-ways-to-be-Helm disambiguation feeds strategy picking

## References

- Code: `pkg/agent/source_truth.go` (types + constants + helpers), `pkg/agent/source_truth_logic.go` (`Derive` — the pure decision function)
- Fixtures: `pkg/agent/source_truth_fixtures_test.go` + `test/fixtures/source-truth/` (the locked golden suite from `#404`)
- Producer fixtures: `#395` / `#400` / `#404`
- Phase 1 (initial 4 strategies): `#393`
- Phase 2 (expansion to 9): `#418`
- Phase 3 (multi-source Argo): tracked separately
- Help-text drift fix: `#450`, PR `#453`
- Connected-mode gate: `pkg/hub.QuickMode()` in `cmd/cub-scout/source_truth.go`
