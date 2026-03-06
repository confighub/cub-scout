#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/ask-mode-contract.sh \
    --command "<proposed command>" \
    [--mode standalone|connected|auto] \
    [--confirm yes|no] \
    [--execute]

Contract:
  verify -> dry-run preference -> explicit confirm (for high-risk actions)

Examples:
  ./scripts/ask-mode-contract.sh --mode connected --command "./cub-scout import -n payments"
  ./scripts/ask-mode-contract.sh --mode connected --command "./cub-scout import -n payments" --confirm yes
  ./scripts/ask-mode-contract.sh --mode standalone --command "./cub-scout map list"
USAGE
}

if [[ "${1:-}" == "--help" || "$#" -eq 0 ]]; then
  usage
  exit 0
fi

mode="auto"
command_text=""
confirm="no"
execute="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      mode="${2:-}"
      shift 2
      ;;
    --command)
      command_text="${2:-}"
      shift 2
      ;;
    --confirm)
      confirm="${2:-}"
      shift 2
      ;;
    --execute)
      execute="true"
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$command_text" ]]; then
  echo "Missing required --command" >&2
  usage
  exit 1
fi

if [[ "$confirm" != "yes" && "$confirm" != "no" ]]; then
  echo "Invalid --confirm value: $confirm (use yes|no)" >&2
  exit 1
fi

if [[ "$mode" == "auto" ]]; then
  if command -v cub >/dev/null 2>&1 && cub context get >/dev/null 2>&1; then
    mode="connected"
  elif command -v kubectl >/dev/null 2>&1; then
    mode="standalone"
  else
    mode="unknown"
  fi
fi

if [[ "$mode" != "standalone" && "$mode" != "connected" && "$mode" != "unknown" ]]; then
  echo "Invalid --mode value: $mode (use standalone|connected|auto)" >&2
  exit 1
fi

cmd_lower="$(printf '%s' "$command_text" | tr '[:upper:]' '[:lower:]')"

risk="low"
requires_confirm="false"
dry_run_command="n/a"
verify_status="ok"
verdict="READY"
allowed_to_execute="true"
next_action="safe to run"

mutating="false"
if [[ "$cmd_lower" == *"kubectl apply"* || "$cmd_lower" == *"kubectl delete"* || "$cmd_lower" == *"kubectl patch"* || "$cmd_lower" == *"kubectl create"* ]]; then
  mutating="true"
fi
if [[ "$cmd_lower" == *"cub-scout import "* || "$cmd_lower" == *"cub-scout import-argocd"* || "$cmd_lower" == *"cub-scout apply"* || "$cmd_lower" == *"cub-scout remedy"* || "$cmd_lower" == *"cub-scout connect"* ]]; then
  mutating="true"
fi
if [[ "$cmd_lower" == *"--force"* || "$cmd_lower" == *" --yes"* ]]; then
  mutating="true"
fi

if [[ "$mutating" == "true" && "$cmd_lower" != *"--dry-run"* ]]; then
  risk="high"
  requires_confirm="true"
  dry_run_command="$command_text --dry-run"
  if [[ "$confirm" != "yes" ]]; then
    verdict="BLOCKED_CONFIRMATION"
    allowed_to_execute="false"
    next_action="run dry-run; then rerun with --confirm yes"
  else
    verdict="READY"
    allowed_to_execute="true"
    next_action="confirmation satisfied; safe to execute"
  fi
elif [[ "$mutating" == "true" && "$cmd_lower" == *"--dry-run"* ]]; then
  risk="medium"
  requires_confirm="false"
  dry_run_command="already in dry-run"
  verdict="READY"
  allowed_to_execute="true"
  next_action="review dry-run output"
fi

cat <<EOF
Verdict: $verdict
Mode: $mode
Risk: $risk
Verify: $verify_status
Command: $command_text
DryRunCommand: $dry_run_command
RequiresConfirm: $requires_confirm
AllowedToExecute: $allowed_to_execute
NextAction: $next_action
EOF

if [[ "$execute" == "true" ]]; then
  if [[ "$allowed_to_execute" != "true" ]]; then
    echo "Execution blocked: explicit confirmation required" >&2
    exit 2
  fi
  bash -lc "$command_text"
fi
