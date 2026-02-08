# Trace Context Troubleshooting (ArgoCD)

Use this when `cub-scout trace` fails because the local `argocd` context points to a stale or invalid endpoint.

## Inspect and reset path

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
