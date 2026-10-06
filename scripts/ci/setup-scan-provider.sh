#!/usr/bin/env bash
# Private release access is scoped to this step; never retain credentials.
set -euo pipefail
umask 077
: "${CUB_SCAN_RELEASE_TOKEN:?Set CUB_SCAN_RELEASE_TOKEN to a read-only credential for the scanner release repository}"
: "${RUNNER_TEMP:?Expected disposable runner directory}"
: "${GITHUB_PATH:?Expected GitHub Actions path file}"
root="$RUNNER_TEMP/scout-scan-v073"
[[ ! -e "$root" ]] || { echo 'Refusing to reuse scanner installation directory' >&2; exit 1; }
mkdir -p "$root"
GH_TOKEN="$CUB_SCAN_RELEASE_TOKEN" gh api -H 'Accept: application/octet-stream' \
  repos/confighubai/confighub-scan/releases/assets/527712349 > "$root/release.tar.gz"
printf '%s  %s\n' 0cd0d284975b43689b11cb7f3222f7be81bb75924fe28dae68e0fa94e5f1ce90 "$root/release.tar.gz" | sha256sum --check --status
tar -xzf "$root/release.tar.gz" -C "$root"
provider="$root/confighub-scan-linux-amd64"
"$provider/cub-scan" --capabilities > "$root/capabilities.json"
python3 - "$root/capabilities.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as stream:
    capabilities = json.load(stream)
if capabilities.get("version") != "v0.7.3":
    sys.exit("Scanner does not report required v0.7.3")
PY
printf '%s\n' "$provider" >> "$GITHUB_PATH"
echo 'Checksum-verified scanner v0.7.3 provisioned'
