# Trace Context Troubleshooting

Use this when `cub-scout trace` fails because the local context for a GitOps tool points to a stale or invalid endpoint, or when ConfigHub connected enrichment is unavailable.

cub-scout detects common context issues automatically and prints remediation steps. This guide documents the full troubleshooting path for each tool.

---

## Graceful Degradation

cub-scout follows a strict graceful degradation policy for trace:

| Scenario | Behavior |
|----------|----------|
| Flux context stale | Error with numbered remediation steps |
| ArgoCD context stale | Error with numbered remediation steps |
| Helm kubeconfig invalid | Error with numbered remediation steps |
| ConfigHub auth expired | **Warning only** — standalone trace succeeds |
| ConfigHub API unreachable | **Warning only** — standalone trace succeeds |
| ConfigHub space not found | **Warning only** — standalone trace succeeds |

**Key principle:** ConfigHub enrichment failure never blocks trace. Standalone trace always completes. Context diagnostics appear as warnings on stderr (CLI) or as a structured remediation box (TUI).

---

## ArgoCD

### Symptoms

- `argocd context appears stale or invalid`
- `authentication required`, `token is expired`, `unauthorized`
- `connection refused`, `i/o timeout`, `dial tcp`

### Inspect and reset path

1. Inspect current context:

```bash
argocd context
```

2. Verify endpoint reachability/auth:

```bash
argocd app list
```

3. Reset stale context:

```bash
argocd logout <server>
```

4. Re-authenticate:

```bash
argocd login <server>
```

5. Retry trace:

```bash
cub-scout trace --app <app-name>
```

---

## Flux

### Symptoms

- `flux context appears stale or invalid`
- `no matches for kind`, `the server doesn't have a resource type`
- `connection refused`, `i/o timeout`, `x509:`

### Inspect and reset path

1. Verify cluster connectivity:

```bash
kubectl cluster-info
```

2. Check Flux installation:

```bash
flux check
```

3. Install or reinstall Flux if needed:

```bash
flux install
```

4. Switch to the correct cluster context:

```bash
kubectl config use-context <context>
```

5. Retry trace:

```bash
cub-scout trace <kind>/<name> -n <namespace>
```

---

## Helm

### Symptoms

- `helm trace context appears stale or invalid`
- `connection refused`, `i/o timeout`, `x509:`
- `secrets is forbidden`, `cannot list resource`

### Inspect and reset path

1. Verify cluster connectivity:

```bash
kubectl cluster-info
```

2. Check permissions to read Helm release secrets:

```bash
kubectl auth can-i list secrets -n <namespace>
```

3. Switch to the correct cluster context:

```bash
kubectl config use-context <context>
```

4. Verify Helm releases are visible:

```bash
helm list -n <namespace>
```

5. Retry trace:

```bash
cub-scout trace <kind>/<name> -n <namespace>
```

---

## ConfigHub Connected Mode

### Symptoms

- `ConfigHub enrichment failed`
- `unauthorized`, `401`, `token expired`, `forbidden`
- `connection refused`, `timeout`, `no such host`
- `space not found`, `unit not found`, `no worker`

### Important

ConfigHub errors are **warnings only**. Standalone trace always completes with cluster-local data. Connected metadata (unit slug, space, drift detection, remediation URL) will be missing, but the ownership chain and health status are still accurate.

### Inspect and reset path

1. Check authentication status:

```bash
cub auth status
```

2. Re-authenticate:

```bash
cub auth login
```

3. Verify active context:

```bash
cub context get
```

4. Check worker status:

```bash
cub worker list
```

5. Retry trace:

```bash
cub-scout trace <kind>/<name> -n <namespace>
```

---

## TUI Trace Diagnostics

When trace fails in the TUI (interactive `cub-scout map` mode), context errors are displayed as structured remediation boxes instead of raw error strings.

The TUI detects context issues for both Flux and ArgoCD and displays numbered steps to resolve the issue. Press any key to dismiss the error and return to the resource list.

### What the TUI shows

- **Context issue detected**: A structured box with the reason and numbered remediation steps
- **Normal errors**: Standard error message (e.g., "resource not managed by Flux")
- **ConfigHub warnings**: Appear on stderr in CLI mode; in TUI mode, connected metadata fields will be absent but trace succeeds

---

## Detection Patterns

cub-scout detects context issues by pattern-matching on error output. The following patterns trigger remediation messages:

### ArgoCD
- Auth: `server address unspecified`, `not logged in`, `authentication required`, `unauthorized`, `token is expired`
- Network: `connection refused`, `i/o timeout`, `dial tcp`, `no such host`, `x509:`, `tls:`, `rpc error: code = unavailable`

### Flux
- Missing CRDs: `no matches for kind`, `the server doesn't have a resource type`, `no flux object found`, `flux is not installed`
- Network: `connection refused`, `i/o timeout`, `dial tcp`, `x509:`, `tls:`, `no such host`

### Helm
- Network: `connection refused`, `i/o timeout`, `dial tcp`, `x509:`, `tls:`, `no such host`
- Permissions: `forbidden`, `cannot list resource`, `secrets is forbidden`, `unauthorized`

### ConfigHub
- Auth: `unauthorized`, `401`, `token expired`, `forbidden`, `403`, `authentication required`
- Network: `connection refused`, `timeout`, `no such host`, `dial tcp`
- Context: `space not found`, `unit not found`, `no active space`, `no worker`
