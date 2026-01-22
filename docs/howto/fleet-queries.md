# Fleet Queries with ConfigHub

cub-scout is a single-cluster tool. For multi-cluster fleet queries, use ConfigHub directly.

## Prerequisites

```bash
# Install and authenticate ConfigHub CLI
brew install confighub/tap/cub
cub auth login
```

## Cross-Space Queries

Query across ALL spaces (clusters/environments):

```bash
# List all units across all spaces (612 units)
cub unit list --space "*"

# List all targets across all spaces (238 targets)
cub target list --space "*"

# Filter by labels
cub unit list --space "*" --where "Labels.Environment = 'prod'"
```

## Find Pending Upgrades

Identify units that need version upgrades:

```bash
# Find units with upstream changes (370 units)
cub unit list --space "*" --where "UpstreamRevisionNum > 0" \
  --columns Unit.Slug,Space.Slug,Unit.HeadRevisionNum,Unit.UpstreamRevisionNum
```

Example output:
```
NAME                  SPACE                      HEAD    UPSTREAM
postgres              playful-cub-us-staging     3       2
frontend              happy-claws-asia-staging   5       3
backend               happy-claws-asia-staging   7       4
```

## Configuration Inheritance Trees

See how configurations flow across environments:

```bash
# Clone tree (configuration inheritance)
cub unit tree --space "*" --edge clone \
  --columns "Unit.HeadRevisionNum,Unit.UpstreamRevisionNum"
```

Example output:
```
NODE                         SPACE                    HEAD  UPSTREAM
└── namespace                sweet-growl-traderx-base    1    0
    └── namespace            sweet-growl-traderx-dev     2    1
        └── namespace        sweet-growl-traderx-staging 1    2
            └── namespace    sweet-growl-traderx-prod    1    1
```

## Dependency Trees

See producer/consumer relationships:

```bash
# Link tree (dependencies)
cub unit tree --space appchat-prod --edge link

NODE            SPACE         STATUS    UPGRADE-NEEDED
└── frontend    appchat-prod  NotLive   Yes
    ├── appchat-ns
    └── backend
        ├── database
        └── appchat-ns
```

## Push Upgrades Across Fleet

Propagate changes from a base configuration to all downstream environments:

```bash
# Push changes from base template to all downstreams
cub unit push-upgrade --space my-space base-template

# Verbose output
cub unit push-upgrade --space my-space base-template --verbose
```

## Diff Between Revisions

Compare versions of a unit:

```bash
# Default: Live vs Head revision
cub unit diff my-unit --space my-space

# Specific revisions
cub unit diff my-unit --from=123 --to=456

# Named revisions (relative)
cub unit diff my-unit --from=-1  # Previous vs current
```

## Workflow: Import from cub-scout, Manage in ConfigHub

1. **Discover** workloads with cub-scout (single cluster):
   ```bash
   cub-scout map workloads
   ```

2. **Import** to ConfigHub:
   ```bash
   cub-scout import -n my-namespace
   ```

3. **Link** across spaces:
   ```bash
   cub link create --space prod --from-unit base --to-unit derived --to-space staging
   ```

4. **Query** fleet-wide:
   ```bash
   cub unit list --space "*" --where "UpstreamRevisionNum > 0"
   ```

5. **Push** upgrades:
   ```bash
   cub unit push-upgrade --space base-space base-unit
   ```

## Real Numbers

From actual ConfigHub organization:
- **612 units** across all spaces
- **238 targets** across all spaces
- **370 units** with upstream revisions (pending upgrades)
- **5 links** in appchat-prod showing dependencies

These are real queries against live data, not mocked examples.
