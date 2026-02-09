# TUI + GUI: One Product, Two Aspects

**For Jesper** — Why TUI and GUI solve the same core problem and must be joined up.

## Industry Direction: OpenTUI

[OpenTUI](https://github.com/anomalyco/opentui) enables unified work across GUI and TUI using React. See [OpenCode demo](https://opencode.ai/_build/assets/opencode-min-CiEsORKQ.mp4) showing this in action.

This validates the direction: TUI and GUI are converging. Build for both, not one or the other.

## The Core Problem

"What's where, why, and what should I do?" across DRY ↔ WET ↔ LIVE.

No single interface handles this. TUI+GUI together do.

## Google Maps Analogy

Users do three things: **discover**, **decide**, **journey**.

| Mode | Maps | ConfigHub | Interface |
|------|------|-----------|-----------|
| **Discover** | Search, find, orient | "What's running? Who owns it?" | TUI |
| **Decide** | Compare routes, plan | "Which import approach? Fleet strategy?" | GUI |
| **Journey** | Turn-by-turn directions | "Migrate this app, deploy to prod" | Both |

## Why TUI Exists (Not "Just Another k9s")

Competing TUIs solve **cluster visualization**. We solve **the app hierarchy problem**.

| Tool | Shows | Doesn't Show |
|------|-------|--------------|
| k9s, Lens | Live cluster | Ownership, GitOps context, WET store |
| ArgoCD UI | Argo apps | Flux, Helm, Native |
| Flux UI | Flux apps | Argo, Helm, Native |
| **cub-agent** | **Everything + ownership + connects to WET** | — |

**Differentiation:** TUI knows about DRY → WET → LIVE because it connects to ConfigHub.

## TUI Design Constraints (By Choice)

| Constraint | Why |
|------------|-----|
| **OSS** | Matches Flux/Argo ecosystem, maximizes adoption |
| **Read-only** | Enterprise-adoptable (RBC example), safe to point anywhere |
| **Single cluster** | Simple entry point, no multi-node friction |
| **Easy to integrate** | We want k9s, flux9s, etc. to add us |

These constraints avoid 3-5 major frictions of non-OSS/read-write/multi-cluster/SaaS.

## TUI = CLI++ (Not GUI Alternative)

- CLI is comfortable after tutorials
- CLI lacks feedback loop
- TUI provides feedback loop
- ":" key shells out to CLI, staying in context
- GUI+CLI feels like one product → GUI+TUI does too

## The Upsell Model

Each TUI tab shows: "Connect to ConfigHub to do X" / "Use ConfigHub for Y (fleets)"

TUI gets users in → shows value → natural path to GUI/paid.

## Problem → Solution Progression

| Problem | TUI (OSS) | TUI+GUI | Full GUI |
|---------|-----------|---------|----------|
| "What's running?" | `map list` | — | Fleet-wide view |
| "Who owns this?" | Ownership detection | — | Cross-cluster audit |
| "Find shadow IT" | `map orphans` | — | Compliance reports |
| "Trace to source" | `map trace` | Repo context in GUI | Full DRY→WET→LIVE |
| "Import to ConfigHub" | Wizard (lossy) | Plan in GUI, execute in TUI | Git import (complete) |
| "Deploy to prod" | — | Plan in GUI | Apply + verify in TUI |
| "Why did prod break?" | Quick trace | Root cause in GUI | Full incident view |
| "Fleet-wide query" | Single cluster | Connect to ConfigHub | Full fleet queries |
| "Onboard new team" | — | Design in GUI | Execute in TUI |
| "Config anti-patterns" | `scan` (Risk) | — | Remediation workflows |

## Two Import Paths

### Path 1: Live → ConfigHub (TUI)
```bash
cub-agent import
```
Safe, read-only. **Lossy:** misses repo layout, dev/prod relationship, generator context.

### Path 2: Git → ConfigHub (GUI)
Full DRY structure visible. GUI handles complexity (repos × folders × environments × templates) like Google Maps handles geography.

### Proposed Enhancements
```bash
cub-agent import --recommend  # Suggest order by dependencies
cub-agent migrate             # Full wizard: discover → plan → execute
```

## Addressing Concerns

### "There are lots of competing TUIs" (Jesper)
Those compete on cluster viz. We compete on **joined-up journeys** across DRY→WET→LIVE. No one else connects terminal to WET store.

### "We'll lose focus" (Jesper)
Focus comes from joined-up journeys, not dropping TUI. UX must understand CLI+TUI+GUI together. GUI focuses on what GUI does better (complex viz, Git import, fleet). TUI focuses on what TUI does better (quick queries, execution, feedback loop).

### "TUI is vibe coded"
Make it a virtue: "We're learning how users vibe code with config and AI. Here's a fun OSS project to get started."

## Why Now

| Factor | Implication |
|--------|-------------|
| Series A bar higher | Need more customers faster, can't be in deep niche |
| AI tools are TUI-first | Claude Code, OpenCode, Copilot — "customers are 100% AI soon" |
| TUIs are "cool" | Fast, great for devs, converging with GUIs ("Tuimorphic") |
| Customer signals | Dan loves TUI (ignored SDK/SaaS); QubeRT wants "find out" |
| Enterprise adoption | OSS read-only is adoptable at places like RBC |

## The Chainguard Model

Chainguard has Wolfi (OSS Linux) as entry point. OSS and paid products "slot together" rather than cannibalize.

**Our model:** CLI+TUI (OSS) slots into GUI (paid). TUI gets broad support, lays ground for commercial conversation.

## User Journeys (The Product)

The product IS the user journeys. Neither TUI nor GUI alone is sufficient.

| Journey | Start | End | Path |
|---------|-------|-----|------|
| "What's running?" | Alert | Ownership map | TUI |
| "Import Flux apps" | Git repos | ConfigHub units | GUI → TUI |
| "Deploy to prod" | WET revision | Live cluster | GUI plan → TUI execute |
| "Why did prod break?" | Incident | Root cause | TUI trace → GUI deep-dive |
| "Audit shadow IT" | Compliance req | Native bucket report | TUI → GUI report |
| "Onboard new team" | Nothing | Full GitOps setup | GUI design → TUI verify |

## Fun Entry Point

User has Flux/Argo on single cluster:
1. Turn off clobbering
2. Add TUI (`cub-agent map`)
3. Connect TUI to confighub.com
4. Simple way to play with ConfigHub + GitOps

## Summary

- **One product** — TUI and GUI are two aspects, not competitors
- **Joined-up journeys** — discover (TUI) → decide (GUI) → journey (both)
- **OSS entry point** — maximizes adoption, avoids friction
- **Upsell built-in** — TUI shows ConfigHub value on every tab
- **AI-aligned** — TUI-first matches where industry is going
