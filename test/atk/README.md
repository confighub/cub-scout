# cub-scout Test Infrastructure

> **DEPRECATED (2026-01-14):** The bash-based ATK scripts are deprecated in favor of the Go TUI and Go tests.
>
> **Use instead:**
> - `cub-scout map` — Local cluster TUI (replaces `./map`)
> - `cub-scout map --hub` — ConfigHub hierarchy TUI (replaces `./map-confighub`)
> - `cub-scout scan` — CCVE scanner (replaces `./scan`)
> - `cub-scout trace` — Ownership tracing
> - `go test ./...` — All tests (replaces `./verify`)
>
> The Go TUI has feature parity plus: snapshot persistence, cross-reference navigation, dependencies view, and more.
> See `docs/map/reference/keybindings.md` for the full keybinding reference.

---

Test tools for validating cub-scout functionality.

## Tools

### `map` - Cluster State Visualization

Displays cluster resources with ownership tracking and ConfigHub hierarchy.

**Usage:**
```bash
./map                    # Show cluster overview
./map status            # Hero banner with health
./map problems          # Failed resources only
./map pipelines         # GitOps pipelines
./map deployers         # Deployers (Flux/Argo)
./map sources           # Git sources
./map workloads         # All workloads
./map suspended         # Suspended resources
```

**ConfigHub Hierarchy Display:**

Without API (offline mode):
```
ConfigHub Resources:
Hierarchy: Hub → App Space → Application → Variant → Cluster

  demo-prod (Space ID: 550e8400-e29b-41d4-a716-446655440000)
    └── backend @ rev 42  [atk-confighub-basic/backend]
    └── Application: payment-service
        └── [variant=dev] @ rev 42  [atk-confighub-variant/payment-service-dev]

  💡 Set CONFIGHUB_TOKEN to see full hierarchy (Hub/Organization names)
```

With API (set `CONFIGHUB_TOKEN`):
```
ConfigHub Resources:
Hierarchy: Hub → App Space → Application → Variant → Cluster

  Hub: My Platform
    └── App Space: demo-prod (ID: 550e8400-e29b-41d4-a716-446655440000)
      └── backend @ rev 42  [atk-confighub-basic/backend]
      └── Application: payment-service
          └── [variant=dev] @ rev 42  [atk-confighub-variant/payment-service-dev]
```

**Variant Inference:**

The tool automatically detects application/variant patterns in unit slugs:
- `payment-service-dev` → Application: payment-service, variant: dev
- `api-staging` → Application: api, variant: staging
- `backend-prod` → Application: backend, variant: prod

Supported variant names: `dev`, `staging`, `prod`, `qa`, `test`

### `scan` - CCVE Scanner

Scans cluster for ConfigHub Common Vulnerabilities and Errors.

**Simple usage (Level 1 - CLI):**
```bash
./scan                           # Scan all resources
./scan traefik                   # Scan Traefik resources only
./scan --ccve CCVE-2025-0027     # Scan for specific CCVE
./scan traefik --auto-fix        # Scan and create fixes
./scan --severity high           # High severity only
```

**Config-based (Level 2 - Team Standards):**
```yaml
# .confighub/scans/traefik.yaml
scan: traefik
schedule: "0 */6 * * *"
notify: slack:#platform-security
auto-fix: false
severity: high
```

**Behind the scenes:** The scan tool queries the Map, applies CCVE detection Functions, and optionally creates changesets (Action with side effects).

### `verify` - Ownership Verification

Verifies ownership detection logic.

```bash
./verify                # Verify all ownership patterns
```

### `verify-connected` - Connected Mode Verification

Verifies ConfigHub connected mode works correctly with workers and targets.

```bash
./verify-connected              # Full verification
./verify-connected --quick      # Skip slow hierarchy tests
./verify-connected --verbose    # Show detailed output
```

**What it verifies:**
- Preflight requirements (cub CLI, authentication, active space)
- Worker connected and slug not null
- Target exists and slug not null
- `map confighub` shows hierarchy with workers/targets
- `map --mode=admin` produces valid output
- `map --mode=fleet` produces valid output
- No null/unknown values in output (prevents issue #1)
- ConfigHub API helper works correctly

### `confighub-api` - ConfigHub API Helper

Helper script for querying ConfigHub API.

```bash
./confighub-api space SPACE_ID           # Get space+org details
./confighub-api unit SPACE_ID UNIT_SLUG  # Get unit details
```

Requires: `CONFIGHUB_TOKEN` environment variable or `cub auth login`

## Test Fixtures

### `fixtures/confighub-basic.yaml`
Basic ConfigHub-managed resources without variant pattern.

### `fixtures/confighub-variant.yaml`
ConfigHub-managed resources with variant-style naming (app-variant pattern).

### `fixtures/flux-basic.yaml`
Flux CD managed resources.

### `fixtures/argo-basic.yaml`
Argo CD managed resources.

## API Integration

The map tool can query the ConfigHub API to enrich hierarchy display:

**Setup:**
```bash
# Option 1: Set token directly
export CONFIGHUB_TOKEN="your-token-here"

# Option 2: Use cub CLI
cub auth login
# Token will be auto-discovered from ~/.config/confighub/context.json
```

**What the API provides:**
- Organization (Hub) name
- Full Space details
- Future: Target/Cluster mapping

**How it works:**
1. Map tool detects ConfigHub resources via `confighub.com/UnitSlug` label
2. Extracts SpaceID from annotations
3. Calls `confighub-api space <SPACE_ID>` to get Space+Organization data
4. Displays full hierarchy: Hub → App Space → Application → Variant

**Offline mode:**
If `CONFIGHUB_TOKEN` is not available, the tool falls back to annotation-only display.

## Demos

Interactive demos to show the agent in action:

```bash
./demo --list              # List all demos
./demo quick               # 30-second ownership demo
./demo ccve                # CCVE-2025-0027 (BIGBANK Grafana bug)
./demo scenario clobber    # Platform updates vs app overlays
./demo connected           # ConfigHub connected mode
```

## See Also

- [ConfigHub Concepts](../../docs/GLOSSARY-OF-CONCEPTS.md) - ConfigHub terms (Hub, App Space, Unit, etc.)
- [GSF Schema](../../docs/GSF-SCHEMA.md) - GitOps State Format output schema
- [TUI Trace](../../docs/TUI-TRACE.md) - Trace resource ownership chains
