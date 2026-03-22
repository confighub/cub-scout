#!/usr/bin/env bash
# demo-worker-lifecycle.sh - Manage discovery worker process lifecycle for demos.
#
# Commands:
#   start   create worker if needed and launch `cub worker run` as detached process
#   stop    stop detached worker process tracked by pid file
#   cleanup stop unless --keep is provided

set -euo pipefail

MODE="${1:-}"
if [[ $# -gt 0 ]]; then
    shift
fi

SPACE=""
WORKER=""
TARGET="Kubernetes"
PID_FILE=""
LOG_FILE=""
KEEP=false

usage() {
    cat <<USAGE
Usage:
  $(basename "$0") start --space <slug> --worker <slug> --pid-file <path> --log-file <path> [--target <type>]
  $(basename "$0") stop --pid-file <path>
  $(basename "$0") cleanup --pid-file <path> [--keep]

Commands:
  start    Launch detached worker process and write PID to file
  stop     Stop worker process referenced by PID file
  cleanup  Stop worker unless --keep is set
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --space)
            [[ $# -ge 2 ]] || { echo "missing value for --space" >&2; usage; exit 2; }
            SPACE="$2"
            shift 2
            ;;
        --space=*)
            SPACE="${1#*=}"
            shift
            ;;
        --worker)
            [[ $# -ge 2 ]] || { echo "missing value for --worker" >&2; usage; exit 2; }
            WORKER="$2"
            shift 2
            ;;
        --worker=*)
            WORKER="${1#*=}"
            shift
            ;;
        --target)
            [[ $# -ge 2 ]] || { echo "missing value for --target" >&2; usage; exit 2; }
            TARGET="$2"
            shift 2
            ;;
        --target=*)
            TARGET="${1#*=}"
            shift
            ;;
        --pid-file)
            [[ $# -ge 2 ]] || { echo "missing value for --pid-file" >&2; usage; exit 2; }
            PID_FILE="$2"
            shift 2
            ;;
        --pid-file=*)
            PID_FILE="${1#*=}"
            shift
            ;;
        --log-file)
            [[ $# -ge 2 ]] || { echo "missing value for --log-file" >&2; usage; exit 2; }
            LOG_FILE="$2"
            shift 2
            ;;
        --log-file=*)
            LOG_FILE="${1#*=}"
            shift
            ;;
        --keep)
            KEEP=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            exit 2
            ;;
    esac
done

require_pid_file() {
    if [[ -z "$PID_FILE" ]]; then
        echo "--pid-file is required" >&2
        exit 2
    fi
}

pid_from_file() {
    local pid_raw=""
    if [[ ! -f "$PID_FILE" ]]; then
        echo ""
        return
    fi
    pid_raw="$(tr -d '[:space:]' <"$PID_FILE" 2>/dev/null || true)"
    if [[ "$pid_raw" =~ ^[0-9]+$ ]]; then
        echo "$pid_raw"
        return
    fi
    echo ""
}

find_running_worker_pid() {
    if ! command -v pgrep >/dev/null 2>&1; then
        echo ""
        return
    fi

    local pattern="cub worker run --space ${SPACE} -t ${TARGET} ${WORKER}"
    local pid=""
    pid="$(pgrep -f "$pattern" 2>/dev/null | tail -n 1 | tr -d '[:space:]')"
    if [[ "$pid" =~ ^[0-9]+$ ]]; then
        echo "$pid"
        return
    fi
    echo ""
}

start_worker_with_pty() {
    if ! command -v script >/dev/null 2>&1; then
        return 1
    fi

    # util-linux `script` uses: script -q -c "<cmd>" /dev/null
    if script -q -c true /dev/null >/dev/null 2>&1; then
        local quoted_cmd=""
        quoted_cmd="$(printf '%q ' cub worker run --space "$SPACE" -t "$TARGET" "$WORKER")"
        nohup script -q -c "$quoted_cmd" /dev/null >>"$LOG_FILE" 2>&1 < /dev/null &
        echo "$!"
        return 0
    fi

    # BSD/macOS `script` uses: script -q /dev/null <cmd> [args...]
    if script -q /dev/null true >/dev/null 2>&1; then
        nohup script -q /dev/null cub worker run --space "$SPACE" -t "$TARGET" "$WORKER" >>"$LOG_FILE" 2>&1 < /dev/null &
        echo "$!"
        return 0
    fi

    return 1
}

stop_worker() {
    require_pid_file
    local pid=""
    pid="$(pid_from_file)"
    if [[ -z "$pid" ]]; then
        rm -f "$PID_FILE"
        echo "no running worker pid tracked"
        return
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
        rm -f "$PID_FILE"
        echo "worker already stopped pid=$pid"
        return
    fi

    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 20); do
        if ! kill -0 "$pid" 2>/dev/null; then
            break
        fi
        sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
    echo "stopped worker pid=$pid"
}

start_worker() {
    if [[ -z "$SPACE" || -z "$WORKER" || -z "$PID_FILE" || -z "$LOG_FILE" ]]; then
        echo "start requires --space, --worker, --pid-file, and --log-file" >&2
        exit 2
    fi
    if ! command -v cub >/dev/null 2>&1; then
        echo "cub CLI not found in PATH" >&2
        exit 1
    fi

    mkdir -p "$(dirname "$PID_FILE")"
    mkdir -p "$(dirname "$LOG_FILE")"

    local existing_pid=""
    local launcher_pid=""
    local worker_pid=""
    local start_mode="nohup"
    existing_pid="$(pid_from_file)"
    if [[ -n "$existing_pid" ]] && kill -0 "$existing_pid" 2>/dev/null; then
        echo "worker already running pid=$existing_pid"
        return
    fi
    rm -f "$PID_FILE"

    cub worker create --space "$SPACE" "$WORKER" >/dev/null 2>&1 || true
    if launcher_pid="$(start_worker_with_pty)"; then
        start_mode="pty"
    else
        nohup cub worker run --space "$SPACE" -t "$TARGET" "$WORKER" >>"$LOG_FILE" 2>&1 < /dev/null &
        launcher_pid="$!"
    fi

    if [[ "$start_mode" == "pty" ]]; then
        for _ in $(seq 1 50); do
            worker_pid="$(find_running_worker_pid)"
            if [[ -n "$worker_pid" ]] && kill -0 "$worker_pid" 2>/dev/null; then
                break
            fi
            sleep 0.1
        done
    fi
    if [[ -z "$worker_pid" ]]; then
        worker_pid="$launcher_pid"
    fi

    echo "$worker_pid" >"$PID_FILE"

    sleep 0.2
    if ! kill -0 "$worker_pid" 2>/dev/null; then
        rm -f "$PID_FILE"
        echo "worker process exited early (see log: $LOG_FILE)" >&2
        exit 1
    fi

    echo "started worker pid=$worker_pid space=$SPACE worker=$WORKER target=$TARGET mode=$start_mode log=$LOG_FILE"
}

case "$MODE" in
    start)
        start_worker
        ;;
    stop)
        stop_worker
        ;;
    cleanup)
        require_pid_file
        if [[ "$KEEP" == "true" ]]; then
            echo "keeping worker process running (pid file: $PID_FILE)"
        else
            stop_worker
        fi
        ;;
    ""|-h|--help)
        usage
        exit 2
        ;;
    *)
        echo "unknown command: $MODE" >&2
        usage
        exit 2
        ;;
esac
