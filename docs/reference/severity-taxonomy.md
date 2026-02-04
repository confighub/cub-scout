# Drift Severity Taxonomy

**Version:** v0.14.4
**Purpose:** Define severity classification for drift findings

---

## Severity Levels

| Severity | Meaning | CI Behavior |
|----------|---------|-------------|
| `critical` | Requires immediate attention | Fails with `--fail-on critical` |
| `warning` | Requires attention | Fails with `--fail-on warning` or stricter |
| `info` | Informational only | Fails with `--fail-on info` only |

---

## Severity Inference Rules

Severity is inferred from the **type of drift** and **context**. These rules are deterministic and encoded in the comparator engine.

### Replica Drift

| Condition | Severity | Rationale |
|-----------|----------|-----------|
| `live < desired` | warning | Underscaled, potential capacity issue |
| `live > desired` | info | Overscaled, likely intentional (HPA, manual) |

### Image Drift

| Condition | Severity | Rationale |
|-----------|----------|-----------|
| Different repository | critical | Completely different workload |
| Different tag (same repo) | warning | Version mismatch |

### Environment Variable Drift

| Condition | Severity | Rationale |
|-----------|----------|-----------|
| Any change | warning | Configuration mismatch |

### Resource Requests/Limits Drift

| Condition | Severity | Rationale |
|-----------|----------|-----------|
| Invalid config (`limits < requests`) | critical | Kubernetes will reject the pod |
| Normal drift | warning | Resource allocation mismatch |

### Image Pull Policy Drift

| Condition | Severity | Rationale |
|-----------|----------|-----------|
| Any change | warning | Rollout behavior mismatch |

---

## Classification Reference

Each drift finding also has a `classification` field:

| Classification | Description | Typical Drift Types |
|----------------|-------------|---------------------|
| `capacity` | Scaling and resources | replicas, requests/limits |
| `image` | Container images | image tag, image repo |
| `config` | Configuration | env vars, configmap refs |
| `rollout` | Deployment behavior | imagePullPolicy, strategy |
| `health` | Health checks | liveness/readiness probes |
| `label` | Labels | label changes |
| `annotation` | Annotations | annotation changes |
| `other` | Unclassified | catch-all |

---

## Ordering Rules

Findings are sorted by severity for consistent output:

1. **Primary:** `severity` (descending: critical > warning > info)
2. **Secondary:** `object_id` (ascending, lexicographic)
3. **Tertiary:** `path` (ascending, lexicographic)

This ordering is **semantic** (backed by the `severity` JSON field), not narrative.

---

## Semantic Contract

Severity is a **structural fact** (JSON authority), not a **narrative semantic** (ASCII authority).

- Severity is determined by the comparator engine, not the renderer
- ASCII displays severity but does not invent it
- The Leak Test invariant: removing ASCII would not change severity classification

See `docs/semantic-contract.md` for the full model.
