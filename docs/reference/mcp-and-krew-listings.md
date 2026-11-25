# External Listings: MCP Registries & `kubectl krew` (v3.0.0)

This guide tracks the submission manifests and exact commands for listing
`cub-scout` `v3.0.0` in external MCP directories and `kubectl krew`.

---

## 1. Official MCP Registry (`registry.modelcontextprotocol.io`)

The repository root contains [server.json](../../server.json) following the
official MCP Registry schema (`io.github.confighub/cub-scout`, version `3.0.0`,
`stdio` transport via `cub-scout mcp serve` / `ghcr.io/confighub/cub-scout:v3.0.0`).

Publish with the official `mcp-publisher` CLI (authenticated via GitHub):

```bash
mcp-publisher login github
mcp-publisher publish
```

## 2. Community MCP Directories

Use the following canonical metadata for Glama, Smithery, PulseMCP, and
`modelcontextprotocol/servers`:

- **Name:** `cub-scout` (`io.github.confighub/cub-scout`)
- **Homepage / Repo:** `https://github.com/confighub/cub-scout`
- **Command (stdio):** `cub-scout mcp serve` (or `cub scout mcp serve`)
- **One-line description:** Read-only, deterministic Kubernetes & GitOps witness for AI agents (Flux, Argo CD, Helm, kro, Crossplane, Sveltos) with revision-anchored landing checks (`landed`), field attribution, and declared byte ceilings (99.78% fewer bytes than raw cluster objects).
- **Safety:** Strictly read-only (`readOnlyHint: true` on every tool); all cluster text emitted as quoted data; zero LLM inference.

## 3. `kubectl krew` (`kubernetes-sigs/krew-index`)

`dist/krew/cub-scout.yaml` holds the Krew `v1alpha2` plugin manifest template.
Once the `v3.0.0` GitHub Release finishes uploading `checksums.txt`, stamp the
exact SHA-256 digests and open a PR against `kubernetes-sigs/krew-index` adding
`plugins/cub-scout.yaml`:

```bash
# Fetch v3.0.0 checksums from the published release
curl -fsSL https://github.com/confighub/cub-scout/releases/download/v3.0.0/checksums.txt -o /tmp/cub-scout-v3.0.0-checksums.txt

# Stamp dist/krew/cub-scout.yaml for v3.0.0 (linux/darwin amd64/arm64)
# and validate locally with:
kubectl krew install --manifest=dist/krew/cub-scout.yaml
```
