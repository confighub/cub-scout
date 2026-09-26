---
type: agent
abort_when: never; answer every call, using the not-found reply for anything unrecognised
---

You stand in for the cub-scout MCP `trace` tool. Every answer below was recorded
from a real `cub-scout mcp serve` against this cluster. Reply with one of them
exactly as written: no commentary, no reformatting, no summary, no code fences.

Pick the recording by the call's `resource` argument, which must be
`KIND/NAME`. KIND matches a Deployment when, ignoring case, it is `deploy`,
`deployment` or `deployments`. The `namespace` argument, if given, must match
the recording's namespace.

If `resource` has no `/` (a bare name), reply exactly:

tool command failed (trace RESOURCE -n NAMESPACE --format json): Error: invalid resource format: use kind/name (e.g., deployment/nginx)

If it has a `/` but matches none of the recordings below, reply exactly:

tool command failed (trace RESOURCE -n NAMESPACE --format json): Error: deployments.apps "NAME" not found

In both, take RESOURCE, NAMESPACE and NAME from the call; that is what the real
tool returns.

## Recording: Deployment `checkout` in namespace `shop`

{{file:fixtures/trace/checkout.txt}}

## Recording: Deployment `cart` in namespace `shop`

{{file:fixtures/trace/cart.txt}}

## Recording: Deployment `payments-api` in namespace `payments`

{{file:fixtures/trace/payments-api.txt}}

## Recording: Deployment `inventory` in namespace `inventory`

{{file:fixtures/trace/inventory.txt}}

## Recording: Deployment `hotfix-worker` in namespace `default`

{{file:fixtures/trace/hotfix-worker.txt}}

## Recording: Deployment `debug-nginx` in namespace `temp-testing`

{{file:fixtures/trace/debug-nginx.txt}}
