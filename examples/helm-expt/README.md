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

This is a general pattern for any chart/release that `helm-expt` can render
and compare. Redis is the first worked value set, not the shape of the whole
example.

## Helm release-secret decoding contract (#588, partial)

Before release decoding can be trusted, a valid stored release must retain its
name, namespace, and positive version. A malformed or over-limit candidate
Helm Secret must produce explicit incomplete/error evidence from release
listing, direct lookup, and history; it must never disappear silently and make
an older version look current or history look empty. A namespace with no Helm
release Secrets keeps the existing no-record behavior. Decoder limits are 9
MiB of encoded `data.release`, 6 MiB after base64 decode, and 32 MiB of
decompressed JSON. Decoder errors identify the failure class without echoing
secret contents.

This work affects standalone `trace` paths that read Helm's Kubernetes Secret
storage. The same tracer's connected consumer, where present, must preserve the
explicit error/incomplete result; this packet adds deterministic unit coverage,
not a connected integration run. It does not cover SQL or other Helm storage
drivers, exact parsed manifest-to-resource identity, hooks, application
success, or any Helm 3/4 compatibility matrix. A valid `deployed` release is
still only Helm's stored release status. The caps bound each `data.release`
decode only; they do not bound Secret-list cardinality/response bytes or API
request duration. This packet does not add pagination, namespace inventory
limits, or a new request timeout; those remain separate limits on the caller
and Kubernetes client.

The optional disposable-cluster smoke is
[`reproduce-helm-release-secret-trace.sh`](./reproduce-helm-release-secret-trace.sh).
It creates a uniquely named kind cluster, builds a minimal deterministic chart
with an explicit Deployment namespace and fixed `registry.k8s.io/pause:3.9`
image, traces that release, and asserts JSON reports Helm ownership and exactly
one matching Deployment chain node before deleting only the cluster it
created. It is a valid-release smoke only; it does not test malformed
candidates or fallback behavior. It is not run by unit tests and was not run
for the original bounded-decoding packet (#661); its output is not Helm-version parity evidence. It preserves
Helm version, rendered-chart hash, command outputs/status, and Secret names in
the requested evidence directory (or a printed temporary directory) without
recording Secret payloads. A non-empty requested evidence directory is refused
without modifying its contents; verify this locally with
`bash examples/helm-expt/test-reproduction-evidence-safety.sh`. The deterministic unit contract includes valid
releases, no records, denied Secret listing, malformed encodings/data,
oversize expansion, invalid identity metadata, and an older valid record beside
a bad candidate in either listing order.

On 2026-09-30, the lead ran this smoke at source `e8e7523` for #663 using
Helm `v4.1.4+g05fa379`. It exited 0 and reported Helm ownership with the exact
`Deployment/helm-release-decode/release-probe` chain node. The owned cluster
was removed and the shared-context result was unchanged. This validates that
minimal explicit-namespace release only, not malformed live cases or a
Helm 3/4 compatibility matrix. See [the PR proof](https://github.com/confighub/cub-scout/pull/663).

## Helm manifest identity contract (#588, exact matching)

Success criteria: a resource trace may identify a release only when one parsed
manifest document has the exact caller-canonicalized `kind`, exact
`metadata.name`, and an explicit `metadata.namespace` equal to the requested
namespace. The existing tracer API has no `apiVersion` input, so API version
cannot narrow candidates; duplicate matching documents or releases, including
matches at different API versions, are ambiguous and must return an explicit
error. Namespace omission by the caller or a candidate document is unresolved;
Helm release namespace defaults and cluster/custom-resource scope are not
inferred. Malformed, non-object, or unsupported nonempty manifest documents
must return an explicit error rather than turn unreadable data into a confirmed
absence. Empty and comment-only YAML documents are ignored.

The fixture set in `pkg/agent/testdata/helm-manifest-identity/` covers one
explicit exact match (including nested, comment, and prefix lookalikes), wrong
and omitted namespaces, malformed/unsupported or apiVersion-less documents
(including malformed evidence after an otherwise exact match), non-string
identity scalars, `kind: List`, and duplicate identities at the same or
different API versions. Tests reverse candidate document and release order;
only a unique exact match may produce a managed trace. No Kubernetes API
discovery or scope guess is introduced.

## Runnable Demo (Self-Contained)

The rest of this page is the full integration narrative against a real
helm-expt render. If you just want to run the `object-set-matches` path
end-to-end with no helm-expt checkout, use the bundled scripts and fixture:

```bash
cd examples/helm-expt
./setup.sh     # ephemeral kind cluster + fixtures/release-objects.yaml
./verify.sh    # runs object-set-matches + prerequisites-met + workloads-converged
./cleanup.sh   # tears the cluster down
```

`verify.sh` runs all three install-receipt predicates against the same install and
prints a scorecard. The fixture reproduces helm-expt finding **F3**, so
`object-set-matches` is `PASS` (present + match) — a false green on its own — while
`prerequisites-met` (#477) BLOCKs pre-flight (the required Secret is absent) and
`workloads-converged` (#476) BLOCKs at runtime (the pod is in
`CreateContainerConfigError`); `--ttl` stamps receipt freshness (#478). The
remaining gaps and the full set are in
[`docs/proposals/helm-expt-driven-gaps.md`](../../docs/proposals/helm-expt-driven-gaps.md).
Start with [AI_START_HERE.md](./AI_START_HERE.md). The scripts never touch a
helm-expt checkout.

## Why This Exists

cub-scout already answers useful runtime questions:

- `doctor` / `map status`: are workloads healthy?
- `gitops status` / `trace --artifacts`: did the delivery controller converge?
- `compare drift`: do supported workload fields, such as replicas and images,
  differ?
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

## Render-Side Proof

Start in `confighub/helm-expt`. For the chart/release under test, run the
matching render/equivalence/package checks from that repository.

For the current Redis worked case:

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

For another chart, replace the Redis scripts and paths with that chart's
equivalent `helm-expt` recipe and run directory.

## Runtime Setup

After rendering and applying/syncing a chart variant, point cub-scout at the
same live cluster and the exact rendered object file or directory that was
applied.

Build cub-scout from this repository, then set the values for the run you are
inspecting:

```bash
go build ./cmd/cub-scout

HELM_EXPT=/path/to/helm-expt
NS=example-namespace
WORKLOAD=deployment/example
MANIFESTS=/path/to/rendered/release-objects.yaml
RUN_DIR=/path/to/run-output
DELIVERY_STRATEGY=confighub-oci-argo
mkdir -p "$RUN_DIR"
```

Redis worked values:

```bash
HELM_EXPT=/path/to/helm-expt
NS=redis
WORKLOAD=statefulset/redis-master
MANIFESTS="$HELM_EXPT/recipes/bitnami/redis/25.5.3/revisions/default/r001/rendered/release-objects.yaml"
RUN_DIR="$HELM_EXPT/runs/redis-local-kind/latest"
DELIVERY_STRATEGY=confighub-oci-argo
mkdir -p "$RUN_DIR"
```

If the install intentionally separates Secrets or requires target facts, verify
the applied manifest set, not an abstract chart source. Include support-object
YAML in `--file` only when that object is part of the install contract you want
cub-scout to verify.

## Runtime Verification Menu

Use these commands to build the operator's mental map of the install. Treat
this as a menu: a CI job may run only the gates, while a human demo can walk
more of the story.

Standalone live-cluster checks:

| Question | Command |
|----------|---------|
| Is the namespace broadly healthy? | `./cub-scout map status --namespace "$NS" --json` |
| What should an operator look at first? | `./cub-scout doctor --namespace "$NS" --format json` |
| Which objects exist, and who owns them? | `./cub-scout map list --namespace "$NS" --format json` |
| Which GitOps deployers are present? | `./cub-scout map deployers --json` |
| Did GitOps deployers converge? | `./cub-scout gitops status --json` |
| What changed recently? | `./cub-scout map activity --namespace "$NS" --since 1h --format json` |
| Are there obvious broken workloads or deployers? | `./cub-scout map issues --namespace "$NS" --json` |
| How is ownership grouped? | `./cub-scout tree ownership --namespace "$NS" --format md` |
| What is this workload, in plain language? | `./cub-scout explain "$WORKLOAD" -n "$NS" --format md` |
| Where did this workload come from? | `./cub-scout trace "$WORKLOAD" -n "$NS" --artifacts --format json` |
| Is the rendered YAML risky before apply? | `./cub-scout scan --file "$MANIFESTS" --json` |
| Are runtime reconcilers stuck? | `./cub-scout scan -n "$NS" --state --json` |
| Did supported drift fields, such as replicas and images, drift? | `./cub-scout compare drift --file "$MANIFESTS" -n "$NS" --format json --fail-on warning` |

Optional connected ConfigHub checks:

| Question | Command |
|----------|---------|
| Do ConfigHub intent, rendered state, and live state agree? | `./cub-scout compare three-way --scope namespace/"$NS" --format json --fail-on warning` |
| Does the declared ConfigHub delivery strategy pass? | `./cub-scout compare source-truth "$WORKLOAD" -n "$NS" --strategy "$DELIVERY_STRATEGY"` |

Useful supporting artifacts:

```bash
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

## Install Receipt

Close the loop with the install/object-set receipt:

```bash
./cub-scout receipt verify \
  --file "$MANIFESTS" \
  --scope namespace/"$NS" \
  --format json \
  --out "$RUN_DIR/cub-scout-object-set.receipt.json" \
  --fail-on any-non-pass
```

Use `receipt verify --file`, not a new `verify install receipt` verb. The
receipt command family is already the artifact surface; `verify` builds a new
receipt, and `--file` selects the install/object-set predicate.

## What the Receipt Proves

The predicate is `object-set-matches`. It emits an in-toto Statement v1
receipt with:

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

For a `cub install` work directory, use the same shape:

```bash
INSTALL_WORKDIR=/tmp/chart-run

./cub-scout receipt verify \
  --file "$INSTALL_WORKDIR/out/manifests" \
  --scope namespace/"$NS" \
  --format json \
  --out "$INSTALL_WORKDIR/install.object-set.receipt.json" \
  --fail-on any-non-pass
```

## Set-level diff receipt (`object-set-diff`, #496)

`object-set-matches` answers a boolean ("does the whole set match?") and
`compare three-way` answers per resource. Neither emits a single **set-level
delta receipt**. `compare object-set --dry-from` does: it diffs a rendered
(desired) object set against live and emits an `object-set-diff` receipt
aggregating per-object **authored-field deltas** (`changedObjects`), plus
`removedObjects` and — with `--diff` — `addedObjects`.

One receipt shape serves both day-1 and day-2, tool-agnostically (it reads the
cluster + the `--dry-from` render, never the reconciler):

```bash
# drift (post-apply): does the current render still match live across the set?
./cub-scout compare object-set \
  --dry-from "$MANIFESTS" \
  --scope namespace/"$NS" \
  --format json \
  --out "$RUN_DIR/object-set-diff.drift.receipt.json"

# dry-run (pre-apply): what would a PROPOSED change (e.g. an image.digest bump)
# touch across the set? A changed image -> BLOCK with one changedObject.
./cub-scout compare object-set \
  --dry-from "$RUN_DIR/release-objects.changed-image.yaml" \
  --scope namespace/"$NS" \
  --format json \
  --fail-on any-non-pass
```

Verdict: **BLOCK** if any object has authored-field deltas; **WATCH** if only
closure deltas (added/removed objects); **PASS** if none. The receipt is signed
(fingerprint) and chain-walkable, like the other receipts. `verify.sh` runs both
the drift and dry-run cases against this example's live fixture, reproducing the
helm-expt#992 `image.digest` worked example offline. (Value-provenance per
changed object is Issue B; a cross-fleet roll-up is Issue C — see
[`docs/proposals/object-set-diff.md`](../../docs/proposals/object-set-diff.md).)

## Full Proof Chain

The strongest story is a sequence of independent receipts:

```text
1. Helm equivalence proof
   helm-expt compare/verify command for this chart variant

2. Installer package proof
   helm-expt package verification for this chart variant

3. ConfigHub upload / OCI proof
   run-specific upload or OCI receipt, when that path is used

4. Live cluster object-set proof
   cub-scout receipt verify --file ... --scope namespace/<namespace>

5. Optional workload/source-truth proof
   cub-scout receipt verify "$WORKLOAD" -n "$NS" --strategy "$DELIVERY_STRATEGY"
```

Today, `--input-attestation` chains prior cub-scout receipts whose fingerprints
can be verified by cub-scout. Keep `helm-expt` receipts adjacent in the run
directory until those upstream receipts are emitted or bridged as cub-scout /
in-toto Statement receipts.

## Pinned Helm 3/4 release-secret matrix (prepared; not run)

The companion `run-helm-version-matrix.sh` defines a disposable, serial matrix
for the bounded namespaced Deployment case. It uses one newly created pinned
kind cluster and three isolated releases: fresh Helm 3.22.0 install, fresh
Helm 4.1.4 default install, and Helm 3.22.0 install followed by Helm 4.1.4
`upgrade --server-side=auto` with replicas changed from one to two. Each run
records binary/image/chart hashes, exact commands, Deployment identity/UID,
replica counts, managedFields, Secret names only, release history, and matching
standalone/plugin `trace --format json` results. The script refuses existing
cluster/evidence state and uses a private kubeconfig and Helm homes.

After review, the intended invocation is:

```bash
mkdir /path/to/new-empty-evidence
HELM3_BIN=/absolute/path/to/helm-v3.22.0 \
  examples/helm-expt/run-helm-version-matrix.sh /path/to/new-empty-evidence
```

Pins are kind `v0.31.0`, node image
`kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661`,
Helm 3.22.0 (`8566ea7d76445d174050eca068bc6d685ba3607de8430524a809a16f505a6138`),
and Helm 4.1.4 (`11e3c9fb6548fa1661a72000a6a483a31f1c2a0bf300f3d2422270feee180d34`).
The offline preflight guard is `examples/helm-expt/test-helm-version-matrix-safety.sh`;
it checks evidence preservation, rejects a binary hash mismatch, and proves a
pre-existing kind cluster is refused before cluster creation.

This is a validation protocol, not evidence that the matrix has passed. It
covers ordinary namespaced install/upgrade and does not cover hooks, CRDs,
rollback, or server-side conflict modes. Manager fields are recorded as
observations; they do not by themselves prove which apply method Helm used.
Run only in an explicitly authorized disposable environment after reviewing
the script and ensuring no other live lane is active.
