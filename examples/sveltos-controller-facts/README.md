# Separate Sveltos controller reports

The authored input in [observations.yaml](../../evals/sveltos-controller-facts/fixtures/cluster/observations.yaml) contains delivery `Provisioned` and continuous-health condition `False`. Field shapes are pinned to sveltos-confighub reporter source `8187910f9fe226e109e55c4d9c7c0e21297ff424`; these objects are not a live capture. Deterministic collector/projection tests bind the objects to [status.json](status.json).

Replay the summary without a cluster:

```bash
CUB_SCOUT_OFFLINE=true CUB_SCOUT_TEST_GITOPS_JSON="$PWD/examples/sveltos-controller-facts/status.json" ./cub-scout gitops status --format json
CUB_SCOUT_OFFLINE=true CUB_SCOUT_TEST_GITOPS_JSON="$PWD/examples/sveltos-controller-facts/status.json" ./cub-scout gitops status --format ascii
CUB_SCOUT_OFFLINE=true CUB_SCOUT_TEST_GITOPS_JSON="$PWD/examples/sveltos-controller-facts/status.json" ./cub-scout gitops status --format md
CUB_SCOUT_OFFLINE=true CUB_SCOUT_TEST_GITOPS_JSON="$PWD/examples/sveltos-controller-facts/status.json" ./cub-scout gitops status --tui
```

For MCP, use the same environment with `./cub-scout mcp serve` and call `gitops_status` with `{}`. The shared report is under `structuredContent.data.sveltosControllerReports`.

Source identities and reported target references remain separate. No name/prefix or timestamp joins occur. Neither report establishes present workload health, check execution freshness, release correlation or gate acceptance. `lastAppliedTime` and `lastTransitionTime` remain reported timestamps. Missing fields and bounded omissions are explicit; denied lists remain controller coverage omissions.

[Host rendering proof](render-proof.json) records real local CLI/MCP/PTY executions against this authored summary with private empty HOME/kubeconfig and offline mode. It proves rendering and propagation, not cluster collection, controller reconciliation or live acceptance. The source base and modified source state are disclosed; raw streams remain at `/private/tmp/scout-v213-sveltos-render-final-20261004`.
