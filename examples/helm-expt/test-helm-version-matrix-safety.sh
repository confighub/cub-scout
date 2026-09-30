#!/usr/bin/env bash
# Offline preflight regression: preserve evidence, reject the owned cluster
# name, ignore unrelated clusters, and clean up only the private owned cluster.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/run-helm-version-matrix.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
mkdir -p "$TMP_DIR/bin"
SYSTEM_PATH=$PATH

make_stub() {
	local path=$1 body=$2
	printf '#!/usr/bin/env bash\n%s\n' "$body" >"$path"
	chmod +x "$path"
}
make_stub "$TMP_DIR/helm3" 'echo v3.22.0+g144ca65'
make_stub "$TMP_DIR/helm4" 'echo v4.1.4+g05fa379'
make_stub "$TMP_DIR/kubectl" 'exit 0'
make_stub "$TMP_DIR/jq" 'exit 0'
# shellcheck disable=SC2016 # The generated fake hash command expands at runtime.
make_stub "$TMP_DIR/bin/shasum" 'case "$3" in */helm3) h=8566ea7d76445d174050eca068bc6d685ba3607de8430524a809a16f505a6138;; *) h=11e3c9fb6548fa1661a72000a6a483a31f1c2a0bf300f3d2422270feee180d34;; esac; printf "%s  %s\n" "$h" "$3"'
# shellcheck disable=SC2016 # The generated fake kind evaluates these parameters at runtime.
make_stub "$TMP_DIR/kind" 'printf "env=%s args=%s\n" "${KUBECONFIG:-}" "$*" >>"$KIND_CALLS"; case "${1:-}" in version) echo "kind v0.31.0 gofake darwin/arm64";; get) printf "%s\n" "${KIND_CLUSTERS:-}";; create) exit "${KIND_CREATE_EXIT:-0}";; delete) exit 0;; esac'
# shellcheck disable=SC2016 # The generated Go shim evaluates args at runtime.
make_stub "$TMP_DIR/go-fail-build" 'case "${1:-}" in version) echo "go version go1.25.0 test";; build) echo build >>"$GO_CALLS"; exit 77;; esac'
# shellcheck disable=SC2016 # The generated fake Go command writes only to the -o path supplied by the harness.
make_stub "$TMP_DIR/go-build" 'case "${1:-}" in version) echo "go version go1.25.0 test";; build) while (($#)); do if [[ "$1" == -o ]]; then shift; printf "#!/usr/bin/env bash\nexit 0\n" >"$1"; chmod +x "$1"; break; fi; shift; done;; esac'

export KIND_CALLS="$TMP_DIR/kind-calls"
export GO_CALLS="$TMP_DIR/go-calls"
: >"$KIND_CALLS"

# Existing caller evidence is preserved before any executable version call.
mkdir "$TMP_DIR/nonempty-evidence"
printf 'keep-me\n' >"$TMP_DIR/nonempty-evidence/sentinel"
if HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" GO_BIN="$TMP_DIR/go-fail-build" \
	"$SCRIPT" "$TMP_DIR/nonempty-evidence" >"$TMP_DIR/existing.out" 2>"$TMP_DIR/existing.err"; then
	echo "expected refusal for non-empty evidence directory" >&2
	exit 1
fi
grep -Fq "refusing to overwrite existing evidence" "$TMP_DIR/existing.err"
test "$(cat "$TMP_DIR/nonempty-evidence/sentinel")" = keep-me

# Wrong Helm bytes fail by hash before either Helm binary is executed.
mkdir "$TMP_DIR/mismatch-evidence"
if HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" GO_BIN="$TMP_DIR/go-fail-build" \
	"$SCRIPT" "$TMP_DIR/mismatch-evidence" >"$TMP_DIR/mismatch.out" 2>"$TMP_DIR/mismatch.err"; then
	echo "expected refusal for a Helm binary whose pinned hash does not match" >&2
	exit 1
fi
grep -Fq "Helm 3 binary SHA-256 mismatch" "$TMP_DIR/mismatch.err"
test ! -s "$KIND_CALLS"

# The exact owned name blocks use; unrelated cluster names are preserved.
mkdir "$TMP_DIR/owned-collision-evidence"
: >"$GO_CALLS"
if PATH="$TMP_DIR/bin:$SYSTEM_PATH" KIND_CLUSTERS=$'other-cluster\nscout-helm-version-matrix' \
	HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" GO_BIN="$TMP_DIR/go-fail-build" \
	"$SCRIPT" "$TMP_DIR/owned-collision-evidence" >"$TMP_DIR/owned.out" 2>"$TMP_DIR/owned.err"; then
	echo "expected refusal for the pre-existing owned cluster" >&2
	exit 1
fi
grep -Fq "refusing to reuse owned kind cluster: scout-helm-version-matrix" "$TMP_DIR/owned.err"
test ! -s "$GO_CALLS"
if grep -Fq 'args=delete cluster' "$KIND_CALLS"; then echo "owned-name collision triggered delete" >&2; exit 1; fi

# An unrelated cluster passes the ownership preflight; stop at a fake build
# failure before kind create, proving it is neither adopted nor deleted.
mkdir "$TMP_DIR/unrelated-evidence"
: >"$KIND_CALLS"
: >"$GO_CALLS"
if PATH="$TMP_DIR/bin:$SYSTEM_PATH" KIND_CLUSTERS='unrelated-cluster' \
	HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" GO_BIN="$TMP_DIR/go-fail-build" \
	"$SCRIPT" "$TMP_DIR/unrelated-evidence" >"$TMP_DIR/unrelated.out" 2>"$TMP_DIR/unrelated.err"; then
	echo "expected fake build stop after unrelated cluster preflight" >&2
	exit 1
fi
grep -Fxq build "$GO_CALLS"
if grep -Fq 'args=create cluster' "$KIND_CALLS" || grep -Fq 'args=delete cluster' "$KIND_CALLS"; then
	echo "unrelated cluster preflight reached kind create/delete" >&2
	exit 1
fi

# If creation fails after being attempted, cleanup targets only the fixed
# owned name and passes the same private kubeconfig in env and arguments.
mkdir "$TMP_DIR/create-failure-evidence"
: >"$KIND_CALLS"
if PATH="$TMP_DIR/bin:$SYSTEM_PATH" KIND_CLUSTERS='unrelated-cluster' KIND_CREATE_EXIT=42 \
	HELM3_BIN="$TMP_DIR/helm3" HELM4_BIN="$TMP_DIR/helm4" KIND_BIN="$TMP_DIR/kind" \
	KUBECTL_BIN="$TMP_DIR/kubectl" JQ_BIN="$TMP_DIR/jq" GO_BIN="$TMP_DIR/go-build" \
	"$SCRIPT" "$TMP_DIR/create-failure-evidence" >"$TMP_DIR/create.out" 2>"$TMP_DIR/create.err"; then
	echo "expected fake kind create failure" >&2
	exit 1
fi
grep -Fq 'args=create cluster --name scout-helm-version-matrix' "$KIND_CALLS"
grep -E '^env=.*/scout-helm-matrix\.[^/]+/kubeconfig args=delete cluster --name scout-helm-version-matrix --kubeconfig .*/scout-helm-matrix\.[^/]+/kubeconfig$' "$KIND_CALLS" >/dev/null
if grep -Fq 'delete cluster --name unrelated-cluster' "$KIND_CALLS"; then
	echo "cleanup targeted unrelated cluster name" >&2
	exit 1
fi
test "$(cat "$TMP_DIR/create-failure-evidence/exit-status.txt")" = 42

echo "Helm version matrix safety preflight tests passed"
