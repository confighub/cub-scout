# New User Onboarding Philosophy

> **Archive status:** Historical planning context (non-canonical for current releases).
> For current `cub-scout` behavior and command contracts, use:
> - `docs/roadmap.md`
> - `docs/reference/commands.md`
> - `docs/reference/import-docs-crosswalk.md`
>
> Scope note: examples here that use mutating `cub` workflows (`drift accept`, `mutate`, `promote`, `changeset`, worker lifecycle) describe ConfigHub platform behavior, not direct `cub-scout` command commitments.

**Status:** Implemented (in confighub-agent)
**Last Updated:** 2026-01-09

> **Note:** This document describes the Map-first onboarding philosophy that has been
> implemented in the confighub-agent. The key principle — "start with visibility, progress
> to control" — is now the standard approach across all documentation and demos.

---

## Current State

### Documentation Structure

```
docs.confighub.com/
├── get-started/
│   ├── setup.md           # CLI install
│   └── tutorial/
│       ├── create.md      # Config Units
│       ├── edit.md        # Making changes
│       ├── deploy.md      # Workers
│       ├── clone.md       # Clone + upgrade
│       ├── validate.md    # Policy guards
│       ├── drift.md       # Drift management
│       └── bulk.md        # Labels, filters, functions
├── background/
│   ├── why-confighub.md   # WET config philosophy
│   └── architecture.md    # Workers, SaaS core
└── guide/
    └── ...                # Reference docs
```

### Examples (github.com/confighub/examples)

```
examples/
├── global-app/              # Multi-region microservices
├── helm-platform-components/ # Helm chart management
└── vm-fleet/                # VM configuration
```

### Problem: Conceptual Gap

The current onboarding assumes users understand:
- Why WET config matters
- How Units relate to each other
- Where Workers fit in the deployment story

Users arrive asking: **"What's running in my clusters?"**

Current answer: "Sign up, install CLI, take tutorial, create Units..."

That's backwards. Users want to **see** before they **create**.

---

## How the New Model Helps

### Entry Point: Map First

```bash
# Current onboarding
kubectl apply -f https://confighub.io/agent.yaml
cub auth login
# ... create spaces, units, workers ...
# ... finally see something

# New onboarding
kubectl apply -f https://confighub.io/agent.yaml
cub map
```

```
CLUSTER     NAMESPACE    KIND          NAME              OWNER
prod-east   default      Deployment    nginx             flux:helmrelease/nginx
prod-east   default      Deployment    mystery-app       unknown
staging     default      Deployment    redis             helm:release/redis

312 units across 3 clusters. 4 unowned.
```

**Users see value in 30 seconds.** No spaces. No units created. Just install Agent, run `cub map`.

### Progressive Discovery

| What User Wants | Command | Requires |
|-----------------|---------|----------|
| "What's running?" | `cub map` | Agent only |
| "What's drifted?" | `cub map --drifted` | Agent only |
| "Who owns this?" | `cub map --owner=unknown` | Agent only |
| "What changed?" | `cub map history --since=1h` | Agent only |
| "I want to fix drift" | `cub apply --drifted` | Bridge or OCI |
| "I want to organize this" | Hub + App Space | Optional |

The tutorial becomes: "Start at layer 1, add layers as needed."

---

## Revised Onboarding Flow

### Phase 1: See Everything (Agent)

```bash
# Install Agent (read-only, zero-risk)
kubectl apply -f https://confighub.io/agent.yaml

# See everything
cub map

# Find problems
cub map --drifted
cub map --owner=unknown

# Understand relationships
cub map relations nginx
```

**Time to value: 30 seconds.**

### Phase 2: Query and Explore (Still Agent)

```bash
# Fleet-wide queries
cub map --query "image contains log4j"
cub map --query "replicas < 3 and namespace=prod"

# History
cub map history --since=1h

# Diff environments
cub map diff staging prod
```

**No write access. No risk. Full visibility.**

### Phase 3: Organize (Hub + App Space)

When user says: "I found unknown resources, want to assign ownership."

```bash
# Create organizational structure
cub hub create platform-team
cub app-space create my-team --hub=platform-team

# Assign units to spaces
cub unit assign mystery-app --space=my-team
```

**Optional. Only when user needs organization.**

### Phase 4: Operate (Bridge or OCI)

When user says: "I want to fix this drift."

```bash
# Install Bridge (or configure OCI + Argo/Flux)
cub worker create my-worker --space=my-team
cub worker install my-worker --export > worker.yaml
kubectl apply -f worker.yaml

# Now operations work
cub apply --drifted
cub rollback nginx --to=2h-ago
```

**Only when user needs to make changes.**

---

## Documentation Restructure

### Current Structure

```
Get Started → Setup → Tutorial (create, edit, deploy...)
```

### Proposed Structure

```
See Your Fleet
├── Install Agent (30 seconds)
├── Map: What's Running
├── Find Drift
├── Query the Fleet
└── Understand History

Organize (when ready)
├── Hub: Governance Layer
├── App Space: Team Workspace
└── Assign Ownership

Operate (when ready)
├── Deploy via Bridge
├── Deploy via OCI + Argo/Flux
├── Bulk Changes
└── Rollback

Reference
├── CLI Reference
├── API Reference
└── Concept Glossary
```

### Example READMEs

**Before (global-app):**
> This example demonstrates how to use ConfigHub to manage a typical micro-service application deployed in different variants...

**After:**
```markdown
# Global App: From Visibility to Operations

## 1. See What You Have

Install Agent and run:
```bash
cub map --cluster=global-app-cluster
```

You'll see all deployments, their owners, and any drift.

## 2. Find Problems

```bash
# What's drifted?
cub map --drifted

# What's unmanaged?
cub map --owner=unknown
```

## 3. Organize (Optional)

Create Hub and App Space when you want to assign ownership:
```bash
cub hub create acme-platform
cub app-space create frontend-team --hub=acme-platform
```

## 4. Operate (Optional)

Install Bridge when you want to make changes:
```bash
cub worker install frontend-worker --export > worker.yaml
```
```

---

## Concept Mapping for New Users

### Before: Abstract First

```
Why ConfigHub? → WET Config Philosophy → Architecture → Tutorial
```

Users must understand philosophy before seeing anything.

### After: Concrete First

```
Install → See → Query → (Optional: Organize → Operate)
```

Users see their actual clusters, then learn concepts as needed.

### New Concept Glossary

| Concept | When You Need It | Analogy |
|---------|------------------|---------|
| **Agent** | Immediately | Security camera (watch only) |
| **Map** | Immediately | Fleet inventory + search |
| **Unit** | When querying | Single managed resource |
| **Bridge** | When changing things | Remote control (read + write) |
| **Hub** | When organizing teams | Shared governance layer |
| **App Space** | When organizing teams | Team workspace |

---

## github.com/confighub/examples Updates

### Current Examples

| Example | Focus |
|---------|-------|
| global-app | Multi-region deployment |
| helm-platform-components | Helm chart management |
| vm-fleet | VM configuration |

### Proposed Examples

| Example | Focus | New Model Tie-In |
|---------|-------|------------------|
| **see-your-fleet** | Agent + Map only | Phase 1 onboarding |
| **find-drift** | Drift detection queries | Map value prop |
| **cve-response** | Fleet-wide image query | Saved queries |
| **multi-team** | Hub + App Space setup | Organizational model |
| global-app | Multi-region (existing) | Full workflow |
| helm-platform-components | Helm (existing) | OCI integration |

### New Example: see-your-fleet

```
see-your-fleet/
├── README.md
├── bin/
│   ├── install-agent       # kubectl apply agent
│   ├── explore             # cub map examples
│   └── cleanup
└── scenarios/
    ├── find-orphans.md     # cub map --owner=unknown
    ├── find-drift.md       # cub map --drifted
    └── image-audit.md      # cub map --query "image..."
```

**No spaces. No workers. No units created.** Pure visibility.

---

## Documentation Principles

### 1. Show Before Tell

Every doc page should start with a command and its output, not an explanation.

**Before:**
> A Config Unit is the fundamental building block of ConfigHub...

**After:**
```bash
$ cub map --cluster=prod
CLUSTER  NAMESPACE  KIND        NAME    OWNER
prod     default    Deployment  nginx   flux:hr/nginx
```
> This is the Map. Each row is a Unit.

### 2. Layer by Need

Don't explain Hub governance until user has found orphans and wants to assign ownership.

### 3. Reference, Don't Repeat

Link to concept pages. Don't inline explanations in tutorials.

---

## Summary

| Aspect | Current | Proposed |
|--------|---------|----------|
| Entry point | Sign up, CLI, create things | Install Agent, `cub map` |
| Time to value | Minutes (tutorial) | 30 seconds |
| First concept | Units, Spaces | Map (what's running) |
| Organizational model | Assumed upfront | Introduced when needed |
| Write operations | Part of tutorial | Optional, later phase |
| Example focus | Create things | See things, then create |

**The new model inverts onboarding: see first, organize later.**
