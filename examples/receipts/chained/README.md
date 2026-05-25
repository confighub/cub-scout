# Receipt Example: Chained Receipts with `--input-attestation`

This example walks through using `cub-scout receipt verify --input-attestation` to compose **multi-stage delivery chains** as a tamper-evident DAG of receipts. Each stage's receipt references the prior stage by digest. At chain-construction time, each upstream's fingerprint is verified against its stamped value — a tampered upstream is refused. After construction, post-hoc upstream tampering is detected only when a verifier walks the chain and re-validates each upstream's fingerprint independently; the downstream's own fingerprint covers the digest *reference*, not the upstream's bytes.

Issue: [`#448`](https://github.com/confighub/cub-scout/issues/448).
Parent capability: [`#446`](https://github.com/confighub/cub-scout/issues/446).
Shipped in: [`#463`](https://github.com/confighub/cub-scout/pull/463) (chained half) + Codex round-6 API-boundary fix.

## The shape

```
   stage 1: pre-deploy (baseline)
        │
        │  receipt verify --strategy git-argo --at-commit <pre-sha>
        │    --save --out stage1-pre.receipt.json
        │  → verdict: PASS (or whatever the baseline is)
        │
        ▼
   stage 2: at-deploy (release fires)
        │
        │  receipt verify --strategy git-argo --at-commit <release-sha>
        │    --input-attestation stage1-pre.receipt.json     ◄── chain
        │    --save --out stage2-at.receipt.json
        │  → verdict: depends on whether the release landed
        │
        ▼
   stage 3: post-deploy (verification)
        │
        │  receipt verify --strategy git-argo --at-commit <release-sha>
        │    --input-attestation stage2-at.receipt.json      ◄── chain
        │    --save --out stage3-post.receipt.json
        │  → verdict: PASS if reconciliation finished cleanly
        │
        ▼
   audit:
      receipt validate stage1-pre.receipt.json    # exit 0
      receipt validate stage2-at.receipt.json     # exit 0
      receipt validate stage3-post.receipt.json   # exit 0
      → the chain holds; the delivery story is tamper-evident
```

Each stage's receipt includes `predicate.inputAttestations[]` referencing the prior stage by SHA-256 digest. The downstream fingerprint covers `inputAttestations[]` by construction — tampering with `stage1-pre.receipt.json` after the fact invalidates `stage2-at.receipt.json`'s recomputed fingerprint, which in turn invalidates `stage3-post.receipt.json`'s.

## Chain integrity property

`cub-scout receipt verify --input-attestation <path>` **verifies the upstream receipt's fingerprint at chain-construction time**:

- Empty fingerprint → error
- Malformed fingerprint (not `sha256:...`) → error
- Fingerprint hex < 12 chars → error
- Recomputed fingerprint doesn't match the stamped value → error ("refusing to chain a receipt whose fingerprint doesn't verify")

The verify-on-construction property is enforced via the `VerifiedAttestationRef` typed wrapper (Codex round-6 P1 fix in `#463`): programmatic callers cannot bypass the verify step. The CLI path goes through `BuildAttestationRefsFromPaths` which loads each path, recomputes the fingerprint, and refuses tampered receipts.

## Multi-stage CD pipeline

```bash
#!/usr/bin/env bash
# Multi-stage release verification.
# stage 1: pre-deploy baseline (pinned to the SHA the team thought was live)
# stage 2: at-deploy (the release fires and Argo / Flux reconciles)
# stage 3: post-deploy verification (matches the new SHA + bindings)

set -euo pipefail

RESOURCE="deploy/payments-api"
NAMESPACE="prod"
STRATEGY="git-argo"
PRE_SHA="9e7d12fa"     # baseline SHA before this release
RELEASE_SHA="abc123de"  # new SHA Argo is applying
OUT_DIR="./release-evidence/$(date -u +%Y-%m-%dT%H%M%S)"

mkdir -p "$OUT_DIR"

# Stage 1 — pre-deploy baseline
cub-scout receipt verify $RESOURCE -n $NAMESPACE \
  --strategy $STRATEGY \
  --at-commit $PRE_SHA \
  --save \
  --out "$OUT_DIR/stage1-pre.receipt.json"

# (Argo / Flux apply happens here; settle window 30-180s)
sleep 60

# Stage 2 — at-deploy
cub-scout receipt verify $RESOURCE -n $NAMESPACE \
  --strategy $STRATEGY \
  --at-commit $RELEASE_SHA \
  --input-attestation "$OUT_DIR/stage1-pre.receipt.json" \
  --save \
  --out "$OUT_DIR/stage2-at.receipt.json"

# Stage 3 — post-deploy verification (settle further if needed)
sleep 60
cub-scout receipt verify $RESOURCE -n $NAMESPACE \
  --strategy $STRATEGY \
  --at-commit $RELEASE_SHA \
  --input-attestation "$OUT_DIR/stage2-at.receipt.json" \
  --save \
  --out "$OUT_DIR/stage3-post.receipt.json"

# Attach the chain to the release record
echo "Chain artifacts at $OUT_DIR:"
ls "$OUT_DIR"
```

Reading the chain six months later:

```bash
$ for stage in stage1-pre stage2-at stage3-post; do
    cub-scout receipt validate "$OUT_DIR/$stage.receipt.json"
    echo "$stage: exit $?"
  done
stage1-pre: exit 0
stage2-at: exit 0
stage3-post: exit 0
```

If any of the three were tampered with, `receipt validate` exits 1 and the chain is provably broken.

## Tampered-upstream behavior

```bash
# Trigger the refusal: tamper with stage 1's verdict and try to chain
$ sed -i 's/"verdict": "PASS"/"verdict": "BLOCK"/' stage1-pre.receipt.json

$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation stage1-pre.receipt.json \
    --save
Error: build input-attestations: build input-attestation from stage1-pre.receipt.json: build-attestation-ref: refusing to chain a receipt whose fingerprint doesn't verify: ...
exit 1
```

The chain construction refuses upfront; no downstream receipt is produced.

## Incident postmortem chain

The chain shape is also the right tool for **incident close-out evidence** (see [`pilot-incident-evidence`](../../../skills/pilot-incident-evidence/SKILL.md)):

```bash
# Incident SEV-2 fired at 14:00, resolved at 16:00
# Stage 1: pre-incident baseline (the cluster was healthy here)
# Stage 2: at peak (the bad state)
# Stage 3: post-resolution (recovered)

cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --at-commit $PRE_INCIDENT_SHA \
  --save --out incident/stage1-pre.receipt.json

cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --input-attestation incident/stage1-pre.receipt.json \
  --save --out incident/stage2-at.receipt.json
# verdict: likely BLOCK (manual-edit during incident triage)

cub-scout receipt verify deploy/payments-api -n prod \
  --strategy git-argo \
  --input-attestation incident/stage2-at.receipt.json \
  --save --out incident/stage3-post.receipt.json
# verdict: PASS if we recovered
```

The chain's verdict timeline `PASS → BLOCK → PASS` reads as "we had a manual-edit episode that we recovered from." `PASS → BLOCK → BLOCK` reads as "we never recovered intent; the live state is still divergent."

## Wire format

Chained `inputAttestations[]` entries look like this in the receipt's predicate:

```json
{
  "inputAttestations": [
    {
      "uri": "cub-scout-receipt://abc123def456",
      "digest": {"sha256": "abc123def456789..."}
    }
  ]
}
```

- **`uri`** uses the `cub-scout-receipt://<short-fingerprint>` scheme; the short fingerprint is the first 12 hex chars of the upstream's SHA-256 digest for readability
- **`digest.sha256`** carries the full 64-hex SHA-256 — that's what's actually cryptographically meaningful

The chain is a **set**, not a list: the order of `inputAttestations[]` entries is canonicalized by RFC 8785 sort order when computing the downstream fingerprint, so reordering doesn't change the chain identity.

## See also

- [`../README.md`](../README.md) — receipt v1 + v2 overview
- [`../ci-gate/README.md`](../ci-gate/README.md) — CI gating with `--fail-on` (companion v2 surface)
- [`../aggregate/README.md`](../aggregate/README.md) — aggregate receipts over namespace scope (composes chained refs into one synthetic aggregate)
- [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Receipt Contract — chained-receipts wire format
- [`docs/reference/commands.md`](../../../docs/reference/commands.md) § `receipt verify` — `--input-attestation` flag
- Issue: [`#448`](https://github.com/confighub/cub-scout/issues/448) (chained + aggregate); shipped in [`#463`](https://github.com/confighub/cub-scout/pull/463) (chained half) + [`#469`](https://github.com/confighub/cub-scout/pull/469) (aggregate half)
