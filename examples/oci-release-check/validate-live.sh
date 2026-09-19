#!/usr/bin/env bash
# Read-only live acceptance harness for release check running-image evidence.
set -euo pipefail
umask 077

usage() {
  cat <<'EOF'
Usage:
  examples/oci-release-check/validate-live.sh \
    --bundle oci://repository@sha256:<64-lowercase-hex> \
    --controller Kind/name \
    --api-version group/version \
    --controller-namespace namespace \
    --kube-context target-context \
    [--controller-context management-context] \
    [--max-pods 50] [--max-objects 100] \
    [--expect-verdict PASS|WATCH|BLOCK|INCONCLUSIVE] \
    [--out-dir path] [--binary path] [--oci-layout path] [--no-plugin]

The bundle, controller, API version, namespaces and contexts are caller-supplied.
This script never derives an expected identity from the live cluster and never
creates, updates or deletes Kubernetes objects.
EOF
}

die() {
  printf 'validate-live: %s\n' "$*" >&2
  exit 64
}

redact_stream() {
  sed -E \
    -e 's#(oci://)[^/@:[:space:]]+:[^/@[:space:]]+@#\1[REDACTED]@#Ig' \
    -e 's#((token|password|passwd|secret|authorization|client[-_]?secret)[=:])[[:space:]]*[^[:space:]]+#\1[REDACTED]#Ig'
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "sha256sum or shasum is required to capture the binary checksum"
  fi
}

json_check() {
  local path="$1"
  if [[ ! -s "$path" ]]; then
    return 1
  fi
  jq -e '
    def verdict: IN("PASS", "WATCH", "BLOCK", "INCONCLUSIVE");
    type == "object"
    and .version == "v1"
    and (.verdict | type == "string" and verdict)
    and (.stages | type == "array" and any(.[]; .name == "running-image" and (.verdict | type == "string" and verdict)))
    and (.runningImage | type == "object")
    and (.runningImage.verdict | IN("match", "mismatch", "unknown"))
    and (.runningImage.workloads | type == "array")
  ' "$path" >/dev/null
}

json_pass_check() {
  local path="$1"
  jq -e '
    def supported:
      ((.id.apiVersion == "apps/v1" and .id.kind == "Deployment")
       or (.id.apiVersion == "v1" and .id.kind == "Pod"));
    .verdict == "PASS"
    and .runningImage.verdict == "match"
    and (.runningImage.workloads | length > 0)
    and all(.runningImage.workloads[];
      supported
      and .verdict == "match"
      and (if .id.kind == "Deployment" then (.deployment | type == "object" and .complete == true) else true end)
    )
  ' "$path" >/dev/null
}

json_verdict() {
  local path="$1"
  jq -r '.verdict' "$path"
}

json_canonical() {
  local path="$1"
  jq -S -c . "$path"
}

validate_nonempty() {
  local option="$1"
  local value="$2"
  [[ -n "$value" ]] || die "$option requires a non-empty value"
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || die "$option cannot contain a newline"
}

bundle_ref=""
oci_layout=""
binary_override=""
controller=""
api_version=""
controller_namespace=""
kube_context=""
controller_context=""
max_pods=50
max_objects=100
expected_verdict=""
out_dir=""
skip_plugin=0

while (($# > 0)); do
  case "$1" in
    --bundle)
      (($# >= 2)) || die "--bundle requires a value"
      bundle_ref="$2"
      shift 2
      ;;
    --oci-layout)
      (($# >= 2)) || die "--oci-layout requires a value"
      oci_layout="$2"
      shift 2
      ;;
    --binary)
      (($# >= 2)) || die "--binary requires a value"
      binary_override="$2"
      shift 2
      ;;
    --controller)
      (($# >= 2)) || die "--controller requires a value"
      controller="$2"
      shift 2
      ;;
    --api-version)
      (($# >= 2)) || die "--api-version requires a value"
      api_version="$2"
      shift 2
      ;;
    --controller-namespace)
      (($# >= 2)) || die "--controller-namespace requires a value"
      controller_namespace="$2"
      shift 2
      ;;
    --kube-context)
      (($# >= 2)) || die "--kube-context requires a value"
      kube_context="$2"
      shift 2
      ;;
    --controller-context)
      (($# >= 2)) || die "--controller-context requires a value"
      controller_context="$2"
      shift 2
      ;;
    --max-pods)
      (($# >= 2)) || die "--max-pods requires a value"
      max_pods="$2"
      shift 2
      ;;
    --max-objects)
      (($# >= 2)) || die "--max-objects requires a value"
      max_objects="$2"
      shift 2
      ;;
    --expect-verdict)
      (($# >= 2)) || die "--expect-verdict requires a value"
      expected_verdict="$2"
      shift 2
      ;;
    --out-dir)
      (($# >= 2)) || die "--out-dir requires a value"
      out_dir="$2"
      shift 2
      ;;
    --no-plugin)
      skip_plugin=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

validate_nonempty --bundle "$bundle_ref"
validate_nonempty --controller "$controller"
validate_nonempty --api-version "$api_version"
validate_nonempty --controller-namespace "$controller_namespace"
validate_nonempty --kube-context "$kube_context"
if [[ -n "$controller_context" ]]; then
  validate_nonempty --controller-context "$controller_context"
fi
if [[ -n "$oci_layout" ]]; then
  validate_nonempty --oci-layout "$oci_layout"
fi
if [[ -n "$binary_override" ]]; then
  validate_nonempty --binary "$binary_override"
fi
if [[ -n "$out_dir" ]]; then
  validate_nonempty --out-dir "$out_dir"
fi

[[ "$bundle_ref" =~ ^oci://[^/@[:space:]]+(/[^@[:space:]]+)*@sha256:[0-9a-f]{64}$ ]] \
  || die "--bundle must be an explicit oci://repository@sha256:<64-lowercase-hex> reference"
[[ ! "$bundle_ref" =~ ^oci://[^/]*:[^/]*@ ]] \
  || die "--bundle must not contain registry credentials"
[[ "$controller" =~ ^[^/[:space:]]+/[^/[:space:]]+$ ]] \
  || die "--controller must be exactly Kind/name"
[[ "$api_version" =~ ^[^/[:space:]]+/v[0-9]+(alpha[0-9]+|beta[0-9]+)?$ ]] \
  || die "--api-version must be group/vN, group/vNalphaN or group/vNbetaN"
[[ "$controller_namespace" != */* ]] || die "--controller-namespace must be a namespace, not a path"
case "$expected_verdict" in
  ""|PASS|WATCH|BLOCK|INCONCLUSIVE) ;;
  *) die "--expect-verdict must be PASS, WATCH, BLOCK or INCONCLUSIVE" ;;
esac

[[ "$max_pods" =~ ^[0-9]+$ && "$max_pods" -ge 1 && "$max_pods" -le 200 ]] \
  || die "--max-pods must be an integer from 1 through 200"
[[ "$max_objects" =~ ^[0-9]+$ && "$max_objects" -ge 1 && "$max_objects" -le 100 ]] \
  || die "--max-objects must be an integer from 1 through 100"

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
if [[ -n "$binary_override" ]]; then
  if [[ "$binary_override" == /* ]]; then
    binary="$binary_override"
  else
    binary="$PWD/$binary_override"
  fi
else
  binary="$repo_root/cub-scout"
fi
[[ -x "$binary" ]] || die "missing executable $binary; run: go build -o ./cub-scout ./cmd/cub-scout"
command -v jq >/dev/null 2>&1 || die "jq is required to validate JSON reports"
if [[ -n "$oci_layout" ]]; then
  [[ -d "$oci_layout" ]] || die "--oci-layout must name an existing OCI layout directory"
fi

if [[ -z "$out_dir" ]]; then
  out_dir="$PWD/oci-release-check-live-$(date -u +%Y%m%dT%H%M%SZ)"
  suffix=0
  while [[ -e "$out_dir" ]]; do
    suffix=$((suffix + 1))
    out_dir="$PWD/oci-release-check-live-$(date -u +%Y%m%dT%H%M%SZ)-$suffix"
  done
elif [[ "$out_dir" != /* ]]; then
  out_dir="$PWD/$out_dir"
fi
mkdir -p "$(dirname -- "$out_dir")" || die "cannot create output parent directory: $(dirname -- "$out_dir")"
mkdir "$out_dir" || die "output directory already exists or cannot be created: $out_dir"
tmp_dir=$(mktemp -d "$out_dir/.tmp.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
binary_checksum=$(sha256_file "$binary")
binary_version=$("$binary" version 2>&1 | redact_stream || true)
source_commit=$(git -C "$repo_root" rev-parse HEAD 2>/dev/null || printf 'unavailable')
source_dirty=$(if [[ -n "$(git -C "$repo_root" status --short 2>/dev/null)" ]]; then printf true; else printf false; fi)

{
  printf 'startedAt=%s\n' "$started_at"
  printf 'binary=%s\n' "$binary"
  printf 'binarySHA256=%s\n' "$binary_checksum"
  printf 'binaryVersion=%s\n' "$binary_version"
  printf 'sourceCommit=%s\n' "$source_commit"
  printf 'sourceDirty=%s\n' "$source_dirty"
  printf 'bundle=%s\n' "$bundle_ref"
  printf 'ociLayout=%s\n' "${oci_layout:-not-used}"
  printf 'controller=%s\n' "$controller"
  printf 'apiVersion=%s\n' "$api_version"
  printf 'controllerNamespace=%s\n' "$controller_namespace"
  printf 'kubeContext=%s\n' "$kube_context"
  printf 'controllerContext=%s\n' "${controller_context:-$kube_context}"
  printf 'maxPods=%s\n' "$max_pods"
  printf 'maxObjects=%s\n' "$max_objects"
  printf 'expectedVerdict=%s\n' "${expected_verdict:-not-asserted}"
  printf 'credentials=not captured by this harness; reports may contain cluster metadata\n'
  printf 'redaction=best-effort for retained stderr; inspect evidence before sharing\n'
} >"$out_dir/run-info.txt"

base_args=(
  release check
  --bundle "$bundle_ref"
  --controller "$controller"
  --api-version "$api_version"
  --controller-namespace "$controller_namespace"
  --kube-context "$kube_context"
  --check-running-image
  --max-pods "$max_pods"
  --max-objects "$max_objects"
  --format json
  --fail-on any-non-pass
)
if [[ -n "$oci_layout" ]]; then
  base_args+=(--oci-layout "$oci_layout")
fi
if [[ -n "$controller_context" ]]; then
  base_args+=(--controller-context "$controller_context")
fi

overall_non_pass=0
harness_error=0
reports_seen=0

run_check() {
  local mode="$1"
  shift
  local report="$out_dir/$mode.report.json"
  local stdout_path="$out_dir/$mode.stdout.json"
  local stderr_path="$out_dir/$mode.stderr.log"
  local raw_stderr="$tmp_dir/$mode.stderr.raw"
  local command_path="$out_dir/$mode.command.txt"
  local status
  local verdict

  printf '%q ' "$@" | redact_stream >"$command_path"
  printf '\n' >>"$command_path"

  if "$@" --out "$report" </dev/null >"$stdout_path" 2>"$raw_stderr"; then
    status=0
  else
    status=$?
  fi
  redact_stream <"$raw_stderr" >"$stderr_path"
  printf '%s\n' "$status" >"$out_dir/$mode.exit-code"

  if ! json_check "$report" || ! json_check "$stdout_path"; then
    printf 'mode=%s status=%s report=invalid-or-missing\n' "$mode" "$status" >>"$out_dir/summary.txt"
    printf 'validate-live: %s did not produce valid JSON report and JSON stdout\n' "$mode" >&2
    return 1
  fi
  if [[ "$(json_canonical "$report")" != "$(json_canonical "$stdout_path")" ]]; then
    printf 'validate-live: %s saved report and stdout differ\n' "$mode" >&2
    return 1
  fi

  verdict=$(json_verdict "$report")
  printf 'mode=%s status=%s verdict=%s report=%s\n' "$mode" "$status" "$verdict" "$report" >>"$out_dir/summary.txt"
  reports_seen=$((reports_seen + 1))

  if [[ "$verdict" == PASS ]]; then
    [[ "$status" -eq 0 ]] || {
      printf 'validate-live: %s returned %s for PASS; expected 0\n' "$mode" "$status" >&2
      return 1
    }
    if ! json_pass_check "$report"; then
      printf 'validate-live: %s PASS lacks complete running-image evidence\n' "$mode" >&2
      return 1
    fi
  else
    overall_non_pass=1
    [[ "$status" -eq 2 ]] || {
      printf 'validate-live: %s returned %s for %s; expected 2\n' "$mode" "$status" "$verdict" >&2
      return 1
    }
  fi
  if [[ -n "$expected_verdict" && "$verdict" != "$expected_verdict" ]]; then
    printf 'validate-live: %s verdict=%s, expected=%s\n' "$mode" "$verdict" "$expected_verdict" >&2
    return 1
  fi
  return 0
}

if ! run_check standalone "$binary" "${base_args[@]}"; then
  harness_error=1
fi

plugin_status=skipped
plugin_reason="disabled by --no-plugin"
plugin_config="$tmp_dir/cub-config"
if [[ "$skip_plugin" -eq 0 ]]; then
  if cub_path=$(command -v cub); then
    mkdir -p "$plugin_config/plugins/scout"
    ln -s "$binary" "$plugin_config/plugins/scout/main"
    help_stderr="$tmp_dir/plugin-help.stderr"
    if CUB_CONFIG="$plugin_config" CUB_CONTEXT='' CUB_SPACE='' CUB_TOKEN='' \
      "$cub_path" scout release check --help >"$tmp_dir/plugin-help.stdout" 2>"$help_stderr"; then
      plugin_status=available
      plugin_reason="cub host and local plugin loaded"
      plugin_args=(scout "${base_args[@]}")
      if ! run_check plugin env CUB_CONFIG="$plugin_config" CUB_CONTEXT='' CUB_SPACE='' CUB_TOKEN='' \
        "$cub_path" "${plugin_args[@]}"; then
        harness_error=1
      fi
    else
      plugin_reason="cub host found, but scout release check is unavailable"
      redact_stream <"$help_stderr" >"$out_dir/plugin-detection.stderr.log"
    fi
  else
    plugin_reason="cub command not found"
  fi
fi

{
  printf 'plugin=%s\n' "$plugin_status"
  printf 'pluginReason=%s\n' "$plugin_reason"
  printf 'finishedAt=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >>"$out_dir/run-info.txt"

if [[ "$reports_seen" -eq 0 ]]; then
  harness_error=1
fi
if [[ "$harness_error" -ne 0 ]]; then
  printf 'validate-live: failed; inspect %s for redacted diagnostics\n' "$out_dir" >&2
  exit 1
fi
if [[ "$overall_non_pass" -ne 0 && -z "$expected_verdict" ]]; then
  printf 'validate-live: non-PASS evidence captured in %s\n' "$out_dir" >&2
  exit 2
fi

printf 'validate-live: accepted %s (%s)\n' "$out_dir" "${expected_verdict:-PASS-only gate}"
