# State and Snapshots in cub-scout

cub-scout has two distinct state concepts that must not be conflated.

---

## Session State

**Purpose:** Resume your own work.

Session state is personal UI convenience:
- Last active view/lens
- Expanded/collapsed nodes
- Search filters
- Cursor position
- Scroll position

**Properties:**
- Local only (stored in `~/.cub-scout/sessions/`)
- Mutable
- Not sanitized
- Not deterministic
- **Not shareable**

Session state is never embedded in snapshots.

---

## Shareable Views (Snapshots)

**Purpose:** Explain your work to others.

A snapshot is a frozen, deterministic capture of **one hierarchy map** at a point in time:
- What the cluster looked like
- Why something was broken
- Who owned what

**Properties:**
- Immutable once created
- Sanitized (no secrets, cluster name redacted)
- Deterministic (same input = same output)
- Replayable without cluster access
- **Shareable**

---

## The Distinction

| Aspect | Session State | Snapshot |
|--------|---------------|----------|
| Purpose | Resume work | Explain work |
| Audience | Me | Others |
| Mutability | Mutable | Immutable |
| Contains secrets | Possibly | Never |
| Requires cluster | No | No |
| Shareable | No | Yes |

---

## Related Concepts

### Graph Export vs Snapshot

| Artifact | Purpose |
|----------|---------|
| Graph export | Data artifact for CI/diagrams (`cub-scout graph export`) |
| Snapshot | Replayable debugging artifact with context (`cub-scout snapshot create`) |

Graph exports are raw data. Snapshots include diagnostics, explanations, and are viewable in the TUI.

### Offline Mode

Snapshots enable **offline replay** as a first-class workflow:
- Incident review without cluster access
- Onboarding with real examples
- Security-restricted environments

Use `cub-scout snapshot view <file>` to replay any snapshot.

---

## Future: Connected Mode Enrichment

In future releases (v0.6+), Connected Mode may enrich snapshots with:
- Intent metadata from ConfigHub
- Historical context ("what changed")
- Revision information

Snapshots will remain standalone-viewable even when enriched.
