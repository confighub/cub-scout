# Combined recorded runtime diagnostic

This executable two-arm diagnostic **passed on attempt five** at source
`1b80b8c47fa6c08b97c595ca3a9a0ba53832b0a6` in 9.091 seconds. It remains separate
from the frozen benchmark and paid admission. Both arms completed real Read,
recorded kubectl and explicit Task/Agent refusal controls. Treatment advertised
35 pinned skills and made one recorded map call; its complete 90,892-byte result
was retained and validated before container removal. The model received a
preview, not the complete persisted body. Both containers were verified absent;
shared staged inputs and treatment plugin hashes were unchanged.

The [report](../reports/2026-10-01-combined-runtime.json) retains all five attempts:

| Attempt | Source | Seconds | Outcome |
| --- | --- | ---: | --- |
| 1 | c40bf9f | 7.379 | Failed: nested mountpoint and Bash newline comparison |
| 2 | 4b65288 | 11.130 | Failed: CLI transport, skill listing, persisted map and unknown request |
| 3 | e2dd588 | 6.188 | Failed: correct mounts returned in a different order; baseline passed |
| 4 | 8b75e26 | 8.843 | Failed: persisted-path parser; token-count route identified |
| 5 | 1b80b8c | 9.091 | Passed the bounded diagnostic contract |

All five attempts verified owned-container cleanup. Baseline attempt five had
six provider-endpoint requests (five scripted messages plus startup probe);
treatment had eight (six messages, startup probe and one declined token-count
request). These are fake-provider requests, not model calls or billing evidence.
No real model, external provider or live cluster was contacted. Eighteen offline
tests passed; raw-event and mount-order regressions failed before their fixes.

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
No repeat is needed for the proved source. The reproducible invocation shape is:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/combined-recorded-runtime/run_pair.py \
  --assets ~/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets \
  --context <reviewed-local-context> --out /tmp/scout-combined-runtime-<fresh-id>
```

Do not run this command until the source and exact invocation have been reviewed.
Unknown CLI flag/plugin semantics fail closed and are not asserted by the pure
tests. The five attempts above used real CLI/container execution and local listeners
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
This does not mean the model saw the complete persisted result. Unknown
request paths are retained with a bound and hash but still fail acceptance.
The observed `POST /v1/messages/count_tokens?beta=true` request may receive one
explicit 404 decline per arm after fake-key authentication and bounded JSON
capture. It counts toward the eight-request limit; no token count is fabricated.
This local unavailable-service behavior is not a paid-provider billing claim.

The historical runner still refuses later skill metadata drift. Offline tests
reconstruct its pinned tree using the exact archived AI-agent skill fixture from
`1b80b8c47fa6c08b97c595ca3a9a0ba53832b0a6`, verify the original whole-tree hash,
and separately assert that current ChangeOrder metadata is refused. The runner,
original source pin and historical runtime receipt are unchanged; these tests
do not rerun containers or admit refreshed metadata under old proof.
