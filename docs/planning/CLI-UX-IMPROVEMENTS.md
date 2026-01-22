# CLI UX Improvement Plan

**Status:** DRAFT
**Date:** 2026-01-22
**Goal:** Make cub-scout CLI commands the BEST user experience for GitOps exploration

---

## Current State Assessment

### What Works Well

| Command | Strengths |
|---------|-----------|
| `trace` | Excellent formatting, colors, shows full chain with status |
| `scan` | Actionable FIX commands, clear severity levels, CCVE references |
| `map workloads` | Good table with STATUS column, owner grouping, image info |
| `map deep-dive` | Comprehensive, well-organized sections |
| `map deployers` | Clean table with revision and resource counts |
| `map status` | Perfect one-liner for quick health check |
| `--help` | Comprehensive, good examples |

### What Needs Improvement

| Command | Issues |
|---------|--------|
| `map orphans` | No context header, no explanation of why orphans matter |
| `map crashes` | Identical output to `map issues`, no differentiation |
| `map issues` | No suggested next steps |
| `map list` | Summary at end is good but could be more helpful |

---

## Improvement Plan

### Priority 1: Add Context Headers

**Problem:** Users see raw data without understanding why it matters.

**Solution:** Add contextual headers to commands that need explanation.

#### `map orphans` - Before:
```
NAMESPACE           KIND           NAME                    OWNER
argocd              Application    api-gateway             Native
argocd              StatefulSet    argocd-application-controller   Native
...
```

#### `map orphans` - After:
```
ORPHAN RESOURCES
════════════════════════════════════════════════════════════════════
Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub).
These may be: legacy systems, manual hotfixes, debug pods, or shadow IT.

NAMESPACE           KIND           NAME                    OWNER
argocd              Application    api-gateway             Native
...

Total: 45 orphan resources across 8 namespaces

→ To import into ConfigHub: cub-scout import --wizard
→ To trace ownership: cub-scout trace <kind>/<name> -n <namespace>
```

#### Implementation:
```go
// In runMapOrphans, before calling runMapList:
if !mapJSON && !mapCount && !mapNamesOnly {
    fmt.Println(orphanHeaderStyle.Render("ORPHAN RESOURCES"))
    fmt.Println(strings.Repeat("═", 68))
    fmt.Println(dimStyle.Render("Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub)."))
    fmt.Println(dimStyle.Render("These may be: legacy systems, manual hotfixes, debug pods, or shadow IT."))
    fmt.Println()
}
```

### Priority 2: Add "Next Steps" Suggestions

**Problem:** Users see problems but don't know what to do next.

**Solution:** Add contextual suggestions after output.

#### `map issues` - After:
```
✗ Kustomization/payment-api in break-glass-demo: ArtifactFailed
✗ HelmRelease/payment-api in flux-system: SourceNotReady
...

31 issues found

→ For remediation commands: cub-scout scan
→ To trace a failing resource: cub-scout trace <kind>/<name> -n <namespace>
→ To see full details: cub-scout map deep-dive
```

#### `map crashes` - After:
```
✗ Deployment/postgresql in demo-prod: 0/1 ready
✗ Deployment/prometheus in monitoring: 1/2 ready
...

24 crashing/failing workloads

→ To see pod logs: kubectl logs -n <namespace> <pod>
→ To describe pod: kubectl describe pod -n <namespace> <pod>
→ To trace ownership: cub-scout trace deploy/<name> -n <namespace>
```

### Priority 3: Differentiate Similar Commands

**Problem:** `map crashes` and `map issues` show nearly identical output.

**Current behavior:**
- `map crashes` → calls `runMapProblems`
- `map issues` → calls `runMapProblems`
- Both show the same output

**Solution:** Differentiate them clearly:

| Command | Focus | Shows |
|---------|-------|-------|
| `map crashes` | Pod health only | CrashLoopBackOff, ImagePullBackOff, OOMKilled, Error |
| `map issues` | GitOps health | All: deployers (Kustomizations, HelmReleases, Applications) + workloads |

#### `map crashes` - Proposed:
```
CRASHING PODS
═══════════════════════════════════════════════════════════════════
Pods in CrashLoopBackOff, Error, OOMKilled, or ImagePullBackOff.

NAMESPACE      POD                           STATUS           RESTARTS   AGE
demo-prod      postgresql-abc123-xyz         CrashLoopBackOff 47         2d
monitoring     prometheus-def456-uvw         OOMKilled        12         6h
...

5 crashing pods

→ View logs: kubectl logs -n <namespace> <pod> --previous
→ Describe: kubectl describe pod -n <namespace> <pod>
```

#### `map issues` - Proposed:
```
RESOURCES WITH ISSUES
═══════════════════════════════════════════════════════════════════
Deployers and workloads with conditions != Ready.

DEPLOYERS (7 issues)
✗ Kustomization/payment-api in break-glass-demo: ArtifactFailed
✗ HelmRelease/payment-api in flux-system: SourceNotReady
...

WORKLOADS (24 issues)
✗ Deployment/postgresql in demo-prod: 0/1 ready
✗ Deployment/prometheus in monitoring: 1/2 ready
...

31 total issues (7 deployers, 24 workloads)

→ For remediation: cub-scout scan
→ To trace: cub-scout trace <kind>/<name> -n <namespace>
```

### Priority 4: Summary Line Consistency

**Problem:** Some commands have summaries, some don't.

**Solution:** All commands should have a consistent summary format.

| Command | Current Summary | Proposed Summary |
|---------|-----------------|------------------|
| `map list` | ✓ Has summary | Keep as-is |
| `map orphans` | ✗ No summary | Add: "45 orphan resources across 8 namespaces" |
| `map crashes` | ✗ No summary | Add: "5 crashing pods" |
| `map issues` | ✗ No summary | Add: "31 issues (7 deployers, 24 workloads)" |
| `map workloads` | ✗ No summary | Add: "48 workloads: 28 Flux, 12 Helm, 8 Native" |
| `map deployers` | ✗ No summary | Add: "13 deployers: 8 Kustomizations, 3 HelmReleases, 2 Applications" |

### Priority 5: JSON Output Consistency

**Problem:** Not all commands support `--json` consistently.

**Audit needed:**
- [ ] `map orphans --json` - Should output JSON array of orphan entries
- [ ] `map crashes --json` - Should output JSON array of crash info
- [ ] `map issues --json` - Should output JSON array of issue entries
- [ ] `map workloads --json` - Should output JSON array of workload entries
- [ ] `map deployers --json` - Should output JSON array of deployer entries

### Priority 6: Exit Codes for Scripting

**Problem:** Commands always exit 0 even when issues are found.

**Solution:** Use exit codes for scripting:
- `0` - Success (no issues found, or command completed)
- `1` - Error (command failed)
- `2` - Issues found (e.g., `map issues` found problems)

This allows:
```bash
cub-scout map issues || echo "Issues found!"
cub-scout scan --severity critical && echo "No critical issues"
```

---

## Implementation Order

| Phase | Commands | Effort |
|-------|----------|--------|
| **Phase 1** | `map orphans` header + summary | 1 hour |
| **Phase 2** | `map issues` differentiation + suggestions | 2 hours |
| **Phase 3** | `map crashes` differentiation | 1 hour |
| **Phase 4** | Summaries for all commands | 1 hour |
| **Phase 5** | JSON output audit | 2 hours |
| **Phase 6** | Exit codes | 1 hour |

**Total: ~8 hours**

---

## Success Criteria

After implementation:

1. **First-time users** can understand what each command shows without reading docs
2. **Every command** suggests what to do next
3. **All commands** have consistent summary format
4. **JSON output** works for all list-style commands
5. **Exit codes** enable scripting

---

## Non-Goals (What We're NOT Doing)

- Changing the TUI (already excellent)
- Adding new commands (focus on existing)
- Breaking existing output formats (backwards compatibility)
- Adding colors to tabular output (keep it simple for scripting)

---

## Testing Plan

1. **Manual testing:** Run each command before/after
2. **Golden file tests:** Update expected outputs
3. **JSON validation:** Ensure `--json` output is valid JSON
4. **Scripting test:** Verify exit codes work as expected

---

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/cub-scout/map.go` | `runMapOrphans`, `runMapCrashes`, `runMapIssues`, `runMapWorkloads`, `runMapDeployers` |
| `cmd/cub-scout/localcluster.go` | Shared styles/helpers if needed |
| `test/expected-outputs/cli/*.md` | Update expected outputs |

---

## Appendix: Current Command Output Samples

### `map orphans` (current)
```
NAMESPACE           KIND           NAME                    OWNER
argocd              Application    api-gateway             Native
...
```

### `map issues` (current)
```
✗ Kustomization/payment-api in break-glass-demo: ArtifactFailed
✗ Deployment/postgresql in demo-prod: 0/1 ready
...
```

### `map crashes` (current)
```
(identical to map issues)
```

### `map workloads` (current)
```
STATUS  NAMESPACE         NAME                   OWNER      MANAGED-BY             IMAGE
──────  ─────────         ────                   ─────      ──────────             ─────
✓       boutique          cart                   Flux       cart                   podinfo:6.9.4
...
```
