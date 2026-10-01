# INV-04 RBAC capture preparation

This is a capture harness, not a recorded fixture or benchmark case. It has not
been run against a cluster. A reviewer must inspect it before any execution.

The harness requires an explicit `--execute`, a caller-supplied shared
kubeconfig (hashed before/after but never used for API requests), a pinned
`cub-scout` binary, and a new output directory outside this checkout. The
source checkout must be clean, and binary hash/build commit are pinned. It
refuses an existing output directory, a non-local Docker endpoint, a set
`DOCKER_HOST`, an existing generated kind-cluster name, an uncached node image,
or an unexpected kind version. It verifies the local node image repository
digest, creates only the unique cluster name it records, and deletes only that
name. A mode-0600 owned-cluster marker is written before creation so a
hard-killed run can be identified for manual cleanup. The script handles
INT/TERM and enforces a 12-minute capture deadline, then suppresses further
signals during bounded cleanup. The marker records only the exact cluster name
and private temp-directory basename; it contains no kubeconfig or token. If a
hard kill leaves these behind, inspect that marker and clean only its named
cluster and exact private temp directory. Failed captures retain response files and
`provenance.json` when cluster execution has started.

All observer requests use a separate, short-lived ServiceAccount token and
minimal private kubeconfig. The admin kubeconfig remains in a private
mode-0700 temp directory; output from `kubectl config view --raw` remains only
in process memory. Neither is written to the output directory or logs. Output contains no
kubeconfig contents, bearer token, client key/certificate, or Secret payload.
The three raw API bodies are literal bounded GET responses: populated 200,
empty-but-readable 200, and denied 403 Status. Scout CLI JSON/stderr is stored
separately. Capture order is sequential and not atomic; only the named
namespace Deployment list requests are in scope.

After review, the intended invocation is:

```sh
KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  PYTHONDONTWRITEBYTECODE=1 python3 evals/inv04-rbac/capture.py \
    --execute \
    --shared-kubeconfig /absolute/path/to/operator-kubeconfig \
    --cub-scout-binary /tmp/cub-scout-inv04-capture-a99d7d4 \
    --output-dir /tmp/scout-inv04-rbac-20261001T120000Z-unique
```

The `KUBECONFIG` prefix is only for deterministic local test hygiene; the
capture commands pass explicit private kubeconfig paths and do not use that
environment value for cluster access. Do not run this command as part of
ordinary tests. The operator-supplied config is read only to hash it and must
be a real regular file, not a symlink. The capture should only be run in the
serial live lane after explicit lead review.

Offline safety tests:

```sh
KUBECONFIG=/tmp/scout-offline-validation.kubeconfig PYTHONDONTWRITEBYTECODE=1 \
  python3 -m unittest discover -s evals/inv04-rbac -p 'test_*.py' -v
```
