# Sveltos inferred revision versus delivery receipt

This is a single recorded-evidence case prepared for DEL-03 review. It has not
been run. It is mapped in benchmark-v1 as a prepared source projection, not
admitted to the baseline or evidence of product quality. Missing/stale input
variants remain pending; rejected grader answers do not replace those controls. Both arms receive identical
case-owned files through `scaffold.sh`; the case requires no MCP call and no
live or shell access.

## Success criteria

A verified answer distinguishes the short status revision and its documented
time-based inference from the receipt's full ConfigHub release/profile
evidence. It must not claim the receipt identifies the Sveltos runtime digest
or joins the separately scoped status record. The deterministic grader accepts
only the exact unordered JSON contract and rejects swapped identity, full-vs-
short digest, runtime-binding, person, freshness, and atomic-join claims. The
unit test verifies fixture hashes, byte-identical scaffolding, and grader
behavior in Python and JavaScript. This case remains prepared, not run.

The source-document excerpts come from `confighub/sveltos-confighub` at pinned
commit `8187910f9fe226e109e55c4d9c7c0e21297ff424`:

| Fixture | Original source | Whole-source SHA-256 | Selected lines / excerpt SHA-256 |
|---|---|---|---|
| `fixtures/source-doc/onboard-excerpt.md` | `docs/user/onboard-your-sveltos-fleet.md` | `2ead82bbbd648bfdd12aaa45f6f6401c87efd8f8e93aac4aa84b42f8bdcce62e` | 537–562 / `c32e6ffa43b150517ef5afded1ee2ab50401e25071199f0b0a0b7c76ed096b4b` |
| `fixtures/source-doc/known-behaviours-excerpt.md` | `docs/user/known-behaviours.md` | `db709e29616470f90a04972a2c06758e91b9ee0236a52d24deb42c335ff5c399` | 36–38 / `142a9dfd6119e4d3fff6cdf82b99ebae26ace4f5286992706a19b4e470ec7751` |

The status documentation says Sveltos does not report which release it fetched;
the displayed shortened revision is worked out from timestamps. Close release
creation times or clock disagreement can lead to the wrong mapping. It is not
proof that a cluster runs that exact release.

The third file is the full `runs/sveltos-oci-delivery-proof/receipt.yaml` at
the same source commit (SHA-256
`9aa3ac57090001aa5b37e5a91c095aaca95ea2a91edcbb2bdb77fe526aa857b2`). It is a
separate ConfigHub delivery receipt recorded on 2026-08-14 for
`hx-sveltos-fleet-pilot`. It records full release/manifest digests and
`profileMatchesApprovedRevision: true`; its workload observation records a
deployed `kyverno-3.8.1` HelmRelease. The `eu-central-uat1` status row itself
has no exact observation timestamp in the excerpt. The receipt does not bind a
full Sveltos runtime digest to that row. These are different artifacts and
named clusters, not one atomic runtime capture. The receipt's workload evidence
is not a Sveltos runtime digest join.

These are pinned source-document excerpts and a checked-in receipt projection,
not raw Kubernetes/Sveltos runtime output or a fresh capture. They contain no
Secret payload or credentials. The case stores no authored/reference graded
answer. The grader checks a bounded JSON contract only; no claim is made about
arbitrary explanation quality.

## Reproduce the prepared files

From the repository root, run the case scaffold in a temporary working
directory and compare the three output files byte-for-byte with the fixtures.
The unit test `TestSveltosInferredRevisionEvidenceAndGrader` performs this
check. No source-repository checkout or cluster access is needed to reproduce
the checked-in case.
