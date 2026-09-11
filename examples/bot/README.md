# cub-scout Bot Example

Run cub-scout continuously inside Kubernetes as a read-only observation bot.

The bot is the deployable form of `cub-scout watch`: it uses the same polling
engine, event types, queueing, and optional receipt emission, but reads
configuration from `CUB_SCOUT_BOT_*` environment variables and uses in-cluster
Kubernetes authentication.

## What This Covers

- `cub-scout bot` as an in-cluster observer
- read-only ServiceAccount, ClusterRole, and ClusterRoleBinding
- webhook delivery using `CUB_SCOUT_BOT_WEBHOOK_URL`
- warning/critical filtering and bounded receipt-build backpressure
- numeric nonroot image identity compatible with `runAsNonRoot` (fixed in v2.9.0)

## Quick Run

Create the webhook Secret:

```bash
kubectl create namespace cub-scout
kubectl -n cub-scout create secret generic cub-scout-bot-webhook \
  --from-literal=url='https://events.example.com/cub-scout'
```

Apply the bot manifest:

```bash
kubectl apply -f examples/bot/deployment.yaml
```

Inspect the Pod:

```bash
kubectl -n cub-scout logs deploy/cub-scout-bot
```

## Scope Tuning

The default manifest observes all namespaces. To scope it down, set:

```yaml
- name: CUB_SCOUT_BOT_NAMESPACE
  value: prod
```

The RBAC is intentionally read-only. If you remove access to a resource type,
cub-scout will omit that evidence or report partial observations rather than
mutating the cluster.

## Event Types

The bot emits the same event types as `watch`:

- `resource.discovered`
- `ownership.changed`
- `drift.detected`
- `scan.finding`

Use `CUB_SCOUT_BOT_EMIT_RECEIPT_ON=all` to attach inline receipts where
supported. `CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP` bounds receipt-building per
poll so bursts do not turn into unbounded API pressure.
