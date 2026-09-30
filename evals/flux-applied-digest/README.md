# DEL-01 — exact Flux applied apps digest

## Success criteria

Both arms receive byte-identical pinned source excerpt. The verified answer names the full `apps` release digest and full Git revision reported alongside it, treats the digest—not its timestamp—as release identity, and says the input is a trimmed sequential run log rather than raw/atomic cluster data. It must not substitute the Git SHA for the applied release digest or claim workload-specific identity absent from the excerpt. The exact JSON grader rejects truncated/changed digests, timestamp-only identity, an incorrect evidence/snapshot limit, extra fields, or prose outside the object.

## Provenance and limits

Projection source: [`cub-flux/docs/runs/2026-09-30-bootstrapped-handover.md`](https://github.com/confighub/examples/blob/64a6c499ce824d4700a8dfbc1945333dc2e4a1e3/cub-flux/docs/runs/2026-09-30-bootstrapped-handover.md), pinned repository revision `64a6c499ce824d4700a8dfbc1945333dc2e4a1e3`. The excerpt combines source lines 1–10 (run context and the author's note that output is trimmed) and 66–86 (handover output and later UID/rollout-revision statement). Source-file, selected-excerpt and checked-in projection SHA-256 values are in `fixtures/cluster/flux-applied-digest.yaml` and the benchmark manifest. This is a projection of a public Markdown run log, not saved raw cluster/API output. It does not establish an atomic observation or independently bind every workload object to the digest; don't overstate that join. No live or paid run was made. This case is `recorded_projection_prepared_not_run` and does not make benchmark-v1 executable.

The handover excerpt contains printed historical administrative commands. They are inert source text. The case permits only file reading/search and the scaffold writes only the checked-in projection.
