# Start Here

**Know what is actually running in Kubernetes — who owns it, what will revert your change, and whether your deploy really landed.**

Start with a question: is it safe to write this change, what is running, where
did it come from, did my revision reach running Pods, or did it land across my
fleet? Scout gathers strictly read-only, deterministic evidence without
becoming another deployment controller.

| Goal | Start here |
|---|---|
| **Coding agent / `cub mini` write loop** | [Agent Write Loop](../howto/agent-write-loop.md) · [`cub mini` Integration Contract](../howto/cub-mini-integration.md) |
| **CI/CD promotion gate (GitHub Actions)** | [Reusable `verify-landed` CI Gate](../../examples/ci-gates/verify-landed/) |
| **Multi-cluster fleet proof (`dev → staging → prod`)** | [Fleet Promotion Proof](../howto/fleet-promotion-proof.md) · [Runnable 4-Cluster Example](../../examples/fleet-promotion-proof/) |
| **Ops bots (`k8sbot`, Pilot, Slack/incident)** | [Ops-Bot Consumer Contract](../../examples/bot/consumer-contract.md) · [Hardened Bot Deploy Kit](../../examples/bot/) |
| **Check whether an image is deployed** | [Is This Image Deployed?](../howto/is-this-image-deployed.md) |
| **Token cost & 3.x roadmap** | [Answer Sizes (99.78% smaller)](../reference/answer-sizes.md) · [3.x Roadmap](../proposals/vision-3.x.md) |

Choose the **`cub` plugin** (`cub scout`, primary), the **standalone CLI/TUI** (`cub-scout`), an **MCP server** launched by
your agent host, a **watch stream**, or an **in-cluster bot**. The
[five-mode table](../../README.md#five-ways-to-run-cub-scout) explains which to use.
Live checks need Kubernetes access; saved files and bundles can be inspected
offline. A ConfigHub account is optional.

If you just want the command index, use:
- [Complete CLI Reference (A-Z)](../reference/cli-reference.md)
- [Command Reference](../reference/commands.md)

Checking an application image? Start with
[Is This Image Deployed?](../howto/is-this-image-deployed.md) for the shipped
v2.12.0 limits, the **v2.12.1** Deployment proof, the difference between
configuration and image digests, and what pod-reported identity can and cannot
prove.

---

## First-Time User

Run:

```bash
# Plugin form (primary)
cub plugin install confighub/cub-scout
cub scout doctor
cub scout gitops status
cub scout map

# Or standalone binary
brew install confighub/tap/cub-scout
cub-scout doctor
```

`doctor` gives the first triage summary, `gitops status` reports controller
pipeline evidence, and `map` opens the explorer. For one object, use
`cub-scout explain deploy/<name> -n <namespace>` and follow with `trace` for its
source chain. See [installation](install.md) for other platforms or the plugin.

For one exact object with an explicit read budget (v2.10.0):

```sh
cub-scout explain Deployment/api -n team-a --bounded \
  --api-version apps/v1 --kube-context my-test-context --format json
```

Replace the scope with your resource. This checks object-local facts using one
discovery document and one GET; it does not establish desired/live agreement or
delivery success. [Bounded evidence example](../../examples/bounded-resource-read/).

Then:
- [First Map](first-map.md)
- [Ownership Detection](../howto/ownership-detection.md)
- [Trace Ownership](../howto/trace-ownership.md)
- [New User Puzzle Quest](../../examples/new-user-puzzle-quest/)

---

## Connected Questions

Run:

```bash
cub auth login
cub scout compare three-way --scope namespace/<namespace>
cub scout history deploy/<name> -n <namespace> --space <space>
cub scout impact <unit> --space <space>
cub scout audit list --since 7d --space <space>
```

Each of these reads one named ConfigHub space: pass `--space`, or export
`CUB_SPACE`. `cub` has no default space, so with neither they refuse. The
standalone `cub-scout` binary accepts the exact same commands.

`fleet outliers` was removed in 3.0 with no replacement because its
single-space comparison could not establish reliable cross-cluster lineage.
See the [3.0 migration note](../migration-3.0.md#fleet-outliers).

Then:
- [Canonical Import Path](../howto/import-to-confighub.md)
- [Migration Playbook](../howto/migration-playbook.md)
- [Connect and Compare Demo](../../examples/connect-and-compare/)

---

## AI Tools

An agent with shell access can run `cub scout ... --format json` (or standalone
`cub-scout ... --format json`) directly. MCP is the structured-tool
alternative: configure the host to launch `cub-scout mcp serve` over stdio.
Scout does not have to be running beforehand. Each server has its own process
lifetime and credentials; it is not a fleet service. Bounded MCP calls expose
cache/freshness and explicit refresh.

Full read-only agent write loop (pre-write safety, post-write Pod image-digest
witness, air-gapped `--recording`, and offline Ed25519 signed proof):

```bash
# Before writing: ownership, revert risk, sync windows, contention, PDBs (<1 KB)
cub scout before-write deploy/api -n prod --field-path .spec.replicas --format json

# After writing: verify anchored revision + running Pod imageIDs
cub scout landed deploy/api -n prod --revision <sha> --verify-pod-images --wait 60s --format json

# Offline / air-gapped: capture once, evaluate anywhere
cub scout snapshot --format yaml -o snap.yaml
cub scout landed deploy/api -n prod --revision <sha> --verify-pod-images --recording snap.yaml --format json

# Tamper-evident offline proof (single cluster and multi-cluster fleet)
cub scout receipt sign --input landed.json --key priv.pem -o signed.json
cub scout receipt verify --input signed.json --pubkey pub.pem --format json
cub scout receipt verify-fleet --inputs ./receipts --pubkey pub.pem --revision <sha> --require-clusters dev,staging,prod --format json
```

MCP server, context-pack, and ask-mode contract:

```bash
cub-scout mcp serve
cub-scout context-pack --format json --max-bytes 16384
./scripts/ask-mode-contract.sh --mode auto --command "./cub-scout import -n payments --dry-run"
```

Then:
- [Agent Write Loop](../howto/agent-write-loop.md) (`before-write`, `landed --verify-pod-images`, `--recording`, Ed25519 receipts, `--view summary` / `--if-unchanged`)
- [`cub mini` Integration Contract](../howto/cub-mini-integration.md) (pre-write/post-write hooks, boundary table, `receipt verify-fleet`)
- [CI Promotion Gate (`verify-landed`)](../../examples/ci-gates/verify-landed/) (drop-in read-only GitHub Actions gate)
- [Answer Sizes (99.78% smaller)](../reference/answer-sizes.md)
- [Using cub-scout from an AI Tool](../howto/using-cub-scout-from-ai-tool.md)
- [Context-Pack v2](../howto/context-pack-v2.md)
- [AI Ask-Mode Contract](../howto/ai-ask-mode-contract.md)
- [MCP Gateway Example](../../examples/mcp-gateway/)
- [AI Integration Examples](../../examples/ai-integration/)

---

## Adopt Existing Config

Use this after you have observed the live cluster and want to preview how
existing workloads would map into ConfigHub.

```bash
cub-scout import --dry-run -n <namespace>
cub-scout import --json
```

Advanced repo preview, only when the repository itself is the object under review:

```bash
cub-scout import --git-path ./repo --dry-run --json
cub-scout import parse-repo --path ./repo --json
```

Then:
- [Import from Live Cluster](../howto/import-from-live.md)
- [Canonical Import Path](../howto/import-to-confighub.md)
- [Migration Playbook](../howto/migration-playbook.md)

---

## Ongoing Observation

Use `watch` for a local poller; run `bot` in a Pod with read-only RBAC and an
approved webhook or JSONL destination. Both use the same polling engine. A
consumer can reuse their output, but they do not serve a shared query cache or
trigger deployment. [Bot setup and limits](../../examples/bot/).

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
