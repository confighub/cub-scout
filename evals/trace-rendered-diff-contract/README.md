# Trace rendered-diff interpretation contract

This opt-in agent-facing case is a synthetic, offline interpretation exercise
for the public Trace rendered-diff result shape. Its UID, resource version and
timestamp are teaching markers, not recorded cluster evidence. The fixture
does not evaluate controller-desired rendering, applied revisions, future
reconciliation, or agent benefit. It has not been run with a model.

The fixture is owned byte-for-byte by `scaffold.sh`; unit tests verify both
inputs and the exact staged file set. No renderer, cluster, network, paid call,
or MCP process is involved.
