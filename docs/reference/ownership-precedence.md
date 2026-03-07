# Ownership & Precedence Rules

This document defines how cub-scout detects ownership of Kubernetes resources
and the precedence rules when multiple ownership signals are present.

**This is the authoritative reference for v0.5.**

---

## Ownership Detection Order (Precedence)

cub-scout checks for ownership signals in the following order. The **first match wins**.

> **Important:** Some signals can be set by non-authoritative sources. See
> [Known Weak Signals](#known-weak-signals--false-positive-risks) below.

| Priority | Owner | Detection Method |
|----------|-------|------------------|
| 1 | **Flux** | Labels: `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| 2 | **Argo CD** | Label: `argocd.argoproj.io/instance` or annotation: `argocd.argoproj.io/tracking-id` |
| 3 | **Helm** | Label: `app.kubernetes.io/managed-by: Helm` or `helm.sh/chart` |
| 4 | **Terraform** | Annotation: `app.terraform.io/run-id` or label: `app.terraform.io/managed` |
| 5 | **ConfigHub** | Label or annotation: `confighub.com/UnitSlug` |
| 6 | **Crossplane (system)** | API group: `pkg.crossplane.io` or `apiextensions.crossplane.io` |
| 7 | **Crossplane (managed)** | Labels: `crossplane.io/claim-name`, `crossplane.io/composite`, or ownerRef to Crossplane resource |
| 8 | **Custom (config file)** | `~/.cub-scout/detectors.yaml` (or `$CUB_SCOUT_OWNERSHIP_DETECTORS`) detectors, first matching detector wins |
| 9 | **Kubernetes (native)** | OwnerReferences present (controller preferred) |
| 10 | **Unknown** | No ownership signals detected |

---

## Detailed Signal Reference

### Flux

**Confidence: High**

| Signal Type | Key | Example Value | SubType |
|-------------|-----|---------------|---------|
| Label | `kustomize.toolkit.fluxcd.io/name` | `my-app` | kustomization |
| Label | `kustomize.toolkit.fluxcd.io/namespace` | `flux-system` | kustomization |
| Label | `helm.toolkit.fluxcd.io/name` | `my-release` | helmrelease |
| Label | `helm.toolkit.fluxcd.io/namespace` | `flux-system` | helmrelease |

**What cub-scout extracts:**
- Owner name from the `/name` label
- Owner namespace from the `/namespace` label

---

### Argo CD

**Confidence: Medium**

| Signal Type | Key | Example Value |
|-------------|-----|---------------|
| Label | `argocd.argoproj.io/instance` | `my-app` |
| Annotation | `argocd.argoproj.io/tracking-id` | `my-app:apps/Deployment:default/nginx` |

**Detection logic:**
1. If `argocd.argoproj.io/instance` label exists, use its value as owner name
2. If label value is empty, fall back to `app.kubernetes.io/instance`
3. Alternatively, parse `argocd.argoproj.io/tracking-id` annotation (format: `<app-name>:<group>/<kind>:<namespace>/<name>`)

**Why medium confidence:** The `argocd.argoproj.io/instance` label alone is sufficient,
but the tracking-id annotation format can be malformed.

---

### Helm

**Confidence: High**

| Signal Type | Key | Value |
|-------------|-----|-------|
| Label | `app.kubernetes.io/managed-by` | `Helm` (exact match) |
| Label | `helm.sh/chart` | `nginx-1.0.0` (legacy) |
| Label | `app.kubernetes.io/instance` | Release name |

**Detection logic:**
1. Check for `app.kubernetes.io/managed-by: Helm`
2. Fall back to `helm.sh/chart` label (legacy Helm 2 pattern)
3. Release name extracted from `app.kubernetes.io/instance`

---

### Terraform

**Confidence: High/Medium**

| Signal Type | Key | Confidence |
|-------------|-----|------------|
| Annotation | `app.terraform.io/run-id` | High |
| Annotation | `app.terraform.io/workspace-name` | (extracted) |
| Label | `app.terraform.io/managed` | Medium |

---

### ConfigHub

**Confidence: High**

| Signal Type | Key | Example Value |
|-------------|-----|---------------|
| Label | `confighub.com/UnitSlug` | `my-unit` |
| Annotation | `confighub.com/UnitSlug` | `my-unit` |
| Annotation | `confighub.com/SpaceName` | `production` |

**Detection logic:**
1. Check label `confighub.com/UnitSlug` first
2. Fall back to annotation `confighub.com/UnitSlug`
3. Extract space name from annotations

---

### Crossplane

**System resources (Confidence: High/Medium)**

| Signal Type | Condition | SubType |
|-------------|-----------|---------|
| API Group | `pkg.crossplane.io` | system |
| API Group | `apiextensions.crossplane.io` | system |
| Namespace + Kind | `crossplane-system` + known system kinds | system |

Known system kinds: `providerrevision`, `configurationrevision`, `functionrevision`,
`provider`, `configuration`, `function`, `deploymentruntimeconfig`

**Managed resources (Confidence: High/Medium)**

| Signal Type | Key | SubType |
|-------------|-----|---------|
| Label | `crossplane.io/claim-name` | claim |
| Label | `crossplane.io/claim-namespace` | claim |
| Label | `crossplane.io/composite` | composite |
| Annotation | `crossplane.io/composition-resource-name` | managed-resource |
| OwnerReference | APIVersion contains `crossplane.io` or `upbound.io` | (from owner kind) |

---

### Kubernetes (Native)

**Confidence: Medium**

Detected when OwnerReferences are present but no GitOps/IaC signals match.

| Signal Type | Condition | SubType |
|-------------|-----------|---------|
| OwnerReference | `controller: true` preferred | (from owner kind) |
| OwnerReference | First owner if no controller | (from owner kind) |

**What cub-scout extracts:**
- Owner kind (lowercased) as SubType
- Owner name
- Resource namespace

---

### Unknown

When no ownership signals are detected, cub-scout returns:

```json
{
  "type": "unknown"
}
```

**This does NOT mean the resource is orphaned.** It means cub-scout could not
determine ownership from available signals.

---

### Custom (Config File)

**Confidence: High** (explicit configured match rules)

Custom detectors are loaded from:

- `~/.cub-scout/detectors.yaml`
- or `$CUB_SCOUT_OWNERSHIP_DETECTORS` (path override)

Format:

```yaml
detectors:
  - name: internal-platform
    labels:
      - key: platform.company.com/managed-by
        value: platform-controller
    owner_name: Internal Platform
    owner_type: custom
```

Rules:

1. Built-in detectors run first (priorities 1-7 above).
2. Custom detectors run next, in file order (first match wins).
3. If the config file is missing, behavior is unchanged.
4. If the config file is invalid, cub-scout prints a warning and continues with built-ins only.

---

## Precedence: Multiple Signals Present

When a resource has multiple ownership signals, cub-scout uses **first-match wins**
based on the priority order above.

### Example Scenarios

| Signals Present | Winner | Reason |
|-----------------|--------|--------|
| Flux + Helm labels | **Flux** | Flux checked before Helm (priority 1 vs 3) |
| ArgoCD + Helm labels | **ArgoCD** | ArgoCD checked before Helm (priority 2 vs 3) |
| Helm + OwnerRef | **Helm** | Helm checked before K8s native (priority 3 vs 9) |
| Crossplane claim + OwnerRef | **Crossplane** | Crossplane checked before K8s native (priority 7 vs 9) |
| Flux + matching custom detector | **Flux** | Built-ins run before custom detectors |
| Two matching custom detectors | **First custom detector** | Custom detectors are evaluated in file order |
| Only OwnerRef | **Kubernetes** | Only signal present |

### Flux HelmRelease Special Case

A resource deployed by Flux via HelmRelease will have **both**:
- `helm.toolkit.fluxcd.io/*` labels (from Flux)
- `app.kubernetes.io/managed-by: Helm` (from Helm)

cub-scout correctly identifies this as **Flux** (owner: flux, subType: helmrelease)
because Flux labels are checked first.

---

## Known Weak Signals / False-Positive Risks

Some ownership signals can be set by tools other than the expected owner. cub-scout
detects based on labels/annotations as documented, but users should be aware:

| Signal | Risk | Mitigation |
|--------|------|------------|
| `app.kubernetes.io/managed-by: Helm` | Can be set by non-Helm tools | Check for `helm.sh/chart` or Helm secret in namespace |
| `app.kubernetes.io/instance` | Generic label, used by many tools | Only used as fallback, not primary signal |
| `argocd.argoproj.io/tracking-id` | Annotation can be malformed | Parse failures return partial or unknown |
| OwnerReferences | Can point to deleted resources | Returns K8s native, not unknown |

**cub-scout's stance:** When a signal is present, cub-scout reports it. It does not
second-guess whether the signal is "authentic." If this leads to incorrect results,
the correct fix is to remove the misleading label/annotation from the resource.

cub-scout prefers **unknown over incorrect certainty** only when no signals are
present. When signals exist, it trusts them.

---

## Unknown and Ambiguous Cases

cub-scout explicitly returns "unknown" rather than guessing in these cases:

| Scenario | Result | Reason |
|----------|--------|--------|
| No labels, no annotations, no ownerRefs | `unknown` | No signals to detect |
| Labels present but not matching any pattern | `unknown` | Unrecognized ownership pattern |
| Malformed tracking-id annotation | `unknown` or partial | Parse failure handled gracefully |
| Empty label values | Falls through | Empty string is not a valid owner name |

### What cub-scout will NOT claim

cub-scout will **never**:

1. **Infer ownership from resource names** - A deployment named `flux-*` is not assumed to be Flux-owned
2. **Infer ownership from namespace names** - Resources in `argocd` namespace are not assumed ArgoCD-owned
3. **Guess based on resource type** - CRDs are not assumed to belong to their controller
4. **Claim ownership without explicit signals** - Heuristics are avoided in favor of explicit markers

---

## Confidence Levels

Each ownership detection includes a confidence level:

| Level | Meaning |
|-------|---------|
| **high** | Strong signal, unlikely to be wrong |
| **medium** | Valid signal but could be ambiguous |

Confidence is informational. cub-scout does not change behavior based on confidence.

---

## Source Field

Every ownership result includes a `source` field indicating which signal was used:

```
label:kustomize.toolkit.fluxcd.io/name
annotation:argocd.argoproj.io/tracking-id
label:app.kubernetes.io/managed-by=Helm
ownerRef:controller
apiGroup:pkg.crossplane.io
```

This enables debugging when ownership appears incorrect.

---

## Related Documentation

- [How To: Understand Ownership Detection](../howto/ownership-detection.md) - User guide
- [Reference: Commands](commands.md) - CLI usage
- Source: `pkg/agent/ownership.go`
- Custom detector loader: `pkg/agent/ownership_custom.go`
