# Recorded scale input-binding packet

This folder prepares an **offline, not-yet-run** scale evaluation packet. It
does not launch Claude, perform a paid evaluation, or access a cluster. The
historical scale result and `evals/fixtures/scale/recording-2026-09-30.json`
remain unchanged. That source manifest explicitly says the historical MCP arm
was live; this packet does not rewrite that history.

The pinned input comes from source commit
`7f5d38e8881d2f6b22e132b3da57ca60f483e703`. The historical seven-file
manifest SHA-256 is
`e139701bf9d894ca9dd16ddb302fe0e4f28f3122922030cd9cdda57e8eb3fa22`; the
Deployment List used by recorded MCP is
`822716e41eaf59674cec8b52913b2613c76932b578c45d54285f71747dd228b6`.
Preparation verifies the manifest, each listed file and the supplied product
binary hash. Fixed content hashes bind the stated source revision even in a
shallow checkout; Git ancestry is not asserted. The actual preparation commit
is recorded separately. It retains the original manifest in the
output as provenance.

Both model arms receive identical byte-for-byte copies of all seven exports:
ConfigMaps, Deployments, Events, Namespaces, Pods, ReplicaSets, and Services.
The treatment plugin has the repository plugin metadata and skills, while its
MCP server command is replaced in the generated temporary copy with a pinned
wrapper for `mcp serve --recording`. That recorded gateway exposes only
`explain` and `map`; the preflight calls map with exact `apps/v1` Deployments
and the literal `team-` namespace prefix. It accepts no live context or arbitrary
tool arguments. The recorded server process gets a minimal environment with
an explicit packet-owned kubeconfig containing no clusters, users, or
contexts; it does not set or reset HOME. This is process configuration, not
OS-level containment. Ordinary model tools and the two case prompts are copied
unchanged into both arms. This file-tools-only setup is narrower than
Experiment A's ordinary kubectl/Helm baseline and cannot establish full
benchmark comparability.

The copied case `allowed_tools` fields are unchanged, but this offline packet
does not verify the evaluation harness's MCP tool-permission grant. Before any
separate run, review that admission explicitly and keep the ordinary tool
permissions equal across arms; adding recorded MCP availability remains part
of the treatment. Neither parity with Experiment A nor benchmark readiness is
established here.

The staged cases retain their existing answer graders and remove the old
`used-cub-scout-*` graders. Whether an arm chooses MCP, reads files, or does
both is an observation for later trace analysis, not a correctness score.

## Prepare and preflight

Use a fresh output path outside the repository. `prepare.py` only stages files
and runs local deterministic scaffolds; it never invokes the model or starts
the MCP server. Its `--binary` argument is required and its SHA-256 must match.

```sh
python3 evals/recorded-scale/prepare.py \
  --binary /absolute/path/to/cub-scout \
  --binary-sha256 EXPECTED_64_CHARACTER_SHA256 \
  --out /tmp/cub-scout-recorded-scale-prepared
```

After reviewing the generated plugin and `prepared.json`, the separate
no-model preflight requires the same explicit binary and digest:

```sh
python3 evals/recorded-scale/preflight.py \
  /tmp/cub-scout-recorded-scale-prepared \
  --binary /absolute/path/to/cub-scout \
  --binary-sha256 EXPECTED_64_CHARACTER_SHA256
```

Preflight checks the CLI and MCP produce the same map report, verifies the
302 parsed / 300 selected / 2 excluded scope and owner totals, confirms the 12
Native identities, and rejects unsupported live tool/argument requests. It
mutates only the private staged Deployment file after server startup and
checks the in-memory MCP report still reflects the startup snapshot, then
restores the staged bytes. A subprocess timeout terminates only the process
group created by that preflight. No process discovery or shared HOME/config
mutation is used.

The recorded map response is a per-object inventory, not a count-only view.
In the prior offline proof it was 78,427 JSON bytes for 300 selected objects,
compared with a 1,031,673-byte Deployment input. Those are output/input size
observations only: they are not token, time, cost, or savings measurements.
The preparation packet does not show whether an agent will read the files,
call MCP, or avoid redundant work. No model result, grading outcome, or
benchmark admission is claimed here. Existing ordinary-tool permissions,
prompt/model/cache/time controls, the remaining cases, and protocol gates still
need review before any separate run is authorized.

## Offline tests

```sh
python3 -m unittest discover -s evals/recorded-scale -p 'test_*.py' -v
```

Tests use synthetic MCP subprocesses and fixture data; they do not require
Claude, credentials, a network, or a Kubernetes cluster.
