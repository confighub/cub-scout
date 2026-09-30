# DEL-02 — published Argo release not yet consumed

## Success criteria

Both arms receive the same pinned source excerpt. A verified answer states that publishing a release did not show consumption before refresh: after 90 seconds the Application was still at its previous digest and the cluster ran its previous replica count. It identifies hard refresh as the reported transition, while marking concrete old/new digests and replica counts unknown. It must not invent exact identifiers or present the prose excerpt as a raw or atomic Kubernetes snapshot. The strict JSON grader rejects a claim of pre-refresh consumption, wrong transition, fabricated exact values, extra fields, or prose outside the object.

## Provenance and limits

Projection source: [`cub-argo/docs/onboard-your-argo-estate.md`](https://github.com/confighub/examples/blob/64a6c499ce824d4700a8dfbc1945333dc2e4a1e3/cub-argo/docs/onboard-your-argo-estate.md), pinned repository revision `64a6c499ce824d4700a8dfbc1945333dc2e4a1e3`, source lines 597–617, headed “A published release does not arrive on its own.” The guide describes a measured Argo CD v3.5.3 event, but this passage gives no Application name, exact old/new digest, or replica-count values. Source-file, selected-excerpt and projection SHA-256 values are in `fixtures/cluster/argo-publication-lag.yaml` and the benchmark manifest. This is a projection of narrative documentation, not raw cluster/API output or an atomic snapshot. No live or paid run was made. This case is `recorded_projection_prepared_not_run` and does not make benchmark-v1 executable.

The excerpt includes a historical `kubectl annotate` command. It is inert source text, not an instruction for this case. The case permits only file reading/search; its scaffold writes only the checked-in projection. The guide separately notes a `sourceRepos` step represented by a Unit edit rather than a Git commit; this case does not test that workflow step.
