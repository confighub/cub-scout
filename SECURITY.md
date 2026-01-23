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

### The One Exception: `remedy`

The `cub-scout remedy` subcommand is the **only** exception. It can apply fixes for detected configuration issues.

**Safeguards:**
1. `remedy` always shows the exact changes before applying
2. `remedy` requires explicit `--apply` flag (dry-run by default)
3. `remedy` prompts for confirmation before each change
4. `remedy` logs all actions to a file

```bash
# Dry-run (default) - shows what would change
cub-scout remedy CCVE-2025-0027

# Apply changes (requires explicit flag + confirmation)
cub-scout remedy CCVE-2025-0027 --apply
```

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

For the `remedy` command (optional), you'd need:

```yaml
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list", "watch", "update", "patch"]
  # Add other resources as needed for specific remedies
```

### Audit Trail

- All `remedy` actions are logged to `~/.cub-scout/remedy.log`
- Dry-run output can be captured: `cub-scout remedy CCVE-X --dry-run > plan.yaml`
- The `remedy` command prints a summary of changes after completion

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
