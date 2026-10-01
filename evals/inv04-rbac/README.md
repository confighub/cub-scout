# INV-04 namespaced Deployment inventory under partial RBAC

This is a prepared recorded-evidence case for the frozen INV-04 question. It is
not admitted to the benchmark mapping or paid baseline. Both arms receive the
same four case-owned files: three literal Kubernetes API list responses and a
compact request-scope/provenance record. `scaffold.sh` stages those files
without invoking cub-scout or a cluster.

## Captured evidence

A real, serial read-only capture completed on 2026-10-01. The three GETs were
made in order between 01:41:39Z and 01:41:53Z, not as an atomic snapshot:

| Namespace | Request | Result | Fixture | SHA-256 |
|---|---|---|---|---|
| `inv04-readable-populated` | `apps/v1 deployments` | 200, two items | `readable-populated-deployments.json` | `c98c09f2cb1675b58455ae124d0584ecc948394013fbb0b437c39f6b35e4a1b6` |
| `inv04-readable-empty` | `apps/v1 deployments` | 200, zero items | `readable-empty-deployments.json` | `8d70075f02a75588293ec2b45c8b765cf39d8d268366418ce76dce61386e1aaf` |
| `inv04-denied` | `apps/v1 deployments` | 403 Forbidden | `denied-deployments.json` | `e80dc99c4fd3f8325744d2e030dea4ad1e5fe4cf19056d62d52c198f632077d5` |

The populated response contains `inv04-api` and `inv04-worker`, with their UIDs
and object resourceVersions. The empty result is scoped only to that namespace's
Deployment list at capture time. The denied response is not an empty result:
its count and ownership remain unknown. These three requests do not establish a
complete cluster inventory. A `Native` / `no_known_marker` classification in a
separate cub-scout output would mean no supported marker was observed on the
returned object; it would not establish an orphan. Scout answer/output files
are intentionally not included as model evidence.

`fixtures/capture-scope.json` records request paths, HTTP statuses, timestamps,
source revisions and hashes, cleanup, and shared/private kubeconfig integrity
checks. The capture used harness source `faad29c120ab772324bd48caf8c5be12e5bfc20d`
and a cub-scout binary built from `a99d7d4387740fd2c9ccb183f81ea8004c2e7206`
(SHA-256 `9b36a5b571499c9798ebf1440c7f3d68fd10d513f66920339d6f32e375a005d3`).
The temporary cluster cleanup was verified; shared config bytes remained
unchanged. No kubeconfig contents, tokens, credentials, Secret payloads,
literal manifests, or cub-scout stdout answer documents are staged here.

The first two capture attempts are retained as failed provenance, not hidden
passes: `/tmp/scout-inv04-rbac-20261001T0136Z/provenance.json` (SHA-256
`14cb387eec8c9b5ae7f2f86913c921a73aba5fa6fa84b79f8e8138956ce911a1`, kind
creation failed) and `/tmp/scout-inv04-rbac-20261001T0141Z-diagnostic/provenance.json`
(SHA-256 `debd3138352c1a8b21363cddd20a313b19b8678993abda861e54eaaa2dbc2961`,
kind node name exceeded the limit). The latter's creation stderr is retained at
that path but not copied into the case. The final run shortened the generated
node name and passed. These references point to local capture artifacts, not
files required to reproduce the checked-in case.

## Validation

`TestINV04RBACEvidenceAndGrader` verifies the raw response SHA-256 values,
namespace/kind, UIDs, resourceVersions, empty-list and 403 status identity,
request-scope provenance, and byte-identical scaffolding in two workspaces. It
also exercises the strict grader with the harness's JavaScript regex engine,
including wrong counts, false ownership certainty, extra/duplicate fields,
missing fields, and surrounding prose. The case is prepared only; ordinary
harness tool grants and benchmark admission remain pending.

Run the focused offline contract test with the repository's empty kubeconfig:

```sh
KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./test/unit -run 'TestINV04RBACEvidenceAndGrader|TestEvalScaffoldsWriteTheRecordedExport' -count=1
```
