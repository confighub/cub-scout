# Delegated Apply Fixtures

Test fixtures for OCI-based GitOps pipelines (issues #26-#32).

## Flux OCI Fixtures

| File | Scenario | Source Status | Apply Status |
|------|----------|---------------|--------------|
| `flux-oci-healthy.yaml` | Healthy pipeline | Ready | ReconciliationSucceeded |
| `flux-oci-source-auth-failure.yaml` | Registry auth failure | AuthenticationFailed | ArtifactFailed |
| `flux-oci-source-not-found.yaml` | Tag/artifact missing | OCIOperationFailed | ArtifactFailed |
| `flux-oci-apply-failure.yaml` | Build/apply failure | Ready | BuildFailed |
| `flux-oci-revision-mismatch.yaml` | Stuck on old revision | Ready | ReconciliationFailed |
| `flux-oci-cross-namespace.yaml` | Cross-namespace sourceRef | Ready | ReconciliationSucceeded |

## Argo CD OCI Fixtures

| File | Scenario | Sync Status | Health Status |
|------|----------|-------------|---------------|
| `argo-oci-healthy.yaml` | Healthy pipeline | Synced | Healthy |
| `argo-oci-sync-failed.yaml` | Manifest apply failure | OutOfSync | Degraded |
| `argo-oci-chart-not-found.yaml` | Chart not found | Unknown | Unknown |

## Usage

These fixtures provide status data for testing without cluster access:

```go
// Load fixture for testing
data, _ := os.ReadFile("test/fixtures/delegated-apply/flux-oci-healthy.yaml")
```

## Failure Stage Classification

### Source Failures (fetch stage)
- `AuthenticationFailed` - Registry credentials invalid/missing
- `OCIOperationFailed` - Artifact not found, network error

### Apply Failures (reconcile stage)
- `BuildFailed` - Kustomize build error, invalid YAML
- `ReconciliationFailed` - Kubernetes API rejected manifests
- `ArtifactFailed` - Source not ready (depends on source failure)

## Cross-Namespace References

Flux supports cross-namespace sourceRef:
```yaml
sourceRef:
  kind: OCIRepository
  name: shared-manifests
  namespace: flux-system  # Different from Kustomization namespace
```

The `flux-oci-cross-namespace.yaml` fixture tests this pattern.
