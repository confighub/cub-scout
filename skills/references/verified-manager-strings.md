# Reference: verified manager strings

The enumeration of `metadata.managedFields[].manager` strings cub-scout recognizes, with upstream source citations. This is the data behind the [scout-attribute](../scout-attribute/SKILL.md) skill's `cause` classification — when a manager string matches an entry here, the classifier produces a deterministic verdict.

The principle, locked in via the attribution layer (#435): **verified strings only, no guessing**. Strings not in this table fall through to `cause: unknown` rather than being misclassified.

## GitOps / orchestration controllers

When a manager string from this section matches **and** the resource's labels/annotations indicate the expected controller (per `pkg/agent/ownership.go`), the classifier emits `cause: controller-drift`.

| Manager string | Source | Match | Upstream citation |
|---|---|---|---|
| `argocd-controller` | Argo CD application controller (SSA, default since v2.6) | exact | [`util/argo/managedfields/managedfields.go` in argo-cd](https://github.com/argoproj/argo-cd) |
| `kubectl-client-side-apply` (Argo CSA migration only) | Argo CD CSA migration default — **ambiguous**, disambiguated by `argocd.argoproj.io/tracking-id` co-signal | exact + co-signal | [Argo CD CSA→SSA migration docs](https://argo-cd.readthedocs.io/en/stable/operator-manual/server-side-apply/) |
| `kustomize-controller` | Flux kustomize-controller | exact | [fluxcd/kustomize-controller](https://github.com/fluxcd/kustomize-controller) `controllerName` constant |
| `helm-controller` | Flux helm-controller | exact | [fluxcd/helm-controller](https://github.com/fluxcd/helm-controller) `controllerName` constant |
| `source-controller` | Flux source-controller | exact | [fluxcd/source-controller](https://github.com/fluxcd/source-controller) `controllerName` constant |
| `helm` | Helm CLI direct (i.e., `helm install` / `helm upgrade` without an external controller) | exact | [helm/helm](https://github.com/helm/helm) `kube.ManagedFieldsManager` default |
| `apiextensions.crossplane.io/composite` | Crossplane composite (XR) controller | exact | [crossplane/crossplane](https://github.com/crossplane/crossplane) composite reconciler |
| `apiextensions.crossplane.io/composed-<hash>` | Crossplane composed children (one hash per XR) | **prefix** | composed-resource reconciler; hash is per-XR |
| `apiextensions.crossplane.io/claim` | Crossplane claim controller | exact | crossplane claim reconciler |
| `apiextensions.crossplane.io/managed` | Crossplane MRD (managed-resource definition) reconciler | exact | crossplane MRD reconciler |
| `managed.crossplane.io/api-simple-reference-resolver` | Crossplane managed-resource reference resolver | exact | crossplane-runtime reference resolver |
| `kro.run/applyset` | kro applyset controller (resource group) | exact | [kro-run/kro](https://github.com/kro-run/kro) applyset controller |
| `kro.run/applyset-parent` | kro applyset parent reconciler | exact | kro applyset parent reconciler |
| `kro.run/labeller` | kro labeller controller | exact | kro labeller controller |
| `application/apply-patch` | Sveltos deployed policy apply writer | exact | [`projectsveltos/libsveltos` `lib/deployer.UpdateResource`](https://github.com/projectsveltos/libsveltos/tree/main/lib/deployer) |

Modelplane is built on Crossplane. For Modelplane-owned composed resources,
cub-scout treats the verified Crossplane manager strings above as expected
controller writers when the resource also carries Modelplane ownership signals.

## Interactive tools (`kubectl` family)

When **only** strings from this section are present in `managedFields` (no controller strings from above), the classifier emits `cause: manual-edit`. When **both** controller and kubectl strings are present, the classifier emits `cause: manual-edit` (mixed — the operator edited on top of reconciliation).

| Manager string | Source | Notes |
|---|---|---|
| `kubectl-client-side-apply` | `kubectl apply` without `--server-side` | The classic legacy apply. Ambiguous with Argo CD's CSA migration default — disambiguated by the `argocd.argoproj.io/tracking-id` annotation co-signal. |
| `kubectl` | `kubectl apply --server-side` | Bare `kubectl` is the SSA default. |
| `kubectl-edit` | `kubectl edit <kind>/<name>` | Always Update operation; conflicting writes overwrite silently. |
| `kubectl-patch` | `kubectl patch <kind>/<name> [--type=...]` | Strategic / merge / JSON patch. |
| `kubectl-create` | `kubectl create -f <yaml>` or imperative create | Initial resource creation. |
| `kubectl-replace` | `kubectl replace -f <yaml>` | Wholesale replacement. |
| `kubectl-last-applied` | The `kubectl.kubernetes.io/last-applied-configuration` annotation writer (legacy migration path) | Marker; not usually a primary signal. |

## The disambiguation rule

The string `kubectl-client-side-apply` appears in two contexts:

1. **Argo CD's CSA migration default.** Argo CD writes this manager string when applying via client-side apply for backward compatibility with non-SSA controllers.
2. **`kubectl apply` (client-side).** The default CLI mode for `kubectl apply` without `--server-side`.

cub-scout disambiguates by looking at the resource's labels and annotations:

- `argocd.argoproj.io/tracking-id` present → Argo CD's CSA migration → `cause: controller-drift`
- No Argo CD annotation, just `kubectl-client-side-apply` → human / agent ran `kubectl apply` → `cause: manual-edit`

This is the **label co-signal** locked in `pkg/agent/manager_strings.go` and `pkg/agent/field_ownership.go`. The classifier never trusts the manager string alone for this case.

## Crossplane prefix matching

Crossplane composed resources use a manager string with a per-XR hash suffix:

```
apiextensions.crossplane.io/composed-9f8e7d6c5b4a
apiextensions.crossplane.io/composed-abc123def456
apiextensions.crossplane.io/composed-1a2b3c4d5e6f
```

Each unique XR produces a unique hash. The classifier matches by **prefix** (`apiextensions.crossplane.io/composed`) and never by exact string. The full manager string is preserved in the receipt evidence as `managerHint` for transparency.

## What's *not* in the table (yet)

The following writers exist in production clusters but are **not yet enumerated** in cub-scout's classifier. They fall through to `cause: unknown` until enumerated:

- **Tekton Pipelines** — Tekton controllers write `tekton.dev/pipeline-controller` or similar variants. Not yet verified.
- **Argo Workflows / Argo Events** — workflow-controller and events-controller manager strings. Not yet verified.
- **Cluster API (CAPI)** — `cluster.x-k8s.io/...` family. Cluster lifecycle controllers.
- **OIDC-based CD systems** (Spinnaker, Harness deployer, etc.) — vendor-specific manager strings.
- **kpt** — `kpt-` prefixed managers, when kpt is the writer of last resort.
- **terraform-controller / Crossplane terraform-provider** — `terraform-controller` and provider-specific strings.

Adding any of these requires:
1. Verifying the actual manager string from a real cluster run (or upstream source code)
2. Confirming the ownership-detection labels (`pkg/agent/ownership.go`) for the controller type
3. Adding the constant + entry to `pkg/agent/manager_strings.go`
4. Adding tests with golden fixtures

The attribution layer's non-goal list (#436) explicitly defers these writers — they'll be filed as follow-on child issues if the variant-management story demands them.

## Where this enumeration lives in code

The verified table is the source of truth at:

- `pkg/agent/manager_strings.go` — the `var` table mapping manager strings → categories (controller / kubectl) → match kind (exact / prefix)
- `pkg/agent/field_ownership.go` — the classifier that consumes the table and produces `cause`
- `pkg/agent/manager_strings_test.go` — golden tests asserting each row produces the expected classification

Any addition to this table is a **code change**, not a doc-only update. The table is fixture-backed: every string has a corresponding test fixture demonstrating the classification.

## See also

- [`kubernetes-managedfields.md`](kubernetes-managedfields.md) — the data substrate this enumeration interprets
- [`scout-attribute/SKILL.md`](../scout-attribute/SKILL.md) — the skill that produces classifications using this table
- cub-scout JSON contract: `docs/reference/json-contracts.md` § "Field Mutation Attribution Contract"
- Attribution layer parent issue: #435 (closed) — shipped via #437 / #438 / #439 / #440
