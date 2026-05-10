# Using cub scout from an AI Tool

Use this guide when you want an AI assistant (Claude/Codex/other) to answer:

- "Can I do X with cub scout or ConfigHub?"
- "Show me exactly how to do it."
- "If this is not possible, file a feature request."

## Step 0: Load the Skill

For coding/review sessions inside this repo, load:
- [../../AI-README-FIRST.md](../../AI-README-FIRST.md)

Canonical skill file:
- [../../skills/cub-scout/SKILL.md](../../skills/cub-scout/SKILL.md)

Long-form capability-assistant reference:
- [../../skills/cub-scout/references/capability-assistant.md](../../skills/cub-scout/references/capability-assistant.md)

At session start, ask your AI:

```text
First read AI-README-FIRST.md, then load and follow skills/cub-scout/SKILL.md. For capability-triage or demo requests, also load skills/cub-scout/references/capability-assistant.md.
```

If your AI tool cannot load local files, copy the session prompt from the capability-assistant reference file.

## Related Tool Abilities

Use these boundaries consistently:

- `cub scout` (preferred documented form; local repo commands use `./cub-scout`)
  - read-only cluster/GitOps observation
  - connected comparison such as `compare three-way`
  - local Git structure preview via `import --git-path`
- `cub`
  - ConfigHub intended-state workflows
  - `cub gitops discover`
  - `cub gitops import`
- `confighub/sdk`
  - renderer implementation detail behind `cub`
  - not an automatic capability of `cub scout`

Important:
- `cub scout import --git-path` previews repo structure and import proposals
- `cub gitops import` renders/imports discovered GitOps resources from cluster targets
- do not blur these into one imaginary command surface

## What to Expect

When this flow works well, the AI assistant should:

1. Classify your ask as:
   - `Standalone cub scout`
   - `Connected cub scout + ConfigHub`
   - `ConfigHub/cub workflow`
   - or `Git preview vs render/import boundary`
2. Verify commands and flags before claiming capability.
3. Run read-only discovery first.
4. Use `--dry-run` before any write action.
5. If capability is missing, offer to file a GitHub issue with evidence.

## Presentation Modes

For AI-assisted use, prefer an explicit presentation-mode model instead of trying to infer too much from the host environment.

Presentation mode is separate from:

- whether you are in standalone or connected operation
- whether the current installation has paid or governed features available

Recommended modes:

- `human` for direct operator reading
- `ai` for AI-oriented summaries and handoffs
- `paired` for human-plus-assistant workflows

Important rules:

- explicit mode selection should win over auto-detection
- any host detection should be advisory only
- JSON and MCP (Model Context Protocol) outputs should stay structurally stable across presentation modes
- text and markdown outputs can adapt more for readability and handoff quality

Useful concepts:

- `requested_mode`: what the caller explicitly asked for
- `detected_context`: what the environment appears to be
- `effective_mode`: the mode actually used after applying defaults

Keep these separate from:

- `entitlement_tier`: what commercial/hosted features may be unlocked
- `capability_mode`: what the system can actually do in this invocation

## Prerequisites

### Local tools

```bash
./cub-scout version
kubectl config current-context
```

### Connected mode (required for import/fleet)

```bash
cub auth login
./cub-scout status
```

Expected `status` result for connected mode:
- ConfigHub shows connected
- cluster/context are detected

If not connected, AI should stop and ask you to authenticate first.

## Safety Model (Important)

- `cub scout` is cluster read-only by default.
- Connected import writes inventory/state to ConfigHub.
- Connected import does not mutate Kubernetes manifests in your cluster.
- AI should start with preview commands (`--dry-run`) before prompting to proceed.

## The Three Recommended Interfaces

### 1) Claude Code Session (Primary recommendation)

Use when:
- You want "ask -> verify -> run -> explain" in one place.
- You want the AI to execute commands and inspect output.
- You want the AI to file capability-gap issues automatically when blocked.

Why this is first:
- Best operator UX for demos and onboarding.
- Strongest evidence loop (AI reads real command output, not guesses).

Typical flow:
1. Ask: "Can I import this Argo cluster into ConfigHub?"
2. AI verifies `import` / `import argocd` help and connection status.
3. AI runs `./cub-scout import --dry-run ...`.
4. AI asks for confirmation before real import.
5. If blocked, AI proposes and files issue with evidence.

### 2) Command Line Only (Foundation recommendation)

Use when:
- You want maximal determinism and scriptability.
- You need repeatable CI/dev scripts independent of chat interfaces.
- You want AI optional, not required.

Why this is foundational:
- All capabilities should remain available via CLI commands/scripts.
- AI layers should orchestrate CLI, not replace it.
- Presentation style should be selectable explicitly rather than inferred implicitly.

Recommended commands to keep in every flow:

```bash
./cub-scout --help
./cub-scout <command> --help
./cub-scout import --dry-run -n <namespace>
```

When presentation-mode support is added, prefer explicit flags such as:

```bash
./cub-scout doctor --presentation ai
./cub-scout explain deploy/api -n prod --presentation paired
```

### 3) Slack Front Door (Team recommendation)

Use when:
- Users ask high-level "Can we do X?" questions in shared channels.
- You want lightweight triage and routing.

Why this is third:
- Great for discoverability and team adoption.
- Weak for authenticated execution context and full command evidence.

Best model:
- Slack bot answers scope + likely path.
- Bot links to exact command runbook.
- Execution is handed to Claude session or CLI.

## Interface Decision Matrix

| Need | Best Interface |
|------|----------------|
| Fastest end-to-end demo execution | Claude Code session |
| Most repeatable automation/CI | Command line |
| Team discovery and triage | Slack |
| High-confidence capability verification | Claude Code session + CLI |

## Canonical AI Prompt

Source of truth:
- [../../skills/cub-scout/references/capability-assistant.md](../../skills/cub-scout/references/capability-assistant.md)

Paste at the start of an AI session if needed:

```text
You are my cub scout + ConfigHub capability assistant.
For each request:
1) Classify as standalone, connected, ConfigHub/cub workflow, or Git preview vs render/import boundary.
2) Verify relevant cub-scout and cub help before claiming support.
3) Provide shortest safe command path (dry-run first).
4) Distinguish cub-scout local Git preview from cub gitops import rendering.
5) If unsupported, explain gap clearly and offer to file a GitHub issue.
6) Use command output as evidence; do not guess.
```

## Example: "Can I import this cluster now?"

```bash
./cub-scout status
./cub-scout import --dry-run -n payments
./cub-scout import -n payments
```

Expected interaction:
1. AI checks connected status.
2. AI shows preview (`--dry-run`) result.
3. AI asks before real import.
4. You confirm at prompt:

```text
Import this into ConfigHub? [y/N]
```

## Recommended Rollout For AI Presentation Support

For product and implementation work, treat presentation mode as a separate concern from evidence collection.

Recommended sequence:

1. define explicit presentation modes first
2. add a small read-only CLI slice on commands like `doctor` and `explain`
3. keep JSON and MCP contracts unchanged
4. add advisory detection and richer propagation only later if needed

## If It Is Not Possible Yet

Capture:
- user goal
- commands attempted
- observed behavior
- expected behavior
- demo/user impact

Generate an issue-ready draft from a failed session transcript:

```bash
./scripts/run-to-issue-evidence.sh \
  --title "Capability gap: <short title>" \
  --goal "<user goal>" \
  --expected "<expected behavior>" \
  --impact "<demo/user impact>" \
  --transcript <failed-session-transcript.txt> \
  --output /tmp/cub-scout-issue-draft.md
```

Open directly on GitHub:

```bash
./scripts/run-to-issue-evidence.sh \
  --title "Capability gap: <short title>" \
  --goal "<user goal>" \
  --expected "<expected behavior>" \
  --impact "<demo/user impact>" \
  --transcript <failed-session-transcript.txt> \
  --open
```

Fallback (manual fields):

```bash
./scripts/create-ai-capability-issue.sh \
  "Capability gap: <short title>" \
  "<user goal>" \
  "<commands attempted>" \
  "<observed gap>" \
  "<expected behavior>"
```

## Related Guides

- [claude-capability-assistant.md](claude-capability-assistant.md)
- [import-to-confighub.md](import-to-confighub.md)
- [import-from-live.md](import-from-live.md)
