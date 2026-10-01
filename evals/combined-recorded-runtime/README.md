# Combined recorded runtime diagnostic

This executable two-arm diagnostic remains **failed**, separate from the frozen
benchmark, admission audit and historical results. Two actual attempts are
retained in the [report](../reports/2026-10-01-combined-runtime.json): 7.379 seconds
at c40bf9f and 11.130 seconds at 4b65288. Both verified owned-container cleanup;
neither passed the paired receipt contract. The first exposed missing nested
mount staging and Bash newline handling, repaired before the second. The second
exposed CLI/provider event-shape differences, reminder augmentation, skill
advertisement in messages, persisted MCP output, and one rejected unknown
request. These are runtime findings, not successful admission. No real model
or external provider was contacted.

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
Staged tool and plugin copies remain beneath that private output directory for
inspection; container removal does not mean host staging removal. The diagnostic
uses fixed baseline-then-treatment order, not a randomized benchmark.
A further invocation requires a reviewed fix for the recorded failures. The
invocation shape is:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/combined-recorded-runtime/run_pair.py \
  --assets /Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets \
  --context <reviewed-local-context> --out /tmp/scout-combined-runtime-<fresh-id>
```

Do not run this command until the source and exact invocation have been reviewed.
Unknown CLI flag/plugin semantics fail closed and are not asserted by the pure
tests. The two attempts above used real CLI/container execution and local listeners
after source review. No live cluster or real model provider was accessed.

Offline checks use synthetic dictionaries, response fixtures, and temporary
staging only:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/combined-recorded-runtime -v
```

The prospective CLI transport projection is specific to Claude 2.1.274. It
compares tool ID/name/input fields and permits only the observed generated
`total_tokens` reminder suffix, plus removal of the final TAB on an empty
numbered Read line. Raw CLI and provider bytes are both retained; the reminder
counter is not usage or billing. Skill names must occur in the structured
available-skills message listing. A persisted MCP result is read only from its
exact private session/tool-result path, with symlink and 1-MiB size checks, then
retained and independently checked with the existing recorded-map validator.
This does not mean the model saw the complete persisted result. Unexpected
request paths are retained with a bound and hash but still fail acceptance.
