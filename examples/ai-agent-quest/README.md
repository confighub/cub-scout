# Giving Your AI Eyes

## An AI Agent Quest for Kubernetes Operators

Your AI assistant is brilliant at reasoning. But right now it is blind
to your infrastructure. It cannot see your cluster. It does not know
who owns what, what changed, or what is broken.

This quest gives it sight — and then memory.

**Act 1** (Stages 0–3) gives your AI read-only cluster vision using
cub-scout as its eyes. Every command works today, standalone, no account
required.

**Act 2** (Stages 4–5) shows you the wall — questions your AI *cannot*
answer with cluster observation alone — and then removes it by connecting
ConfigHub as a data resource.

Complete Act 1 to earn the title: **Agent of the Observable Realm**.
Complete both acts to earn: **Agent of Intent and Evidence**.

---

## How This Quest Works

You direct. Your AI operates.

At each stage you give your AI a prompt. It uses cub-scout commands to
explore, reason, and report. You watch the reasoning — not just the
output, but the *thinking*.

This is not "run these commands in order." This is "watch an AI agent
understand your cluster."

### Supported AI Tools

| Tool | How cub-scout connects |
|------|----------------------|
| Claude Code | Runs `./cub-scout` commands directly in terminal |
| Any MCP client | Via `cub-scout mcp serve` gateway (v1.4+) |
| Any CLI-capable agent | Shell execution of `./cub-scout` commands |

### Execution Modes

| Mode | Who runs commands | Best for |
|------|------------------|----------|
| **AI-directed** | Your AI runs everything | Full experience |
| **Pair mode** | You run commands, AI reasons about output | Learning |
| **Demo mode** | Pre-recorded outputs, AI reasons about them | Presentations |

---

## Stage 0: Give Your AI a Toolkit

Before your AI can see, it needs tools.

### Step 1 — Build cub-scout

```bash
cd <repo-root>
go build ./cmd/cub-scout
./cub-scout version
```

### Step 2 — Verify your AI can execute commands

Ask your AI:

> Run `./cub-scout version` and tell me what you see.

If your AI returns a version string, it has hands. Now give it eyes.

### Step 3 — Point at a cluster

**Option A — Your own cluster** (recommended for Argo/Flux operators):

```bash
kubectl config current-context
kubectl get namespaces
```

Your AI will explore YOUR infrastructure. This is the most valuable path.

**Option B — Demo cluster** (if you do not have a cluster available):

```bash
# ArgoCD demo (ArgoCD + Helm + Native, ~5 min)
cd examples/argo-import-confighub-demo && ./demo.sh --keep && cd <repo-root>

# OR Flux demo (Flux + Helm + Native, ~5 min)
cd examples/flux-import-confighub-demo && ./demo.sh --keep && cd <repo-root>
```

### Success condition

Your AI can run `./cub-scout version` and `kubectl config current-context`.

### Passphrase

**"My AI has hands. Now give it eyes."**

---

## Stage 1: "Map My Cluster"

Give your AI its first look at the cluster.

### Ask your AI

> Use cub-scout to map this cluster. Tell me: how many resources are
> there, who owns them, and what ownership patterns do you see? Are
> there any surprises?

### What to watch for

Your AI should run something like:

```bash
./cub-scout map list
./cub-scout map list --json | jq -r '.[].owner' | sort | uniq -c | sort -rn
./cub-scout map list --json | jq 'length'
```

Then it should *reason*:

- "I see N resources across M namespaces."
- "Ownership is split between ArgoCD (X), Helm (Y), and Native (Z)."
- "The Native count is high — these resources have no GitOps owner."

### The reveal

**For Argo operators:** Your AI sees ownership that ArgoCD's UI cannot
show you. ArgoCD knows about *its* Applications. It does not know about
the Helm releases next door, or the Native resources nobody claimed.
cub-scout shows the full picture — every owner, every resource, one view.

**For everyone:** Your AI just did in 10 seconds what takes a human
20 minutes of `kubectl get` commands across namespaces.

### Follow-up prompt (optional)

> Show me just the unmanaged resources. Why might they exist?

```bash
./cub-scout map orphans
```

Watch your AI reason about why resources might be unmanaged: leftover
from a migration, system components, or forgotten manual deployments.

### Passphrase

**"My AI sees the whole board."**

---

## Stage 2: "Trace the Lineage"

Ownership labels tell you WHO. Lineage tells you HOW and WHERE FROM.

### Ask your AI

> Pick three workloads with different owners and trace each one back to
> its source. Compare the lineage chains. What is different about how
> each tool manages its resources?

### What to watch for

Your AI should run traces for different ownership types:

```bash
# Trace an ArgoCD-managed workload
./cub-scout trace deploy/<name> -n <namespace> --explain

# Trace a Helm-managed workload
./cub-scout trace deploy/<name> -n <namespace> --explain

# Trace an unmanaged workload
./cub-scout trace deploy/<name> -n <namespace> --explain
```

Then it should *compare*:

- "The ArgoCD workload has a complete chain: Git repo → Application → ReplicaSet → Pod."
- "The Helm workload shows the chart source but not the original Git repo."
- "The Native workload has no chain at all — someone `kubectl apply`'d it directly."

### The reveal (Argo operators)

If your cluster uses **App-of-Apps** or **ApplicationSets**, ask:

> Can you show me the full Application hierarchy? Not just one app —
> the parent-child structure.

```bash
./cub-scout tree
```

Your AI sees what ArgoCD's UI shows one application at a time:
the full hierarchy from ApplicationSet generators through generated
Applications down to workloads. cub-scout renders this as a single
tree. At scale (50+ Applications), this is the only way to see the
shape of your Argo deployment.

### Follow-up prompt (Argo-specific)

> Are there any lifecycle hazards? Helm hooks that might break under
> ArgoCD sync?

```bash
./cub-scout scan --lifecycle-hazards
./cub-scout map hooks
```

Watch your AI find Helm `pre-install` hooks that map to ArgoCD `PreSync`
phases — and flag the ones that could cause sync failures. ArgoCD does
not warn about this. cub-scout does.

### Passphrase

**"My AI reads lineage, not just labels."**

---

## Stage 3: "What's Broken?"

Your AI can see structure. Now make it diagnose.

### Ask your AI

> Check the health of every GitOps pipeline on this cluster. For
> anything that is failing, explain why and suggest what an operator
> should investigate. Generate a structured report I could paste
> into a Slack channel.

### What to watch for

Your AI should run:

```bash
./cub-scout gitops status
./cub-scout gitops status --json
./cub-scout scan
```

Then it should *diagnose*:

- "2 deployers are healthy, 3 are failing."
- "The failing deployers reference source repos that do not resolve —
  this could be a DNS issue, a private repo without credentials, or
  stale configuration."
- "The scan found 2 policy violations: a deployment without resource
  limits and a container running as root."

And it should *format* — a structured report with sections, not a wall
of text.

### The reveal

Watch the difference between what `kubectl` gives you and what your AI
produces. `kubectl get applications -A` shows sync status. Your AI,
using cub-scout, explains *why* something is failing, correlates it with
scan findings, and produces a human-readable incident summary.

This is the AI reasoning layer. cub-scout provides the evidence.
The AI provides the judgment.

### Follow-up prompt

> Capture everything you found as an evidence bundle I can attach to
> an issue.

```bash
./cub-scout bundle create --output /tmp/cluster-evidence.tar.gz
./cub-scout bundle summarize --output /tmp/cluster-summary.md
```

Your AI just created a portable, timestamped evidence package.
Attach it to a GitHub issue, a Jira ticket, or a Slack thread.

### Passphrase

**"My AI diagnoses, not just describes."**

---

## Intermission: The Wall

Stop. Look at what your AI can do now.

| Capability | How |
|-----------|-----|
| See every resource and its owner | `map list` |
| Trace lineage back to Git | `trace --explain` |
| Visualize full Application hierarchy | `tree` |
| Find lifecycle hazards and policy violations | `scan` |
| Diagnose pipeline health | `gitops status` |
| Generate structured reports | AI reasoning |
| Capture portable evidence | `bundle create` |

Your AI is now a **read-only cluster expert**. It can see everything
that exists *right now*.

But there are questions it cannot answer.

---

## Stage 4: "What Changed?"

### Ask your AI

> What changed in this cluster in the last week? Who made changes and
> why? Is this cluster different from our other production clusters?
> If I change the shared database config, what would be affected?

### What happens

Your AI tries. It runs the tools it knows:

```bash
./cub-scout map list        # Shows current state
./cub-scout trace ...       # Shows current lineage
./cub-scout gitops status   # Shows current health
```

And then it tells you the truth:

> "I can see what EXISTS right now. But I cannot see what CHANGED —
> the cluster API does not store history. I cannot compare this cluster
> to your other clusters — I can only see this one. And I cannot predict
> impact — I do not have a dependency graph across environments."

This is the wall. It is real. It is not artificial. The Kubernetes API
is a snapshot of *now*. It has no memory. It has no fleet context. It
has no intent model.

### What your AI needs

| Question | What's missing |
|----------|---------------|
| "What changed this week?" | **Change history** — the cluster API does not store this |
| "Is this cluster different?" | **Fleet context** — the API only sees one cluster |
| "What's the blast radius?" | **Dependency graph** — the API has no cross-environment model |
| "What SHOULD be running?" | **Intent** — the API shows observed state, not desired state |

These are not cub-scout limitations. They are cluster API limitations.
No read-only tool can answer these from the cluster alone.

### The insight

Your AI is already more capable than most human operators at reading
cluster state. But the questions that MATTER for operations — what
changed, what should be, what's the impact — require data that lives
outside the cluster.

That data lives in **ConfigHub**.

### Passphrase

**"Sight without memory is not enough."**

---

## Stage 5: The Upgrade

Connect ConfigHub. Watch your AI's capabilities transform.

### What ConfigHub adds to the MCP surface

ConfigHub is the MCP server. It owns the full loop:

```
AI chat → MCP → ConfigHub API → trigger changes → GUI updates display
```

When your AI connects through cub-scout's MCP gateway, its tool surface
expands from cluster observation to full operational intelligence:

| Tool surface | Standalone | + ConfigHub |
|-------------|-----------|-------------|
| Current state | map, trace, scan | same |
| Change history | -- | ChangeSets with who/when/why |
| Intent (DRY/WET) | -- | Source intent vs rendered manifest |
| Comparison | -- | DRY vs WET vs LIVE three-way diff |
| Fleet context | -- | Cross-cluster comparison |
| Impact analysis | -- | Dependency graph + blast radius |
| Audit trail | -- | Break-glass decisions with evidence |

### Step 1 — Connect

```bash
# Install the cub CLI (if not already installed)
# https://confighub.com/docs/cli

cub auth login
./cub-scout status
```

If `./cub-scout status` shows `connected` — your AI just gained memory,
intent, and fleet awareness.

**If you do not have a ConfigHub account:** You have already earned
**Agent of the Observable Realm**. The wall you hit is real. Come back
when you are ready to remove it. Everything your AI learned in Act 1
still works.

### Step 2 — Import your cluster (if not already imported)

```bash
# Preview what ConfigHub would structure (still read-only)
./cub-scout import --dry-run

# Import (requires confirmation)
./cub-scout import -y
```

### Step 3 — Ask the same questions again

Now return to the questions from Stage 4. Your AI can answer them.

> What changed in this cluster in the last week?

Your AI queries ConfigHub ChangeSets and reports:

- "Three changes in the last 7 days."
- "Tuesday: ci-bot rolled the API image from v1.4.2 to v1.4.3."
- "Monday: sarah manually scaled the worker from 3 to 1 replicas — this
  might be intentional or might be a forgotten debugging change."
- "Last Friday: the shared-config ConfigMap was updated with new
  database connection pool settings."

> Is this cluster consistent with production?

Your AI queries the fleet and reports:

- "us-west-2 is running API v1.4.1 — two versions behind the fleet
  norm of v1.4.3. us-east-1 and eu-west-1 are consistent."
- "The worker service has 1 replica in us-west-2 but 3 in all other
  clusters — this confirms the manual scaling from Monday."

> What is the blast radius if I change the shared database config?

Your AI queries the dependency graph and reports:

- "4 services depend on shared-db-config across 2 environments."
- "api-gateway, worker-service, auth-service, and cron-jobs would all
  be affected in production and staging."
- "Risk: HIGH — this touches production workloads."

### The reveal

Your AI went from "I can see what exists" to:

- **Temporal reasoning:** what changed, when, and by whom
- **Spatial reasoning:** how this cluster compares to the fleet
- **Causal reasoning:** what would happen if you changed something

The same AI. The same cub-scout. The only difference: ConfigHub as a
data resource behind the MCP gateway.

This is not a feature demo. This is the difference between a cluster
tool and an operational intelligence platform.

### Passphrase

**"My AI remembers. My AI compares. My AI predicts."**

---

## Stage 6: The Full Report

### Ask your AI

> Generate a complete cluster intelligence report. Include everything
> you can observe (standalone) AND everything you know from ConfigHub
> (connected). Make it clear which insights come from each source.

### What your AI produces

```
=== CLUSTER INTELLIGENCE REPORT ===

Generated by: [AI tool name]
Cluster:      [kubectl config current-context]
cub-scout:    [version]
Mode:         [standalone | connected]
Date:         [timestamp]

━━━ OBSERVABLE STATE (cub-scout standalone) ━━━

Ownership Breakdown:
  ArgoCD:  12 resources (3 Applications, 9 workloads)
  Helm:     4 resources (2 releases)
  Native:  47 resources (unmanaged)

Lineage:
  Complete chains:  8 workloads (traceable to Git source)
  Partial chains:   4 workloads (ownership known, source unresolved)
  No chain:        47 resources (Native / unmanaged)

Pipeline Health:
  Backend: ArgoCD
  Healthy: 2 deployers
  Failing: 1 deployer (source repo unreachable)

Risk Findings:
  2 policy violations (missing resource limits, privileged container)
  1 lifecycle hazard (Helm pre-install hook under ArgoCD sync)

━━━ OPERATIONAL INTELLIGENCE (ConfigHub connected) ━━━

Change History (7 days):
  3 changes across 2 services
  1 automated (ci-bot), 1 manual (sarah), 1 config update

Fleet Consistency:
  1/3 production clusters divergent (us-west-2)
  Image version drift: 2 versions behind
  Replica count anomaly: manual scaling detected

Intent vs Reality:
  DRY intent: 3 replicas, image v1.4.3
  LIVE state: 1 replica, image v1.4.1
  Drift: YES — manual changes not reflected in source

Impact Surface:
  shared-db-config: 4 dependent services, 2 environments
  Next change risk: HIGH

━━━ ASSESSMENT ━━━

What is working:    [AI assessment]
What needs attention: [AI assessment]
Recommended actions:  [AI assessment]

Evidence bundle: /tmp/cluster-evidence.tar.gz
```

### The insight

Look at the two sections. The standalone section tells you WHAT.
The connected section tells you WHY, WHEN, WHERE ELSE, and WHAT IF.

Your AI produced both. But only one requires ConfigHub.
That difference is the value of connected mode.

### Passphrase

**"Evidence over opinion. Intelligence over observation."**

---

## Bonus: The Argo Deep Dive

If your cluster runs ArgoCD, ask your AI these Argo-specific questions.
Each reveals something ArgoCD's own UI cannot show:

### 1. "Show me the full Application hierarchy"

```bash
./cub-scout tree
```

ArgoCD shows one Application's resource tree at a time. cub-scout
renders the complete hierarchy: ApplicationSets, generated Applications,
and all their workloads in a single tree view.

### 2. "Which Applications came from ApplicationSets?"

```bash
./cub-scout map list --json | jq '[.[] | select(.owner == "ArgoCD")] | group_by(.source) | .[] | {source: .[0].source, count: length}'
```

See the generator-to-Application relationship that ApplicationSets
create. At scale, this is the only way to understand your deployment
topology.

### 3. "What is Helm doing under ArgoCD?"

```bash
./cub-scout scan --lifecycle-hazards
./cub-scout map hooks
```

Helm hooks (pre-install, post-install) map to ArgoCD sync phases
(PreSync, PostSync). Some mappings cause sync failures. cub-scout
detects these hazards. ArgoCD does not warn about them.

### 4. "Show me everything ArgoCD does NOT manage"

```bash
./cub-scout map orphans
./cub-scout map list --json | jq '[.[] | select(.owner == "Native")] | group_by(.namespace) | .[] | {namespace: .[0].namespace, count: length}'
```

ArgoCD knows about its Applications. It does not know about the
resources nobody claimed. These are your operational blind spots.

---

## Cleanup

```bash
# If you used a demo cluster
kind delete cluster --name argo-import-demo    # ArgoCD path
kind delete cluster --name flux-import-demo    # Flux path
```

---

## Quest Progression

| Stage | Title | What your AI gains | Requires |
|-------|-------|-------------------|----------|
| 0 | Give Your AI a Toolkit | Command execution | cub-scout binary |
| 1 | Map My Cluster | Ownership vision | Cluster access |
| 2 | Trace the Lineage | Source awareness | -- |
| 3 | What's Broken? | Diagnostic reasoning | -- |
| 4 | What Changed? | Awareness of its own limits | -- |
| 5 | The Upgrade | Memory, intent, fleet, impact | ConfigHub |
| 6 | The Full Report | Operational intelligence | ConfigHub |

### Titles

- **Agent of the Observable Realm** — Complete Stages 0–4
- **Agent of Intent and Evidence** — Complete all stages

---

## Designer Notes

- **AI-first, human-directed.** The human asks questions. The AI
  operates. The value is in the reasoning, not the commands.
- **The wall is real.** Stage 4 is not artificial scarcity. The cluster
  API genuinely cannot answer temporal, fleet, or intent questions. This
  is the honest argument for ConfigHub.
- **Argo features are woven in, not separated.** Argo operators see
  their specific reveals within the universal quest flow, plus a
  dedicated deep-dive section.
- **ConfigHub is a data resource, not a product pitch.** The quest shows
  what the AI can DO with ConfigHub data. The pitch is implicit: if you
  want your AI to answer these questions, it needs this data.
- **MCP architecture.** ConfigHub is the MCP server (full read-write
  loop). cub-scout provides the read-only MCP gateway. In standalone
  mode, the AI gets observation tools. In connected mode, it gets the
  full ConfigHub API surface through the same gateway.
- **CLI/TUI parity.** Every command shown here works in both CLI and TUI
  mode. The quest uses CLI for scriptability, but the same exploration
  works interactively in the TUI.
- **Every command exists today** (Act 1). Act 2 uses connected mode
  commands that are available when ConfigHub is connected. The MCP
  gateway (v1.4) and history/compare/fleet commands (v1.6) will make
  Act 2 smoother but the import/status/dry-run commands work now.

### Related Quests

- [The Missing Ownership Maze](../new-user-puzzle-quest/) — The original
  step-by-step quest for first-time operators. Start here if you want
  to learn cub-scout commands before giving them to your AI.
