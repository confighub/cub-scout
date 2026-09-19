#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
harness="$script_dir/validate-live.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

digest=$(printf 'a%.0s' {1..64})
bundle="oci://registry.example/config@sha256:$digest"
layout="$tmp_dir/layout"
fake_bin="$tmp_dir/fake-cub-scout"
fake_path="$tmp_dir/fake-path"
fake_log="$tmp_dir/fake.log"
good_dir="$tmp_dir/nested/live-evidence/good"
mkdir "$layout" "$fake_path"

cat >"$fake_bin" <<'FAKE_BINARY'
#!/usr/bin/env bash
set -u
log=$(printenv FAKE_LOG)
if [[ "$#" -gt 0 && "$1" == version ]]; then
  printf 'fake cub-scout version\n'
  exit 0
fi
if [[ "$#" -ge 3 && "$1" == release && "$2" == check && "$3" == --help ]]; then
  printf 'fake release check help\n'
  exit 0
fi
if IFS= read -r -n 1 stdin_probe; then
  printf 'stdin=not-eof\n' >>"$log"
else
  printf 'stdin=eof\n' >>"$log"
fi
plugin_config=$(printenv CUB_CONFIG || true)
if [[ -n "$plugin_config" ]]; then
  printf 'plugin_config=%s\n' "$plugin_config" >>"$log"
else
  printf 'plugin_config=unset\n' >>"$log"
fi
out=""
layout=""
while (($# > 0)); do
  case "$1" in
    --out) out="$2"; shift 2 ;;
    --oci-layout) layout="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[[ -n "$out" ]] || exit 51
[[ -n "$layout" && -d "$layout" ]] || exit 52
verdict=$(printenv FAKE_VERDICT || true)
[[ -n "$verdict" ]] || verdict=PASS
if [[ "$(printenv LEGACY_PASS || true)" == 1 ]]; then
  printf '{"version":"v1","verdict":"PASS"}\n' >"$out"
elif [[ "$verdict" == PASS ]]; then
  printf '%s\n' '{"version":"v1","verdict":"PASS","stages":[{"name":"running-image","verdict":"PASS"}],"runningImage":{"verdict":"match","workloads":[{"id":{"apiVersion":"apps/v1","kind":"Deployment"},"verdict":"match","deployment":{"complete":true}}]}}' >"$out"
else
  printf '%s\n' "{\"version\":\"v1\",\"verdict\":\"$verdict\",\"stages\":[{\"name\":\"running-image\",\"verdict\":\"INCONCLUSIVE\"}],\"runningImage\":{\"verdict\":\"unknown\",\"workloads\":[]}}" >"$out"
fi
cat "$out"
[[ "$verdict" == PASS ]] && exit 0
exit 2
FAKE_BINARY
chmod 700 "$fake_bin"

cat >"$fake_path/cub" <<'FAKE_CUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -ge 1 && "$1" == scout ]] || exit 61
shift
config=$(printenv CUB_CONFIG || true)
[[ -n "$config" ]] || exit 62
plugin="$config/plugins/scout/main"
[[ -x "$plugin" ]] || exit 63
if [[ "$#" -ge 3 && "$3" == --help ]]; then
  exec "$plugin" "$@"
fi
exec env CUB_PLUGIN=1 "$plugin" "$@"
FAKE_CUB
chmod 700 "$fake_path/cub"

base_args=(
  --bundle "$bundle"
  --controller Application/payments
  --api-version argoproj.io/v1alpha1
  --controller-namespace argocd
  --kube-context production
  --binary "$fake_bin"
  --oci-layout "$layout"
  --no-plugin
)

expect_status() {
  local want="$1"
  shift
  local got
  set +e
  "$@" >/dev/null 2>&1
  got=$?
  set -e
  [[ "$got" -eq "$want" ]] || {
    printf 'expected status %s, got %s: %s\n' "$want" "$got" "$*" >&2
    exit 1
  }
}

expect_status 64 "$harness"
"$harness" --help >/dev/null
expect_status 64 "$harness" "${base_args[@]}" --expect-verdict BOGUS --out-dir "$tmp_dir/bad-verdict"
expect_status 64 "$harness" "${base_args[@]}" --controller-context $'arn:aws:eks:eu-west-2:123456789012:cluster/name\nbad' --out-dir "$tmp_dir/bad-context"
expect_status 64 "$harness" "${base_args[@]}" --bundle oci://registry.example/config:latest --out-dir "$tmp_dir/bad-bundle"

printf 'sentinel-from-parent\n' | PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  "$harness" "${base_args[@]}" \
  --controller-context arn:aws:eks:eu-west-2:123456789012:cluster/name \
  --expect-verdict PASS \
  --out-dir "$good_dir"

jq -e '.verdict == "PASS"' "$good_dir/standalone.report.json" >/dev/null
grep -q '^plugin=skipped$' "$good_dir/run-info.txt"
grep -q 'controllerContext=arn:aws:eks:eu-west-2:123456789012:cluster/name' "$good_dir/run-info.txt"
grep -q '^ociLayout=' "$good_dir/run-info.txt"
grep -q '^stdin=eof$' "$fake_log"

FAKE_VERDICT=BLOCK PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  "$harness" "${base_args[@]}" --expect-verdict BLOCK --out-dir "$tmp_dir/expected-block"
[[ "$(cat "$tmp_dir/expected-block/standalone.exit-code")" == 2 ]]

FAKE_VERDICT=INCONCLUSIVE PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  "$harness" "${base_args[@]}" --expect-verdict INCONCLUSIVE --out-dir "$tmp_dir/expected-inconclusive"
[[ "$(cat "$tmp_dir/expected-inconclusive/standalone.exit-code")" == 2 ]]

FAKE_VERDICT=BLOCK PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  expect_status 2 "$harness" "${base_args[@]}" --out-dir "$tmp_dir/ungated-block"

printf 'sentinel-from-parent\n' | PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  "$harness" \
  --bundle "$bundle" \
  --controller Application/payments \
  --api-version argoproj.io/v1alpha1 \
  --controller-namespace argocd \
  --kube-context production \
  --controller-context arn:aws:eks:eu-west-2:123456789012:cluster/name \
  --binary "$fake_bin" \
  --oci-layout "$layout" \
  --expect-verdict PASS \
  --out-dir "$tmp_dir/plugin"

grep -q '^plugin=available$' "$tmp_dir/plugin/run-info.txt"
grep -q '^pluginReason=cub host and local plugin loaded$' "$tmp_dir/plugin/run-info.txt"
grep -q '^plugin_config=/.*' "$fake_log"
grep -q 'plugin_config=unset' "$fake_log"
grep -q '^stdin=eof$' "$fake_log"

mode_of() {
  local mode
  if mode=$(stat -c %a "$1" 2>/dev/null); then
    printf '%s\n' "$mode"
  else
    stat -f %Lp "$1"
  fi
}
[[ "$(mode_of "$tmp_dir/plugin")" == 700 ]]
[[ "$(mode_of "$tmp_dir/plugin/standalone.report.json")" == 600 ]]

FAKE_VERDICT=BOGUS PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  expect_status 1 "$harness" "${base_args[@]}" --out-dir "$tmp_dir/bogus-report"

LEGACY_PASS=1 PATH="$fake_path:$PATH" CUB_CONFIG='' FAKE_LOG="$fake_log" \
  expect_status 1 "$harness" "${base_args[@]}" --out-dir "$tmp_dir/legacy-pass"

printf 'validate-live_test: pass\n'
