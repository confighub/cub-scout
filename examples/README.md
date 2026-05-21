# cub-scout Examples

This directory is the examples catalog for `cub-scout`.

Use these examples when you want one of three things:

- read-only cluster troubleshooting
- GitOps ownership and lineage visibility
- connected import or comparison with ConfigHub

Do not start by reading the whole directory. Pick one path that matches the
question you have right now.

## Start With One Path

| Goal | Start here | Why this is the right first path |
|---|---|---|
| Learn the core workflow from zero | [`new-user-puzzle-quest`](./new-user-puzzle-quest/) | guided first run through `quickstart`, `doctor`, `map`, `trace`, and import preview |
| Show an AI assistant what `cub-scout` adds | [`ai-agent-quest`](./ai-agent-quest/) | clean story for giving an AI read-only cluster eyes before connected ConfigHub steps |
| Show connected value in under a minute | [`connect-and-compare`](./connect-and-compare/) | deterministic fixture path for doctor, connect, compare, and history |
| Import from a real cluster into ConfigHub | [`import-from-live`](./import-from-live/) | the simplest brownfield cluster-first story |
| Compare a Git repo with live cluster state | [`combined-git-live`](./combined-git-live/) | good when the repo and cluster already both exist |
| Prove Argo or Flux import on a real kind cluster | [`argo-import-confighub-demo`](./argo-import-confighub-demo/) or [`flux-import-confighub-demo`](./flux-import-confighub-demo/) | full demo paths with real controllers and ConfigHub import |

If you are unsure, start with `new-user-puzzle-quest` for learning or
`connect-and-compare` for a fast deterministic demo.

## Quick Commands

From the repo root:

```bash
go build ./cmd/cub-scout

./cub-scout quickstart
./cub-scout doctor
./cub-scout map
./cub-scout import --dry-run -n default
```

`cub-scout` is standalone by default. Log in with `cub auth login` only when
you want connected ConfigHub features such as import, compare, history, or
summary storage.

## Run Order That Repeats Well

For the smoothest first run:

1. Start with a fixture-backed or quest example.
2. Prove the read-only CLI flow locally.
3. Only then move to connected ConfigHub examples.
4. Only then move to real-cluster controller demos.

That order avoids burning time on cluster or auth setup before the operator
understands the core ownership model.

## Example Families

### First-run and learning paths

- [`new-user-puzzle-quest`](./new-user-puzzle-quest/)
- [`ai-agent-quest`](./ai-agent-quest/)
- [`connect-and-compare`](./connect-and-compare/)

### Connected import and brownfield discovery

- [`import-from-live`](./import-from-live/)
- [`combined-git-live`](./combined-git-live/)
- [`argo-import-confighub-demo`](./argo-import-confighub-demo/)
- [`flux-import-confighub-demo`](./flux-import-confighub-demo/)
- [`fleet-import`](./fleet-import/)
- [`import-from-bundle`](./import-from-bundle/)

### GitOps pattern fixtures and ownership demos

- [`apptique-examples`](./apptique-examples/)
- [`platform-example`](./platform-example/)
- [`d2-control-plane`](./d2-control-plane/)
- [`flux-boutique`](./flux-boutique/)
- [`kro-composition`](./kro-composition/)
- [`custom-ownership-detectors`](./custom-ownership-detectors/)
- [`orphans`](./orphans/)
- [`drift`](./drift/)
- [`receipts`](./receipts/) — typed, fingerprinted evidence artifacts (#446)
- [`lifecycle-hazards`](./lifecycle-hazards/)
- [`demo-data`](./demo-data/)
- [`demo-data-adt`](./demo-data-adt/)

### AI, integrations, and automation surfaces

- [`ai-integration`](./ai-integration/)
- [`mcp-gateway`](./mcp-gateway/)
- [`watch-webhook`](./watch-webhook/)
- [`connected-summary-storage`](./connected-summary-storage/)
- [`graph-export`](./graph-export/)
- [`workflows`](./workflows/)

### Demo fixtures and concept material

- [`demos`](./demos/) — fast ownership and risk demos
- [`impressive-demo`](./impressive-demo/) — presentation-oriented incident demo
- [`rm-demos-argocd`](./rm-demos-argocd/) — simulation/storytelling, not live
- [`app-config-rtmsg`](./app-config-rtmsg/) — concept mockup, not a runnable
  cluster example

## What To Use For Which Question

- "What is broken right now on this cluster?" Start with `doctor`, then use
  `new-user-puzzle-quest` or `ai-agent-quest`.
- "Who owns this resource and where did it come from?" Start with
  `platform-example`, `flux-boutique`, or `apptique-examples`.
- "Can we import what we already run into ConfigHub?" Start with
  `import-from-live`, then move to the Argo or Flux demo that matches your
  controller.
- "Can an AI tool use cub-scout safely?" Start with `ai-agent-quest`,
  `ai-integration`, or `mcp-gateway`.
- "Can I show a deterministic demo without a live cluster?" Start with
  `connect-and-compare` or `demos`.

## Pointers Outside This Directory

- [Getting started](../docs/getting-started/start-here.md)
- [Command reference](../docs/reference/commands.md)
- [CLI reference](../docs/reference/cli-reference.md)

Those docs explain the CLI. This directory is for choosing the right concrete
example and running it without getting lost.
