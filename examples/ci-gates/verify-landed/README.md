# Reusable CI Promotion Gate (`verify-landed`)

A drop-in, strictly read-only GitHub Actions composite action that proves a
revision actually landed on a Kubernetes workload before CI promotes to the next
environment.

## What It Does

1. **Optional pre-write safety check (`run-before-write: "true"`):** Runs
   `cub-scout before-write <target> -n <ns> [--field-path <path>] --format json`
   to record ownership, revert risk, sync windows, contention, and PDBs.
2. **Post-deploy landing verification:** Runs
   `cub-scout landed <target> -n <ns> --revision <rev> [--verify-pod-images] [--image <img>] --wait <dur> --format json`
   (or `--digest` / `--unit-revision`).
3. **Optional offline Ed25519 receipt signing and verification:** Signs the
   captured JSON payload with `cub-scout receipt sign` and verifies it offline
   with `cub-scout receipt verify --input ... --pubkey ...`.
4. **Fail-closed gate:** Exits non-zero unless `answer.state == "landed"` and
   (when configured) receipt signature verification succeeds.

## Strictly Read-Only Guarantee

- Uses only read-only `cub-scout` commands (`before-write`, `landed`,
  `receipt sign`, `receipt verify`).
- Zero cluster mutations, zero LLM calls, deterministic output.
- Supports `--recording <file>` for completely offline / air-gapped verification.

## Files

- [`action.yml`](action.yml) — Composite GitHub Action definition.
- [`verify-landed.sh`](verify-landed.sh) — Gate runner script invoked by `action.yml`.
- [`workflow-example.yaml`](workflow-example.yaml) — Copy-pasteable staging-to-prod CI promotion workflow.
- [`test_verify_landed.sh`](test_verify_landed.sh) — Deterministic offline test (pass, fail-closed `not-landed`, and invalid signature).

## Inputs

| Input | Required | Default | Description |
|---|---|---|---|
| `target` | Yes | — | Target workload (`deploy/api`, `sts/db`, `ro/checkout`) |
| `namespace` | No | `default` | Target namespace (`-n`) |
| `revision` | No* | `""` | Expected Git SHA or controller revision (`--revision`) |
| `digest` | No* | `""` | Expected OCI digest (`--digest`) |
| `unit-revision` | No* | `""` | Expected ConfigHub unit revision (`--unit-revision`) |
| `verify-pod-images` | No | `"false"` | Independently verify running Pod `imageID` digests (`--verify-pod-images`) |
| `image` | No | `""` | Expected Pod image ref or `sha256:` digest (`--image`) |
| `wait` | No | `"60s"` | Bounded read-only wait duration (`--wait`, max `5m`) |
| `poll-interval` | No | `""` | Poll interval for bounded wait (`--poll-interval`) |
| `kube-context` | No | `""` | Exact kube context (`--kube-context`) |
| `recording` | No | `""` | Offline YAML/JSON cluster snapshot (`--recording`) |
| `run-before-write` | No | `"false"` | Run `cub-scout before-write` before `landed` |
| `field-path` | No | `""` | Optional `--field-path` for `before-write` |
| `sign-key-path` | No | `""` | Optional Ed25519 PKCS#8 private key PEM for `receipt sign` |
| `verify-pubkey-path` | No | `""` | Optional Ed25519 PKIX public key PEM for `receipt verify` |
| `cub-scout-bin` | No | `"cub-scout"` | Path or command name for `cub-scout` |
| `output-dir` | No | `".cub-scout-gate"` | Directory for output JSON receipts |

\*Exactly one of `revision`, `digest`, or `unit-revision` must be set.

## Outputs

| Output | Description |
|---|---|
| `state` | `answer.state` from `cub-scout landed` (`landed`, `converging`, `not-landed`, `not-observable`) |
| `receipt-path` | Path to raw `landed.json` |
| `signed-receipt-path` | Path to Ed25519 `landed.signed.json` (empty if signing not enabled) |

## Offline Verification

Run the deterministic offline test without any Kubernetes cluster:

```bash
bash examples/ci-gates/verify-landed/test_verify_landed.sh
```
