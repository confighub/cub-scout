# Combined recorded runtime diagnostic

This source packet provides an executable, two-arm diagnostic runner and a
strict receipt validator. It is deliberately separate from the frozen
benchmark, admission audit, and historical results. The runner has **not** been
invoked; the CLI arguments, plugin loading, tool schedule, and cleanup behavior
remain unverified at runtime. The next invocation requires independent source
review.

`run_pair.py` creates two fresh containers from the pinned Linux asset archive.
Both arms receive the same seven recorded scale files and PRE-01 replay packet.
Baseline physically omits the Scout binary, plugin, skills, and MCP wrapper.
Treatment stages the pinned binary, 35 hash-pinned skill files, and one local
recorded MCP map/explain server. Both arms get Read/Bash and fixed read-only
kubectl/Helm probes; Task and Agent are explicit refusal controls. Treatment
adds the Skill tool and recorded MCP. This is a wiring diagnostic, not the full
24-grant experiment or a proof of skill value. The runner records skill names
advertised in the provider request, which does not establish skill use or
quality.

The Anthropic-compatible endpoint is a loopback synthetic fixture with a
literal fake key; it never contacts an actual model or provider. PRE-01 API
traffic is replayed from the pinned local recording. Each arm runs in a fresh
`network=none` container as UID 65534 with read-only root/tools, dropped
capabilities, no-new-privileges, 64-PID/1-GiB/1-CPU bounds, and a 64-MiB tmpfs.
Limits are eight provider requests per arm, 1 MiB per request, 8 MiB combined
CLI stdout/stderr, 120 seconds execution and 30 seconds cleanup per arm, and
300 seconds for the pair. The container is removed and checked absent; this is
not a claim of complete OS process-tree accounting. Billing and credits are
unknown and unmeasured.

The host runner writes mode-600 receipts, tool output, raw synthetic provider
requests/responses, and Docker operation logs under a new directory beneath
`/tmp`; its read-only staging directories and treatment plugin are retained
there as source-hash evidence. Failures remain failed diagnostics; there is no paid retry. It requires
the separately prepared pinned asset archive and a named local Docker context.
After source review, the invocation shape is:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/combined-recorded-runtime/run_pair.py \
  --assets /Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets \
  --context <reviewed-local-context> --out /tmp/scout-combined-runtime-<fresh-id>
```

Do not run this command until the source and exact invocation have been reviewed.
Unknown CLI flag/plugin semantics fail closed and are not asserted by the pure
tests. No CLI, container, Docker command, provider, network listener, or cluster
was run while implementing this packet.

Offline checks use synthetic dictionaries, response fixtures, and temporary
staging only:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/combined-recorded-runtime -v
```
