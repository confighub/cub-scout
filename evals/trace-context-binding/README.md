# Trace context: real MCP process, local HTTP evidence

At product checkout HEAD `c70bb96326b8c344b644b213b0fc080a8b4d2f3b`, the local
`./cub-scout mcp serve` process and its actual Trace child processes completed
four calls in the final attempt-3 capture in 1.690 seconds, including fixture
listener cleanup.
The [receipt](fixtures/provenance.json) pins binary and capture-script hashes
and records the checkout HEAD reported at capture. It does **not** prove that
the executable was built from a clean tree at that commit; the hash pins the
artifact, while the commit is reported source context only.
The final capture output is preserved at
`/tmp/scout746-mcp-process-proof-3-final`; the earlier attempt-3 output at
`/tmp/scout746-mcp-process-proof-3` is also preserved, not used as the current
receipt.
These are fake HTTP Kubernetes endpoints, not a live Kubernetes/controller run.

Ambient kubeconfig selected Beta. Explicit Alpha produced two Application GETs
and one denied Events GET, retaining the source URL and **declared target**
revision `alpha-branch`. The fixture's observed sync revision was different.
Empty and missing selectors returned MCP errors without extra reads. A separate
denied endpoint returned one Application 403; Beta received zero requests.
All four requests were GETs. The private kubeconfig stayed byte-identical and
was removed; the three owned listeners stopped. No shared configuration,
ConfigHub service, model account or external network endpoint was used.

The denied endpoint models access refusal; it does not prove bearer credentials
or real RBAC enforcement. The first capture attempt remains archived at
`/tmp/scout746-mcp-process-proof-1` on the execution host: it failed on a missing
source URL in JSON. That attempt also used a flawed HTTP bearer-based denial
fixture. The second attempt fixed the production source projection and used a
separate denied endpoint; it remains at
`/tmp/scout746-mcp-process-proof-2`. The final attempt-3 capture uses the
generic method recorder and separate denied endpoint. Do not report the first
attempt as passing evidence.

## Reproduce without model spending

Build from this repository root with cached dependencies, then choose a new
output directory (the capture refuses to overwrite an existing one):

```bash
GOPROXY=off GOTOOLCHAIN=local go build ./cmd/cub-scout
PYTHONDONTWRITEBYTECODE=1 python3 evals/trace-context-binding/capture_mcp.py \
  --binary ./cub-scout --output /tmp/scout-trace-mcp-new-attempt
```

The subprocess has a 45-second timeout, an empty executable search directory,
and only the private kubeconfig. The Scout child executable is the same binary,
not a mock shell shim. The recorder captures and rejects all HTTP verbs other
than GET. The fixture files copy actual successful-run output. The receipt has
no absolute observation timestamp; it is not freshness proof.

## Agent case status

This is an opt-in recorded interpretation case outside the fixed 24-case suite.
Both arms receive identical files. The prompt and strict grader are prepared;
**paid/model execution has not run**. No dollar, credit, quality or speed-saving
claim follows from the subprocess duration or four-request count. Full #746
still needs controller-diff resolution, live proof and final review/CI.
