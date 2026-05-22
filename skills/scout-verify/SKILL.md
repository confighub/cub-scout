---
name: scout-verify
description: 'Use when the user wants a TYPED, FINGERPRINTED, IMMUTABLE evidence artifact about a Kubernetes resource — the Verify verb group of cub-scout (the receipt capability shipped in #446). Natural phrasing: "produce a receipt that this is in compliance", "give me an attestation that the cluster matches Git", "I need a verifiable record for the audit trail", "build a receipt verifying no manual edits since deploy", "verify deploy/api against source-truth and save the artifact", "validate this receipt is authentic", "show me what receipts we have", "is this receipt tampered with?", "attach evidence to my CI gate". Load whenever intent is receipt / attestation / evidence-artifact / signed-evidence / verified-record / audit-trail / postmortem-evidence / acceptance-judge-input. Do NOT load for: a live comparison without a persisted artifact (use scout-compare), interpreting a single field''s provenance (use scout-attribute), or actually applying a fix (cub-scout never mutates — route to cub / argocd / kubectl with the user driving).'
phase: verify
allowed-tools: Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout receipt list) Bash(cub-scout receipt list) Bash(cub scout receipt list) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub * get) Bash(cub * list) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)
---

# scout-verify

The Verify verb group of cub-scout. Produces typed, fingerprinted, immutable evidence artifacts — **receipts** — wrapping cub-scout's existing field-level evidence into a verifiable record CI/CD gates, audit trails, postmortems, and acceptance-judge tooling can attach to a decision and later prove the inputs were what they claim to be.

Receipts are HISTORICAL records. Updates produce new receipts; old ones never mutate.

## When to use

Explicit phrasings:

- "Produce a receipt that deploy/api is in compliance"
- "Give me a verifiable attestation that the cluster matches Git"
- "I need an evidence artifact for the audit trail"
- "Build a receipt verifying no manual edits since the freeze started"
- "Verify deploy/api against source-truth and save the artifact"
- "Validate this receipt is authentic / hasn't been tampered with"
- "List the receipts I've generated locally"
- "Show me yesterday's receipts where the verdict was BLOCK"
- "Attach typed evidence to my CI gate / promotion gate / release ticket"

Implicit intents:

- The user wants **persistence** of an evidence snapshot, not just an ephemeral comparison
- The user is gating CI / promotion / acceptance on a verdict
- The user needs to **prove later** that the inputs were what they claim to be (forensic / audit / postmortem)
- The user is integrating with Pilot or a similar acceptance kernel that consumes receipt-shaped evidence
- The user wants a tamper-evident artifact, not a log line

## Do not load for

- Live comparison without a persisted artifact — [`scout-compare`](../scout-compare/SKILL.md) is faster and the right shape
- Interpreting *why* a single field is wrong (which controller, which file) — [`scout-attribute`](../scout-attribute/SKILL.md)
- Inventory / status / "what's running" — [`scout-observe`](../scout-observe/SKILL.md)
- Forcing an apply / sync / patch — cub-scout cannot. Receipts emit; they never act.

## Standalone vs connected

- **Standalone (cluster only):** `receipt verify --predicate applied-matches-spec` and `--predicate no-manual-edits-since` work without ConfigHub. The receipt records `OmissionConfigHubUnitSubject` to flag the missing connected-mode evidence.
- **Connected (cluster + `cub auth login`):** `--predicate source-truth-pass --strategy <name>` becomes available. Connected mode also adds the `confighub-unit://` subject to every receipt, anchoring it to the unit revision in addition to the live K8s digest.
- **Offline (file only):** `receipt show` / `receipt validate` / `receipt list` all work on on-disk artifacts with no cluster and no ConfigHub. Useful in CI containers that received the receipt as an artifact from an upstream job.

## Tool boundary

- **Allowed (read-only):** the four receipt verbs (`verify` / `show` / `validate` / `list`); `kubectl get/describe`, `kubectl get --show-managed-fields` (for inspecting what `no-manual-edits-since` saw); `cub * get/list`, `cub unit get`, `cub link list` (connected); `argocd app get`, `flux get` (when building source-truth-pass evidence).
- **Not allowed:** `kubectl apply/edit/patch/delete`, `argocd app sync` (as mutation), `flux reconcile --with-source`, `cub * create/update/delete`. Receipts are evidence, never recommendations to mutate. `FilterNextSteps` (in `pkg/agent/receipt_predicates.go`) rejects mutating `actionType` / `nextCommand` at receipt-emit time as defense in depth.
- **`TestReceiptPackageReadOnlyClient`:** the static guard in `cmd/cub-scout/receipt_readonly_test.go` greps every `receipt*.go` source file and fails the build if any mutating K8s client method appears. Receipts are categorically read-only.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Produce a receipt about this resource" | `cub-scout receipt verify <kind>/<name> -n <ns>` | Auto-detects the predicate from owner + signals. Defaults to applied-matches-spec for Argo/Flux/ConfigHub-owned resources with a resolved git anchor. |
| "Render a saved receipt" | `cub-scout receipt show <path>` | ASCII or JSON. Does NOT verify the fingerprint — works on tampered receipts for forensic inspection. |
| "Is this receipt authentic?" | `cub-scout receipt validate <path>` | Recomputes the fingerprint and compares. Exit 0 OK / 1 mismatch / 2 I/O. JSON output for CI. |
| "What receipts do I have locally?" | `cub-scout receipt list` | Walks `$CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME/cub-scout/receipts → $HOME/.local/share/cub-scout/receipts`. Sortable, newest first. |

## The predicates

cub-scout v1 ships three predicates. Each maps a *required input signal* to a *receipt-level verdict*:

| Predicate | Required signal | Verdict logic |
|---|---|---|
| `applied-matches-spec` | Argo/Flux/ConfigHub owner + controller-resolved git anchor | PASS on `controller-drift` cause; BLOCK on `manual-edit` cause or anchor mismatch; INCONCLUSIVE on missing anchor or unknown cause. |
| `source-truth-pass` | `--strategy <name>` (one of 9) + connected-mode ConfigHub auth | Mirrors `compare source-truth` Status → Verdict. Strategy mismatch between caller and evidence → BLOCK. |
| `no-manual-edits-since` | `--since <RFC3339>` cutoff | PASS when no `kubectl-*` writer touched `managedFields` after the cutoff. BLOCK on any late interactive write. INCONCLUSIVE on missing managedFields or nil-Time entries (parse-don't-guess). |

cub-scout NEVER infers a strategy or a cutoff. `source-truth-pass` requires `--strategy`; `no-manual-edits-since` requires `--since`. Pass them or the CLI errors upfront.

## Verdicts

Every receipt carries one of four:

| Verdict | Meaning |
|---|---|
| `PASS` | Evidence supports the claim |
| `WATCH` | Evidence is ambiguous; situation needs monitoring |
| `BLOCK` | Evidence contradicts the claim |
| `INCONCLUSIVE` | Evidence missing or unavailable; always carries one or more `omissions[]` entries |

INCONCLUSIVE is **not** failure — it's an honest "I don't have enough to claim PASS." Pilot and similar acceptance kernels treat it as `ASK` (need human or richer evidence).

## The loop

1. **Pick the predicate.** Most users mean `applied-matches-spec` (default for Argo/Flux/ConfigHub-owned resources with a resolved anchor). If the user mentions a delivery strategy, they want `source-truth-pass`. If they mention a freeze window or cutoff timestamp, they want `no-manual-edits-since`.
2. **Decide standalone vs connected.** Receipts work in both modes; connected mode adds the `confighub-unit://` subject + ConfigHub-side evidence. Source-truth-pass requires connected mode.
3. **Invoke verify.** Use `--format json` for CI / agent consumption. Use `--save` to land the artifact in the local store (immutable; refuses overwrite).
4. **Read the verdict + omissions.** The verdict is the headline; `omissions[]` is the "what I deliberately did not claim" list. Empty omissions = comprehensive coverage. Non-empty = honest gaps.
5. **Persist or attach.** `--out <path>` writes a specific file; `--save` writes into the store under the canonical name. CI gates that need to attach the artifact to a build record use `--out`.
6. **Hand off.** If the verdict is BLOCK and the user wants to *act*, refuse to mutate. Point them at the appropriate `cub` skill (for ConfigHub state changes), at `kubectl get --show-managed-fields` (to investigate a manual-edit BLOCK), or at the original-author path (commit the change back to git).

## Worked examples

### A: applied-matches-spec, standalone

The default path for an Argo-owned Deployment. Works without ConfigHub auth.

```bash
$ cub-scout receipt verify deploy/api -n prod
Receipt: applied-matches-spec
  Verdict: PASS
  Claim:   applied matches spec at apps/prod/api
  Scope:   Deployment/api in prod
  By:      cub-scout v2.3.0 at 2026-05-22T10:30:00Z
  Spec:    https://github.com/org/platform-config @abc123def456 path=apps/prod/api

Subjects
  - k8s-live://apps/v1/Deployment/prod/api  sha256:595f08ae073d…

Evidence (attribution)
  cause:       controller-drift
  managerHint: argocd-controller
  gitSource:   https://github.com/org/platform-config @abc123def456 path=apps/prod/api

Omissions (deliberate non-claims)
  - confighub-unit-subject  — standalone-mode build; no ConfigHub auth available to fetch unit subject  [info]

Next steps (read-only)
  - [read-only] controller is reconciling; spec anchor matches the resolved git source
      → cub-scout explain

Fingerprint: sha256:252ef3104d0a3cc8ac62e7bde40cf6174b6f045d8ed2b97c6512781d8cb690f4
```

Save it to disk for a build record:

```bash
$ cub-scout receipt verify deploy/api -n prod --format json --out api-prod.receipt.json
```

Or land it in the local store for casual accumulation:

```bash
$ cub-scout receipt verify deploy/api -n prod --save
saved: /home/user/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__applied-matches-spec__Deployment-api__252ef3104d0a.receipt.json
```

### B: source-truth-pass, declared strategy (connected mode)

```bash
$ cub-scout receipt verify deploy/api -n prod --strategy git-argo --format json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {"name": "k8s-live://apps/v1/Deployment/prod/api", "digest": {"sha256": "..."}},
    {"name": "confighub-unit://payments-api@rev=42", "digest": {"sha256": "..."}}
  ],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "predicateName": "source-truth-pass",
    "verdict": "PASS",
    "evidence": {
      "sourceTruth": {
        "declared_strategy": "Git -> Argo -> Kubernetes",
        "status": "PASS",
        "source_truth": "AGREED",
        "surfaces": { ... }
      }
    },
    "omissions": [],
    "fingerprint": "sha256:..."
  }
}
```

Strategy mismatch is BLOCK — never silently honor the caller's strategy over what the evidence records:

```bash
$ cub-scout receipt verify deploy/api -n prod --strategy git-argo
# ... evidence carries confighub-oci-argo, not git-argo ...
Verdict: BLOCK
Omissions
  - strategy-mismatch  — receipt was asked to verify under strategy "git-argo" but source-truth evidence carries "ConfigHub -> OCI -> Argo -> Kubernetes"
```

### C: no-manual-edits-since freeze gate (standalone)

```bash
$ cub-scout receipt verify deploy/api -n prod --since 2026-05-22T00:00:00Z --save
saved: /home/user/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__no-manual-edits-since__Deployment-api__abc123.receipt.json

$ cub-scout receipt show ~/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__no-manual-edits-since__Deployment-api__abc123.receipt.json
Receipt: no-manual-edits-since
  Verdict: PASS
  Claim:   no manual edits to Deployment/api in prod since 2026-05-22T00:00:00Z
```

A late `kubectl edit` produces BLOCK and names the offending writer:

```
Verdict: BLOCK
Next steps (read-only)
  - [read-only] interactive writer wrote managedFields after the cutoff (2026-05-22T00:00:00Z): kubectl-edit@2026-05-22T12:00:00Z — investigate the unplanned edit before any apply
      → kubectl get --show-managed-fields
```

### D: validate + list

```bash
$ cub-scout receipt validate ./api-prod.receipt.json
Fingerprint OK: ./api-prod.receipt.json
  sha256:252ef3104d0a3cc8ac62e7bde40cf6174b6f045d8ed2b97c6512781d8cb690f4

$ cub-scout receipt list
Store: /home/user/.local/share/cub-scout/receipts

VERDICT        PREDICATE               SCOPE                                     VERIFIED-AT             PATH
PASS           applied-matches-spec    Deployment/api in prod                    2026-05-22T10:30:00Z    2026-05-22T10-30-00Z__applied-matches-spec__Deployment-api__252ef3104d0a.receipt.json
BLOCK          no-manual-edits-since   Deployment/api in prod                    2026-05-22T08:00:00Z    2026-05-22T08-00-00Z__no-manual-edits-since__Deployment-api__abc123.receipt.json
```

JSON list output for CI consumption (filter by verdict):

```bash
$ cub-scout receipt list --format json | jq '.[] | select(.verdict == "BLOCK")'
```

## Output evidence

What every receipt carries (the structured surface a downstream consumer pattern-matches on):

| Field | Meaning |
|---|---|
| `predicate.predicateName` | One of `applied-matches-spec` / `source-truth-pass` / `no-manual-edits-since` |
| `predicate.verdict` | One of `PASS` / `WATCH` / `BLOCK` / `INCONCLUSIVE` |
| `predicate.scope` | `kind / name / namespace / cluster` |
| `predicate.verifier` | `{tool, version}` — always `cub-scout` plus the build tag |
| `predicate.verifiedAt` | RFC 3339 timestamp |
| `predicate.spec.anchor` | Git repo + revision + path (applied-matches-spec only) |
| `predicate.evidence.attribution` | `cause` / `managerHint` / `gitSource` — cub-scout's existing attribution layer |
| `predicate.evidence.sourceTruth` | Source-truth derivation surfaces (source-truth-pass only) |
| `predicate.omissions[]` | Structured list of what the receipt deliberately does NOT claim — empty array means comprehensive coverage |
| `predicate.nextSteps[]` | Read-only follow-up hints (mutating verbs filtered at emit time) |
| `predicate.fingerprint` | `sha256:` SHA-256 over RFC 8785 canonical JSON of the full Statement minus only this field |

## Storage convention

The local receipt store is a flat directory of `*.receipt.json` files keyed by a deterministic sortable name:

```
<verifiedAt-rfc3339-safe>__<predicate>__<kind>-<name>__<short-fingerprint>.receipt.json
```

Resolved in priority order:

1. `--save-dir <path>` on verify, `--dir <path>` on list (explicit override)
2. `$CUB_SCOUT_RECEIPTS_DIR` env var
3. `$XDG_DATA_HOME/cub-scout/receipts`
4. `$HOME/.local/share/cub-scout/receipts`

The store is **immutable**: re-running verify on the same resource at the same instant with the same fingerprint produces the same canonical filename → "already saved" warning and exit 0. The on-disk artifact never changes.

## v2 surface (CI / agent shaped)

Three v2 extensions layer on top of the v1 envelope without changing the wire format:

- **`--fail-on <verdict-list>` on `receipt verify`** (`#451`) — exit non-zero (code 2) when the verdict matches. Accepts `WATCH`, `BLOCK`, `INCONCLUSIVE` (comma-separated) or the sugar `any-non-pass`. The receipt is still printed / saved / written to `--out` — the gate fires AFTER the artifact is durable.
- **`--input-attestation <path>` chained receipts** (`#448` chained half) — repeatable; each referenced receipt is loaded + fingerprint-verified + attached to the new receipt's `inputAttestations[]`. Tampered references refused at chain-construction time. The downstream fingerprint covers the field by construction.
- **`cub-scout watch --emit-receipt-on <event-types>`** (`#449`) — attaches a receipt to each matching event payload inline (JSONL or webhook). v1 of the flag builds receipts for `drift.detected` and `ownership.changed` (highest signal); other event types pass through silently. Failures are non-fatal.

```bash
# CI gate
./cub-scout receipt verify deploy/api -n prod --fail-on any-non-pass

# Chain a per-resource receipt with an upstream "release gate passed" one
./cub-scout receipt verify deploy/api -n prod \
  --input-attestation ./release-gate.receipt.json \
  --save

# Streaming acceptance: each drift event becomes a fingerprinted receipt
./cub-scout watch -n prod \
  --output-file ./acceptance.jsonl \
  --emit-receipt-on drift.detected,ownership.changed
```

## Not yet in cub-scout

Future directions worth flagging — none of these are implemented today:

- Cryptographic signing as a hardening layer on top of fingerprint-only — DSSE wrapped in a Sigstore Bundle (cosign keyless + ed25519 fallback) is one candidate design; the wire format is intentionally additive-friendly so signatures can land without an envelope change
- Aggregate-with-discovery half of `#448` — `cub-scout receipt verify --scope namespace/<ns>` that emits N per-resource receipts + 1 aggregate via `synthetic-aggregate://` subject + max-severity verdict synthesis
- Backpressure / batching for high-frequency `--emit-receipt-on` events (Codex round-1 open question on `#449`)
- `resource.discovered` / `scan.finding` event-type support for `--emit-receipt-on` (would flood on first poll; deferred pending the backpressure design)

Shipping releases use fingerprint-only integrity; future hardening is honest about what's not yet there — the contract is in `docs/reference/json-contracts.md` § Receipt Contract.

## References

- Contract: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § Receipt Contract
- Locked design: [`docs/proposals/receipts-way-forward.md`](../../docs/proposals/receipts-way-forward.md)
- Examples: [`examples/receipts/`](../../examples/receipts/) — 8 canonical receipts across the three predicates, generated deterministically by `tools/gen-receipt-examples`
- Implementation: `pkg/agent/receipt*.go`, `cmd/cub-scout/receipt*.go`
- Read-only triad: [`scout-attribute`](../scout-attribute/SKILL.md) (companion evidence surface), [`scout-compare`](../scout-compare/SKILL.md) (live comparison sibling)
