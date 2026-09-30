#!/usr/bin/env bash
# Offline preflight regression: preserve existing evidence and reject an
# unpinned/mismatched Helm executable before any cluster-changing command.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/run-helm-version-matrix.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir "$TMP_DIR/nonempty-evidence"
printf 'keep-me\n' >"$TMP_DIR/nonempty-evidence/sentinel"
make_stub() {
	local path=$1 body=$2
	printf '#!/usr/bin/env bash\n%s\n' "$body" >"$path"
	chmod +x "$path"
}
make_stub "$TMP_DIR/helm3" 'echo v3.22.0+g144ca65'
make_stub "$TMP_DIR/helm4" 'echo v4.1.4+g05fa379'
# shellcheck disable=SC2016 # Bodies are deliberately emitted literally into isolated shims.
make_stub "$TMP_DIR/kind" 'printf "%s\n" "$*" >>"$KIND_CALLS"; if [[ "${1:-}" == version ]]; then echo "kind v0.31.0 gofake darwin/arm64"; fi'
make_stub "$TMP_DIR/kubectl" 'exit 0'
make_stub "$TMP_DIR/jq" 'exit 0'
mkdir "$TMP_DIR/bin"
# shellcheck disable=SC2016 # Shim bodies must keep their runtime expansions literal.
make_stub "$TMP_DIR/bin/shasum" 'if [[ "$3" == */helm3 ]]; then printf "8566ea7d76445d174050eca068bc6d685ba3607de8430524a809a16f505a6138  %s\n" "$3"; else printf "11e3c9fb6548fa1661a72000a6a483a31f1c2a0bf300f3d2422270feee180d34  %s\n" "$3"; fi'
# shellcheck disable=SC2016 # Runtime parameters are evaluated inside the generated shim.
make_stub "$TMP_DIR/kind-with-cluster" 'printf "%s\n" "$*" >>"$KIND_CALLS"; case "${1:-}" in version) echo "kind v0.31.0 gofake darwin/arm64";; get) echo "unrelated-existing-cluster";; create) echo "unexpected create" >&2; exit 99;; esac'

export KIND_CALLS="$TMP_DIR/kind-calls"
if HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" \
	"$SCRIPT" "$TMP_DIR/nonempty-evidence" >"$TMP_DIR/existing.out" 2>"$TMP_DIR/existing.err"; then
	echo "expected refusal for non-empty evidence directory" >&2
	exit 1
fi
grep -Fq "refusing to overwrite existing evidence" "$TMP_DIR/existing.err"
test "$(cat "$TMP_DIR/nonempty-evidence/sentinel")" = keep-me

mkdir "$TMP_DIR/empty-evidence"
if HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" \
	"$SCRIPT" "$TMP_DIR/empty-evidence" >"$TMP_DIR/mismatch.out" 2>"$TMP_DIR/mismatch.err"; then
	echo "expected refusal for a Helm binary whose pinned hash does not match" >&2
	exit 1
fi
grep -Fq "Helm 3 binary SHA-256 mismatch" "$TMP_DIR/mismatch.err"
if grep -Fq "create cluster" "$TMP_DIR/kind-calls"; then
	echo "preflight mismatch unexpectedly attempted cluster creation" >&2
	exit 1
fi
test -z "$(find "$TMP_DIR/empty-evidence" -mindepth 1 -print -quit)"

mkdir "$TMP_DIR/shared-cluster-evidence"
GO_DIR="$(dirname "$(command -v go)")"
if PATH="$TMP_DIR/bin:$GO_DIR:/usr/bin:/bin" HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" \
	KIND_BIN="$TMP_DIR/kind-with-cluster" KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" \
	"$SCRIPT" "$TMP_DIR/shared-cluster-evidence" >"$TMP_DIR/shared.out" 2>"$TMP_DIR/shared.err"; then
	echo "expected refusal when an existing kind cluster is present" >&2
	exit 1
fi
grep -Fq "refusing to start while any kind cluster exists" "$TMP_DIR/shared.err"
if grep -Fq "create cluster" "$TMP_DIR/kind-calls"; then
	echo "existing-cluster preflight unexpectedly attempted cluster creation" >&2
	exit 1
fi
test -z "$(find "$TMP_DIR/shared-cluster-evidence" -mindepth 1 -print -quit)"

echo "Helm version matrix safety preflight tests passed"
