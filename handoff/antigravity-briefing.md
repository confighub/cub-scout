# Briefing: take cub-scout from v2.19.0 to 3.0, and plan 3.x

You are taking over the next stages of **cub-scout**, an open-source,
read-only GitOps observer for agents. Until now it was built by the
maintainer with Claude Code and, earlier, Codex. This briefing is all
the context you get, so read it to the end before acting, then read the files
it names. State on 2026-11-24.

---

## 1. Where everything is

| What | Where |
|---|---|
| Repository (local) | `~/code/cub-scout` — work here, on a branch per change |
| Repository (GitHub) | https://github.com/confighub/cub-scout (public), default branch `main`, squash merges |
| Go module | `github.com/confighub/cub-scout/v2`; Go 1.26.0, toolchain go1.26.9 |
| Releases | https://github.com/confighub/cub-scout/releases — latest **v2.19.0** (2026-11-19, commit `bcf38522`) |
| `main` now | `3e22cd71` — no open PRs; 79 open issues |
| Milestones | https://github.com/confighub/cub-scout/milestones — `v3.0.0` (4 open: #386, #562, #595, #645). `v2.13.0`, `v2.14.0`, `v2.15.0` still hold open issues and are stale: those releases shipped. |
| GitHub Projects boards | Not used for this repo as far as we know. They could not be listed, because the local `gh` token lacks the `read:project` scope. Issues, milestones and the docs below are the tracker. |
| Execution tracker issue | [#645](https://github.com/confighub/cub-scout/issues/645) (body last updated 2026-10-10; stale — the roadmap doc below is current) |
| 3.0 transition issue | [#595](https://github.com/confighub/cub-scout/issues/595) |
| ConfigHub `cub` CLI source (read-only reference) | `~/code/confighub/public/cmd/cub` — read it to see exactly what a `cub` command sends and prints |
| Homebrew tap | https://github.com/confighub/homebrew-tap (`cub-scout.rb` source formula, `Casks/cub-scout.rb`), updated by the release workflow |
| Container image | `ghcr.io/confighub/cub-scout:<tag>` (linux/amd64 only) |
| Codex's old worktrees | `~/.codex/worktrees/<name>/cub-scout` (22 of them). **Do not edit inside them.** |
| Remote branches | 125 on `origin`, 60 already merged into `main`. Do not delete any without the maintainer's word. |

## 2. What cub-scout is, and the rules that never bend

Read `CLAUDE.md` in the repo root first; it is binding. In short:

- **Read-only.** cub-scout never writes to a cluster except where the
  maintainer allowed it: the demos, `import argocd --disable-sync`/`--delete-app`,
  the import wizard's Argo CD hand-off and `map hub`'s import. All of those
  refuse unless the cluster is named with `--kube-context`.
  `scripts/check-readonly.sh` enforces this, including `kubectl` write verbs.
- **Deterministic, no AI/ML, parse don't guess.** Ownership comes from real
  labels, annotations and owner references. "Unknown" always beats a wrong
  claim. Never assert "unmanaged" or "orphan" from missing data.
- **Witness / judge / authority.** cub-scout is the witness. Pilot is the
  judge. ConfigHub is the authority. cub-scout reports facts and whose word
  each rests on. It never gives verdicts like "safe" or "healthy", and never
  remediates.
- **CLI/TUI/MCP parity.** Every capability has a CLI command with
  `--format ascii|json|md`, a TUI rendering and, where agent-facing, an MCP
  tool. A person is never told more than an agent is.
- **Measured for agents** (principle 9). Claim an agent benefit only where the
  evals show it. **Paid eval runs are not approved.**
- **Six questions** (principle 10,
  `docs/reference/agentic-gitops-observability.md`). For every capability,
  answer these:
  1. Which question of an agent's does it answer?
  2. Whose word does each fact rest on?
  3. How do unknowns show?
  4. Does "not yet" differ from "no"?
  5. What does it cost?
  6. Is cluster text quoted?
- **Definition of done** (CLAUDE.md, "Pre-Coding Test & Success Proof"):
  - deterministic unit tests;
  - an example;
  - a real-cluster proof where behaviour touches a cluster;
  - graceful-degradation rules;
  - an agent eval case for agent-facing changes.

  State any waiver visibly.

## 3. Read these, in order

1. `CLAUDE.md`
2. `AI-README-FIRST.md` and the newest sections at the **end** of `HANDOVER.md`
   (it is chronological, about 2,400 lines; read the last six sections)
3. `docs/roadmap-3.0-execution.md` — **the state of the plan** (top section,
   "State of the plan — 2026-11-19") wins over everything else; then "Agent
   plan — 2026-11-13", "Trimmed plan — 2026-11-10" and "3.x direction —
   2026-11-08". Lower sections are history.
4. `docs/reference/agentic-gitops-observability.md` — the vision
5. `docs/migration-3.0.md` — what 3.0 removes
6. `docs/proposals/answer-envelope.md` and the "answer" sections of
   `docs/reference/json-contracts.md`
7. `docs/reference/commands.md`, `docs/reference/cli-contract.md`
8. `evals/README.md`
9. `docs/roadmap.md` — the "Untracked Backlog Checklist" near the top (some
   ticks are stale; see section 6)

Release notes for everything shipped are in `docs/releases/v2.*.md`. v2.18.0
and v2.19.0 describe the last two weeks of work.

## 4. Is 3.0 written up? Honest answer

- **Vision: yes.** `docs/reference/agentic-gitops-observability.md` defines
  agentic GitOps observability and cub-scout's place in the agent's change
  loop. The claim 3.0 should be able to make: *when an agent changes
  configuration, cub-scout is how it finds out what happened in the cluster,
  in one call, at a known cost, with the evidence attached and nothing
  guessed.*
- **3.0 and 3.1 roadmap: yes, but spread out.** It sits across three dated
  sections of `docs/roadmap-3.0-execution.md` plus #595 and the
  roadmap checklist. Later sections supersede earlier ones in places, and no
  single list says what is left. Section 5 below is that list.
- **3.2 and later: not written.** Nothing is planned beyond 3.1.

## 5. What is left to reach 3.0

### Already done (do not redo)

- The `answer` envelope (`state`, `reason`, `summary`, `observedAt`,
  `cluster`, `basis[]` with whose word, `unknowns[]`, `next[]`, `cost`) is on
  nine commands: `trace`, `compare source-truth` (lean), `gitops status`,
  `explain`, `doctor`, `map list`, `scan`, `gitops settings`, `release check`.
  A test (`TestAnswerIsTheSameThroughMCP`, plus per-command `*_answer_mcp_test.go`)
  holds the MCP output to the CLI's, and each answer is checked on a real
  cluster in CI.
- Every command takes `--kube-context` or a stated exemption, held by
  `cmd/cub-scout/command_context_registry_test.go`. Every cluster write
  refuses without it.
- Deprecation notices for everything 3.0 removes shipped in v2.18.0, so the
  one-minor deprecation window #595 requires is already met.
- `doctor --before-attach` (#888), Crossplane `Ready` in `trace`, Flux
  "not yet" is `WATCH` in `release check`. The independent check of a
  Release's recorded live status (#887) has been seen to agree and to
  disagree on real clusters under Flux and Argo CD.

### The 3.0 transition (#595, #386, #562) — no decision needed

1. Module path `/v2` → `/v3`, every self-import rewritten, a test that no
   un-suffixed self-import remains.
2. Remove the deprecated surfaces listed in `docs/migration-3.0.md`:
   - `trace --json`, `tree --json`, `graph export --json`, `gitops status --json`;
   - `connect --go`, `app create --set-context`;
   - `hierarchy`, `fleet outliers`.

   A removed flag must fail and name its replacement, never be silently
   ignored.
3. Remove `compare source-truth`'s verdict words (`status`, `source_truth`)
   from JSON and MCP output; they stay in ASCII and Markdown (decided
   2026-11-16).
4. A cub version floor, named at release time against the then-current cub
   (the "0.6" in #595 predates the SDK reads; current cub is v0.11.x). It is
   enforced by the connected gate and stated in `cli-contract.md`.
5. Plugin-first naming (#386): `cub scout …` is the documented primary form.
   Keep the standalone `cub-scout` binary. Show both forms in examples, the
   plugin form first.
6. Write `docs/releases/v3.0.0-migration.md` (or promote
   `docs/migration-3.0.md`), and add a test that runs its before/after
   commands against the built binary.

### The agent plan's 3.0 items (roadmap "Agent plan — 2026-11-13")

| Item | State | What is left |
|---|---|---|
| A(1) "did my change land" as one tool and command: `landed` / `converging` / `not-landed` / `not-observable`, naming the broken link (with G1) | not started as its own tool | Design the state words with the maintainer first. They choose state words. Propose, don't decide. |
| G2: a bounded, read-only wait on the landing check | not started | Must not be a watch service (#539 is not planned) |
| A(2) field value, last manager, will a controller revert it | partly (`explain`, field attribution) | A single answer shaped for the question |
| A(3) changes no one recorded (from `compare three-way`) | partly | Same |
| A(4) `next[]` reads on every answer, exact commands, never writes | done on the nine answers | Keep it so |
| G5: agent identity in attribution | not started | A field-manager convention to propose; report what is read, never guess |
| B: envelope enforced on every command | nine commands done | A conformance test across all agent-facing commands; the TUI, watch and bot surfaces of #596 |
| C: cost an agent can count on | MCP catalog size ceilings done (`mcp_catalog_size_test.go`); answer sizes vs raw `kubectl` recorded by the live lane | Per-answer size ceilings checked in CI; a summary view per tool; the deterministic bytes-and-tokens benchmark over the 24 prepared cases (no model, no credits) |
| E: measurement | rung 1 partly (answer sizes) | Rung 1 complete. Rungs 2–3 are paid and **not approved**. |
| F: be found and be tried | repository topics and homepage set; the README opens with a real-cluster `trace` answer, but no doc records the "one command, one answer" first screen as done | confirm the first screen with the maintainer; MCP directory and krew listings; one announcement at 3.0 carrying C's numbers |
| #599 remainder | explicit context done | cluster identity on every answer (waits on a decision), merge-safe IDs, failure isolation |

### Waiting on the maintainer (do not decide these)

- Any paid eval run. Three things wait on one: whether the shorter catalog
  helps, whether `--tools core` becomes the default in 3.0, and whether the
  `answer` block earns its bytes.
- `cluster.id` in every answer: read on request or always.
- Where to raise fact storage and delivery attestation with ConfigHub (3.1).
- The repository secret `CUB_SCAN_RELEASE_TOKEN`. Without it the `full` CI
  lane cannot provision its scanner.
- State words for any new answer.
- Closing issues. Candidates are listed in section 6.

## 6. Housekeeping found during the last sync

- **Issues that look done, for the maintainer to close or keep:**
  - #948: fixed by #963.
  - #955: fixed by #962 and #964.
  - #888: done by #947 and #952; the apps-Space overlap is left.
  - #561: fixed in v2.12.1; ancillary items are left.
  - #887: every comparison is now proven; a hosted server and the eval case are left.
- Stale milestones v2.13.0, v2.14.0 and v2.15.0 still hold open issues.
- #645's body was last updated 2026-10-10.
- In `docs/roadmap.md`'s checklist, item B says `explain`, `gitops status`
  and `doctor` do not carry `answer` yet. They do.
- `AI-README-FIRST.md`'s first line is dated 2026-11-16.
- `skills/` is pinned by a benchmark (`evals/combined-recorded-runtime`).
  Editing a skill file needs that pin moved, which is the benchmark owner's
  call; see that directory's README.

## 7. 3.1 and 3.x

Defined so far:

- **3.1**, which waits on ConfigHub's agreement and has no date:
  - facts in ConfigHub (#600): cub-scout as the only author of a fact,
    labelled observed rather than intended, opt-in;
  - a delivery attestation a ConfigHub ChangeWorkflow can require (G4);
  - signed receipts (#889);
  - a pullable bot image (#520).
- **Not planned** (each issue is kept, labelled `not-planned`):
  - #641, #605, #604, #602, #601, #584, #539, #519, #642, #594;
  - rebuilding `fleet outliers`.
- **Unchanged boundaries:**
  - no writes;
  - no remediation;
  - no metrics, logs or traffic;
  - no UI of cub-scout's own.

**Not written: 3.2 and after.** A first proposal for 3.x is welcome, as a
document for the maintainer to approve, not as work. Ground it in the vision
doc and the not-planned list. One idea already raised: a local graph query
layer for agents, such as loading `graph export` output into an embedded
graph store. It must not become a second fact store beside ConfigHub, and
like everything else it needs an eval before any benefit is claimed.

## 8. How to work here

**Branches, commits, PRs**
- One small change per branch and PR, opened against `main`.
- Write "Refs #N" in PR bodies, never "Closes"/"Fixes"/"does not close #N".
  Any closing keyword closes the issue.
- Never name colleagues or other people in public artifacts: issues, PRs,
  comments, commits, docs.
- **Self-merge when CI is green** (delegated by the maintainer on 2026-11-24).
  **Do not tag or release.** Get CI green, squash-merge, report. The
  maintainer still decides when to tag and release.

**Tests**
- `go build ./cmd/cub-scout && go vet ./... && go test ./...` for code.
- `go test ./test/unit` for any docs change: it pins exact phrases in the
  user docs.
- CI's Unit job also runs about 30 Python unittest steps listed in
  `.github/workflows/ci.yaml`.
- Run the local lint that is affordable: `ineffassign` takes about a second.
  `staticcheck` and `unused` are too heavy locally.
- Commit before a mutation check (break the fix, see a test fail, restore).
  Restoring with `git checkout` on uncommitted work wipes the fix.
- Don't pipe a test into `tail && push`: the pipe hides the exit code. Write
  to a file and check the code first.

**CI lanes**
- PRs run Unit, Integration, GitOps E2E (real Flux and Argo CD on kind) and
  Proof Artifact.
- Heavier lanes are manual:

  ```bash
  gh workflow run CI --ref <branch> -f level=<lane>
  ```

  Lanes: `connected`, `live-status`, `demos`, `attach`, `modelplane`,
  `windows`, `full`. `full` is blocked by the missing scanner secret. The
  `connected` and `live-status` lanes give a disposable real ConfigHub, so
  no production login is ever needed.
- When a lane flakes, re-run the failed job:

  ```bash
  gh run rerun <id> --failed
  ```

  One recent example: `cub cluster up` timed out waiting on Sveltos's own
  Deployments.
- Before pushing a CI step, check its flags against the built binary's
  `--help`. For example, `map list` takes `--namespace`, not `-n`.

**Safety: production and other people's systems**
- **Every local `cub` context points at production (hub.confighub.com).**
  Never run a connected cub-scout command or a `cub` command with the default
  config. When you must run `cub` locally:

  ```bash
  env -i PATH="$PATH" HOME=<private dir> CUB_CONFIG=<private dir> cub ...
  ```

  `localhost:32180` is a shared local ConfigHub with no logged-in context.
- Docker runs through OrbStack, and there are about ten kind clusters on this
  machine that are not yours. Do not use, change or delete them, and never
  run `kubectl` against the shared kubeconfig's current context. For a local
  cluster, create your own with a private `--kubeconfig` file and delete it
  afterwards. Better still, use the CI lanes.
- No `brew install`. Don't run Windows binaries. Never enter credentials.
- Spend as little as possible: no paid eval runs. Use cheap models for
  delegated chores.

**Shell gotcha**
- zsh does not word-split `$var`. Loop over literal values, or use arrays.

## 9. Suggested order for your first stages

Each stage is one or more PRs, opened for the maintainer to review.

1. **Consolidate the plan.**
   - Rewrite the top of `docs/roadmap-3.0-execution.md` into one
     "What is left to 3.0" table (section 5 above is the draft).
   - Fix the stale checklist ticks and the `AI-README-FIRST.md` date.
   - Propose an updated #645 body in the PR description. Do not edit #645
     yourself.
2. **The 3.0 transition** (#595), each in its own PR:
   1. `/v3`;
   2. removals with fail-and-name-the-replacement tests;
   3. the verdict words leaving JSON and MCP;
   4. plugin-first docs (#386);
   5. the migration guide with its executed-commands test.

   These need no new decisions. The cub floor needs the version named; ask.
3. **C without credits.**
   - Per-answer size ceilings in CI and a summary view per tool.
   - The deterministic bytes-and-tokens benchmark over the 24 cases, with its
     method published.
4. **A(1) "did my change land" with G1 and G2.**
   - Start with a proposal doc: the question, the six answers, the state
     words for the maintainer to choose, the recorded fixtures.
   - Build only after approval.
5. **A 3.x vision draft** (3.2+), as a proposal doc only.

Ask before anything that writes outside this repository, tags, releases, closes
issues, deletes branches, or decides a state word or product question.
