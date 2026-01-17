# User Documentation

User documentation for ConfigHub Agent. For planning docs, see [planning/](planning/).

**Main entry point:** [map/README.md](map/README.md) — Start here for cub-scout map.

---

## Quick Navigation

| What You Want | Where To Go |
|---------------|-------------|
| Get started quickly | [map/QUICKSTART.md](map/QUICKSTART.md) |
| Full map documentation | [map/README.md](map/README.md) |
| CLI command reference | [map/reference/commands.md](map/reference/commands.md) |
| Query syntax | [map/reference/query-syntax.md](map/reference/query-syntax.md) |
| Keyboard shortcuts | [map/reference/keybindings.md](map/reference/keybindings.md) |

---

## How-To Guides

| Guide | What You'll Learn |
|-------|-------------------|
| [ownership-detection.md](map/howto/ownership-detection.md) | Understand who owns what |
| [find-orphans.md](map/howto/find-orphans.md) | Find unmanaged (Native) resources |
| [trace-ownership.md](map/howto/trace-ownership.md) | Trace from resource to Git source |
| [scan-for-ccves.md](map/howto/scan-for-ccves.md) | Detect configuration issues |
| [query-resources.md](map/howto/query-resources.md) | Filter and query resources |
| [import-to-confighub.md](map/howto/import-to-confighub.md) | Import workloads into ConfigHub |

---

## Reference Documentation

| Doc | What It Covers |
|-----|----------------|
| [map/reference/commands.md](map/reference/commands.md) | All 12 CLI commands |
| [map/reference/views.md](map/reference/views.md) | 9+ TUI views |
| [map/reference/query-syntax.md](map/reference/query-syntax.md) | Query language |
| [map/reference/keybindings.md](map/reference/keybindings.md) | Keyboard shortcuts |
| [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) | ConfigHub terms (Hub, App Space, Unit) |
| [GSF-SCHEMA.md](GSF-SCHEMA.md) | GitOps State Format — JSON output schema |
| [ARCHITECTURE.md](ARCHITECTURE.md) | GSF protocol, API contracts |

---

## Enhanced Query Features (New)

| Doc | What It Covers |
|-----|----------------|
| [ENHANCED-QUERIES.md](ENHANCED-QUERIES.md) | Reverse trace, relationship queries, drift detection, dangling refs |

---

## CCVE & Extending

| Doc | What It Covers |
|-----|----------------|
| [CCVE-GUIDE.md](CCVE-GUIDE.md) | Intro to Config vulnerability scanning |
| [EXTENDING.md](EXTENDING.md) | Custom CCVEs, ownership detectors |
| [EXAMPLES-OVERVIEW.md](EXAMPLES-OVERVIEW.md) | Usage patterns, integrations, scripts |
| [TESTING-GUIDE.md](TESTING-GUIDE.md) | How to test the agent |

---

## Business Outcomes

| Doc | What It Covers |
|-----|----------------|
| [outcomes/README.md](outcomes/README.md) | Business value and outcomes |
| [outcomes/ownership-visibility.md](outcomes/ownership-visibility.md) | See what's running, who owns it |
| [outcomes/break-glass-scenarios.md](outcomes/break-glass-scenarios.md) | Emergency scenarios |
| [outcomes/confighub-integration.md](outcomes/confighub-integration.md) | ConfigHub platform integration |

---

## Archived Docs

Old documentation is preserved in [archive/](archive/). See [archive/README.md](archive/README.md) for mapping to new locations.

## Core Concepts

### GitOps State Format (GSF)

The agent outputs **GSF** — a JSON format for cluster state:

```json
{
  "entries": [{
    "id": "prod/default/Deployment/nginx",
    "owner": { "type": "flux", "ref": "kustomization/apps" },
    "drift": { "path": "spec.replicas", "from": 2, "to": 3 }
  }],
  "relations": [{
    "from": "prod/default/Deployment/nginx",
    "to": "prod/default/Service/nginx",
    "type": "selects"
  }]
}
```

Pipe GSF to jq, Grafana, Argo extensions, k9s plugins, or ConfigHub.

### Ownership Detection

The agent auto-detects who manages each resource:

| Owner | Detection Method |
|-------|------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **Argo CD** | `argocd.argoproj.io/instance` label or tracking annotation |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **Terraform** | `app.terraform.io/*` annotations |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | OwnerReferences to other K8s resources |

### Drift Detection

Compares live state to `kubectl.kubernetes.io/last-applied-configuration` annotation.

### CCVEs (Config Common Vulnerabilities and Errors)

Like CVEs, but for config anti-patterns. 46 active scanner patterns, 4,500+ reference database:

```bash
# Full scan (stuck resources + Kyverno violations)
./cub-scout scan
```

Example findings:
- `CCVE-FLUX-001`: Source not found
- `CCVE-ARGO-003`: Drift detected (OutOfSync)
- `CCVE-HELM-003`: Pending rollback
- `CCVE-DRIFT-003`: Image tag drift

## Usage Patterns

### One-Time Snapshot

```bash
./cub-scout snapshot -o state.json
```

### Dashboard View

```bash
cub-scout map               # Full dashboard TUI
cub-scout map list          # Plain text output
cub-scout map list -q "..."  # Filtered output
```

### CCVE Scanning

```bash
./cub-scout scan                # Full scan (state + Kyverno)
./cub-scout scan --state        # Stuck resources only
./cub-scout scan --kyverno      # Kyverno violations only
./cub-scout scan --json         # JSON for tooling
```

### Pipe to Other Tools

```bash
./cub-scout snapshot -o - | jq '.entries[] | select(.drift)'
./cub-scout snapshot -o - | your-custom-script
```

## Integration Examples

### k9s Plugin

```yaml
# ~/.k9s/plugin.yml
plugin:
  confighub-map:
    shortCut: Shift-M
    command: cub-scout
    args: ["map"]
  confighub-scan:
    shortCut: Shift-V
    command: cub-scout
    args: ["scan"]
```

### Grafana Dashboard

The agent can expose Prometheus metrics:

```bash
# gsf_entries_total{cluster="prod",kind="Deployment",owner="flux"} 45
# gsf_entries_drifted{cluster="prod"} 3
# gsf_entries_unowned{cluster="prod"} 12
```

### Custom Alerting

```bash
#!/bin/bash
./cub-scout snapshot -o - | jq -e '.entries[] | select(.drift)' && \
  slack-notify "#alerts" "Drift detected in cluster"
```

## Standalone vs Connected

| Mode | What Works | What Doesn't |
|------|------------|--------------|
| **Standalone** | Ownership, drift, CCVEs, local CLI | Fleet queries, ConfigHub UI |
| **Connected** | Everything above + fleet aggregation | Requires ConfigHub API token |

See the main [README](../README.md) for installation and configuration.
