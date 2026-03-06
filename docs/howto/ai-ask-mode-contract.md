# AI Ask-Mode Contract

Use this contract when an AI proposes running commands.

Contract stages:
1. Verify proposed command and context mode.
2. Prefer dry-run for higher-risk commands.
3. Require explicit confirm for non-dry-run high-risk actions.

## Command

```bash
./scripts/ask-mode-contract.sh --command "<proposed command>" [--mode standalone|connected|auto] [--confirm yes|no] [--execute]
```

## Examples

High-risk connected command (blocked until confirm):

```bash
./scripts/ask-mode-contract.sh --mode connected --command "./cub-scout import -n payments"
```

Low-risk standalone command (ready):

```bash
./scripts/ask-mode-contract.sh --mode standalone --command "./cub-scout map list"
```

Execute only after explicit confirm:

```bash
./scripts/ask-mode-contract.sh --mode connected --command "./cub-scout import -n payments" --confirm yes --execute
```

## Fixture Outputs

- Failure path: `test/fixtures/ai/ask-mode/failure.txt`
- Success path: `test/fixtures/ai/ask-mode/success.txt`

These fixtures mirror the fixed output contract used by unit tests.
