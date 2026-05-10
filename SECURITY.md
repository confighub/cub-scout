# Security Policy

## Read-Only by Default

**cub-scout is designed to be safe to run against production clusters.**

### What "Read-Only" Means

cub-scout uses only these Kubernetes API operations:

| Operation | Used By | Purpose |
|-----------|---------|---------|
| `Get` | All commands | Fetch individual resource details |
| `List` | All commands | Enumerate resources in namespaces |
| `Watch` | `map` TUI | Live updates in interactive mode |

**We never use:**
- `Create` — cub-scout cannot create resources
- `Update` / `Patch` — cub-scout cannot modify resources
- `Delete` — cub-scout cannot remove resources

This holds for every command, including `suggest-remedy` (formerly `remedy`).
See [Suggested Remedies are Read-Only](#suggested-remedies-are-read-only).

### Suggested Remedies are Read-Only

`cub-scout suggest-remedy` describes the patch that *would* resolve a risk
finding — but cub-scout never applies it. The output records the kubectl
command, the current state, and the expected change. A separate tool
(ConfigHub Pilot, an operator, or your CI pipeline) is responsible for any
apply, governed by ConfigHub's normal authority path.

```bash
# Show the suggested fix for a specific finding
cub-scout suggest-remedy CCVE-2025-0687 -n production

# Describe suggestions for all auto-suggestable findings
cub-scout suggest-remedy --all -n production --json
```

The legacy `remedy` verb is still accepted as an alias for backwards
compatibility. It is no longer a mutating command; the previous
`--dry-run`, `--force`, `--audit`, and `--audit-file` flags have been
removed.

This change is the result of [#410](https://github.com/confighub/cub-scout/issues/410)
(triad-compliance audit) and [#428](https://github.com/confighub/cub-scout/issues/428)
(implementation). cub-scout is the read-only **witness**; ConfigHub
(driven by `cub`) is the **authority** that owns mutation.

### RBAC Requirements

cub-scout needs only read permissions. A minimal ClusterRole:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cub-scout-reader
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
```

No write verbs are required — not for `suggest-remedy`, not for any
command. If you previously granted cub-scout `update` / `patch` /
`delete` permissions for the legacy `remedy` execution path, you can
revoke them.

## Vulnerability Reporting

If you discover a security vulnerability, please report it via:

1. **GitHub Security Advisories:** [Create a security advisory](https://github.com/confighub/cub-scout/security/advisories/new)
2. **Email:** security@confighub.com

Please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We'll respond within 48 hours and work with you on a fix before public disclosure.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Previous minor | Yes (security fixes only) |
| Older versions | No |
