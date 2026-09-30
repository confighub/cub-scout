# Recorded MCP command-access diagnostic — 2026-09-30

This was one paid paired diagnostic of a fixed `cub-scout mcp serve --recording` endpoint, using Claude Code 2.1.274 and pinned `claude-haiku-4-5-20251001` at normal speed (`fast_mode_state: off`). It is **plumbing evidence only**: it does not establish Experiment A, product-quality uplift, cost savings, benchmark readiness, or reduced evidence retrieval.

| Arm | Answer grader | MCP-use indicator | Turns in result | Duration | Inclusive list-price cost |
|---|---:|---:|---:|---:|---:|
| With recorded MCP | 1/1 pass | 1/1 pass (not scored in paired mode) | 5 | 14 s | $0.0826860 |
| Without plugin | 0/1 pass | N/A | 3 | 26 s | $0.0831163 |
| Pair total | 1/2 answer checks passed | 1 MCP call | 8 | 40 s | **$0.1658023** |

The result records `partial: false` and aggregate scores WITH 1.0, W/OUT 0.0, Δ +1.0. Preserve those harness scores, but do not interpret the delta as a quality comparison: the prompt asked for `owner` without clarifying resource ownership versus a field manager. The with-arm reported `Flux`; the no-plugin arm reported `kubectl-set`, which also appears as the selected field manager. That is a prompt-scope ambiguity and likely ownership/manager conflation. The other four requested values matched the frozen contract in both arms: field path, manager list, input hash, and `resourceRead: null`.

Both traces show the same 12 ordinary advertised tools: `Task`, `Glob`, `Grep`, `Read`, `Skill`, `TaskCreate`, `TaskGet`, `TaskList`, `TaskOutput`, `TaskStop`, `TaskUpdate`, and `ToolSearch`. The with trace adds exactly `mcp__plugin_recorded-mcp-probe_cub-scout__explain`. It made two `Read` calls (the complete deployments file and recording metadata), then `ToolSearch` and one exact `explain` call. The no-plugin arm made the same two `Read` calls. Both therefore read the full raw fixture; the tool did not eliminate raw retrieval. No command shell, Kubernetes API, ConfigHub, or live cluster access was exercised.

The case metadata reports `runsPerCase: 3`, while the invocation requested `--runs 1` and the result contains one run per arm. The case configured `maxTurns: 4`, while the with-arm reports 5 turns (without reports 3). These discrepancies are recorded as observed; this packet does not claim the case metadata/run cap was enforced as intended. No further paid run is proposed here.

The recorded pair cost is $0.1658023 inclusive list-price estimate; account credits or incremental subscription billing were not measured. Adding it to existing smoke spend $1.3109446 gives **$1.4767469 smoke-stage**. Adding it to total new paid-eval spend $3.7801511 gives **$3.9459534**. Existing live-only completion spend remains $2.4692065; baseline spend remains $0. The existing $20 smoke, $15 live-only, and $200 baseline envelopes remain the ledger limits. No speed-max mode was used.

## Provenance and limits

- Source checkout used for the pinned binary: `5165f14b1510b75c8d97a9ad4ea526a72788086e` from PR #668, which has since merged as `12b8f419cadc5a0c5364ffba93943de1c862ea08`. The evidence remains tied to the pinned binary and does not claim behavior from later source revisions. The revision is not embedded in Go build metadata. Binary SHA-256: `e94fe84d3e16b6c111ffa68b532523395c6c6f66649be3a890d3033e3afd94c5`.
- Frozen full-list input SHA-256: `305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8` (67,696 bytes, 14 objects); the same case scaffold provided it to both arms.
- Case file manifest SHA-256: `6c1f0409f0e8c368702d7bf6d9aba3d537fcd48e159fb91f2b3640995b0520e6`.
- Raw result SHA-256: `3431ff4b1a57911b0b45217efe9be7b0fe16124b6ea4d02be53b08f1d843c440`. With trace SHA-256: `676978ce03ed1158a5c8e46307fa2d568e9e07013bcf0c7fc4af6dab5141e33c`. Without trace SHA-256: `6af010eec23e6ea68512aa234a2012b8b9cc6036b89595e6d426e94e451daaa4`.
- The server process ran as the user outside Claude's shell-tool sandbox, but its fixed wrapper used the reviewed binary, a hash-pinned recording, an empty owned kubeconfig, and isolated empty `HOME`. Trace inventory and exact call are retained for audit.
- An earlier unbounded ad hoc preflight lost its exec-session identifier; that process state remains unresolved. No global process scan or signal was used. This incident is disclosed rather than treated as cleaned up.

The evidence archive is retained at `evals/results/recorded-mcp-probe-c37a81a201c8c72d/`; per-file hashes, including the grader checks and trace audit, are indexed in `archive-index.json` (SHA-256 `14e278cba90ee38c2ee26885bb6d67fa928d411198165fdf0d94ca10b15d67b4`). No sealed run-home content was read, changed, or archived.
