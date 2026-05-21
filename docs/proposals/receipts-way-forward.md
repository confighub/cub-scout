# cub-scout Receipts — Way Forward

**Status:** Proposal / R&D synthesis
**Tracks:** #446 (capability), #447 (roadmap entry)
**Last updated:** 2026-05-21

## The question

> Could cub-scout have a "receipt creation" capability, in which it could be asked to verify some property — for example that Argo, or an agent, had completed updates to the cluster — and provide proof against the spec?

cub-scout already produces the field-level evidence such a verification would need (`compare three-way`, `compare source-truth`, attribution layer #435). What's missing is the typed envelope around that evidence — claim, scope, verdict, fingerprint — that makes it into a *verifiable artifact* rather than transient CLI output.

This document is the synthesis of the R&D pass against existing receipt / attestation prior art and the integration decisions that follow from it.

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

A complete proof of "an operation happened as intended" naturally decomposes into four layers:

| Layer | What it asserts | Who produces it | cub-scout's role |
|---|---|---|---|
| 1 — Governance of mutation | "This change was intended, decided, and authorized." | The governance ledger. *No dedicated tool currently shipped in the ConfigHub stack — concept is in design, out of scope for this issue.* | none |
| 2 — Controller delivery | "The reconciler applied it." | Argo / Flux native status objects | partial — `gitSource` already cites the controller-resolved source |
| **3 — Runtime fact** | **"This is what's actually in the cluster, and here's the evidence."** | **cub-scout** | **this work** |
| 4 — GUI proof | "A human can open a URL and verify each layer themselves." | The ConfigHub GUI verifier | not in scope; cub-scout emits the `confighubUrl` fields the GUI layer verifies |

cub-scout's receipt is the **Runtime-layer artifact**. Upstream acceptance assembles all four into a complete proof; cub-scout contributes layer 3 — same separation the read-only triad lock (#410/#428) already enforces.

## v1 design

**cub-scout v1 receipt = in-toto Statement v1 envelope + custom Runtime-proof predicate, fingerprint-only, immutable, with explicit omissions.**

### Statement v1 envelope (forward-compat)

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [{
    "name": "apps/v1/Deployment/prod/frontend",
    "digest": {"sha256": "<canonical-manifest-sha>"}
  }],
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
      "sourceTruth": {"strategy": "git-argo", "verdict": "PASS"}
    },
    "omissions": [],
    "inputAttestations": [],
    "nextSteps": [],
    "fingerprint": "sha256:..."
  }
}
```

Wrapping in Statement v1 even when unsigned makes v2's DSSE + OCI referrers + composition purely additive instead of a breaking change.

### Predicate vocabulary

| Predicate | Means | Backed by | v1? |
|---|---|---|:---:|
| `applied-matches-spec` | LIVE matches the spec at the given anchor (git path:line / OCI digest / ConfigHub unit revision) | `compare three-way` + `gitSource` | **✓** |
| `source-truth-pass` | `compare source-truth --strategy <s>` returned PASS | `compare source-truth` (9 strategies, post-#418) | **✓** |
| `no-manual-edits-since` | No `cause: manual-edit` entries in `managedFields` after a given timestamp | attribution layer (#435) | **✓** |
| `no-drift` | LIVE = WET = DRY across the connected three-way | `compare three-way` | v2 |
| `controller-reconciling` | Attribution says `cause: controller-drift` for the expected owner (controller actively converging) | attribution layer | v2 |
| `binding-matches` | Per-field `bindingSource` resolved to the expected upstream unit + path | C2 attribution (#439) | v2 |
| `apply-completed` | An apply happened (Argo last-synced / Flux last-applied-revision matches expected commit) | controller-side query via `cub` / `argocd` / `flux` CLI | v2 |

Every v1 predicate is backed by evidence cub-scout already produces today. Batch 1 needs a new envelope, a new evaluator, and a new fingerprint scheme around evidence that exists — not new evidence collection.

### Verdict vocabulary

| Verdict | Means |
|---|---|
| `PASS` | Evidence supports the claim |
| `WATCH` | Evidence is ambiguous; situation needs monitoring (includes `compare source-truth` ASK case at v1) |
| `BLOCK` | Evidence contradicts the claim |
| `INCONCLUSIVE` | Evidence is missing or unavailable (e.g., source-truth `INCOMPLETE`, `managedFields` stripped, ConfigHub offline in standalone mode) |

`ASK` from `compare source-truth` merges into `WATCH` at the receipt verdict level. Full source-truth fidelity is preserved in `evidence.sourceTruth.verdict` for consumers that need it.

### Receipt body (the predicate)

| Field | Required? | Notes |
|---|:---:|---|
| `claim` | yes | Free-text human-readable assertion |
| `scope` | yes | `{kind, name, namespace, cluster}` — what was verified |
| `verifier` | yes | `{tool, version}` |
| `verifiedAt` | yes | ISO 8601 timestamp |
| `predicateName` | yes | Which predicate was evaluated (from the table above) |
| `spec` | conditional | The anchor being verified against (required for spec-based predicates) |
| `verdict` | yes | PASS / WATCH / BLOCK / INCONCLUSIVE |
| `evidence` | yes | The Runtime-layer evidence body — compareThreeWay + attribution + sourceTruth |
| `omissions` | yes | Explicit list of what cub-scout deliberately does *not* claim — see below |
| `inputAttestations` | optional | References to other receipts by digest (for chained verification) |
| `nextSteps` | optional | Structured hints reusing `nextSteps[]` shape from #370 |
| `fingerprint` | yes | sha256 over canonical-JSON of the entire predicate body, excluding the fingerprint field |

### Omissions

Omissions are a required field. When evidence is missing, ambiguous, or inapplicable, the receipt **explicitly records the gap** rather than silently passing. This is the production-grade `parse, don't guess` rule made structural.

Examples of conditions that must produce `omissions[]` entries:

- `compare source-truth` returned `INCOMPLETE`
- `metadata.managedFields` was missing or stripped on the live object
- ConfigHub was unavailable in standalone-mode verification (no `bindingSource`, no `confighubUrl`)
- Only a subset of the resource's fields was checked
- A predicate was evaluated against a default anchor because none was provided

Without `omissions`, a receipt that says PASS could mean *"all checks passed"* or *"the checks we ran passed, but we didn't run all of them."* Those are very different claims. The receipt schema forces the distinction.

### Read-only triad invariant

Receipts emit artifacts. They do not mutate the cluster, ConfigHub, or any external store. The consumer (Pilot, CI/CD, audit log, governance ledger via separate upload) decides what to persist. No new authority gradient is created.

## Simplicity bar — what using receipts feels like

**The simplest command is the most useful one.** cub-scout should pick the predicate, spec anchor, and verdict thresholds for the user based on labels and annotations already on the resource. The user types the obvious command; cub-scout figures the rest out.

### Scenario A: verifying stages of a GitOps delivery

**1. One-resource, post-deploy: "did this reach prod?"**

```bash
$ cub-scout receipt verify deploy/api -n prod
PASS  applied-matches-spec   from git@org/platform-config:apps/prod/api/deployment.yaml@abc123
      controller: argocd-application-controller   no manual-edits since 14:30 UTC
      receipt: ./receipts/2026-05-21T14:31:00Z-rcpt_01HFK7Z3J.json
```

No flags. cub-scout reads the `argocd.argoproj.io/tracking-id` annotation, resolves the source repo + revision from the Argo Application, picks `applied-matches-spec` as the natural predicate, runs `compare three-way`, and emits the receipt.

**2. Whole namespace: "did the release land everywhere?"**

```bash
$ cub-scout receipt verify --scope namespace/prod
12/12 PASS   2026-05-21T14:32:00Z-batch_01HFK7Z4M.json
```

One aggregate receipt with 12 `inputAttestations[]` entries, one per workload. Drill down with `cub-scout receipt show <id>`.

**3. Tied to a specific commit (post-merge CI gate):**

```bash
$ cub-scout receipt verify deploy/api -n prod --at-commit abc123 --fail-on WATCH
PASS   exit 0
```

`--at-commit` swaps the auto-detected source anchor for an explicit one. `--fail-on` makes it a CI gate — exit 0 on PASS, exit 2 on WATCH/BLOCK. Drop this into a GitHub Actions step and the receipt becomes the gate's artifact.

**4. Multi-stage delivery: chain receipts as the release progresses:**

```bash
# Stage 1: post-merge — did Argo see it?
$ cub-scout receipt verify deploy/api -n prod --predicate apply-completed
PASS   argocd: last-synced rev=abc123  matches

# Stage 2: post-sync — does cluster match Git?
$ cub-scout receipt verify deploy/api -n prod
PASS   applied-matches-spec   argocd-application-controller reconciling

# Stage 3: post-cooldown — has anyone bypassed?
$ cub-scout receipt verify deploy/api -n prod --predicate no-manual-edits-since 14:30Z
PASS   no manual-edit cause in managedFields since 2026-05-21T14:30:00Z
```

Three commands, three receipts, one chain — each pointing at the prior via `inputAttestations[]`. The chain *is* the delivery audit trail.

### Scenario B: verifying an AI-led agentic change

**1. Agent verifies its own claim:**

```bash
# Agent says: "I just scaled deploy/api to 5 replicas"
$ cub-scout receipt verify deploy/api -n prod --claim "replicas=5 from intent rev=plan_4f8e"
PASS   spec.replicas: LIVE=5   cause: controller-drift (argocd-application-controller)
      receipt: ./receipts/2026-05-21T14:33:00Z-rcpt_01HFK8Q.json
```

The agent attaches its own claim string; cub-scout verifies the runtime state matches and emits the receipt. The agent hands the receipt to the operator (or to upstream acceptance-judge tooling) as proof.

**2. Agent proves "nothing else changed":**

```bash
# Agent ran its operation at $START_TIME
$ cub-scout receipt verify deploy/api -n prod --predicate no-manual-edits-since $START_TIME
PASS   no manual-edit cause in managedFields since 2026-05-21T14:30:00Z
      omissions: []   (full managedFields available; full coverage)
```

The agent's defensive receipt: "during my operation window, no human bypassed me." If the operator later asks "was there a kubectl edit while the agent was running?", the receipt answers definitively.

**3. Watch-driven verification (couples with `pilot-watch-alert-response` skill from #444):**

```bash
# Upstream acceptance tooling subscribes to cub-scout watch; each event triggers a verify
$ cub-scout watch -n prod --emit-receipt-on=update,delete --format jsonl > acceptance.jsonl
{"event":"update","resource":"deploy/api","receipt":"rcpt_01HFK8R...","verdict":"PASS"}
{"event":"update","resource":"deploy/worker","receipt":"rcpt_01HFK8S...","verdict":"WATCH","omissions":["sourceTruth=INCOMPLETE"]}
```

One streaming command. Every cluster mutation produces a receipt the upstream layer reads and decides on. No round-trip; no asking cub-scout twice.

**4. Multi-resource batch: agent did several things, prove they all landed:**

```bash
# Agent's plan touched 3 resources
$ cub-scout receipt verify deploy/api,deploy/worker,configmap/api-config -n prod
3/3 PASS   2026-05-21T14:34:00Z-batch_01HFK8T.json
      api:         applied-matches-spec
      worker:      applied-matches-spec
      api-config:  applied-matches-spec
```

One command, three sub-receipts, one aggregate receipt. The agent hands the aggregate ID to the operator; the chain is verifiable end to end.

**5. Agent-friendly JSON-first output (MCP / programmatic):**

```bash
$ cub-scout receipt verify deploy/api -n prod --format json | jq '.predicate.verdict'
"PASS"
```

For agents, `--format json` is the agent contract. Upstream acceptance tooling consumes the structured receipt directly; no ASCII parsing needed. Same `--format json` cub-scout already supports across every other command.

### What makes each of these simple

| Move | Why it matters |
|---|---|
| **No predicate flag in the common case.** cub-scout reads the ownership labels and picks the right one. | One-command UX. The user doesn't learn the predicate menu unless they want to. |
| **No anchor flag in the common case.** Source repo + revision come from the Argo/Flux spec automatically. | The user doesn't need to know which git path the controller is reading. |
| **Concise default output.** Verdict + 1-line summary + receipt path. Full JSON via `--format json`. | Humans can read it; agents can parse it. |
| **`--fail-on WATCH` makes it a CI gate.** Same shape as `compare three-way`'s existing `--fail-on`. | Receipts plug into existing CI patterns without new vocabulary. |
| **Watch-driven receipts via `--emit-receipt-on=…`.** One streaming command, one receipt per event. | Acceptance tooling doesn't need to poll; the event *is* the receipt trigger. |
| **Batch by comma-separated list or `--scope`.** Aggregate receipt with `inputAttestations[]` per resource. | One round trip for "did all three of my changes land?" — agent-friendly. |
| **Always emits a receipt.** Even on `INCONCLUSIVE` — the gap is recorded in `omissions[]`, never silenced. | Operators and upstream acceptance always have an artifact to reference; "I couldn't tell" is itself proof. |

These examples define the **UX acceptance bar for v1**. If batch 1 ships and a user has to type more than one command + one resource reference for the common case, the implementation has missed the bar.

## What cub-scout receipts are NOT

Boundaries that lock the scope:

1. **Not coupled to ConfigHub.** Standalone-mode receipts must work — no ConfigHub auth required. `bindingSource` and `confighubUrl` fields are optional; their absence is recorded as `omissions`, never as failure. Receipts work the same way they always have for cub-scout: cluster only by default, ConfigHub-aware when connected.
2. **Not a mirror of an upstream acceptance-judge decision record.** Upstream acceptance records carry operational fields (autonomy level, alternatives considered, counterfactuals, blocked actions, next gate). Those belong to the acceptance layer. cub-scout receipts carry **evidence + verdict** — period. The two compose via `inputAttestations[]`; they don't duplicate.
3. **Not a competing receipt-management surface.** Existing acceptance tooling already has receipt-management UX. cub-scout produces the Runtime-layer artifact those surfaces consume — it doesn't replicate them.
4. **Not signed with Notation / Notary v2.** Those sign artifacts, not predicates — wrong abstraction. DSSE + cosign (v2) is the right path.
5. **Not subject to "freshness" semantics.** Receipts are historical, immutable. Any state change produces a *new* receipt with a different fingerprint; the old one stays exactly as it was at the time of issue.
6. **Not gated on signing at v1.** Fingerprint-only at v1. Signing is purely additive in v2 (DSSE envelope around the Statement; cosign keyless or ed25519 key-based).

## v2 direction — designed, deferred

Documented now so v1 doesn't preclude v2:

| Capability | v2 form |
|---|---|
| Signing | DSSE envelope around Statement v1. Cosign keyless (Fulcio + Rekor) default for CI; ed25519 key-based fallback for airgap. |
| Sinks | `--sink oci://` (push as OCI artifact with `subject` field, OCI 1.1 referrers-compatible); `--sink rekor`; `--sink policyreport` (Kyverno PolicyReport CRD translation for in-cluster visualization). `--sink file://` remains v1 default. |
| Composition | Chain receipts via `subject.digest` references in `inputAttestations[]`. Fleet aggregates are themselves Statements over a synthetic fleet-snapshot digest. |
| Additional predicates | `no-drift`, `controller-reconciling`, `binding-matches`, `apply-completed` |

## Cross-references

| Issue / PR | Relationship |
|---|---|
| #435 (closed) | Attribution layer — produces the `cause`, `managerHint`, `gitSource`, `bindingSource` fields that go into the receipt's `evidence` body |
| #393 (shipped) | Source-truth contract v0.1 — the `source-truth-pass` predicate maps to this directly |
| #409 / #418 | Source-truth Phase 2 (9 strategies) — broadens the source-truth-pass predicate's reach |
| #441 (shipped) | Docs restructure — defined the 7 verb groups; this work adds an 8th group (`Verify`) |
| #442 | Comprehensive skills coverage — `scout-verify` will be the 8th verb-group skill once #446 batch 1 lands |
| #444 | Pilot–cub-scout integration skills — `pilot-watch-alert-response` couples `cub-scout watch` events to on-demand receipt creation; each watch event handled produces or references a receipt at the moment of decision |
| #410 / #428 | Read-only triad lock — the invariant receipts must respect |

## Pending decisions

Three calls needed before #446 batch 1 starts:

1. **Predicate URI ownership.** `https://cub-scout.dev/receipt/v1` (new domain) vs `https://confighub.com/cub-scout/receipt/v1` (vendor-anchored) vs upstream-contributed in-toto predicate. Upstream is slow but maximizes interop.
2. **Subject digest canonicalization.** What hashes into `subject.digest`: (a) live `kubectl get -o yaml` minus dynamic fields, (b) ConfigHub Unit canonical YAML, (c) both as separate subject entries. (c) is most honest; doubles subject array length.
3. **VSA interop layer.** Emit a parallel `verificationResult: PASSED|FAILED` field so SLSA VSA consumers parse cub-scout receipts as a degenerate VSA, or keep PASS/WATCH/BLOCK/INCONCLUSIVE purely in our predicate. Former buys free interop; latter avoids semantic distortion.

## Plan

### v1 — three PRs against #446

1. **Foundation + first predicate.** `pkg/agent/receipt.go` types, canonical-JSON, SHA-256 fingerprint, `cub-scout receipt verify` command + ASCII renderer + tests. First predicate: `applied-matches-spec`. JSON contract documented under `docs/reference/json-contracts.md` § Receipt Contract.
2. **Remaining v1 predicates.** `source-truth-pass` + `no-manual-edits-since`. Tests + example under `examples/receipts/`.
3. **Management UX.** `cub-scout receipt show` / `validate` / `list` subcommands.

### Skill coverage

`scout-verify` becomes the 8th verb-group skill in #442. Implementation lands in the appropriate batch.

### v2

Separate issue once v1 is in production. Designed but not implemented per the v2-direction section above.

## What I'd do next

1. **Get the three pending decisions answered.** 30-minute call or async thread.
2. **Open PR for batch 1**: `pkg/agent/receipt.go` foundation + `applied-matches-spec` predicate + fingerprint + Statement v1 envelope + ASCII renderer + tests + first example + json-contracts.md § Receipt Contract.

The R&D pass is complete. The design is ready. Three decisions and one PR away from `cub-scout receipt verify` being a real command.
