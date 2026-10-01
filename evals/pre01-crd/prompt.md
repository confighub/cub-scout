Use only the supplied files in `cluster/`. They record sequential API reads and
dependent apply operations in one owned kind lifecycle. Preserve the raw
responses as recorded: an HTTP 404 body that is not a Kubernetes `Status` is
not a typed object-level NotFound response. The calls are not an atomic
snapshot. Do not contact a live cluster or network or use evidence outside the
supplied files.

Determine what the CRD read and dependent apply showed before setup, what the
CRD and API discovery reads showed after setup, and what the repeated apply
and object read showed. Distinguish route absence, typed NotFound, apply
failure, CRD establishment, API registration, and created-object identity.
Use the exact identity fields and values from the final ServiceMonitor GET.
Do not infer operator reconciliation, Prometheus discovery, or target health
from CRD establishment or object creation.

Return one bare JSON object, with exactly these string-valued keys in this
order and no surrounding prose:
`crd_name`, `dependent_operation`, `crd_before`, `pre_setup_apply`, `crd_after_failed_apply`, `crd_registration`,
`api_discovery`, `object_before_successful_apply`, `post_setup_apply`,
`object_identity`, `health_claim`, `evidence`.

Encoding contract:

- `crd_name`: exact CRD metadata name.
- `dependent_operation`: `verb|apiVersion|kind|namespace/name` for the authored
  dependent apply.
- `crd_before` and `crd_after_failed_apply`: `ABSENT`, `PRESENT`, or `UNKNOWN`.
- `pre_setup_apply`: `MISSING_GVK_FAILURE`, `SUCCEEDED`, or `UNKNOWN`.
- `crd_registration`: `ESTABLISHED`, `NOT_ESTABLISHED`, or `UNKNOWN`.
- `api_discovery`: `REGISTERED`, `NOT_REGISTERED`, or `UNKNOWN`.
- `object_before_successful_apply`: `TYPED_NOT_FOUND`, `PRESENT`, or `UNKNOWN`.
- `post_setup_apply`: `CREATED`, `FAILED`, or `UNKNOWN`.
- `object_identity`: `apiVersion|kind|namespace/name|uid|resourceVersion` from
  the final object GET.
- `health_claim`: `NOT_PROVEN`, `PROVEN`, or `UNKNOWN`.
- `evidence`: supplied filenames joined with `+` in this order:
  `capture-scope.json+absent-crd.json+absent-discovery.body+absent-servicemonitor.body+absent-after-apply-crd.json+absent-after-apply-servicemonitor.body+present-crd.json+present-discovery.json+present-before-apply-servicemonitor.json+present-servicemonitor.json+servicemonitor.normalized.yaml+absent-apply.stdout.txt+absent-apply.stderr.txt+present-apply.stdout.txt+present-apply.stderr.txt`.
