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
this source-preparation packet; do not invoke the capture until independent
review the helper and its pins before any separately authorized bounded serial live capture.

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
