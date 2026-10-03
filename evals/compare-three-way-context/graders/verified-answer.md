# Strict synthetic interpretation grader

Execution status: not run. No quality, cost or savings score is recorded.

Accept only a bare JSON object with the ten specified string keys and values:

```json
{"context":"alpha-context","image":"alpha:v1","rollout_coverage":"INCOMPLETE","denied_inventory":"UNKNOWN","view_membership":"EXCLUDED","revision_role":"DECLARED_TARGET","followup_selector":"--kube-context alpha-context","stable_cluster_identity":"NOT_PROVEN","agreement":"PARTIAL","dollar_savings":"NOT_PROVEN"}
```

Reject ambient Beta substitution, missing/extra keys, claims that unavailable
Pods are healthy, empty denied discovery is empty inventory, cross-space slug
matches are exact membership, or declared branches are applied revisions.
