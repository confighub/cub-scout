# Three Sources of Truth

Configuration lives in three places. They often disagree. The Map shows when they do.

---

## The Three Sources

| Source | What It Holds | Role |
|--------|---------------|------|
| **Git** | DRY templates | What you WROTE |
| **ConfigHub** | WET rendered manifests | What SHOULD run |
| **Cluster** | Actual resources | What IS running |

---

## Git Says WHAT (DRY)

Git holds your **intent** — what you want deployed.

But Git holds **DRY** config: templates, not final manifests.

- Helm charts with `{{ .Values.replicas }}`
- Kustomize bases with overlays and patches
- Variable substitutions like `${ENVIRONMENT}`

**Problem:** What you see in Git isn't what deploys. You have to "mentally compile" bases + overlays + patches + substitutions to understand what actually runs.

---

## ConfigHub Says HOW (WET)

ConfigHub holds **rendered manifests** — the actual YAML that deploys.

This is **WET** config: fully rendered, no templates.

```yaml
# What's in Git (DRY)
replicas: {{ .Values.replicas }}

# What's in ConfigHub (WET)
replicas: 5
```

**ConfigHub shows:**
- Which tool deploys what (Flux, Argo, Helm)
- Who owns each resource
- What revision is live
- The actual values, not template variables

**Benefit:** What you see is what deploys. No mental compilation.

---

## Cluster Says NOW

The cluster holds **reality** — what's actually running.

This may differ from both Git and ConfigHub because:
- Someone ran `kubectl edit` at 2am
- A controller modified the resource
- Drift happened

---

## When They Disagree

| Scenario | Git | ConfigHub | Cluster | What Happened |
|----------|-----|-----------|---------|---------------|
| **Synced** | A | A | A | Everything matches |
| **Drift** | A | A | B | Cluster changed (kubectl edit, controller) |
| **Pending** | B | A | A | Git updated, not yet deployed |
| **Stale** | B | B | A | Deployed but cluster reverted |

The Map shows these disagreements. See [MERGES-AND-WRITE-FLOWS.md](MERGES-AND-WRITE-FLOWS.md) for how to resolve them.

---

## Why Three, Not Two?

**Traditional GitOps:** Git → Cluster (two sources)

**Problem:** You can't see what *should* be running vs what *is* running. Git shows templates, not rendered output.

**ConfigHub:** Git → ConfigHub → Cluster (three sources)

**Benefit:** ConfigHub is the **operational truth** — rendered manifests that match what deploys. You can compare:
- Git vs ConfigHub: "Did my template render correctly?"
- ConfigHub vs Cluster: "Did the deploy succeed? Did it drift?"

---

## The Map Connects Them

```
Git (DRY)          ConfigHub (WET)         Cluster (NOW)
    │                    │                      │
    │   render           │     deploy           │
    └──────────────>─────┴────────────────>─────┘
                         │
                         │ observe (cub-agent)
                    ─────┴─────
                         │
                      THE MAP
                         │
            "Here's what matches and what doesn't"
```

The agent watches the cluster, compares to ConfigHub, and shows drift.

---

## Summary

| Source | Content | Format |
|--------|---------|--------|
| **Git** | Templates, intent | DRY (needs rendering) |
| **ConfigHub** | Rendered manifests | WET (ready to deploy) |
| **Cluster** | Running resources | Live state |

**Git says WHAT.** ConfigHub says HOW. Cluster says NOW.

The Map shows when they agree — and when they don't.

---

## See Also

- [01-MAP-CONCEPT.md](01-MAP-CONCEPT.md) — The Map as central concept
- [06-MERGES-AND-WRITE-FLOWS.md](06-MERGES-AND-WRITE-FLOWS.md) — How to resolve disagreements
- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Where WET manifests live (Units)

---

**Next:** [06-MERGES-AND-WRITE-FLOWS.md](06-MERGES-AND-WRITE-FLOWS.md) — Reconciliation and write strategies
