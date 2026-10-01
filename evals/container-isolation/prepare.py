#!/usr/bin/env python3
"""Prepare one bounded, disposable, offline container-isolation proof."""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import shutil
import signal
import sys
import tempfile
import time
import uuid
from datetime import datetime, timezone

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
INV04 = REPO / "evals" / "inv04-rbac" / "capture.py"
_spec = importlib.util.spec_from_file_location("container_isolation_inv04", INV04)
_inv04 = importlib.util.module_from_spec(_spec)
assert _spec and _spec.loader
_spec.loader.exec_module(_inv04)

IMAGE_ID = "sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5"
IMAGE_TAG = "python:3.11-slim"  # Informational only; never pulled or used as the image selector.
FIXTURE = HERE / "fixtures" / "authored.txt"
FIXTURE_BYTES = b"cub-scout-container-isolation-fixture-v1\n"
OWNER_LABEL = "io.confighub.cub-scout.container-isolation.owner"
MAX_OUTPUT = 1024 * 1024
EXECUTION_TIMEOUT = 30
CLEANUP_TIMEOUT = 15
TOTAL_TIMEOUT = 60
PAYLOAD_ASSERTIONS = {
    "fixture_read_exact", "fixture_write_denied", "outside_tmp_write_denied", "tmp_write_allowed",
    "synthetic_host_marker_unavailable", "loopback_http_ok", "non_loopback_child_network_denied",
}


class CaptureError(RuntimeError):
    pass


class CaptureInterrupted(CaptureError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def strict_json(data: bytes, description: str) -> Any:
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise CaptureError(f"{description} contains a duplicate JSON key")
            result[key] = value
        return result
    def reject_constant(value):
        raise CaptureError(f"{description} contains a non-finite JSON value")
    def finite_float(value):
        parsed = float(value)
        if not math.isfinite(parsed):
            raise CaptureError(f"{description} contains a non-finite JSON value")
        return parsed
    try:
        return json.loads(data.decode("utf-8", "strict"), object_pairs_hook=unique,
                          parse_constant=reject_constant, parse_float=finite_float)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError(f"{description} is malformed JSON") from None


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def validate_image_pin(image_id: str) -> None:
    if image_id != IMAGE_ID:
        raise CaptureError("image ID must match the reviewed cached-image pin")


def validate_context_name(context: str) -> None:
    if not isinstance(context, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,62}", context):
        raise CaptureError("Docker context name is malformed")


def docker_launcher(path: Path) -> Path:
    """Validate the resolved executable, but preserve the requested launcher argv[0]."""
    requested = path.expanduser().absolute()
    try:
        target = requested.resolve(strict=True)
    except OSError:
        raise CaptureError("Docker launcher is unavailable") from None
    if not target.is_file() or not os.access(target, os.X_OK):
        raise CaptureError("Docker launcher target is not an executable regular file")
    return requested


def docker_env(source: dict[str, str] | None = None) -> dict[str, str]:
    """Keep only local client configuration needed for explicit context lookup."""
    source = os.environ if source is None else source
    clean = {key: source[key] for key in ("PATH", "HOME", "DOCKER_CONFIG", "TMPDIR", "LANG") if source.get(key)}
    clean.setdefault("PATH", "/usr/bin:/bin")
    return clean


def _safe_output(path: Path, repo_root: Path = REPO) -> Path:
    requested = path.expanduser().absolute()
    if requested.name in ("", ".", "..") or ".." in path.parts:
        raise CaptureError("output path is unsafe")
    if requested.is_symlink() or requested.exists():
        raise CaptureError("output path must be new and must not be a symlink")
    try:
        parent = requested.parent.resolve(strict=True)
        repo = repo_root.resolve(strict=True)
        home = Path.home().resolve(strict=True)
    except OSError:
        raise CaptureError("output parent or safety root is unavailable") from None
    if requested.parent != parent and requested.parent.resolve() != parent:
        raise CaptureError("output parent is unsafe")
    try:
        parent.relative_to(repo)
        raise CaptureError("output must be outside the source checkout")
    except ValueError:
        pass
    try:
        parent.relative_to(home)
        raise CaptureError("output must be outside the home directory")
    except ValueError:
        pass
    # Permit only conventional temporary roots; output is private and transient.
    allowed_roots = {Path(tempfile.gettempdir()).resolve(), Path("/tmp").resolve(), Path("/var/tmp").resolve()}
    if not any(parent == root or root in parent.parents for root in allowed_roots):
        raise CaptureError("output must be under a temporary directory")
    try:
        requested.mkdir(mode=0o700)
        os.chmod(requested, 0o700)
    except OSError:
        raise CaptureError("could not create private output directory") from None
    if requested.is_symlink() or (requested.stat().st_mode & 0o777) != 0o700:
        raise CaptureError("output directory is not private")
    return requested


def _write(path: Path, content: bytes) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(content)


def _record_command(out: Path, name: str, code: int | None, stdout: bytes, stderr: bytes,
                    started: str, began: float, error: str | None = None,
                    persist: bool = True) -> dict:
    if persist:
        _write(out / (name + ".stdout.bin"), stdout)
        _write(out / (name + ".stderr.bin"), stderr)
    result = {"operation": name, "startedAt": started, "endedAt": utc_now(),
              "elapsedSeconds": time.monotonic() - began, "exitCode": code,
              "stdoutBytes": len(stdout), "stderrBytes": len(stderr),
              "stdoutSha256": sha256(stdout), "stderrSha256": sha256(stderr),
              "outputFilesRetained": persist}
    if error:
        result["error"] = error[:300]
    return result


def _call(docker: Path, context: str, args: list[str], env: dict[str, str], deadline: float,
          timeout: float = 10) -> tuple[int, bytes, bytes]:
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise CaptureError("overall execution budget expired")
    argv = [str(docker), "--context", context, *args]
    return _inv04.run_bounded(argv, min(timeout, remaining), env=env, max_output=MAX_OUTPUT)


def validate_local_endpoint(data: bytes) -> None:
    try:
        endpoint = json.loads(data)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError("Docker context endpoint response is malformed") from None
    if not isinstance(endpoint, str) or not endpoint.startswith("unix://") or any(c in endpoint for c in "\r\n\x00"):
        raise CaptureError("selected Docker context must use a local Unix socket")


def validate_image_inspect(data: bytes) -> str:
    try:
        image_id = data.decode("ascii", "strict").strip()
    except UnicodeDecodeError:
        raise CaptureError("cached image inspect response is malformed") from None
    if image_id != IMAGE_ID:
        raise CaptureError("cached image ID does not match reviewed pin")
    return image_id


def build_container_args(name: str, owner: str, fixture: Path, marker_path: str,
                         payload_source: bytes | None = None) -> list[str]:
    if not re.fullmatch(r"scout-isolation-[0-9a-f]{16}", name) or not re.fullmatch(r"[0-9a-f]{32}", owner):
        raise CaptureError("generated container ownership identity is malformed")
    if fixture.is_symlink() or not fixture.is_file() or fixture.read_bytes() != FIXTURE_BYTES:
        raise CaptureError("authored fixture is missing, changed, or unsafe")
    if any(char in str(fixture.absolute()) for char in ",\r\n\x00"):
        raise CaptureError("authored fixture path cannot be safely represented in Docker mount syntax")
    marker_file = Path(marker_path)
    try:
        marker_canonical = marker_file.resolve(strict=True)
    except OSError:
        raise CaptureError("synthetic host marker file is unavailable") from None
    temp_roots = {Path(tempfile.gettempdir()).resolve(), Path("/tmp").resolve(), Path("/var/tmp").resolve()}
    if (not marker_file.is_absolute() or marker_file.as_posix() != marker_path
            or marker_canonical != marker_file or marker_file.is_symlink() or not marker_file.is_file()
            or not any(marker_file == root or root in marker_file.parents for root in temp_roots)):
        raise CaptureError("synthetic host marker path is unsafe")
    payload_path = HERE / "payload.py"
    if payload_path.is_symlink() or not payload_path.is_file():
        raise CaptureError("authored payload is missing or unsafe")
    try:
        payload = (payload_source if payload_source is not None else payload_path.read_bytes()).decode("utf-8", "strict")
    except UnicodeDecodeError:
        raise CaptureError("authored payload is not valid UTF-8") from None
    # Inject only a generated marker path; payload is inline so it is not a second mount.
    marker_expression = 'os.environ.get("SCOUT_SYNTHETIC_HOST_MARKER", "/scout-host-marker-unmounted")'
    if payload.count(marker_expression) != 1:
        raise CaptureError("payload marker injection point is missing or ambiguous")
    payload = payload.replace(marker_expression, repr(marker_path))
    mount = "type=bind,src=" + str(fixture.absolute()) + ",dst=/fixture/input.txt,readonly"
    tmpfs = "/tmp:rw,noexec,nosuid,size=16m,uid=65534,gid=65534"
    return ["container", "create", "--name", name, "--label", OWNER_LABEL + "=" + owner,
            "--pull=never", "--network=none", "--read-only", "--user", "65534:65534",
            "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=32",
            "--memory=256m", "--cpus=0.5", "--tmpfs", tmpfs, "--mount", mount,
            IMAGE_ID, "python", "-c", payload]


def _one_inspect(data: bytes) -> dict:
    value = strict_json(data, "container inspect response")
    if isinstance(value, list) and len(value) == 1 and isinstance(value[0], dict):
        return value[0]
    if isinstance(value, dict):
        return value
    raise CaptureError("container inspect response must identify exactly one container")


def validate_owned_identity(obj: dict, *, name: str, owner: str) -> str:
    ident = obj.get("Id")
    raw_config = obj.get("Config")
    labels = raw_config.get("Labels") if isinstance(raw_config, dict) else None
    if (not isinstance(ident, str) or not re.fullmatch(r"[0-9a-f]{64}", ident)
            or obj.get("Name") != "/" + name or not isinstance(labels, dict)
            or labels.get(OWNER_LABEL) != owner):
        raise CaptureError("container ownership marker/name did not match this run")
    return ident


def _tmpfs_tokens(value: str) -> set[str]:
    tokens = value.split(",")
    if len(tokens) != len(set(tokens)) or any(not token for token in tokens):
        raise CaptureError("bounded /tmp tmpfs options are malformed")
    token_set = set(tokens)
    accepted = [
        {"rw", "noexec", "nosuid", "size=16777216", "uid=65534", "gid=65534"},
        {"rw", "noexec", "nosuid", "size=16m", "uid=65534", "gid=65534"},
    ]
    if token_set not in accepted:
        raise CaptureError("bounded /tmp tmpfs options differ from the exact reviewed setting")
    return token_set


def validate_owned_inspect(data: bytes, *, name: str, owner: str, fixture: Path,
                           require_running_state: bool = False) -> tuple[str, dict]:
    obj = _one_inspect(data)
    ident = validate_owned_identity(obj, name=name, owner=owner)
    raw_config = obj.get("Config")
    config, host = raw_config, obj.get("HostConfig")
    if not isinstance(config, dict) or not isinstance(host, dict):
        raise CaptureError("container inspect configuration is malformed")
    if not isinstance(host.get("CapDrop"), list) or not isinstance(host.get("SecurityOpt"), list):
        raise CaptureError("container capability/security settings are malformed")
    if not isinstance(host.get("Tmpfs"), dict):
        raise CaptureError("container tmpfs settings are malformed")
    if config.get("Image") != IMAGE_ID or obj.get("Image") != IMAGE_ID:
        raise CaptureError("container inspect image ID differs from reviewed pin")
    if config.get("User") != "65534:65534" or host.get("ReadonlyRootfs") is not True:
        raise CaptureError("container user or read-only rootfs setting differs")
    if host.get("NetworkMode") != "none" or host.get("CapDrop") != ["ALL"]:
        raise CaptureError("container network or capability-drop setting differs")
    security_options = host.get("SecurityOpt", [])
    if (any(not isinstance(value, str) for value in security_options)
            or set(security_options) not in ({"no-new-privileges"}, {"no-new-privileges:true"})):
        raise CaptureError("container no-new-privileges setting is absent")
    if (host.get("Privileged") is not False or host.get("CapAdd") not in (None, [])
            or host.get("PidMode") not in (None, "", "private")
            or host.get("IpcMode") not in (None, "", "private")):
        raise CaptureError("container privilege, capability-add, PID, or IPC setting is unsafe")
    if host.get("PidsLimit") != 32 or host.get("Memory") != 268435456 or host.get("NanoCpus") != 500000000:
        raise CaptureError("container resource settings differ from configured bounds")
    tmpfs = host.get("Tmpfs", {}).get("/tmp", "")
    if not isinstance(tmpfs, str):
        raise CaptureError("private bounded /tmp tmpfs setting is absent")
    _tmpfs_tokens(tmpfs)
    mounts = obj.get("Mounts")
    if not isinstance(mounts, list) or any(not isinstance(mount, dict) for mount in mounts):
        raise CaptureError("container inspect mount list is malformed")
    binds = [mount for mount in mounts if mount.get("Type") == "bind"]
    tmpfs_mounts = [mount for mount in mounts if mount.get("Type") == "tmpfs"]
    if (len(binds) != 1 or binds[0].get("Destination") != "/fixture/input.txt"
            or binds[0].get("Source") != str(fixture.resolve()) or binds[0].get("RW") is not False):
        raise CaptureError("container must have exactly one read-only authored fixture mount")
    if any(not (mount.get("Type") == "bind" and mount in binds
                or mount.get("Type") == "tmpfs" and mount.get("Destination") == "/tmp")
           for mount in mounts):
        raise CaptureError("unexpected extra container mount")
    if len(tmpfs_mounts) > 1:
        raise CaptureError("unexpected duplicate tmpfs mount")
    if tmpfs_mounts and tmpfs_mounts[0].get("RW") is not True:
        raise CaptureError("bounded /tmp tmpfs is not writable")
    state = obj.get("State", {})
    if not isinstance(state, dict):
        raise CaptureError("container inspect state is malformed")
    if require_running_state and (state.get("Status") != "exited"
                                  or type(state.get("ExitCode")) is not int or state.get("ExitCode") != 0):
        raise CaptureError("container did not exit before inspection")
    selected = {"containerId": ident, "name": name, "imageId": config["Image"],
                "user": config["User"], "readOnlyRootfs": host["ReadonlyRootfs"],
                "networkMode": host["NetworkMode"], "capDrop": host["CapDrop"],
                "securityOpt": host["SecurityOpt"], "pidsLimit": host["PidsLimit"],
                "memoryBytes": host["Memory"], "nanoCpus": host["NanoCpus"],
                "tmpfsConfigured": True, "fixtureMount": {"destination": "/fixture/input.txt", "readOnly": True}}
    if require_running_state:
        selected["exitCode"] = state.get("ExitCode")
    return ident, selected


def validate_payload(stdout: bytes, stderr: bytes = b"") -> dict:
    if stderr.strip():
        raise CaptureError("container payload wrote unexpected stderr")
    value = strict_json(stdout, "container payload output")
    if not isinstance(value, dict) or set(value) != {"schema", "assertions", "childProcessAttempted"}:
        raise CaptureError("container payload assertion object has unexpected/missing fields")
    assertions = value.get("assertions")
    if (value.get("schema") != "container-isolation-payload.v1" or value.get("childProcessAttempted") is not True
            or not isinstance(assertions, dict) or set(assertions) != PAYLOAD_ASSERTIONS
            or any(type(item) is not bool or item is not True for item in assertions.values())):
        raise CaptureError("container payload did not demonstrate every required isolation assertion")
    return assertions


def _inspect_by_name(docker: Path, context: str, target: str, env: dict[str, str], deadline: float,
                     runner=None) -> tuple[int, bytes, bytes]:
    run = _inv04.run_bounded if runner is None else runner
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise CaptureError("cleanup budget expired")
    return run([str(docker), "--context", context, "container", "inspect", "--format", "{{json .}}", target],
               min(5, remaining), env=env, max_output=MAX_OUTPUT)


def _is_exact_missing(code: int, stdout: bytes, stderr: bytes, target: str) -> bool:
    if type(code) is not int or code == 0 or stdout not in (b"", b"\n") or not target:
        return False
    try:
        lines = [line.strip() for line in stderr.decode("utf-8", "strict").splitlines() if line.strip()]
    except UnicodeDecodeError:
        return False
    expected = {"Error response from daemon: No such container: " + target,
                "Error: No such object: " + target}
    return len(lines) == 1 and lines[0] in expected


def cleanup_owned_container(docker: Path, context: str, name: str, owner: str, fixture: Path,
                            env: dict[str, str], deadline: float, runner=None,
                            known_id: str = "") -> dict:
    """Remove only a container whose inspected ID, name and owner label match this run."""
    run = _inv04.run_bounded if runner is None else runner
    result = {"attempted": True, "verifiedAbsent": False, "errors": [], "operations": []}
    try:
        target = known_id or name
        started, began = utc_now(), time.monotonic()
        code, data, err = _inspect_by_name(docker, context, target, env, deadline, runner=run)
        result["operations"].append({"operation": "inspect-before-remove", "startedAt": started,
            "endedAt": utc_now(), "elapsedSeconds": time.monotonic() - began, "exitCode": code,
            "stdoutSha256": sha256(data), "stderrSha256": sha256(err), "outputFilesRetained": False})
        if code:
            if _is_exact_missing(code, data, err, target):
                result["verifiedAbsent"] = True
                return result
            raise CaptureError("owned container inspection failed; cleanup is uncertain")
        # Ownership is sufficient to authorize removal; configuration failure must
        # not leak a container this invocation created and can safely identify.
        ident = validate_owned_identity(_one_inspect(data), name=name, owner=owner)
        if known_id and ident != known_id:
            raise CaptureError("container ID changed before cleanup")
        result["containerId"] = ident
        result["ownerLabelMatched"] = True
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise CaptureError("cleanup budget expired")
        started, began = utc_now(), time.monotonic()
        rm_code, _rm_out, rm_err = run([str(docker), "--context", context,
                                        "container", "rm", "--force", ident],
                                       min(10, remaining), env=env, max_output=MAX_OUTPUT)
        result["operations"].append({"operation": "remove-owned-container", "startedAt": started,
            "endedAt": utc_now(), "elapsedSeconds": time.monotonic() - began, "exitCode": rm_code,
            "stderrSha256": sha256(rm_err), "outputFilesRetained": False})
        result["removeExitCode"] = rm_code
        result["removeStderrSha256"] = sha256(rm_err)
        started, began = utc_now(), time.monotonic()
        code, data, err = _inspect_by_name(docker, context, ident, env, deadline, runner=run)
        result["operations"].append({"operation": "verify-container-absent", "startedAt": started,
            "endedAt": utc_now(), "elapsedSeconds": time.monotonic() - began, "exitCode": code,
            "stdoutSha256": sha256(data), "stderrSha256": sha256(err), "outputFilesRetained": False})
        if _is_exact_missing(code, data, err, ident):
            result["verifiedAbsent"] = True
        else:
            raise CaptureError("owned container remains or absence could not be verified")
    except BaseException as exc:
        result["errors"].append((str(exc) if isinstance(exc, CaptureError) else type(exc).__name__)[:250])
    return result


def _capture_signals():
    prior = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
    def interrupt(signum, _frame):
        raise CaptureInterrupted("capture interrupted by " + signal.Signals(signum).name)
    for sig in prior:
        signal.signal(sig, interrupt)
    return prior


def _restore_signals(prior: dict) -> None:
    for sig, handler in prior.items():
        signal.signal(sig, handler)


def capture(docker_path: Path, docker_context: str, image_id: str, output_path: Path,
            *, runner=None) -> int:
    validate_image_pin(image_id)
    validate_context_name(docker_context)
    docker = docker_launcher(docker_path)
    fixture_input = FIXTURE.absolute()
    if fixture_input.is_symlink() or not fixture_input.is_file():
        raise CaptureError("authored fixture is missing or unsafe")
    fixture = fixture_input.resolve(strict=True)
    if fixture.read_bytes() != FIXTURE_BYTES:
        raise CaptureError("authored fixture is missing, changed, or unsafe")
    out = _safe_output(output_path)
    owner = uuid.uuid4().hex
    name = "scout-isolation-" + uuid.uuid4().hex[:16]
    host_marker = out / ("host-marker-" + owner + ".txt")
    _write(host_marker, b"synthetic host-only marker; intentionally not mounted\n")
    marker_path = str(host_marker.resolve(strict=True))
    if not host_marker.is_file() or host_marker.is_symlink():
        raise CaptureError("synthetic host marker is not a regular owned file")
    marker_hash = sha256(host_marker.read_bytes())
    fixture_hash = sha256(fixture.read_bytes())
    payload_path = HERE / "payload.py"
    helper_path = Path(__file__)
    if payload_path.is_symlink() or helper_path.is_symlink():
        raise CaptureError("helper or payload source must not be a symlink")
    payload_source = payload_path.read_bytes()
    helper_source = helper_path.read_bytes()
    helper_hash = sha256(helper_source)
    docker_target = docker.resolve(strict=True)
    docker_binary_hash = sha256_file(docker_target)
    inv04_hash = sha256(INV04.read_bytes())
    started_at, start_clock = utc_now(), time.monotonic()
    total_deadline = start_clock + TOTAL_TIMEOUT
    execution_deadline = start_clock + TOTAL_TIMEOUT - CLEANUP_TIMEOUT
    commands: list[dict] = []
    inspected = None
    container_id = ""
    create_attempted = False
    cleanup = {"attempted": False, "verifiedAbsent": False, "errors": [], "operations": []}
    assertions = None
    error = None
    source_revision = ""
    receipt = {"schema": "container-isolation-capture.v1", "status": "running"}
    receipt["imageTagInformational"] = IMAGE_TAG
    receipt["expectedImageId"] = IMAGE_ID
    receipt["requestedDockerContext"] = docker_context
    receipt["ownedContainerName"] = name
    receipt["ownershipLabelKey"] = OWNER_LABEL
    receipt["ownershipLabelValueSha256"] = sha256(owner.encode("ascii"))
    receipt["contextMutationPolicy"] = "All Docker calls name this context explicitly; no context use/change command is invoked."
    receipt["kubeconfigAccess"] = False
    receipt["configuredBounds"] = {"network": "none", "readOnlyRootfs": True, "user": "65534:65534",
        "capDrop": ["ALL"], "noNewPrivileges": True, "pidsLimit": 32,
        "memoryBytes": 268435456, "nanoCpus": 500000000,
        "tmpfs": {"path": "/tmp", "sizeBytes": 16777216, "noexec": True, "nosuid": True}}
    receipt["containerCreatePolicy"] = ["--pull=never", "--network=none", "--read-only", "--user 65534:65534",
        "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=32", "--memory=256m", "--cpus=0.5",
        "one read-only fixture bind at /fixture/input.txt", "bounded writable /tmp tmpfs"]
    receipt["containerConfigurationInspectVerified"] = False
    receipt["boundInterpretation"] = ("configuredBounds and containerCreatePolicy are requested settings; container inspection verifies them only if "
        "containerConfigurationInspectVerified is true. Resource saturation limits are not stress-tested. "
        "One child process was started in a successful payload; process creation is not claimed to be prohibited.")
    env = docker_env()
    run = _inv04.run_bounded if runner is None else runner
    signal_state = None

    def call(args: list[str], op: str, timeout: float = 10,
             persist: bool = True) -> tuple[int, bytes, bytes]:
        if time.monotonic() >= execution_deadline:
            raise CaptureError("overall execution budget expired")
        started, began = utc_now(), time.monotonic()
        code = None
        stdout = stderr = b""
        exception = None
        try:
            remaining = execution_deadline - time.monotonic()
            code, stdout, stderr = run([str(docker), "--context", docker_context, *args],
                                       min(timeout, remaining), env=env, max_output=MAX_OUTPUT)
        except BaseException as exc:
            exception = type(exc).__name__
        commands.append(_record_command(out, op, code, stdout, stderr, started, began, exception, persist))
        if exception:
            raise CaptureError(op + " failed: " + exception)
        return code, stdout, stderr

    try:
        signal_state = _capture_signals()
        git = shutil.which("git")
        if not git:
            raise CaptureError("git is unavailable for source provenance")
        code, raw, _ = call(["--version"], "docker-version", 5)
        if code:
            raise CaptureError("Docker launcher version check failed")
        # Reuse the bounded runner directly for local source metadata, not Docker.
        code, raw, _ = run([git, "-C", str(REPO), "rev-parse", "HEAD"], 5, env=env, max_output=4096)
        if code:
            raise CaptureError("could not identify source revision")
        source_revision = raw.decode("ascii", "strict").strip()
        if not re.fullmatch(r"[0-9a-f]{40,64}", source_revision):
            raise CaptureError("source revision is malformed")
        code, raw, _ = run([git, "-C", str(REPO), "status", "--porcelain"], 5, env=env, max_output=16384)
        if code or raw.strip():
            raise CaptureError("source checkout must be clean before offline probe")
        code, endpoint, _ = call(["context", "inspect", docker_context, "--format", "{{json .Endpoints.docker.Host}}"], "inspect-local-context", 8, persist=False)
        if code:
            raise CaptureError("could not inspect explicit Docker context")
        validate_local_endpoint(endpoint)
        code, image_out, _ = call(["image", "inspect", "--format", "{{.Id}}", IMAGE_ID], "inspect-cached-image", 8, persist=False)
        if code:
            raise CaptureError("pinned image is not cached locally")
        image_actual = validate_image_inspect(image_out)
        args = build_container_args(name, owner, fixture, marker_path, payload_source=payload_source)
        injected_payload = args[args.index("python") + 2].encode("utf-8")
        create_attempted = True
        code, raw_id, _ = call(args, "create-owned-container", 10)
        if code:
            raise CaptureError("owned container create failed")
        container_id = raw_id.decode("ascii", "strict").strip()
        if not re.fullmatch(r"[0-9a-f]{64}", container_id):
            raise CaptureError("Docker create did not return one container ID")
        code, inspect_bytes, _ = call(["container", "inspect", "--format", "{{json .}}", container_id], "inspect-owned-container", 8, persist=False)
        if code:
            raise CaptureError("could not inspect newly created owned container")
        container_id, inspected = validate_owned_inspect(inspect_bytes, name=name, owner=owner, fixture=fixture)
        receipt["containerConfigurationInspectVerified"] = True
        start_code, stdout, stderr = call(["start", "--attach", container_id], "run-isolation-payload", EXECUTION_TIMEOUT)
        _write(out / "container.stdout.bin", stdout)
        _write(out / "container.stderr.bin", stderr)
        inspect_code, final_inspect, _ = call(["container", "inspect", "--format", "{{json .}}", container_id], "inspect-container-result", 5, persist=False)
        if inspect_code:
            raise CaptureError("could not inspect completed container")
        container_id, inspected = validate_owned_inspect(final_inspect, name=name, owner=owner,
                                                         fixture=fixture, require_running_state=True)
        if type(inspected.get("exitCode")) is not int or inspected.get("exitCode") != 0 or type(start_code) is not int or start_code != 0:
            raise CaptureError("container payload did not exit successfully")
        assertions = validate_payload(stdout, stderr)
        receipt.update({"imageId": image_actual, "dockerContext": docker_context,
                        "dockerEndpointKind": "local-unix-socket", "container": inspected,
                        "demonstratedAssertions": assertions,
                        "demonstratedBounds": {"networkNone": assertions.get("non_loopback_child_network_denied") is True,
                                               "readOnlyFixture": assertions.get("fixture_write_denied") is True,
                                               "readOnlyRootfsOutsideTmp": assertions.get("outside_tmp_write_denied") is True,
                                               "writableTmpfs": assertions.get("tmp_write_allowed") is True,
                                               "loopbackOnlyHTTP": assertions.get("loopback_http_ok") is True},
                        "hostMarkerContainerPath": marker_path,
                        "hostMarkerUnavailableInContainer": assertions.get("synthetic_host_marker_unavailable") is True})
    except BaseException as exc:
        error = str(exc)[:300] if isinstance(exc, CaptureError) else type(exc).__name__
    finally:
        if signal_state is not None:
            # Restore signal behavior only after bounded cleanup.
            pass
        cleanup_started = time.monotonic()
        cleanup_deadline = min(total_deadline, cleanup_started + CLEANUP_TIMEOUT)
        if create_attempted:
            cleanup = cleanup_owned_container(docker, docker_context, name, owner, fixture,
                                              env, cleanup_deadline, runner=run, known_id=container_id)
        else:
            cleanup["verifiedAbsent"] = True
        cleanup["elapsedSeconds"] = time.monotonic() - cleanup_started
        if not cleanup["verifiedAbsent"] or cleanup["errors"]:
            cleanup["verifiedAbsent"] = False
        if signal_state is not None:
            _restore_signals(signal_state)
        try:
            fixture_after = sha256(fixture.read_bytes())
        except OSError:
            fixture_after = ""
        if fixture_after != fixture_hash:
            error = error or "authored fixture changed during run"
        try:
            payload_after = sha256(payload_path.read_bytes())
            helper_after = sha256(helper_path.read_bytes())
            inv04_after = sha256(INV04.read_bytes())
        except OSError:
            payload_after = helper_after = inv04_after = ""
        if payload_after != sha256(payload_source) or helper_after != helper_hash or inv04_after != inv04_hash:
            error = error or "helper or payload source changed during run"
        try:
            if host_marker.is_symlink() or not host_marker.is_file():
                raise OSError("host marker was replaced or removed")
            marker_after = sha256(host_marker.read_bytes())
        except OSError:
            marker_after = ""
        if marker_after != marker_hash:
            error = error or "synthetic host marker changed during run"
        receipt.update({"status": "passed" if error is None and assertions and cleanup["verifiedAbsent"] else "failed",
                        "sourceRevision": source_revision,
                        "helperSha256": helper_hash,
                        "payloadSha256": sha256(payload_source),
                        "injectedPayloadSha256": sha256(injected_payload) if "injected_payload" in locals() else None,
                        "dockerLauncher": {"requestedPath": str(docker), "resolvedPath": str(docker_target),
                                           "sha256": docker_binary_hash},
                        "inv04Runner": {"path": str(INV04), "sha256": inv04_hash,
                                        "afterSha256": inv04_after},
                        "fixtureSha256": fixture_hash,
                        "fixtureAfterSha256": fixture_after,
                        "hostMarkerSha256": marker_hash,
                        "hostMarkerAfterSha256": marker_after,
                        "hostMarkerPath": str(host_marker.resolve()),
                        "executionSecondsLimit": EXECUTION_TIMEOUT,
                        "cleanupSecondsLimit": CLEANUP_TIMEOUT,
                        "totalSecondsLimit": TOTAL_TIMEOUT,
                        "containerCleanup": cleanup,
                        "commands": commands,
                        "startedAt": started_at,
                        "endedAt": utc_now(),
                        "elapsedSeconds": time.monotonic() - start_clock,
                        "limits": {"stdoutBytesEach": MAX_OUTPUT, "stderrBytesEach": MAX_OUTPUT},
                        "requestedMountPolicy": "no Docker socket, home, kubeconfig, or credentials are mounted; one fixture bind only",
                        "error": error})
        receipt_written = False
        try:
            _write(out / "receipt.json", (json.dumps(receipt, sort_keys=True, indent=2) + "\n").encode())
            receipt_written = True
        except Exception:
            receipt_written = False
    return 0 if receipt_written and receipt.get("status") == "passed" else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required to create and remove one owned container")
    parser.add_argument("--docker-binary", type=Path, required=True)
    parser.add_argument("--docker-context", required=True)
    parser.add_argument("--image-id", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.execute:
        parser.error("--execute is required; this helper creates and removes one owned container")
    try:
        return capture(args.docker_binary, args.docker_context, args.image_id, args.output_dir)
    except CaptureError as exc:
        parser.exit(1, "container isolation capture failed: " + str(exc) + "\n")


if __name__ == "__main__":
    raise SystemExit(main())
