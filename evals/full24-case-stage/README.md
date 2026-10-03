# Full24 single-case source stage

## Success contract

Given a source directory that passes `evals/full24-pair-preflight/prepare.py`
validation, one frozen case, one arm (`without` or `with`), and optionally that
case's exact declared authored-input control ID, this packet must:

- create a fresh private output directory containing a read-only `model-stage/`
  and a separate mode-600 host-side `receipt.json`;
- copy the selected case's exact prepared prompt and ordinary-tool grant plus
  its source evidence; the answer contract is recorded in the host receipt and
  remains encoded in the prepared prompt. For the `with` arm, also copy only the
  treatment files declared by the validated preflight;
- when a DEL-03 or DEL-04 control is selected, replace the case prompt/evidence
  with that control's arm inputs, retain only the case's ordinary-tool grant,
  and never include the original case evidence or the control's oracle;
- bind the source report and every staged file by SHA-256, verify the exact
  model-stage file/directory set, and reject symlinks, unsafe path overlap,
  stale source bindings, malformed receipt shapes, special filesystem entries,
  and output drift;
- keep the prepared MCP marker inert and report `not_admitted_not_run`.

The offline tests must cover all 24 ordinary cases in both arms, common-file
byte parity and the exact treatment delta, both authored controls in both arms,
and negative cases for source corruption, invalid selection, path/symlink
hazards, output drift, oracle leakage, and sibling-case injection.

## Use

First prepare and validate the source package using its documented commands in
`evals/full24-pair-preflight/README.md`. Then stage exactly one case and arm:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-case-stage/stage.py \
  --source-prep /private/tmp/cub-scout-full24-preflight \
  --case DEL-03 --arm with \
  --out /private/tmp/cub-scout-stage-del03-with
```

To select the authored DEL-03 control, add its exact ID:

```sh
  --control del03-missing-stale-identity-input-control.v1
```

Use `--control del04-divergent-identity-mutable-tag-input-control.v1` for the
DEL-04 control. Controls are optional, case-specific inputs, not new weighted
questions. The destination's parent must already exist, and the output path
must be new. The stage rejects symlinks in source/destination path components.
On macOS, use the resolved `/private/tmp/...` path instead of the `/tmp` alias.
Verify a completed stage with:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-case-stage/stage.py \
  --source-prep /private/tmp/cub-scout-full24-preflight \
  --case DEL-03 --arm with \
  --out /private/tmp/cub-scout-stage-del03-with --verify
```

The stage is only a future mount candidate. Any later runtime must mount only
`model-stage/`, never the output root, source preparation tree, host receipt,
or sibling stage. This packet does not implement or verify that runtime.

Run the bounded source-only tests with:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-case-stage/test_stage.py
```

## Boundaries

This is source staging only. It does not execute a container, process, client,
ordinary tool, MCP server, skill, model, provider, or grader. The ordinary-tool
grant is recorded but not enforced; the sandbox and treatment MCP are not
verified usable. The receipt makes no answer-quality, descendant/process
accounting, token, billing, credit, cost-saving, or benchmark-admission claim.
