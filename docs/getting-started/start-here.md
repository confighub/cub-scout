# Start Here

Fast routing guide for new and returning `cub-scout` users.

If you just want the command index, use:
- [Complete CLI Reference (A-Z)](../reference/cli-reference.md)
- [Command Reference](../reference/commands.md)

---

## 1) First-Time User (Standalone, no account required)

Run:

```bash
brew install confighub/tap/cub-scout
cub-scout quickstart --yes
cub-scout doctor
cub-scout explain deploy/<name> -n <namespace>
cub-scout map
```

Then:
- [First Map](first-map.md)
- [Ownership Detection](../howto/ownership-detection.md)
- [Trace Ownership](../howto/trace-ownership.md)
- [New User Puzzle Quest](../../examples/new-user-puzzle-quest/)

---

## 2) Connected Value (ConfigHub context)

Run:

```bash
cub auth login
cub-scout compare three-way --scope namespace/<namespace>
cub-scout history deploy/<name> -n <namespace>
cub-scout impact <unit>
cub-scout fleet outliers
cub-scout audit list --since 7d
```

Then:
- [Canonical Import Path](../howto/import-to-confighub.md)
- [Migration Playbook](../howto/migration-playbook.md)
- [Connect and Compare Demo](../../examples/connect-and-compare/)

---

## 3) AI Tooling Path (Read-only gateway)

Run:

```bash
cub-scout mcp serve
cub-scout context-pack --format json --max-bytes 16384
./scripts/ask-mode-contract.sh --mode auto --command "./cub-scout import -n payments --dry-run"
```

Then:
- [Using cub-scout from an AI Tool](../howto/using-cub-scout-from-ai-tool.md)
- [Context-Pack v2](../howto/context-pack-v2.md)
- [AI Ask-Mode Contract](../howto/ai-ask-mode-contract.md)
- [MCP Gateway Example](../../examples/mcp-gateway/)
- [AI Integration Examples](../../examples/ai-integration/)

---

## 4) Platform-Scale Features (v1.7 line)

Run:

```bash
cub-scout tree composition
cub-scout map meaning
cub-scout summary list --since 24h
cub-scout summary slack --dry-run --since 24h
cub-scout watch --output-file /tmp/cub-scout-events.jsonl --once
```

Then:
- [kro Composition Example](../../examples/kro-composition/)
- [Connected Summary Storage Example](../../examples/connected-summary-storage/)
- [Watch Webhook Example](../../examples/watch-webhook/)
- [Extending cub-scout](../howto/extending.md)

