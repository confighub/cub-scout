# URGENT: Worker Disconnect Visibility

**Created:** 2026-01-12
**Priority:** URGENT
**Status:** Open

## Problem

Worker `cluster-worker` was disconnected for **8 days** (2026-01-04 to 2026-01-12) without anyone noticing. This is a critical visibility gap.

## Issues

### 1. No Proactive Alerting
- Worker disconnects silently
- No email/notification when worker goes offline
- User must manually check `cub worker list` to discover issue

### 2. TUI Doesn't Show Worker Status Prominently
- The `map confighub` view shows workers in a small section
- Disconnected workers should trigger a **warning banner**
- User should see "WARNING: Worker disconnected for 8 days" immediately

### 3. No Historical Tracking
- No way to see when worker disconnected
- No audit trail of worker uptime/downtime
- `last-seen` shows last activity but not duration of outage

## Required Fixes

### Fix 1: TUI Warning Banner (cub-agent)
**File:** `cmd/cub-agent/hierarchy.go`

When rendering ConfigHub view, check worker status and show warning:
```
┌────────────────────────────────────────────────────────────────────────┐
│  ⚠️  WARNING: Worker 'cluster-worker' disconnected since 2026-01-04   │
│      Run: cub worker run cluster-worker                               │
└────────────────────────────────────────────────────────────────────────┘
```

### Fix 2: CLI Warning (cub)
When running any `cub` command, warn if worker has been disconnected > 1 hour:
```
⚠️  Worker 'cluster-worker' disconnected for 8d 1h. Run: cub worker run cluster-worker
```

### Fix 3: Email/Webhook Alerts (ConfigHub Backend)
- Alert when worker disconnects for > 1 hour
- Alert when worker reconnects after outage
- Configurable thresholds per space

### Fix 4: Status Dashboard
- Show worker uptime history
- Show last N disconnect events
- Quick health check at login

## Acceptance Criteria

1. [x] TUI shows warning banner when any worker is disconnected (DONE - 2026-01-12)
2. [ ] Warning includes duration of disconnect
3. [x] Warning includes command to reconnect (DONE - 2026-01-12)
4. [ ] cub CLI warns on any command when worker disconnected > 1 hour
5. [ ] Documentation updated on worker monitoring best practices

## Implementation Status (2026-01-12)

### Completed
- **Shell TUI (`test/atk/map-confighub`)**: Added `render_worker_warning()` function that shows a red double-bordered warning box when any worker has Disconnected/NotReady/unknown status
- **Go TUI (`cmd/cub-agent/hierarchy.go`)**: Added `getDisconnectedWorkers()` and `renderWorkerWarning()` functions that display warning banner at top of hierarchy view

### Remaining
- Add duration of disconnect to warning message
- Add warning to cub CLI commands (backend change needed)

## Notes

This is a real incident - worker was down for 8 days during active development. This should never happen silently.

## Related

- Worker last seen: 2026-01-04 19:39:22
- Worker reconnected: 2026-01-12 20:41:57
- Space: platform-dev
