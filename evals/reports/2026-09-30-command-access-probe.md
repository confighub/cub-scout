# Command-access sandbox probe — 2026-09-30

This bounded paired diagnostic confirmed that the pinned Claude Code runner
could invoke the same narrow Bash command in both arms and enforce the two
specific denials tested. It is plumbing evidence only: not Experiment A, not a
full Kubernetes snapshot, not a product-quality comparison, and not evidence
of cost savings or general command/tool parity.

| Arm | Binary probe-output grader | Tool-call indicator | Turns | Seconds | Inclusive list-price estimate | Fast mode / service tier |
|---|---:|---:|---:|---:|---:|---|
| With cub-scout plugin | 1/1 pass | 1/1 pass | 2 | 33 | $0.0424534 | off / standard |
| Without plugin | 1/1 pass | 1/1 pass | 2 | 24 | $0.0230482 | off / standard |
| Total | 2/2 pass | 2/2 pass | 4 | 57 | **$0.0655016** | — |

Both arms used Claude Code 2.1.274 with `claude-haiku-4-5-20251001`, normal
speed (`fast_mode_state: off`, disabled by the environment; trace usage speed
and service tier were `standard`). Each returned the exact same fixture SHA-256
`5604893ddfc9c6cfab3789932400e02f330769bffe2cb241929d310a623f9f3e`, with
`jq=pass`, `python=pass`, `outside_write=blocked`, and
`localhost_connect=blocked`. The saved traces show exactly one actual tool call
per arm: `Bash` with `bash ./probe.sh`. The operator confirmed the loopback
listener remained active during both runs and was stopped afterward; the
owned canary cleanup passed, and no listener remained. The preflight manifest
and source fixture hashes were unchanged.

The only input was the same previously recorded raw `team-02/auth` Deployment
object in each run. It is one object, not the equal full raw-export snapshot
required by Experiment A. The probe made no Kubernetes API request, and it did
not test `kubectl`, Helm, broader file access, or general shell behavior.

A tool-surface limitation is visible in the traces: although the case's
frontmatter declared only `allowed_tools: [Bash]`, both traces advertised the
same inventory: `Task`, `Bash`, `Read`, `Skill`, `TaskCreate`, `TaskGet`,
`TaskList`, `TaskOutput`, `TaskStop`, `TaskUpdate`, and `ToolSearch`. Only Bash
was actually called. Thus the case allowlist did not describe the complete
trace inventory; this run does not demonstrate that other tools were
unavailable.

The harness result and run log say the cub-scout MCP server was withheld and
not started; neither arm exposed an MCP tool or made an MCP call. The with-arm
trace initializer nevertheless records a dynamic `connected` MCP-server
entry. We retain this discrepancy rather than infer from that metadata whether
a real server process existed. No `--allow-real-servers` was used.

The result reports $0.0655016 inclusive list-price cost for this diagnostic;
account credits and incremental subscription billing were not measured. With
this pair, September 30 new paid-eval spend totals $3.7801511: $2.4692065
live-only completion and $1.3109446 smoke, within the respective $15 and $20
envelopes. The $200 baseline campaign remains unspent. No savings or model
enablement claim follows from this probe.

## Reproduction record

The paired run was serial, one run per arm, with
`--ablation with-without --runs 1 --concurrency 1`, the same pinned model,
`CLAUDE_CODE_DISABLE_FAST_MODE=1`, and the exact operator grant
`Bash(bash ./probe.sh)`. The harness used `--mocks record`; no real-server flag
was enabled. The $1 `--max-cost-usd` is a pre-launch ceiling, not a hard cap on
an already launched run.

Raw result, both saved traces, authored case, preflight manifest, original
pre-run plan, run log, and the enriched trace audit are retained locally under
ignored `evals/results/evidence-command-probe-8325744506b8cac0/`. The
`archive-index.json` lists each file hash and the source result/trace hashes.
The temporary probe case is not added to the benchmark suite.
