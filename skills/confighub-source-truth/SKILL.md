---
name: confighub-source-truth
description: 'Use when the user wants a STRATEGY-TYPED, PILOT-ACCEPTANCE-SHAPED VERDICT about whether ConfigHub / controller / runtime agree under a declared delivery strategy — the `compare source-truth` workflow with the explicit `--strategy <name>` requirement. Natural phrasing: "give me a source-truth verdict for deploy/api", "verify under git-argo strategy that ConfigHub matches the cluster", "run the source-truth contract for upstream Pilot acceptance", "compare source-truth confighub-oci-flux strategy across the namespace", "produce strategy-relative correctness evidence", "show me where the strategy declaration is being violated". Composes `compare source-truth --strategy <s>` + the source-truth evidence contract + (optionally) `receipt verify --strategy <s>` for a fingerprinted version. Requires CONNECTED MODE. Do NOT load for: a single-resource live drift question without a declared strategy (use investigate-drift), a triage (use triage-unhealthy-workload), or a fleet-wide audit that doesn''t need strategy framing (use audit-fleet-conformance). cub-scout NEVER infers a strategy — this skill enforces that boundary.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub link list *) Bash(cub auth status) Bash(argocd app get *) Bash(flux get *)
---

# confighub-source-truth

The strategy-typed source-truth loop. The operator (or an acceptance kernel like Pilot) wants a **deterministic verdict** about whether ConfigHub / controller / runtime agree under a **declared delivery strategy**. cub-scout never infers the strategy — the user passes it; the verdict is then strategy-relative.

Composes `compare source-truth --strategy <s>` + the source-truth evidence contract + optionally `receipt verify --strategy <s>` (to persist the verdict as a fingerprinted artifact).

Requires **connected mode** (`cub auth login`).

## When to use

Explicit phrasings:

- "Give me a source-truth verdict for deploy/api"
- "Verify under `git-argo` strategy that ConfigHub matches the cluster"
- "Run the source-truth contract for upstream Pilot acceptance"
- "Compare source-truth `confighub-oci-flux` strategy across the namespace"
- "Produce strategy-relative correctness evidence"
- "Show me where the strategy declaration is being violated"
- "Did the controller pull from the declared source?"

Implicit intents:

- The user is operating in the **architectural triad** (cub-scout evidence ↔ ConfigHub authority ↔ Pilot judge); Pilot needs strategy-typed input
- The user has a **declared expectation** about the delivery chain (`Git→Argo`, `ConfigHub→OCI→Flux`, etc.) and wants to check it
- The user is gating CI / promotion on conformance with the declared strategy
- The user wants the **proof gaps** surfaced explicitly (the `omissions[]`-style contract is the whole point)

## Do not load for

- Single-resource live drift WITHOUT a declared strategy — [`investigate-drift`](../investigate-drift/SKILL.md)
- Triage of a broken workload — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- Fleet-wide audit when strategy framing isn't needed — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) (which itself can compose `compare source-truth` per resource, but the broader-scope skill is the right entry point)
- Incident-evidence capture after the fact — [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md)
- Authoring source-truth strategies. The strategy enumeration is fixed (see below); proposing new ones is a `cub-scout/#393`-class issue.

## The nine strategies

cub-scout's source-truth contract enumerates **nine strategies** (Phase 1 + Phase 2 from `#393` and `#418`). The user MUST pick one:

| Strategy | Delivery chain | Controller |
|----------|---------------|-----------|
| `confighub-oci-argo` | ConfigHub authors a Unit → renders to OCI → Argo CD reads OCI → cluster | Argo |
| `confighub-oci-flux` | Same authoring → Flux Kustomization reads OCI → cluster | Flux |
| `git-argo` | Git → Argo CD → cluster (vanilla GitOps) | Argo |
| `git-flux` | Git → Flux → cluster | Flux |
| `helm-argo` | Helm chart → Argo as Helm renderer → cluster | Argo |
| `helm-flux` | Helm chart → Flux helm-controller → cluster | Flux |
| `kustomize-flux` | Kustomize base → Flux kustomize-controller → cluster | Flux |
| `oci-argo` | OCI artifact (non-ConfigHub) → Argo → cluster | Argo |
| `oci-flux` | OCI artifact (non-ConfigHub) → Flux → cluster | Flux |

Each strategy declares **expected surfaces** (which of ConfigHub / controller / runtime is required), **expected controller kind** (Argo vs Flux — never silently fall back), and **strategy-implied authority** (which surface is canonical for the comparison).

Pick by what's deployed, not by what's convenient. If the user can't name the strategy, the right answer is **ASK** (the contract's fourth status), not a guess.

## The four statuses + five verdicts

cub-scout's source-truth contract emits two separate axes:

**Status** (cub-scout's evidence-quality verdict):

| Status | Meaning |
|--------|---------|
| `PASS` | All required surfaces present, all required equalities hold, no proof gaps. Pilot may accept. |
| `WATCH` | Mostly present and consistent, but at least one soft signal warrants confirmation. |
| `BLOCK` | A hard rule fails (strategy mismatch, controller-source resolution failure). Must not accept. |
| `ASK` | cub-scout cannot classify deterministically. Used when strategy is unknown or surfaces fundamentally cannot be compared. |

**Verdict** (cross-surface agreement, separate from status):

| Verdict | Meaning |
|---------|---------|
| `AGREED` | The three surfaces line up under the declared strategy |
| `MISMATCH` | At least one surface diverges from the strategy-implied authority |
| `INCOMPLETE` | One or more surfaces missing required proof |
| `BLOCKED` | cub-scout could not fetch one of the surfaces in read-only mode |
| `UNKNOWN` | cub-scout has no opinion (used in ASK) |

Status and verdict ARE separate — a verdict can be `MISMATCH` while status is `BLOCK` (strategy violation) OR `WATCH` (soft mismatch with proof gaps). See `pkg/agent/source_truth.go` for the closed enums.

## The loop

1. **Confirm connected mode.** `cub-scout status` must report connected; source-truth is meaningless without all three surfaces.
2. **Declare the strategy.** The user MUST pass `--strategy <name>`. If they don't know, the answer is ASK + UNKNOWN.
3. **Invoke per resource.** `cub-scout compare source-truth <kind>/<name> -n <ns> --strategy <s> --format json`.
4. **Read the structured evidence.** Status + verdict + outlier + proof_gaps[] + safe_next_action + the three surfaces verbatim. See [`scout-compare`](../scout-compare/SKILL.md).
5. **Optionally persist as a receipt.** `cub-scout receipt verify <kind>/<name> -n <ns> --strategy <s> --save` produces a fingerprinted, immutable version Pilot or the audit log can attach to a decision. See [`scout-verify`](../scout-verify/SKILL.md).

## Step-by-step

### Step 1 — confirm mode

```bash
$ cub-scout status
Mode: connected (CONFIGHUB_API_KEY set; cub auth status: OK)
Cluster: prod-use2
ConfigHub: hub.confighub.com
```

If `mode: standalone`, refuse to claim a source-truth verdict — say so to the user and recommend `cub auth login`.

### Step 2 — pick the strategy

Match the actual delivery chain. Look at:

- Workload labels (`argocd.argoproj.io/instance` → Argo; `kustomize.toolkit.fluxcd.io/name` → Flux Kustomization; `helm.toolkit.fluxcd.io/name` → Flux HelmRelease)
- Argo Application spec (`spec.source.repoURL` — git URL vs OCI URL vs Helm chart URL)
- Flux Kustomization spec (`spec.sourceRef.kind` — `GitRepository` vs `OCIRepository` vs `HelmRepository` vs `Bucket`)
- ConfigHub linkage (`confighub.com/UnitSlug` label, plus the unit's render target)

`cub-scout explain <kind>/<name>` surfaces all these in one read. From there, picking the strategy is mechanical:

| You see... | Strategy is... |
|-----------|----------------|
| Argo + git source | `git-argo` |
| Argo + OCI source (ConfigHub-rendered) | `confighub-oci-argo` |
| Argo + OCI source (non-ConfigHub) | `oci-argo` |
| Argo + Helm source | `helm-argo` |
| Flux Kustomization + GitRepository | `git-flux` |
| Flux Kustomization + OCIRepository | `confighub-oci-flux` (if ConfigHub-managed) or `oci-flux` |
| Flux Kustomization (any source) | `kustomize-flux` (if specifically the kustomize rendering) |
| Flux HelmRelease | `helm-flux` |

### Step 3 — invoke

```bash
$ cub-scout compare source-truth deploy/api -n prod --strategy git-argo --format json
{
  "declared_strategy": "Git -> Argo -> Kubernetes",
  "status": "PASS",
  "source_truth": "AGREED",
  "outlier": "unknown",
  "surfaces": {
    "confighub": {
      "space": "payments",
      "unit": "payments-api",
      "revision": "42",
      "url": "https://hub.confighub.com/p/payments/u/payments-api"
    },
    "controller": {
      "kind": "Argo",
      "source": "https://github.com/org/platform-config",
      "revision_or_digest": "abc123def456",
      "health": "Synced",
      "multi_source": false
    },
    "runtime": {
      "resource": "Deployment/api in prod",
      "field": "spec.template.spec.containers[0].image",
      "value": "ghcr.io/org/api:v2.3.0",
      "health": "Ready"
    }
  },
  "proof_gaps": [],
  "safe_next_action": ""
}
```

The empty `proof_gaps[]` is the headline. `status: PASS`, `source_truth: AGREED`, all three surfaces present. Pilot accepts.

### Step 4 — when the verdict is not PASS

```bash
$ cub-scout compare source-truth deploy/api -n prod --strategy git-argo --format json
{
  "declared_strategy": "Git -> Argo -> Kubernetes",
  "status": "BLOCK",
  "source_truth": "MISMATCH",
  "outlier": "controller",
  ...
  "proof_gaps": [],
  "safe_next_action": "Argo is tracking a different revision than the controller-resolved git anchor; investigate the divergence before any apply."
}
```

`status: BLOCK` + `outlier: controller` = the controller is the surface diverging from the strategy-implied authority. The `safe_next_action` field names the next read-only step (never a mutation).

### Step 5 — persist as a receipt (optional)

For Pilot acceptance kernels or audit trails:

```bash
$ cub-scout receipt verify deploy/api -n prod --strategy git-argo --save
saved: /home/user/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__source-truth-pass__Deployment-api__abc123.receipt.json
```

The receipt wraps the source-truth evidence (verbatim) in an in-toto Statement v1 envelope with a SHA-256 fingerprint. Strategy mismatch between the caller and the evidence body is itself a BLOCK + `OmissionStrategyMismatch` — see [`scout-verify`](../scout-verify/SKILL.md).

Receipts are **immutable**. Same scope + same fingerprint at the same instant → same canonical filename → "already saved" warning (file unchanged).

## Strategy-relative correctness

This is the contract's central rule: **identical observations get opposite verdicts under different declared strategies**.

Example: an Argo CD Application reading from a git URL.

- Under `--strategy git-argo`: PASS / AGREED. Argo reads Git; Git is the declared source; cluster matches Git. All good.
- Under `--strategy confighub-oci-argo`: BLOCK / MISMATCH (outlier: controller). The strategy declares Argo should be reading from a ConfigHub-rendered OCI artifact — not Git. Argo is the outlier.

Same Argo, same git, same cluster, opposite verdict. Strategy declaration changes the question.

Lock that in your head: cub-scout doesn't grade the deployment — cub-scout grades it **against the declared strategy**. This is why `--strategy` is mandatory.

## Proof gaps

Even on PASS / WATCH outcomes, `proof_gaps[]` lists the things cub-scout couldn't fully verify:

- `controller.revision_or_digest` — controller didn't expose its revision/digest
- `confighub.unit_revision` — unit didn't have a revision number (rare; ConfigHub usually populates)
- `runtime.helm_chart_anchor` — Helm chart version not extractable from runtime (Phase 2 strategies; the chart-version extractor isn't fully wired — see #409)
- `controller.multi_source` — Argo `spec.sources[]` has len > 1; cub-scout uses index 0, can't verify equality across sources

Proof gaps are **not failures**. They're explicit non-claims (the same pattern as receipt omissions). Pilot reads them and decides whether to accept the verdict given the named gaps.

## Tool boundary

- **Allowed:** `compare source-truth`, `compare three-way` (for the broader DRY/WET/LIVE context); `receipt verify --strategy --save`, `receipt show / list / validate`; `explain`, `trace`, `status`; `cub * get / list`, `cub unit get`, `cub link list`; `cub auth status`; `argocd app get`, `flux get`.
- **Not allowed:** any mutation. `cub * create/update/delete`, `argocd app sync`, `flux reconcile`, `kubectl apply/edit/patch/delete`, `cub-scout demo / import apply`. The verdict is evidence; remediation is the user's call (or Pilot's downstream).

## Pilot integration

cub-scout's source-truth contract is the canonical input for **Pilot**, the architectural-triad acceptance judge tracked alongside cub-scout in `#444`. Pilot reads source-truth evidence + receipts and produces verdicts that gate CD / promotion / rollback / compliance.

- cub-scout = evidence (this skill produces it)
- ConfigHub = authority (where the unit / Link / target lives)
- Pilot = judge (decides accept / wait / block)

For agent workflows wiring Pilot to cub-scout, see [`scout-mcp`](../scout-mcp/SKILL.md) (Pilot can call `compare_source_truth` as an MCP tool) and the `pilot-*` skill set (planned in `#444`).

## References

- [`scout-compare`](../scout-compare/SKILL.md) — verb-group skill for the four Compare verbs
- [`scout-verify`](../scout-verify/SKILL.md) — receipts as fingerprinted evidence
- [`scout-govern`](../scout-govern/SKILL.md) — connected-mode trust surface
- [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) — fleet-wide composition over per-resource source-truth
- Source-truth contract: `#393` (parent), `#418` (Phase 2 enum expansion), `#395` (producer fixtures)
- Read-only triad: `#410`, `#428`
- Code: `pkg/agent/source_truth.go`, `pkg/agent/source_truth_logic.go` (`Derive`)
- Examples: [`examples/connect-and-compare/`](../../examples/connect-and-compare/)

## Constraints

- **Strategy is always declared, never inferred.** This is the single most important contract rule. If the user doesn't know the strategy, the right answer is ASK + UNKNOWN, not a guess.
- Connected mode is required. Standalone source-truth verdicts are meaningless (no ConfigHub surface to read).
- The strategy enum is **closed** at 9 entries. New strategies require a `#393`-class issue and a code change. Don't claim cub-scout supports an unlisted strategy.
- `helm-flux` / `helm-argo` produce `runtime.helm_chart_anchor` proof gaps because the runtime extractor isn't fully wired. Surface the gap explicitly; don't pretend the verdict is comprehensive.
- `confighub-oci-*` strategies also have an unshipped equality (ConfigHub doesn't yet expose a rendered digest per unit revision). The status field is correct; the per-field digest equality lands once the ConfigHub-side field exists.
