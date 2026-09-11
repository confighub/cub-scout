# Exact OCI Configuration Release Check

Unreleased, planned v2.11. Tracking: #532 / #502 / #505. This answers:
did this exact configuration release reach this target, and where is it waiting?
It does not deploy, render, retry, force sync, approve or claim application success.

## Run

Use the **configuration bundle** digest, not a container image digest:

```bash
./cub-scout release check \
  --bundle 'oci://registry.example/config@sha256:<64-lowercase-hex-digits>' \
  --controller Application/payments \
  --api-version argoproj.io/v1alpha1 \
  --controller-namespace delivery \
  --controller-context management \
  --kube-context production \
  --format json --out release-check.json
```

For an OCI-backed Kustomization, use `--controller Kustomization/payments`,
`--api-version kustomize.toolkit.fluxcd.io/v1` and the same explicit controller
and target context. `--controller-context` defaults to `--kube-context`.
The controller/source/target adapter boundaries below still apply.

Add `--interactive` instead of `--format`/`--out` for the standalone TUI.
`r` starts a new observation, arrows scroll, and Esc cancels/closes. Refresh
clears the old report before reading; late results cannot replace a newer check.
`cub scout release check` uses the same provider through the plugin. MCP exposes
`release_check` with `bundle`, `controller`, `api_version`,
`controller_namespace`, `context`, optional `controller_context`, `oci_layout`
and `max_objects`. Repeated calls are fresh checks, not shared cached verdicts.

## What The Report Means

| Stage | Evidence | Boundary |
|---|---|---|
| Bundle | Manifest and layer bytes match their SHA-256 identifiers | Content identity, not signature or intended-release authority |
| Controller | Exact repository, revision, target binding and desired inventory coverage | Reported comparison/apply evidence, not execution history or controller liveness |
| Configuration | Every authored field in the supplied literal object set compared with live | Server-added map fields, status and extra-object closure are outside this comparison |
| Workloads | Shared current live observations evaluated by the existing convergence model | Supported workload-controller status, not pod image identity, process reload, functional or traffic success |

`PASS` is scoped to those checks. `WATCH` includes a different reported bundle
or an incomplete rollout. `BLOCK` includes missing objects, authored-field
differences and workload failure. `INCONCLUSIVE` means required evidence could
not be checked. Known failures remain visible even when another check is
inconclusive. No supported workloads is `NOT_ASSESSED` for that stage, never
"the application is running". Configuration checks still run when controller
evidence is unavailable. An unavailable discovery API is inconclusive, not a
missing object; absence requires a 404 from the actual object GET.

`--fail-on any-non-pass` exits 2 after preserving the report. Invalid arguments
exit 1. Without that flag, an inconclusive report can exit 0: inspect `verdict`.
The report contains existing, independently fingerprint-verifiable configuration
and workload receipts. `--out` overwrites a regular report file, not the immutable
receipt store. The whole report is not itself fingerprinted or signed.

## Input And Adapter Boundaries

- One OCI image manifest with one tar or tar+gzip layer containing literal
  YAML/JSON objects. Indexes, multiple layers, charts, rendering inputs, Lists,
  duplicate identities/keys, archive links and unsafe paths are rejected.
- Namespaced objects must include `metadata.namespace`. Scout does not guess
  custom-resource scope or silently substitute a default namespace. Secret
  payloads are excluded from live reads and cause incomplete set coverage.
- Single-source Application: whole-bundle directory source; nested files need
  `directory.recurse`. No Helm/Kustomize/plugin rendering or directory filters.
  Destination server must match the target reader's API endpoint, or be the
  in-cluster server with the same context. Named destinations remain unknown.
- Kustomization v1 with OCIRepository v1: current-generation source/controller
  evidence, matching repository and artifact revision, whole-bundle path, no
  transforms, layer selection or remote kubeConfig. No source index is guessed.
- Other controller APIs do not get a fabricated adapter. Live configuration
  evidence can still be useful; the release-level result stays inconclusive.
- Workload convergence covers apps/v1 Deployment, StatefulSet and DaemonSet,
  batch/v1 Job and v1 Pod. Custom-resource, CronJob and other readiness is not
  assessed. Pod fan-out, image verification and application checks are separate.
- Intended bundle identity is supplied by the caller. There is no release-number
  lookup, ConfigHub authority/history join or production event-cursor read.
- Watch/bot release scheduling remains follow-up work. The existing watch/bot
  event and receipt behavior is unchanged.

## Read Budget

- `--max-objects` is 1-100, default 100. Oversized bundles are rejected before
  Kubernetes requests, not silently truncated.
- Each exact resource read costs at most one discovery GET and one object GET,
  with no REST retries and no redirects. No object LIST or broad discovery.
- For N desired objects: at most `2N + 4` Kubernetes requests for Application,
  or `2N + 8` for Kustomization including its source. Excluded/failed reads may
  cost less. The final controller/source reads check for concurrent change.
- Configuration and convergence share each workload read. Authentication
  transport requests are outside the Kubernetes discovery/object counters.
- Registry traffic includes auth and bounded HTTPS redirects: at most 16 HTTP
  requests, 32 MiB total body bytes, 8 MiB per response, 30 seconds overall.
  Manifest limit 1 MiB, layer 8 MiB, expanded archive 16 MiB, file 1 MiB,
  archive 1,024 entries. Existing Docker credential configuration is read;
  Scout does not log in or write credentials.
- A complete check has a 90-second deadline; each Kubernetes read has its
  existing 10-second deadline and 2 MiB response cap. JSON output is capped at
  4 MiB. Reads are sequential dated observations, not an atomic snapshot.

## Reproduce Without Publishing Or Deploying

The fixture has a Deployment and ConfigMap. Its image stays unchanged when
the negative test changes `LOG_LEVEL`, demonstrating configuration-only drift.

```bash
# The output directory must not already exist. This only writes local fixtures.
go run ./examples/oci-release-check/create-layout \
  examples/oci-release-check/objects.yaml /tmp/scout-release-layout

# Use the printed reference with --bundle and add:
# --oci-layout /tmp/scout-release-layout

go test ./pkg/agent ./cmd/cub-scout -run '^TestRelease' -count=1
```

Tests use a real ORAS client against a TLS registry fixture and exact HTTP
Kubernetes fixtures, with actual CLI/plugin and stdio MCP processes. They do not
publish a bundle or mutate a cluster. This is recorded integration proof, not
an authenticated production release validation.

### Read-Only Live Smoke

An opt-in test reads an existing Deployment and creates a temporary local OCI
fixture containing its identity and observed replica field. This checks live
API integration, not independent intended state or a published release:

```bash
CUB_SCOUT_RELEASE_LIVE_CONTEXT=<non-production-context> \
CUB_SCOUT_RELEASE_LIVE_NAMESPACE=<namespace> \
CUB_SCOUT_RELEASE_LIVE_NAME=<deployment-name> \
go test ./cmd/cub-scout -run '^TestReleaseCheckLive$' -count=1 -v
```

Recorded on 2026-09-11: bundle, configuration and workload-convergence stages
passed for an existing non-production Deployment; controller evidence stayed
`INCONCLUSIVE` because a Deployment is not a supported delivery controller.
The check made three discovery and three object GETs, plus the one preparatory
bounded read outside the report. No deployment, namespace, registry object,
credential or context was changed. Positive controller joins remain fixture
proof until an explicitly scoped live OCI release is available.
