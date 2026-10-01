# Prepare the blinded 24-case benchmark evidence

This offline example prepares both arms from the frozen source fixtures. It
runs no model, provider, client, container or cluster. Use an environment with
PyYAML 6.0.3 already available, as pinned by
[`evals/pre01-crd/requirements.txt`](../../evals/pre01-crd/requirements.txt).
From the repository root, choose a new empty output directory:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-pair-preflight/prepare.py --out /tmp/cub-scout-full24-example
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-pair-preflight/prepare.py --out /tmp/cub-scout-full24-example --verify
```

Both commands should report `caseCount: 24` and
`status: prepared_source_only_not_run_not_admitted`. Inspect `preflight.json`
on the host for the source/evidence hashes, six frozen group weights, exact
selected answer graders, and ordinary tool declarations. The scale dataset
has 302 supplied rows, 300 selected and two excluded; this does not establish
original-cluster completeness.

Changing a staged prompt, input or tool grant, adding an answer file, or
editing a treatment skill must make verification fail even if the saved hash
map is edited too. DEL-03/04 include separate authored missing/stale/conflicting
identity inputs, with private acceptance data. Original source fixtures and
historical grading defaults remain unchanged.

Only the selected case/control arm and its allowed treatment delta may be
mounted for a later model run. Never expose the whole output directory: it
contains private oracle material and sibling cases. The staged MCP command is
an inert marker pending a reviewed recorded adapter. Tool enforcement,
descendant accounting, paid admission, quality and savings are not proved by
this preparation. See the [full contract](../../evals/full24-pair-preflight/README.md).
