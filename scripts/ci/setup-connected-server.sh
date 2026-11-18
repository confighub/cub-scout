#!/usr/bin/env bash
# Own a disposable server; retain the existing workload cluster for import tests.
set -euo pipefail
umask 077

root=${1:?Usage: setup-connected-server.sh NEW_PRIVATE_DIRECTORY}
[[ "$root" = /* && "$root" != *$'\n'* && ! -e "$root" ]] || {
  echo 'Expected a new absolute private directory' >&2; exit 1;
}
[[ $(uname -s) = Linux && $(uname -m) = x86_64 ]] || {
  echo 'This CI installer requires Linux amd64' >&2; exit 1;
}
for tool in curl sha256sum kubectl kind; do command -v "$tool" >/dev/null; done
: "${GITHUB_ENV:?Expected GitHub Actions environment file}"
: "${GITHUB_PATH:?Expected GitHub Actions path file}"
clusters=$(kind get clusters)
if printf '%s\n' "$clusters" | awk '$0 == "scout-ci-connected-server" {found=1} END {exit !found}'; then
  echo 'Refusing to reuse an existing server cluster' >&2; exit 1
fi
mkdir -p "$root/bin" "$root/cub-config/plugins/server"
kubectl config view --raw --flatten > "$root/workload.kubeconfig"

download() {
  local url=$1 digest=$2 output=$3
  curl --fail --silent --show-error --location --retry 3 \
    --proto '=https' --proto-redir '=https' "$url" -o "$output"
  printf '%s  %s\n' "$digest" "$output" | sha256sum --check --status
  chmod 700 "$output"
}
download https://github.com/confighub/sdk/releases/download/v0.11.9/cub-linux-amd64 \
  320673da57b21ccec6fcd7b3967ca55284659e193536543c333c2683d683a188 "$root/bin/cub"
download https://github.com/confighub/cub-server/releases/download/v0.3.0/cub-server-linux-amd64 \
  17555178b39655efe99d060a45e63cba8741f3bd38260a2b239ba833942f33a6 "$root/cub-config/plugins/server/server"
cat > "$root/cub-config/plugins/server/cub-plugin.yaml" <<'YAML'
name: server
version: 0.3.0
commands:
  - name: server
    summary: Install and manage a self-hosted ConfigHub server
    entrypoint: server
YAML
export CUB_CONFIG="$root/cub-config"
export PATH="$root/bin:$PATH"
# Never change the runner's original kubeconfig. The installer writes its own
# generated kubeconfig under --out-dir. Logs can contain keys: keep them private.
export KUBECONFIG="$root/server-input.kubeconfig"
touch "$root/owns-server-cluster"
if ! cub server install --cluster-name scout-ci-connected-server \
  --image ghcr.io/confighubai/confighub@sha256:6c25cd220887852f669a351ad7734b7adc179568fceb7c1d645e010070456695 \
  --ui-image ghcr.io/confighub/ui@sha256:99a9a54029813741144d85da8b82e7c001c01f3ade9d2e886ecb720b7b773741 \
  --admin-key-name scout-ci-admin \
  --out-dir "$root/server" > "$root/install.log" 2>&1; then
  echo 'Disposable ConfigHub installation failed; private installer log retained on runner' >&2
  exit 1
fi
cub auth status > "$root/auth-status.txt" 2>&1
cub version > "$root/version.txt"
awk '/^[[:space:]]*Version:/ {if ($2 != "v0.11.9") exit 1; n++} END {if (n != 2) exit 1}' "$root/version.txt"
printf 'CUB_CONFIG=%s\nKUBECONFIG=%s\n' "$CUB_CONFIG" "$root/workload.kubeconfig" >> "$GITHUB_ENV"
printf '%s\n' "$root/bin" >> "$GITHUB_PATH"
echo 'Authenticated disposable ConfigHub v0.11.9; workload kubeconfig preserved'
