# Helm Experiment Install Verification

This example shows where cub-scout fits with
[`confighub/helm-expt`](https://github.com/confighub/helm-expt) and
[`confighub/installer`](https://github.com/confighub/installer).

The short version:

```text
helm-expt proves:   Helm render == cub install render
installer proves:   package/spec -> rendered objects -> ConfigHub Units/OCI
cub-scout proves:   rendered objects are present and matching in the live cluster
```

Use `receipt verify --file`, not a new `verify install receipt` verb. The
receipt command family is already the artifact surface; `verify` builds a new
receipt, and `--file` selects the install/object-set predicate.

## What This Proves

The new predicate is `object-set-matches`:

```bash
./cub-scout receipt verify \
  --file out/manifests \
  --scope namespace/redis \
  --format json \
  --out install.object-set.receipt.json
```

It emits an in-toto Statement v1 receipt with:

- subject `rendered-object-set://sha256/<id>` for the desired YAML set
- subject `k8s-live-object-set://namespace/<ns>` for the observed live set
- predicate `object-set-matches`
- evidence summary for matched, missing, mismatched, and inconclusive objects

PASS means every desired object identity in the rendered YAML was present live
and every authored field still matched. Kubernetes server-added map fields and
`status` are not part of the claim. Missing objects or changed authored fields
produce BLOCK. API mapping or read gaps produce INCONCLUSIVE.

The receipt deliberately includes an `extra-live-object-coverage` omission:
cub-scout verifies the desired rendered set, but does not yet prove that no
extra live resources exist outside that desired identity set.

## Redis Walkthrough

In `confighub/helm-expt`, the Redis proof already has the render-side checks:

```bash
npm run redis:compare
npm run redis:verify-proof
npm run redis:verify-package
npm run redis:verify-use-more-now
npm run top20:verify-local-e2e
```

As of May 28, 2026, those checks verified:

- Redis Helm and `cub install` semantic equivalence for `default` and
  `reuse-existing-secret`
- Redis proof/package/use-more-now receipts
- 20 top-chart local kind observation receipts

After rendering or applying a variant, add the cub-scout runtime receipt:

```bash
# From a cub install work directory:
./cub-scout receipt verify \
  --file /tmp/redis-default/out/manifests \
  --scope namespace/redis \
  --format json \
  --out /tmp/redis-default/install.object-set.receipt.json \
  --fail-on any-non-pass
```

For the checked-in Redis rendered revision, use the exact rendered object file
that was applied:

```bash
./cub-scout receipt verify \
  --file recipes/bitnami/redis/25.5.3/revisions/default/r001/rendered/release-objects.yaml \
  --scope namespace/redis \
  --format json \
  --out runs/redis-local-kind/latest/cub-scout-object-set.receipt.json \
  --fail-on any-non-pass
```

If the install intentionally separates Secrets or requires target facts, verify
the applied manifest set, not an abstract chart source. For Redis default, the
installer separates one Secret; for `reuse-existing-secret`, the target Secret
is external. Include support-object YAML in `--file` only when that object is
part of the install contract you want cub-scout to verify.

## Runtime Verification Menu

The receipt is the durable proof artifact. Before producing it, use these
commands to build the operator's mental map of the install. Treat this as a
menu: a CI job may run only the gates, while a human demo can walk more of the
story.

Set the variables for the run you are inspecting:

```bash
NS=redis
WORKLOAD=statefulset/redis-master
MANIFESTS=recipes/bitnami/redis/25.5.3/revisions/default/r001/rendered/release-objects.yaml
RUN_DIR=runs/redis-local-kind/latest
```

Core live-cluster checks:

| Question | Command |
|----------|---------|
| Is the namespace broadly healthy? | `./cub-scout map status --namespace "$NS" --json` |
| What should an operator look at first? | `./cub-scout doctor --namespace "$NS" --format json` |
| Which objects exist, and who owns them? | `./cub-scout map list --namespace "$NS" --format json` |
| Which GitOps deployers are present? | `./cub-scout map deployers --json` |
| Did the delivery controller converge? | `./cub-scout gitops status -n "$NS" --json` |
| What changed recently? | `./cub-scout map activity --namespace "$NS" --since 1h --format json` |
| Are there obvious broken workloads or deployers? | `./cub-scout map issues --namespace "$NS" --json` |
| How is ownership grouped? | `./cub-scout tree ownership --namespace "$NS" --format md` |
| What is this workload, in plain language? | `./cub-scout explain "$WORKLOAD" -n "$NS" --format md` |
| Where did this workload come from? | `./cub-scout trace "$WORKLOAD" -n "$NS" --artifacts --format json` |
| Is the rendered YAML risky before apply? | `./cub-scout scan --file "$MANIFESTS" --json` |
| Are runtime reconcilers stuck? | `./cub-scout scan -n "$NS" --state --json` |
| Did key authored fields drift? | `./cub-scout compare drift --file "$MANIFESTS" -n "$NS" --format json --fail-on warning` |
| Do ConfigHub intent, rendered state, and live state agree? | `./cub-scout compare three-way --scope namespace/"$NS" --format json --fail-on warning` |
| Does the declared ConfigHub delivery strategy pass? | `./cub-scout compare source-truth "$WORKLOAD" -n "$NS" --strategy confighub-oci-argo` |

Useful supporting artifacts:

```bash
mkdir -p "$RUN_DIR"

./cub-scout graph export -n "$NS" \
  --format html \
  -o "$RUN_DIR/cub-scout-graph.html"

./cub-scout snapshot -n "$NS" \
  --relations \
  -o "$RUN_DIR/cub-scout-snapshot.json"

./cub-scout context-pack -n "$NS" \
  --format json > "$RUN_DIR/cub-scout-context-pack.json"

./cub-scout patterns detect -n "$NS" \
  --json > "$RUN_DIR/cub-scout-patterns.json"
```

Then close the loop with the install/object-set receipt:

```bash
./cub-scout receipt verify \
  --file "$MANIFESTS" \
  --scope namespace/"$NS" \
  --format json \
  --out "$RUN_DIR/cub-scout-object-set.receipt.json" \
  --fail-on any-non-pass
```

## Full Proof Chain

The strongest story is a sequence of independent receipts:

```text
1. Helm equivalence proof
   npm run redis:compare

2. Installer package proof
   npm run redis:verify-package

3. ConfigHub upload / OCI proof
   runs/redis-confighub/latest/upload-oci-receipt.yaml

4. Live cluster object-set proof
   cub-scout receipt verify --file ... --scope namespace/redis

5. Optional workload/source-truth proof
   cub-scout receipt verify deploy/redis -n redis --strategy confighub-oci-argo
```

Today, `--input-attestation` chains prior cub-scout receipts whose fingerprints
can be verified by cub-scout. Keep `helm-expt` receipts adjacent in the run
directory until those upstream receipts are emitted or bridged as cub-scout /
in-toto Statement receipts.

## Why This Matters

Before this receipt, cub-scout could say useful runtime things:

- `doctor` / `map status`: are workloads healthy?
- `gitops status` / `trace --artifacts`: did the delivery controller converge?
- `compare drift`: do workload fields differ?
- `compare source-truth`: do ConfigHub / controller / runtime agree under a
  declared strategy?

The missing piece was an install-level runtime receipt:

```text
I expected this rendered object set.
I observed the live cluster.
Every desired object and authored field matched.
Here is the fingerprinted receipt.
```

That is the piece `helm-expt` needs to show Helm and ConfigHub+installer can be
equivalent not only at render time, but also after the objects land in a real
cluster.
