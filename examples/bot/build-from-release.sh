#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "Usage: bash examples/bot/build-from-release.sh vMAJOR.MINOR.PATCH amd64|arm64 [local-image:tag]" >&2
  exit 1
fi
version=$1
arch=$2
if [[ ! $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "An explicit stable release tag is required (for example v2.10.1)." >&2
  exit 1
fi
case "$arch" in
  amd64|arm64) ;;
  *) echo "Architecture must be amd64 or arm64, matching the target nodes." >&2; exit 1 ;;
esac
image=${3:-cub-scout-bot:${version}-${arch}}
case "$image" in
  ""|[!a-z0-9]*|*[!a-z0-9._/:@-]*) echo "Invalid local image name." >&2; exit 1 ;;
esac
for tool in curl tar docker awk; do
  command -v "$tool" >/dev/null || { echo "Required tool unavailable: $tool" >&2; exit 1; }
done
if command -v sha256sum >/dev/null; then
  checksum_tool=sha256sum
elif command -v shasum >/dev/null; then
  checksum_tool=shasum
else
  echo "Install sha256sum or shasum before building." >&2
  exit 1
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
archive="cub-scout_${version#v}_linux_${arch}.tar.gz"
base="https://github.com/confighub/cub-scout/releases/download/$version"
for asset in checksums.txt "$archive"; do
  curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --max-time 120 --output "$stage/$asset" "$base/$asset"
done

# Require exactly one checksum for exactly this archive, not a substring match.
expected=$(awk -v name="$archive" 'NF == 2 && $2 == name {print $1}' "$stage/checksums.txt")
if [[ ! $expected =~ ^[0-9a-f]{64}$ ]]; then
  echo "Missing, duplicate or malformed checksum for $archive." >&2
  exit 1
fi
if [[ $checksum_tool == sha256sum ]]; then
  actual=$(sha256sum "$stage/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$stage/$archive" | awk '{print $1}')
fi
if [[ $actual != "$expected" ]]; then
  echo "Checksum mismatch for $archive; no image was built." >&2
  exit 1
fi

mkdir "$stage/image"
# Stream the named member into our own file; do not materialize archive paths.
tar -xOzf "$stage/$archive" cub-scout > "$stage/image/cub-scout"
[[ -s "$stage/image/cub-scout" ]] || { echo "Release archive has no binary payload." >&2; exit 1; }
chmod 0755 "$stage/image/cub-scout"
echo "Verified $archive ($actual). Building local image $image for linux/$arch."
docker build --load --platform "linux/$arch" --file "$root/Dockerfile" --tag "$image" "$stage/image"
echo "Built $image. No image was pushed and no cluster was changed."
