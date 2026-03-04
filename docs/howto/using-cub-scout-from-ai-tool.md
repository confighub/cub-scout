# Using cub-scout from an AI Tool

Use this guide when you want an AI assistant (Claude/Codex/other) to answer:

- "Can I do X with cub-scout or ConfigHub?"
- "Show me exactly how to do it."
- "If this is not possible, file a feature request."

## Step 0: Load the Skill

Canonical skill file:
- [../ai/cub-scout-skill.md](../ai/cub-scout-skill.md)

At session start, ask your AI:

```text
Load and follow docs/ai/cub-scout-skill.md as your operating profile for this session.
```

If your AI tool cannot load local files, copy the `Canonical Prompt` from that skill file.

## What to Expect

When this flow works well, the AI assistant should:

1. Classify your ask as:
   - `Standalone cub-scout`
   - `Connected cub-scout + ConfigHub`
   - `ConfigHub/cub workflow`
2. Verify commands and flags before claiming capability.
3. Run read-only discovery first.
4. Use `--dry-run` before any write action.
5. If capability is missing, offer to file a GitHub issue with evidence.

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

- `cub-scout` is cluster read-only by default.
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
2. AI verifies `import`/`import-argocd` help and connection status.
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

Recommended commands to keep in every flow:

```bash
./cub-scout --help
./cub-scout <command> --help
./cub-scout import --dry-run -n <namespace>
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
- [../ai/cub-scout-skill.md](../ai/cub-scout-skill.md) (`Canonical Prompt` section)

Paste at the start of an AI session if needed:

```text
You are my cub-scout + ConfigHub capability assistant.
For each request:
1) Classify as standalone, connected, or ConfigHub workflow.
2) Verify commands/flags from help before claiming support.
3) Provide shortest safe command path (dry-run first).
4) If unsupported, explain gap clearly and offer to file a GitHub issue.
5) Use command output as evidence; do not guess.
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

## If It Is Not Possible Yet

Capture:
- user goal
- commands attempted
- observed behavior
- expected behavior
- demo/user impact

File with helper script:

```bash
./scripts/create-ai-capability-issue.sh \
  "Capability gap: <short title>" \
  "<user goal>" \
  "<commands attempted>" \
  "<observed gap>" \
  "<expected behavior>"
```

Or use GitHub issue template: `AI capability gap`.

## Related Guides

- [claude-capability-assistant.md](claude-capability-assistant.md)
- [import-to-confighub.md](import-to-confighub.md)
- [import-from-live.md](import-from-live.md)
