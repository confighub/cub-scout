#!/usr/bin/env bash
# Run one cub-scout command and print every cub call it makes.
#
# Read-only: the wrapper records each argument vector (never output, never
# credentials) and passes the call through to the real cub unchanged.
#
# usage: record-cub-calls.sh <cub-scout> [args...]
#   CUB_SPACE=platform ./record-cub-calls.sh ./cub-scout history deploy/api -n prod
set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "usage: $0 <cub-scout binary> [args...]" >&2
    exit 2
fi

real_cub=$(command -v cub) || { echo "cub is not on PATH" >&2; exit 2; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cat > "$work/cub" <<WRAP
#!/usr/bin/env bash
printf '%s\n' "cub \$*" >> "$work/calls.log"
exec "$real_cub" "\$@"
WRAP
chmod +x "$work/cub"

status=0
PATH="$work:$PATH" "$@" || status=$?

echo
echo "--- cub calls (CUB_SPACE=${CUB_SPACE:-<unset>})"
if [[ -s "$work/calls.log" ]]; then
    cat "$work/calls.log"
else
    echo "(none)"
fi
exit "$status"
