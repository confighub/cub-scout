# How-To: Use Receipts End-to-End

cub-scout receipts are typed, fingerprinted, immutable evidence artifacts. This how-to walks through the **operational lifecycle** of a receipt in a real CD pipeline — from pre-deploy gate, through chained multi-stage delivery, through namespace-wide audit, through real-time event-driven emission.

For reference material, see:
- [JSON Contracts § Receipt Contract](../reference/json-contracts.md)
- [Command Reference § receipt verify](../reference/commands.md)
- [CLI Contract § cub-scout receipt](../reference/cli-contract.md)
- [Worked examples](../../examples/receipts/) — 4 example directories with paste-ready CI snippets

For Pilot / acceptance-judge consumer integrations, see the `pilot-*` skill files at [`skills/`](../../skills/).

## What this how-to covers

| Stage | Verb | Use case |
|---|---|---|
| 1. Pre-deploy gate | `receipt verify --fail-on` | Block a deploy if source-truth disagrees |
| 2. Audit-grade chain | `receipt verify --input-attestation` | Multi-stage delivery with tamper-evident lineage |
| 3. Namespace conformance | `receipt verify --scope namespace/<ns>` | One aggregate verdict across many resources |
| 4. Real-time emission | `watch --emit-receipt-on` | Each watch event carries an inline receipt |
| 5. Reading back | `receipt show / validate / list` | Audit + tamper-detection later |

All five stages are read-only by construction. Receipts emit; they never mutate the cluster or ConfigHub.

## Stage 1 — Pre-deploy gate

The shape: your CD pipeline is about to apply a release. You want to block it if the proposed state diverges from intent.

```bash
# Run before `argocd app sync` / `flux reconcile` / `cub unit apply`
cub-scout receipt verify deploy/payments-api \
  -n prod \
  --strategy git-argo \
  --fail-on any-non-pass \
  --save \
  --out gate.receipt.json
```

- `--strategy git-argo` declares the source of truth (one of 9; see [`source-truth-strategies`](../../skills/references/source-truth-strategies.md))
- `--fail-on any-non-pass` exits **2** if the receipt verdict is WATCH / BLOCK / INCONCLUSIVE
- `--save` writes to the immutable local store; `--out gate.receipt.json` writes a copy for the release ticket
- The artifact is **persisted BEFORE the gate fires** — failures still produce the receipt

### Exit codes

| Code | Meaning | What CI should do |
|---|---|---|
| 0 | PASS, or verdict not in `--fail-on` set | Continue the pipeline |
| 2 | Verdict matched `--fail-on` | Halt; preserve `gate.receipt.json` for review |
| 1 | Operational error (cluster unreachable, bad flag value) | Retry or fail; the gate did NOT fire |

`--fail-on PASS` is rejected upfront — gating on a passing receipt is a workflow bug.

**Worked example:** [`examples/receipts/ci-gate/`](../../examples/receipts/ci-gate/) has paste-ready GitHub Actions / GitLab CI / Jenkins snippets.

## Stage 2 — Audit-grade chain

The shape: your release rolls through several stages (pre-deploy baseline → at-deploy → post-deploy). You want a **tamper-evident chain** of receipts so the audit trail shows the full delivery path.

```bash
# Stage 1: pre-deploy baseline
cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --at-commit <pre-sha> \
  --save \
  --out stage1-pre.receipt.json

# (Apply the release here — Argo / Flux / cub.)

# Stage 2: at-deploy, chained to stage 1
cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --at-commit <release-sha> \
  --input-attestation stage1-pre.receipt.json \
  --save \
  --out stage2-at.receipt.json

# Stage 3: post-deploy verification, chained to stage 2
cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --at-commit <release-sha> \
  --input-attestation stage2-at.receipt.json \
  --save \
  --out stage3-post.receipt.json
```

### The chain integrity property

Each `--input-attestation` is **verified at chain-construction time**:

- Empty / malformed fingerprint → error
- Recomputed fingerprint doesn't match the stamped value → error ("refusing to chain a receipt whose fingerprint doesn't verify")

The downstream receipt's fingerprint covers `inputAttestations[]` by construction. Tampering with `stage1-pre.receipt.json` after the fact invalidates `stage2-at.receipt.json`'s recomputed fingerprint, which in turn invalidates `stage3-post.receipt.json`'s.

The verify-on-construction property is enforced via a typed wrapper at the API boundary: programmatic callers (not just the CLI) cannot bypass the verify step.

### Reading the chain timeline

- `PASS → PASS → PASS` reads as: clean delivery; no divergence at any stage
- `PASS → BLOCK → PASS` reads as: a manual-edit episode that recovered
- `PASS → BLOCK → BLOCK` reads as: live state never recovered to intent

**Worked example:** [`examples/receipts/chained/`](../../examples/receipts/chained/).

## Stage 3 — Namespace conformance

The shape: a quarterly compliance run or pre-promotion audit needs **one verdict** over many resources, with the per-resource detail still available.

```bash
# Auto-discover Deployments / StatefulSets / DaemonSets / CronJobs / Jobs
cub-scout receipt verify --scope namespace/prod \
  --strategy git-argo \
  --save \
  --out aggregate.receipt.json
```

Or for an explicit set:

```bash
cub-scout receipt verify \
  deploy/api,deploy/worker,statefulset/db \
  -n prod \
  --strategy git-argo \
  --save
```

Both shapes emit **N per-resource receipts (JSONL)** followed by **1 aggregate receipt (pretty-printed JSON)**. `--fail-on` applies to the **aggregate verdict**.

### Aggregate verdict synthesis

The default policy is **max-severity**: `BLOCK > INCONCLUSIVE > WATCH > PASS`. INCONCLUSIVE outranks WATCH because missing evidence is *louder* than ambiguous evidence — the consumer needs to fix the gap before claiming coverage.

| Per-resource verdicts | Aggregate verdict |
|---|---|
| All PASS | PASS |
| Some WATCH, rest PASS | WATCH |
| Any INCONCLUSIVE | INCONCLUSIVE + `aggregate-partial-coverage` omission |
| Any BLOCK | BLOCK |

The aggregate's subject is `synthetic-aggregate://sha256/<id>` — deterministically derived from the inputs (order-independent; reordering produces the same subject).

### Per-resource failures are non-fatal

If a per-resource verify fails (404 on a resource, load error), the aggregate is composed from the **successful subset** and records an `aggregate-partial-coverage` omission entry. The CLI prints a stderr warning per failure.

**Worked example:** [`examples/receipts/aggregate/`](../../examples/receipts/aggregate/).

## Stage 4 — Real-time emission

The shape: an acceptance-judge tool (e.g., Pilot) subscribes to your watch stream and renders a verdict per event without a synchronous call back.

```bash
cub-scout watch -n prod \
  --webhook https://acceptance.acme/events \
  --output-file /var/log/cub-scout-watch.jsonl \
  --emit-receipt-on all \
  --emit-receipt-batch-cap 10 \
  --severity warning,critical
```

### Event types

All four known watch event types build receipts:

| Event type | Predicate auto-detected | Typical reaction |
|---|---|---|
| `drift.detected` | `applied-matches-spec` | The drift event already implies a verdict question |
| `ownership.changed` | `applied-matches-spec` | Owner shifts often indicate delivery-chain changes |
| `resource.discovered` | `applied-matches-spec` | Captures live state at first observation |
| `scan.finding` | `applied-matches-spec` | Records resource state at finding time |

### Per-poll backpressure

`--emit-receipt-batch-cap N` (default 10): when a single poll produces more than N receipt-eligible events, the first N get receipts; the rest emit with the `receipt` key omitted plus a single stderr summary line. The cap is per-poll, so quiet polls between bursts don't accumulate suppression state.

| Value | Behaviour |
|---|---|
| Default `10` | Bounds first-poll bursts |
| `0` | Disables receipt-build entirely while keeping the flag explicit |
| `1000` | Effectively disables the cap |

### `omitempty` wire shape

When a receipt isn't attached (build failure or backpressure cap), the `receipt` key is **omitted from the JSON entirely**, not emitted as `"receipt": null`. Consumer code must check key presence:

```python
# Correct
if "receipt" in event:
    verdict = event["receipt"]["predicate"]["verdict"]

# Incorrect — raises KeyError when receipt was suppressed
verdict = event["receipt"]["predicate"]["verdict"]
```

**Worked example:** [`examples/receipts/watch-emit/`](../../examples/receipts/watch-emit/).

## Stage 5 — Reading back

Receipts are durable. Six months later, an auditor can:

```bash
# Validate fingerprint integrity
cub-scout receipt validate gate.receipt.json
# exit 0 = OK; 1 = mismatch (tampered or corrupted); 2 = I/O (file missing)

# Render the human-readable form
cub-scout receipt show gate.receipt.json --format ascii

# Walk the local store
cub-scout receipt list --format json | jq '[.[] | select(.verdict == "BLOCK")]'

# List with custom filters
cub-scout receipt list --dir /path/to/audit-archive --format json
```

### Receipt store

`--save` writes to a resolved store directory:

1. `--save-dir <path>` (explicit override)
2. `$CUB_SCOUT_RECEIPTS_DIR` environment variable
3. `$XDG_DATA_HOME/cub-scout/receipts`
4. `$HOME/.local/share/cub-scout/receipts`

Files are immutable — `SaveStatement` uses `O_EXCL` atomic create and refuses to overwrite. Filenames are canonical:

```
<verifiedAt>__<predicate>__<kind>-<name>__<short-fingerprint>.receipt.json
2026-05-25T10-30-00Z__applied-matches-spec__Deployment-api__a1b2c3d4e5f6.receipt.json
```

### `--out` vs `--save`

| Flag | Path | Mutability | Use when |
|---|---|---|---|
| `--out <path>` | Caller picks | Overwrites existing files | Ad-hoc / CI temp artifact |
| `--save` | Resolved store | Immutable (`O_EXCL`) | Long-lived audit trail |

`--out` paths that fall under the resolved store directory are **rejected** — the store's immutability invariant can't be bypassed via `--out`.

## Standalone vs connected

Most stages above work in **both modes**:

| Mode | What works | What's narrower |
|---|---|---|
| **Standalone** (cluster only) | `applied-matches-spec` + `no-manual-edits-since` predicates; chained receipts; namespace aggregate; watch emit | No `source-truth-pass` (needs ConfigHub); no `confighub-unit://` subject (receipt records an `OmissionConfigHubUnitSubject` entry) |
| **Connected** (cluster + `cub auth login`) | All of the above + `source-truth-pass`; `confighub-unit://` second subject; `bindingSource` per field on attribution | Full surface |

Standalone-mode receipts honestly record what's missing via structured `omissions[]` entries — they don't silently degrade.

Run `cub-scout status` to confirm which mode you're in:

```bash
$ cub-scout status
Cluster:       kind-cub-scout (connected: kubectl)
ConfigHub:     not connected
Mode:          standalone
```

## Read-only triad

cub-scout receipts are evidence. They never:

- Mutate the cluster (`kubectl apply / edit / patch / delete`)
- Mutate ConfigHub (`cub * create / update / delete`)
- Trigger controller syncs (`argocd app sync`, `flux reconcile`)
- Recommend mutating next-steps (`FilterNextSteps` rejects `actionType=mutating` at receipt-emit time)

Three enforcement layers guard this:

1. `scripts/check-readonly.sh` — CI grep for mutating K8s client calls outside an allow-list
2. `TestReceiptPackageReadOnlyClient` — static-grep that scans every `*receipt*.go` source file
3. `FilterNextSteps` — runtime filter that drops mutating `actionType` / `nextCommand` before stamping

Pilot, ConfigHub, or whatever downstream tool you use can act on a receipt — but the receipt itself is purely a witness.

## Common workflows

| Goal | Stages to compose |
|---|---|
| CI gate one resource on every deploy | 1 |
| Audit trail for one release across stages | 1 → 2 |
| Quarterly compliance over a namespace | 3 |
| Real-time Pilot integration | 4 |
| Postmortem evidence pack | 2 (chained pre/at/post receipts) |
| All of the above | 1 → 2 → 4 → 5 (read back during audit) |

## See also

- **Reference**: [JSON Contracts § Receipt Contract](../reference/json-contracts.md), [commands.md § receipt verify](../reference/commands.md), [cli-contract.md § cub-scout receipt](../reference/cli-contract.md), [watch-events.md](../reference/watch-events.md)
- **Examples**: [`examples/receipts/ci-gate/`](../../examples/receipts/ci-gate/), [`chained/`](../../examples/receipts/chained/), [`aggregate/`](../../examples/receipts/aggregate/), [`watch-emit/`](../../examples/receipts/watch-emit/)
- **Skills**: [`scout-verify`](../../skills/scout-verify/SKILL.md) (cub-scout operator side); [`pilot-cd-gate`](../../skills/pilot-cd-gate/SKILL.md) and 8 sibling Pilot consumer skills
- **Design**: [`docs/proposals/receipts-way-forward.md`](../proposals/receipts-way-forward.md) — locked design synthesis
- **Issues**: `#446` (parent), `#451` (`--fail-on`), `#448` (chained + aggregate), `#449` (watch emit + backpressure), `#444` (Pilot consumer skills) — all closed as of v2.3.0
