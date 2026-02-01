# Why Connect cub-scout to ConfigHub (and Why It's Worth Paying For)

---

## Overview

**cub-scout** is a free, read-only **GitOps explorer and debugger**.

On its own, cub-scout helps you understand:
- What is running in your cluster right now
- How resources were applied (Flux, Argo, Helm)
- Why something is broken
- Whether GitOps discipline has been violated

However, there are hard limits to what any tool can learn from a single cluster's APIs.

**Connected Mode** exists to go beyond those limits by integrating cub-scout with **ConfigHub**, the system of record for configuration intent, history, and fleets.

---

## The Core Distinction

**Standalone cub-scout answers:**

> *"What exists right now, and why?"*

**Connected cub-scout answers:**

> *"What should exist, what changed, across which environments — and what happens next?"*

Both modes are valuable. They solve different problems.

---

## What You Get Without Connecting (Always Free)

Standalone cub-scout requires only Kubernetes API access and GitOps CRDs.

It provides:

- Single-cluster exploration
- Ownership and provenance tracing
- Delegated apply visibility (Flux / Argo via OCI)
- Failure-stage explanation (source vs apply/reconcile)
- Drift detection using GitOps controller signals
- Guided GitOps debugging
- Exportable ownership/dependency graphs (JSON, DOT)
- Shareable, sanitized diagnostic snapshots
- Full CLI and TUI functionality, including `:` shell-out

No account required.
No network dependency.
No mutation of cluster state.

---

## Why Standalone Mode Has Limits

A Kubernetes cluster can only show **current reality**.

It cannot reliably answer:
- What *should* exist
- What changed last week
- Whether this cluster is an outlier
- What will be affected by an upcoming change
- How intent differs across environments

Solving those problems requires a **system of record outside the cluster**.

---

## What Connecting to ConfigHub Unlocks

### 1. Intent Awareness

Understand what *should* exist, not just what does.

- ConfigHub spaces, targets, and revisions
- Declarative ownership definitions
- Intended vs actual comparisons

ConfigHub treats configuration as **first-class data**, rather than scattered files and templates.

---

### 2. History & Time

Answer questions clusters cannot.

- "When did this break?"
- "What changed since it last worked?"
- Correlated timelines across config revisions and reconciliations

History requires durable storage and indexing beyond the cluster.

---

### 3. Fleet & Multi-Cluster Views

See the bigger picture.

- Fleet-wide health
- Cross-cluster comparisons
- Version skew and rollout state
- Outlier detection ("this cluster is the weird one")

These insights cannot be inferred from a single cluster in isolation.

---

### 4. Impact Analysis (Before You Change Things)

Understand consequences ahead of time.

- Blast radius analysis
- Downstream dependency impact
- Cross-environment effects

This requires knowing intent, scope, and relationships across environments.

---

### 5. Git-Aware Navigation

Bridge runtime and source control.

- Repo, branch, path, and commit context
- Commit authorship and messages
- Navigation from resource → intent source

cub-scout remains read-only; ConfigHub manages source access securely.

---

### 6. Governance & Policy Context

Surface governance information without enforcing it.

- Policy evaluation results
- Approval and gate context
- Compliance signals

cub-scout explains outcomes; ConfigHub owns governance.

---

### 7. Smarter Debugging & CLI Guidance

Make the right next step obvious.

- Context-aware command suggestions
- Richer explanations in the TUI
- Better command completion using intent metadata

This builds on cub-scout's existing role as a guided debug shell.

---

## Why Connected Mode Is a Paid Feature

Connected Mode depends on capabilities that cannot exist locally:

- Durable storage of intent and history
- Secure, multi-tenant APIs
- Cross-cluster aggregation
- Indexing and query infrastructure
- Governance and policy engines

A paid ConfigHub subscription funds:
- The ConfigHub control plane
- Secure authentication and authorization
- Ongoing data retention and analysis
- Support for teams operating at scale

cub-scout itself remains free, read-only, and safe.

---

## What Connected Mode Does *Not* Change

Even when connected:

- cub-scout never applies changes
- cub-scout never enforces policy
- cub-scout never becomes a system of record
- cub-scout always degrades gracefully when disconnected

Connected Mode **adds context**, not control.

---

## Summary

cub-scout alone helps you understand **what is happening**.

cub-scout connected to ConfigHub helps you understand:
- what *should* be happening
- what *changed*
- what *will be affected next*
- across *all* your environments

This is how teams reduce configuration **sprawl** and operate complex systems with confidence.
