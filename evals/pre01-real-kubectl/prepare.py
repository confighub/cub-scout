#!/usr/bin/env python3
"""Run six pinned real-kubectl GET --raw calls in one owned network-none container."""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import signal
import sys
import time
import uuid

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
VERSION_HELPER_PATH = REPO / "evals/linux-runtime-versions/prepare.py"
spec = importlib.util.spec_from_file_location("pre01_kubectl_version_helper", VERSION_HELPER_PATH)
if spec is None or spec.loader is None:
    raise RuntimeError("reviewed Linux runtime helper is unavailable")
version_helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(version_helper)

IMAGE_ID = version_helper.IMAGE_ID
OWNER_LABEL = version_helper.OWNER_LABEL
CAP = 4 * 1024 * 1024
EXECUTION_SECONDS, CLEANUP_SECONDS, TOTAL_SECONDS = 60, 30, 90
PER_COMMAND_SECONDS, PER_STREAM_BYTES, PHASE_WALL_SECONDS, PHASE_MAX_REQUESTS = 5, 64 * 1024, 24, 12
ASSET = {"bytes": 55_640_226, "sha256": "9f9d9c44a7b5264515ac9da5991584e2395bd50662e651132337e7b4d0c56f8f"}
SOURCE_PINS = {
    "evals/recorded-api/replay.py": (29_895, "5b147c7bfe409a93d9e20e6d5994e653e3c524e082722068471bd6e7aa7d8d93"),
    "evals/reports/2026-10-01-pre01-crd.json": (15_604, "5585ecd0298e2b6f4201a638cdc0aeab9cdeb0dfab38d3b52e090cc3cb8f5c0a"),
    "evals/pre01-crd/fixtures/capture-scope.json": (10_684, "6d39e83612d52f5124bad2dd63b27b30b94cce492eb70223f759248de71bfec4"),
    "evals/pre01-crd/fixtures/absent-crd.json": (336, "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0"),
    "evals/pre01-crd/fixtures/absent-discovery.body": (19, "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793"),
    "evals/pre01-crd/fixtures/absent-servicemonitor.body": (19, "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793"),
    "evals/pre01-crd/fixtures/absent-after-apply-crd.json": (336, "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0"),
    "evals/pre01-crd/fixtures/absent-after-apply-servicemonitor.body": (19, "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793"),
    "evals/pre01-crd/fixtures/present-crd.json": (44_289, "a5e571a80d5882cc9294cfabc27e0e6cefead3612e8e880afb885e3bf60f3e3c"),
    "evals/pre01-crd/fixtures/present-discovery.json": (509, "e791bb13f75f7767dd2c40df7485653342b0894ca58fd3af17b1b03ed087c1a8"),
    "evals/pre01-crd/fixtures/present-servicemonitor.json": (2_355, "78836df757ca3eb79f25bb0190f03d465fb282b455b8a27e79fbef67247a650e"),
    "evals/pre01-real-kubectl/payload.py": (0, ""),
}


class GateError(RuntimeError):
    pass


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _strict_json(raw: bytes, label: str):
    def pairs(items):
        out = {}
        for key, value in items:
            if key in out:
                raise GateError(label + " contains duplicate JSON keys")
            out[key] = value
        return out
    try:
        return json.loads(raw.decode("utf-8", "strict"), object_pairs_hook=pairs,
                          parse_constant=lambda _: (_ for _ in ()).throw(ValueError("non-finite")))
    except (ValueError, UnicodeDecodeError):
        raise GateError(label + " is malformed JSON") from None


def _read_source(path: Path, rel: str) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise GateError(f"pinned source {rel} is missing or unsafe")
    data = path.read_bytes()
    expected_size, expected_hash = SOURCE_PINS[rel]
    if expected_size and len(data) != expected_size:
        raise GateError(f"pinned source {rel} size changed")
    if expected_hash and sha(data) != expected_hash:
        raise GateError(f"pinned source {rel} hash changed")
    return data


def source_inventory() -> dict[str, bytes]:
    files = {rel: _read_source(REPO / rel, rel) for rel in SOURCE_PINS if not rel.endswith("payload.py")}
    payload_path = HERE / "payload.py"
    if payload_path.is_symlink() or not payload_path.is_file():
        raise GateError("authored payload is missing or unsafe")
    files["evals/pre01-real-kubectl/payload.py"] = payload_path.read_bytes()
    return files


def stage_files(root: Path, sources: dict[str, bytes], kubectl: Path) -> dict[str, str]:
    asset = version_helper.validate_asset(kubectl, "kubectl")
    if asset != {"bytes": ASSET["bytes"], "sha256": ASSET["sha256"], "elf": "ELF64 little-endian AArch64"}:
        raise GateError("kubectl asset differs from reviewed Linux arm64 pin")
    entries = {"kubectl": kubectl.read_bytes(), **sources}
    for rel, data in entries.items():
        dst = root / rel
        dst.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        fd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o555 if rel == "kubectl" else 0o444)
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
        os.chmod(dst, 0o555 if rel == "kubectl" else 0o444)
    for directory, _, _ in os.walk(root, topdown=False):
        os.chmod(directory, 0o555)
    return {rel: sha(data) for rel, data in entries.items()}


def _walk_stage(stage: Path) -> dict[str, str]:
    if stage.is_symlink() or not stage.is_dir():
        raise GateError("staged root is unsafe")
    result = {}
    for base, dirs, files in os.walk(stage, followlinks=False):
        for name in dirs:
            if (Path(base) / name).is_symlink():
                raise GateError("staging directory was replaced with symlink")
        for name in files:
            path = Path(base) / name
            if path.is_symlink() or not path.is_file():
                raise GateError("staging file is unsafe")
            result[path.relative_to(stage).as_posix()] = sha_file(path)
    return result


def require_stage_unchanged(stage: Path, expected_hashes: dict[str, str]) -> dict[str, str]:
    actual = _walk_stage(stage)
    if actual != expected_hashes:
        raise GateError("staged inputs changed or the staged file set differs")
    return actual


def remove_stage(stage: Path, expected_paths: set[str]) -> None:
    actual = set(_walk_stage(stage))
    if actual != expected_paths:
        raise GateError("staged file set changed; refusing recursive cleanup")
    for base, dirs, files in os.walk(stage, topdown=False):
        os.chmod(base, 0o700)
        for name in files:
            (Path(base) / name).unlink()
        for name in dirs:
            (Path(base) / name).rmdir()
    stage.rmdir()


def build_container_create_args(name: str, owner: str, stage: Path) -> list[str]:
    if not re.fullmatch(r"scout-pre01-kubectl-[0-9a-f]{16}", name) or not re.fullmatch(r"[0-9a-f]{32}", owner):
        raise GateError("owned container identity is malformed")
    if stage.is_symlink() or not stage.is_dir():
        raise GateError("staging directory is unsafe")
    return ["container", "create", "--name", name, "--label", OWNER_LABEL + "=" + owner,
            "--pull=never", "--network=none", "--read-only", "--user", "65534:65534", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=1g", "--cpus=1", "--tmpfs",
            "/tmp:rw,nosuid,nodev,size=64m,uid=65534,gid=65534", "--mount",
            "type=bind,src=" + str(stage.resolve()) + ",dst=/tools,readonly", "--entrypoint=python3", IMAGE_ID,
            "/tools/evals/pre01-real-kubectl/payload.py"]


def validate_container_inspect(raw: bytes, name: str, owner: str, stage: Path, *, finished=False) -> dict:
    obj = version_helper._one(raw)
    _, summary = version_helper.validate_inspect(raw, name, owner, stage, finished=finished)
    config = obj.get("Config")
    if (not isinstance(config, dict) or config.get("Entrypoint") != ["python3"]
            or config.get("Cmd") != ["/tools/evals/pre01-real-kubectl/payload.py"]):
        raise GateError("container command differs from the fixed authored payload")
    return summary


def _expected_routes():
    replay_path = REPO / "evals/recorded-api/replay.py"
    spec = importlib.util.spec_from_file_location("pre01_recorded_route_expected", replay_path)
    if spec is None or spec.loader is None:
        raise GateError("pinned replay source cannot be loaded for acceptance validation")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return {phase: module.load_routes(phase)[0] for phase in ("absent", "present")}


def _command_receipt(argv, code, stdout, stderr, elapsed):
    def facts(data):
        if not isinstance(data, bytes):
            return None, None
        return len(data), sha(data)
    stdout_bytes, stdout_sha = facts(stdout)
    stderr_bytes, stderr_sha = facts(stderr)
    return {"argv": argv, "exitCode": code, "elapsedSeconds": round(elapsed, 6),
            "stdoutBytes": stdout_bytes, "stderrBytes": stderr_bytes,
            "stdoutSha256": stdout_sha, "stderrSha256": stderr_sha,
            "returnedOutputRetained": False}


def validate_payload(raw: bytes, expected: dict[str, list[dict]], source_hashes: dict[str, str] | None = None) -> dict:
    result = _strict_json(raw, "gate payload")
    if not isinstance(result, dict) or result.get("schema") != "pre01-real-kubectl-gate.v1" or result.get("status") != "passed":
        raise GateError("payload did not report a passing complete gate")
    if set(result) != {"schema", "status", "source", "limits", "elapsedSeconds", "phases"}:
        raise GateError("payload has missing or unexpected top-level fields")
    elapsed_total = result.get("elapsedSeconds")
    if isinstance(elapsed_total, bool) or not isinstance(elapsed_total, (int, float)) or not math.isfinite(elapsed_total) or not 0 <= elapsed_total <= EXECUTION_SECONDS:
        raise GateError("payload elapsed time is malformed or over limit")
    if result.get("limits") != {"perCommandSeconds": PER_COMMAND_SECONDS, "perStreamBytes": PER_STREAM_BYTES,
                                "phaseWallSeconds": PHASE_WALL_SECONDS, "totalExecutionSeconds": EXECUTION_SECONDS}:
        raise GateError("payload execution bounds differ from the reviewed limits")
    source = result.get("source")
    if (not isinstance(source, dict)
            or set(source) != {"phaseOrder", "atomicSnapshot", "kubectlSha256", "replaySha256", "reportSha256", "scopeSha256"}
            or source.get("phaseOrder") != ["absent", "present"]
            or source.get("atomicSnapshot") is not False):
        raise GateError("payload source provenance is malformed")
    if source_hashes is not None:
        expected_source = {"kubectlSha256": ASSET["sha256"],
                           "replaySha256": source_hashes["evals/recorded-api/replay.py"],
                           "reportSha256": source_hashes["evals/reports/2026-10-01-pre01-crd.json"],
                           "scopeSha256": source_hashes["evals/pre01-crd/fixtures/capture-scope.json"]}
        if any(source.get(key) != value for key, value in expected_source.items()):
            raise GateError("payload source file hashes differ from the reviewed inputs")
    phases = result.get("phases")
    if not isinstance(phases, list) or [phase.get("phase") if isinstance(phase, dict) else None for phase in phases] != ["absent", "present"]:
        raise GateError("payload does not contain both phases in fixed order")
    for phase_data in phases:
        phase = phase_data["phase"]
        if set(phase_data) != {"phase", "status", "startedAt", "endedAt", "elapsedSeconds", "sourceRows",
                              "transportReady", "commands", "replayTrace", "listenerCleanupConfirmed"}:
            raise GateError("phase payload has missing or unexpected fields")
        if phase_data.get("status") != "passed" or phase_data.get("listenerCleanupConfirmed") is not True:
            raise GateError(f"{phase} phase or replay cleanup did not pass")
        elapsed = phase_data.get("elapsedSeconds")
        if isinstance(elapsed, bool) or not isinstance(elapsed, (int, float)) or not math.isfinite(elapsed) or not 0 <= elapsed <= 24:
            raise GateError("phase elapsed time is malformed or over limit")
        ready = phase_data.get("transportReady")
        if (not isinstance(ready, dict) or set(ready) != {"host", "port", "sourceRevision", "reportSha256", "scopeSha256", "atomicSnapshot"}
                or ready.get("host") != "127.0.0.1" or type(ready.get("port")) is not int or not 1 <= ready["port"] <= 65535
                or ready.get("sourceRevision") != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0"
                or ready.get("atomicSnapshot") is not False
                or ready.get("reportSha256") != source.get("reportSha256")
                or ready.get("scopeSha256") != source.get("scopeSha256")):
            raise GateError("loopback replay provenance is malformed")
        commands = phase_data.get("commands")
        routes = expected[phase]
        trace = phase_data.get("replayTrace")
        if not isinstance(commands, list) or len(commands) != 3 or not isinstance(trace, dict):
            raise GateError(f"{phase} phase has incomplete command or trace evidence")
        if (trace.get("phase") != phase or trace.get("status") != "stopped" or trace.get("coverageFailure") is not False
                or trace.get("listenerCleanupConfirmed") is not True or trace.get("requestsObserved") != 3
                or trace.get("coverageComplete") is not False):
            raise GateError(f"{phase} replay trace did not prove a clean three-request phase")
        if (trace.get("schema") != "recorded-api-replay-trace.v1" or trace.get("case") != "PRE-01"
                or trace.get("sourceRevision") != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0"
                or trace.get("reportSha256") != source.get("reportSha256")
                or trace.get("scopeSha256") != source.get("scopeSha256")
                or trace.get("maxRequests") != PHASE_MAX_REQUESTS):
            raise GateError("replay trace source pins or request bound differ")
        events = trace.get("events")
        if not isinstance(events, list) or len(events) != 3:
            raise GateError(f"{phase} trace request count differs")
        expected_source_rows = [{k: v for k, v in route.items() if k != "body"} for route in routes]
        if phase_data.get("sourceRows") != expected_source_rows:
            raise GateError("phase source route manifest differs from pinned responses")
        for index, (command, route, event) in enumerate(zip(commands, routes, events)):
            status = route["status"]
            if not isinstance(command, dict) or not isinstance(event, dict):
                raise GateError("command or trace event is malformed")
            if (command.get("phase") != phase or command.get("index") != index
                    or command.get("path") != route["path"] or command.get("expectedHTTPStatus") != status
                    or command.get("expectedSourceBodySha256") != route["bodySha256"]
                    or command.get("expectedSourceBodyBytes") != len(route["body"])
                    or event.get("method") != "GET" or event.get("path") != route["path"]
                    or event.get("requestNumber") != index + 1 or event.get("status") != status
                    or event.get("responseBodySha256") != route["bodySha256"]
                    or event.get("responseBodyBytes") != len(route["body"]) or event.get("captured") is not True
                    or event.get("reason") != "captured-response"
                    or event.get("sourceRows") != route["sourceRows"]):
                raise GateError(f"{phase} command {index} and source route evidence disagree")
            if command.get("status") != "passed" or command.get("outputTruncated") is not False:
                raise GateError(f"{phase} command {index} failed or truncated")
            elapsed_command = command.get("elapsedSeconds")
            if (isinstance(elapsed_command, bool) or not isinstance(elapsed_command, (int, float))
                    or not math.isfinite(elapsed_command) or not 0 <= elapsed_command <= 5
                    or command.get("error") is not None):
                raise GateError("kubectl command time or error metadata is invalid")
            stdout = _decode_stream(command, "stdout")
            stderr = _decode_stream(command, "stderr")
            exit_code = command.get("exitCode")
            if type(exit_code) is not int:
                raise GateError("kubectl exit code is malformed")
            if status == 404 and (exit_code != 1 or not stderr):
                raise GateError("captured 404 did not produce the pinned kubectl error exit")
            if status == 200 and (exit_code != 0 or stdout != route["body"]):
                raise GateError("captured 200 did not return exact response bytes through kubectl")
            argv = command.get("argv")
            if argv != ["/tools/kubectl", "--kubeconfig", f"/tmp/kubeconfig-{phase}", "--context", "recorded", "get", "--raw", route["path"]]:
                raise GateError("kubectl argv differs from the fixed read-only command")
    return result


def _decode_stream(command: dict, name: str) -> bytes:
    try:
        data = __import__("base64").b64decode(command.get(name + "Base64"), validate=True)
    except (ValueError, TypeError):
        raise GateError(f"kubectl {name} evidence is malformed") from None
    byte_count = command.get(name + "Bytes")
    if (type(byte_count) is not int or byte_count < 0 or byte_count > PER_STREAM_BYTES
            or len(data) > PER_STREAM_BYTES or len(data) != byte_count
            or sha(data) != command.get(name + "Sha256")):
        raise GateError(f"kubectl {name} bytes/hash do not reconcile")
    return data


def capture(assets: Path, docker_path: Path, context: str, output_path: Path, *, runner=None) -> int:
    version_helper.validate_context(context)
    docker, docker_target = version_helper.docker_launcher(docker_path)
    sources = source_inventory()
    expected_routes = _expected_routes()
    if any(len(expected_routes[p]) != 3 for p in expected_routes):
        raise GateError("both phases must have exactly three pinned routes")
    # Require new caller-owned output before copying the 55 MB binary.
    out = version_helper.safe_output(output_path)
    stage = out / "tools"
    stage.mkdir(mode=0o700)
    stage_hashes = {}
    try:
        stage_hashes = stage_files(stage, sources, assets / "kubectl")
        expected_paths = set(stage_hashes)
        source_hashes = {rel: sha(data) for rel, data in sources.items()}
        helper_hash = sha(Path(__file__).read_bytes())
        runtime_helper_hash = sha(VERSION_HELPER_PATH.read_bytes())
        source_hashes["assets/kubectl"] = stage_hashes["kubectl"]
        source_hashes["evals/pre01-real-kubectl/prepare.py"] = helper_hash
        source_hashes["evals/linux-runtime-versions/prepare.py"] = runtime_helper_hash
        payload_hash = source_hashes["evals/pre01-real-kubectl/payload.py"]
        replay_hash = source_hashes["evals/recorded-api/replay.py"]
        run = version_helper._inv04.run_bounded if runner is None else runner
        env = version_helper.docker_env()
        started_mono = time.monotonic()
        exec_deadline = started_mono + EXECUTION_SECONDS
        total_deadline = started_mono + TOTAL_SECONDS
        receipt = {"schema": "pre01-real-kubectl-capture.v1", "status": "running",
                   "case": "PRE-01", "issue": 732, "sourceFiles": {k: {"bytes": len(v), "sha256": sha(v)} for k, v in sources.items()},
                   "sourceAsset": {"name": "kubectl", **version_helper.validate_asset(assets / "kubectl", "kubectl")},
                   "stagedSha256": stage_hashes, "helperSha256": helper_hash, "payloadSha256": payload_hash,
                   "runtimeHelperSha256": runtime_helper_hash,
                   "replaySha256": replay_hash, "expectedImageId": IMAGE_ID, "requestedDockerContext": context,
                   "dockerLauncher": {"requestedPath": str(docker), "resolvedPath": str(docker_target), "sha256": sha_file(docker_target)},
                   "scope": "six exact kubectl get --raw calls against absent/present PRE-01 recorded responses; no target cluster or provider",
                   "configuredBounds": {"network": "none", "readonlyRootfs": True, "user": "65534:65534", "capDrop": ["ALL"],
                       "noNewPrivileges": True, "pidsLimit": 64, "memoryBytes": 1073741824, "cpus": 1,
                       "tmpfs": {"path": "/tmp", "sizeBytes": 67108864, "nosuid": True, "nodev": True}},
                   "limits": {"perKubectlCommandSeconds": 5, "perStreamBytes": 65536, "executionSeconds": EXECUTION_SECONDS,
                              "perPhaseReplaySeconds": 24, "cleanupSeconds": CLEANUP_SECONDS, "totalSeconds": TOTAL_SECONDS},
                   "containerConfigurationInspectVerified": False,
                   "containerCleanup": {"attempted": False, "verifiedAbsent": False, "errors": []}, "commands": []}
        owner = uuid.uuid4().hex
        name = "scout-pre01-kubectl-" + uuid.uuid4().hex[:16]
        container_id = ""
        create_attempted = False
        error = None
        result = None
        prior_signals = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
        def interrupted(signum, _frame):
            raise GateError("capture interrupted by " + signal.Signals(signum).name)
        for sig in prior_signals:
            signal.signal(sig, interrupted)

        def call(args, timeout):
            remaining = exec_deadline - time.monotonic()
            if remaining <= 0:
                raise GateError("overall execution deadline expired")
            began = time.monotonic(); out_bytes = err_bytes = None; code = None
            try:
                code, out_bytes, err_bytes = run([str(docker), "--context", context, *args], min(timeout, remaining), env=env, max_output=CAP)
                return code, out_bytes, err_bytes
            finally:
                receipt["commands"].append(_command_receipt(
                    [str(docker), "--context", context, *args], code, out_bytes, err_bytes,
                    time.monotonic() - began))

        try:
            code, raw, _ = call(["context", "inspect", context, "--format", "{{json .Endpoints.docker.Host}}"], 8)
            if code != 0:
                raise GateError("explicit Docker context inspection failed")
            receipt["dockerEndpointKind"] = "local-unix-socket"
            version_helper.validate_local_endpoint(raw)
            code, raw, _ = call(["image", "inspect", "--format", "{{.Id}}", IMAGE_ID], 8)
            if code != 0:
                raise GateError("exact cached image is unavailable; pull is disabled")
            version_helper.validate_image_inspect(raw)
            create_args = build_container_create_args(name, owner, stage)
            create_attempted = True
            code, raw, _ = call(create_args, 10)
            if code != 0:
                raise GateError("owned container creation failed")
            container_id = raw.decode("ascii", "strict").strip()
            if not re.fullmatch(r"[0-9a-f]{64}", container_id):
                raise GateError("container create returned malformed ID")
            code, raw, _ = call(["container", "inspect", "--format", "{{json .}}", container_id], 8)
            if code != 0:
                raise GateError("owned container could not be inspected before start")
            (out / "container-before.inspect.json").write_bytes(raw)
            validate_container_inspect(raw, name, owner, stage)
            receipt["containerConfigurationInspectVerified"] = True
            code, raw, err = call(["start", "--attach", container_id], EXECUTION_SECONDS)
            (out / "payload.stdout.bin").write_bytes(raw)
            (out / "payload.stderr.bin").write_bytes(err)
            receipt["commands"][-1]["returnedOutputRetained"] = True
            try:
                diagnostic = _strict_json(raw, "gate payload")
            except GateError:
                diagnostic = None
            if isinstance(diagnostic, dict) and diagnostic.get("schema") == "pre01-real-kubectl-gate.v1":
                _persist_payload_diagnostics(out, diagnostic)
            if code != 0:
                raise GateError("payload/container did not exit successfully")
            result = validate_payload(raw, expected_routes, source_hashes)
            code2, final_raw, _ = call(["container", "inspect", "--format", "{{json .}}", container_id], 8)
            if code2 != 0:
                raise GateError("completed container could not be inspected")
            summary = validate_container_inspect(final_raw, name, owner, stage, finished=True)
            receipt["containerAfter"] = summary
            if code != 0 or summary["exitCode"] != 0:
                raise GateError("payload/container did not exit successfully")
            (out / "container-after.inspect.json").write_bytes(final_raw)
            receipt["resultStatus"] = result["status"]
        except BaseException as exc:
            error = str(exc)[:240] if isinstance(exc, GateError) else type(exc).__name__
        finally:
            cleanup_started = time.monotonic()
            deadline = min(total_deadline, cleanup_started + CLEANUP_SECONDS)
            if create_attempted:
                receipt["containerCleanup"] = version_helper.cleanup(docker, context, name, owner, env, deadline, run, container_id)
            else:
                receipt["containerCleanup"] = {"attempted": False, "verifiedAbsent": True, "errors": []}
            clean = receipt["containerCleanup"].get("verifiedAbsent") is True and not receipt["containerCleanup"].get("errors")
            source_paths = {rel: REPO / rel for rel in SOURCE_PINS if not rel.endswith("payload.py")}
            source_paths["evals/pre01-real-kubectl/payload.py"] = HERE / "payload.py"
            source_paths["evals/pre01-real-kubectl/prepare.py"] = Path(__file__)
            source_paths["evals/linux-runtime-versions/prepare.py"] = VERSION_HELPER_PATH
            source_paths["assets/kubectl"] = assets / "kubectl"
            source_after = {}
            for rel, path in source_paths.items():
                try:
                    source_after[rel] = sha_file(path)
                except OSError:
                    source_after[rel] = None
                    error = error or f"pinned input {rel} could not be rechecked"
            if source_after != source_hashes:
                error = error or "pinned source changed during run"
            receipt["sourceAfterSha256"] = source_after
            try:
                staged_after = require_stage_unchanged(stage, stage_hashes)
            except (OSError, GateError):
                staged_after = {}
                error = error or "staged inputs could not be verified"
            if staged_after != stage_hashes:
                error = error or "staged inputs changed during run"
            receipt["stagedAfterSha256"] = staged_after
            if clean:
                try:
                    remove_stage(stage, expected_paths)
                    receipt["stagingCleanupVerified"] = True
                except (OSError, GateError):
                    receipt["stagingCleanupVerified"] = False
                    error = error or "owned staging cleanup failed"
            else:
                receipt["stagingCleanupVerified"] = False
                receipt["stagingRetainedBecauseContainerAbsenceUnverified"] = True
            for sig, handler in prior_signals.items():
                signal.signal(sig, handler)
            receipt.update({"status": "passed" if error is None and result is not None and clean else "failed",
                            "error": error, "elapsedSeconds": round(time.monotonic() - started_mono, 6),
                            "cleanupElapsedSeconds": round(time.monotonic() - cleanup_started, 6)})
            (out / "receipt.json").write_text(json.dumps(receipt, sort_keys=True, indent=2) + "\n")
        return 0 if receipt["status"] == "passed" else 1
    except BaseException:
        if stage.exists():
            # Pre-container staging errors leave evidence untouched for diagnosis.
            pass
        raise


def _persist_payload_diagnostics(out: Path, payload: dict) -> None:
    """Retain bounded child evidence on failures without treating it as accepted."""
    (out / "gate-result.json").write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n")
    phases = payload.get("phases")
    if not isinstance(phases, list):
        return
    import base64
    for phase in phases:
        if not isinstance(phase, dict) or phase.get("phase") not in {"absent", "present"}:
            continue
        phase_name = phase["phase"]
        trace = phase.get("replayTrace")
        if isinstance(trace, dict):
            (out / (phase_name + ".replay-trace.json")).write_text(json.dumps(trace, sort_keys=True, indent=2) + "\n")
        commands = phase.get("commands")
        if not isinstance(commands, list):
            continue
        for command in commands:
            if not isinstance(command, dict) or type(command.get("index")) is not int or command["index"] not in range(3):
                continue
            for stream_name in ("stdout", "stderr"):
                try:
                    data = base64.b64decode(command.get(stream_name + "Base64", ""), validate=True)
                except (ValueError, TypeError):
                    continue
                if len(data) > PER_STREAM_BYTES:
                    continue
                target = out / f"{phase_name}-{command['index']}-{stream_name}.bin"
                fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd, "wb") as stream:
                    stream.write(data)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required before creating an owned container")
    parser.add_argument("--assets", type=Path, required=True)
    parser.add_argument("--docker-binary", type=Path, required=True)
    parser.add_argument("--docker-context", required=True)
    parser.add_argument("--image-id", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.execute:
        parser.error("--execute is required")
    if args.image_id != IMAGE_ID:
        parser.error("--image-id must match the reviewed cached image pin")
    try:
        return capture(args.assets, args.docker_binary, args.docker_context, args.output_dir)
    except (GateError, version_helper.CaptureError) as error:
        parser.exit(1, "PRE-01 real-kubectl gate failed: " + str(error) + "\n")


if __name__ == "__main__":
    raise SystemExit(main())
