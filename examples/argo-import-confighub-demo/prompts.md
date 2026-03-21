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

Guide me through `argo-import-confighub-demo` step by step.

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

## 3. Verify The Import And Scan

After the example is running, verify:

- the cluster is reachable
- ArgoCD applications exist
- the healthy guestbook reference applications are present
- the brownfield `myapp-*` Applications are present
- worker targets exist if connected mode was used
- imported units are listable if connected mode was used
- connected readiness is checked without overclaiming live reconciliation
- `cub-scout` status and ownership views are shown separately from ConfigHub
 - `cub-scout scan` output is shown with a summary and one sample finding

Separate cluster evidence, ConfigHub evidence, and cub-scout evidence in the
final summary.

Do not treat scan/finding evidence as proof that ConfigHub import/render
succeeded.

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether post-import `cub-scout scan` evidence is:

- present and proved
- only explorable manually
- or still missing from the scripted story

For this Argo example, note that `./verify.sh` now includes scan/finding
evidence. Then call out what is still missing elsewhere or still separate from
import/render proof.
