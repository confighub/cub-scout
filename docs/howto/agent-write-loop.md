# Agent Write Loop: Safe Pre-Write, Post-Write, Offline & Signed Proof

This guide shows how **`cub mini`**, coding agents (**Claude Code**, **Codex**,
**Cursor**, **Antigravity**), ops bots (**`k8sbot`**, **Pilot**), and **CI
promotion gates** use `cub-scout` as the strictly read-only witness around
every cluster or GitOps write.

## Boundary: Who Writes, Who Witnesses

| Role | Tool | Permissions |
|---|---|---|
| Author & mutate intended state | `cub`, `cub mini`, GitOps repo, agent skill | Write to ConfigHub / Git / cluster |
| Independent live & offline witness | **`cub scout`** (or standalone `cub-scout` / MCP) | **Strictly read-only (`get`, `list`, `watch`), zero LLM** |

`cub-scout` never applies manifests, never triggers sync, and never guesses
missing evidence. Every command below works identically as the primary `cub`
plugin (`cub scout ...`), the standalone binary (`cub-scout ...`), or a
read-only stdio MCP tool (`cub-scout mcp serve`).

---

## The 4-Step Write Loop

```text
1. BEFORE WRITE   cub scout before-write  -> owner, revert risk, syncWindows, contention, PDBs (<1 KB)
2. WRITE          cub / cub mini / git    -> make the change (set --field-manager agent/<tool>/<run>)
3. AFTER WRITE    cub scout landed        -> revision/digest + running Pod imageIDs (--verify-pod-images)
4. SIGN PROOF     cub scout receipt sign  -> offline Ed25519 signed-receipt/v1 artifact for CI/audit
```

---

## 1. Before Writing: `cub scout before-write`

Run one sub-kilobyte read-only check before patching a workload or editing a
GitOps source:

```bash
cub scout before-write deploy/api -n prod \
  --field-path .spec.replicas \
  --format json
```

Standalone binary:

```bash
cub-scout before-write deploy/api -n prod \
  --field-path .spec.replicas \
  --format json
```

MCP tool: `before_write` (arguments: `resource: "deploy/api"`, `namespace: "prod"`, `field_path: ".spec.replicas"`).

### What it returns in one call

- **Owner (`owner`)**: Flux, Argo CD, Helm, kro, Crossplane, Sveltos, Modelplane, ConfigHub, or Native.
- **Field attribution (`fieldAttribution`)**: current live `value`, `managedFields` writers, `cause`, **writer contention** across multiple managers (including `agent/<tool>[/<run>]` self-reported managers), and `revertPrediction` (`likely-reverted`, `not-expected`, or `unknown`).
- **Deployer guardrails (`deployer`)**: Flux `spec.suspend` or Argo CD `autoSync`, `selfHeal`, `prune`, `ignoreDiffCount`, and active `AppProject` `syncWindows`.
- **Disruption budgets (`pdbs`)**: matching `policy/v1` `PodDisruptionBudget`s in the namespace (`minAvailable`, `maxUnavailable`, `disruptionsAllowed`).
- **Standard `answer` envelope**: reuses `explain`'s `answer.state` (`explained` or `not-observable`), with `basis[]`, `unknowns[]`, and read cost.

### How an agent should act on the result

- **`revertPrediction == "likely-reverted"`**: do not patch the live object; edit the GitOps source or ConfigHub Unit instead.
- **Active Argo `deny` `syncWindow` or `disruptionsAllowed == 0`**: surface the constraint to the operator or defer the rollout.
- **Writer contention detected**: inspect the competing manager (for example `agent/codex/run-42`) before overwriting the same field path.

> **Tip (G5 manager convention):** When an agent performs a direct Kubernetes
> server-side apply outside GitOps, pass `--field-manager agent/<tool>/<run>`
> (for example `agent/claude-code/pr-104`). `before-write` and `explain` parse
> this convention deterministically so other agents immediately see who owns
> the field.

---

## 2. After Writing: `cub scout landed --verify-pod-images`

After pushing a commit, publishing an OCI revision, or updating a ConfigHub
Unit, verify that the exact anchored revision actually reached running Pods:

```bash
cub scout landed deploy/api -n prod \
  --revision <sha> \
  --verify-pod-images \
  --wait 60s \
  --format json
```

Other anchor flags (supply exactly one):

- `--revision <sha>` — Git commit SHA or controller revision.
- `--digest sha256:...` — OCI artifact digest.
- `--unit-revision <rev>` — ConfigHub unit revision.
- `--image <ref-or-sha256>` — optional explicit expected container image or digest (requires `--verify-pod-images`).

MCP tool: `landed` (bounded read-only `--wait` is capped at `60s` via MCP and `5m` on CLI; poll interval defaults to `5s`, minimum `2s`).

### Interpreting `answer.state`

| `answer.state` | Meaning | Next step |
|---|---|---|
| `landed` | Controller applied the exact anchor, workload rolled out, and (with `--verify-pod-images`) running Pod `imageID`s match. | Proceed or sign the receipt. |
| `converging` | Source is fetching, controller is reconciling, or rollout is still progressing (`convergence.recheckAfterSeconds`). | Re-check after the hinted interval (or let `--wait` finish). |
| `not-landed` | Broken link (`brokenLink.stage`), wrong settled revision, failed workload, or running Pod `imageID` mismatch. | Inspect `brokenLink` and `answer.reason`; never treat as success. |
| `not-observable` | Missing anchor, unreadable RBAC scope, unreachable cluster, or unsupported controller. | Fix scope/credentials or inspect manually. |

### Zero false-landed invariant

With `--verify-pod-images`, `cub scout landed` inspects running Pod
`status.containerStatuses[].imageID` digests directly. If running Pods do not
match the expected image/digest, `landed` returns `not-landed` **even if the
GitOps controller reports `Synced` and `Ready`**.

Supported controllers and workloads: **Flux**, **Argo CD**, **Helm v3/v4**,
**kro**, and **Crossplane** (with honest `not-observable` for Sveltos) across
`Deployment`, `StatefulSet`, `DaemonSet`, `Job`, and `CronJob`.

---

## 3. Offline / Air-Gapped Loop: `snapshot` + `--recording`

When an agent, reviewer, or CI runner cannot reach the cluster directly,
capture one bounded, secret-stripped snapshot and run the entire read-only
loop offline:

```bash
# Capture once on a host with read-only cluster access
cub scout snapshot --format yaml -o snap.yaml

# Evaluate offline with zero Kubernetes API calls
cub scout before-write deploy/api -n prod --field-path .spec.replicas --recording snap.yaml --format json
cub scout landed deploy/api -n prod --revision <sha> --verify-pod-images --recording snap.yaml --format json
cub scout trace deploy/api -n prod --recording snap.yaml --format json
cub scout gitops status --recording snap.yaml --format json
cub scout doctor --recording snap.yaml --format json
```

### Sanitization and staleness guarantees

- `snapshot --format yaml|json` strips Secret payloads and Pod sensitive fields
  (`env`, `envFrom`, `args`, `command`, `volumeMounts`, `volumes`, and
  `kubectl.kubernetes.io/last-applied-configuration`) while preserving safe Pod
  `status.containerStatuses[].imageID` and `PodDisruptionBudget` objects needed
  by `landed --verify-pod-images` and `before-write`.
- Every `--recording` answer carries the snapshot's `observedAt` timestamp and
  an explicit `kind: stale` entry in `answer.unknowns[]` so offline evidence is
  never mistaken for a live cluster read.
- Combining `--recording` with live-only flags such as `--kube-context` is
  refused explicitly.

---

## 4. Tamper-Evident Proof: Offline Ed25519 Receipts

Turn any `landed --format json` or `receipt/v1` output into a cryptographically
signed `signed-receipt/v1` artifact that downstream CI gates or auditors can
verify with zero cluster or network access:

```bash
# 1. Generate an Ed25519 key pair locally (PKCS#8 0600 private, PKIX 0644 public)
cub scout receipt keygen --out-private priv.pem --out-public pub.pem

# 2. Capture and sign the landed proof
cub scout landed deploy/api -n prod --revision <sha> --verify-pod-images --format json > landed.json
cub scout receipt sign --input landed.json --key priv.pem -o signed.json

# 3. Verify offline in a promotion gate or audit pipeline (exit 0 = valid, non-zero = tampered/wrong key)
cub scout receipt verify --input signed.json --pubkey pub.pem --format json
```

You can also pipe directly via `--input -`:

```bash
cub scout landed deploy/api -n prod --revision <sha> --format json \
  | cub scout receipt sign --input - --key priv.pem -o signed.json
```

The `signed-receipt/v1` envelope records the algorithm (`ed25519`), `keyId`,
canonical SHA-256 `payloadDigest`, base64 `signature`, RFC 3339 UTC `signedAt`,
and the original JSON `payload`.

---

## 5. Controlling Agent Token Budgets

Autonomous loops burn tokens fast if they dump raw `kubectl get -o json`
objects on every turn. Use three built-in controls:

1. **Compact summary views (`--view summary`)**: supported on `gitops status`,
   `gitops settings`, and `doctor` (CLI and MCP). Preserves the exact `answer`
   block and top-level counts while omitting per-object detail lists.
   ```bash
   cub scout gitops status --view summary --format json
   cub scout doctor --view summary --format json
   ```
2. **Delta polling (`--if-unchanged <hash>`)**: supported on `landed` and
   `gitops status`. Pass the previous response's `fingerprint`; if nothing
   changed, `cub-scout` returns a ~89-byte `unchanged` envelope instead of
   repeating the full payload.
   ```bash
   fp=$(jq -r '.fingerprint' landed.json)
   cub scout landed deploy/api -n prod --revision <sha> --if-unchanged "$fp" --format json
   ```
3. **Lean MCP tool catalog (`--tools core`)**: reduces one-time session catalog
   overhead from 15,218 bytes (~3,805 tokens) to 9,390 bytes (~2,348 tokens)
   standalone.

### Measured byte reduction

Across the 24 frozen cluster cases in [docs/reference/answer-sizes.md](../reference/answer-sizes.md):

- **Raw Kubernetes objects:** 8,398,144 bytes (~2,099,546 tokens)
- **Declared `cub-scout` `answer` ceilings:** 18,800 bytes (~4,700 tokens) — a **99.78% reduction** in evidence bytes read (enforced per command in CI).

*Boundary:* this benchmark measures deterministic evidence size on frozen
fixtures, not a paid paired model run; see [answer-sizes.md](../reference/answer-sizes.md)
for the full methodology and caveats.

---

## 6. Integration Patterns

### `cub mini` and Coding Agents (Claude Code, Codex, Cursor, Antigravity)
- Configure the MCP host to launch `cub-scout mcp serve --tools core` over stdio
  (or allow read-only shell calls to `cub scout before-write` and `cub scout landed`).
- Require `before_write` before any live patch or ConfigHub Unit edit, and
  `landed` (with `verify_pod_images: true`) before declaring a rollout complete.

### Ops Bots (`k8sbot`, Pilot)
- Run `cub-scout bot --watch-backed` (or `cub-scout watch`) to stream
  discovery, ownership-change, drift, and scan events to JSONL or a webhook.
- Each event carries the same standard `answer` envelope (reusing `doctor`'s
  `answer.state` words, capped at <= 700 bytes, with untrusted cluster text
  quoted) so ops bots can route alerts without running an LLM over raw cluster
  YAML.

### CI Promotion Gates
```bash
# Fail the pipeline unless the exact revision landed on running Pods, then sign the proof
cub scout landed deploy/api -n staging \
  --revision "$GIT_SHA" \
  --verify-pod-images \
  --wait 120s \
  --format json > landed.json

test "$(jq -r '.answer.state' landed.json)" = "landed"
cub scout receipt sign --input landed.json --key "$SIGNING_KEY_PATH" -o landed.signed.json
```

---

## Related Docs

- [Answer sizes & 99.78% benchmark](../reference/answer-sizes.md)
- [Using cub-scout from an AI Tool](using-cub-scout-from-ai-tool.md)
- [Is This Image Deployed?](is-this-image-deployed.md)
- [Receipts End-to-End](receipts-end-to-end.md)
- [JSON Contracts](../reference/json-contracts.md)
