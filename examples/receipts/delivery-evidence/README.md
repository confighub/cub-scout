# Receipt Delivery Evidence

Use this example when the question is:

> Did the delegated delivery/status system report back, and can I keep that
> exact observation for review?

`receipt verify --with-confighub` attaches the same object-correlated delivery
evidence used by `trace --with-confighub` and `explain --with-confighub` under
`predicate.evidence.deliveryEvidence`.

```bash
./cub-scout receipt verify deploy/payments-api -n prod \
  --with-confighub \
  --confighub-space payments-prod \
  --confighub-since 24h \
  --confighub-stale-after 15m \
  --format json \
  --out payments-api.delivery.receipt.json
```

What the receipt can freeze:

- live-status writeback freshness
- sync status and operation phase
- separate delivery and application-health verdicts
- recent matching releases and unit events
- observed event-consumer Deployment health
- omissions when exact object-level joins cannot be proven

This is supporting evidence. The selected receipt predicate still owns
`predicate.verdict`; the delivery controller and application-health systems
remain the authorities for their own status.

The [v2.10.1 freshness correction](../../live-delivery-observability/#trusting-feedback-freshness)
retains old/undated reports as supporting evidence without presenting them as
current success or failure. The corrected nested verdict and freshness omission
are fingerprint-covered; they do not override the selected receipt predicate.

Current limitation: `--with-confighub` is single-resource only. Aggregate,
object-set, workload-convergence, and prerequisites receipts reject the flag
upfront rather than silently omitting delivery evidence.
