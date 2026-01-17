# TUI Session State Persistence

**Status:** Design spec (not yet implemented)

**Source:** Brian Grant / Jesper Joergensen feedback

> "You might want to save where you are session state. Otherwise, you have to start over from scratch and go back to where you are."

---

## Problem

The TUI import workflow is multi-step:

```
1. Scan cluster (select namespaces)
2. Review detected workloads
3. Select/confirm App Space structure
4. Connect to ConfigHub
5. Start worker
6. Wait for targets to register
7. Verify import complete
```

If user exits at step 4, they lose everything and must restart from step 1.

---

## Solution: Session State File

### Location

```
~/.confighub/sessions/
├── import-latest.json          # Symlink to most recent
├── import-2026-01-09-143022.json
└── import-2026-01-08-091544.json
```

### Session State Structure

```json
{
  "version": "1.0",
  "type": "import",
  "created_at": "2026-01-09T14:30:22Z",
  "updated_at": "2026-01-09T14:45:33Z",
  "cluster_context": "kind-atk",
  "step": "worker_connecting",
  "steps_completed": ["scan", "review", "select", "connect"],

  "scan_result": {
    "namespaces": ["payment-prod", "order-prod"],
    "workloads_detected": 12,
    "owners": {
      "Flux": 8,
      "ArgoCD": 3,
      "Native": 1
    }
  },

  "selections": {
    "app_space": "payment-team",
    "hub": "platform-team",
    "selected_workloads": [
      {"namespace": "payment-prod", "name": "payment-api", "owner": "Flux"},
      {"namespace": "payment-prod", "name": "payment-worker", "owner": "Flux"}
    ]
  },

  "confighub": {
    "authenticated": true,
    "org": "acme-corp",
    "user": "user@example.com",
    "units_created": ["payment-api", "payment-worker"],
    "worker_slug": "dev",
    "worker_status": "connecting"
  }
}
```

---

## UX Flow

### On TUI Start

```
┌─ RESUME SESSION? ─────────────────────────────────────────┐
│                                                           │
│  Found previous session from 2 hours ago:                 │
│                                                           │
│  Cluster: kind-atk                                        │
│  Step: Worker connecting (step 5 of 7)                    │
│  Workloads: 2 selected, 2 imported                        │
│                                                           │
│  [R] Resume    [N] New session    [D] Delete & start new  │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

### During TUI

Auto-save on every step completion:

```bash
# After each significant action
save_session_state()
```

### On TUI Exit

```
┌─ EXIT ────────────────────────────────────────────────────┐
│                                                           │
│  Session saved. You can resume later with:                │
│                                                           │
│    ./test/atk/map-import --resume                         │
│                                                           │
│  Or start fresh:                                          │
│                                                           │
│    ./test/atk/map-import --new                            │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

---

## Implementation

### Session Manager Functions

```bash
# In test/atk/lib/session.sh

SESSION_DIR="$HOME/.confighub/sessions"
SESSION_FILE="$SESSION_DIR/import-latest.json"

session_init() {
    mkdir -p "$SESSION_DIR"
}

session_exists() {
    [[ -f "$SESSION_FILE" ]]
}

session_load() {
    if session_exists; then
        cat "$SESSION_FILE"
    fi
}

session_save() {
    local step="$1"
    local data="$2"

    local timestamp=$(date +%Y-%m-%dT%H:%M:%SZ)
    local session_id=$(date +%Y-%m-%d-%H%M%S)

    # Update or create session file
    jq --arg step "$step" \
       --arg updated "$timestamp" \
       '.step = $step | .updated_at = $updated' \
       "$SESSION_FILE" > "$SESSION_FILE.tmp" && mv "$SESSION_FILE.tmp" "$SESSION_FILE"

    # Also save timestamped backup
    cp "$SESSION_FILE" "$SESSION_DIR/import-$session_id.json"
}

session_clear() {
    rm -f "$SESSION_FILE"
}

session_get_step() {
    jq -r '.step' "$SESSION_FILE" 2>/dev/null || echo "none"
}
```

### Integration Points

| TUI Script | Session Actions |
|------------|-----------------|
| `map-import` | Load/save full import state |
| `map` | Save current view preferences |
| `map-confighub` | Save ConfigHub connection state |

---

## Session Expiry

- Sessions older than 24 hours: Prompt "session may be stale"
- Sessions older than 7 days: Auto-archive to `sessions/archive/`
- Keep last 10 sessions, delete older

---

## CLI Flags

```bash
./test/atk/map-import              # Auto-detect: resume if exists, else new
./test/atk/map-import --resume     # Resume existing session (error if none)
./test/atk/map-import --new        # Force new session (archive existing)
./test/atk/map-import --status     # Show current session state without starting
```

---

## What Gets Persisted

| Data | Persisted | Notes |
|------|-----------|-------|
| Namespace selections | Yes | User choices |
| Workload selections | Yes | What to import |
| App Space name | Yes | User input |
| ConfigHub auth | No | Use `cub context get` |
| Worker status | Yes | But re-check on resume |
| Scan results | Yes | Avoids re-scan |
| Import progress | Yes | Units created |

---

## Connected Mode: Cloud Session Sync

When TUI is connected to ConfigHub.com, session state can sync to the cloud.

### Benefits

| Feature | What It Enables |
|---------|-----------------|
| **Cross-device resume** | Start import on laptop, continue on desktop |
| **Team visibility** | See what colleagues are working on |
| **Audit trail** | Track import attempts over time |
| **Handoff** | "Alex started this import, can you finish?" |
| **Recovery** | Machine crashes mid-import → resume elsewhere |

### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│  TUI (Local)                                                    │
│  ┌──────────────┐                                               │
│  │ Session State│──── sync on save ────▶ ConfigHub API          │
│  └──────────────┘                              │                │
│         ▲                                      │                │
│         │                                      ▼                │
│    load on start ◀───────────────────── Session Store           │
│                                          (per-user, per-org)    │
└─────────────────────────────────────────────────────────────────┘
```

### Session Visibility in GUI

ConfigHub GUI could show:

```
┌─ ACTIVE IMPORT SESSIONS ───────────────────────────────────────┐
│                                                                │
│  user@example.com                                          │
│  └─ kind-atk cluster                                           │
│     ├─ Step: Worker connecting (5/7)                           │
│     ├─ Started: 2 hours ago                                    │
│     └─ Workloads: 2 selected, 2 imported                       │
│                                                                │
│  brian@confighub.com                                           │
│  └─ prod-east cluster                                          │
│     ├─ Step: Review workloads (2/7)                            │
│     └─ Workloads: 15 detected                                  │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### API Endpoints (Proposed)

```
POST /api/v1/sessions/import          # Create/update session
GET  /api/v1/sessions/import/current  # Get my active session
GET  /api/v1/sessions/import          # List all sessions (org-wide)
DELETE /api/v1/sessions/import/{id}   # Clear session
```

### Session Ownership

| Scope | Who Can See | Who Can Resume |
|-------|-------------|----------------|
| Personal | Only me | Only me |
| Team | Team members | Team members (with permission) |
| Org | Org admins | Org admins |

Default: Personal scope. Explicit share to enable team handoff.

### Offline/Disconnected Behavior

```
Connected:    Auto-sync to cloud on each step
Disconnected: Save locally, sync when reconnected
Conflict:     Cloud wins (with local backup)
```

### Implementation Phases

| Phase | Scope | Priority |
|-------|-------|----------|
| **Phase 1** | Local file persistence | P2 (now) |
| **Phase 2** | Cloud sync (personal) | P3 |
| **Phase 3** | Team visibility + handoff | P4 |

Phase 1 ships first — Phase 2/3 are enhancements when connected.

---

## Integration with TUI Logs

TUI already logs activity to `~/.confighub/logs/tui-import-YYYY-MM-DD-HHMMSS.log`.

### Logs vs Session State

| Aspect | Logs | Session State |
|--------|------|---------------|
| **Purpose** | Debug, audit, troubleshoot | Resume, handoff |
| **Format** | Human-readable text | Structured JSON |
| **Retention** | Long-term archive | Short-term (7 days active) |
| **Content** | Everything that happened | Current state only |
| **Use case** | "What went wrong?" | "Where did I leave off?" |

### Linking Logs to Sessions

Session state includes log file reference:

```json
{
  "session_id": "import-2026-01-09-143022",
  "log_file": "~/.confighub/logs/tui-import-2026-01-09-143022.log",
  ...
}
```

On resume, TUI can:
1. Show link to previous log: "View previous session log?"
2. Continue logging to new file (with session ID prefix)
3. In connected mode: sync logs to ConfigHub for team visibility

### Cloud Sync: Logs + Sessions

When connected to ConfigHub:

```
┌─ Session State ─────────────────────────────────────────────────┐
│  Lightweight JSON                                               │
│  Syncs on every step                                            │
│  Used for: resume, progress visibility                          │
└─────────────────────────────────────────────────────────────────┘

┌─ Log Files ─────────────────────────────────────────────────────┐
│  Full activity log                                              │
│  Syncs on session complete (or on error)                        │
│  Used for: troubleshooting, audit trail                         │
└─────────────────────────────────────────────────────────────────┘
```

GUI can show both:
- Session progress (real-time)
- Full log (on demand, for debugging)

---

## TUI: Session State Panel

When connected to ConfigHub, the TUI shows a session state panel in the ConfigHub tab:

```
┌─ SESSION STATE ────────────────────────────────────────────────┐
│                                                                │
│  Last session: 2026-01-09 14:30 (2 hours ago)                 │
│  Step: Worker connecting (5/7)                                 │
│  Workloads: 12 detected, 5 imported                            │
│                                                                │
│  Log file: ~/.confighub/logs/tui-import-2026-01-09-143022.log │
│                                                                │
│  Last upload to ConfigHub: 2026-01-09 14:45                   │
│  ✓ Session synced                                              │
│                                                                │
│  [R] Resume session    [N] New session    [V] View log        │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Panel States

**No previous session:**
```
┌─ SESSION STATE ────────────────────────────────────────────────┐
│                                                                │
│  No previous session found.                                    │
│                                                                │
│  [I] Start import                                              │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

**Session found, not synced (disconnected mode):**
```
┌─ SESSION STATE ────────────────────────────────────────────────┐
│                                                                │
│  Last session: 2026-01-09 14:30 (2 hours ago)                 │
│  Step: Review workloads (2/7)                                  │
│  Workloads: 12 detected                                        │
│                                                                │
│  Log file: ~/.confighub/logs/tui-import-2026-01-09-143022.log │
│                                                                │
│  ○ Not synced to ConfigHub (offline)                          │
│                                                                │
│  [R] Resume session    [N] New session    [V] View log        │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

**Session complete:**
```
┌─ SESSION STATE ────────────────────────────────────────────────┐
│                                                                │
│  Last import: 2026-01-09 15:00 (1 hour ago)                   │
│  ✓ Complete — 5 units imported                                 │
│                                                                │
│  Log file: ~/.confighub/logs/tui-import-2026-01-09-143022.log │
│  Last upload: 2026-01-09 15:00 ✓                              │
│                                                                │
│  [I] New import    [V] View log    [H] View in ConfigHub      │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Panel Location

The session state panel appears in the `map-import` TUI when viewing the ConfigHub tab:

```
┌─ ⚡ CONFIGHUB MAP ──────────────────────────────────────────────┐
│                                                                 │
│  Tab: [Cluster] [ConfigHub] [Import]                           │
│                                                                 │
│  ┌─ CONNECTION ──────────────┬─ SESSION STATE ───────────────┐ │
│  │                           │                                │ │
│  │  ✓ Authenticated          │  Last session: 2h ago         │ │
│  │  ✓ 5 Units imported       │  Step: 5/7                    │ │
│  │  ✓ Worker: dev            │  Log: ~/.confighub/logs/...   │ │
│  │  ✓ 1 Target               │  Last upload: 14:45 ✓         │ │
│  │                           │                                │ │
│  │  ALL SET                  │  [R]esume  [N]ew  [V]iew log  │ │
│  │                           │                                │ │
│  └───────────────────────────┴────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Priority

**P2** — Important for UX but not blocking launch.

Add to TODO-CONFIGHUB-AGENT.md under "Priority 2: User Journeys" as it directly impacts the import journey.
