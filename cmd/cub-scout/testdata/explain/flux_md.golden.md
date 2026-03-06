## Explain

- **Resource:** `Deployment/payments-api`
- **Namespace:** `prod`
- **Owner:** Flux
- **Source:** https://github.com/acme/platform-config.git (path: ./apps/payments, revision: main@sha1:abc1234)
- **Deployed via:** GitRepository/platform-config -> Kustomization/payments -> Deployment/payments-api
- **Health:** Healthy
- **Risks:** Not assessed
- **Drift:** Unknown

### Try Next

- `cub-scout trace deployment/payments-api -n prod --explain`
- `cub-scout map list -n prod -q "owner=Flux"`
- `cub-scout doctor -n prod`
