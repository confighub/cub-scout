# Offline container-isolation preparation

The first bounded preflight ran on October 1 at source `47e15c6`. Its payload
passed all seven assertions and exited 0; both configuration inspections passed.
The helper nevertheless returned 1 because cleanup's exact missing-container
parser rejected Docker's single stdout newline. The original failed receipt is
preserved. A separate read-only inspection of the same owned full container ID
returned the exact missing-container error with byte hashes matching the original
cleanup response, confirming absence. The parser repair accepts only empty stdout
or that single formatting newline; regression tests fail before and pass after
the repair. No container rerun was needed or performed.

See the [retained result report](../reports/2026-10-01-container-isolation.json).
This is not equal ordinary-tool access, runtime model accounting, provider
billing/credits, answer quality, or paid-evaluation admission. The private raw
archive is `evals/results/container-isolation-20261001/`; the report excludes
private marker paths and full inspect data.

`prepare.py` requires `--execute`, an explicit Docker launcher, an explicit
Docker context, the exact cached image ID
`sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5`,
and a new output directory under a temporary root. The informational
`python:3.11-slim` tag is never used as the image selector. The launcher path is
kept as argv[0] while its resolved target is validated, preserving Docker
launcher dispatch behavior.

The helper creates one new uniquely labeled container and does not reuse
existing resources. It requests `--pull=never`, `--network=none`, read-only
rootfs, UID/GID 65534, all capabilities dropped, no-new-privileges, pids 32,
256 MiB memory, 0.5 CPU, a bounded `/tmp` tmpfs, and one read-only authored
fixture mount. Only the selected local Unix-socket Docker context is accepted.
The host Docker socket, home, kubeconfig, and credentials are never mounted;
context/TLS overrides and Kubernetes credentials are removed from the Docker
client environment. The helper does not prune images, change contexts, or scan
or signal unrelated processes.

The in-container payload checks byte-exact fixture reading, denied fixture
mutation, denied writes outside `/tmp`, successful `/tmp` writing, absence of a
synthetic unmounted host marker, a loopback-only HTTP exchange, and a denied
non-loopback socket attempt from one explicitly spawned child. It reads no
real host-sensitive paths and makes no successful remote request. It never
claims process creation is prohibited.

The marker assertion uses the exact canonical host marker file created inside
the private output directory. The helper verifies that file exists and is
unchanged before and after the payload, passes only its path string into the
inline payload, and confirms the path is not mounted. Since the container has
its own private `/tmp`, the in-container check demonstrates that the host file
is unavailable at that path without probing other host paths.

The helper caps stdout and stderr independently at 1 MiB, bounds payload
execution to 30 seconds, cleanup to 15 seconds, and overall lifetime to 60
seconds. It inspects the exact newly created container and checks its owner
label, image, mounts and requested settings before and after execution, then
removes only that exact owned container ID and verifies absence. If ownership
or cleanup cannot be established, it returns failure and does not claim
cleanup success. Container lifecycle limits descendant cleanup to that owned
container; it does not prove descendants were never spawned.

The receipt distinguishes **configured and inspect-verified** settings from
**demonstrated** payload assertions. CPU, memory, and pids bounds are not
stress-tested; passing the payload does not demonstrate saturation behavior.
The receipt hashes the helper, injected payload, Docker executable (including
requested launcher and resolved target paths), reused INV04 runner, image ID
and authored fixture, records
operation status/timing and bounded-output hashes, and checks that the host
fixture and synthetic marker stayed unchanged. Raw full Docker-inspect output
is validated in memory and not written to the receipt or output files.
Container ownership (exact generated name, ID, and owner label) is checked
separately from configuration validation, so a container created by this run
can still be removed if one of its settings fails inspection. Missing-container
recognition requires one exact Docker error line for the exact target with no
stdout (apart from one observed formatting newline) or contradictory output. A receipt write failure fails the capture.

Pure tests mock all process calls and never execute Docker or the payload:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/container-isolation -v
```

Further runtime integration requires a separately reviewed packet; this result
does not establish full tool parity or model/provider evaluation evidence.
