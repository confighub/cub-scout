# Claude Code Integration

## 1. Configure MCP

Use [`mcp.json`](mcp.json) as your Claude Code MCP server definition for `cub scout`.

Key command:

```json
"command": "./cub-scout",
"args": ["mcp", "serve"]
```

## 2. Run Offline Fixture Walkthrough

```bash
./examples/ai-integration/run-fixture-session.sh
```

## 3. Prompt Template

```text
Use cub scout to diagnose deployment/checkout from the latest fixture outputs.
Show trace evidence, then summarize likely operator actions.
```

## Blog-Ready Walkthrough

See [WALKTHROUGH.md](WALKTHROUGH.md).
