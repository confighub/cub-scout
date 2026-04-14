# cub-scout Capability Assistant Reference

Use this reference with the canonical skill when the user is asking:

- "Can cub scout do X?"
- "Should I use cub scout or kubectl here?"
- "Show me the exact supported path."
- "If this is missing, file a GitHub issue with evidence."

## Load order

1. `AI-README-FIRST.md`
2. `skills/cub-scout/SKILL.md`
3. this file for capability-triage or demo-style conversations

Session start prompt:

```text
First read AI-README-FIRST.md, then load and follow skills/cub-scout/SKILL.md. For capability-triage or demo requests, also load skills/cub-scout/references/capability-assistant.md.
```

## Operating rules

1. Verify before claiming:
   - `./cub-scout --help`
   - `./cub-scout <command> --help`
   - `cub gitops --help` and `cub gitops import --help` when the request crosses into ConfigHub workflows
2. Classify every ask as:
   - `Supported now`
   - `Supported with prerequisites`
   - `Not supported yet`
3. Prefer read-only inspection first.
4. Use `--dry-run` before any write path.
5. Distinguish:
   - standalone `cub scout`
   - connected `cub scout` + ConfigHub
   - ConfigHub / `cub` workflows
   - Git preview versus render/import boundaries
6. Use command output and docs evidence; do not guess.

## Tool boundary reminders

- `cub scout import --git-path` is a local structure/import-preview path
- `cub gitops import` is a target + render-target workflow
- `confighub/sdk` renderers are implementation detail for `cub`, not an implied `cub scout` feature

## Response format

```text
Verdict: Supported now | Supported with prerequisites | Not supported yet
Why: <brief evidence-based rationale>
Do this:
  <exact commands>
If blocked:
  <specific prerequisite or limitation>
Issue option:
  <ask whether to file issue, with proposed title>
```

## Issue escalation

When the user approves filing a request, capture:

- user goal
- commands attempted
- observed behavior
- expected behavior
- demo or user impact

Preferred path:

```bash
./scripts/run-to-issue-evidence.sh \
  --title "Capability gap: <short title>" \
  --goal "<user goal>" \
  --expected "<expected behavior>" \
  --impact "<demo/user impact>" \
  --transcript <failed-session-transcript.txt> \
  --open
```

Fallback:

```bash
./scripts/create-ai-capability-issue.sh ...
```
