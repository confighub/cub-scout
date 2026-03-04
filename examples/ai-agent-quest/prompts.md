# AI Agent Quest — Prompt Reference

Copy-paste prompts for each stage. Use with Claude Code, Copilot, Cursor,
or any CLI-capable AI assistant.

> **Prerequisite:** Your AI must be able to run `./cub-scout` commands.
> Test with: "Run `./cub-scout version` and tell me what you see."

---

## Stage 1: Map My Cluster

### Basic

> Use cub-scout to map this cluster. Tell me: how many resources are
> there, who owns them, and what ownership patterns do you see? Are
> there any surprises?

### Argo-focused

> Map this cluster with cub-scout. I'm running ArgoCD — show me
> everything ArgoCD manages, everything Helm manages, and everything
> that nobody manages. Which category is largest and why?

### Fleet-focused

> Map this cluster and tell me: is this a well-managed cluster or a
> messy one? What percentage of resources have a GitOps owner? What
> would a platform team need to clean up?

---

## Stage 2: Trace the Lineage

### Basic

> Pick three workloads with different owners and trace each one back to
> its source. Compare the lineage chains. What is different about how
> each tool manages its resources?

### Argo deep-dive

> Show me the full ArgoCD Application hierarchy with `cub-scout tree`.
> Then trace one Application and explain the chain from Git repo to
> running pod. Are there any ApplicationSets generating Applications?

### Ownership comparison

> Trace an ArgoCD workload, a Helm workload, and a Native workload.
> For each one, explain: where did it come from? Can it be
> automatically reconciled if someone modifies it? What happens if
> the source disappears?

---

## Stage 3: What's Broken?

### Basic

> Check the health of every GitOps pipeline on this cluster. For
> anything that is failing, explain why and suggest what an operator
> should investigate. Generate a structured report I could paste
> into a Slack channel.

### Incident mode

> Act as an SRE. Run a full diagnostic: pipeline health, risk scan,
> and lifecycle hazards. Prioritize findings by severity. For each
> finding, state the impact and the remediation. Produce an incident
> summary with evidence.

### Evidence capture

> Run a full diagnostic and capture everything as an evidence bundle.
> Then summarize the bundle in a format I can attach to a GitHub issue.
> Include: cluster context, ownership breakdown, pipeline health,
> and top findings.

---

## Stage 4: The Wall

### The questions that break

> What changed in this cluster in the last week? Who made changes and
> why? Is this cluster different from our other production clusters?
> If I change the shared database config, what would be affected?

### Forcing the admission

> I need you to tell me: what CAN'T you answer about this cluster
> right now? What data would you need to answer those questions? Where
> would that data come from?

---

## Stage 5: The Upgrade

### After connecting ConfigHub

> Now that ConfigHub is connected, answer the questions from Stage 4
> again. What changed this week? Is this cluster consistent with the
> fleet? What is the blast radius of changing the shared config?

### The comparison

> Generate two reports side by side: what you could tell me BEFORE
> ConfigHub, and what you can tell me AFTER. Make the difference
> obvious.

---

## Stage 6: Full Report

### The capstone

> Generate a complete cluster intelligence report. Include everything
> you can observe standalone AND everything you know from ConfigHub.
> Make it clear which insights come from each source. Structure it
> so a platform team lead can read the executive summary in 30 seconds
> and drill into details as needed.
