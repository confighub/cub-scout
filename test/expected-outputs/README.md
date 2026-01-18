# Expected Outputs

Expected outputs for every CLI command, demo, example, and test.

## Purpose

These expected outputs serve three functions:

1. **User Documentation** - Help users know what to expect
2. **Test Assertions** - Verify commands produce correct output
3. **Actions Foundation** - Become `assert:` statements in Actions framework

## Command Coverage

### Top-Level Commands (14)

| Command | Has Expected Output | Mode |
|---------|:------------------:|------|
| `map` | Yes | standalone/connected |
| `trace` | Yes | standalone |
| `scan` | Yes | standalone |
| `snapshot` | No | standalone |
| `import` | No | connected |
| `import-argocd` | No | connected |
| `app-space` | No | connected |
| `remedy` | No | standalone |
| `combined` | No | standalone/connected |
| `parse-repo` | No | standalone |
| `demo` | Yes (via demos/) | standalone |
| `version` | Yes | standalone |
| `completion` | No | standalone |
| `setup` | No | standalone |

### Map Subcommands (17)

| Subcommand | Has Expected Output | Mode |
|------------|:------------------:|------|
| `map` (TUI) | No (interactive) | standalone/connected |
| `map --hub` | No (interactive) | connected |
| `map list` | Yes | standalone |
| `map status` | No | standalone |
| `map workloads` | No | standalone |
| `map deployers` | No | standalone |
| `map orphans` | No | standalone |
| `map crashes` | No | standalone |
| `map issues` | No | standalone |
| `map drift` | No | standalone |
| `map bypass` | No | standalone |
| `map sprawl` | No | standalone |
| `map deep-dive` | No (interactive) | standalone |
| `map app-hierarchy` | No (interactive) | standalone |
| `map dashboard` | No | standalone |
| `map queries` | No | standalone |
| `map fleet` | No | connected |
| `map hub` | No | connected |

## Complete Inventory

### CLI Commands (5 files)

| File | Command | Mode |
|------|---------|------|
| `cli/map/standalone.yaml` | `./cub-scout map list` | standalone |
| `cli/map/connected.yaml` | `./cub-scout map --hub` | connected |
| `cli/scan/standalone.yaml` | `./cub-scout scan` | standalone |
| `cli/trace/standalone.yaml` | `./cub-scout trace` | standalone |
| `cli/version/version.yaml` | `./cub-scout version` | standalone |

### ATK Scripts (6 files)

| File | Command | Mode |
|------|---------|------|
| `atk/setup-cluster.yaml` | `./test/atk/setup-cluster` | standalone |
| `atk/verify.yaml` | `./test/atk/verify` | standalone |
| `atk/verify-connected.yaml` | `./test/atk/verify-connected` | connected |
| `atk/map.yaml` | `./test/atk/map` | standalone/connected |
| `atk/scan.yaml` | `./test/atk/scan` | standalone |
| `atk/demo.yaml` | `./test/atk/demo <name>` | standalone |

### Demos (9 files)

| File | Demo | Mode |
|------|------|------|
| `demos/quick.yaml` | Quick 30-second demo | standalone |
| `demos/ccve.yaml` | CCVE-2025-0027 detection | standalone |
| `demos/connected.yaml` | ConfigHub connected mode | connected |
| `demos/query.yaml` | Query language demo | standalone |
| `demos/healthy.yaml` | Enterprise healthy cluster | standalone |
| `demos/unhealthy.yaml` | Enterprise unhealthy cluster | standalone |
| `demos/scenarios/bigbank-incident.yaml` | BIGBANK 4-hour outage story | standalone |
| `demos/scenarios/orphan-hunt.yaml` | Find orphan resources | standalone |
| `demos/scenarios/monday-morning.yaml` | Weekly health check | standalone |

### Examples (6 files)

| File | Example | Mode |
|------|---------|------|
| `examples/impressive-demo/demo.yaml` | Full demo with bad/fixed configs | standalone |
| `examples/integrations/argocd-extension.yaml` | ArgoCD UI extension | standalone |
| `examples/integrations/flux-operator.yaml` | Flux Operator integration | standalone |
| `examples/rm-demos-argocd/monday-panic.yaml` | Monday Panic demo | standalone |
| `examples/rm-demos-argocd/2am-kubectl.yaml` | 2AM kubectl drift demo | standalone |
| `examples/rm-demos-argocd/security-patch.yaml` | Security patch demo | standalone |

### Fixtures (5 files)

| File | Fixture | Owner Type |
|------|---------|------------|
| `fixtures/ownership/flux-basic.yaml` | Flux Kustomization | Flux |
| `fixtures/ownership/argo-basic.yaml` | ArgoCD Application | ArgoCD |
| `fixtures/ownership/helm-basic.yaml` | Helm release | Helm |
| `fixtures/ownership/confighub-basic.yaml` | ConfigHub labels | ConfigHub |
| `fixtures/ownership/native-basic.yaml` | kubectl-applied | Native |

**Total: 31 expected output files**

## Directory Structure

```
expected-outputs/
├── README.md                 # This file
├── cli/                      # CLI commands
│   ├── map/
│   │   ├── standalone.yaml
│   │   └── connected.yaml
│   ├── scan/
│   │   └── standalone.yaml
│   ├── trace/
│   │   └── standalone.yaml
│   └── version/
│       └── version.yaml
├── atk/                      # ATK test scripts
│   ├── setup-cluster.yaml
│   ├── verify.yaml
│   ├── verify-connected.yaml
│   ├── map.yaml
│   ├── scan.yaml
│   └── demo.yaml
├── demos/                    # Demo scripts
│   ├── quick.yaml
│   ├── ccve.yaml
│   ├── connected.yaml
│   ├── query.yaml
│   ├── healthy.yaml
│   ├── unhealthy.yaml
│   └── scenarios/
│       ├── bigbank-incident.yaml
│       ├── orphan-hunt.yaml
│       └── monday-morning.yaml
├── examples/                 # Example repos
│   ├── impressive-demo/
│   │   └── demo.yaml
│   ├── integrations/
│   │   ├── argocd-extension.yaml
│   │   └── flux-operator.yaml
│   └── rm-demos-argocd/
│       ├── monday-panic.yaml
│       ├── 2am-kubectl.yaml
│       └── security-patch.yaml
└── fixtures/                 # Test fixtures
    └── ownership/
        ├── flux-basic.yaml
        ├── argo-basic.yaml
        ├── helm-basic.yaml
        ├── confighub-basic.yaml
        └── native-basic.yaml
```

## YAML Format

Each file defines expected outputs with assertions:

```yaml
name: "command name"
description: "What this tests"
mode: standalone|connected

requires:
  cluster: true|false
  cub_auth: true|false
  tools: [kubectl, jq]
  workers:
    - space: tutorial
      count: 1

commands:
  - id: unique_id
    command: "./path/to/command"
    description: "What this command does"

    expected:
      exit_code: 0

      contains:
        - "literal string"
        - pattern: "(regex|pattern)"
          description: "Why this matters"

      not_contains:
        - "error"
        - "panic"

      assertions:
        - name: "assertion_name"
          condition: "output.contains('expected')"
          description: "What this verifies"

    instructions: |
      Human-readable instructions for running this manually.

# For user documentation
sample_output: |
  Example of what the output looks like.

hints:
  - when: "output.contains('error')"
    show: "Suggestion for fixing this error"
```

## Using Expected Outputs

### For Testing

```bash
# Run all expected output validations
./test/validate-expected-outputs.sh

# Run specific category
./test/validate-expected-outputs.sh --category=cli
./test/validate-expected-outputs.sh --category=demos
./test/validate-expected-outputs.sh --category=examples
./test/validate-expected-outputs.sh --category=atk
./test/validate-expected-outputs.sh --category=fixtures

# Run specific file
./test/validate-expected-outputs.sh --file=cli/map/standalone.yaml

# Include connected mode tests
./test/validate-expected-outputs.sh --connected

# Verbose output
./test/validate-expected-outputs.sh --verbose
```

### For Documentation

Expected outputs are referenced in user docs:

```markdown
See expected output: [cli/map/standalone.yaml](../test/expected-outputs/cli/map/standalone.yaml)
```

### For Actions Framework

Expected outputs become `assert:` statements:

```yaml
steps:
  - name: "Map cluster"
    uses: cub-scout/map@v1
    assert: "map.standalone.assertions.resources_detected"
```

## Validation Script

The validation script (`test/validate-expected-outputs.sh`) does:

1. Checks prerequisites (cluster, tools, auth)
2. Runs each command
3. Checks exit code matches expected
4. Verifies `contains` patterns present
5. Verifies `not_contains` patterns absent
6. Evaluates structured assertions
7. Reports pass/fail for each

## Adding New Expected Outputs

1. Create YAML file in appropriate category
2. Define `command`, `expected`, `instructions`
3. Run validation to ensure it passes
4. Reference in user documentation if applicable

## Modes

| Mode | Requirements |
|------|--------------|
| `standalone` | Cluster only (kubectl access) |
| `connected` | Cluster + cub CLI authenticated + workers running |

## Related Documentation

- [docs/TESTING-GUIDE.md](../../docs/TESTING-GUIDE.md) - Step-by-step testing guide
- [test/atk/DEMO-REQUIREMENTS.yaml](../atk/DEMO-REQUIREMENTS.yaml) - Per-demo requirements
- [CLAUDE.md](../../CLAUDE.md) - Testing strategy in project context
