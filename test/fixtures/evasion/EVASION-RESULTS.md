# Evasion Testing Results - Session 005

## Summary

| # | Evasion Attempt | Result | Notes |
|---|-----------------|--------|-------|
| 01 | HPA minReplicas=0 | **BLOCKED** | Kubernetes rejects minReplicas < 1 |
| 02 | Service empty selector `{}` | **EVADED** | Design decision - empty selectors skipped |
| 03 | PDB string "100%" | **DETECTED** | String format works correctly |
| 04 | Ingress with ExternalName | **NOT EVASION** | ExternalName service exists correctly |
| 05 | NetworkPolicy matchExpressions | **DETECTED** | Fixed display bug in this session |

## Details

### Evasion 01: HPA minReplicas=0

**File:** `evasion-01-hpa-min-zero.yaml`

**Attempt:** Set minReplicas=0 to create an HPA that could scale to zero.

**Result:** Kubernetes validation rejects:
```
spec.minReplicas: Invalid value: 0: must be greater than or equal to 1
spec.metrics: Forbidden: must specify at least one Object or External metric to support scaling to zero replicas
```

**Conclusion:** Built-in Kubernetes protection. Not a detection gap.

---

### Evasion 02: Service Empty Selector

**File:** `evasion-02-service-empty-selector.yaml`

**Attempt:** Create a ClusterIP service with empty selector `{}`.

**Result:** Not detected - empty selectors are intentionally skipped.

**Reasoning:** Empty selectors are valid for:
- ExternalName services
- Services with manually managed Endpoints
- Headless services for StatefulSets

**Potential Enhancement:** Could detect ClusterIP services with empty selector AND no Endpoints, but this requires additional logic to check for manual Endpoint management.

---

### Evasion 03: PDB String Percentage

**File:** `evasion-03-pdb-string-percentage.yaml`

**Attempt:** Use string "100%" instead of integer for minAvailable.

**Result:** Correctly detected as CCVE-2025-0678:
```
PodDisruptionBudget/evasion-pdb-string-percent
Reason: MinAvailable100Percent
Message: minAvailable: 100% blocks all evictions
```

**Conclusion:** Detection handles both string and integer formats.

---

### Evasion 04: Ingress with ExternalName Service

**File:** `evasion-04-ingress-externalname.yaml`

**Attempt:** Point ingress to ExternalName service instead of ClusterIP.

**Result:** Not detected (correctly).

**Reasoning:** ExternalName service exists and is a valid backend. The ingress is not dangling.

**Conclusion:** Not an evasion - correct behavior.

---

### Evasion 05: NetworkPolicy matchExpressions

**File:** `evasion-05-netpol-matchexpressions.yaml`

**Attempt:** Use matchExpressions instead of matchLabels for pod selector.

**Result:** Detected, but display was broken (showed empty selector).

**Fix Applied:** Added `checkPodsMatchExpressions()` and `buildLabelSelectorString()` helper functions to properly handle matchExpressions.

**After Fix:**
```
NetworkPolicy/evasion-netpol-expressions
Target: Pod/app In (nonexistent-app-expression)
Message: NetworkPolicy podSelector matches no pods: app In (nonexistent-app-expression)
FIX: kubectl get pods -n default --selector='app in (nonexistent-app-expression)'
```

**Conclusion:** Detection works, fixed display/command output.

---

## Code Changes

### Files Modified

- `pkg/agent/state_scanner.go`:
  - Enhanced `scanDanglingNetworkPolicies()` to handle matchExpressions
  - Added `checkPodsMatchExpressions()` helper
  - Added `buildLabelSelectorString()` helper

- `cmd/cub-scout/scan.go`:
  - Added `--dangling` flag
  - Added dangling result output formatting
  - Added `outputDanglingFinding()` function

### New Test Fixtures

- `test/fixtures/dangling/` - 4 fixtures for basic dangling detection
- `test/fixtures/evasion/` - 5 fixtures for evasion testing

---

## Recommendations

1. **Low Priority:** Consider detecting ClusterIP services with empty selector + no Endpoints
2. **Complete:** matchExpressions now properly detected and displayed
3. **No Action:** Kubernetes built-in validation handles HPA minReplicas=0
