# Multi-Cluster Fleet Promotion Proof (Offline Example)

This self-contained example demonstrates how four independent Kubernetes
clusters (`dev`, `staging`, `prod-east`, `prod-west`) each produce a
cryptographically signed `signed-receipt/v1` delivery receipt from
`cub-scout landed --verify-pod-images` + `cub-scout receipt sign`, and how a
single offline `cub-scout receipt verify-fleet` call proves fleet-wide landing
with zero cluster access and zero fleet database.

> **Test-only example key:** [`test-pub.pem`](test-pub.pem) is a **test-only
> example Ed25519 public key** generated solely for this committed fixture set.
> The ephemeral private key used to sign the example receipts was discarded
> immediately after generation and is never committed. Never commit real
> private keys to source control.

## Files

| Path | Description |
|---|---|
| [`test-pub.pem`](test-pub.pem) | Test-only Ed25519 PKIX public key (`keyId: df41b88d81b29aa5`) |
| [`receipts/dev.json`](receipts/dev.json) | Signed `landed --verify-pod-images` receipt for `dev` (`observedAt: 2026-11-27T08:00:00Z`) |
| [`receipts/staging.json`](receipts/staging.json) | Signed `landed --verify-pod-images` receipt for `staging` (`observedAt: 2026-11-27T08:06:00Z`) |
| [`receipts/prod-east.json`](receipts/prod-east.json) | Signed `landed --verify-pod-images` receipt for `prod-east` (`observedAt: 2026-11-27T08:15:00Z`) |
| [`receipts/prod-west.json`](receipts/prod-west.json) | Signed `landed --verify-pod-images` receipt for `prod-west` (`observedAt: 2026-11-27T08:22:00Z`) |
| [`verify.sh`](verify.sh) | Executable offline verification + tamper fail-closed check |

## Quick Start

From the repository root (after `go build -o ./cub-scout ./cmd/cub-scout`):

```bash
./examples/fleet-promotion-proof/verify.sh
```

Or run `receipt verify-fleet` directly from this directory:

```bash
cd examples/fleet-promotion-proof

# Primary cub plugin form
cub scout receipt verify-fleet \
  --inputs ./receipts \
  --pubkey ./test-pub.pem \
  --revision 7c9e2a4f \
  --require-clusters dev,staging,prod-east,prod-west

# Standalone binary form
../../cub-scout receipt verify-fleet \
  --inputs ./receipts \
  --pubkey ./test-pub.pem \
  --revision 7c9e2a4f \
  --require-clusters dev,staging,prod-east,prod-west
```

Expected ASCII output (exit `0`):

```text
FLEET VERIFY: landed (fleet-landed)
  Summary: 4/4 clusters verified and landed
  Revision: 7c9e2a4f
  - dev              state=landed         sig=ok rev=7c9e2a4f
  - prod-east        state=landed         sig=ok rev=7c9e2a4f
  - prod-west        state=landed         sig=ok rev=7c9e2a4f
  - staging          state=landed         sig=ok rev=7c9e2a4f
```

## How Each Cluster Produces Its Signed Receipt

In a real promotion pipeline (`dev` → `staging` → `prod-east` → `prod-west`),
each cluster runs its own strictly read-only `cub-scout` witness against a
single kube context:

```bash
# 1. Generate a signing key pair once in your CI secret store / KMS wrapper
cub scout receipt keygen --out-private /secure/priv.pem --out-public ./test-pub.pem

# 2. On each cluster (dev, staging, prod-east, prod-west), verify landing
#    including running Pod imageID digests and sign the JSON receipt offline:
for cluster in dev staging prod-east prod-west; do
  cub scout landed deploy/api -n payments \
    --revision 7c9e2a4f \
    --verify-pod-images \
    --image sha256:4f8b9c2e1d3a5b7c9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d \
    --kube-context "$cluster" \
    --wait 60s \
    --format json \
  | cub scout receipt sign \
      --input - \
      --key /secure/priv.pem \
      -o "./receipts/${cluster}.json"
done

# 3. Verify the entire fleet offline in one call (zero kubeconfig, zero network):
cub scout receipt verify-fleet \
  --inputs ./receipts \
  --pubkey ./test-pub.pem \
  --revision 7c9e2a4f \
  --require-clusters dev,staging,prod-east,prod-west \
  --format json
```

## What `verify.sh` Proves

1. **All 4 clusters verified (`landed`, exit `0`):** Verifies every Ed25519
   signature against `test-pub.pem`, checks revision `7c9e2a4f`, confirms all
   four required clusters (`dev`, `staging`, `prod-east`, `prod-west`) are
   present and `landed`, and asserts `answer.state == "landed"`.
2. **Fail-closed tamper detection (`not-landed`, exit `1`):** Copies the four
   receipts into a temporary directory, modifies a single byte in
   `prod-west.json` without re-signing, and asserts `receipt verify-fleet`
   exits non-zero with `answer.state == "not-landed"`.

## Further Reading

- Full guide including all failure modes and ConfigHub `G4` / `D6` graduation:
  [`docs/howto/fleet-promotion-proof.md`](../../docs/howto/fleet-promotion-proof.md)
