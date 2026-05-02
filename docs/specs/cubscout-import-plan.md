# Cub-scout: Argo and Flux import as managed Components

**Status:** Plan, not yet scoped with engineering
**Date:** 2026-05-02
**Author:** Alexis (with Claude)
**Audience:** Brian, Jesper, Alexis
**Subject repos:** `cub-scout`, `confighub`, `confighub-ai-demo` (peripheral)

---

## What this is

A plan to make cub-scout the onboarding path for customers with existing
Argo CD or Flux installations. The customer's controllers keep running,
their cluster stays as-is, and their workloads come under ConfigHub
governance via a Component → Deployable Variant → Target structure with a
defined controller-takeover step.

The plan is bounded. It does not redesign the world. It names a v1, an
explicit takeover pattern, the surfaces that need to ship, the docs that
need to track shipped capability, and the discipline that keeps cub-scout
and ConfigHub from conflating again.

---

## What just landed (context for the plan)

Three things make this the right moment:

1. **Component/Variant doctrine landed in ConfigHub on 2026-04-30.**
   PR #104 merged. Component → Deployable Variant → Target is now the
   user-visible model. AI Variant is a flavour of Deployable Variant.
   Connection is named as the next graduation candidate.

2. **GitHub issue #2985 closed on the higher-level constructs question
   on 2026-05-01.** The model is settled: Component is the family
   (VariantSet), Variant is the member (ConfigSet), Variant is either a
   Base or a Deployment, Deployment binds to a Target. The Promotions UI
   in v0.1.36 reflects this. Jesper has done the canonical work for the
   ConfighubOps dogfood.

3. **Pattern 1 (OCI + repoint) is demonstrated working.** Jesper set up
   Argo CD as a Component in the Kubara org on 2026-05-01: argocd-base
   plus argocd-kubara deployment Variant, ingress and OIDC, working
   end-to-end. Alexis applied an urgent image-pin change on 2026-05-02
   (`:latest` to `:v3.3.9`), four-layer proof captured. The mechanism
   works for fresh-deployed Argo. The gap is *attaching to existing*
   Argo without redeploy.

---

## Goal

Within an opinionated v1 scope, deliver:

A customer with running Argo CD or running Flux can run a `cub-scout`
command (or sequence) that:

- discovers their existing controller-managed deployment;
- proposes a Component, Variants, and Targets that match the doctrine;
- creates the ConfigHub state to register them as managed;
- publishes rendered output as OCI through ConfigHub;
- repoints the existing controller's source from Git to ConfigHub OCI;
- leaves the customer with the same running workloads, now governed.

After v1, the same customer can apply a reviewed change through ConfigHub
and the existing controller reconciles it from the new OCI source. The
Argo CVE pattern Alexis demonstrated on 2026-05-02 then becomes
applicable to that customer's real production, not just to a fresh
demo cluster.

---

## What's already in cub-scout that this builds on

This is the inventory that makes the plan tractable rather than a
rewrite. Cub-scout has been shipping import-adjacent capability for
a year.

**Discovery (mature, shipping).**
Ownership detection across Argo, Flux, Helm, Terraform, ConfigHub,
Crossplane, kro, native Kubernetes — with deterministic precedence.
`map list`, `map orphans`, `tree ownership`, `trace`. GSF v1
relations: `owns`, `selects`, `mounts`, `references`. Documented in
`docs/concepts/architecture.md` and `docs/reference/gsf-schema.md`.

**Argo and Flux specifically.**
ApplicationSet hierarchy parsing (#363). Application → Workload →
Pod chain tracing. Flux operator interop slice (v0.20). Kubara-specific
debugging guide already exists at `docs/howto/kubara-argo-debugging.md`.

**Comparison and proof.**
`compare three-way` for intent vs render vs observed. Governed reconnect
in `compare three-way` (v1.12). Argo-aware `explain` (v1.12). Conformance
reporting with `--fail-on` for CI gates (#342).

**Connected mode foundations.**
v1.6 shipped WET/LIVE/DRY. v1.13 shipped canonical ConfigHub trust URLs
in `compare`, `trace`, `explain`, MCP, and `history`. Connected
`compare_three_way` MCP tool (#377). Revision-aware hints (#370).

**Existing import path.**
`cub-scout import -n <namespace>` already exists. There is a
canonical-import doc at `docs/howto/import-to-confighub.md`. Conformance
import (#342) shipped in v1.9. ApplicationSet git-generator parsing
(#363) shipped. The `cub-scout import` command performs connected
delegation for Argo/Flux when available. Bundle-based import
(`--from-bundle`) shipped.

**Key honest observation.**
The current `cub-scout import` produces ConfigHub Apps/Deployments using
the *previous* App-centric vocabulary. The glossary at
`docs/reference/glossary.md` flags the transition explicitly: "The
Hub/AppSpace model is being replaced by App/Deployment/Target." Now
that the Component/Variant/Base/Deployment doctrine is settled, the
import path needs to produce output that matches the new model.

---

## Where the gap is

Three real gaps separate today's import path from the v1 goal.

**Gap 1: vocabulary alignment with the new doctrine.**
The import command produces Apps/Deployments. The doctrine says
Components/Deployable Variants. The mapping is:

- App → Component
- Deployment → Deployable Variant
- App's "base template" → Base Variant
- Target → Target (unchanged)

This is mostly a rename and a small structural adjustment. Spaces with
`Labels.Variant` per the v0.1.36 implementation. Not architecturally
hard, but needs explicit migration in the import code path, the JSON
output schema, and the docs.

**Gap 2: the takeover step (Pattern 1 done as a first-class flow).**
Today, `cub-scout import` creates ConfigHub state and (per the docs)
recommends keeping the existing deployer during validation, then "cut
over by policy, not accident". That's a manual handoff. For the v1 goal,
we need an explicit, automatable takeover step:

- ConfigHub publishes rendered output as OCI;
- the existing Argo Application's `spec.source` is updated from Git
  to ConfigHub OCI;
- the existing Flux Kustomization's source ref is similarly updated;
- the controller continues reconciling, now from a ConfigHub-governed
  source.

This needs: an OCI publish path that exactly matches what Argo or Flux
was previously rendering (so the takeover is byte-equivalent and no
reconciliation churn happens at takeover), and a CLI command that
performs the source repoint.

The byte-equivalence requirement is the hard part. If the OCI render
differs from the controller's previous render — even cosmetically,
e.g. field ordering or whitespace — Argo/Flux will see drift at
takeover. The v1 design needs to either guarantee byte-equivalence, or
provide a one-time "accept the new normal" reconcile step that is
explicit and proof-bearing.

**Gap 3: Connection extraction as a side-effect of import.**
Cub-scout's `trace` and GSF relations already surface what a workload
depends on (Secrets, ConfigMaps, ServiceAccounts, ImagePullSecrets,
PersistentVolumeClaims). Cub-scout's secret evidence work (#328) traces
secret references through Crossplane providers and Flux deployers.

What does *not* yet exist is a step that takes those discovered
dependencies and writes them as a typed Connection contract on the
imported Component. This is where the Connection spec ask for next week
intersects this work. Cub-scout produces the discovered-dependencies
graph; the Connection object stores the typed contract; an explicit
step bridges the two.

For v1 of *import*, the Connection extraction should produce a draft
Connection contract per Component, populated with discovered
dependencies, marked as "imported" rather than "declared", for human
review. v2 can add automatic typing and validation.

---

## V1 scope and non-scope

**In scope for v1.**

1. `cub-scout` learns to produce ConfigHub state in the new vocabulary
   (Component, Deployable Variant, Base Variant, Target).
2. A new flow — call it `cub-scout onboard` for now, naming TBD with
   Brian — that performs Pattern 1 takeover: scan, propose, register,
   publish OCI, repoint controller, verify.
3. Argo CD and Flux are the two supported controllers for v1.
4. A draft Connection contract per imported Component, populated from
   GSF relations and trace results.
5. Updated documentation: `docs/howto/import-to-confighub.md` rewritten
   for the new doctrine, plus a new `docs/howto/onboard-existing.md`
   that walks through Pattern 1 specifically.
6. A worked example each for Argo and Flux, with verifiable proof
   chains (Kubara as one of them, given Jesper's existing setup).

**Explicitly out of scope for v1.**

- Pattern 2 (write-through-Git via cub-gen). Tracked separately for v2.
- Pattern 3 (suspend and assume direct write authority).
- Helm releases without Argo or Flux.
- Multi-cluster federated import.
- Automatic Connection typing beyond "draft from discovery."
- AI Variant creation as part of onboarding.
- Promotion-from-imported-Variant flows.
- Drift remediation across the takeover boundary.
- Crossplane and kro as primary onboarding targets.

---

## Plan, in five tracks

(Tracks 1–5 as detailed in the source plan. Owners: Brian for cub-scout
import code, Brian + Jesper for OCI publish + repoint, Brian for
Connection draft, Brian + Jesper + Alexis for worked examples, Brian for
docs.)

### Track 1 — Vocabulary alignment in cub-scout import output
### Track 2 — OCI publish + controller repoint (Pattern 1 takeover)
### Track 3 — Connection draft from import
### Track 4 — Argo and Flux worked examples
### Track 5 — Documentation alignment

---

## Adjacent product issues, surfaced

- **P1** Connection v1 spec — open ask, no scheduled date.
- **P2** Cub-scout vs ConfigHub boundary framing.
- **P3** Multi-controller customers (Argo + Flux together).
- **P4** Helm-only customers.
- **P5** Application-of-Applications / ApplicationSet takeover semantics.
- **P6** ConfigHub cluster-access read path (kubeconfig-from-platform).
- **P7** Onboarding state vs running state divergence (uncommitted Git changes).

---

## Risks and how to mitigate

- **Risk 1** Pattern 1 takeover causes reconciliation churn — mitigate
  with a dry-run diff before repoint plus explicit acceptance.
- **Risk 2** Customers who refuse to repoint Argo from Git to OCI —
  Pattern 2 covers them; do not oversell v1.
- **Risk 3** Vocabulary alignment ripples beyond cub-scout — do the
  rename surgically in one coordinated PR.
- **Risk 4** Connection draft step diverges from the v1 spec — Track 3
  ships *after* the spec lands, otherwise as plain "discovered
  dependencies."
- **Risk 5** "Side hustle" framing of cub-scout returns — frame the
  work as ConfigHub gaining an onboarding path, implemented across
  cub-scout (read) and ConfigHub (register/publish/repoint).

---

## What done at v1 looks like

A customer with running Argo CD or Flux can:

1. Install cub-scout and the `cub` CLI.
2. Authenticate (`cub auth login`).
3. Run `cub-scout onboard --controller argo -n <namespace>` (name TBD)
   and review the proposal.
4. Confirm. Cub-scout creates ConfigHub state, ConfigHub publishes OCI,
   the controller is repointed, and workloads are now governed.
5. Verify with `compare three-way` (intent / render / observed aligned).
6. Apply a reviewed change through ConfigHub. The existing controller
   reconciles from OCI. Three-way compare confirms.

The customer's running infrastructure is unchanged in shape. Their
controllers keep doing their job. ConfigHub becomes the governance and
review surface.

---

*This is the planning input that drives the two specifications in this
directory: `vocabulary-alignment-v1.md` and `pattern-1-takeover-v1.md`,
together with the documentation rewrites in `docs/howto/`.*
