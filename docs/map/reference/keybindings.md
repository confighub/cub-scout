# Keyboard Shortcuts Reference

Complete reference for all TUI keyboard shortcuts.

## Quick Reference

```
Navigation:  ↑/k ↓/j ←/h →/l  Enter  Tab  ]/[ (namespace)
Views:       s w p d o c i u a b x M D G 4 5/A (Hub: P Panel, g Suggest, B Hub/AppSpace, a Toggle)
Actions:     T (trace) S (scan) f/F (fix) i (import) / Q :
Help:        ? (help)  q (quit)
```

---

## Navigation

| Key | Action |
|-----|--------|
| `↑` or `k` | Move up |
| `↓` or `j` | Move down |
| `←` or `h` | Collapse / go back |
| `→` or `l` | Expand / go forward |
| `Enter` | Select / load details / cross-references |
| `Tab` | Switch focus (list ↔ details) |
| `]` | Next namespace |
| `[` | Previous namespace |
| `Escape` | Close overlay / clear filter |

---

## View Switching

| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Health dashboard |
| `w` | Workloads | Resources by owner |
| `p` | Pipelines | GitOps deployers |
| `P` | Pipelines (enhanced) | Visual flow with type grouping |
| `d` | Drift | Out-of-sync resources |
| `D` | Dependencies | Upstream/downstream relations |
| `o` | Orphans | Native (unmanaged) resources |
| `c` | Crashes | Failing workloads |
| `i` | Issues | All problems |
| `u` | Suspended | Paused/stale resources |
| `a` | Apps | Group by app label (Local) / Toggle filter (Hub) |
| `b` | Bypass | Factory bypass detection |
| `x` | Sprawl | Config sprawl analysis |
| `M` | Three Maps | All hierarchies view |
| `G` | Git Sources | Forward trace: Git → Deployers → Resources |
| `4` | Cluster Data | All data sources TUI reads from cluster |
| `5` / `A` | App Hierarchy | Inferred ConfigHub model |
| `B` | Hub/AppSpace | Group spaces into Hub (platform) vs AppSpaces (Hub mode) |
| `P` | Panel | WET↔LIVE side-by-side (Hub mode) |
| `g` | Suggest | Recommend Units from cluster (Hub mode) |

---

## Search & Filter

| Key | Action |
|-----|--------|
| `/` | Start search |
| `n` | Next search match |
| `N` | Previous search match |
| `f` | Toggle filter mode (hide non-matching) |
| `Q` | Open saved queries |
| `:` | Open command palette |
| `Escape` | Clear search/filter |

---

## Actions

| Key | Action | Mode |
|-----|--------|------|
| `T` | Trace ownership chain | Local |
| `S` | Scan for CCVEs | Local |
| `f` | Preview fix (dry-run) | Scan results |
| `F` | Apply fix | Scan results |
| `i` | Import wizard | ConfigHub |
| `c` | Create resource | ConfigHub |
| `d` or `x` | Delete resource | ConfigHub |
| `o` | Open in browser | ConfigHub |
| `r` | Refresh data | Both |

---

## Mode Switching

| Key | Action |
|-----|--------|
| `H` | Switch to ConfigHub hierarchy (from local) |
| `L` | Switch to local cluster (from ConfigHub) |
| `O` | Switch organization (ConfigHub mode) |

---

## Help & Exit

| Key | Action |
|-----|--------|
| `?` | Show help overlay |
| `q` | Quit |
| `Ctrl+C` | Force quit |

---

## ConfigHub Mode (--hub)

### Tree Navigation

| Key | Action |
|-----|--------|
| `↑/k` | Move to previous item |
| `↓/j` | Move to next item |
| `←/h` | Collapse node |
| `→/l` | Expand node |
| `Enter` | Load details in right pane |

### ConfigHub Actions

| Key | Action |
|-----|--------|
| `i` | Import workloads from cluster |
| `c` | Create new space/unit/target |
| `d` or `x` | Delete selected resource |
| `o` | Open in ConfigHub web |
| `O` | Switch organization |
| `B` | Toggle Hub/AppSpace view (group by platform vs app teams) |
| `a` | Activity view (recent changes) |

---

## Vim-Style Navigation

The TUI supports vim-style navigation:

| Vim Key | Standard Key | Action |
|---------|--------------|--------|
| `j` | `↓` | Down |
| `k` | `↑` | Up |
| `h` | `←` | Left / collapse |
| `l` | `→` | Right / expand |
| `gg` | `Home` | Go to top |
| `G` | `End` | Go to bottom |

---

## Search Mode

When search is active (`/`):

| Key | Action |
|-----|--------|
| Type | Add to search query |
| `Enter` | Execute search |
| `Escape` | Cancel search |
| `Backspace` | Delete character |

After search:

| Key | Action |
|-----|--------|
| `n` | Jump to next match |
| `N` | Jump to previous match |
| `f` | Toggle filter (show only matches) |
| `Escape` | Clear search |

---

## Command Palette (`:`)

Type commands or queries directly:

```
:owner=Flux                    # Filter by owner
:namespace=prod*               # Filter by namespace
:owner=Flux AND status=Ready   # Complex query
```

| Key | Action |
|-----|--------|
| Type | Enter command/query |
| `Enter` | Execute |
| `Escape` | Cancel |
| `Tab` | Autocomplete |

---

## Saved Queries (`Q`)

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate queries |
| `Enter` | Apply selected query |
| `Escape` | Close without applying |

Available queries:
- `all` - All resources
- `orphans` - Native only
- `gitops` - Non-native only
- `flux` - Flux only
- `argo` - ArgoCD only
- `helm` - Helm only
- `prod` - Production namespaces
- `dev` - Development namespaces

---

## Cheat Sheet

Print this and keep handy:

```
┌─────────────────────────────────────────────────────────────────┐
│                     CUB-AGENT MAP SHORTCUTS                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  NAVIGATION       VIEWS              ACTIONS                    │
│  ↑/k  Up          s  Status          T  Trace                   │
│  ↓/j  Down        w  Workloads       S  Scan                    │
│  ←/h  Collapse    p  Pipelines       f  Preview fix             │
│  →/l  Expand      d  Drift           F  Apply fix               │
│  Tab  Focus       o  Orphans         i  Import                  │
│  Enter Select     c  Crashes         /  Search                  │
│  ]/[  Namespace   u  Suspended       Q  Queries                 │
│                   a  Apps            :  Command                 │
│                   D  Dependencies                               │
│  HELP             G  Git Sources     MODE SWITCH                │
│  ?  Help          M  Three Maps      H  → ConfigHub             │
│  q  Quit          4  Cluster Data    L  → Local                 │
│                   5/A App Hierarchy  B  Hub/AppSpace (Hub)      │
│                   P  Panel (Hub)     a  Toggle filter (Hub)     │
│                   g  Suggest (Hub)                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## See Also

- [Views Reference](views.md) - What each view shows
- [Query Syntax](query-syntax.md) - Query language
