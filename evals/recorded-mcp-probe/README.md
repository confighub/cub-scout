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
