# PRE-01 recorded API through real kubectl

This is a source-only, no-model plumbing gate prepared for issue [#732](https://github.com/confighub/cub-scout/issues/732). It stages the reviewed Linux arm64 kubectl, the source-pinned PRE-01 replay and its exact report/scope/response bytes into one invocation-owned container. In that network-none container, it runs three exact `kubectl get --raw` paths against a fresh loopback replay for the `absent` phase, then repeats the same three paths against a separately started `present` replay. It does not contact a Kubernetes cluster, invoke Helm, cub-scout, Claude, or any provider.

The acceptance contract is six commands in a fixed order. For each request, the replay trace must contain exactly one GET for the expected raw path with the source response status and body hash. The three captured 404s must yield nonzero kubectl exits; the three captured 200s must yield exit zero and stdout byte-for-byte equal to the pinned response. A missing request, duplicate/extra/unavailable route, wrong phase, timeout, altered input, malformed trace, or uncertain cleanup fails the gate. Kubectl's 404 stderr is retained as the CLI result and is not expected to equal the API body; the replay separately retains the body/status evidence.

Each phase uses an anonymous kubeconfig generated under its private container `/tmp`, an empty private HOME, and only explicit loopback HTTP to the owned replay listener. The only host-side Docker connection uses the named local Unix-socket context. No credentials, kubeconfig, proxy environment, Docker socket, or home directory is mounted into the container. The `/tools` stage is read-only; the container is UID/GID 65534, network-none, read-only-root, no-new-privileges, capabilities dropped, 64 PIDs, 1 GiB memory, one CPU and a 64 MiB private `/tmp`. These are requested and inspected settings, not resource-exhaustion tests.

The staged set is fixed: the pinned kubectl, authored payload, pinned replay source, PRE-01 report, capture scope, and only response files selected by the two declared phases. The report and all response hashes are checked before staging; the staged tree and source bytes are checked again before cleanup. Replay inputs preserve the original sequential, non-atomic capture limits. The replay server's general `coverageComplete` field remains false; this gate applies its own exact six-request acceptance check to the returned per-phase trace.

## Offline source tests

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/pre01-real-kubectl -v
```

Tests use fake executable results and/or the reviewed local loopback replay implementation. They do not run Docker or the pinned kubectl asset.

## Separate invocation

The helper requires `--execute`, an exact pinned asset path, the cached image ID from the reviewed Linux runtime gate, an explicit local Docker context and a new output directory under `/tmp` or `/var/tmp`. It is not to be invoked until independent source review. A successful result establishes only that the pinned kubectl consumed these six exact recorded responses through this transport. It does not establish broad Kubernetes discovery compatibility, Helm API coverage, ordinary-tool parity, live cluster state, model quality, cost savings, or paid benchmark admission.

After independent review, the bounded invocation shape is:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/pre01-real-kubectl/prepare.py \
  --execute \
  --assets /Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets \
  --docker-binary /usr/local/bin/docker \
  --docker-context orbstack \
  --image-id sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5 \
  --output-dir /tmp/scout-pre01-real-kubectl-20261001-r1
```

This packet has not run that command. Its prepared-only status and exact pins are recorded in [`2026-10-01-pre01-real-kubectl.json`](../reports/2026-10-01-pre01-real-kubectl.json).
