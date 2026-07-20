# cub-scout Handover for the Next AI Coder

Last updated: 2026-07-09 — captures the post-#500 live delivery observability release slice merged at `ee84f01`. Current release tag is `v2.6.0`; `v2.7.0` is the next release candidate and release notes live at [`docs/releases/v2.7.0.md`](docs/releases/v2.7.0.md). The May 2026 receipts and skill-catalog sections below remain historical context.

## Current repo state

- Branch: `main`
- Canonical roadmap: `docs/roadmap.md`
- Delivery rules: `docs/workflows/agent-milestone-plan.md`
- First repo-specific AI entrypoint: `AI-README-FIRST.md`
- Canonical CLI doc layout:
  - `README.md` = overview + Fast Path
  - `CLI-GUIDE.md` = workflow-first guide
  - `docs/reference/cli-reference.md` = A-Z command catalog
  - `docs/reference/commands.md` = detailed usage and examples
  - `docs/reference/cli-contract.md` = stable flags, exit codes, and schemas

## July 2026 update — live delivery observability (`#500`)

`#500` is merged and is the release slice after `v2.6.0`. It adds the README user-question table, generation-aware current-change evidence on diagnostic surfaces, aggregate delivery-resource support, audited action events, and the live-delivery how-to/example fixture.

| PR | Subject |
|----|---------|
| [#500](https://github.com/confighub/cub-scout/pull/500) | Live delivery observability docs and implementation slice: user-question framing, `currentChange`/`rollouts` evidence on `doctor`/`explain`/`compare three-way`, first-class `fluxcd.controlplane.io/v1` aggregate resources, audited action event parsing, live-delivery fixture, operator how-to, v2.7.0 release notes, and CI hardening follow-ups. |

Design decisions from this slice:

- Diagnostic surfaces share the rollout progress/verdict shape, but receipt-specific desired-set and grace-window gates remain receipt-specific.
- Aggregate resources are first-class for discovery, ownership, deployer/status views, `gitops status`, direct trace lookup, and activity timelines. Deeper source-to-generated lineage and top-level `doctor` findings remain follow-ups.
- Action event parsing preserves raw evidence and omits missing fields. It does not infer actor, subject, groups, or action when the Event does not carry the recognized annotations.
- The low-load repeated-question path points to existing captured, watched, summarized, or receipt evidence. Broader API-load-aware freshness metadata is still future work.

## May 2026 completions — session 2026-05-25 (receipts v2 closure + Pilot consumer skills)

Six PRs landed on top of the morning `#465` handover refresh, **closing `#444`, `#448`, and `#449` end-to-end**. The receipts arc is now feature-complete; the consumer-side Pilot skill catalog is shipped across two batches.

| PR | Subject |
|----|---------|
| [#466](https://github.com/confighub/cub-scout/pull/466) | `#444` batch A — 5 Pilot–cub-scout integration scenario skills (`pilot-cd-gate`, `pilot-fleet-conformance`, `pilot-patch-and-drift`, `pilot-watch-alert-response`, `pilot-incident-evidence`); the consumer-side complement to the `#442` verb-group / workflow skills. 1562 lines docs. |
| [#467](https://github.com/confighub/cub-scout/pull/467) | Codex round-7 P1/P2/P3 fixes on `#466`: 3 P1s + 6 P2s + 1 P3 — broad `compare *` allowed-tools tightened (legacy `compare --suggest --apply` was mutating); strategy enum corrected (9 strategies; `git-helm`/`oci-helm` weren't real); invented `pilot decide --evidence` CLI removed; source-truth JSON shape `strategy`/`sourceTruth`/`proofGaps` → `declared_strategy`/`source_truth`/`proof_gaps` per `pkg/agent/source_truth.go:280-287`; watch receipt fixture `"version":"1"` → `"v1"`; receipt verdict table conflated with source-truth `ASK` (receipt verdicts are PASS/WATCH/BLOCK/INCONCLUSIVE only); invented "WATCH ceiling" standalone wording removed; "five cause values" reframed as Pilot policy cases × 3 cause values; specific mutation verbs abstracted. |
| [#468](https://github.com/confighub/cub-scout/pull/468) | `#444` batch B — 4 governance Pilot scenarios (`pilot-rollback-decision`, `pilot-promotion-gate`, `pilot-compliance-audit`, `pilot-release-verification`). Codex round-7 learnings applied upfront. **Closed `#444`** (9 scenarios total). 1107 lines docs. |
| [#469](https://github.com/confighub/cub-scout/pull/469) | `#448` aggregate-with-discovery half — `cub-scout receipt verify --scope namespace/<ns>` auto-discovery + comma-list batch + `synthetic-aggregate://sha256/<id>` subject + max-severity verdict synthesis policy + `--aggregate-policy` knob + aggregate-partial-coverage omission. 31 new tests; 1862 insertions. The chained half shipped in `#463`. **Closed `#448`** (only GUAC graph-ingestibility verification, an external test, remains as deferred non-goal). |
| [#470](https://github.com/confighub/cub-scout/pull/470) | `#449` event-type expansion + per-poll backpressure — `resource.discovered` and `scan.finding` now build receipts (gated by the new cap). `--emit-receipt-batch-cap N` (default 10): first N builds, rest emit with `receipt` key omitted + single stderr summary. Forward-compat startup warning preserved for future unsupported event types. 5 new tests + 3 updated. **Closed `#449`**. |
| [#471](https://github.com/confighub/cub-scout/pull/471) | Cleanup + 3 new worked examples: `examples/receipts/chained/`, `examples/receipts/aggregate/`, `examples/receipts/watch-emit/` (companion to the existing `examples/receipts/ci-gate/` from `#464`). Pruned 36 stale local branches in this worktree. |

### Key design decisions locked in this session

- **Aggregate-receipt subject is order-independent.** `BuildSyntheticAggregateSubject` sorts input digests by sha256 hex before concatenation, so re-running the verify with inputs in a different order produces the same `synthetic-aggregate://sha256/<id>` subject (the subject is set-shaped). Note: `predicate.inputAttestations[]` is emitted in caller order, so the aggregate's full **receipt-level fingerprint** is order-dependent — only the subject digest is set-shaped today. A future correctness pass may sort the wire array before stamping so the receipt-level fingerprint also becomes set-shaped; the current contract only guarantees order-independence for the subject.
- **`PredicateAggregateVerdict = "aggregate-verdict"`** is a new predicate distinct from the per-resource predicates. Aggregates don't evaluate evidence; they synthesize verdicts over input receipts. The per-resource `evidence` body is empty on the aggregate; the consumer drills into `inputAttestations[]` for per-resource detail.
- **Max-severity is the only synthesis policy in v1.** The `AggregateVerdictPolicy` interface + `--aggregate-policy` flag are wired for future policies (majority, weighted); shipping one policy intentionally keeps the review surface narrow.
- **API-boundary verify enforced on aggregates too.** `BuildAggregateReceipt` rejects zero-value `VerifiedAttestationRef` entries — same Codex round-6 P1 guard as `BuildReceipt`. Programmatic callers cannot forge inputs into the aggregate.
- **Per-poll backpressure, not across-poll.** `watchReceiptBatchCap` resets every call to `attachReceiptsIfRequested`. A long-running watch with quiet polls between bursts doesn't accumulate suppression state.
- **`--emit-receipt-batch-cap 0` disables receipt-build while keeping the flag explicit.** Negative values rejected upfront. Mirrors the pattern of `--fail-on PASS` rejection.
- **Forward-compat warning preserved.** Even though all 4 known event types are now supported, the `unsupportedEmitReceiptTypes` helper stays in place as a safety net: a future event type added to `watchKnownEventTypes` without receipt-build support will trigger the startup warning automatically.
- **Pilot mutation paths abstracted in skills.** Codex round-7 P2 found pilot-patch-and-drift naming `argocd app sync`, `flux reconcile`, `cub unit apply`. Per issue scope discipline (mutation is Pilot's, not cub-scout's), all batch B skills abstract as "Pilot's REVERT path / intent-update path; mutation lives outside cub-scout skills."
- **Receipt-verdict enum is 4-valued.** PASS/WATCH/BLOCK/INCONCLUSIVE. Source-truth `ASK` is a separate enum value on `compare source-truth --status`; when wrapped in a receipt, ASK maps to receipt `WATCH` (per `pkg/agent/receipt_predicates.go:396-407`). The Codex round-7 P2 finding caught this conflation in pilot-incident-evidence.

### Codex round-7 outcome

External Codex review of `#466` returned **3 P1s + 6 P2s + 1 P3** — all substantive with file:line citations against shipping code. All 10 landed in `#467`. Round-7 learnings applied upfront in batch B `#468` so the same review cycle didn't re-fire.

## May 2026 completions — session 2026-05-22 (receipts v1 + AI-agent skills)

Two major issue trees closed this session: `#446` (Verify / Receipt capability) v1 and `#442` (Comprehensive skills coverage). Both arrived in their full scope plus a Codex round-5 review fix-up.

### `#446` Receipts v1 — complete

Typed, fingerprinted, immutable evidence artifacts wrapping cub-scout's existing field-level evidence into a verifiable record. Wire format is the in-toto Statement v1 envelope wrapping `https://cub-scout.dev/receipt/v1`. Fingerprint is SHA-256 over RFC 8785 canonical JSON of the full Statement minus only `predicate.fingerprint` (delete-the-key, not zero-it; per Codex round-4 + round-5).

| PR | Subject |
|----|---------|
| [#454](https://github.com/confighub/cub-scout/pull/454) | Foundation — types, canonical JSON via `gowebpki/jcs`, fingerprint, dual subjects (`k8s-live://` + `confighub-unit://`), `applied-matches-spec` predicate, ASCII renderer, `verify` subcommand, 4 example receipts |
| [#455](https://github.com/confighub/cub-scout/pull/455) | Predicates batch 2 — `source-truth-pass` + `no-manual-edits-since`; `--strategy` / `--since` flags; `collectSourceTruthForReceiptFn` seam; 4 more example receipts |
| [#456](https://github.com/confighub/cub-scout/pull/456) | Management UX — `receipt show / validate / list`; local store at `$CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME/cub-scout/receipts → $HOME/.local/share/cub-scout/receipts`; `--save` flag |
| [#461](https://github.com/confighub/cub-scout/pull/461) | Codex round-5 fix-up — ASK → WATCH (locked design); `O_EXCL` atomic save + `--out` rejects store paths; exit codes 0/1/2 via `errors.As`; mutating-wildcard scrub in 4 skills; public repo-path leak scrub |

Read-only-triad guards: `TestReceiptPackageReadOnlyClient` static-greps receipt source files for any mutating K8s client method; `FilterNextSteps` drops 18 mutating-verb fragments at emit time. `#446` parent issue closed.

### `#442` AI-agent skill catalog — complete

Modeled on [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) for `cub`. Five PRs total; ~35 skill files plus 9 references:

| PR | Subject |
|----|---------|
| [#452](https://github.com/confighub/cub-scout/pull/452) | Batch 1 — scaffolding (`SKILL_TEMPLATE.md`, router, umbrella) + 4 verb-group skills (Observe/Diagnose/Compare/Attribute) + 2 references |
| [#457](https://github.com/confighub/cub-scout/pull/457) | Batch 2 — 4 more verb-group skills (Ingest/Govern/Integrate/Verify) |
| [#458](https://github.com/confighub/cub-scout/pull/458) | Batch 3 — 7 controller-observer skills (argocd/flux/helm/crossplane/kro/confighub-managed/native), each verified against `pkg/agent/ownership.go` + `pkg/agent/manager_strings.go` |
| [#459](https://github.com/confighub/cub-scout/pull/459) | Batch 4 — 8 workflow scenario skills (triage / drift / fleet / adopt / migrate / agent-context / incident / source-truth) |
| [#460](https://github.com/confighub/cub-scout/pull/460) | Batch 5 — 7 remaining shared references; closes `#442` |

### Key design decisions locked this session

- **Receipts are evidence, never recommendations to mutate.** `FilterNextSteps` rejects `actionType=mutating` + a long list of mutating-command fragments at emit time. Documented in `references/read-only-triad.md`.
- **Receipt store is immutable by construction.** `SaveStatement` uses `O_EXCL` atomic create; the on-disk filename is canonical (`<verifiedAt>__<predicate>__<kind>-<name>__<short-fingerprint>.receipt.json`); duplicate saves return `os.ErrExist` and never overwrite. `--out` refuses to write under the resolved store path.
- **Fingerprint covers the full Statement.** Not just the predicate body — `_type`, `subject`, `predicateType`, and every predicate field other than `fingerprint` itself are in scope. Hashing predicate-only would leave the envelope unprotected.
- **ASK → WATCH (not INCONCLUSIVE).** Per the locked synthesis in `docs/proposals/receipts-way-forward.md`; INCONCLUSIVE is reserved for receipts that themselves can't be built (missing strategy, missing evidence body), not for cases where the source-truth surface couldn't classify.
- **Skill `allowed-tools` enumerate read-only verbs only.** No broad `Bash(cub-scout *)` / `Bash(./cub-scout *)` wildcards anywhere — those would silently grant `demo`, `import apply`, `compare --suggest --apply`, all mutating. Verified clean across all ~35 skill files (Codex round-5 caught 4 leaks, all fixed).

### Open follow-ons from this session (now resolved or progressed)

Most of these moved in the afternoon session below; this list is preserved for traceability:

- ~~**`#446` v2 spin-offs**~~ — `#451` closed via #463 (fully shipped); `#448` and `#449` progressed in #463 (chained half + v1 watch surface shipped; aggregate-with-discovery and backpressure remain).
- ~~**MCP enum drift**~~ — still open (one-file fix in `cmd/cub-scout/mcp.go`). Did not land this session.
- ~~**Codex P3**~~ — source-truth receipt precedence tests still open; not blocking.
- **`#444` Pilot–cub-scout integration skills** — still open; the consumer-side scenarios remain the highest-leverage track after receipts.

## May 2026 completions — session 2026-05-22 (afternoon: receipts v2 + Codex round-6 + docs visibility)

Three PRs landed on top of the morning #462 refresh, in deliberate merge order: docs-drift (#462) → code (#463) → docs-visibility (#464). The afternoon arc closes the receipts capability end-to-end and fills the top-of-funnel docs gap that #462 itself surfaced.

| PR | Subject |
|----|---------|
| [#462](https://github.com/confighub/cub-scout/pull/462) | Docs-drift refresh — HANDOVER + AI-README-FIRST + v2.0.0-plugin-plan updated to v2.2.1 reality; capability map gained receipts + skills; "blocker" framing scrubbed (v2.0.0 shipped long ago). Codex round-6 P1/P2 followups: `confighubai/confighub-ai-demo` scrubbed (confidentiality boundary); Views #419/#420 scope-citation drift corrected (scope #3 shipped via #420, not scope #2). |
| [#463](https://github.com/confighub/cub-scout/pull/463) | Receipts v2 surface — three additive extensions on the v1 envelope: `receipt verify --fail-on <verdict>` (`#451`), `--input-attestation <path>` for chained receipts via `inputAttestations[]` (the explicit half of `#448`), and `watch --emit-receipt-on <event-types>` (v1 of `#449`). 33 new tests; wire format unchanged. Codex round-6 P1/P2 fixes squashed in (commit `56d9123` on the branch): read-only guard now scans `watch_receipt.go` via substring glob + expected-paths assertion; `VerifiedAttestationRef` typed wrapper enforces fingerprint verification at the `BuildReceipt` API boundary (programmatic callers can't bypass the verify step); `--fail-on` parses upfront so a bad value can't leak stdout/--out/--save; `omitempty` on `watchEvent.Receipt` reconciled with docs; startup warning when `--emit-receipt-on` includes unsupported types; rate-limited stderr warnings (first 10 + summary every 100); stale "v2 adds DSSE" wording softened across `receipt.go`, `json-contracts.md`, `commands.md`, `scout-verify/SKILL.md`. |
| [#464](https://github.com/confighub/cub-scout/pull/464) | Docs visibility — the Verify verb group was invisible in the top-level navigation (README capability map had 7 groups, not 8; cli-reference had only the top-level `receipt` row, no subcommands; cli-contract had no receipt section; examples/receipts had no v2 surface section). Surfaced in 6 files including a full GitHub Actions / GitLab CI / Jenkins CI-gate walkthrough at `examples/receipts/ci-gate/README.md`. Closes the docs gap noted in the #451 closing comment. |

### Codex round-6 review — outcome

External Codex review of #462 + #463 returned 3 P1s and 5 P2s. All landed in the same merge cycle:

| Finding | Status |
|---|---|
| P1 #463-A: read-only guard `TestReceiptPackageReadOnlyClient` didn't scan `watch_receipt.go` | Fixed — substring glob + `expectedPaths` allow-set asserts every receipt-shaped file is scanned |
| P1 #463-B: `BuildReceiptInput.InputAttestations []AttestationRef` allowed programmatic forgery (bypass of `BuildAttestationRef`'s verify step) | Fixed — typed wrapper `VerifiedAttestationRef` with unexported field; only constructible via `BuildAttestationRef(s)`; BuildReceipt rejects zero-valued wrappers |
| P1 #462: `confighubai/confighub-ai-demo` + "Slack discussion" provenance leaked into public docs | Fixed — scrubbed; generic "tracked in a separate non-public repo" pointer |
| P2 #463: `--fail-on` parsed after stdout/`--out`/`--save` (CI footgun on bad value) | Fixed — parses upfront in `runReceiptVerify` |
| P2 #463: docs said `receipt: null` but `omitempty` actually omits the key entirely | Fixed — keep `omitempty`, update docs + doc-comments to "key omitted; check key presence" |
| P2 #463: silent skip for `--emit-receipt-on` event types that don't yet build a receipt (`resource.discovered`/`scan.finding`) | Fixed — one-time startup stderr warning lists them at watch start |
| P2 #463: unbounded watch receipt-build warnings could spam stderr on transient cluster-read failures | Fixed — `makeReceiptWarnFn` rate limiter: first 10 verbatim, then summary every 100 |
| P2 #463: stale "v2 adds DSSE signing wrapped in Sigstore Bundle v0.3" wording presented future signing as inevitable | Fixed — softened to "future hardening direction" across `receipt.go`, `json-contracts.md`, `commands.md`, `scout-verify/SKILL.md`, `examples/receipts/README.md`, `docs/roadmap.md` |
| P2 #462: Views #419/#420 scope-citation drift (claimed #419/#420 shipped scope #2; reality: #419 closed unmerged, #420 merged scope #3) | Fixed — #420 → scope #3 shipped; #419 removed from references; scope #2 (open) tracked under #422 |

### Key design decisions locked this session (afternoon)

- **Typed wrapper for chained-receipt API boundary.** `VerifiedAttestationRef` is a struct with one unexported field (`ref AttestationRef`). External packages cannot construct a non-zero value without calling `BuildAttestationRef` (which verifies the source Statement's fingerprint). The zero value is the entire forgeable surface, and `BuildReceipt` rejects it explicitly. Wire format is unchanged — `Predicate.InputAttestations` stays `[]AttestationRef` for the JSON shape; `BuildReceipt` unwraps internally.
- **`--fail-on PASS` rejected upfront.** Gating on a passing receipt is a workflow bug, not a feature. The CLI errors with exit 1 (operational), not 2 (gate fired). Documented in `cli-contract.md`.
- **Artifact preservation on `--fail-on` exit 2 is intentional and load-bearing.** The receipt is printed to stdout / written to `--out` / saved to `--save` BEFORE the gate evaluates. CI gets both the failure code AND the durable evidence artifact. Codex round-6 P2 tightened this: a bad `--fail-on` value rejects upfront BEFORE any side effect, so there's no path where a misconfigured gate leaks an artifact.
- **`omitempty` on `watchEvent.Receipt` over `null` in the wire format.** Consumers must check for key presence (`"receipt" in event`), not null-ness. The doc comments now state this explicitly with an idiomatic Python example.
- **Watch `--emit-receipt-on` is lenient on type, strict on receipt-build.** Unsupported types pass through silently (so `--emit-receipt-on all` is forward-compat as new event types land), BUT a startup warning lists the unsupported subset so users don't silently get less than they asked for.
- **Receipt-build warnings rate-limited, not unbounded.** First 10 pass through; afterwards a periodic summary fires every 100 suppressions. Counters live on the closure across poll cycles. Disable with knob = 0.
- **Read-only triad guard tightened.** The static-grep static guard now uses a substring glob (`strings.Contains(name, "receipt")`) rather than a prefix glob (`strings.HasPrefix`), and asserts an explicit allow-set of receipt-shaped basenames. A future receipt file with a non-prefix basename (e.g., `aggregate_receipt.go`) is covered automatically; a rename that silently drops a file is a loud test failure.
- **Docs visibility is its own deliverable.** Receipts shipped across five PRs; the docs followed in the same PRs but only on per-feature surfaces (`json-contracts.md` § Receipt Contract, `commands.md` § receipt, per-predicate `examples/receipts/<predicate>/`). The top-of-funnel surfaces (README capability map, cli-reference A-Z, cli-contract exit codes, examples/receipts top-level README, roadmap) never got a refresh until #464.

### Open follow-ons from this afternoon session

- **`#448` aggregate-with-discovery half** — `cub-scout receipt verify --scope namespace/<ns>` auto-discovery + `synthetic-aggregate://` subject + max-severity verdict synthesis. Issue stays open.
- **`#449` backpressure / batching** — high-frequency event handling + `resource.discovered` / `scan.finding` support gated on the backpressure design. Issue stays open.
- **MCP `compare_source_truth` enum drift** — still open (one-file fix in `cmd/cub-scout/mcp.go`).
- **Codex round-5 P3** — source-truth receipt precedence tests still open; not blocking.

## May 2026 completions — session 2026-05-21 (attribution layer)

The full attribution layer (`#435`) shipped in four squash-merged PRs. Every `compareFieldMismatch` now carries `cause`, `managerHint`, `gitSource`, and (when connected) `bindingSource` — answering "which controller, or which human, last touched this field, what's it sourced from in git, and which ConfigHub Link supplies it?" all in one read-only evidence record.

| PR | Stages | Subject |
|----|--------|---------|
| [#437](https://github.com/confighub/cub-scout/pull/437) | A1 + A1.5 + A2 | `managedFields` classifier (`controller-drift` / `manual-edit` / `unknown`) co-signaled with detected owner; verified manager-string enumeration (Argo CD / Flux kustomize+helm+source / Helm direct / Crossplane composite+composed+claim+MRD+refs / kro applyset+parent+labeller / `kubectl-*`); per-field-path resolution via `FieldsV1` decoding (`sigs.k8s.io/structured-merge-diff/v4/fieldpath`); resource-level git anchor (`repoUrl` / `revision` / `path`) via existing Argo/Flux tracers. |
| [#438](https://github.com/confighub/cub-scout/pull/438) | C1 | Incoming-binding introspection via `cub link list -o json`; `compareLinkRunner` injection seam; `IncomingBinding` + ASCII/Markdown rendering. |
| [#439](https://github.com/confighub/cub-scout/pull/439) | C2 | Per-field `bindingSource` on each mismatch; `expandBindings` handles array + object Bindings JSON shapes plus `target/source/transform` aliases; ASCII renders `<- bound from unit:... path:... via link:...` under the diff line. |
| [#440](https://github.com/confighub/cub-scout/pull/440) | Stage B | Raw-YAML `gitSource.file:line` back-resolution via opt-in `--source-path <local-checkout>`; `BackResolveGitSource` walks YAML/YML files with `gopkg.in/yaml.v3` line tracking, matches by `kind`/`name`/`namespace`, navigates dotted canonical paths. Helm/Kustomize templating explicitly deferred. |

### Key design decisions locked in this track

- **Verified manager-string enumeration, not guessed**: every string in `pkg/agent/manager_strings.go` has an upstream citation. Unknown strings fall through to `unknown` rather than misclassifying — same `parse, don't guess` rule used by ownership detection.
- **Owner co-signal disambiguates kubectl-client-side-apply**: Argo CD's CSA migration default and `kubectl apply --client-side` write the same manager string. The classifier reads `pkg/agent/ownership.go` first so Argo-owned resources resolve as `controller-drift` while non-Argo resources resolve as `manual-edit`.
- **Crossplane composed children use prefix match**: `apiextensions.crossplane.io/composed-<hash>` varies per XR. Match is `strings.HasPrefix`, not exact.
- **Per-field rollup degrades cleanly to resource-level**: when `FieldsV1` is missing (older K8s or stripped), classification is resource-level (A1). When present, per-field (A1.5). Fields like `images` that span multiple container list items intentionally fall back to resource-level pending list-key selector support.
- **`cub link list` runner seam**: `compareLinkRunner` function variable lets tests inject prefab JSON, matching the `viewCubRunner` pattern in `views.go` (#414).
- **A2 anchor and B file:line are decoupled**: A2 surfaces `repoUrl`/`revision`/`path` automatically (tracer-driven). Stage B `file:line` is opt-in via `--source-path` to avoid surprising filesystem walks on the standard `compare three-way` run.
- **Doc + example land with the code**: `docs/reference/json-contracts.md` § Field Mutation Attribution Contract carries the full contract; `examples/drift/mutation-cause-attribution/` carries the operator-facing narrative with `controller-drift.json` + `manual-edit.json` fixtures.

## May 2026 completions — session 2026-05-09

| PR | Issue(s) | Subject |
|----|----------|---------|
| [#414](https://github.com/confighub/cub-scout/pull/414) | #408, #391 scope #1 | `--view` flag on `compare three-way`; `cubRunner` subprocess-injection seam; 7 new tests |
| [#411](https://github.com/confighub/cub-scout/pull/411) | #388 | Flake fix: startup grace `0.2s → 1s` in demo-worker lifecycle script |
| [#412](https://github.com/confighub/cub-scout/pull/412) | #385 | GitHub Actions Node 24 bump (checkout v6, setup-go v6, upload-artifact v7, download-artifact v8, golangci-lint v9, docker/login v4, goreleaser v7) |
| [#413](https://github.com/confighub/cub-scout/pull/413) | #389 | GoReleaser `brews:` → `homebrew_casks:` deprecation migration |

### Key design decisions locked in #414

- **Testability seam**: `viewCubRunner cubRunner` func var in `views.go` replaces all direct `exec.Command("cub", ...)` calls. Tests inject a fake runner returning prefab JSON; production delegates to the real binary. This seam also covers `views resolve` and any future `cub`-shelling code in the views layer.
- **`report.Scope` for views**: always `view/<uuid>` regardless of whether the input was a UUID or URL. Stable, compact, discriminable from `resource:`, `namespace/`, `cluster`.
- **Mutual exclusion**: `--scope` and `--view` produce a clear error if both are passed. `--view` requires connected mode; error message mirrors `views resolve`.
- **Triad audit finding**: `cmd/cub-scout/remedy.go` had a genuine architectural triad violation — it executed `kubectl apply / patch / delete` via `executor.Execute`. Resolved in #428 (option (b) per #410): rename to `suggest-remedy`, delete the executor path, drop `--dry-run`/`--force`/audit flags. cub-scout is now categorically read-only.

## May 2026 completions — council source-truth track

The council (April 2026) prescribed the **truth-floor → source-truth contract → fixtures** sequence. The full sequence is now in main:

| PR | Issue(s) | Subject |
|----|----------|---------|
| [#397](https://github.com/confighub/cub-scout/pull/397) | #387, #390, narrow #394 | Truth-floor: Flux false-pessimism fix, tree-patterns nil panic, kstatus narrow slice |
| [#398](https://github.com/confighub/cub-scout/pull/398) | #384 | Pre-existing main lint debt cleanup (5 dead funcs + 1 ineffassign) |
| [#399](https://github.com/confighub/cub-scout/pull/399) | — | Parity script reads canonical CLI reference (post-#374 doc move) |
| [#400](https://github.com/confighub/cub-scout/pull/400) | #393 | Source-truth evidence contract v0.1 (`compare source-truth`) |
| [#401](https://github.com/confighub/cub-scout/pull/401) | #396 | Direct integration coverage for `discoverWorkloads` / `getManagedResources` |
| [#402](https://github.com/confighub/cub-scout/pull/402) | #394 (rest) | kstatus migration complete (state_scanner.go) |
| [#403](https://github.com/confighub/cub-scout/pull/403) | #391 v0.1 | Views resolver with URL-as-positional convention (`views resolve`) |
| [#404](https://github.com/confighub/cub-scout/pull/404) | #395 v0.1 | Source-truth producer fixture suite (6 fixtures, byte-equal goldens) |

### Architectural triad locked in code (not just intent)

- **cub-scout** = read-only evidence provider for source truth
- **Pilot** = acceptance judge (separate consumer of cub-scout's surfaces)
- **ConfigHub** = authority and workflow engine

cub-scout never mutates, repairs, approves, or infers authority. The source-truth contract enforces this surface in code.

### Strict rules locked in tests

- **Strategy-relative correctness.** Identical observations get opposite verdicts under different declared strategies. Argo reading Git is `PASS` under `git-argo` and `BLOCK` (controller outlier) under `confighub-oci-argo`. Locked by `TestDerive_StrategyMismatch_ControllerOutlier` + `TestDerive_VanillaGitOps_PASS`.
- **Missing proof never produces PASS.** Any blank source/digest/runtime field forces at least `WATCH` / `INCOMPLETE`. Locked by `TestDerive_NeverPASS_OnAnyMissingProof` sweep.
- **Connected-mode gate.** `compare source-truth` and `views resolve` refuse to run without `cub` auth — both contracts are meaningless without the ConfigHub surface.

### Pre-existing main CI breakage (cleared)

Discovered while landing the truth-floor: main CI had been red since 2026-04-19, broken by two pre-existing issues unrelated to this work — `#384` lint debt and a parity-script path drift introduced by the `#374` doc consolidation. Both fixed in `#398` / `#399` so subsequent PRs inherit a green baseline.

## Recent completions

The connected/AI-first issue cluster from March 2026 is now closed:

| Issue | Title | Status |
|-------|-------|--------|
| #348 | Preserve status fields in scan/export path | Closed (Apr 2) |
| #329 | Make GitOps import and connected demos AI-first | Closed |
| #327 | Kubara-oriented Argo debugging guide | Closed |
| #326 | Refresh ADT docs with connected commands | Closed |
| #325 | Treat OSS SDK cmd/cub as source of truth | Closed |
| #324 | Align import proposals with app-centric model | Closed |
| #323 | Define canonical App/Deployment/Target mapping | Closed |
| #321 | Make ADT primary model across docs | Closed |
| #343-347 | Archive stale branch ideas | Closed (Apr 2) |

Key deliverables now in place:
- AI-first demo structure in `examples/argo-import-confighub-demo/` and `examples/flux-import-confighub-demo/`
  - `AI_START_HERE.md`, `prompts.md`, `contracts.md`, `verify.sh`
- Status-dependent scanner rules work correctly (#348 regression test added)
- App/Deployment/Target is the primary mental model across connected docs
- Deterministic `TRY NEXT` hints are stronger and more contextual (`cec20f8`, `#349`)
- `explain` suggests ConfigHub GUI deep-links when connected context provides a unit URL (`a5016ad`, `#350`)
- CLI color output with `NO_COLOR` support for doctor and explain commands (`2768e0e`, `#351`)
- Explicit `--presentation` flag for doctor and explain commands (`a5a5e59`, `#352`)
  - Three modes: `human`, `ai`, `paired`
  - Opt-in only: omit the flag to keep the legacy/default render path
  - AI mode uses uppercase markers, bracket notation, `RECOMMENDED ACTIONS` heading
- #342 bidirectional snapshot and conformance workflow (both slices)
  - Conformance reporting for import proposals
  - Curated import selection with include/exclude filtering
- #328 secrets — complete (`2346598`, `a6b5128`, `e527bdd`, `53e4ca0`)
  - Slice 1: Secret evidence in `trace` for workloads and Flux sources
  - Slice 2: Crossplane ProviderConfig with cross-namespace resolution
  - Slice 3: Secret issues in `map issues` output
  - Slice 4: TUI integration (secret panel in trace view)
  - Dynamic CRD discovery for any *.crossplane.io / *.upbound.io provider
  - Status classification: present, missing, unreadable, unresolved
  - RBAC-aware error detection (Forbidden vs NotFound)
  - Safe metadata only (never exposes .data or .stringData)
  - HelmRepository and Bucket coverage for Flux source secrets
  - Optional secrets correctly excluded from issues

## Open issues

Current tracked follow-ons (verified 2026-07-09; reflects post-#500 state):

**Recently closed arcs:**
- ~~**`#500`**~~ — live delivery observability release slice. **Closed via `#500`** on 2026-07-09.
- ~~**`#446`**~~ — Receipt capability parent. **Closed** (v1 in `#454`/`#455`/`#456`; v2 surface in `#463`/`#469`/`#470`).
- ~~**`#444`**~~ — Pilot–cub-scout integration skills, 9 scenarios. **Closed via `#468`** (batch B; batch A in `#466`, Codex round-7 fixes in `#467`).
- ~~**`#448`**~~ — Receipts v2 aggregate / chained receipts. **Closed via `#469`** (aggregate half; chained half shipped in `#463`).
- ~~**`#449`**~~ — `watch --emit-receipt-on` event types + backpressure. **Closed via `#470`** (full 4-event-type set + per-poll cap; v1 had shipped in `#463`).
- ~~**`#451`**~~ — `--fail-on RECEIPT_VERDICT` exit semantics. **Closed via `#463`** (Codex round-6 P2 tightened upfront parsing).

**Live delivery observability follow-ups from `#500`:**
- Aggregate delivery failures as top-level `doctor` findings where controller status refs expose source/generated-artifact lineage.
- Audited action events as history and receipt supporting evidence.
- Broader source-freshness metadata for snapshot, watch, summary, and receipt-backed reads.
- ConfigHub history-backed event / Argobot evidence integration with OCI release correlation, no production cursor sharing, and direct observer cursors only as fallback; see [`docs/proposals/event-consumer-argobot-integration.md`](docs/proposals/event-consumer-argobot-integration.md) and `#502`.
- Controller-family parity rules and structured fallback omissions where controllers lack status, source, event, or generation evidence.

**Untracked v2 follow-ups (no separate issue):**
- MCP `compare_source_truth` strategy-enum drift — MCP schema lists 4 strategies; CLI supports 9 (Phase 2 from `#418`). One-file fix in `cmd/cub-scout/mcp.go`.
- Source-truth receipt precedence edge coverage, especially `StatusBLOCK + VerdictBLOCKED`. Nice-to-have, not blocking.

**Open tracked issues:**
- **`#481`** — Helm/Kustomize provenance back-resolution for templated-source attribution (`gitSource.file:line` / `sourceMapRef` honesty markers).
- **`#502`** — ConfigHub history-backed event / Argobot evidence integration; current Argobot force-syncs Argo from release events, but status feedback write-back is future work.
- **`#475`** — Blog/documentation publication series for introducing cub-scout.
- **`#432`** — Grafana collector / data-source path using existing cub-scout JSON outputs. Design rather than code.
- **`#427`** — Watch kstatus migration may flip `Ready=true → false` for stalled workloads in v2.1.0+ (behavior-change design needed).
- **`#422`** — Views project: TUI Hub view integration (`#391` scope #2 follow-up).
- **`#421`** — Views project: CEL + JSONPath column evaluators.
- **`#392`** — Initiatives compliance overlay. **Deferred** until ConfigHub exposes Initiative as a backend primitive. Design doc at [`docs/howto/initiatives-integration-when-ready.md`](docs/howto/initiatives-integration-when-ready.md).
- **`#386`** — `preferInvocationForm` lint extension to non-hint legacy invocation-form leaks in strings.

### Attribution-layer next-up

- **`#481`** — Helm / Kustomize back-resolution to extend stage B's `gitSource.file:line` from raw YAML to templated sources
- List-key selectors in `compareFieldToPath` (e.g., `.spec.template.spec.containers[name="api"].image`)
- More standalone `--dry-from` / `--source-path` polish for source anchoring and proposal review flows
- `import --git-path --output-dir` polish (disk-PR proposal flow)
- Hierarchy-aware ingest (ApplicationSet / app-of-apps / Flux Kustomization composition)
- Additional manager-string writers (Tekton, Argo Workflows, Cluster API, OIDC-based CD)

### Cross-repo dependencies

- **`confighub/confighub#4356`** — ArgoCDOCI Helm-source shape symptom classifier. Blocks accurate `confighub-oci-argo` classification in `compare source-truth`.
- Pilot consumer-side fixtures pairing with cub-scout's source-truth + receipts surfaces (tracked in a separate non-public repo; cub-scout's surface contract is the canonical reference for downstream consumers).

## Current checkpoint

Current release tag: **`v2.6.0`**. The next release candidate is **`v2.7.0`**; release notes are in [`docs/releases/v2.7.0.md`](docs/releases/v2.7.0.md). The v2.0.0 plugin switchover (`cub scout` as the preferred invocation, MCP gateway as the AI front door) shipped in `v2.0.0` and is now historical — `docs/releases/v2.0.0-plugin-plan.md` is preserved as a historical artifact.

`main` HEAD at handover time: `ee84f01` (`Merge pull request #500 from confighub/codex/feature-docs-live-delivery-observability`). Zero open PRs.

`#500` CI was green before merge. Re-run `go test ./...` on any new release-polish branch before publishing.

Recent shipped capability surface:

- Live delivery observability release slice (`#500`): README user-question table; generation-aware rollout decision evidence on `doctor`, `explain`, and `compare three-way`; aggregate delivery resources; audited action event parsing; live-delivery fixture and how-to.
- Standalone rendered-object comparison: `compare three-way --dry-from`, `compare object-set --dry-from`, install/object-set receipts, object-set diff receipts, normalization profiles, `--ttl`, `--no-extras`, and external reference evidence.
- Delivery-done predicates: `object-set-matches`, `workloads-converged`, `prerequisites-met`, `receipt digest`, and `receipt chain`.
- Controller coverage hardening: Sveltos and Modelplane first-class support alongside Flux, Argo CD, Helm, Crossplane, kro, ConfigHub-managed, Terraform, and native resources.
- Attribution layer end-to-end (`cause` / `managerHint` / `gitSource` / `bindingSource` per field; stage B file:line back-resolution)
- Source-truth contract Phase 1 + 2 (9 strategies)
- Architectural triad locked in code (read-only-triad invariant; enforced at three layers)
- **Receipts v1 + v2 — feature-complete.** 3 predicates (`applied-matches-spec`, `source-truth-pass`, `no-manual-edits-since`); store + management UX (`receipt show / validate / list`); CI-gate exit semantics (`--fail-on`); chained receipts via `--input-attestation` with API-boundary `VerifiedAttestationRef` verify; aggregate-with-discovery via `--scope namespace/<ns>` + comma-list batch + `synthetic-aggregate://` subject + `--aggregate-policy` (max-severity default); real-time emission via `watch --emit-receipt-on` covering all 4 event types + `--emit-receipt-batch-cap` per-poll backpressure. 4 worked examples (`ci-gate/`, `chained/`, `aggregate/`, `watch-emit/`).
- AI-agent skill catalog: **42 skill files** = 8 verb-group skills (`#442` batch 1+2) + 7 controller-observer skills (`#442` batch 3) + 8 workflow scenario skills (`#442` batch 4) + **9 Pilot consumer-integration skills (`#444` batch A+B)** + 1 umbrella router + 9 shared references
- MCP gateway with closed read-only tool catalog (5 standalone + 5 connected tools)
- `--presentation human|ai|paired` on `doctor` / `explain` / `trace`

## Next milestone

Open work, in roughly descending leverage:

1. **`v2.7.0` release tag.** Release notes draft at `docs/releases/v2.7.0.md`; `#500` is the only post-`v2.6.0` merge currently on `main`.
2. **`#481` Helm/Kustomize provenance back-resolution.** Extends raw-YAML field attribution to templated sources while preserving honesty markers when exact file:line evidence is unavailable.
3. **`#432` Grafana collector / data-source path.** Uses existing cub-scout JSON outputs; design rather than code work.
4. **Views project tail.** `#422` TUI Hub View column projection (`#391` scope #2 follow-up); `#421` CEL + JSONPath column evaluators. Scopes #1 (`#414`) and #3 (`#420`) already shipped.
5. **`#427` watch kstatus migration behavior change.** Stalled workloads may flip `Ready=true → false` in v2.1.0+; needs design.
6. **`#386` `preferInvocationForm` lint extension** to non-hint legacy string leaks.
7. **`#392` ConfigHub Initiatives.** Deferred until ConfigHub exposes Initiative as a backend primitive — out of cub-scout's control.
8. **`#475` publication docs.** Use the README user-question table and v2.7.0 release notes as the source of truth.

Small untracked follow-ups carried from the receipts arc:
- **MCP `compare_source_truth` strategy-enum drift** — schema lists 4 strategies; CLI supports 9. One-file fix in `cmd/cub-scout/mcp.go`.
- **Source-truth receipt precedence edge coverage** — especially `StatusBLOCK + VerdictBLOCKED`. Nice-to-have, not blocking.

### CLI migration table (`#375`)

| Old top-level | New canonical path | Compatibility |
|-------|-------|-------|
| `discover` | `map workloads` | Hidden deprecated alias kept for one release |
| `health` | `map issues` | Hidden deprecated alias kept for one release |
| `combined` | `compare` | Alias kept; `compare` is the primary name |
| `connect` | `setup connect` | Hidden deprecated alias kept for one release |
| `completion` | `setup completion` | Hidden deprecated alias kept for one release |
| `apply` | `import apply` | Hidden deprecated alias kept for one release |
| `parse-repo` | `import parse-repo` | Hidden deprecated alias kept for one release |
| `import-argocd` | `import argocd` | Hidden deprecated alias kept for one release |
| `import-cluster-aggregator` | `import cluster-aggregator` | Hidden deprecated alias kept for one release |
| `drift` | `compare drift` | Hidden deprecated alias kept for one release |
| `demo` | `quickstart demo` | Hidden deprecated alias kept for one release |

Recent closures:
- #372 — Trim and restructure README (Apr 11)
  - Rewrote `README.md` from 845 lines down to 293 lines
  - Kept one signature trace example and moved deeper command detail to the canonical reference docs
  - Tightened the top-down flow to overview -> install -> fast path -> interfaces -> docs map
- #373 — Consolidate import commands under `import` (Apr 11)
  - `import argocd`, `import cluster-aggregator`, and `import parse-repo` are the canonical command paths
  - Hidden deprecated aliases remain available for one release: `import-argocd`, `import-cluster-aggregator`, and `parse-repo`
  - Active examples and docs now point at the canonical subcommand paths
- #374 — Consolidate overlapping CLI reference docs (Apr 11)
  - Recast `CLI-GUIDE.md` as a workflow-first guide instead of a second command encyclopedia
  - Updated `cli-reference.md` to match the current post-#375 command tree
  - Added `scripts/check-cli-docs/main.go` to verify canonical links and prevent README command tables from drifting back
- #375 — Reduce top-level command sprawl (Apr 11)
  - Reduced visible top-level command count to 29 while keeping legacy entrypoints as hidden deprecated aliases
  - Canonicalized `compare` as the primary name with `combined` kept as an alias
  - Reparented commands under `setup`, `import`, `compare`, and `quickstart`
- #377 — Audit `cub-scout mcp serve` tool descriptions against the cold-test sharpening lessons (Apr 11)
  - Sharpened MCP tool descriptions around first-tool identity, chain boundaries, and fallbacks
  - Added connected `compare_three_way` MCP tool as a thin read-only wrapper over `cub-scout compare three-way --format json`
  - Cold-test gap for “compare governed state to live state” is now covered by the MCP surface itself
- #368 — Beat Argo CD GUI as first stop for troubleshooting (Apr 9) — v1.10 wedge
  - Added recent K8s events to `explain` and `trace` commands
  - Bounded (top 5), prioritized (errors/warnings first), readable age format
  - JSON contract: `events` field with events[], totalCount, warningCount, errorCount
- #362 — Stabilize intermittent TestContextPack_FormatJSON kill (Apr 9)
  - Root cause: 30-second timeout was too tight for `go run ./cmd/cub-scout`
  - Fix: increased test runner timeout to 90 seconds
- #359 — Extend `--presentation` to additional read-only commands (Apr 9)
  - Added `--presentation=ai|human|paired` to trace command
  - AI mode: bracket notation, uppercase markers, `[end trace]` outro
  - Human/Paired mode: title-case headings, owner info, ownership chain heading
  - Legacy (no flag): preserves original formatting
- #370 — Structured action-typed next-step hints for JSON + MCP (Apr 9)
  - Added `nextSteps` field to doctor and explain JSON output
  - Each hint has actionType (read-only, mutating, waiting, human-decision), reason, nextCommand/nextSurface
  - MCP clients get structured hints for AI-driven workflows
- #360 — Carry secret evidence through v0.14 trace JSON converter (Apr 9)
  - v0.14 trace JSON now includes `secrets` field with full safe metadata
  - Fields: name, namespace, refType, refPath, status, secretType, createdAt, owner
  - Safe metadata only — no secret data exposed
- #364 — Render integration investigation (Apr 9)
  - Finding: Keep tools separate (`cub-scout` for structure, `cub gitops` for rendering)
  - `cub-scout import --git-path` for local preview, `cub gitops import` for actual rendering
  - Optional future enhancements: RenderableType detection, KustomizePath metadata
- #369 — Expose doctor as first-class MCP tool (Apr 9)
  - Added `doctor` to MCP gateway standalone tool set
  - Parameters: namespace (optional), top (optional integer)
  - First in tool list, matching its role as first troubleshooting command
  - Wraps `cub-scout doctor --format json`
- #371 — Three-way agreement/convergence summary (Apr 9)
  - New `AgreementSummary` in `compare three-way` output
  - States: agreed, converging, diverged, partial
  - Derived deterministically from per-resource patterns
  - Shows "Agreement: ✓ AGREED - All 5 resources agree" in ASCII
  - JSON: `summary.agreement.{state,summary,reasons,sources}`
- #363 — Enhanced Git parser for ArgoCD ApplicationSet git generators (Apr 9)
  - Extracts `directories[].path` patterns from git generators
  - Supports matrix generators with nested git generators
  - Exclude pattern support (paths starting with `!`)
  - Path-centric model for unique identification (handles duplicate basenames)
  - Full-path slugs in import proposals (`apps-team-a-api` vs `services-team-a-api`)

Investigation status:
- #364 — Render integration investigation (Apr 9) — **CLOSED**
  - Finding: Keep tools separate (`cub-scout` for structure, `cub gitops` for rendering)
  - Recommendation: Add optional "render-readiness" metadata (renderableType, kustomizePath)
  - See issue comment for detailed findings
- #366 — Three-way disagreement surfacing for connected mode (Apr 9)
  - `explain` shows THREE-WAY STATUS section when ConfigHub/Argo/cluster disagree
  - Patterns: change-in-progress, sync-stale, rollout-pending, multi-change
  - `doctor` hints to run `compare three-way` for full conformance check
- #367 — Phase-aware next-step hints for Argo-managed resources (Apr 9)
  - Three phases: incident (investigate), verify (confirm), closeout (read-only)
  - Phase detection is deterministic from ExplainSummary facts (health, drift, risks)
  - Hints are tailored to each phase - no new commands, just better guidance
- #365 — `explain` ownership for ArgoCD ApplicationSet-managed resources (Apr 9)
  - Two commits: ownership-preserving fallback + negative mismatch candidate filtering
  - `explain` now correctly reports ArgoCD for tracking-id annotated resources
- #357 — Git as a first-class source: initial local `--git-path` preview slice complete
- #356, #358, #361 — Docs/example sync cluster complete (Apr 6)
- #328 — Secrets in cub-scout: all slices complete (trace + Crossplane + map issues + TUI)
- #342 — Bidirectional snapshot and conformance workflow: both slices shipped
- #349, #350, #351, #352 — CLI polish cluster complete

Important: the current deterministic hints are already good and should not be
diminished. `#349` strengthened them; any follow-on hint work should keep moving
in that direction rather than flattening the current system.

Also important: the Apr 9 Argo truth-and-guidance track is now complete:
- `#365` (closed) — ownership truth for ApplicationSet-managed resources
- `#367` (closed) — phase-aware hints for Argo incidents/verification/closeout
- `#366` (closed) — three-way disagreement surfacing in connected mode

The Git import track is now investigated (see #364 comment):
- #363 complete: parser supports ApplicationSet git generators with path-centric model
- #364 investigated: recommendation is to keep tools separate, optionally add render-readiness metadata

Recommended next steps (optional enhancements):
1. Add `RenderableType` detection to parser (infer from kustomization.yaml vs Chart.yaml)
2. Add `KustomizePath` to JSON output for kustomize apps
3. Document combined workflow (`cub-scout` scouts, `cub gitops` renders)

## Git Import Architecture (Critical Context for #363 / #364)

This section documents the relationship between different import tools across
multiple repos. Understanding both `cub-scout import` and the broader `cub`
CLI import/rendering path is essential for continuing the Git import track.

### The Three Import Surfaces

1. **`cub gitops import`** (confighub/sdk - `cmd/cub/gitops_import.go`)
   - Discovers ArgoCD/Flux resources **from live K8s cluster** (not Git directly)
   - Uses render targets to get manifests (ArgoCD API or Flux renderer)
   - Creates ConfigHub units: -dry (renderer), -crds, -wet (rendered output)
   - Requires: running ArgoCD/Flux controller, K8s target, render target

2. **`cub gitops discover`** (confighub/sdk - `cmd/cub/gitops_discover.go`)
   - Finds ArgoCD Applications, Flux HelmReleases/Kustomizations in cluster
   - Prerequisite step for `cub gitops import`
   - Query: `import.include_custom = true AND kind IN ('Application','HelmRelease','Kustomization')`

3. **`cub-scout import --git-path`** (this repo - `cmd/cub-scout/import.go`)
   - Parses Git repo structure **locally** without cluster
   - Shows what SHOULD be deployed (Git source of truth)
   - Enables Git↔cluster comparison for verification
   - Initial implementation: uses `gitops.ParseRepo()` for Flux-style patterns

### The Full ConfigHub Loop

```
Git repo → ArgoCD syncs to cluster
        → cub gitops discover (finds ArgoCD apps)
        → cub gitops import (renders via ArgoCD API)
        → ConfigHub units created
        → Edit in ConfigHub
        → Apply back via Argo+OCI
```

Sample repo demonstrating this: `jesperfj/gitops-argocd`

### Rendering Implementations (confighub/sdk - bridge-impl/)

**ArgoCD Renderer** (`argocd-renderer/renderer.go`):
- Calls ArgoCD API to render Applications to manifests
- Requires running ArgoCD controller in cluster
- Creates/updates Application, waits for sync, fetches rendered manifests

**Flux Renderer** (`flux-renderer/`):
- `kustomize.go`: Fetches artifact URL, runs `kustomize build` locally
- `helm.go`: Loads chart from URL, uses Helm template engine locally
- Can render without cluster controller (just needs artifact URLs)

### ArgoCD ApplicationSet Git Generator Support (Complete)

The parser now fully supports ArgoCD ApplicationSets with git generators:

```yaml
# applicationsets/apps.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: apps/*
          - path: "!apps/excluded"  # Exclude patterns supported
```

**Implementation in cub-scout (#363 complete):**
- `pkg/gitops/parser.go`:
  - `extractGitGeneratorPatternsWithExcludes()` extracts include/exclude patterns
  - `extractMatrixGitPatternsWithExcludes()` handles nested git generators in matrix
  - full-path scanning preserves matched relative paths instead of guessing
  - path-centric target tracking preserves duplicate basenames safely
- `cmd/cub-scout/import.go` / `suggest.go`:
  - import proposals use path-derived unique slugs for duplicate-basename apps
- `cmd/cub-scout/import_git_test.go`: End-to-end ApplicationSet integration tests

`cub-scout import --git-path` now correctly discovers apps from ApplicationSet
git generators like those in `jesperfj/gitops-argocd`.

### Files Across Repos

| Repo | File | Purpose |
|------|------|---------|
| cub-scout | `cmd/cub-scout/import.go` | `--git-path` flag, Git preview flow |
| cub-scout | `cmd/cub-scout/import_git_test.go` | Git import tests |
| cub-scout | `pkg/gitops/parser.go` | `ParseRepo()` with ApplicationSet git generator support |
| confighub/sdk | `cmd/cub/gitops_import.go` | Import from K8s cluster |
| confighub/sdk | `cmd/cub/gitops_discover.go` | Discover GitOps resources in cluster |
| confighub/sdk | `bridge-impl/argocd-renderer/` | ArgoCD manifest rendering |
| confighub/sdk | `bridge-impl/flux-renderer/` | Flux manifest rendering (kustomize, helm) |

### Decision: Comparison is Automatic

When both Git and cluster sources are provided, comparison happens automatically:
- `import --git-path ./repo` → Git-only preview
- `import --git-path ./repo -n prod` → Git↔cluster comparison
- `import --git-path ./repo --from-bundle ./bundle` → Git↔bundle comparison

No separate `--compare` flag needed.

## Suggested next milestones

1. v1.10 shipped: Events wedge for #368 complete
   - Recent K8s events in `explain` and `trace`
   - Closes the main P0 gap in cub-scout vs Argo GUI comparison

2. Future directions (no open issues):
   - Sync status detail for deeper reconciliation visibility
   - Resource conditions surfacing (#348 follow-on)
   - Application-level grouping
   - Standalone desired-vs-live diff

## External publication boundary

Do not publish work externally without explicit user approval.

That includes:
- pushing to any non-public ConfigHub repository
- opening PRs or issue comments in non-public repositories
- creating public or secret gists for the work
- posting branch status externally

## Current wedge

The near-term product wedge is:

`GitHub + Argo/Flux + AI/CLI + ConfigHub`

## Themes to preserve

1. AI-first is structural, not decorative
   The examples follow the incubator pattern: four-file bundle, progressive disclosure,
   three-surface evidence boundaries. Runnable by Claude/Codex with minimal guessing.
2. Additive to Argo/Flux, not a replacement
   `cub-scout` explains, imports, compares, and proves. It does not need to be the runtime authority.
3. Command vs watch must stay explicit
   ConfigHub is for intended config and command intent. Runtime state/status
   comes from live systems and is authoritative there.
4. App/Deployment/Target as the primary mental model
   Space/Unit remains the current storage/API vocabulary, but not the front-door explanation.
5. Trustworthy evidence over optimistic status
   If ConfigHub/runtime status is uncertain, say so and show cluster/controller evidence.
6. Import must pay off immediately
   If a demo only shows import success and not value after import, it will feel academic.

## What "AI-first" means here

AI-first is structural, not decorative. It shapes the example's script structure,
evidence model, and file layout.

For the core connected demos, "AI-first" means:

1. each major example has:
   - `README.md` — human-oriented, answers the six reader questions
   - `AI_START_HERE.md` — safe entry point for AI agents with mutation boundaries
   - `prompts.md` — copyable prompts (orient, walkthrough, verify)
   - `contracts.md` — stable inspection paths with evidence boundaries
   - `verify.sh` — three-surface verification script
2. evidence boundaries are explicit and three-surfaced:
   - cluster evidence (kubectl, controller API)
   - ConfigHub evidence (cub unit list, cub target list, unit-action results)
   - cub-scout evidence (gitops status, map list, ownership classification)

## File hotspots

### AI-first demo examples (now complete)

- `examples/argo-import-confighub-demo/` — full AI-first structure
- `examples/flux-import-confighub-demo/` — full AI-first structure

### Key docs

- `docs/howto/import-to-confighub.md`
- `docs/howto/import-from-live.md`
- `docs/howto/using-cub-scout-from-ai-tool.md`
- `docs/reference/commands.md`
- `docs/reference/json-contracts.md`

### Presentation mode implementation (#352, #359 complete)

- `cmd/cub-scout/presentation.go` — mode types and helper functions (including Trace*)
- `cmd/cub-scout/doctor.go` — `--presentation` flag and rendering
- `cmd/cub-scout/explain.go` — `--presentation` flag and rendering
- `cmd/cub-scout/trace.go` — `--presentation` flag and mode-aware output

### Secret evidence implementation (#328 complete)

- `pkg/agent/secret_evidence.go` — core model and collector
- `pkg/agent/secret_evidence_test.go` — unit tests
- `cmd/cub-scout/trace.go` — wiring, Crossplane discovery, ASCII output
- `cmd/cub-scout/trace_providerconfig_test.go` — ProviderConfig-specific tests
- `cmd/cub-scout/map.go` — `map issues` secret collection and formatting
- `cmd/cub-scout/map_secret_issues_test.go` — map issues secret tests
- `cmd/cub-scout/localcluster.go` — TUI trace secret panel (renderTrace, runTrace)

### Git import implementation (#357 initial slice complete)

- `cmd/cub-scout/import.go` — `--git-path` flag, `runImportFromGit()`, `buildImportFromGitPreview()`
- `cmd/cub-scout/import_git_test.go` — Git path tests, evidence JSON tests
- `pkg/gitops/parser.go` — `ParseRepo()` with ApplicationSet git-generator + path-centric discovery support

## Proof expectations

Follow the repo's proof-first rules:

```bash
go build ./cmd/cub-scout
go test ./...
```

And if example/demo behavior changes:

1. run the relevant example script in its non-destructive mode first (`--explain`, `--dry-run`)
2. run `verify.sh` and confirm it passes
3. verify documented commands are copy-pasteable
4. check docs against current `cub` and `cub-scout` command reality
5. confirm the example still tells one coherent connected story

## Quick start for the next coder

1. Read `CLAUDE.md` for build/run/test commands
2. Read `AI-README-FIRST.md` for tool boundaries and current queue
3. Read `docs/roadmap.md` for strategic context
4. Check open issues: `gh issue list --state open`
5. Inspect AI-first examples:
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
6. Choose one small slice and keep it proof-first

## One-sentence strategy anchor

For the current wedge, `cub-scout` should help an AI-first operator import and understand GitOps-managed WET config, verify the import with three-surface evidence, show scan/policy results quickly, and make ConfigHub feel additive to Argo/Flux rather than heavier than them.
