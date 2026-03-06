# AI Tool Integration Examples

Reproducible AI integration examples for `cub-scout` with no live cluster required.

This package documents CLI and MCP usage for:
- Claude Code
- Cursor
- GitHub Copilot

## What You Get

- Per-tool MCP config examples (`claude-code/mcp.json`, `cursor/mcp.json`, `copilot/mcp.json`)
- Fixture-backed scenarios that run offline
- A blog-ready Claude Code walkthrough

## Offline Scenario Set

The fixture runner demonstrates these scenarios:

1. Debug why a deployment is failing (`trace`)
2. What changed in prod since yesterday (`history`)
3. Is this manifest safe to deploy (`scan --file`)
4. Show unmanaged resources (`map orphans`)

## Run the Fixture Session

From repo root:

```bash
./examples/ai-integration/run-fixture-session.sh
```

This writes sample outputs to:

```text
examples/ai-integration/sample-output/
```

Use a custom output directory:

```bash
./examples/ai-integration/run-fixture-session.sh --output-dir /tmp/cub-scout-ai-demo
```

## Tool Guides

- [Claude Code](claude-code/README.md)
- [Cursor](cursor/README.md)
- [GitHub Copilot](copilot/README.md)

## Notes

- Fixture mode is explicit and opt-in via `CUB_SCOUT_TEST_*` env vars.
- Normal command behavior is unchanged when test env vars are not set.
- `history` stays connected-mode by default; fixture mode is for demos/tests only.
