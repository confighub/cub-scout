# Exact Configuration Release Check

Status: scoped implementation complete; merged in #536 for the next minor release,
not published. Prerequisite #535 is merged.
Tracking: #532, #502, #505. Builds on controller revision evidence in #535.

## User Value

Answer: did this exact configuration bundle reach this target, and which step
is preventing completion? A configuration-only change matters even when every
container image is unchanged. Configuration bundle and container image digests
are different identities and must never be compared to one another.

## Execution Checklist

- [x] Define scope, evidence boundaries, and success fixtures before coding.
- [x] Load a digest-pinned literal configuration bundle through a proven OCI
  client, verifying manifest and layer bytes; support a read-only local OCI
  layout for offline/recorded proof. Reject indexes, Helm charts, rendering
  inputs, unsafe archives, duplicates, and size/object-limit violations.
- [x] Read an explicitly selected controller and target context. Join the OCI
  repository, source configuration and digest, not a digest or name alone.
  Support a single-source Application and an OCI-backed Kustomization first;
  unsupported or ambiguous controller/target shapes remain inconclusive.
- [x] Reuse object-set authored-field comparison and workload convergence on
  the same dated live reads. Expose actual request counts and coverage. No
  full-cluster discovery, inventory LIST, pod fan-out, or implicit polling.
- [x] Present separate bundle, controller, configuration and convergence
  results, with a precise headline, omissions and next step. Preserve existing
  comparison/receipt semantics. Application success and running-image identity
  are explicitly not assessed.
- [x] Expose the same provider via standalone/plugin CLI, MCP and a refreshable
  TUI. Preserve five run modes; watch/bot release-check scheduling is a named
  follow-up, not an implied shipped feature.
- [x] Add README User questions, command/JSON contracts, a reproducible example,
  deterministic tests, recorded integration and non-production read-only proof.

## First-Slice Contract

`release check` takes an explicit `oci://repository@sha256:...`, exact
controller API identity, controller kube context and target kube context.
The caller supplies intended release identity; no mutable `latest` lookup,
release-number guessing or authenticated intended-state authority lookup.
Local OCI layout input must still match the expected manifest digest.

The first bundle shape is one OCI image manifest containing one tar or tar+gzip
layer of literal YAML/JSON. No re-rendering or template execution. Remote reads
use HTTPS and existing registry credentials; no login, credential writes,
registry permissions, publication or cluster mutation.

Target reads are individually bounded and sequential, with a finite object
limit and overall timeout. Namespaced manifests must explicitly identify their
namespace; no unknown custom-kind namespace is guessed. Secret payloads remain
excluded and create incomplete coverage, not a passing whole-set claim.
The existing two-request bounded-explain contract is unchanged.

Convergence is workload-controller status, not pod identity, process config
reload, traffic success or SLO proof. No workloads is not equivalent to a
running application. Independent configuration checks remain useful even if
controller evidence is unavailable. A known mismatch must not be hidden by
another missing check. Observations are sequential, not an atomic snapshot.

## Success Before Implementation

Deterministic fixtures must cover:

- Exact bundle match, wrong bundle digest, altered manifest/layer, moving tag,
  empty bundle, duplicate object identity, chart/template inputs, traversal,
  links, archive bombs, object cap and cancelled read.
- Application source/destination mismatch, OCI Kustomization source mismatch,
  old revision, missing source, stale generation, unsupported controller and
  remote/named destinations without a proven binding.
- Configuration-only drift with identical image references, absent object,
  forbidden read, excluded Secret, recreated UID, changed controller while the
  check runs, stale workload status and incomplete rollout.
- Exact-match configuration and converged workloads can produce a scoped
  positive result; no application-success or running-image claim is emitted.
- API counts, context separation, no mutation/LIST, no hidden retries, finite
  output/transfer limits, TUI cancellation/refresh and late-result rejection.
- CLI ASCII/JSON/Markdown and plugin equivalence, actual stdio MCP, TUI render
  and lifecycle tests. No new connected/fleet aggregation; recorded context
  isolation is required and live authenticated authority proof stays deferred.

## Later Work

Tracked in #502/#505/#532: release-history-to-digest input, additional controller
adapters, richer remote-target binding, multi-source/rendering bundles,
pod/artifact identity, and explicitly budgeted watch/bot release checks.
These are not prerequisites for the first literal-bundle check.

## Local Verification

Recorded on 2026-09-11: full suite passed twice; focused checks passed repeatedly,
including race detection and a final run after documentation. Build, vet,
read-only guard, CLI guide parity, CLI docs, doc freshness and module tidy pass.
The standalone fixture generator is subprocess-tested, including refusal to
overwrite an existing layout. Actual terminal entry/scroll/refresh/close passed
against a local layout and read-only cluster observations.

Positive controller joins are deterministic HTTP fixtures, not production
validation. The live Deployment smoke demonstrates configuration/convergence
reads and an honest unsupported-controller result. No existing cluster or
registry resources, credentials, current context or installed plugins changed.
CI passed for the exact merged file tree:
[run 34637625505](https://github.com/confighub/cub-scout/actions/runs/34637625505)
at head `538a255`; merge `bd70fc6` has the identical tree. Unit, Integration,
GitOps E2E and Proof Artifact passed; Connected E2E, Full Verification and Demo
Tests were skipped. Publication and the broader v2.11 observation roadmap are
separate work; completion of this scoped plan does not mean released.
