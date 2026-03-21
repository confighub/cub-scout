# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what this example is for
- what it reads
- what it writes
- which steps mutate ConfigHub
- which steps mutate live infrastructure
- what success looks like

Then run only:

```bash
./setup.sh --explain
./setup.sh --explain-json
```

## 2. Safe Walkthrough

Guide me through `flux-import-confighub-demo` step by step.

Before each command:

- explain what it does
- say whether it mutates ConfigHub
- say whether it mutates live infrastructure
- tell me what success looks like
- tell me what evidence surface it affects next

Use this path:

```bash
./setup.sh
./verify.sh
```

If I want ConfigHub import too, switch to:

```bash
./setup.sh --with-worker
./verify.sh
```

Keep `demo.sh` as the narrated human path, but use `setup.sh`/`verify.sh` as the
AI-first path.

## 3. Verify The Import

After the example is running, verify:

- the cluster is reachable
- Flux sources and deployers exist
- the healthy `podinfo` reference path is present
- the D2 brownfield paths are present even when they are unhealthy
- worker targets exist if connected mode was used
- imported units are listable if connected mode was used
- connected readiness is checked without overclaiming live reconciliation
- `cub-scout` status and ownership views are shown separately from ConfigHub

Separate cluster evidence, ConfigHub evidence, and cub-scout evidence in the
final summary.

## 4. Call Out The Current Gap

Evaluate this example honestly.

Say whether post-import `cub-scout scan` evidence is:

- present and proved
- only explorable manually
- or still missing from the scripted story

Do not imply that scan/finding evidence is part of `./verify.sh` if it is not.
