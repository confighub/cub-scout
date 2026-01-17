# Introduction to ConfigHub Agent

Welcome! This repo contains the **ConfigHub Agent** — please try it!

## What's In This Repo

- **ConfigHub Agent and TUI** — The main tool. See what's running, who owns it, find misconfigurations.
- **Hub/App Space model** — Architecture for application spaces and platform governance
- **Examples** — IITS use cases, app config patterns, integrations
- **GSF (GitOps State Format)** — Standard JSON format for GitOps integrations
- **CCVE database** — 1,700+ configuration anti-patterns (including Kyverno policies)

For the Agent and TUI, start with [README.md](../README.md). For architecture and planning, see [docs/planning/](planning/).

---

## What Problems Does This Solve?

Every feature exists to solve one of these user problems.

## The Core Problem

> "Every DevOps user we spoke to complained about 'too many moving parts' leading to a complete loss of visibility and operability. Changes take too long and compliance is unverifiable."

**64% of outages are caused by configuration and change management** (Uptime Institute, 2023).

At BIGBANK, one extra space in a Grafana config file caused a 3-day cascade of breakages. The root cause? Too many systems touching too many YAML and Helm files.

---

## Problems by Stage

We solve different problems at each stage of adoption. You can stop at Stage 1 and still get value.

### Stage 1: Read-Only Agent (FREE, Available Now)

What you can answer today with `map` and `scan`:

| Your Question | What You Do Now | With Agent | Time Saved |
|---------------|-----------------|------------|------------|
| "What's running on my cluster?" | kubectl + grep + spreadsheets | `./test/atk/map` | 30-60 min → 5 sec |
| "Who owns each resource?" | Check labels, guess, ask around | `./test/atk/map` | 10-30 min → 5 sec |
| "What config bugs exist?" | Wait for outage, manual review | `./test/atk/scan` | Hours → 10 sec |
| "Can you fix it for me?" | Manual YAML edits | `./test/atk/scan --auto-fix` | Minutes → seconds |
| "What's broken right now?" | kubectl describe, check dashboards | `./test/atk/map problems` | 15 min → 5 sec |
| "What's suspended/forgotten?" | Search through YAML files | `./test/atk/map suspended` | 20 min → 5 sec |
| "What are my GitOps pipelines?" | Check Flux/Argo dashboards separately | `./test/atk/map pipelines` | 10 min → 5 sec |

**Connected mode** (with `cub auth login`) adds:

| Your Question | Tool |
|---------------|------|
| "Show me the ConfigHub hierarchy" | `./test/atk/map confighub` |
| "What does the platform team see?" | `./test/atk/map --mode=admin` |
| "What does the fleet look like?" | `./test/atk/map --mode=fleet` |
| "Which clusters are behind?" | ConfigHub hierarchy shows revisions per unit |

**Lock-in: None.** Delete the agent anytime.

---

### Stage 2: Full ConfigHub Platform (Commercial, Coming)

Problems we want to solve with the full platform:

| Your Problem | What Happens Today | Proposed Solution |
|--------------|--------------------|--------------------|
| "What I see in Git isn't what deployed" | Mental compilation of bases + overlays + patches | WET configs — what you see is what deploys |
| "Platform update clobbered my overlay" | Silent patch failures, 3-day debugging | Structural boundaries — platform can't override app settings |
| "Did my change land everywhere?" | Check each cluster manually | Transaction-scoped queries prove changes landed |
| "Merge this hotfix back to Git" | Manual edits, hope you got it right | `cub drift merge` creates clean PR |
| "Query across all 50 clusters" | Repeat kubectl 50 times | Fleet-wide Map queries |
| "Bulk update with audit trail" | Individual changes, no grouping | Changesets with intent, ticket links, atomic apply |
| "Compliance checked too late" | Policies run at deploy time | Hub constraints validated on import |
| "Triggers diverge across environments" | Per-environment configs drift apart | Actions on App Space with label filters — one place |

**Lock-in: Medium.** Export configs as YAML anytime.

---

### Stage 3: AI and Automation (Future)

Problems we want to solve once AI and self-service apps are common:

| Your Problem | What Happens Today | Proposed Solution |
|--------------|--------------------|--------------------|
| "AI-generated configs need governance" | No controls on what AI produces | Configuration as Data platform with policy gates |
| "Self-service apps create chaos" | Teams deploy whatever, platform can't keep up | Hub constraints, agentic workers |
| "Automated remediation" | Manual fixes for every issue | AI copilots that understand config structure |
| "Vibe coding meets production" | AI outputs YAML, crosses fingers | Policy validation before deployment |

**Lock-in: Higher.** Would need to re-implement logic elsewhere.

---

## Why These Problems Matter

### Lack of Fidelity

What you see in Git isn't what deploys. You have:
- Base configs
- Kustomize overlays
- Helm values files
- Variable substitutions
- Post-build patches

To understand what actually runs in production, you need to "mentally compile" all these layers — or run `flux build` for each kustomization. Code review is nearly impossible.

### Compliance Too Late

Policies get checked at deploy time, if at all. By then the bad config is already in Git, already reviewed, already merged. The deploy fails or — worse — succeeds and causes an outage.

### Slow Root Cause

When something breaks, the issue could be:
- In the base configuration
- In a patch that's not applying correctly
- In a variable substitution that's missing or wrong
- In the dependency chain

It took BIGBANK 3 days to find one space character. Every layer of abstraction adds another place where things can go wrong.

### Complex Remediation

You fix something in production. Now you need to:
1. Figure out where in Git to change it
2. Navigate the base/overlay/patch structure
3. Hope your change doesn't get clobbered by the next platform update
4. Hope you don't clobber something else

And there's no audit trail grouping related changes together.

### Loss of Velocity

All of the above means:
- Deployments take longer (worse DORA lead time)
- Changes fail more often (worse change failure rate)
- Recovery takes longer (worse MTTR)
- Teams deploy less frequently (worse deployment frequency)

The toil and complexity of traditional config tools directly hurts your DORA metrics.

---

## Try It Now

```bash
# 30 seconds to answers
curl -sL https://get.confighub.com/agent | bash
./test/atk/map      # What's running, who owns it
./test/atk/scan     # What config bugs exist
```

---

## What Each Tool Shows You

### Map (`./test/atk/map`)

Answers: "What's running and who owns it?"

Shows you:
- Every Kubernetes resource in your cluster
- Who manages it (Flux, Argo CD, Helm, ConfigHub, or native K8s)
- The Git source for GitOps-managed resources
- Drift status (does cluster match Git?)
- Problems, suspended resources, pipelines

### Scan (`./test/atk/scan`)

Answers: "What config bugs exist?"

Shows you:
- 1,700+ known configuration anti-patterns (CCVEs)
- Real bugs that have caused outages at other companies
- Severity and category for each finding
- How to fix each issue

### Fleet View (requires auth)

Answers: "Same questions, all my clusters"

Shows you:
- Aggregated view across all connected clusters
- Consistent ownership detection everywhere
- Fleet-wide CCVE scanning results
- Which clusters are behind on deployments

---

## Note

These are problems we would like to solve if we are correct — not problems we have definitively solved. Stage 1 is available now. Stage 2 and 3 are our roadmap.

---

## Research

For detailed research on Kubernetes configuration issues and CCVE mining:
- [K8s CCVE Issues Research](planning/ccve/K8S-CCVE-ISSUES-RESEARCH.md) — GitHub issues analysis (controller bugs, API validation gaps)
- [CCVE Mining Log](#) — Session-by-session CCVE discovery
- [K8s Exhaustive Mining Plan](planning/ccve/K8S-EXHAUSTIVE-MINING-PLAN.md) — Research strategy
