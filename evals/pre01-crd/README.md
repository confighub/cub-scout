# PRE-01 raw CRD prerequisite capture helper

This helper prepares one bounded, owned `kind` run for #708. It captures the
real chart-authored kube-state-metrics `ServiceMonitor` with the actual pinned
`servicemonitors.monitoring.coreos.com` CRD. It does not install a chart or
operator, make a controller-reconciliation claim, run a model, alter a
benchmark case, or promote/publish evidence. A successful dependent apply and
an `Established` CRD prove API registration only; they say nothing about
Prometheus discovery or target health.

## Source controls

The helper accepts an explicit helm-expt Git checkout containing commit
`9ab4c753a888dc305a3c07956c9f8f5a19eb70a0`. It checks full-file SHA-256 pins
for the rendered release objects, base-variant record and lifecycle CRD file
and reads only those committed blobs with `git show`; working-tree edits are
not inputs. It checks full-file SHA-256 pins before parsing YAML. The unique authored object must be exactly
`monitoring.coreos.com/v1 ServiceMonitor/monitoring/kube-prometheus-stack-kube-state-metrics`.
The unique CRD must be the real namespaced v1 CRD with matching group, kind,
plural, served/storage flags and no conversion webhook. Duplicates, source
drift, unsupported conversion and identity mismatch fail closed.

The helper depends on PyYAML; install the pinned dependency with
`python3 -m pip install -r evals/pre01-crd/requirements.txt`. This
implementation and its offline proof use PyYAML 6.0.3. The helper does not install packages. PyYAML parses all source
documents, selects the single matching mapping and serializes that mapping as normalized YAML. The source repository remains
unchanged. Output includes source file hashes/byte counts, parsed-object
hashes, normalized-file hashes and an explicit note that normalization
changes formatting, anchors and comments. The record file must still declare
the external ServiceMonitor CRD prerequisite.

## Capture contract

The capture needs `kind v0.31.0`, the pinned cached Kind node image and a local
Docker socket; these pins are inherited from the tested INV-04 lifecycle
helpers. It creates a fresh uniquely named cluster and a private temporary
admin kubeconfig. A separate observer receives only exact-name CRD `get`,
namespaced exact-name ServiceMonitor `get`, and the read-only discovery paths
needed by this check. Its private kubeconfig has one explicit
`pre01-observer` current context and is supplied through `KUBECONFIG`; the
receipt command has no context-selection flag. The observer has no Secret
access. The API client uses only
loopback HTTPS and three exact GET paths: CRD, API group/version discovery,
and the namespaced ServiceMonitor. It retains raw HTTP bodies, including
non-200 bodies, with status, time, byte count and SHA-256. A Kubernetes
`Status/NotFound` HTTP 404 is absence; 403, malformed NotFound, transport
failure or denied reads leave acceptance unknown and fail the capture.
For the absent phase only, the two unregistered group/version routes may return
the apiserver's exact plain-text `404 page not found` body. This is recorded as
`route-unregistered`, not object absence; the exact CRD GET must independently
return a typed Kubernetes NotFound. The present-phase object absence checkpoint
still requires typed NotFound. These potentially non-JSON bodies use `.body`.

The first actual attempt at capture source `e87c4d8624ec75e880713cd7846cdf3396148593`
stopped on that plain-text discovery response after confirming the CRD's typed
404. The failed attempt is retained, not an accepted case; owned cleanup and
shared/private config integrity passed. A regression guard now preserves the
distinction between route registration and object existence.

One cluster runs two serial phases. The absent phase records CRD/discovery/
object reads and the actual failed apply of the normalized authored
ServiceMonitor. That failure is accepted only when kubectl reports the exact
missing `ServiceMonitor` GVK; permission, timeout and transport failures do
not establish causality. The CRD and dependent-object reads repeat after the
failed apply. Setup then creates **only the pinned CRD** with `kubectl
create`, waits for `Established=True`, and repeats the same dependent apply
using a new kubectl process and fresh cache directory. Before this apply the
observer must still receive exact-object 404; afterward the present phase
records CRD UID/resourceVersion and Established status, discovery, and the
created ServiceMonitor UID/resourceVersion. The phases are sequential rather
than an atomic cluster snapshot.

Separate Scout receipts run against the observer config and the single
declared CRD prerequisite. The helper validates the current typed receipt
shape and expected `BLOCK/missing` and `PASS/present` facts. Those receipts are
saved under `derivedScoutReceipts` as diagnostics and are explicitly excluded
from raw model evidence. Output directories must be new and outside the repo.
The generated cluster name and owner marker gate bounded cleanup. The helper
also records shared-config before/after hashes and private-config hashes
before cleanup; tokens and admin config bytes are never written to output or
argv. Failed status/body evidence is preserved, and acceptance remains
failed/unknown when evidence is missing or denied.

The command requires `--execute` as an explicit acknowledgement and requires
all source and executable paths, full revisions, SHA-256 pins, kubeconfig path
and fresh output path as arguments. A dry-run or actual capture is not part of
this source-preparation packet. Review the helper and its pins before any
separately authorized bounded serial live capture.

## Offline checks

Run the focused suite with bytecode disabled after installing the pinned
PyYAML requirement:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/pre01-crd -v
```

The pinned external-source audit is optional: set `HELM_EXPT_SOURCE` to a clone
containing the pinned commit to enable it. Regular source and semantic guards
always run. CI installs `requirements.txt` and runs those guards; the external
checkout audit remains optional when that separate checkout is not available.
No test creates a cluster or makes a network/model call. These checks do not
replace review, live capture acceptance or later case/scaffold preparation.


# PRE-01 raw CRD prerequisite recording

This case packages the independently accepted capture from 2026-10-01. It is
prepared for review, not run as a benchmark and not admitted to paid execution.
The fixture contains nine byte-preserved API response bodies, the authored
normalized ServiceMonitor applied in both phases, both dependent-apply
stdout/stderr pairs, and a factual capture-scope index. `scaffold.sh` copies the
same files into each arm's `cluster/` directory.

The capture used one owned kind cluster and serial absent/present phases; it is
not an atomic snapshot. The CRD was absent on the exact CRD read before and
after the failed dependent apply. The discovery and object routes initially
returned raw `404 page not found` bodies, which are preserved and are not
typed Kubernetes object-level NotFound responses. After creating only the
CRD, the CRD GET showed `Established=True`, API discovery listed
ServiceMonitor, a typed object NotFound preceded the repeated apply, and the
final GET returned the created object. Those facts establish API registration
and resource creation only. No operator or controller was installed, so
reconciliation, Prometheus discovery, and target health were not measured.

Source provenance pins the helm-expt revision and full source-file hashes,
PyYAML 6.0.3 normalization, capture helper revision/hash, cub-scout source and
binary hash, and timestamps. The 3.87 MB authored CRD source file and its
normalized duplicate are intentionally excluded: the exact registered CRD API
response is present. The authored ServiceMonitor is included as normalized
YAML; normalization changes formatting, anchors, and comments while retaining
the selected parsed mapping. See `fixtures/capture-scope.json` for the raw file
inventory, hashes, observed request ordering and operation status/times.

The raw fixture omits derived Scout prerequisite receipts and their validation,
credential-bearing kubeconfigs or tokens, credentials/config hashes, owned
cluster marker, Kubernetes caches, and unrelated setup output. The old CRD
source projection is not substituted for the actual API response. Raw API
bodies and dependent apply outputs remain unchanged.

The strict grader requires one exact JSON object and rejects extra keys,
duplicate keys, extra prose, unsupported values, incorrect object identity, or
claims of health. Offline staging/hash and grader-negative tests do not make a
model call or contact a cluster. Preparation does not establish harness
ordinary-tool/MCP parity and does not authorize paid runs.
