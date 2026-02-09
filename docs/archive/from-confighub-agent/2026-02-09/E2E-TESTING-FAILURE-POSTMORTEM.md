# E2E Testing Failure Postmortem: 8-Day Broken Worker

**Date:** 2026-01-13
**Incident:** Workers were broken for 8 days, no tests caught it

## The Problem

We ran for **8 days** with a broken worker. During this time:
- **Connected mode demos** would have failed (`demo connected`, fleet queries)
- **KubeCon demo spaces** would have been unusable (apptique, appchat, etc.)
- **Tutorial flows** requiring workers would have failed
- None of our tests caught this

**What still worked:** Standalone demos (`demo quick`, `demo ccve`, scenarios) don't need workers and would have worked fine.

This is a **testing gap for connected features** that could have embarrassed us in front of customers.

## Root Cause Analysis

### Why Tests Didn't Catch It

1. **Connected mode tests were opt-in (`--connected` flag)**
   - Default test run: `./test/run-all.sh`
   - Connected tests only run with: `./test/run-all.sh --connected`
   - Nobody was consistently running connected tests

2. **Worker status was a warning, not a failure**
   - `mini-tck --connected` would warn about unhealthy workers
   - But warnings don't block execution
   - You can still run demos that would fail

3. **Demos, examples, and use cases didn't check dependencies**
   - `./test/atk/demo connected` would just fail confusingly
   - No pre-flight check for "are workers actually working?"

4. **No daily/session health checks**
   - Nothing enforced checking worker health before starting work
   - The first failure would be deep in a demo, not at the start

### The Core Insight

Demos, examples, and use cases are **end-to-end experiences** that require workers to be healthy. They're not isolated unit tests - they're integration tests that depend on external services.

We were treating worker health as "nice to have" when it's actually "mandatory for connected features."

## Prevention Measures Implemented

### 1. Demo Requirements Manifest (`test/atk/DEMO-REQUIREMENTS.yaml`)

Each demo/example now declares its specific requirements:

```yaml
demos:
  quick:
    standalone: true      # No workers needed
    cluster: true
    workers: []

  connected:
    standalone: false     # Workers required
    cub_auth: true
    workers:
      - space: current
        count: 1
```

### 2. Per-Demo Requirement Checking

The demo script now checks requirements specific to each demo:

```bash
# Standalone demos - just check cluster access
$ ./test/atk/demo quick         # Works without workers

# Connected demos - check workers first
$ ./test/atk/demo connected
✗ NO WORKERS in space 'tutorial'
  Start a worker: cub worker run <slug> --space tutorial
```

### 3. Worker Health Check Script (`test/preflight/worker-health`)

Check workers for specific spaces (not all spaces):

```bash
./test/preflight/worker-health --space tutorial  # Check specific space
./test/preflight/worker-health                   # Check test spaces only
./test/preflight/worker-health --json            # Output for CI
```

### 4. Demo Readiness Check (`test/preflight/demo-ready`)

Full check for connected features:

```bash
./test/preflight/demo-ready                 # Check all E2E dependencies
./test/preflight/demo-ready --space X       # Check specific space
./test/preflight/demo-ready --json          # Output for CI
```

### 5. Auto-Detection of Connected Mode (`test/run-all.sh`)

```bash
./test/run-all.sh                           # Auto-detects if authenticated
./test/run-all.sh --skip-connected          # Explicitly skip connected
```

## Testing Strategy Going Forward

### E2E Test Matrix

Each demo/example has specific requirements. Tests verify requirements before running.

| Demo | Standalone | Workers | Spaces |
|------|------------|---------|--------|
| `demo quick` | Yes | None | - |
| `demo ccve` | Yes | None | - |
| `demo query` | Yes | None | - |
| `demo healthy` | Yes | None | - |
| `demo unhealthy` | Yes | None | - |
| `scenario *` | Yes | None | - |
| `demo connected` | **No** | 1+ | current space |
| KubeCon demos | **No** | 1+ per space | apptique-*, appchat-*, appvote-*, traderx |

### When to Check Workers

| Situation | Check Workers? | Command |
|-----------|----------------|---------|
| Running standalone demo | No | Just run it |
| Running connected demo | **Yes** | `./test/preflight/worker-health --space <space>` |
| Before customer demo | **Yes** | `./test/preflight/demo-ready` |
| Starting work on connected features | **Yes** | `./test/preflight/worker-health` |

### Test Spaces That May Need Workers

| Space | Workers | When Needed |
|-------|---------|-------------|
| `tutorial` | 2 | Tutorial flows, getting started |
| `platform-dev` | 1 | Platform team demos |
| `platform-prod` | 1 | Platform team demos |
| `apptique-dev` | 1 | KubeCon e-commerce demo |
| `apptique-prod` | 1 | KubeCon e-commerce demo |

### Starting Workers (Only When Needed)

```bash
# Only start workers when running connected features
cub context set space tutorial
cub worker run dev

# Or for KubeCon demos
cub context set space apptique-dev
cub worker run demo-worker
```

## Lessons Learned

1. **Not all demos need workers**
   - Standalone demos work fine without ConfigHub connection
   - Only connected features need workers

2. **Each demo should declare its requirements**
   - Use `DEMO-REQUIREMENTS.yaml` manifest
   - Check requirements specific to what you're running

3. **Fail early for connected features**
   - `demo connected` checks workers before running
   - Better to fail at startup than halfway through

4. **Don't over-check**
   - Don't require workers at session start (they're not always needed)
   - Check workers when running features that need them

5. **Context matters**
   - Different demos need different spaces/workers
   - Workers may need to be shut down before starting new ones

## Files Changed

| File | Change |
|------|--------|
| `test/atk/DEMO-REQUIREMENTS.yaml` | **NEW** - Per-demo requirements manifest |
| `test/preflight/worker-health` | **NEW** - Worker health check script |
| `test/preflight/demo-ready` | **NEW** - Demo readiness check |
| `test/run-all.sh` | Auto-detect connected mode |
| `test/atk/demo` | Check per-demo requirements, require workers for connected |
| `CLAUDE.md` | Contextual guidance (when workers are needed) |
| `docs/planning/E2E-TESTING-FAILURE-POSTMORTEM.md` | **NEW** - This document |

## Next Steps

- [ ] Add CI job for connected demos (runs when workers are available)
- [ ] Add KubeCon demo space verification to E2E tests
- [ ] Consider webhook/notification when workers disconnect
- [ ] Add more demos to requirements manifest as they're created

---

*This postmortem documents the 2026-01-13 incident and the prevention measures implemented. The key insight: workers are needed contextually, not universally. Each demo declares its requirements.*
