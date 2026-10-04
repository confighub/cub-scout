# Recorded typed DeploymentList contract

This authored opt-in case copies the original RUL-03 readable response bytes,
with SHA-256 `e0102f91e1417c8554ed377352b50450f2376e69d40632f109ea012c5c1560a2`.
It checks normalized item type and input-wide derivation provenance under the
exact apps/v1 DeploymentList envelope. It claims no model execution, Scout
invocation, live state or benchmark admission. Frozen cases remain unchanged.

The shared loader's deterministic Go controls exercise this same source across
map, explain, MCP and TUI; CLI smoke uses the actual local binary. Run the authored
scaffold/answer controls offline with:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/recorded-typed-list -v
```
