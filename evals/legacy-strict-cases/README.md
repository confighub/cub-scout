# Prospective strict answers for five legacy cases

This offline packet prepares an opt-in answer-only variant for INV-03 and
ATR-01 through ATR-04. It leaves the historical case folders, frozen benchmark
questions/references/group weights, current graders, and previous results
unchanged. Ordinary evals continue to use the historical prompt and grader.

The strict mode is selected only when preparing a separate case directory:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/legacy-strict-cases/prepare.py \
  --contract legacy-answer-line.v1 \
  --out /tmp/scout-legacy-strict-packet
PYTHONDONTWRITEBYTECODE=1 python3 evals/legacy-strict-cases/prepare.py \
  --contract legacy-answer-line.v1 --verify \
  --out /tmp/scout-legacy-strict-packet
```

The packet contains matching `arms/without/evals/legacy-strict/` and
`arms/with/evals/legacy-strict/` copies. For each case, only the generated
prompt and answer grader differ from its pinned source; `case.yaml`, tool-use
indicator graders, and `scaffold.sh` remain byte-identical. Both staged arms
are byte-identical. The raw scaffold is the same complete recorded Kubernetes
export described in [`../README.md`](../README.md); no scaffold is executed
during preparation. `prepared.json` records the frozen manifest digest, source
and generated file hashes, case references, and arm equality. Verification
rejects changed source inputs, changed/missing/extra staged files, and
symlinks.

The generated answer schemas distinguish an API field manager from a human
actor and literal argv, use `UNKNOWN` where the recording does not identify a
person or command, and state recorded-evidence-only scope. The Argo answer
schema requires the configured annotation tracking method and the conflicting
instance label to be explained together. The expected answers live only in
the grader definitions; the generated prompts give a format and evidence
boundary, not the answer values.

The generator writes one correctness `regex` grader per case, anchored to the
entire `last_message`, with no multiline or case-insensitive flag. It also
retains each original `tool_used` grader as a non-correctness indicator. The
grader frontmatter follows the repo's supported schema (`type`, `pattern`,
`flags`, `target`); no weight field is invented. `evals/scripts/report.py`
uses default weight 1 for a missing embedded regex weight and ignores
`tool_used` when computing binary verified answers. A future run must still
retain and inspect its embedded grader definitions/results and require the one
positive-weight correctness grader to appear exactly once and pass.

The source control `owner-unlabelled` remains pinned and outside the generated
five-case set. ATR-03 `changed-by-payments` is both a strict case and the
existing no-manual-edit attribution control. The Argo tracking-id/instance
label conflict remains the case's built-in trap. No historical fixture,
answer, or grader is replaced in place.

This is grading preparation only. It does not prove a model can answer, run an
equal-arm harness comparison, establish cost or savings, or admit the frozen
benchmark. No model, eval CLI, cluster, provider, or runtime call is made.
Follow-up issue: [#731](https://github.com/confighub/cub-scout/issues/731),
under [#603](https://github.com/confighub/cub-scout/issues/603) and
[#645](https://github.com/confighub/cub-scout/issues/645).
