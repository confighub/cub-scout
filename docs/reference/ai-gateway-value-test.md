# AI Gateway Value Test

This document defines how to judge whether `cub-scout`'s MCP gateway and AI-facing routing surfaces add real value.

The bar is not "an AI can call tools."

The bar is:

1. the AI picks the right tool first more often
2. the AI reaches exact proof with fewer wrong turns
3. the AI stops earlier when the evidence is complete
4. the human rescues the session less often

## Terms

### MCP gateway

`cub-scout mcp serve` exposes read-only tools with stable names, parameters, and JSON outputs.

Its main value is:

1. remove shell and flag drift
2. give the AI exact tool surfaces
3. preserve deterministic JSON contracts

### AI gateway

This is the higher-level "which tool, in what order, for what reason?" layer.

In `cub-scout` today it is spread across:

1. MCP tool descriptions
2. structured next-step hints
3. command framing such as `doctor`, `explain`, and `trace`
4. repo skills and AI-facing docs

Its main value is:

1. better first-tool routing
2. better workflow chaining
3. better stop behavior

## Main User Scenarios

These are the highest-value scenarios for `cub-scout`'s MCP and AI gateway surfaces.

### 1. Cold-start troubleshooting

User asks:

- "what's wrong?"
- "what's broken in prod?"
- "should I start with cub-scout or kubectl?"
- "kubectl cannot reach the cluster after restart; is the cluster broken or is my access broken?"

Expected first move:

- `doctor`

Why the gateway matters:

1. gives one safe read-only entrypoint
2. returns computed health, drift, ownership, and next steps
3. reduces aimless first moves like broad `kubectl get` or raw Argo UI wandering

### 2. Cluster or namespace inventory

User asks:

- "what's running in this cluster?"
- "show me what is deployed in prod"

Expected first move:

- `map`

Why the gateway matters:

1. turns raw inventory into ownership-aware inventory
2. makes the AI reach for a scoped list instead of broad ad hoc shell commands

### 3. Resource ownership and meaning

User asks:

- "who owns this resource?"
- "what is this deployment and why is it here?"

Expected first move:

- `explain`

Why the gateway matters:

1. gives computed ownership, health, drift, events, and next steps
2. is more truthful for AI routing than raw `kubectl describe`

### 4. Provenance and source chain

User asks:

- "where did this resource come from?"
- "what GitOps chain produced this?"

Expected first move:

- `trace`

Why the gateway matters:

1. gives exact end-to-end source and deployer chain
2. reduces guesswork about Helm, Argo, Flux, or native ownership

### 5. Governed state versus live state

User asks:

- "compare governed state to live state"
- "do ConfigHub, the deployer, and the cluster agree?"
- "would you sign off on this change?"

Expected first move:

- `compare_three_way` in connected mode

Why the gateway matters:

1. gives a direct proof surface for convergence
2. prevents the AI from faking this answer with a partial live-only view

### 6. Connected unit lookup and governed details

User asks:

- "which ConfigHub unit corresponds to this resource?"
- "show me the governed unit details"
- "what is the first useful ConfigHub object I should open for this resource?"

Expected first moves:

1. `confighub_units` after cluster-side scope is known
2. `confighub_unit_get` only after the unit slug is known

Why the gateway matters:

1. enforces preconditions in the tool descriptions
2. keeps the AI from opening governed-state tools too early

### 7. Governed change history and receipts

User asks:

- "what changed?"
- "who changed it?"
- "show me the governed receipt"

Expected first move:

- `confighub_changesets`

Why the gateway matters:

1. gives a distinct connected history surface
2. avoids confusing current cluster health with durable change history

## What Good Looks Like

The gateway is adding real value when these things improve compared with shell-only operation:

1. first-tool accuracy
2. fewer wrong-tool detours
3. fewer bad command-shape guesses
4. faster arrival at proof surfaces
5. fewer human rescues
6. better stop behavior after proof is reached

## Core Metrics

Use these metrics for live evaluations and release reviews.

### 1. First-tool accuracy

For a fixed prompt set, did the AI choose the right first tool?

Track:

- correct
- acceptable but slower
- wrong

### 2. Tool-hop count

How many tool calls did it take to reach the first truthful proof surface?

Lower is better if proof quality stays high.

### 3. Rescue rate

How often did a human need to say things like:

- "start with doctor"
- "you need explain, not map"
- "compare governed state, not just live state"

### 4. Command-shape drift

How often did the AI invent or misuse shell flags or command forms?

This is where MCP should outperform shell mode sharply.

### 5. Proof quality

Did the session end with one of the intended proof surfaces?

Examples:

1. `doctor` JSON for first-pass scope
2. `explain` JSON for ownership and next steps
3. `trace` JSON for provenance
4. `compare_three_way` JSON for convergence
5. `confighub_changesets` JSON for governed receipts

### 6. Stop quality

Did the AI stop when the proof was sufficient, or did it keep wandering?

## Cold-Test Harness

Use the fixed prompt set in:

- [AI Gateway Cold-Test Prompts](../../examples/ai-integration/ai-gateway-cold-test-prompts.md)

Run the same prompt set in two modes:

1. shell-first mode
2. MCP-enabled mode

The model, repo state, cluster/context, and prompt text should stay the same.

### Suggested procedure

1. Start a fresh AI session.
2. Run the prompt set in shell-first mode.
3. Score each prompt using the rubric below.
4. Start a second fresh AI session.
5. Run the same prompt set with `cub-scout mcp serve` enabled.
6. Score again.
7. Compare the two runs.

### Scoring rubric

For each prompt, record:

1. first tool chosen
2. whether that first tool was correct
3. number of tool hops before proof
4. whether a human rescue was needed
5. proof surface reached
6. whether the AI stopped correctly

### Pass bar

For the MCP-enabled run, a strong result looks like:

1. at least 80% correct first-tool choice
2. fewer rescues than shell-first mode
3. fewer command-shape mistakes than shell-first mode
4. at least one exact proof surface reached for every scoped prompt
5. no false closeout when convergence has not been proven

## Interpretation

### If MCP wins on first-tool choice and drift

The gateway is already paying for itself as an execution surface.

### If MCP wins on proof quality but not stop quality

The next product work is not more tools. It is better stop semantics and next-step guidance.

### If MCP does not beat shell mode

Likely causes:

1. tool descriptions are still not sharp enough
2. the wrong proof surface is missing from MCP
3. the output facts are not computed enough to guide the next step

## Current Read (April 2026)

As of the post-`#377` follow-up:

1. `doctor` is the first troubleshooting and tool-choice entrypoint
2. local-access uncertainty such as wrong context, stale kubeconfig, or API reachability is now explicitly part of the `doctor` routing story
3. `map`, `explain`, and `trace` have sharper chain boundaries
4. connected `compare_three_way` now covers both governed-vs-live convergence and sign-off-readiness intent directly
5. connected lookup tools now say more clearly when to find the first useful ConfigHub object and when to open exact unit facts
6. MCP tools now advertise `annotations.readOnlyHint=true`, making the read-only trust boundary machine-visible as well as human-described
7. the biggest remaining AI value work is not basic attraction, but broader CLI/docs cleanup and continued workflow simplification
