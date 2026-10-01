#!/usr/bin/env python3
"""Run six pinned kubectl raw reads against two owned, local recorded phases."""
from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import os
import selectors
import signal
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

sys.dont_write_bytecode = True
TOOLS = Path("/tools")
REPLAY_PATH = TOOLS / "evals/recorded-api/replay.py"
PHASE_PATHS = {
    "absent": [
        "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/servicemonitors.monitoring.coreos.com",
        "/apis/monitoring.coreos.com/v1",
        "/apis/monitoring.coreos.com/v1/namespaces/monitoring/servicemonitors/kube-prometheus-stack-kube-state-metrics",
    ],
    "present": [
        "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/servicemonitors.monitoring.coreos.com",
        "/apis/monitoring.coreos.com/v1",
        "/apis/monitoring.coreos.com/v1/namespaces/monitoring/servicemonitors/kube-prometheus-stack-kube-state-metrics",
    ],
}
EXPECTED_STATUS = {"absent": [404, 404, 404], "present": [200, 200, 200]}
COMMAND_TIMEOUT = 5.0
COMMAND_OUTPUT_CAP = 64 * 1024
PHASE_WALL_SECONDS = 24
PHASE_MAX_REQUESTS = 12
TOTAL_SECONDS = 60


class GateError(RuntimeError):
    pass


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def load_replay():
    if REPLAY_PATH.is_symlink() or not REPLAY_PATH.is_file():
        raise GateError("staged replay source is missing or unsafe")
    spec = importlib.util.spec_from_file_location("pre01_recorded_replay", REPLAY_PATH)
    if spec is None or spec.loader is None:
        raise GateError("staged replay source cannot be loaded")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def run_kubectl(argv: list[str], deadline: float, kubeconfig_path: str | None = None, *, cwd: str = "/tools") -> dict:
    started = utc_now()
    began = time.monotonic()
    remaining = min(COMMAND_TIMEOUT, deadline - began)
    raw = {"stdout": bytearray(), "stderr": bytearray()}
    sizes = {key: 0 for key in raw}
    digests = {key: hashlib.sha256() for key in raw}
    status = "passed"
    error = None
    proc = None
    selector = None
    env = {"PATH": "/usr/bin:/bin", "HOME": "/tmp/private-home", "TMPDIR": "/tmp",
           "LANG": "C.UTF-8", "KUBECONFIG": kubeconfig_path or argv[2]}
    try:
        if remaining <= 0:
            raise GateError("overall execution deadline expired")
        proc = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                env=env, start_new_session=True, cwd=cwd)
        assert proc.stdout is not None and proc.stderr is not None
        selector = selectors.DefaultSelector()
        selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
        selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
        command_deadline = min(deadline, time.monotonic() + remaining)
        while selector.get_map() or proc.poll() is None:
            left = command_deadline - time.monotonic()
            if left <= 0:
                status, error = "failed", "timeout"
                break
            for key, _ in selector.select(min(left, 0.1)):
                chunk = os.read(key.fileobj.fileno(), 8192)
                if not chunk:
                    selector.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                name = key.data
                sizes[name] += len(chunk)
                digests[name].update(chunk)
                if len(raw[name]) < COMMAND_OUTPUT_CAP:
                    raw[name].extend(chunk[:COMMAND_OUTPUT_CAP - len(raw[name])])
                if sizes[name] > COMMAND_OUTPUT_CAP:
                    status, error = "failed", "output_limit"
                    break
            if error:
                break
        if error:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.wait(timeout=1)
        else:
            proc.wait(timeout=max(0.1, command_deadline - time.monotonic()))
    except BaseException as exc:
        status = "failed"
        error = error or (str(exc)[:160] if isinstance(exc, GateError) else type(exc).__name__)
        if proc is not None and proc.poll() is None:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                proc.wait(timeout=1)
            except subprocess.TimeoutExpired:
                error = "child_teardown_unverified"
    finally:
        if selector is not None:
            selector.close()
        if proc is not None:
            for stream in (proc.stdout, proc.stderr):
                if stream and not stream.closed:
                    stream.close()
    code = proc.returncode if proc is not None and error is None else None
    return {"argv": argv, "startedAt": started, "endedAt": utc_now(),
            "elapsedSeconds": round(time.monotonic() - began, 6), "exitCode": code,
            "status": status, "error": error,
            **{f"{name}Bytes": sizes[name] for name in raw},
            **{f"{name}Sha256": digests[name].hexdigest() for name in raw},
            **{f"{name}Base64": base64.b64encode(bytes(raw[name])).decode("ascii") for name in raw},
            "outputTruncated": any(sizes[name] > COMMAND_OUTPUT_CAP for name in raw)}


def write_private(path: Path, content: bytes) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(content)
        stream.flush()


def _json_bytes(value) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode("utf-8")


def _kubeconfig(port: int) -> bytes:
    value = {"apiVersion": "v1", "kind": "Config", "clusters": [{"name": "recorded", "cluster": {"server": f"http://127.0.0.1:{port}"}}],
             "contexts": [{"name": "recorded", "context": {"cluster": "recorded", "user": "anonymous"}}],
             "current-context": "recorded", "users": [{"name": "anonymous", "user": {}}]}
    return _json_bytes(value)


def _run_phase(replay, phase: str, deadline: float, *, runtime_dir: Path = Path("/tmp")) -> dict:
    routes, ready = replay.load_routes(phase)
    expected_paths = PHASE_PATHS[phase]
    if [route["path"] for route in routes] != expected_paths:
        raise GateError(f"{phase} replay routes differ from the six-command contract")
    if [route["status"] for route in routes] != EXPECTED_STATUS[phase]:
        raise GateError(f"{phase} replay statuses differ from the pinned source observations")
    phase_started = utc_now()
    phase_began = time.monotonic()
    server = replay.ReplayServer(routes, host="127.0.0.1", port=0,
                                 max_requests=PHASE_MAX_REQUESTS, read_timeout=2.0)
    server_status = {"value": None, "error": None}

    def serve():
        try:
            server_status["value"] = server.serve_bounded(PHASE_WALL_SECONDS)
        except BaseException as exc:
            server_status["error"] = type(exc).__name__

    thread = threading.Thread(target=serve, name="owned-recorded-api-replay", daemon=False)
    commands = []
    phase_error = None
    started = False
    try:
        thread.start()
        started = True
        port = server.server_address[1]
        cfg = runtime_dir / ("kubeconfig-" + phase)
        write_private(cfg, _kubeconfig(port))
        kubeconfig_hash = sha(cfg.read_bytes())
        for index, path in enumerate(expected_paths):
            if time.monotonic() >= deadline:
                raise GateError("overall execution deadline expired")
            argv = ["/tools/kubectl", "--kubeconfig", str(cfg), "--context", "recorded", "get", "--raw", path]
            result = run_kubectl(argv, deadline, str(cfg))
            result.update({"phase": phase, "index": index, "path": path,
                           "expectedHTTPStatus": EXPECTED_STATUS[phase][index],
                           "expectedSourceBodySha256": routes[index]["bodySha256"],
                           "expectedSourceBodyBytes": len(routes[index]["body"])})
            commands.append(result)
            expected_code = EXPECTED_STATUS[phase][index]
            if result["status"] != "passed" or result["outputTruncated"]:
                break
            if (expected_code == 404 and (type(result["exitCode"]) is not int or result["exitCode"] != 1
                                          or result["stderrBytes"] == 0)):
                break
            if expected_code == 200:
                try:
                    stdout = base64.b64decode(result["stdoutBase64"], validate=True)
                except (ValueError, TypeError):
                    break
                if result["exitCode"] != 0 or stdout != routes[index]["body"]:
                    break
    except BaseException as exc:
        phase_error = str(exc)[:160] if isinstance(exc, GateError) else type(exc).__name__
    finally:
        server.stop_event.set()
        if started:
            thread.join(timeout=2.5)
        else:
            server.server_close()
        if started and thread.is_alive():
            server.server_close()
            thread.join(timeout=1.0)
    if started and thread.is_alive():
        phase_error = phase_error or "owned replay server thread did not stop"
    trace = replay._trace(ready, server, server_status["value"] or "thread_failed", phase_started, phase_began,
                          server_status["error"])
    trace.update({"kubectlPhaseCommands": len(commands), "kubeconfigSha256": kubeconfig_hash if 'kubeconfig_hash' in locals() else None})
    passed = _check_phase_result(phase, routes, commands, trace) and phase_error is None
    return {"phase": phase, "status": "passed" if passed else "failed",
            **({} if passed else {"error": phase_error or "phase acceptance checks failed"}),
            "startedAt": phase_started, "endedAt": utc_now(), "elapsedSeconds": round(time.monotonic() - phase_began, 6),
            "sourceRows": ready["routes"], "transportReady": {"host": "127.0.0.1", "port": server.server_address[1],
                            "sourceRevision": ready["sourceRevision"], "reportSha256": ready["reportSha256"],
                            "scopeSha256": ready["scopeSha256"], "atomicSnapshot": False},
            "commands": commands, "replayTrace": trace,
            "listenerCleanupConfirmed": server.fileno() == -1 and not thread.is_alive()}


def _check_phase_result(phase: str, routes: list[dict], commands: list[dict], trace: dict) -> bool:
    if len(commands) != 3 or trace.get("phase") != phase or trace.get("status") != "stopped":
        return False
    if (trace.get("coverageFailure") is not False or trace.get("listenerCleanupConfirmed") is not True
            or trace.get("requestsObserved") != 3 or not isinstance(trace.get("events"), list)
            or len(trace["events"]) != 3):
        return False
    for i, (cmd, route, event) in enumerate(zip(commands, routes, trace["events"])):
        status = EXPECTED_STATUS[phase][i]
        if (cmd.get("phase") != phase or cmd.get("index") != i or cmd.get("path") != route["path"]
                or cmd.get("expectedHTTPStatus") != status or event.get("method") != "GET"
                or event.get("path") != route["path"] or event.get("status") != status
                or event.get("responseBodySha256") != route["bodySha256"]
                or event.get("responseBodyBytes") != len(route["body"]) or event.get("captured") is not True
                or event.get("reason") != "captured-response"
                or event.get("sourceRows") != route["sourceRows"]):
            return False
        try:
            stdout = base64.b64decode(cmd.get("stdoutBase64", ""), validate=True)
            stderr = base64.b64decode(cmd.get("stderrBase64", ""), validate=True)
        except (ValueError, TypeError):
            return False
        stdout_size, stderr_size = cmd.get("stdoutBytes"), cmd.get("stderrBytes")
        if (type(stdout_size) is not int or type(stderr_size) is not int
                or len(stdout) > COMMAND_OUTPUT_CAP or len(stderr) > COMMAND_OUTPUT_CAP
                or sha(stdout) != cmd.get("stdoutSha256") or len(stdout) != stdout_size
                or sha(stderr) != cmd.get("stderrSha256") or len(stderr) != stderr_size
                or cmd.get("status") != "passed" or cmd.get("outputTruncated") is not False):
            return False
        code = cmd.get("exitCode")
        if type(code) is not int:
            return False
        if status == 404 and (code != 1 or not stderr):
            return False
        if status == 200 and (code != 0 or stdout != route["body"]):
            return False
    return True


def main() -> int:
    phases = []
    try:
        replay = load_replay()
        started = time.monotonic()
        deadline = started + TOTAL_SECONDS
        for phase_name in ("absent", "present"):
            try:
                phase = _run_phase(replay, phase_name, deadline)
            except BaseException as exc:
                phase = {"phase": phase_name, "status": "failed",
                         "error": str(exc)[:160] if isinstance(exc, GateError) else type(exc).__name__,
                         "commands": [], "replayTrace": None, "listenerCleanupConfirmed": False}
            phases.append(phase)
            if phase["status"] != "passed":
                break
        passed = len(phases) == 2 and all(phase["status"] == "passed" and phase["listenerCleanupConfirmed"] for phase in phases)
        result = {"schema": "pre01-real-kubectl-gate.v1", "status": "passed" if passed else "failed",
                  **({} if passed else {"error": "one or more bounded phases failed or were incomplete"}),
                  "source": {"phaseOrder": ["absent", "present"], "atomicSnapshot": False,
                             "kubectlSha256": _file_sha(TOOLS / "kubectl"),
                             "replaySha256": _file_sha(REPLAY_PATH),
                             "reportSha256": _file_sha(TOOLS / "evals/reports/2026-10-01-pre01-crd.json"),
                             "scopeSha256": _file_sha(TOOLS / "evals/pre01-crd/fixtures/capture-scope.json")},
                  "limits": {"perCommandSeconds": COMMAND_TIMEOUT, "perStreamBytes": COMMAND_OUTPUT_CAP,
                             "phaseWallSeconds": PHASE_WALL_SECONDS, "totalExecutionSeconds": TOTAL_SECONDS},
                  "elapsedSeconds": round(time.monotonic() - started, 6), "phases": phases}
    except BaseException as exc:
        result = {"schema": "pre01-real-kubectl-gate.v1", "status": "failed",
                  "error": str(exc)[:200] if isinstance(exc, GateError) else type(exc).__name__,
                  "phases": phases}
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0 if result.get("status") == "passed" else 1


def _file_sha(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


if __name__ == "__main__":
    raise SystemExit(main())
