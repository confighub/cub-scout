# MCP Gateway Example

Run cub-scout as a read-only MCP server over stdio:

```bash
./cub-scout mcp serve
```

This exposes these standalone tools:
- `map`
- `trace`
- `scan`
- `explain`

When connected to ConfigHub (`cub auth login`), it additionally exposes:
- `confighub_changesets`
- `confighub_units`
- `confighub_unit_get`

## Tool Behavior

The gateway reuses existing CLI JSON command outputs:

- `map` -> `map list --json`
- `trace` -> `trace --format json`
- `scan` -> `scan --json`
- `explain` -> `explain --format json`
- `confighub_changesets` -> `cub changeset list --json`
- `confighub_units` -> `cub unit list --json`
- `confighub_unit_get` -> `cub unit get --json`

That keeps MCP responses aligned with the normal CLI contract.

## Safety

- Read-only: no Kubernetes mutation path.
- Read-only: no ConfigHub write path.
- Works in standalone mode without ConfigHub.
