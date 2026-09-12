# Running-Image Identity Check

Status: proposed; implementation in progress for the next minor release, not
published. Builds on the exact configuration release check (#536) and controller
revision evidence (#535).
Tracking: #502, #505 (named "pod/artifact identity" later work); v2.11 scope in
the closed #532.

## User Value

The release check (#536) answers: did this exact configuration bundle reach this
target, and did the workload controller converge? It deliberately does **not**
answer the question an operator actually asks before they stop investigating:

> Is the image actually running the one I intended?

This is the seam where every visible status can look healthy and the intended
release still is not running: a mutable tag was repushed after rollout, a
`kubectl set image` hotfix changed the live pods out of band, or a digest simply
never rolled. Controller reports the revision, the workload reports converged,
and the pods are serving an image the configuration no longer denotes.

This check adds the **artifact-identity tier**: compare the container image the
intended configuration declares against the image digest actually executing in
the live pods.

### Three identities, never conflated

`#536` already warns that configuration-bundle digests and container-image
digests are different identities. This check introduces a third and keeps all
three separate:

1. **Configuration-bundle digest** — identity of the OCI config artifact (#536).
2. **Intended container-image reference** — what the workload spec *declares*
   (`spec.template.spec.containers[].image`), pinned (`app@sha256:…`) or mutable
   (`app:v2`).
3. **Running container-image digest** — what is *executing*, read from
   `.status.containerStatuses[].imageID` on the live pods.

The bundle tier compares (1). This tier compares (2) against (3). They are never
compared to one another.

## Design principles (non-negotiable)

- **Parse, don't guess.** A mutable tag cannot be resolved to a digest from
  cluster reads alone, so it is `UNKNOWN (mutable-tag)`, never an assumed match.
- **Never a false alarm.** A `MISMATCH` is reported only as the factual claim
  "the running digest is not the intended digest," with the multi-architecture
  caveat attached; interpretation stays caveated, never overstated as "wrong
  image" with certainty we do not have.
- **Bounded reads.** This is the first release-check tier to read pods. Pod reads
  are a single namespaced, selector-scoped, hard-capped read with the added
  request cost reported. It never becomes full-cluster discovery, unbounded
  fan-out or implicit polling. Over the cap is partial coverage, not a whole-set
  claim.
- **Additive and separable.** The bundle, controller, configuration and
  convergence tiers of #536 are unchanged. A missing or unknown running-image
  result never upgrades another tier, and a known running-image mismatch is never
  hidden by another passing tier (weakest-link headline).
- **Graceful degradation.** Denied pod reads, absent `imageID`, selectors we do
  not yet support, or coverage past the cap all resolve to a specific `UNKNOWN`
  reason plus the next check to run — never a silent pass and never an error that
  discards the other tiers.

## First-Slice Contract

The tier is **opt-in and off by default**, exposed as `release check
--check-running-image` (MCP `check_running_image: true`). This preserves the base
check's test-asserted request budget (`2N + 4` / `2N + 8`) exactly: no pods are
read unless the flag is set. `--max-pods` (default 50, max 200) caps the pod read
per workload.

Scope: workloads the release check already identifies and reads for convergence.
Deployment first (same controller/target shapes #536 supports); other kinds
report an explicit unsupported result.

For each identified live workload:

1. Resolve the intended image reference per app container from the bundle's
   workload spec, matched by container name.
2. Perform one bounded pod read: single namespace, scoped by the workload's
   `spec.selector.matchLabels`, capped at a finite pod limit, sequential, with
   the request count exposed. `matchExpressions`-only selectors are
   `UNKNOWN (selector-unsupported)` in this slice.
3. For each running app container, extract the digest from
   `.status.containerStatuses[].imageID` (tolerating `docker-pullable://`,
   registry-qualified and bare `sha256:` forms).
4. Compare intended vs running per container, aggregate to the weakest per pod,
   and to the weakest across the read pods.

Per-container outcomes:

| Outcome | Condition |
|---|---|
| `MATCH` | Intended is digest-pinned, same repository, running digest equals intended digest. |
| `MISMATCH` | Intended digest-pinned, same repository, running digest present and different. Evidence carries the multi-arch index/manifest caveat. |
| `UNKNOWN (mutable-tag)` | Intended reference is a tag, not a digest. Remediation: pin to a digest. |
| `UNKNOWN (different-repository)` | Intended and running repositories differ; digests are not comparable. |
| `UNKNOWN (unreadable)` | `imageID` absent/empty, image not yet pulled, or container not started. |
| `UNKNOWN (container-not-found)` | A declared container is not present in any read pod. |
| `UNKNOWN (read-denied)` | Pod read forbidden by RBAC. |
| `UNKNOWN (selector-unsupported)` | No `matchLabels`, or `matchExpressions`-only. |
| `UNKNOWN (coverage-capped)` | More matching pods than the cap; only a partial set was read. Any confirmed `MISMATCH` within the read set still surfaces. |
| `UNKNOWN (no-running-pods)` | Selector matches no pods. |

Headline integration: the running-image tier is one more evidence line. On an
otherwise-positive release check, any running-image outcome other than `MATCH`
downgrades the headline from a running-as-intended claim to `NOT CONFIRMED`
(for `UNKNOWN`) or `NOT running as intended` (for `MISMATCH`). It never upgrades
a headline and never emits an application-success claim.

Boundaries preserved from #536: HTTPS + existing registry credentials only; no
login, credential writes, registry permissions, publication or cluster mutation.
No re-rendering. Observations are sequential, not an atomic snapshot.

## Success Before Implementation

Deterministic fixtures (recorded pods/workloads, no live cluster) must cover:

- **MATCH**: intended `app@sha256:NEW`, all read pods run `NEW`.
- **MISMATCH (retagged / OLD)**: intended `app@sha256:NEW`, pods run `sha256:OLD`
  in the same repository → `MISMATCH`, headline `NOT running as intended`.
- **UNKNOWN (mutable-tag)**: intended `app:v2`, pods run any digest → `UNKNOWN`
  with pin-to-digest remediation; headline `NOT CONFIRMED`.
- **imageID forms**: `docker-pullable://repo@sha256:…`, `repo@sha256:…` and bare
  `sha256:…` all parse to the same digest.
- **Multi-container**: sidecar matches, app container mismatches → tier
  `MISMATCH` (weakest across containers), matched by name.
- **Multi-pod**: one pod on `OLD`, rest on `NEW` (rollout in progress) → surfaced
  as not-all-converged, not a clean `MATCH`.
- **Degradation**: read-denied, absent `imageID`, `matchExpressions`-only
  selector, over-cap coverage, no matching pods, container-not-found,
  different-repository — each yields its specific `UNKNOWN` reason.
- **Separation**: an exact configuration-bundle match with a running-image
  `MISMATCH` still downgrades the headline; the bundle digest and the image
  digest are never compared to each other.
- **Request accounting**: the added pod read is counted and capped; no full LIST
  fan-out beyond the cap; no hidden retries.
- **Surface parity**: CLI ASCII/JSON/Markdown, plugin equivalence, stdio MCP,
  and TUI render/refresh all present the same tier and headline.

## Known Limitations (documented, with follow-ups)

- **Multi-architecture digests**: an index (manifest-list) digest and the
  resolved per-architecture manifest digest can differ legitimately. Without
  registry resolution we cannot distinguish this from a genuine mismatch, so
  `MISMATCH` evidence always carries this caveat. Registry-side index resolution
  is later work (#505).
- **initContainers and ephemeral containers** are not assessed in this slice.
- **`matchExpressions` selectors** are not resolved in this slice.
- Coverage past the pod cap is reported as partial, never as a whole-set claim.

## Later Work

Registry-side digest/index resolution to firm up `MISMATCH` vs multi-arch;
initContainer/ephemeral coverage; `matchExpressions` selector support; and
budgeted watch/bot running-image checks. Tracked in #502/#505. None are
prerequisites for this first slice.

## Local Verification

Recorded on 2026-09-12: `go build ./...`, `go vet ./...` and the full `go test
./...` suite pass, including `-race` on the release-check and running-image
tests. The CLI-docs parity checker passes. New deterministic proofs:

- `pkg/agent/running_image_test.go` — image-reference and imageID parsing across
  digest/tag/pinned/registry-port forms; per-workload MATCH, MISMATCH (with the
  multi-arch caveat), mutable-tag UNKNOWN, multi-container weakest-link,
  multi-pod rollout, and every degradation reason (read-denied, no-running-pods,
  container-not-found, different-repository, unreadable, coverage-capped);
  bare-digest mismatch; aggregation; and selector resolution.
- `cmd/cub-scout/release_check_test.go` — `TestReleaseCheckRunningImage`
  (recorded pods, no live cluster) proves match→PASS with the confirmation
  headline, mismatch→BLOCK with "not running as intended", read-denied and
  mutable-tag→INCONCLUSIVE with a zero-pod-read mutable tag, and that the tier is
  off by default with the base request budget unchanged; plus MCP arg wiring.

Live authenticated authority proof stays deferred; positive results use recorded
HTTP fixtures, not production validation.
