# Multi-Cluster Fleet Promotion Proof (Offline & ConfigHub Graduation)

This guide explains how to prove that a specific revision landed across an
entire promotion pipeline (`dev` → `staging` → `prod-east` → `prod-west`) using
strictly read-only, single-cluster `cub-scout` witnesses and offline Ed25519
verification — and how the exact same `signed-receipt/v1` artifact graduates
into ConfigHub `G4` `ChangeWorkflow` attestations once upstream `D6` lands.

Runnable example fixtures and verification script:
[`examples/fleet-promotion-proof/`](../../examples/fleet-promotion-proof/README.md).

---

## Architecture: One Witness Per Cluster, Zero Fleet Database

`cub-scout` stays strictly single-cluster and read-only. It never maintains a
long-lived watch cache or a second fleet database. Instead, fleet promotion
proof separates **per-cluster observation** from **offline aggregation**:

```text
dev        ─► cub scout landed --verify-pod-images ─► receipt sign ─► receipts/dev.json
staging    ─► cub scout landed --verify-pod-images ─► receipt sign ─► receipts/staging.json
prod-east  ─► cub scout landed --verify-pod-images ─► receipt sign ─► receipts/prod-east.json
prod-west  ─► cub scout landed --verify-pod-images ─► receipt sign ─► receipts/prod-west.json
                                                                             │
                                                                             ▼
               cub scout receipt verify-fleet --inputs ./receipts --pubkey pub.pem \
                 --revision <sha> --require-clusters dev,staging,prod-east,prod-west
                                                                             │
                                                                             ▼
                                        fleet-receipt-verification/v1 (answer.state)
```

Key properties:

- **Strictly read-only:** Each cluster witness only reads controller and Pod
  state (`get`, `list`, `watch`). `receipt sign` and `receipt verify-fleet`
  perform zero Kubernetes calls and zero network calls.
- **Independent Pod image witness:** `--verify-pod-images` checks running Pod
  `status.containerStatuses[].imageID` digests directly so a stale Pod cannot
  produce `landed` even if the GitOps controller reports `Synced`/`Ready`.
- **Deterministic & zero LLM:** Canonical JSON + SHA-256 digest + Ed25519
  signature (`signed-receipt/v1`) verified with Go standard library crypto.
- **Compact `answer` envelope:** `receipt verify-fleet --format json` emits a
  standard `answer` envelope (`<= 700` bytes) using the same four landing
  states (`landed`, `converging`, `not-landed`, `not-observable`).

---

## Step-by-Step Standalone Workflow

### 1. Generate a Signing Key Pair (Once)

```bash
# Primary cub plugin form
cub scout receipt keygen \
  --out-private /path/to/secure/receipt-priv.pem \
  --out-public ./fleet-pub.pem

# Standalone binary form
cub-scout receipt keygen \
  --out-private /path/to/secure/receipt-priv.pem \
  --out-public ./fleet-pub.pem
```

Keep the PKCS#8 private key (`0600`) in your CI secret store or signing runner;
distribute only the PKIX public key (`fleet-pub.pem`) to verifiers. Never
commit private keys to Git.

### 2. Emit One Signed Receipt Per Cluster

Run `landed --verify-pod-images` against each cluster in your promotion
pipeline (`dev`, `staging`, `prod-east`, `prod-west`) and pipe directly into
`receipt sign`:

```bash
REV="7c9e2a4f"
IMG_DIGEST="sha256:4f8b9c2e1d3a5b7c9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d"

for cluster in dev staging prod-east prod-west; do
  cub scout landed deploy/api -n payments \
    --revision "$REV" \
    --verify-pod-images \
    --image "$IMG_DIGEST" \
    --kube-context "$cluster" \
    --wait 60s \
    --format json \
  | cub scout receipt sign \
      --input - \
      --key /path/to/secure/receipt-priv.pem \
      -o "./receipts/${cluster}.json"
done
```

### 3. Verify Fleet-Wide Delivery Offline

Collect the four signed JSON files (for example as CI artifacts) and verify
them in a single offline command:

```bash
cub scout receipt verify-fleet \
  --inputs ./receipts \
  --pubkey ./fleet-pub.pem \
  --revision 7c9e2a4f \
  --require-clusters dev,staging,prod-east,prod-west \
  --max-age 30m \
  --format json
```

When all four clusters carry valid signatures, match revision `7c9e2a4f`, and
report `landed`, `verify-fleet` exits `0` with `answer.state == "landed"` and
`answer.reason == "fleet-landed"`.

You can run the bundled 4-cluster example immediately without any cluster:

```bash
go build -o ./cub-scout ./cmd/cub-scout
./examples/fleet-promotion-proof/verify.sh
```

---

## Exact Failure Modes (Fail-Closed Contract)

`receipt verify-fleet` fails closed (exits `1`) whenever the aggregated
`answer.state` is not `landed`, and exits `2` on invalid input paths or
unreadable key files. Failure precedence is deterministic:

| Failure Mode | `answer.state` | `answer.reason` | Exit Code | Details Surfaced |
|---|---|---|---|---|
| **Tampered receipt payload** | `not-landed` | `fleet-signature-invalid` | `1` | `summary.invalidReceipts >= 1`; failing cluster has `valid: false`, `reason: "signature-invalid"`, `sig=INVALID` |
| **Wrong public key** | `not-landed` | `fleet-signature-invalid` | `1` | All receipts signed by another key fail Ed25519 verification (`valid: false`, `reason: "signature-invalid"`) |
| **Revision mismatch** | `not-landed` | `fleet-revision-mismatch` | `1` | `summary.revisionMismatch >= 1`; mismatched cluster has `revisionMatch: false`, `reason: "revision-mismatch"` |
| **Cluster reported `not-landed`** | `not-landed` | `fleet-not-landed` | `1` | `summary.notLanded >= 1`; preserves the per-cluster landing failure reason (e.g., Pod image mismatch or broken link) |
| **Missing required cluster** | `not-observable` | `fleet-missing-clusters` | `1` | `summary.missingClusters >= 1`; missing cluster gets `missing: true`, `reason: "missing-required-cluster"`, plus an `answer.unknowns[]` entry (`kind: "not-observable"`) |
| **Cluster reported `not-observable`** | `not-observable` | `fleet-not-observable` | `1` | `summary.notObservable >= 1`; adds an `answer.unknowns[]` entry (`kind: "not-observable"`) |
| **Converging cluster** | `converging` | `fleet-converging` | `1` | `summary.converging >= 1`; rollout is still progressing on at least one cluster (e.g., `prod-west`) |
| **Stale `observedAt` (`--max-age`)** | *(preserves cluster state)* | *(preserves fleet reason)* | *(depends on `answer.state`)* | Marks `stale: true` (`[STALE]` in ASCII), increments `summary.staleReceipts`, and appends an explicit `answer.unknowns[]` entry with `kind: "stale"` and the measured age vs `--max-age` |

> **Enforcing strict freshness in CI:** When `--max-age` is set, stale receipts
> are always surfaced explicitly in `summary.staleReceipts` and
> `answer.unknowns[]` (`kind: "stale"`). A strict CI promotion gate can reject
> stale evidence by asserting `summary.staleReceipts == 0` alongside exit `0`
> and `answer.state == "landed"`.

---

## Graduating to ConfigHub `G4` `ChangeWorkflow` Attestations (Upstream `D6`)

The standalone offline workflow above uses the exact same `signed-receipt/v1`
envelope and per-cluster `(cluster/Target, revision, observedAt, state)` facts
designed for connected ConfigHub fleet promotion (`docs/proposals/vision-3.x.md`,
section 3.5):

1. **Same per-cluster witness, zero lock-in:** Start today with standalone
   `cub-scout landed --verify-pod-images` + `receipt sign` + `receipt verify-fleet`
   in CI with zero server dependency.
2. **No writer conflict on `Release.LiveStatus`:** Even in connected mode,
   `cub-scout` never writes to `Release.LiveStatus` (owned by `argobot`; `#641`
   remains rejected) and never runs a competing fleet database (`#539` remains
   `not-planned`).
3. **Clean graduation once upstream `D6` lands (`#600` + `G4`):**
   - Each cluster's read-only `cub-scout` witness (CLI or `cub-scout bot`)
     publishes its signed `signed-receipt/v1` payload as an opt-in,
     off-by-default ConfigHub observed fact (`#600`) and/or **ConfigHub `G4`
     Release Attestation** tagged with `(Target, Release/Revision, observedAt)`.
   - A ConfigHub `ChangeWorkflow` promotion gate (`dev` → `staging` →
     `prod-east` → `prod-west`) requires passing `G4` attestations for the
     prior stage's clusters and revision before authorizing promotion to the
     next stage.
   - Because the attestation payload is the same `signed-receipt/v1` format
     verified offline by `receipt verify-fleet`, auditors and air-gapped CI
     runners can independently re-verify every ConfigHub-stored attestation
     offline against the signing public key.
