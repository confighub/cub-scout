# GitOps status context interpretation case

This opt-in case contains synthetic, fixture-backed summaries for two
independent Kubernetes contexts, including an explicit Modelplane API denial.
It tests whether an agent reports the context label and partial coverage
without promoting either to stable cluster identity or complete health.

**Model execution has not run.** These records do not come from a live cluster
or ConfigHub service and establish no real RBAC or connected-mode behavior.
The scaffold only copies the two checked-in JSON files into a temporary
`recorded/` directory.
