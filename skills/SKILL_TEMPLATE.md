# Shared skill template (cub-scout, read-only)

Every `SKILL.md` under `skills/` in this repo starts from this scaffold. cub-scout is a **read-only** observer — skills here never mutate the cluster, never write to ConfigHub, never run apply/sync/patch/delete. Keep skills under ~300 lines; push depth into `references/`.

```markdown
---
name: <skill-slug>
description: <One sentence on what the skill does AND when to trigger, including phrases a user would actually say. End with 1–2 concrete "do NOT load for" cases. Be a bit pushy — Claude undertriggers by default.>
phase: verify | cross-cutting
allowed-tools: <Enumerate the specific verbs your skill teaches. DO NOT use broad Bash(cub-scout *) / Bash(./cub-scout *) / Bash(cub scout *) wildcards — those grant `cub-scout demo` (kubectl-applies to set up demo state), `cub-scout import apply` (creates ConfigHub resources), and `cub-scout compare --suggest --apply` (creates ConfigHub Apps + Deployments). The architectural triad is read-only-by-default but specific verbs and flag combinations mutate; allowed-tools must enumerate the read-only verbs only. Pattern per verb: Bash(./cub-scout <verb>) Bash(./cub-scout <verb> *) Bash(cub-scout <verb>) Bash(cub-scout <verb> *) Bash(cub scout <verb>) Bash(cub scout <verb> *). Add Bash(kubectl get *), Bash(kubectl describe *), Bash(kubectl logs *), Bash(kubectl version), Bash(kubectl config current-context) where the skill needs cluster reads. Add Bash(cub * get), Bash(cub * list), Bash(cub link list *), Bash(cub unit get *) for connected-mode ConfigHub reads. Add Bash(argocd app get *), Bash(flux get *) for controller diagnostics. NEVER grant Bash(kubectl apply/edit/patch/delete *), Bash(cub * delete), Bash(cub * update), Bash(cub * create), Bash(argocd app sync), Bash(./cub-scout demo *), Bash(./cub-scout import apply *), or any mutating pattern.>
---

# <skill-name>

One or two sentences on what the skill enables, in plain terms.

## When to use

- <explicit user phrasings — verbatim utterances Claude should match on>
- <implicit intents this should cover>
- <CI / agent / MCP contexts where this skill applies>

## Do not load for

- <adjacent skills that look similar but cover a different surface — name them>
- <cases where a different tool is more appropriate (`cub` for authoring, `kubectl` for direct admin, etc.)>

## Standalone vs connected

State explicitly which inputs the skill needs:

- **Standalone (cluster only):** what works without ConfigHub auth
- **Connected (cluster + `cub auth login`):** what unlocks with ConfigHub
- **Offline (file / bundle only):** what works without a cluster

Receipts and other artifacts produced by cub-scout always work in the lowest available mode; missing connected-mode evidence is recorded as structured `omissions[]`, never as failure.

## Tool boundary

- **Allowed (read-only):** `cub-scout *`, `cub scout *`, `kubectl get/describe/logs`, `argocd app get`, `flux get`, `cub * get`, `cub * list`, `cub link list`, `cub unit get`.
- **Not allowed (mutating):** `kubectl apply/edit/patch/delete`, `argocd app sync` (as a mutation), `flux reconcile --with-source`, `helm install/upgrade`, `cub * delete/update/create`, any `--dry-run=false` carve-out, any path that writes to the cluster or ConfigHub.
- **Boundary with `cub` (the ConfigHub CLI):** cub-scout observes; `cub` authors and governs. If the user wants to *change* state, hand off to `cub` skills in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) — never edit through cub-scout.

## The loop

Numbered imperative steps. Explain **why** each step matters, not just what. Keep the loop small and named. Most cub-scout skill loops look like:

1. **Identify intent** — translate the user's question into the right verb group (Observe / Diagnose / Compare / Attribute / Ingest / Govern / Integrate / Verify).
2. **Pick the verb** — within the group, pick the most specific command.
3. **Invoke** — run with `--format json` for agent contexts, ASCII for humans.
4. **Interpret** — read the structured evidence (verdict / cause / omissions / next steps).
5. **Hand off** — if the answer requires mutation, name the `cub` skill that owns it; never act yourself.

## Worked example

Show one realistic end-to-end invocation with actual cub-scout output (ASCII or JSON), so readers see what the skill produces. Cite the file in `examples/` where the fixtures live, if applicable.

## Output evidence

What the user gets back, structured:

- **Primary artifact** — the JSON / ASCII output the skill produces
- **Receipt** — when applicable, the `cub-scout receipt` artifact attached to the operation (see `scout-verify` once that skill ships)
- **Next-step hints** — structured `nextSteps[]` from #370, where each hint has `actionType: read-only` (mutating actionType is rejected at every cub-scout emit point)

## References

- `references/standalone-vs-connected.md` — the input-mode axis cub-scout commands share
- `references/read-only-triad.md` — cub-scout / Pilot / ConfigHub role separation
- <other refs this skill needs>

## Constraints

- Receipts and attribution evidence are **historical, immutable records** — see `references/...`. Don't suggest "updating" a receipt; updates produce *new* receipts.
- If evidence is missing (managedFields stripped, ConfigHub offline, source-truth INCOMPLETE), record the gap in `omissions[]` and return `INCONCLUSIVE`. Never guess.
- If a user asks the skill to *act* (apply, edit, sync), refuse and route to the appropriate `cub` skill or `kubectl` (with the user's direct hands on the keyboard).
```

## Notes for skill authors

- **Phase value:** most cub-scout skills are `phase: verify` (observation / diagnosis / comparison / attribution / verification). `cross-cutting` is reserved for the umbrella router skill and references-only profiles.
- **Read-only-triad invariant:** every skill's `allowed-tools` line must stay inside #410/#428 — see [Tool boundary] above. A skill that adds a mutating pattern is a bug, not a feature. **Do not use `Bash(cub-scout *)` broad wildcards** — they grant `demo` (kubectl-applies), `import apply` (writes to ConfigHub), and `compare --suggest --apply` (writes to ConfigHub). Enumerate verb-specific patterns only.
- **Why we enumerate per-verb (open question from external review):** cub-scout does not yet have a machine-enforced no-mutation command registry. The architectural triad is policy; the CLI surface is broader than the triad. Until cub-scout exposes a `--read-only` global flag or an enforced command-classification table, skills must list read-only verbs by hand. This is a forward-compat hook for that future enforcement.
- **CI-tool neutrality:** worked examples should not hard-code GitHub Actions / GitLab CI / Jenkins-specific wrapper syntax. Pick the shell-level command; users adapt to their CI tool.
- **Standalone before connected:** lead with the standalone case in worked examples; the connected case is the enrichment, not the primary.
- **Cite the verb-group docs:** reference `README.md` § "Capability Map" and the corresponding `docs/reference/commands.md` section for any verb the skill invokes.
