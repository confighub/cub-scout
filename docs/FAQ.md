# Frequently Asked Questions

Common questions and answers for cub-scout users.

---

## Getting Started

### How do I install cub-scout?

```bash
# macOS/Linux via Homebrew (recommended)
brew install confighub/tap/cub-scout

# Or build from source
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
```

See [getting-started/install.md](getting-started/install.md) for more options.

### What permissions does cub-scout need?

cub-scout is **read-only by default**. It only needs `get`, `list`, and `watch` permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cub-scout-reader
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
```

See [SECURITY.md](../SECURITY.md) for full details.

### Can I run cub-scout without a cluster?

Yes! Use `--file` to scan local manifests:

```bash
cub-scout scan --file ./manifests/
cub-scout drift --file ./manifests/
```

You can also replay debug bundles offline:

```bash
cub-scout bundle replay ./my-bundle --section drift
```

---

## Ownership Detection

### Why does my Flux resource show as "Native"?

cub-scout detects ownership via labels. If a resource shows as "Native" but should be Flux-owned, check for the toolkit labels:

```bash
kubectl get deploy YOUR-DEPLOY -n YOUR-NS -o yaml | grep toolkit.fluxcd.io
```

**Common causes:**
- Flux Kustomization doesn't have `commonLabels` configured
- Resource was created manually, not by Flux reconciliation
- Using Flux v1 (cub-scout supports Flux v2 only)

See [howto/ownership-detection.md](howto/ownership-detection.md) for details.

### Why do Flux CRDs (GitRepository, Kustomization) show as Native?

This is expected. Flux CRDs themselves are not "owned by Flux" - they ARE the configuration that tells Flux what to deploy. The workloads that Flux creates from those CRDs will show as Flux-owned.

### Does cub-scout detect Terraform-managed resources?

Yes. cub-scout looks for `app.terraform.io/workspace-name` annotation or `app.terraform.io/managed` label.

### Does cub-scout support Crossplane?

Yes (experimental). cub-scout detects `crossplane.io/claim-name` labels. See the [v0.16 release notes](releases/v0.16.0.md) for Crossplane attribution details.

### Can I add detection for custom labels?

Yes. See [howto/extending.md](howto/extending.md) for how to add custom ownership detectors.

---

## Demos

### Demo says "no matches for kind GitRepository"

The demos require Flux or ArgoCD CRDs to be installed. Install Flux first:

```bash
flux install
```

Then run the demo again:

```bash
./cub-scout demo quick
```

### Demo shows all "Native" resources, no Flux/ArgoCD

Same issue - GitOps CRDs are not installed. The demo creates resources with GitOps labels, but without the CRDs, ownership detection can't work.

### How do I clean up demo resources?

```bash
./cub-scout demo quick --cleanup
./cub-scout demo ccve --cleanup
```

---

## Trace Command

### Why does trace show red X for healthy resources?

This is a known issue (#107). The trace command may incorrectly parse Flux status conditions, showing success messages as errors. Use `gitops status` for accurate health:

```bash
./cub-scout gitops status
```

### How do I trace a resource to its Git source?

```bash
./cub-scout trace deploy/my-app -n my-namespace
```

This shows the full chain: Git repo -> Flux/Argo deployer -> Deployment -> Pods.

---

## Orphans

### Why does "map orphans" show system resources?

The orphans view shows ALL resources not managed by GitOps, including system components. This is technically correct but can be noisy.

**Workaround:** Filter to specific namespaces:

```bash
./cub-scout map list -q "owner=Native AND namespace=default"
```

### What should I do with orphan resources?

Options:
1. **Add to Git** - Export and commit to your GitOps repo
2. **Delete** - If not needed
3. **Document** - If intentionally unmanaged (cluster-critical CRDs, emergency configs)

See [howto/find-orphans.md](howto/find-orphans.md) for guidance.

---

## ConfigHub / Connected Mode

### cub-scout status shows "Connected" but commands fail

The status command may show "Connected" even with an expired token. Re-authenticate:

```bash
cub auth login
```

Then verify:

```bash
cub auth get-token
```

### How do I import workloads to ConfigHub?

```bash
# Preview what would be imported
./cub-scout import -n my-namespace --dry-run

# Import with wizard
./cub-scout import --wizard

# Import directly
./cub-scout import -n my-namespace -y
```

See [howto/import-to-confighub.md](howto/import-to-confighub.md).

### Can I use cub-scout across multiple clusters?

cub-scout is a single-cluster tool. For multi-cluster fleet queries, use ConfigHub:

```bash
cub unit list --space "*"
```

See [howto/fleet-queries.md](howto/fleet-queries.md).

---

## CI/CD Integration

### How do I use cub-scout in GitHub Actions?

```yaml
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Scan for drift
        run: |
          cub-scout drift --file manifests/ --fail-on warning
```

### What exit codes does cub-scout return?

| Code | Meaning |
|------|---------|
| 0 | Success (or no `--fail-on` specified) |
| 1 | Operational error |
| 2 | Findings met `--fail-on` severity threshold |

See [reference/exit-codes.md](reference/exit-codes.md).

### How do I get JSON output for scripts?

All commands support `--json`:

```bash
./cub-scout map list --json | jq '.[] | select(.owner == "Flux")'
./cub-scout scan --json > findings.json
```

---

## TUI (Terminal UI)

### What are the keyboard shortcuts?

Press `?` in the TUI for help. Quick reference:

```
Navigation:  j/k (up/down)  h/l (collapse/expand)  Enter (select)
Views:       s (status)  w (workloads)  o (orphans)  c (crashes)
Actions:     T (trace)  S (scan)  / (search)  q (quit)
```

See [reference/keybindings.md](reference/keybindings.md) for the full list.

### How do I enable shell completion?

```bash
# Bash
source <(./cub-scout completion bash)

# Zsh
source <(./cub-scout completion zsh)

# Fish
./cub-scout completion fish | source

# Add to your shell profile for persistence
echo 'source <(cub-scout completion bash)' >> ~/.bashrc
```

---

## Comparison with Other Tools

### How does cub-scout compare to k9s or Lens?

| Tool | Focus | cub-scout Advantage |
|------|-------|---------------------|
| k9s | General K8s TUI | cub-scout adds GitOps ownership detection |
| Lens | Desktop K8s IDE | cub-scout is CLI-first, deterministic, offline-capable |
| kubectl | Raw API access | cub-scout adds ownership, queries, traces |

cub-scout complements these tools - it answers "who owns this?" and "where did this come from?"

See [concepts/alternatives.md](concepts/alternatives.md).

---

## Troubleshooting

### cub-scout can't connect to my cluster

Verify kubectl works:

```bash
kubectl get pods -A
```

If kubectl works but cub-scout doesn't, check your KUBECONFIG:

```bash
echo $KUBECONFIG
./cub-scout status
```

### Scan shows "No issues found" but I expected findings

Check that:
1. Resources are actually deployed (not just manifests)
2. You're scanning the right namespace
3. The risk pattern matches your configuration

```bash
# List all risk patterns
./cub-scout patterns list

# Scan with verbose output
./cub-scout scan --verbose
```

---

## See Also

- [README.md](../README.md) - Project overview
- [CLI-GUIDE.md](../CLI-GUIDE.md) - Complete command reference
- [getting-started/first-map.md](getting-started/first-map.md) - Quick start guide
- [concepts/alternatives.md](concepts/alternatives.md) - Tool comparison
