# RUL-03 explicit-context capture preparation

This packet prepares source and offline proof for one bounded RUL-03 capture.
It has not run a cluster or model and does not alter the frozen benchmark
mapping. The helper requires an explicit `--execute` flag and explicit paths
for the existing shared kubeconfig (hash-only), kind, kubectl, Docker, and a
new output directory. It never uses the shared kubeconfig for cluster calls.

The capture creates two uniquely named kind clusters using the exact kind and
cached node-image pins inherited from `evals/inv04-rbac/capture.py`. Each gets
its own private admin kubeconfig, namespace, observer ServiceAccount, and one
Deployment. Only the readable cluster gets a namespace-scoped Role and
RoleBinding for Deployment `get` and `list`. Admin-side setup verification
checks that the authored Deployment exists on each server before the observer
requests.

One private observer kubeconfig contains two separately bound contexts and
uses the readable context as its current default. The capture resolves each
requested context by name and sends only the allowlisted namespaced Deployment
List GET to that context's loopback TLS server. It does not consult or mutate
`current-context`; the denied read is made first with the explicit denied
context. The two server endpoints and CA identities must differ. The raw HTTP
response bodies, HTTP status, timestamps, content type, elapsed time, and
SHA-256 are retained. A typed Kubernetes `Status/Forbidden` is denied evidence;
its message and details must identify the ServiceAccount's `list deployments`
denial in namespace `rul03-proof`. It is never converted into an empty list. A
successful list must contain the one exact authored Deployment and its
UID/resourceVersion.

The helper uses bounded process groups, a 240-second overall deadline plus a
separate 90-second cleanup window, 45-second default command deadlines (90
seconds for each bounded kind create), a 2 MiB process-output cap, and a 4 MiB
API-body cap. Output directories are new,
outside the repository, and mode `0700`; files are created exclusively with
mode `0600`. It records partial command evidence, including interrupted
creation attempts. Cleanup checks ownership markers and deletes only the two
generated cluster names, then verifies both are absent. Shared kubeconfig
hashes and private-config hashes are recorded; no credentials or config bytes
are written to capture output.

The offline tests exercise context-to-endpoint binding despite a readable
default, typed `403` versus a real nonempty `DeploymentList`, malformed bodies,
request/output limits, explicit command construction, safe output creation,
and cleanup after a simulated interruption during partial second-cluster
creation. They make no cluster, network, CLI, or model calls. The independent
actual capture and later fixture packaging remain separate review steps. Even
if captured successfully, this evidence supports only two explicit
namespace-scoped observations; it does not support cross-cluster aggregation,
ownership, workload health, or savings claims.

Run the offline test suite with bytecode disabled:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/rul03-context -v
```
