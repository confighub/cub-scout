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
an older version look current or history look empty. If the `owner=helm` query
returns no candidates, the existing no-record behavior remains; Secrets without
that discovery label are not enumerated. Decoder limits are 9
MiB of encoded `data.release`, 6 MiB after base64 decode, and 32 MiB of
decompressed JSON. Decoder errors identify the failure class without echoing
secret contents.

Release selection also checks the identity fields on each candidate returned
by the `owner=helm` Secret query: Secret and requested namespace, decoded
payload namespace, canonical `sh.helm.release.v1.<name>.v<version>` key,
`helm.sh/release.v1` type, and the `owner`, `name`, and `version` labels must
agree. Missing or inconsistent fields return an incomplete/error result; they
cannot make an older revision appear current. If a query returns repeated
evidence for one release revision, only byte-identical stored payloads with
matching Secret identity metadata are deduplicated. Any payload-byte or
identity-metadata difference for the same name/version is ambiguous,
independent of list order; the decoded projection does not cover every field
in Helm's stored record. Latest-release listing, direct release lookup,
resource tracing, and history use the same checks. Direct lookup/history query `owner=helm`
records before validating and selecting the requested payload name, so a
returned candidate with a mismatched `name` label is not hidden by a narrower
label selector.

An empty or whitespace-only namespace is rejected before any Secret request;
direct lookup and history also reject a blank release name before reading.
An omitted namespace never becomes an all-namespace Secret list.

These deterministic Secret metadata fixtures are synthetic and mirror the
storage shape in [Helm v3.17.3](https://github.com/helm/helm/blob/v3.17.3/pkg/storage/storage.go)
and [v4.0.0](https://github.com/helm/helm/blob/v4.0.0/pkg/storage/storage.go)
storage code and [Secret drivers](https://github.com/helm/helm/blob/v3.17.3/pkg/storage/driver/secrets.go).
The corresponding [v4 Secret driver](https://github.com/helm/helm/blob/v4.0.0/pkg/storage/driver/secrets.go)
uses the same storage metadata shape.
They are not captured releases or a compatibility result. The separately
recorded valid-release smoke used Helm v4.1.4; it does not prove malformed-record
behavior or Helm 3/4 parity.

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

Helm's release time wrapper serializes a zero `time.Time` as `""`; this is a
valid stored value for fields such as `info.deleted` on a newly deployed
release. The decoder treats omitted, `null`, and empty-string timestamps as
zero/unknown, preserves valid RFC3339 timestamps, and rejects malformed or
non-string values. This is deterministic decoder compatibility coverage
(`#676`), not evidence from a Helm 3 live matrix; Helm 4 compatibility remains
unclaimed by this change.

On 2026-09-30, the lead ran this smoke at source `e8e7523` for #663 using
Helm `v4.1.4+g05fa379`. It exited 0 and reported Helm ownership with the exact
`Deployment/helm-release-decode/release-probe` chain node. The owned cluster
was removed and the shared-context result was unchanged. This validates that
minimal explicit-namespace release only, not malformed live cases or a
Helm 3/4 compatibility matrix. See [the PR proof](https://github.com/confighub/cub-scout/pull/663).

The same harness supports an optional metadata-only negative identity probe:
`./examples/helm-expt/reproduce-helm-release-secret-trace.sh --negative-identity`.
After the valid trace, it changes only the created release Secret's `version`
label while leaving the payload and key untouched. The current binary must fail
with explicit incomplete/inconsistent identity evidence. For comparison on the
same corrupted Secret, set `CUB_SCOUT_BASELINE_BIN` to an absolute path to a
previously built executable; the script records whether it still reports a
confident Helm chain. It records Secret metadata only, never `.data` or decoded
release contents. This negative mode was added for reproducible lead-run proof
and has not been run by this packet.

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

## Pinned Helm 3/4 release-secret matrix (bounded install/upgrade proof passed)

The companion `run-helm-version-matrix.sh` defines a disposable, serial matrix
for the bounded namespaced Deployment case. It uses one newly created pinned
kind cluster and three isolated releases: fresh Helm 3.22.0 install, fresh
Helm 4.1.4 default install, and Helm 3.22.0 install followed by Helm 4.1.4
`upgrade --server-side=auto` with replicas changed from one to two. Each run
records binary/image/chart hashes, exact commands, Deployment identity/UID,
replica counts, managedFields, Secret names only, release history, and matching
standalone/plugin `trace --format json` results. The script refuses an existing
`scout-helm-version-matrix` cluster and non-empty evidence state. It leaves
unrelated kind clusters untouched and uses an explicit private kubeconfig and
isolated Helm homes.

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
The Helm 3 archive SHA-256 is
`4c9982a6cdeb458b60258df66b55398ca5b19293f6877faffe2909ad6f23dfe0`. These
hashes pin the tested Darwin arm64 Helm executables; other binaries are
rejected even if they report the same version.
The offline preflight guard is `examples/helm-expt/test-helm-version-matrix-safety.sh`;
it checks evidence preservation, rejects a binary hash mismatch, refuses only
the owned cluster name, leaves unrelated cluster names alone, and verifies
failed-create cleanup uses the private kubeconfig without targeting another
cluster. Deployment reads request `managedFields` explicitly and verify that
the Helm 4 upgrade preserves the Deployment UID while changing replicas.

This is a validation protocol, not evidence that the matrix has passed. It
covers ordinary namespaced install/upgrade and does not cover hooks, CRDs,
rollback, or server-side conflict modes. Manager fields are recorded as
observations; they do not by themselves prove which apply method Helm used.
Run only in an explicitly authorized disposable environment after reviewing
the script and ensuring no other live lane is active.

The first run at script source `9e6a5bb` installed Helm 3 successfully but Scout
rejected its release JSON. It stopped before Helm 4; see #676 for the empty
timestamp regression. The owned cluster was removed. This is retained as a
failed run, not a passing compatibility result.

For the regression repeat, set `BASELINE_SCOUT_BIN` to the frozen pre-fix
Scout binary with SHA-256
`ff6f5a1200a3dae06691c80e5ca4120478c0500ee3a6e18d97ff6b675aa1d16b`.
The harness requires that binary to fail on the same fresh Helm 3 release
that the newly built binary successfully traces. Secret name/UID/resourceVersion
must remain identical across the comparison; no Secret payload is saved.
The comparison is optional for subsequent ordinary matrix runs, and the
pinned baseline hash is checked before any cluster creation.

The September 30 repeat at source `0ef39b5` passed the three stated scenarios
on Kubernetes 1.35.0, after the #676 timestamp fix. The old binary failed on
the same unchanged Helm 3 Secret that the fixed binary successfully traced.
Standalone/plugin projections agreed for all four observations. The Helm 4
upgrade changed ready replicas from one to two while preserving Deployment UID.
See the [result projection and retained raw-artifact hashes](evidence/2026-09-30-release-matrix.json).
The owned cluster was deleted; shared context/config hashes and other cluster
names were unchanged. This does not cover hooks, CRDs, rollback or conflicts,
and manager names alone still do not establish apply method. The earlier
failed run remains part of the evidence history.

### Extended lifecycle acceptance (v2.13 candidate)

The existing matrix has an opt-in `--lifecycle` lane, sourced from
`helm-lifecycle-matrix.sh`. This is an acceptance harness, not a Scout renderer
or migration command. Its required assertions were defined in #588:

- Both pinned Helm versions install a namespaced dependent custom resource after
  its CRD and retain a successful hook Job's actual Complete condition.
- A failing post-upgrade hook must fail the Helm command, retain a failed release
  history row and produce a different Job with a Failed condition.
- Explicit rollback to revision 1 must restore the dependent configuration and
  converged replica count; standalone/plugin Scout traces must agree.
- A changed CRD file must remain separate from the actual CRD: ordinary Helm
  upgrade does not update the `crds/` resource. This is observed behavior, not
  evidence that the CRD change was applied.
- Deleting the retained hook must leave an empty exact-name Job list. Stored hook
  definitions/history remain historical, not proof of a current Job.
- A foreign server-side replica-field owner must make an explicit Helm 4
  server-side transition fail for a conflict without changing that field. An
  explicit force-conflicts retry must converge with the Deployment trace intact.

```bash
mkdir /path/to/new-empty-lifecycle-evidence
HELM3_BIN=/absolute/path/to/helm-v3.22.0 \
  examples/helm-expt/run-helm-version-matrix.sh \
  /path/to/new-empty-lifecycle-evidence --lifecycle
```

The hook uses the official BusyBox 1.37.0 OCI index pinned to
`sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e`;
all existing Helm/kind pins and owned-cluster cleanup apply. Retain failed runs
and cleanup status. The summary's `lifecycleMatrixCompleted` is true only after
these required assertions pass. Application functionality, connected governance
and SQL release storage remain outside this fixture's proof. No successful live
result is claimed by the harness implementation itself.
