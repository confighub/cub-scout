# GitHub Copilot Integration

## MCP Config

Use [`mcp.json`](mcp.json) as the cub-scout MCP server definition for Copilot-enabled tooling.

## Reproducible Demo

```bash
./examples/ai-integration/run-fixture-session.sh
```

Suggested prompt:

```text
Using cub-scout fixture outputs, answer:
1) what is broken,
2) what changed in the last 24h,
3) what is unmanaged,
4) what should be fixed first.
```
