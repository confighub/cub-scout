# App GitOps + AI GitOps (Plain English)

## The Short Version

`post Flux / post Argo` does **not** mean "rip out Flux and Argo tomorrow."

It means:

1. Flux/Argo stay as useful deployment engines.
2. Decision authority moves to a higher control layer.
3. Every AI-assisted change is recorded as a governed transaction.

## Why This Matters

Today, Git + Flux/Argo answer:

1. What changed?
2. Did it sync?

Teams still struggle to answer:

1. Why was this change proposed?
2. Why was it allowed?
3. Who/what executed it?
4. What risk checks ran before and after apply?

AI makes this gap bigger because more changes happen faster.

## What "Checkpoint but for GitOps" Means

A normal AI checkpoint is a session log.

A GitOps checkpoint is a **change ledger card**:

`intent + evidence + decision + execution + outcome`

So the unit is not "chat history."  
The unit is "governed mutation record."

## App GitOps vs AI GitOps

### App GitOps

Focus: app-level intent across environments.

Examples:

1. "Roll payment API from v1.3.1 to v1.3.2 in staging and prod."
2. "Add rate limit policy for checkout service."

### AI GitOps

Focus: using agents to propose and prepare those changes safely.

Examples:

1. Agent proposes manifests and policy deltas.
2. System decides `ALLOW|ESCALATE|BLOCK` by trust tier.
3. Execution is tokened and attested.

## Where Each Component Fits

1. `cub-scout`: explorer + evidence normalizer
2. `confighub-scan`: risk/policy signals
3. `confighub`: decision and attestation authority
4. `confighub-actions`: tokened execution runtime

## Why a Flux User Would Care

They keep their existing Flux flow, but gain:

1. Better review context ("why this change")
2. Better incident forensics ("why was this allowed")
3. Better AI handoffs (no context reset every session)
4. Governance-ready records for production changes

## What Changes in Practice

Before:

`commit -> Flux sync -> maybe alert`

After:

`commit -> intent card -> pre-scan -> decision -> tokened apply -> post-scan -> outcome card`

This is the core of "post Flux/post Argo":
controllers remain useful, but they are not the policy authority.

## First Adoption Path

1. Start read-only: capture intent/evidence cards (Tier 0)
2. Enable low-risk tokened apply (Tier 1)
3. Add approval gates for medium/high risk (Tier 2/3)

That gives teams a gradual path, not a rewrite.

