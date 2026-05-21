# cub-scout Receipts — Way Forward

**Status:** Proposal / R&D synthesis (post-review)
**Tracks:** #446 (capability), #447 (roadmap entry)
**Last updated:** 2026-05-21

## The question

> Could cub-scout have a "receipt creation" capability, in which it could be asked to verify some property — for example that Argo, or an agent, had completed updates to the cluster — and provide proof against the spec?

cub-scout already produces the field-level evidence such a verification would need (`compare three-way`, `compare source-truth`, attribution layer #435). What's missing is the **typed envelope** around that evidence — claim, scope, verdict, fingerprint — that makes it into a *verifiable artifact* rather than transient CLI output.

This document is the synthesis of the R&D pass against existing receipt / attestation prior art, plus the external design review that followed.

## What a receipt is

> **A receipt is the user-verifiable evidence object that makes a claim safe to trust.**

Three forms of cub-scout output, and how a receipt is different:

| Form | What it is | cub-scout examples |
|---|---|---|
| Log | Chronological events | `cub-scout watch` event stream |
| Summary | Prose narrative | `cub-scout explain`, `cub-scout doctor` |
| **Receipt** | **Typed, structured, verifiable evidence object** | (this work) |

Receipts are **historical, immutable records of past events**. Updates produce *new* receipts, never mutate old ones. The chain of receipts is the history; no individual receipt mutates. "Keep the receipt fresh" is a category error — receipts record what was true at time T; a different state at T+1 produces a different receipt.

Vocabulary:

- **Proof** is the canonical product noun.
- **Receipt** is the artifact form (Nix-installer lineage — persistent record of what an operation did).

## The four-layer proof model

A complete proof of "an operation happened as intended" decomposes into four layers:

| Layer | What it asserts | Producer | cub-scout's role |
|---|---|---|---|
| 1 — Governance of mutation | "This change was intended, decided, and authorized." | Governance ledger. *No dedicated tool currently shipped in the ConfigHub stack — concept is in design, out of scope for this issue.* | none |
| 2 — Controller delivery | "The reconciler applied it." | Argo / Flux native status | partial — `gitSource` cites the controller-resolved source |
| **3 — Runtime fact** | **"This is what's actually in the cluster, and here's the evidence."** | **cub-scout** | **this work** |
| 4 — GUI proof | "A human can open a URL and verify each layer themselves." | The ConfigHub GUI verifier | not in scope; cub-scout emits the `confighubUrl` fields the GUI layer verifies |

cub-scout's receipt is the **Runtime-layer artifact**. Upstream tooling composes layers via `inputAttestations[]` digest references; cub-scout contributes layer 3 — same separation the read-only triad lock (#410/#428) already enforces.

## v1 design

**cub-scout v1 receipt = in-toto Statement v1 envelope + custom Runtime-proof predicate, fingerprint-only, immutable, with explicit omissions, single-resource scope.**

### Statement v1 envelope (forward-compat)

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {"name": "k8s-live://apps/v1/Deployment/prod/frontend", "digest": {"sha256": "<canonical-manifest-sha>"}},
    {"name": "confighub-unit://payments-api@rev=42", "digest": {"sha256": "<unit-canonical-sha>"}}
  ],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "version": "v1",
    "claim": "applied matches spec at apps/prod/frontend/deployment.yaml",
    "scope": {"kind": "Deployment", "name": "frontend", "namespace": "prod", "cluster": "prod-use2"},
    "verifier": {"tool": "cub-scout", "version": "v2.2.1"},
    "verifiedAt": "2026-05-21T10:30:00Z",
    "predicateName": "applied-matches-spec",
    "spec": {
      "anchor": {"type": "git", "repoUrl": "https://github.com/org/repo", "revision": "abc123def456", "path": "apps/prod/frontend/deployment.yaml"}
    },
    "verdict": "PASS",
    "evidence": {
      "compareThreeWay": "...",
      "attribution": {"cause": "controller-drift", "managerHint": "argocd-application-controller", "gitSource": "...", "bindingSource": null},
      "sourceTruth": {"strategy": "git-argo", "status": "PASS", "verdict": "AGREED"}
    },
    "omissions": [],
    "inputAttestations": [],
    "nextSteps": [],
    "fingerprint": "sha256:..."
  }
}
```

Wrapping in Statement v1 even when unsigned makes v2's DSSE + OCI referrers + composition purely additive instead of a breaking change. v2's signing target is **Sigstore Bundle v0.3** wrapping DSSE over this Statement, not a bespoke envelope.

### Fingerprint scope (corrected from initial sketch)

**The fingerprint is SHA-256 over canonical-JSON of the full Statement (`_type` + `subject` + `predicateType` + `predicate`) minus only the `predicate.fingerprint` field itself.**

Hashing only the predicate body would leave `subject`, `predicateType`, and the envelope structure unprotected — a tamperer could swap subjects without changing the fingerprint. The full-Statement scope closes that.

**Canonical-JSON serialization: RFC 8785 (JSON Canonicalization Scheme).** Recursive key sorting, strict primitive serialization, no whitespace, UTF-8. Go's `encoding/json` map-key sorting is necessary but not sufficient; the v1 implementation uses an RFC 8785 library or a hand-implemented subset with explicit conformance tests.

### Subject digest — dual-subject canonicalization

Connected mode emits **two subjects** in the `subject[]` array:

1. **`k8s-live://<gvk>/<namespace>/<name>`** — digest is SHA-256 over the live K8s object's canonical YAML, with these dynamic fields pruned: `status`, `metadata.managedFields`, `metadata.resourceVersion`, `metadata.generation`, `metadata.uid`, `metadata.creationTimestamp`.
2. **`confighub-unit://<unit-slug>@rev=<n>`** — digest is SHA-256 over the ConfigHub Unit's canonical YAML at the given revision.

Standalone mode emits only subject 1. Subject 2 is replaced with an `omissions[]` entry: `{"missing": "confighub-unit-subject", "reason": "standalone-mode"}`.

### Predicate vocabulary

| Predicate | Means | Backed by | v1? |
|---|---|---|:---:|
| `applied-matches-spec` | LIVE matches the spec at the given anchor (git path:line / OCI digest / ConfigHub unit revision). **Standalone PASS requires a resolvable controller-side git anchor; otherwise INCONCLUSIVE + `omissions[]` entry naming the missing input.** | `compare three-way` + `gitSource` | **✓** |
| `source-truth-pass` | `compare source-truth --strategy <s>` returned PASS for the explicitly-specified strategy. **Auto-detecting the strategy is out of scope — source-truth's contract requires declared strategy. If `--strategy` isn't passed, this predicate is unavailable.** | `compare source-truth` (9 strategies, post-#418) | **✓** |
| `no-manual-edits-since` | No `cause: manual-edit` entries in `managedFields` after a given timestamp. **Caveat: managedFields is field-manager evidence, not complete mutation-history proof — an admission-log + eBPF event stream would be stronger but is not available here.** | attribution layer (#435) | **✓** |
| `no-drift` | LIVE = WET = DRY across the connected three-way | `compare three-way` | v2 |
| `controller-reconciling` | Attribution says `cause: controller-drift` for the expected owner (controller actively converging) | attribution layer | v2 |
| `binding-matches` | Per-field `bindingSource` resolved to the expected upstream unit + path | C2 attribution (#439) | v2 |
| `apply-completed` | An apply happened (Argo last-synced / Flux last-applied-revision matches expected commit) | controller-side query via `cub` / `argocd` / `flux` CLI | v2 |

Every v1 predicate is backed by evidence cub-scout already produces today. Batch 1 needs a new envelope, evaluator, and fingerprint scheme around evidence that exists — not new evidence-collection code.

### Verdict vocabulary

Receipt-level (always one of):

| Verdict | Means |
|---|---|
| `PASS` | Evidence supports the claim |
| `WATCH` | Evidence is ambiguous; situation needs monitoring (includes `compare source-truth` ASK case at v1) |
| `BLOCK` | Evidence contradicts the claim |
| `INCONCLUSIVE` | Evidence is missing or unavailable (`source-truth` returned `INCOMPLETE`, `managedFields` stripped, ConfigHub offline in standalone mode) |

**Source-truth's native `status` and `verdict` are preserved verbatim inside `evidence.sourceTruth`** — the receipt-level verdict is cub-scout's distilled answer; the embedded `evidence.sourceTruth.{status, verdict}` carry source-truth's own enums:

- `status` (`SourceTruthStatus`): `PASS | WATCH | BLOCK | ASK`
- `verdict` (`SourceTruthVerdict`): `AGREED | MISMATCH | INCOMPLETE | BLOCKED | UNKNOWN`

The two enums are independent in source-truth: `verdict` is the cross-surface agreement signal; `status` is cub-scout's evidence-quality answer Pilot layers acceptance on top of. The receipt's distilled verdict maps:

| Source-truth signal | Receipt verdict |
|---|---|
| `status: PASS` AND `verdict: AGREED` | PASS |
| `status: WATCH` | WATCH |
| `status: ASK` (any verdict) | WATCH |
| `verdict: INCOMPLETE` or `verdict: BLOCKED` (any status) | INCONCLUSIVE |
| `verdict: MISMATCH` AND `status: BLOCK` | BLOCK |
| `verdict: UNKNOWN` (any status) | INCONCLUSIVE |

The mapping table is documented in the JSON contract for downstream consumers.

### Auto-detection priority order

The one-command UX (`cub-scout receipt verify deploy/api -n prod`) picks a predicate using this deterministic order:

1. **Explicit `--predicate` always wins.**
2. **Argo/Flux source anchor resolvable** → `applied-matches-spec`.
3. **Connected ConfigHub + explicit `--strategy` provided** → `source-truth-pass`.
4. **`--since` timestamp supplied** → `no-manual-edits-since`.
5. **Otherwise** → emit a receipt with verdict `INCONCLUSIVE` + `omissions[]` entry `{"missing": "auto-detected-predicate", "reason": "no signal for default predicate; provide --predicate"}`. **Never guess.**

Mixed Argo + ConfigHub still defaults to `applied-matches-spec` for the "did it land?" UX, with ConfigHub/source-truth evidence embedded for consumers that need it.

### Receipt body (the predicate)

| Field | Required? | Notes |
|---|:---:|---|
| `version` | yes | Predicate schema version (`v1`) |
| `claim` | yes | Free-text human-readable assertion |
| `scope` | yes | `{kind, name, namespace, cluster}` — what was verified |
| `verifier` | yes | `{tool, version}` |
| `verifiedAt` | yes | ISO 8601 timestamp |
| `predicateName` | yes | Which predicate was evaluated |
| `spec` | conditional | The anchor being verified against (required for spec-based predicates) |
| `verdict` | yes | PASS / WATCH / BLOCK / INCONCLUSIVE |
| `evidence` | yes | Runtime-layer evidence — `compareThreeWay` + `attribution` + `sourceTruth` |
| `omissions` | yes | Explicit list of what cub-scout deliberately does *not* claim |
| `inputAttestations` | optional | References to other receipts by digest (chain semantics — v1 emits `[]`; v2 populates) |
| `nextSteps` | optional | Read-only structured hints reusing the `nextSteps[]` shape from #370. **Mutating `actionType` is rejected at receipt-emit time** — receipts cannot recommend writes. |
| `fingerprint` | yes | sha256 over canonical-JSON of the **full Statement** minus only this field (see Fingerprint scope above) |

### Omissions

Omissions are a required field. When evidence is missing, ambiguous, or inapplicable, the receipt **explicitly records the gap** rather than silently passing. Production-grade `parse, don't guess` made structural.

Examples of conditions that must produce `omissions[]` entries:

- `compare source-truth` returned `INCOMPLETE`
- `metadata.managedFields` was missing or stripped on the live object
- ConfigHub was unavailable in standalone-mode verification (no `bindingSource`, no `confighubUrl`, no `confighub-unit://` subject)
- Only a subset of the resource's fields was checked
- A predicate was evaluated against a default anchor because none was provided
- Auto-detection couldn't pick a predicate (verdict `INCONCLUSIVE` with `missing: auto-detected-predicate`)

Without `omissions`, a `PASS` receipt could mean *"all checks passed"* or *"the checks we ran passed, but we didn't run all of them."* The schema forces the distinction.

Omission entries follow the CycloneDX-attestations shape: each entry is `{missing: <field/source>, reason: <human-readable>, severity?: info|warning}`.

### Read-only triad invariant

Receipts emit to stdout / file. They do not mutate the cluster, ConfigHub, or any external store. Consumers (CI/CD, audit log, upstream acceptance-judge tooling) decide what to persist. No new authority gradient.

Specific guard rails the v1 implementation enforces:

- `nextSteps[]` rejects entries with `actionType=mutating` or mutating `nextCommand` at receipt-emit time. Acceptance test: `TestReceiptHasNoMutatingNextSteps`.
- The `receipt` package exposes only `Get` / `List` / `Watch` cluster operations. Acceptance test: `TestReceiptPackageReadOnlyClient` greps for forbidden patterns.
- `receipt validate --re-check` may re-read cluster state but must never rewrite the receipt or call mutating tools.

## Simplicity bar — what using receipts feels like (v1)

**The simplest command is the most useful one.** cub-scout picks the predicate and spec anchor for the user based on existing labels and annotations. The user types the obvious command; cub-scout figures the rest out.

v1 ships **single-resource verify + show + validate** only. Batch / aggregate / chained / watch-driven receipts are explicit v2 work — see "v2 direction" below.

### Scenario A: verifying GitOps delivery

**1. One-resource, post-deploy: "did this reach prod?"**

```bash
$ cub-scout receipt verify deploy/api -n prod
PASS  applied-matches-spec   from git@org/platform-config:apps/prod/api/deployment.yaml@abc123
      controller: argocd-application-controller   no manual-edits since 14:30 UTC
      receipt: ./receipts/2026-05-21T14:31:00Z-rcpt_01HFK7Z3J.json
```

No flags. cub-scout reads the `argocd.argoproj.io/tracking-id` annotation, resolves the source repo + revision from the Argo Application, picks `applied-matches-spec` as the natural predicate, runs `compare three-way`, and emits the receipt.

**2. Tied to a specific commit (CI gate input):**

```bash
$ cub-scout receipt verify deploy/api -n prod --at-commit abc123
PASS  rcpt_01HFK8M
```

`--at-commit` swaps the auto-detected source anchor for an explicit one. The CI pipeline reads the receipt file, parses the verdict, and gates on its own logic. (Note: `--fail-on RECEIPT_VERDICT` is a small v1.x follow-on — see spin-off list.)

### Scenario B: verifying an AI-led agentic change

**1. Agent verifies its own claim:**

```bash
$ cub-scout receipt verify deploy/api -n prod --claim "replicas=5 from intent rev=plan_4f8e"
PASS   spec.replicas: LIVE=5   cause: controller-drift (argocd-application-controller)
      receipt: ./receipts/2026-05-21T14:33:00Z-rcpt_01HFK8Q.json
```

The agent attaches its own claim string; cub-scout verifies the runtime state matches and emits the receipt. The agent hands the receipt to the operator (or to upstream acceptance-judge tooling) as proof.

**2. Agent proves "nothing else changed":**

```bash
$ cub-scout receipt verify deploy/api -n prod --predicate no-manual-edits-since $START_TIME
PASS   no manual-edit cause in managedFields since 2026-05-21T14:30:00Z
      omissions: []
```

The agent's defensive receipt. Caveat included in the receipt body: "managedFields is field-manager evidence, not full mutation-history proof."

**3. Agent-friendly JSON output (MCP / programmatic):**

```bash
$ cub-scout receipt verify deploy/api -n prod --format json | jq '.predicate.verdict'
"PASS"
```

For agents, `--format json` is the contract. Upstream acceptance tooling consumes the structured receipt directly; no ASCII parsing.

### What makes each of these simple

| Move | Why it matters |
|---|---|
| **No predicate flag in the common case** — cub-scout reads the ownership labels and picks via auto-detection priority order | One-command UX. The user doesn't learn the predicate menu unless they want to. |
| **No anchor flag in the common case** — source repo + revision come from the Argo/Flux spec automatically | The user doesn't need to know which git path the controller is reading. |
| **Concise default output** — verdict + 1-line summary + receipt path. Full JSON via `--format json` | Humans can read it; agents can parse it. |
| **Always emits a receipt** — even on `INCONCLUSIVE`. The gap is recorded in `omissions[]`, never silenced | Operators and upstream acceptance always have an artifact to reference; "I couldn't tell" is itself proof. |
| **Single-resource only at v1** | Three PRs of cleanly-scoped work, not a maze of composition + streaming complexity. |

**These examples are the v1 UX acceptance bar.** If batch 1 ships and the common single-resource case requires more than one command + one resource reference, the implementation has missed the bar.

### Receipt storage

v1 emits to stdout, to a file via `--out <path>`, or to a local receipt directory by default. Default directory: **`$XDG_DATA_HOME/cub-scout/receipts/`**, falling back to **`~/.cub-scout/receipts/`**. Receipts are individual JSON files named `<timestamp>-<receipt-id>.json`. `cub-scout receipt list` reads the local directory; stdout-only receipts are intentionally not listable (ephemeral by user choice).

External sinks (OCI, Rekor, etc.) are v2; cub-scout v1 has no remote-write paths.

## What cub-scout receipts are NOT

Boundaries that lock the scope:

1. **Not coupled to ConfigHub.** Standalone-mode receipts must work — no ConfigHub auth required. `bindingSource`, `confighubUrl`, and the `confighub-unit://` subject are optional; absence is recorded as `omissions`, never as failure.
2. **Not a mirror of an upstream acceptance-judge decision record.** Upstream acceptance tooling carries operational decision metadata that belongs to that layer. cub-scout receipts carry **evidence + verdict**, period. The two compose via `inputAttestations[]`; they don't duplicate.
3. **Not a competing receipt-management surface.** Existing acceptance tooling has its own receipt-management UX. cub-scout produces the Runtime-layer artifact those surfaces consume.
4. **Not signed with Notation / Notary v2.** Those sign artifacts, not predicates — wrong abstraction. DSSE + Sigstore (v2) is the right path.
5. **Not subject to "freshness" semantics.** Receipts are historical, immutable. Any state change produces a *new* receipt with a different fingerprint.
6. **Not gated on signing at v1.** Fingerprint-only at v1. Signing is purely additive in v2.
7. **Not a producer of PolicyReport CRDs or other cluster-side artifacts.** That writes to the cluster and violates the read-only triad. A separate `cub-scout-to-policyreport` adapter could translate receipts into CRDs externally — but that's not cub-scout's job.

## v2 direction — designed, deferred

Documented now so v1 doesn't preclude v2. Tracked separately to keep v1 scope honest:

| Capability | v2 form | Tracked in |
|---|---|---|
| Signing | DSSE envelope around Statement v1 wrapped in **Sigstore Bundle v0.3**. Cosign keyless (Fulcio + Rekor) default for CI; ed25519 key-based fallback for airgap. | follow-on issue |
| Sinks | `--sink oci://` (OCI 1.1 referrers-compatible). `--sink file://` is the v1 default. PolicyReport sink is explicitly NOT cub-scout's; see spin-off list. | follow-on issue |
| Composition | Chain receipts via `subject.digest` references in `inputAttestations[]`. Aggregate receipts (whole-namespace, multi-resource, fleet) are Statements over synthetic aggregate digests. **Designed to be graph-ingestible (GUAC, etc.) from day 1.** | #448 |
| Watch-driven receipts | `cub-scout watch --emit-receipt-on <event-types>` streaming receipt emission | #449 |
| Additional predicates | `no-drift`, `controller-reconciling`, `binding-matches`, `apply-completed` | v2 issue |
| CI exit semantics | `--fail-on RECEIPT_VERDICT` extending the existing `--fail-on info|warning` pattern from `compare three-way` | v1.x small follow-on |

## Locked design decisions

The three prior open questions are now locked (post external review):

| Decision | Lock |
|---|---|
| **Predicate URI ownership** | `https://cub-scout.dev/receipt/v1`. Standalone OSS identity; upstream in-toto vetted-predicate contribution is a v2+ ambition. |
| **Subject digest canonicalization** | Dual subjects: `k8s-live://...` always; `confighub-unit://...` when connected. Pruned fields: `status`, `metadata.managedFields`, `metadata.resourceVersion`, `metadata.generation`, `metadata.uid`, `metadata.creationTimestamp`. |
| **VSA interop layer** | Keep PASS/WATCH/BLOCK/INCONCLUSIVE pure in the cub-scout predicate. A v2+ Sigstore Bundle export *may* additionally emit a VSA wrapper, but the source predicate vocabulary stays unchanged. |

## Prior-art coverage notes (post-review)

Patterns that informed the design beyond the initial R&D pass:

- **GUAC** ([docs.guac.sh](https://docs.guac.sh/)) — graph aggregation of attestations. v2 composition design ensures cub-scout receipts are graph-ingestible (stable `subject.digest`, predicate URI, `inputAttestations[]`).
- **Sigstore Bundle v0.3** ([sigstore.dev/about/bundle/](https://docs.sigstore.dev/about/bundle/)) — v2 signing target; carries DSSE over Statement v1 plus verification material in a single bundle.
- **CycloneDX Attestations** ([cyclonedx.org/use-cases/attestations/](https://cyclonedx.org/use-cases/attestations/)) — claim / counterclaim / evidence / conformance / confidence vocabulary. Source for our `omissions[]` shape.
- **GitLab SLSA + Jenkins SLSA** — v1 UX wording stays CI-tool-neutral; no GitHub-Actions-specific examples in committed docs.
- **Falco / Tracee** — runtime mutation evidence is stronger via audit-log + eBPF event streams than via `managedFields` alone. Locked into the `no-manual-edits-since` caveat language.
- **Cloud Custodian / AWS Config** — `COMPLIANT|NON_COMPLIANT|INSUFFICIENT_DATA` pattern validates our PASS/WATCH/BLOCK/INCONCLUSIVE shape.
- **Gatekeeper / Connaisseur / OPA Conftest** — adjacent admission-policy patterns. cub-scout receipts are *post-fact* evidence, distinct from *pre-admission* gates.

## Spin-off work (separate issues)

Tracked so v1 stays narrow:

| Topic | Form |
|---|---|
| Aggregate / chained receipts (`--scope`, comma-list, `inputAttestations[]` composition) | #448 |
| `cub-scout watch --emit-receipt-on` streaming receipt emission | #449 |
| Source-truth help-text drift (4→9 strategies) | #450 |
| `--fail-on RECEIPT_VERDICT` exit-semantics extension | #451 |
| PolicyReport / Kyverno integration | External adapter project (not cub-scout) |

## Cross-references

| Issue / PR | Relationship |
|---|---|
| #435 (closed) | Attribution layer — produces `cause`, `managerHint`, `gitSource`, `bindingSource` evidence inside the receipt |
| #393 (shipped) | Source-truth contract v0.1 — `source-truth-pass` predicate maps directly |
| #409 / #418 | Source-truth Phase 2 (9 strategies) — broadens `source-truth-pass` reach |
| #441 (shipped) | Docs restructure — defined 7 verb groups; this work adds an 8th (`Verify`) |
| #442 | Comprehensive skills coverage — `scout-verify` is the 8th verb-group skill |
| #444 | Pilot–cub-scout integration skills — couples watch events to receipt creation |
| #410 / #428 | Read-only triad lock — invariant receipts must respect |

## Plan

### v1 — three PRs against #446

1. **Foundation + `applied-matches-spec` predicate.** `pkg/agent/receipt.go` types + RFC 8785 canonical-JSON + SHA-256 fingerprint over the full Statement minus `predicate.fingerprint` + Statement v1 envelope with dual subjects + first predicate evaluator + auto-detection priority order + `cub-scout receipt verify` + ASCII renderer + tests including tamper / determinism / read-only-triad / nextSteps-mutating-rejection. JSON contract locked in `docs/reference/json-contracts.md` § Receipt Contract.
2. **`source-truth-pass` + `no-manual-edits-since` predicates.** Tests + example under `examples/receipts/`. Strategy auto-detection explicitly out of scope.
3. **Management UX.** `cub-scout receipt show` / `validate` / `list` subcommands. Local-directory storage semantics finalized.

### Skill coverage

`scout-verify` becomes the 8th verb-group skill in #442.

### v2

Separate issue once v1 is in production. Designed but not implemented per the v2-direction section above.

## What I'd do next

1. **Open PR for batch 1:** `pkg/agent/receipt.go` foundation + canonical-JSON (RFC 8785) + fingerprint scope + Statement v1 with dual subjects + `applied-matches-spec` predicate + auto-detection priority + read-only-triad guards + tests + first example + json-contracts.md update.
2. **Spin-off issues #448–#451 are already filed** so v1 scope stays honest.

The R&D pass and external review are both complete. The design is locked. One PR away from `cub-scout receipt verify` being a real command.
