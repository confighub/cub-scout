# Context-Pack v2 for AI Handoffs

Use `context-pack` when you need one deterministic JSON payload for AI-assisted investigation.

## Command

```bash
./cub-scout context-pack --format json
```

## Common usage

All namespaces, bounded payload:

```bash
./cub-scout context-pack --format json --max-bytes 16384 > /tmp/context-pack.json
```

Namespace-scoped, tighter risk/trace focus:

```bash
./cub-scout context-pack -n payments --top-risks 3 --trace-seeds 3 --max-bytes 8192 --format json
```

## Output model (v2)

The pack includes:
- ownership summary
- top risks
- trace seed commands
- command evidence references
- provenance + confidence markers
- truncation/size metadata for bounded payloads

## Deterministic fixture mode

For tests and reproducible demos:

```bash
CUB_SCOUT_TEST_CONTEXT_PACK_INPUT_JSON=test/ascii/context-pack/testdata/basic_input.json \
CUB_SCOUT_TEST_TIME=2026-03-06T20:30:00Z \
./cub-scout context-pack --format json
```
