# Full-24 source-bound launch policy

This internal harness advances the offline prerequisites for the frozen paired
baseline. It does not execute Claude, authorize provider access, or admit a paid
run. Success was defined before implementation in
[#645 comment 5972860099](https://github.com/confighub/cub-scout/issues/645#issuecomment-5972860099).

The four-snapshot follow-up was scoped before code in
[#645 comment 5973024674](https://github.com/confighub/cub-scout/issues/645#issuecomment-5973024674).

The terminal-adapter follow-up was scoped before code in
[#645 comment 5978432564](https://github.com/confighub/cub-scout/issues/645#issuecomment-5978432564).

The prepared dispatch guard was scoped before code in
[#645 comment 5978652786](https://github.com/confighub/cub-scout/issues/645#issuecomment-5978652786).

`policy.py` reconstructs a candidate from a verified selected-case stage and its
source preparation. Both arms retain the exact prompt body, ordinary tool grant
and declared turn/time budget. None of the 24 source grants includes Bash.
DEL-01 and DEL-02 declare no budgets: the candidate explicitly records harness
defaults of 20 turns / 600 seconds in both arms. Other source budgets win.
The two authored controls retain their distinct source inputs and prompts.

Baseline candidates have empty strict MCP configuration and no Scout plugin or
binary. Treatment candidates add only the two recorded Scout tools and the
source-bound plugin overlay. ATR-01–04 and INV-01–03 bind their exact `cluster/deployments.yaml` bytes.
HLT-02 binds `cluster/deployment.json`; PRE-02 and RUL-01 independently bind
`cluster/after-pod.json`; RUL-04 binds `cluster/statefulset.json`. These eleven
recordings have explicit object identities. The other 13 cases
return `blocked_recorded_mcp_binding`, no executable argv and no fabricated tool
response. Partial observations, distinct contexts and sequential frames must
not be merged to construct a convenient replacement dataset.
The [remaining binding contracts](recorded-binding-gaps.md) name each source
shape and the identity/scope work needed; this inventory does not unblock them.

The [source refresh proof](source-refresh-proof.json) separates the archived
preparation used by historical container/stdio proofs from the fresh preparation
after the ChangeOrder tool documentation update. Only two treatment skill
metadata files changed. Frozen cases, inputs, prompts, grants, budgets, graders
and authored controls remain identical. The fresh source passed preparation
validation and an independent tree/hash review; this does not rerun historical
runtime proofs or establish model behavior at the refreshed metadata.

`overlay.py` materializes and verifies candidate metadata into a new read-only
directory. Its Python API is `build(source, stage, case, arm, output)` followed
by `verify(source, stage, case, arm, output)` (paths are `pathlib.Path` values).
Both arms include their policy, strict MCP configuration and prepared dispatch
guard/settings/binding. The baseline has no Scout binary, plugin or MCP tools. The
treatment copies exact selected-stage skills, a manifest with ambient MCP
configuration removed, and the pinned wrapper source and case binding. Source,
stage and overlay cannot overlap; extra files, changed bytes and writable
entries fail verification. It copies no oracle, sibling case or raw recording.
This closes offline overlay preparation only: execution assets and immutable
runtime mounts still require admission.

`evaluate.py` audits a captured terminal JSONL stream against the selected
source grader using an existing local Node executable. Exactly one final
`result` record is required; intermediate assistant messages are never graded.
Errors and budget exits fail, malformed or ambiguous streams remain unknown.
Authored controls use their separate ordered-JSON acceptance contract with
weight zero. Reported cost/usage is retained as unreconciled metadata.

The caller must supply a `full24-attempt-binding.v1` JSON object with exactly
`schema`, `selection` (the launch policy selection),
`sourcePreparationReportSha256`, `stageReceiptSha256` and `traceSha256`.
Hashes bind the verified preparation, exact stage receipt and captured trace.
A future trusted supervisor must establish the binding's provenance; this
adapter does not prove that a particular runtime produced the trace.

```sh
python3 evals/full24-launch-policy/evaluate.py \
  --source-prep "$SOURCE_PREP" --stage "$SELECTED_STAGE" \
  --case ATR-01 --arm with --trace "$TRACE_JSONL" \
  --attempt-binding "$ATTEMPT_BINDING" --node "$LOCAL_NODE"
```

All 24 selected regexes have positive/negative local JavaScript vectors and
terminal-selection controls. These checks establish neither official evaluator
execution, paid-run admission, descendant completion nor billing attribution.

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

The separate explicit host probe uses a locally built binary and only the eleven
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

[Local proof](local-recorded-proof.json) records eleven passed host stdio probes.
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

`dispatch_guard.py` implements the documented
[PreToolUse JSON decision contract](https://code.claude.com/docs/en/hooks).
Both arms prepare an all-tools hook against the same exact selected ordinary
grant. Only the treatment can additionally dispatch the exact recorded map and
explain names. Unknown, write, shell and delegation tools are denied; malformed,
duplicate, oversized or incomplete event documents are refused. Case, arm,
control selection and policy digest must match. Refusals never echo tool inputs.
Permitted calls return no hook permission override and continue through ordinary
CLI permissions. This checks tool names only; it does not prove file access,
tool-argument validity, provider scope or skill behavior.

The overlay source-binds and verifies the guard and settings in both arms.
Python is an unresolved runtime asset in both arms. The host must verify the
whole policy before immutable mounting: mutually matching runtime hashes alone
are not independent source attestation. The pinned Claude hook invocation and
hook error/timeout behavior still need adversarial runtime acceptance. A hook
that fails to launch is not proven to block dispatch; these offline tests do
not admit a paid run or claim actual enforcement.
