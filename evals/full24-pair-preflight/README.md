# Frozen full-24 source preparation

This source-only packet prepares the frozen `benchmark-v1` cases as a blinded,
byte-equal pair and validates every case binding before any evaluation. It does
not start Claude, a model/provider, Docker, Kubernetes, Helm, an MCP server, a
client query, or a grader. It does not make benchmark admission or quality,
cost, savings, credits, or billing claims.

Run from the repository root using a Python environment with PyYAML 6.0.3
(the existing `evals/pre01-crd/requirements.txt` pin). CI reuses its PRE-01
virtual environment; the local proof used the already available system Python:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-pair-preflight/prepare.py --out /tmp/cub-scout-full24-preflight
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-pair-preflight/prepare.py --out /tmp/cub-scout-full24-preflight --verify
```

`arms/without/cases/` and `arms/with/cases/` contain the same frozen question,
selected prospective strict answer contract, per-case ordinary-tool grant, and
source evidence. The package keeps all 24 case references, source graders,
selected positive-weight answer graders, case YAML, and control acceptance data
under `oracle/` or `authored-input-controls/*/oracle/`, outside either model
arm. Tool-use graders are listed separately as instrumentation and do not
substitute for a positive-weight answer grader. `preflight.json` binds each
source prompt, case metadata, scaffold, grader, evidence file, frozen question,
reference, control, ordinary tool grant, and answer contract by hash.

The source-scaffold reader does not execute shell. It accepts a bounded static
quoted-heredoc format for DEL-01/02 and explicit fixture-to-output mappings for
the reviewed copy scaffolds; giant recorded inventory scaffolds use the pinned
scale/legacy preparation sources. Any unmapped/ambiguous fixture, altered
question/control/weight, duplicate or missing case, oracle leakage, byte drift,
or malformed YAML fails closed. INV-01/02 reuses the recorded-scale source
verifier and report validator after checking the raw Deployment list contains
302 parseable unique rows, 300 `apps/v1` Deployments in `team-*`, and 2 outside
that namespace scope. This establishes completeness only for those pinned input
files; original-cluster completeness remains unknown.

DEL-03 and DEL-04 have separate, unweighted authored input-control packages.
DEL-03 redacts the identity-bearing `spec.variants` subtree and status-row
identity, then supplies an explicitly synthetic stale interval. DEL-04 retains
the original chain beside an authored copy that changes the delivery digest
under the same mutable `:latest` reference to a different already-recorded
source digest. Both controls preserve original source hashes, are staged
byte-identically across arms, keep their acceptance rules under `oracle/`, and
are marked not run. They are negative-input controls, not new captures or
weighted questions.

Treatment stages the repository's pinned skill files and sanitized plugin
metadata. The live-default `cub-scout` MCP executable is replaced with an
inert `__RECORDED_MCP_BINDING_PENDING__` marker. A later reviewed runtime must
provide a recorded adapter; this package does not claim that skills were loaded
or MCP was usable. Each case's declared ordinary tools are preserved equally,
but enforcement and descendant/process accounting remain separate gates.

The upstream command `claude plugin eval . --scaffold` is recorded as a future
reproduction command only; it invokes an evaluation and was not run here. The
binary-correctness/cost and trace-admission interfaces remain
`evals/scripts/report.py` and `evals/scripts/admission_audit.py`; this packet
does not fabricate their run results. Frozen defaults and historical reports
are unchanged.

A future runtime must mount only the selected case or authored-control arm,
plus its allowed treatment delta. Never mount the entire preparation root or
other cases: sibling originals, private references and control acceptance data
are host-side preparation material, not extra model evidence.
