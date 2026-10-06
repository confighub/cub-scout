# Same-name composition children (2.14 candidate)

`inventory.yaml` is an authored offline control. It pairs an `XApp/shared`
with a `ConfigMap/shared`, and a `WebApp/checkout` with a
`Deployment/checkout`. Matching names do not make these the same resource.

The composition model retains both children and their actual type, namespace
and API group/version. Its existing JSON map shape and keys are unchanged;
human composition output prints each child. The existing trace lineage output
and loaded TUI trace pane also retain those child references. The deterministic
control `TestCompositionSameNameDistinctChildren` reads this file, reverses input
order, checks both composition groups, round-trips JSON, and checks human and
loaded trace output without cluster reads.

A missing or ambiguous parent remains partial under the lineage resolver's
rules. This correction neither fills missing metadata nor establishes physical
UID/cluster identity, health, controller acceptance or automatic TUI action
routing. Same-display-reference composition root collisions remain a separate
compatibility concern; this example does not certify those joins.
