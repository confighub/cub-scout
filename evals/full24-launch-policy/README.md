# Full-24 source-bound launch policy

This internal harness advances the offline prerequisites for the frozen paired
baseline. It does not execute Claude, authorize provider access, or admit a paid
run. Success was defined before implementation in
[#645 comment 5972860099](https://github.com/confighub/cub-scout/issues/645#issuecomment-5972860099).

`policy.py` reconstructs a candidate from a verified selected-case stage and its
source preparation. Both arms retain the exact prompt body, ordinary tool grant
and declared turn/time budget. None of the 24 source grants includes Bash.
DEL-01 and DEL-02 declare no budgets: the candidate explicitly records harness
defaults of 20 turns / 600 seconds in both arms. Other source budgets win.
The two authored controls retain their distinct source inputs and prompts.

Baseline candidates have empty strict MCP configuration and no Scout plugin or
binary. Treatment candidates add only the two recorded Scout tools and the
source-bound plugin overlay. Only ATR-01–04 and INV-01–03 currently have reviewed
bindings, each to its exact `cluster/deployments.yaml` bytes. The other 17 cases
return `blocked_recorded_mcp_binding`, no executable argv and no fabricated tool
response. Partial observations, distinct contexts and sequential frames must
not be merged to construct a convenient replacement dataset.

The candidate model and previously reviewed Linux binary pins describe future
assets. All runtime claims remain false. Independent review identified the
Python interpreter/dependencies and plugin overlay as unresolved runtime assets;
`runtimeAssetAdmission` explicitly blocks execution. It binds the wrapper's
source digest, which a later launcher must validate along with independently
reviewed Python pins and read-only mounts. Candidate argv is not a host command.
`authorize_tool` is a pure allowlist check requiring prior whole-policy source
verification; it proves neither Claude enforcement nor sandbox containment.

`recorded_server.py` accepts only a versioned exact-case binding, fixed runtime
paths, the reviewed Scout binary pin and the selected recording's SHA-256.
It rejects extra/duplicate properties, symlinks, nonregular files, oversized
inputs, arbitrary executable paths and cross-case substitutions. Its only exec
target is `cub-scout mcp serve --recording`; it has no live fallback. The caller
still owes immutable runtime assets and complete process/descendant accounting.

Run the deterministic contracts without models, containers or live clients:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/full24-launch-policy -v
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/ci -p test_offline_workflow.py -v
```

Generate a plan against an existing verified stage (exit 3 means an unsupported
treatment binding; exit 2 means source/stage validation refused):

```sh
python3 evals/full24-launch-policy/policy.py \
  --source-prep "$SOURCE_PREP" --stage "$SELECTED_STAGE" --case ATR-01 --arm with
```

The separate explicit host probe uses a locally built binary and only the seven
immutable source recordings. It sends newline-delimited MCP initialization,
inventory, map, exact-object explain and an unsupported live-tool request;
checks reply IDs, recording hashes, exact identity, object counts, unknown
capture provenance and refusal; and revalidates source and binary afterward.
It retains request/stdout/stderr for every invocation, including timeout output.
The output directory must be new. Reproduce with an already validated preparation:

```sh
GOPROXY=off GOTOOLCHAIN=local go build ./cmd/cub-scout
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-launch-policy/probe_recorded.py \
  --source-prep "$SOURCE_PREP" --binary "$PWD/cub-scout" --output "$NEW_PROOF_DIR"
```

[Local proof](local-recorded-proof.json) records seven passed host stdio probes.
This does not establish the pinned Linux runtime, actual Claude tools/skills,
all-24 MCP coverage, official evaluator integration, descendants, cost or savings.

Manual CI now honors offline selection:

```sh
gh workflow run ci.yaml --ref codex/v213-offline-runtime -f level=unit
```

Unit and proof-artifact jobs run; cluster jobs skip. Unknown/empty manual levels
also fail closed to offline jobs. Normal PR/main CI still includes live jobs,
so this work stays on a topic branch while live tests are excluded. Skipped live
jobs are pending gates, never passed gates.
