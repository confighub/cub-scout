# UX-BOW: User Experience Benchmark for Observability Workflows

Like xBOW for configuration vulnerability detection, UX-BOW provides a systematic framework for measuring and improving user experience across the ConfigHub tooling ecosystem.

## Philosophy

> "Measure the test user's level of surprise, understanding, ability to make quick progress."

UX-BOW estimates these through **journey difficulty** — how hard is it to complete a task in various scenarios?

## Dimensions

UX-BOW tests across five dimensions:

| Dimension | Values | Example |
|-----------|--------|---------|
| **Cluster** | kind, minikube, eks, gke, aks | Different K8s environments |
| **GitOps Tool** | Flux, ArgoCD, Helm, Native | Management tool in use |
| **Persona** | Developer, Platform Engineer, SRE, Security | User role and expertise |
| **Entry Point** | TUI, CLI, ConfigHub GUI | Where user starts |
| **Goal** | Debug, Audit, Remediate, Discover | What user wants to achieve |

## Scoring System

Each scenario is scored on five metrics (1-5 scale):

| Metric | Question | Weight |
|--------|----------|--------|
| **Discoverability** | Can user find the feature without docs? | 25% |
| **Speed** | How many keystrokes/commands to complete? | 20% |
| **Consistency** | Does behavior match similar actions? | 20% |
| **Recovery** | Can user recover from mistakes? | 15% |
| **Delight** | Does UI/UX feel polished and helpful? | 20% |

### Composite Score

```
UX-BOW Score = (D×0.25) + (S×0.20) + (C×0.20) + (R×0.15) + (L×0.20)
```

Target: **4.0+** composite score across all scenarios.

## Journey Difficulty

Journey difficulty is measured by:

1. **Keystroke Count** — Total keys pressed to complete
2. **Command Count** — Number of distinct commands/actions
3. **Error Rate** — How often users make recoverable errors
4. **Dead Ends** — How often users get stuck with no clear next step
5. **Time to Insight** — How quickly does useful info appear?

### Difficulty Levels

| Level | Keystrokes | Commands | Description |
|-------|------------|----------|-------------|
| **Trivial** | 1-5 | 1 | One action, immediate result |
| **Easy** | 6-15 | 2-3 | Quick workflow, obvious path |
| **Medium** | 16-30 | 4-6 | Multi-step, requires discovery |
| **Hard** | 31-50 | 7-10 | Complex, may need help |
| **Very Hard** | 50+ | 10+ | Expert only, needs documentation |

## Directory Structure

```
test/ux-bow/
├── README.md           # This file
├── scenarios/          # Journey scenario definitions
│   ├── debug-pod-crash.yaml
│   ├── find-orphan-resource.yaml
│   └── ...
├── personas/           # Persona definitions
│   ├── developer.yaml
│   ├── platform-engineer.yaml
│   └── ...
├── matrices/           # Test matrices
│   └── full-matrix.yaml
├── results/            # Test run results
│   └── YYYY-MM-DD/
└── lib/                # Test utilities
    └── ux-bow.sh       # Runner script
```

## Scenario Format

```yaml
# scenarios/debug-pod-crash.yaml
id: debug-pod-crash
name: Debug a crashing pod
category: debug
difficulty_target: easy

description: |
  User sees a pod in CrashLoopBackOff and needs to understand why.

preconditions:
  - cluster: any
  - has_crashing_pod: true

goals:
  - Find the crashing pod
  - Identify the error cause
  - Determine who owns the pod

entry_points:
  tui:
    expected_keystrokes: 8
    expected_commands: 2
    journey:
      - action: "Press 's' for status"
        expect: "See pod list with crash indicators"
      - action: "Navigate to crashing pod, press Enter"
        expect: "See error logs or trace"

  cli:
    expected_keystrokes: 25
    expected_commands: 2
    journey:
      - command: "cub-agent map list -q 'status=CrashLoopBackOff'"
        expect: "List of crashing pods"
      - command: "cub-agent trace deploy/NAME -n NS"
        expect: "Ownership chain and error details"

scoring:
  discoverability:
    tui: 4  # Status view shows crash icons
    cli: 3  # Need to know query syntax
  speed:
    tui: 5  # 2 keystrokes to find
    cli: 3  # Typing query takes time
  consistency:
    tui: 4  # Matches other status checks
    cli: 4  # Query syntax is consistent
  recovery:
    tui: 5  # Easy to navigate back
    cli: 4  # Can re-run with different params
  delight:
    tui: 4  # Visual crash indicators helpful
    cli: 3  # Plain text output
```

## Persona Format

```yaml
# personas/developer.yaml
id: developer
name: Application Developer
expertise_level: intermediate

characteristics:
  - Familiar with kubectl basics
  - Doesn't know GitOps internals
  - Wants quick answers, not deep dives
  - May not have cluster admin access

typical_goals:
  - Debug my failing deployment
  - Find why my config change didn't apply
  - Check if my app is running in prod

expected_knowledge:
  - Basic kubectl commands
  - Pod/Deployment/Service concepts
  - Git branching basics

unknown_concepts:
  - Kustomization CRDs
  - Flux/ArgoCD sync mechanics
  - CCVE categories
```

## Running UX-BOW

```bash
# Run all scenarios for a persona
./test/ux-bow/lib/ux-bow.sh --persona=developer

# Run specific scenario
./test/ux-bow/lib/ux-bow.sh --scenario=debug-pod-crash

# Run full matrix (all combinations)
./test/ux-bow/lib/ux-bow.sh --matrix=full

# Generate report
./test/ux-bow/lib/ux-bow.sh --report
```

## Ralph Mode Integration

UX-BOW is designed for Ralph mode iteration:

1. **Baseline**: Run all scenarios, record scores
2. **Identify**: Find lowest-scoring scenarios
3. **Improve**: Fix discoverability/speed issues
4. **Retest**: Re-run affected scenarios
5. **Repeat**: Until all scores >= 4.0

### Completion Promise

```
Ralph completes when: All UX-BOW scenarios score >= 4.0 composite
```

## Current Status

| Metric | Score |
|--------|-------|
| Scenarios Defined | 10/10 |
| Personas Defined | 4/4 |
| Baseline Run | 2026-01-13 |
| Ralph Iteration | Completed |
| Hallucination Check | Completed |
| Average TUI Score | 4.82 |
| Average CLI Score | 4.49 |
| Average Hub Score | 4.53 |

### Ralph Iteration Results

**Improvements Made:**
- Added `cub-agent map crashes` shortcut command
- Added `cub-agent map orphans` shortcut command
- Added `--since` flag with tab completion for time filtering
- Added query examples to CLI help text
- Added query examples to Hub TUI help overlay
- Improved discoverability of cub-agent commands in both TUIs
- Added 'R' key for Recent changes view in bash TUI (shows workloads sorted by age)
- Added '/' search with query examples to bash TUI
- Added 'a' activity view to Hub TUI (shows recent unit updates, worker/target status)
- **Panel-based TUI layout** - side-by-side panels with navigation always visible
- **Integrated fix flow** - 'f' key in scan view for CCVE remediation with dry-run preview

### Hallucination Check Results (Latest Round)

Corrected scenarios that claimed non-existent features:
- Scenario 01 (debug-pod-crash): Removed claim of j/k navigation - clarified panel layout
- Scenario 03 (trace-ownership): Removed claim of j/k navigation - clarified 't' for trace
- Scenario 07 (gitops-sync-status): Changed 'g' for GitOps to 'p' for Pipelines / 'd' for Drift
- Scenario 02 (orphans): TUI claimed 'o' ownership view - corrected to 'r' Sprawl
- Scenario 06 (filter-by-label): TUI claimed '/' search - then FIXED by adding '/' search
- Scenario 09 (recent): Hub claimed 'a' activity view - then FIXED by adding 'a' activity

**All 10 scenarios now score >= 4.0 across all entry points.**

**Recently Fixed:**
- gitops-sync-status (TUI): 4.8 → 5.0 - Panel layout with visible navigation
- filter-by-label (TUI): 3.15 → 4.6 - Added '/' search with query examples
- find-recent-changes (Hub): 3.15 → 5.0 - Added 'a' activity view
- remediate-ccve (TUI): 3.5 → 5.0 - Added 'f' key for integrated fix flow

## See Also

- [xBOW Benchmark](../../docs/planning/xbow/) — Configuration vulnerability detection benchmark
- [TUI Design Guide](../../.claude/skills/tui-design.md) — TUI component patterns
- [Session 2026-01-13](../../docs/planning/sessions/SESSION-2026-01-13.md) — TUI unification work
