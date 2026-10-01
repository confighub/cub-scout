# PRE-01 recorded API through real kubectl

The bounded no-model plumbing gate for issue [#732](https://github.com/confighub/cub-scout/issues/732) passed once on source revision `75ef04b902747e652c5bc9144ce939bcb34c7ad6`. It staged the reviewed Linux arm64 kubectl, source-pinned PRE-01 replay, and exact report/scope/response bytes into one invocation-owned container. In that network-none container, it ran three exact `kubectl get --raw` paths against a fresh loopback replay for the `absent` phase, then repeated the same paths against a separately started `present` replay. It did not contact a Kubernetes cluster or invoke Helm, cub-scout, Claude, or any provider. The wrapper exited 0 in 3.503 seconds; the gate itself reported 0.777 seconds.

The acceptance contract is six commands in a fixed order. For each request, the replay trace must contain exactly one GET for the expected raw path with the source response status and body hash. The three captured 404s must yield kubectl exit code 1 and nonempty captured stderr; the three captured 200s must yield exit code 0 and stdout byte-for-byte equal to the pinned response. A missing request, duplicate/extra/unavailable route, wrong phase, timeout, altered input, malformed trace, or uncertain cleanup fails the gate. Kubectl's 404 stderr is retained as the CLI result and is not expected to equal the API body; the replay separately retains the body/status evidence.

Each phase uses an anonymous kubeconfig generated under its private container `/tmp`, an empty private HOME, and only explicit loopback HTTP to the owned replay listener. The only host-side Docker connection uses the named local Unix-socket context. No credentials, kubeconfig, proxy environment, Docker socket, or home directory is mounted into the container. The `/tools` stage is read-only; the container is UID/GID 65534, network-none, read-only-root, no-new-privileges, capabilities dropped, 64 PIDs, 1 GiB memory, one CPU and a 64 MiB private `/tmp`. These are requested and inspected settings, not resource-exhaustion tests.

The staged set is fixed: the pinned kubectl, authored payload, pinned replay source, PRE-01 report, capture scope, and only response files selected by the two declared phases. The report and all response hashes were checked before staging; the staged tree and source bytes were checked again before cleanup. The owned container's requested security/resource settings were inspected, its exit code was zero, the owned container's absence was verified, and staging cleanup was verified. Replay inputs preserve the original sequential, non-atomic capture limits. The replay server's general `coverageComplete` field remains false; this gate applies its own exact three-request acceptance check to each per-phase trace.

The accepted run's receipt, wrapper result, gate result, per-phase traces, and twelve raw stdout/stderr files (two for each command) are archived under `evals/results/pre01-real-kubectl-20261001/` in the primary checkout. The machine report at [`2026-10-01-pre01-real-kubectl.json`](../reports/2026-10-01-pre01-real-kubectl.json) records source and evidence hashes, exact routes/statuses, command outcomes, and the scope limits. Raw inspect bodies and host paths are not copied into that report.

## Offline source tests

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/pre01-real-kubectl -v
```

Tests use fake executable results and/or the reviewed local loopback replay implementation. They do not run Docker or the pinned kubectl asset.

## Reproduction

The helper requires `--execute`, the exact pinned asset, the cached image ID from the reviewed Linux runtime gate, an explicit local Docker context, and a new output directory under `/tmp` or `/var/tmp`. The accepted command shape was:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/pre01-real-kubectl/prepare.py \
  --execute \
  --assets <reviewed-linux-runtime-assets>/assets \
  --docker-binary <local-docker-launcher> \
  --docker-context <validated-local-unix-socket-context> \
  --image-id sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5 \
  --output-dir <new-directory-under-tmp>
```

This is a narrow transport proof: it does not establish broad Kubernetes discovery compatibility, Helm API coverage, ordinary-tool parity, live cluster state, model quality, cost savings, or paid benchmark admission. General discovery and additional routes remain untested.
