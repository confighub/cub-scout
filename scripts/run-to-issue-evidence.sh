#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/run-to-issue-evidence.sh \
    --title "<issue-title>" \
    --goal "<user-goal>" \
    --expected "<expected-behavior>" \
    --impact "<demo-or-user-impact>" \
    --transcript <path> \
    [--observed "<observed-behavior>"] \
    [--attempted "<commands-attempted>"] \
    [--output <issue-body.md>] \
    [--repo <owner/repo>] \
    [--open]

Examples:
  ./scripts/run-to-issue-evidence.sh \
    --title "Capability gap: import follow-up" \
    --goal "Import and then generate patch plan" \
    --expected "Offer deterministic patch-plan command" \
    --impact "Blocks AI-assisted remediation flow" \
    --transcript examples/ai-integration/testdata/failed-session.transcript.txt

  ./scripts/run-to-issue-evidence.sh \
    --title "Capability gap: import follow-up" \
    --goal "Import and then generate patch plan" \
    --expected "Offer deterministic patch-plan command" \
    --impact "Blocks AI-assisted remediation flow" \
    --transcript examples/ai-integration/testdata/failed-session.transcript.txt \
    --open
USAGE
}

sanitize_stream() {
  sed -E \
    -e "s|${HOME}|<HOME>|g" \
    -e 's|/private/tmp/[^[:space:]]+|<TEMP_PATH>|g' \
    -e 's|/tmp/[^[:space:]]+|<TEMP_PATH>|g' \
    -e 's|/var/folders/[^[:space:]]+|<TEMP_PATH>|g' \
    -e 's|github_pat_[A-Za-z0-9_]+|<REDACTED_TOKEN>|g' \
    -e 's|ghp_[A-Za-z0-9]+|<REDACTED_TOKEN>|g' \
    -e 's|Bearer[[:space:]]+[A-Za-z0-9._-]+|Bearer <REDACTED_TOKEN>|g' \
    -e 's|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.+-]+Z|<TIMESTAMP>|g'
}

sanitize_text() {
  printf '%s\n' "$1" | sanitize_stream
}

safe_cmd_output() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "unavailable"
    return
  fi
  "$@" 2>/dev/null || echo "unknown"
}

if [[ "${1:-}" == "--help" || "$#" -eq 0 ]]; then
  usage
  exit 0
fi

title=""
goal=""
expected=""
impact=""
transcript=""
observed_override=""
attempted_override=""
output=""
repo="confighub/cub-scout"
open_issue="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --title)
      title="${2:-}"
      shift 2
      ;;
    --goal)
      goal="${2:-}"
      shift 2
      ;;
    --expected)
      expected="${2:-}"
      shift 2
      ;;
    --impact)
      impact="${2:-}"
      shift 2
      ;;
    --transcript)
      transcript="${2:-}"
      shift 2
      ;;
    --observed)
      observed_override="${2:-}"
      shift 2
      ;;
    --attempted)
      attempted_override="${2:-}"
      shift 2
      ;;
    --output)
      output="${2:-}"
      shift 2
      ;;
    --repo)
      repo="${2:-}"
      shift 2
      ;;
    --open)
      open_issue="true"
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

if [[ -z "$title" || -z "$goal" || -z "$expected" || -z "$impact" || -z "$transcript" ]]; then
  echo "Missing required flags." >&2
  usage
  exit 1
fi

if [[ ! -f "$transcript" ]]; then
  echo "Transcript not found: $transcript" >&2
  exit 1
fi

if [[ -z "$output" ]]; then
  output="$(mktemp "${TMPDIR:-/tmp}/run-to-issue-XXXXXX.md")"
fi
mkdir -p "$(dirname "$output")"

max_commands=20
max_observed=20
max_evidence=80

if [[ -n "$attempted_override" ]]; then
  attempted="$attempted_override"
else
  attempted="$(grep -E '^\$ ' "$transcript" | sed 's/^\$ //' | head -n "$max_commands" || true)"
fi
if [[ -z "${attempted// }" ]]; then
  attempted="(no shell commands detected in transcript)"
fi

if [[ -n "$observed_override" ]]; then
  observed="$observed_override"
else
  observed="$(grep -Ei 'error|fail|panic|denied|not found|unsupported|timed out|timeout' "$transcript" | head -n "$max_observed" || true)"
fi
if [[ -z "${observed// }" ]]; then
  observed="$(tail -n "$max_observed" "$transcript")"
fi

evidence_excerpt="$(tail -n "$max_evidence" "$transcript")"

generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
git_commit="unknown"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git_commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
fi
kubectl_context="$(safe_cmd_output kubectl kubectl config current-context)"
cub_scout_version="unknown"
if [[ -x "./cub-scout" ]]; then
  cub_scout_version="$(./cub-scout version 2>/dev/null || echo unknown)"
elif command -v cub-scout >/dev/null 2>&1; then
  cub_scout_version="$(cub-scout version 2>/dev/null || echo unknown)"
fi

attempted_clean="$(sanitize_text "$attempted")"
observed_clean="$(sanitize_text "$observed")"
expected_clean="$(sanitize_text "$expected")"
goal_clean="$(sanitize_text "$goal")"
impact_clean="$(sanitize_text "$impact")"
transcript_clean="$(sanitize_text "$transcript")"
generated_clean="$(sanitize_text "$generated_at")"
git_commit_clean="$(sanitize_text "$git_commit")"
kubectl_context_clean="$(sanitize_text "$kubectl_context")"
cub_scout_version_clean="$(sanitize_text "$cub_scout_version")"
os_clean="$(sanitize_text "$(uname -s)")"
evidence_clean="$(sanitize_text "$evidence_excerpt")"

cat > "$output" <<ISSUE
## User Goal
$goal_clean

## Commands Attempted
\`\`\`bash
$attempted_clean
\`\`\`

## Observed Behavior
$observed_clean

## Expected Behavior
$expected_clean

## Demo/User Impact
$impact_clean

## Evidence
- Transcript: $transcript_clean
- Generated at (UTC): $generated_clean
- Git commit: $git_commit_clean
- kubectl context: $kubectl_context_clean
- cub-scout version: $cub_scout_version_clean
- Host OS: $os_clean

\`\`\`text
$evidence_clean
\`\`\`
ISSUE

printf 'Issue draft written: %s\n' "$output"

if [[ "$open_issue" == "true" ]]; then
  if ! command -v gh >/dev/null 2>&1; then
    echo "Cannot open issue: gh CLI not found. Draft is available at $output" >&2
    exit 1
  fi
  gh issue create -R "$repo" --title "$title" --template ai-capability-gap.yml --body-file "$output"
fi
