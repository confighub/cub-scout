# Claude Capability Assistant for cub-scout + ConfigHub

For interface selection and broader expectations (Claude session vs CLI vs Slack), see:
[using-cub-scout-from-ai-tool.md](using-cub-scout-from-ai-tool.md).
For the shared Claude/Codex skill profile, see:
[../../skills/cub-scout/SKILL.md](../../skills/cub-scout/SKILL.md).

Use this guide when you want Claude to answer:

- "Can I do X with cub-scout or ConfigHub?"
- "Show me exactly how."
- "If not possible, file a feature request."

## 1) Prompt Claude with This Role

Paste this into Claude at session start:

```text
First read AI-README-FIRST.md, then load and follow skills/cub-scout/SKILL.md. For capability-triage or demo requests, also load skills/cub-scout/references/capability-assistant.md.
```

## 2) Capability Triage Flow

For each user question, Claude should run this checklist:

1. Identify scope:
   - Standalone (`cub-scout` only)
   - Connected (`cub auth login` + `cub-scout import/fleet`)
   - ConfigHub (`cub` workflows, spaces, units, workers)
2. Verify command/flag existence:
   - `./cub-scout --help`
   - `./cub-scout <command> --help`
3. Verify behavior against docs:
   - `README.md`
   - `docs/reference/commands.md`
   - `docs/reference/cli-contract.md`
   - `docs/howto/import-*.md`
4. Return one of:
   - `Supported now`
   - `Supported with prerequisites`
   - `Not supported yet`

## 3) Demo Pattern

Use this conversation loop in demos:

1. User: "Can I import Argo workloads into ConfigHub from this cluster?"
2. Claude: Verifies `import`, `import argocd`, and connected prerequisites.
3. User: "Do it."
4. Claude: Runs `--dry-run` first, then guided real import.
5. User: "Can it also auto-create Git patches/upgrades?"
6. Claude: If missing, mark as gap and offer issue creation.

## 4) Issue Escalation Path (When Not Supported)

If the user approves filing a request:

1. Capture evidence:
   - command attempted
   - actual output
   - expected behavior
   - demo impact
2. Generate issue-ready draft from transcript:

```bash
./scripts/run-to-issue-evidence.sh \
  --title "Capability gap: <short title>" \
  --goal "<user goal>" \
  --expected "<expected behavior>" \
  --impact "<demo/user impact>" \
  --transcript <failed-session-transcript.txt> \
  --output /tmp/cub-scout-issue-draft.md
```

3. Optional direct open:

```bash
./scripts/run-to-issue-evidence.sh \
  --title "Capability gap: <short title>" \
  --goal "<user goal>" \
  --expected "<expected behavior>" \
  --impact "<demo/user impact>" \
  --transcript <failed-session-transcript.txt> \
  --open
```

## 5) Response Format for Claude

Keep answers consistent:

```text
Verdict: Supported now | Supported with prerequisites | Not supported yet
Why: <1-3 lines with evidence>
Do this:
  <exact commands>
If blocked:
  <specific prerequisite or limitation>
Issue option:
  <yes/no prompt + proposed title>
```

## 6) Guardrails

- Prefer `--dry-run` before mutating actions.
- Distinguish cluster read-only behavior from ConfigHub writes.
- Do not claim features that only exist in roadmap/reference drafts.
- Use command output as ground truth during demos.
