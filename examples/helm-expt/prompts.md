# Prompts

Copy-paste starting points for driving this example with an AI assistant.

## Run the reproduction

```
Run ./setup.sh then ./verify.sh in examples/helm-expt. Read the gap table in the
verify output. Explain, in two sentences, why the object-set-matches receipt is
PASS while the pod is in CreateContainerConfigError. Then run ./cleanup.sh.
```

## Inspect the receipt

```
Read examples/helm-expt/out/object-set.receipt.json. Confirm: the verdict, that
status is not in the evidence, that there is no freshness/TTL field, and quote the
self-declared omission. Map each finding to an issue in
docs/proposals/helm-expt-driven-gaps.md.
```

## Use it as a spec for a new predicate

```
Using docs/proposals/helm-expt-driven-gaps.md as the spec, sketch the evidence
shape for a workloads-converged predicate: which kinds it reads status for, the
PASS/WATCH/BLOCK rules, and how it chains to an object-set-matches receipt via
--input-attestation. Do not change object-set-matches.
```

## Point it at a real helm-expt render (optional, read-only)

```
I have a helm-expt checkout at $HELM_EXPT. Without modifying that repo, run:
./cub-scout receipt verify --file "$HELM_EXPT/recipes/bitnami/redis/25.5.3/revisions/default/r001/rendered/release-objects.yaml" \
  --scope namespace/redis --format json --out /tmp/redis.receipt.json --fail-on any-non-pass
against a cluster where that render was applied, and summarize the verdict and any omissions.
```
