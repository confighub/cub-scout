# cub-scout Roadmap

**Last Updated:** 2026-01-22

Future features and improvements for cub-scout.

For completed work, see [archive/old-roadmap-jan.md](archive/old-roadmap-jan.md).

---

## Future Features (P2-P3)

### `cub-scout learn` Command (P3)

Interactive learning about GitOps concepts using your live cluster:

```bash
cub-scout learn gitops     # What is GitOps? Interactive explanation
cub-scout learn flux       # How Flux works with live cluster examples
cub-scout learn argocd     # How ArgoCD works with live cluster examples
cub-scout learn kustomize  # What is Kustomize? Base + overlays explained
cub-scout learn helm       # Helm releases, charts, values
cub-scout learn ownership  # How cub-scout detects ownership
```

Each lesson:
1. Explains the concept
2. Shows examples from YOUR cluster (if available)
3. Suggests commands to try
4. Links to documentation

---

### Enhanced Import Wizard (P2)

Improved `cub-scout import --wizard` with:
- Pattern detection (D2, Arnie, Banko, Fluxy)
- Dependency detection between apps
- Suggested ConfigHub structure
- Step-by-step guided import

---

### In-TUI Learning (P3)

Contextual tooltips when hovering/selecting items in the TUI:

```
┌─ cub-scout map ───────────────────────────────────────────────────┐
│ WORKLOADS BY OWNER                                                 │
│                                                                    │
│ Flux (28)                                                          │
│ > ▶ frontend          production    Deployment  ✓                  │
│                                                                    │
│ ┌─ INFO ─────────────────────────────────────────────────────────┐ │
│ │ This Deployment is managed by Flux via:                        │ │
│ │   Kustomization: frontend (flux-system)                        │ │
│ │ Changes should be made in Git, not kubectl.                    │ │
│ │ Press T to trace the full ownership chain                      │ │
│ └────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────┘
```

---

### JSON Output Consistency (P2)

Ensure all commands support `--json`:
- `map orphans --json` ✅
- `map crashes --json`
- `map issues --json`
- `map workloads --json`
- `map deployers --json`

---

### Exit Codes for Scripting (P3)

Consistent exit codes for CI/CD integration:

| Exit Code | Meaning |
|-----------|---------|
| `0` | Success |
| `1` | Error (command failed) |
| `2` | Issues found (e.g., `map issues` found problems) |

```bash
cub-scout map issues || echo "Issues found!"
cub-scout scan --severity critical && echo "No critical issues"
```

---

### Advanced Diff & Tracing (P2-P3)

| Feature | Description | Priority |
|---------|-------------|----------|
| Chart version diff | Show what changed between helm chart versions | P2 |
| Layer-by-layer trace | Show which layer caused a change | P2 |
| Upgrade impact preview | Before upgrading, show what will change | P3 |

---

## Priority Summary

| Priority | Focus |
|----------|-------|
| **P2** | Import wizard, JSON consistency, chart diff |
| **P3** | Learn command, exit codes, in-TUI learning |

---

## Validation Criteria

For each change, verify:
- [ ] Solves a real user problem
- [ ] Teaches, not just shows
- [ ] Works with realistic scale (50+ resources)
- [ ] No breaking changes to existing behavior
- [ ] Can be tested/demoed
