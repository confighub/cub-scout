# Recorded MCP plumbing probe

This packet prepares a local Claude Code plugin around the existing complete
recorded Deployments List. It does not run an eval or invoke a model. Both eval
arms run the same authored scaffold, which verifies and copies the same
67,696-byte fixture (SHA-256
`305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8`). The
server reads only the fixed recording file; its wrapper pins the binary and
recording hashes, gives it an empty owned `HOME` and kubeconfig, and starts it
with an allowlisted minimal environment. This real MCP process runs as the
user outside the model runner's shell sandbox; environment/input restrictions
are not OS-level containment. The no-model preflight uses bounded
`subprocess.run(timeout=...)` for MCP initialize, `tools/list`, the exact
recorded `explain` call, and rejection of an unsupported tool. It also runs
the answer-grader positive/negative cases under Python `re` and JavaScript
`RegExp`. Key order and paired Markdown fences do not affect correctness;
wrong, duplicate or extra fields still fail. Initialize accepts only the two
explicitly supported protocol versions (2024-11-05 and 2025-03-26).

Prepare it from a hash-pinned local binary:

```sh
go build -o ./cub-scout ./cmd/cub-scout
python3 evals/recorded-mcp-probe/prepare.py \
  --binary ./cub-scout \
  --binary-sha256 "$(shasum -a 256 ./cub-scout | awk '{print $1}')" \
  --out /tmp/cub-scout-recorded-mcp-probe
```

Preparation refuses to overwrite an existing output directory. It prints the
prepared plugin path and hashes. The probe's `resourceOwner` is the resource
ownership classification supported by the recorded object's ownership labels;
`fieldManagers` comes separately from `managedFields` for the exact requested
image path. A resource owner label does not identify the manager of that field,
and a field-manager string does not identify a human or prove a literal command.
The input has no trusted capture time and is not current cluster state.

This is a command-access diagnostic only. It instructs both arms to read the full
raw fixture and is outside the fixed benchmark; it establishes no reduced
reading, quality uplift, savings, or benchmark readiness. The saved 2026-09-30 paid diagnostic in PR #669 reported
`maxTurns: 4` in the case but five turns for the with arm; the runner's
max-turn semantics remain unresolved. Before interpreting a future eval result,
inspect both raw run traces: compare the actual ordinary tool inventories,
confirm only the intended MCP tool was added, and check calls, completion,
cost, and source-fixture hashes. The no-model preflight confirms the plugin
server's advertised tools, not the model runner's actual inventory or trace.
Do not open sealed run-home paths merely to complete this preparation packet.

## Economy-purpose preparation (prepared, not run)

`--purpose economy` prepares a separate diagnostic prompt over the same pinned
recording and ordinary tool permissions. It asks for the same JSON answer in
both arms but does not direct either arm to read the raw file or call MCP. It
keeps the answer grader and removes the `tool_used` grader from success scoring;
whether the arm chooses tools or reads data is an outcome to inspect in the raw
trace, not a requirement. The answer distinguishes resource ownership labels
from exact field-manager evidence and makes no person/latest-writer claim.
Preparation records the purpose and hashes for the prompt, case, answer grader,
binary, and fixture.

Prepare a new output directory using the pinned diagnostic binary and its known
hash:

```sh
python3 evals/recorded-mcp-probe/prepare.py \
  --purpose economy \
  --binary /tmp/cub-scout-recorded-explain \
  --binary-sha256 e94fe84d3e16b6c111ffa68b532523395c6c6f66649be3a890d3033e3afd94c5 \
  --out /tmp/cub-scout-recorded-economy-probe
```

This runs only local contract checks and a no-model MCP preflight. It does not
start an eval. If a later review explicitly authorizes the single diagnostic
pair described in issue #603, use one run per arm, serially, normal speed (fast
mode off), with no judge and no automatic retry. The existing tranche ceiling
is $1; the runner checks `--max-cost-usd` before launch, so it is not a strict
in-flight spend stop. For that one pair, the staged command is:

```sh
claude plugin eval /tmp/cub-scout-recorded-economy-probe/plugin \
  --scaffold --case recorded-explain-mcp --runs 1 --concurrency 1 \
  --ablation with-without --mocks off --allow-real-servers \
  --allow-tools mcp__plugin_recorded-mcp-probe_cub-scout__explain \
  --model claude-haiku-4-5-20251001 --max-cost-usd 1 --no-publish \
  --json /tmp/cub-scout-recorded-economy-probe/result.json
```

The `max_turns: 8` setting does not resolve the historical mismatch between
configured max turns and observed turns. Before interpreting any future result,
inspect both traces and actual tool inventories, calls, file reads, completion,
and full spend, including failed attempts. One pair is diagnostic only; it
cannot establish general savings, quality uplift, or benchmark readiness. This
packet does not run the paid command above.

The prepared plugin includes an eval case but this packet runs no Claude Code
eval command and authorizes no spend. Any future paid diagnostic requires a
preplanned budget and the adopted execution plan's preflight and prompt-review gates. Historical
paid result and ledger files remain unchanged.

Run the no-model preparation guard tests with:

```sh
python3 -m unittest discover -s evals/recorded-mcp-probe -p 'test_*.py'
```

These check mismatched binary rejection before output creation, refusal to
overwrite evidence, quoted paths, inherited-environment removal, and rejection
of a changed recording before server execution. Preparation also runs the
14-case answer contract in both regex engines.
