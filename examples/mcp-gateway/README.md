# MCP Gateway Example

Run cub-scout as a read-only MCP server over stdio:

```bash
./cub-scout mcp serve
```

This exposes these tools in the current slice:
- `map`
- `trace`
- `scan`
- `explain`

## Tool Behavior

The gateway reuses existing CLI JSON command outputs:

- `map` -> `map list --json`
- `trace` -> `trace --format json`
- `scan` -> `scan --json`
- `explain` -> `explain --format json`

That keeps MCP responses aligned with the normal CLI contract.

## Safety

- Read-only: no Kubernetes mutation path.
- Read-only: no ConfigHub write path.
- Works in standalone mode without ConfigHub.
