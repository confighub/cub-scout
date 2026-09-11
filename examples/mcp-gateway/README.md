# MCP Gateway Example

Run cub-scout as a read-only MCP server over stdio:

```bash
./cub-scout mcp serve
```

This exposes these standalone tools:
- `doctor`
- `map`
- `trace`
- `scan`
- `explain`
- `gitops_status`

When connected to ConfigHub (`cub auth login`), it additionally exposes:
- `compare_three_way`
- `compare_source_truth`
- `confighub_changesets`
- `confighub_k8s_resources`
- `confighub_k8s_types`
- `confighub_live_status`
- `confighub_releases`
- `confighub_unit_events`
- `confighub_units`
- `confighub_unit_get`

## Tool Behavior

The gateway reuses existing CLI JSON command outputs:

- `doctor` -> `doctor --format json`
- `map` -> `map list --json`
- `trace` -> `trace --format json`
- `scan` -> `scan --json`
- `explain` -> `explain --format json`
- `gitops_status` -> `gitops status --format json`
- `compare_three_way` -> `compare three-way --format json`
- `compare_source_truth` -> `compare source-truth --format json`
- `confighub_changesets` -> `cub changeset list --json`
- `confighub_k8s_resources` -> `cub k8s get <type> [<name> ...] -o json`
- `confighub_k8s_types` -> `cub k8s types [<type>] -o json`
- `confighub_live_status` -> `cub space list -o json --select Slug,SpaceID,Annotations,Labels`
- `confighub_releases` -> `cub release list --space <space> -o json`
- `confighub_unit_events` -> `cub unit-event list [unit] --space <space> -o json`
- `confighub_units` -> `cub unit list --json`
- `confighub_unit_get` -> `cub unit get --json`

That keeps MCP responses aligned with the normal CLI contract.

## Low-load intended-config reads

For broad ConfigHub-side questions, ask the type survey first, then fetch only
the intended resources you need:

```json
{"name":"confighub_k8s_types","arguments":{"space":"prod"}}
```

```json
{"name":"confighub_k8s_resources","arguments":{"type":"deploy","space":"prod","namespace":"payments","show":"detail"}}
```

Both tools read ConfigHub's stored Resource data. They do not read live cluster
state, force sync a controller, or mutate ConfigHub.

## Safety

- Read-only: no Kubernetes mutation path.
- Read-only: no ConfigHub write path.
- Works in standalone mode without ConfigHub.
