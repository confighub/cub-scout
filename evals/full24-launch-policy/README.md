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

## Separate recorded-response host preflight

`probe_context.py` validates the original RUL-03 treatment stage through the
public source/stage verifier, then prepares a private read-only package with
the two reviewed evidence adapter sources, a fixed bootstrap and exactly four
selected evidence files. Its explicit host probe invokes a hash-selected regular
Python executable with `-I -S`, an owned HOME/empty kubeconfig and a clean
environment. No prompt, oracle, skill, plugin or sibling case is supplied to the
child. The adapter itself has no cluster, network or subprocess client.

The six requests check initialization, the sole read-only `recorded_response`
tool, exact captured denied/readable responses, an unsupported implicit context
and refusal to substitute `map`. Responses must retain their original text,
403/200 status and complete source provenance. Byte/message limits and a
15-second child timeout bound the preflight. Raw requests, stdout, stderr and a
proof are retained even after a failed attempt; package, source and interpreter
are rechecked after the child. Existing outputs, symlinks, input overlaps and
unreviewed adapter sources refuse before execution.

```sh
python3 evals/full24-launch-policy/probe_context.py \
  --source-prep "$SOURCE_PREP" --stage "$RUL03_WITH_STAGE" \
  --python "$REGULAR_ABSOLUTE_HOST_PYTHON" --python-sha256 "$EXPECTED_HOST_PYTHON_SHA256" \
  --output "$NEW_PRIVATE_TEMP_PROOF_DIR"
```

[Host proof](local-context-proof.json) records a passed actual stdio child
preflight on the verified refreshed source. Five pure deterministic controls
cover case/arm/control/evidence selection, exact replies, forged/missing/reordered
results output/interpreter guards and retained failed/timed-out attempts; the launch-policy suite now has 22 tests.
These controls do not invoke a process. The host preflight is explicit opt-in.

The grant boundary remains deliberate: `recorded_response` is an eval evidence
transport, not a product Scout inventory/diagnosis capability. It is not granted
by the frozen launch policy. RUL-03 still blocks; the eleven existing candidates
and thirteen blocked cases are unchanged. Host Python selection is not runtime
dependency admission, and read-only package permissions do not prove filesystem
or network isolation. Complete descendant accounting, model tool enforcement,
official evaluation, paid admission and measured advantage remain unproved.

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

## Static Linux Python candidate

The [candidate receipt](runtime-python-candidate.json) binds a local cached OCI
index to its Linux arm64/v8 manifest, configuration and four layer digests/diff
IDs. A static reconstruction records effective filesystem paths, file hashes,
modes and links, including Python 3.11, libpython and Python library files. The
interpreter is AArch64 ELF; it was not executed. Raw archive, inventory and
one-off audit source remain at `/private/tmp/scout-v213-python-image-20261004`.
The image inspect ID is the multi-platform index digest, not its configuration
digest; both the selected platform manifest and configuration are separately
pinned in the receipt.

Only `docker image inspect` and `docker image save` of an already cached image
were used. No image pull, container, model or provider run occurred. This proves
asset availability and static identity, not interpreter compatibility, imported
dependency closure, isolation, mount enforcement, tool grants or descendant
accounting. Existing `runtimeAssetAdmission` stays blocked; no historical runtime
pins, frozen cases or launch policy were changed. Choosing image-native Python
versus a mounted `/runtime/python3` bundle still requires implementation and
review before final runtime acceptance.

## Repeatable offline Python image verification

A later [actual substrate checkpoint](../python-runtime-substrate/README.md)
executes the separately verified image's Python/stdlib assets and the existing
PyYAML 6.0.3 requirement, including `-I -S`, under inspected owned-container
bounds. All attempts and cleanup outcomes are retained. It does not change this
static verifier or admit the model runtime, overlays, grants or official grader.

`python_image.py` verifies the retained cached-image export against the exact
reviewed receipt digest. It accepts one bounded regular archive, refuses
symlink/archive path ambiguity and metadata duplicates, checks selected OCI
index/platform/config/layer identities and bounded expanded diff IDs, rebuilds
whiteout-applied inventory, and checks Python ELF/library/link identities. It
never extracts archives, starts Docker or executes a target asset. Missing,
changed, unsupported or corrupt inputs refuse with exit 2 and no success output.

```bash
python3 -I -S evals/full24-launch-policy/python_image.py /private/tmp/scout-v213-python-image-20261004/image.tar
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/full24-launch-policy -p test_python_image.py -v
```

The archive is retained local evidence, not shipped into either model arm.
Nine synthetic controls exercise identity/corruption, unsafe and duplicate
paths, JSON nesting, platform/ELF/diff mismatch, compressed/total/member limits,
whiteouts and regular-file/receipt refusal. The independently reviewable
[host proof](runtime-python-verifier-proof.json) includes actual success on the
historical archive and refusal of a truncated copy, under isolated host Python
with no external dependencies. It records modified source state
and source hashes. Public command paths use labels; exact argv is retained in
the private proof, whose digest is included. `runtimeAdmission` and `targetExecuted` are explicitly false;
launch policy, frozen inputs and historical receipts remain unchanged.
