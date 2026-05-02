# Pattern 1 Takeover v1: OCI publish + controller repoint

**Status:** Specification, not implemented
**Owner (proposed):** Brian + Jesper (Brian for the cub-scout-side flow,
Jesper for the OCI byte-equivalence question on the ConfigHub side)
**Tracks:** Track 2 of the [import plan](cubscout-import-plan.md)
**Related:** [vocabulary-alignment-v1.md](vocabulary-alignment-v1.md)

---

## What this is

A specification for the v1 takeover flow that brings an existing Argo CD
Application or Flux Kustomization under ConfigHub governance without
redeploying the workload. The customer's controller keeps running. Its
source moves from Git to ConfigHub OCI. The cluster shape does not
change at takeover.

Pattern 1 was demonstrated working for fresh-deployed Argo on 2026-05-01
(Jesper, Kubara org) with a follow-on image-pin change on 2026-05-02
captured as a four-layer proof. The fresh-deployment mechanism is solid.
This spec covers the *attach-to-existing* case, which is the v1 customer
shape.

This is a planning spec. Code lives behind it.

---

## Where the cub-scout / ConfigHub split sits

Cub-scout does discovery, proposal, dry-run diff, verification, and
proof emission. Cub-scout is read-only at the cluster level.

The state-changing steps — creating Components and Variants in
ConfigHub, publishing OCI, and repointing the controller — are
ConfigHub writes invoked through the `cub` CLI. The `cub-scout onboard`
flow orchestrates them but does not perform cluster mutations directly.

| Step | Performs the action | Why |
|------|--------------------|-----|
| Discover existing Application / Kustomization | cub-scout | Read-only cluster access. |
| Propose Component / Variant / Target shape | cub-scout | Same proposal engine as `import`. |
| Dry-run diff: predicted OCI render vs current controller render | cub-scout (calls ConfigHub render API) | Read-only on both sides. |
| Create Component, Base Variant, Deployable Variant in ConfigHub | `cub` (orchestrated by `cub-scout onboard`) | ConfigHub write. |
| Publish first OCI artifact | `cub` (or ConfigHub render pipeline) | ConfigHub side. |
| Repoint controller `spec.source` | `cub` (orchestrated by `cub-scout onboard`) | Cluster write — done via `cub`, not directly by cub-scout. |
| Verify post-takeover (`compare three-way`) | cub-scout | Read-only. |

This preserves the read-only-by-default invariant. cub-scout's role is
to surface what's true and to drive a proven sequence of `cub` writes,
not to mutate the cluster itself.

---

## The flow

Working name: `cub-scout onboard --controller {argo|flux} -n <namespace>`.
Final name to be set by Brian — see open questions below. The plan uses
"onboard"; alternatives include `import --takeover`, `attach`, and
`adopt`.

### Step 0 — Preconditions

- `cub auth login` has been done.
- `kubectl` context targets the cluster the controller is running in.
- The customer's Argo Application or Flux Kustomization has matching
  labels / spec that cub-scout can detect (already true — discovery is
  shipping).
- The customer's Git source is reachable from the machine running the
  command (we need to read the same Git revision the controller is
  reading, for the dry-run diff).

If any precondition fails, fail loud with remediation steps. No
silent fallbacks.

### Step 1 — Discover

```
cub-scout onboard --controller argo -n payments --dry-run
```

Outputs:

- the Argo Application(s) found in the namespace (or matching the
  selector);
- the Component / Base Variant / Deployable Variant proposal in the
  new vocabulary;
- the Target binding (existing controller's destination cluster);
- the discovered Connection drafts (Track 3);
- the *predicted OCI render* vs the *current controller render*,
  field-level diff.

The diff is the key new artifact. Without it, the takeover is a
black-box source swap.

### Step 2 — Dry-run diff

This is the byte-equivalence question made concrete.

The dry-run does:

1. Fetch the Argo Application's current source (Git repo + path + ref).
2. Render it locally the same way Argo would (Helm/Kustomize through the
   same toolchain Argo uses).
3. Ask ConfigHub to render the proposed Component / Deployable Variant
   to OCI bytes.
4. Diff (1) and (3) at the manifest level.

Three possible outcomes:

| Outcome | What it means | What v1 does |
|---------|---------------|--------------|
| **Byte-equivalent** | The OCI render is identical to what Argo was reading from Git. | Takeover is silent — no reconcile churn. Proceed. |
| **Cosmetically different** | Field ordering, whitespace, label normalisation — semantically identical. | Show the diff. Require explicit `--accept-cosmetic-diff`. Document expected churn (e.g. one no-op apply). |
| **Semantically different** | Resource bodies, replica counts, env values, image tags differ. | Block. Engineering loop, not a takeover. The customer's Git and the proposed Component are not yet aligned. Surface the diff and stop. |

The honest position: byte-equivalence is the goal, but Argo and Flux
both perform some normalisation (managedFields stripping, status
removal, label injection) that ConfigHub's render needs to either match
exactly or treat as a known cosmetic delta. Engineering should aim for
byte-equivalence and treat cosmetic diffs as a v1 escape hatch with
explicit operator acceptance, not a silent default.

The dry-run output includes:

```
$ cub-scout onboard --controller argo -n payments --dry-run

Discovered Argo Application: payments-prod
  Source: git@github.com:acme/deploy.git, path apps/payments/overlays/prod, ref main@a1b2c3d
  Destination: prod-east-cluster, namespace payments

Proposed Component: payments
  Base Variant: payments-base
  Deployable Variant: payments-prod (Labels.Variant=prod) → prod-east-cluster

Render diff (controller current vs ConfigHub proposed OCI):
  byte-equivalent: NO
  cosmetic-only:    YES (3 differences)
    - field ordering in 2 Deployments (semantic equivalence: confirmed)
    - extra empty annotation block in 1 Service (semantic equivalence: confirmed)

To accept the cosmetic diff and proceed: rerun without --dry-run and pass --accept-cosmetic-diff.
```

If the diff is semantically meaningful, the command stops. The customer
fixes the divergence in their Component before retrying.

### Step 3 — Execute the takeover

```
cub-scout onboard --controller argo -n payments --accept-cosmetic-diff
```

Sequence (atomic-as-possible, with rollback markers between steps):

1. Cub-scout calls `cub` to create the Component, Base Variant, and
   Deployable Variant in ConfigHub. Idempotent.
2. Cub-scout calls `cub` to publish the first OCI artifact for the
   Deployable Variant.
3. Cub-scout calls `cub` to update the Argo Application's
   `spec.source` (or the Flux Kustomization's source ref) to point at
   the ConfigHub OCI URL. This is the cluster-mutating step.
4. Cub-scout waits for the controller to reconcile (configurable
   timeout, default 5 minutes).
5. Cub-scout runs `compare three-way` and reports the result.

If step 3 succeeds but step 4 times out (controller not reconciling),
the rollback path in Step 5 is available.

### Step 4 — Verify

`compare three-way` after takeover should be all green:

- intent (ConfigHub Variant) matches
- render (ConfigHub OCI) matches
- observed (cluster state) matches

The verification surfaces an explicit "takeover proof" record stored
in the audit log (already shipping via `cub-scout audit`), referencing
the Application/Kustomization name, the OCI URL, the pre-takeover Git
ref, and the diff outcome.

### Step 5 — Rollback

If the takeover went wrong and the customer wants the controller back
on Git:

```
cub-scout onboard --rollback --controller argo -n payments
```

This:

1. Reads the audit log for the most recent takeover of this
   Application.
2. Restores `spec.source` on the Argo Application to the pre-takeover
   Git ref (recorded in the audit).
3. Optionally deletes the ConfigHub Component / Variants (`--purge`),
   or leaves them in place for re-attempt (`--keep-state`, default).
4. Verifies the controller has reconciled back to Git.

Rollback is not free. If changes were made through ConfigHub between
takeover and rollback, those changes are not in Git. Cub-scout warns
loudly in that case and refuses `--rollback` without an additional
`--accept-data-loss` flag.

---

## Argo-specific takeover details

### Single Application (simple case)

`spec.source` is updated from `{repoURL, path, targetRevision}` to
`{repoURL: "oci://hub.confighub.com/<org>/<component>:<variant>",
 chart: "", helm: nil, kustomize: nil}`. Argo CD 2.6+ supports OCI as a
source type natively.

### ApplicationSet generators

ApplicationSet creates child Applications dynamically. Two takeover
strategies:

- **Generated-children takeover** (v1): leave the ApplicationSet alone,
  takeover the *generated* child Applications individually. The parent
  generator continues to create children pointing at Git, which is
  wrong post-takeover. v1 documents this as a known limitation.
- **Generator takeover** (v2): rewrite the ApplicationSet to generate
  OCI-sourced children. Out of v1 scope.

For v1, ApplicationSet customers are supported one-child-Application-at-a-time
with an explicit warning that the parent generator will fight the
takeover on its next sync. Customers either pause the ApplicationSet
or move to v2.

### App-of-Apps

Same as ApplicationSet: the parent App is metadata, the children are
the workload sources of truth. Take over children individually.

---

## Flux-specific takeover details

### Kustomization with GitRepository source

The Flux Kustomization's `spec.sourceRef` is updated from a
`GitRepository` reference to an `OCIRepository` reference. The
`OCIRepository` resource is created in the same namespace, pointing
at the ConfigHub OCI URL. Flux 2.0+ supports OCIRepository as a
first-class source.

### HelmRelease with HelmRepository source

A HelmRelease backed by a HelmRepository is harder. Flux supports
OCI Helm repositories (`HelmRepository.spec.type: oci`). The takeover
swaps the HelmRepository's underlying ref. Out of v1 if the customer
is using a non-OCI HelmRepository — document as a known gap.

### HelmRelease with GitRepository (chart-from-Git)

Migrate to OCIRepository the same way Kustomizations do. ConfigHub's
OCI render handles the chart materialisation.

---

## Verification: what proof looks like

Post-takeover, cub-scout writes an `onboard-proof.json` (or appends to
the audit log):

```json
{
  "schemaVersion": "v1",
  "controller": "argo",
  "namespace": "payments",
  "application": "payments-prod",
  "preTakeover": {
    "source": {"type": "git", "repoURL": "...", "path": "...", "ref": "a1b2c3d"}
  },
  "postTakeover": {
    "source": {"type": "oci", "url": "oci://hub.confighub.com/acme/payments:prod", "digest": "sha256:..."}
  },
  "renderDiff": {
    "byteEquivalent": false,
    "cosmeticOnly": true,
    "differenceCount": 3,
    "operatorAccepted": true,
    "acceptedAt": "2026-05-12T14:23:00Z"
  },
  "verification": {
    "compareThreeWay": "green",
    "ranAt": "2026-05-12T14:25:30Z"
  }
}
```

This is the four-layer proof Alexis captured manually for the
2026-05-02 image-pin change, made repeatable.

---

## Open design questions

These need a 30-minute conversation with ConfigHub engineering before
code starts.

- **Q-BYTEEQ-1 (the hard one).** Does ConfigHub's OCI render produce
  output byte-identical to Argo's and Flux's render of the same
  source? If not, what's the minimal cosmetic-difference set, and is
  that set stable across ConfigHub releases? Owner: Jesper.
- **Q-BYTEEQ-2.** If byte-equivalence is not achievable, do we need a
  one-time "accept-and-reconcile" mode that allows the controller to
  reconcile the cosmetic diff once, with the operator acknowledging
  the diff is expected? Or do we hold the line and require
  byte-equivalence as a v1 ship gate? Owner: Brian + Jesper.
- **Q-NAMING-1.** Final command name. `onboard` is the working name.
  Alternatives: `attach`, `adopt`, `import --takeover`. Owner: Brian.
- **Q-CUB-CMD-1.** Which `cub` subcommand performs the controller
  source repoint? Today there is no `cub argo set-source` or
  equivalent. Either we add one to `cub` or `cub-scout onboard`
  shells out to `kubectl patch` directly (which feels wrong from a
  read-only-substrate perspective). Owner: Brian, ConfigHub side.
- **Q-ROLLBACK-1.** Is rollback a first-class flag (`--rollback`) or
  a separate command (`cub-scout offboard`)? `--rollback` keeps the
  flow symmetric. Separate command is more explicit. Owner: Brian.
- **Q-APPSET-1.** ApplicationSet support strategy for v1: do we
  refuse to take over generated children when the ApplicationSet is
  active, or do we take them over and warn? The plan says warn-only
  for v1; confirm with Jesper.
- **Q-OCI-AUTH-1.** The Argo Application or Flux Kustomization needs
  credentials to pull from ConfigHub OCI. Where does the auth secret
  come from, and does the takeover flow create it, or is that a
  pre-onboard step? Owner: Jesper.
- **Q-AUDIT-1.** The proof artifact above (`onboard-proof.json`) — is
  this a new file format or an extension of the existing
  `cub-scout audit` log? Prefer extension. Owner: Brian.

---

## Honest position on the hard parts

**Byte-equivalence.** The right v1 answer is probably "byte-equivalent
in the common case, cosmetic diff with explicit acceptance in the
edge case, hard fail otherwise." Promising byte-equivalence
universally is a research project. Promising "no surprises and a
clean diff at takeover" is shippable.

**Rollback completeness.** Once a customer has applied changes through
ConfigHub post-takeover, rollback to Git loses those changes unless
ConfigHub publishes them back to Git first (which is Pattern 2 — out
of v1 scope). v1 rollback is therefore "abandon the takeover before
trusting ConfigHub as the source." We document this clearly and
refuse to silently drop changes.

**ApplicationSet and App-of-Apps.** v1 supports them with caveats.
v2 supports them properly by rewriting the parent generator. The v1
docs need to be honest about this.

**Helm-via-non-OCI-source.** Some Flux HelmReleases reference a
`HelmRepository` that is HTTP, not OCI. Takeover for those requires
that the HelmRepository itself be migrated, which is a larger change
than swapping a sourceRef. v1 documents this and supports OCI-capable
sources only.

---

## Out of scope for this spec

- Pattern 2 (write-through-Git via cub-gen).
- Pattern 3 (suspend and assume direct write authority).
- Helm releases without Argo or Flux as the controller.
- Multi-cluster federated takeover.
- Drift remediation across the takeover boundary.
- The implementation code.
- The actual `cub` subcommands that perform the writes — those are a
  ConfigHub-side spec.

---

## Done means

- `cub-scout onboard --controller argo -n <ns> --dry-run` produces
  the proposal, render diff, and Connection draft, with no cluster
  mutations.
- `cub-scout onboard --controller argo -n <ns>` (without dry-run)
  performs the takeover sequence, creates the audit proof, and runs
  the post-takeover `compare three-way`.
- The same flow works for Flux (`--controller flux`).
- The byte-equivalence question has a documented answer and the
  flow's behaviour matches it.
- Rollback (`--rollback`) restores the controller's pre-takeover
  source.
- Both flows have worked examples in `examples/onboard-existing-argocd/`
  and `examples/onboard-existing-flux/` that a new engineer can replay.
