#!/usr/bin/env python3
"""Prepare bounded, owned-cluster INV-04 evidence; never run without --execute."""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import signal
import ssl
import subprocess
import sys
import tempfile
import time
import uuid
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from typing import Callable


NODE_IMAGE = "kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661"
KIND_VERSION = "v0.31.0"
PRODUCT_BINARY_SHA256 = "9b36a5b571499c9798ebf1440c7f3d68fd10d513f66920339d6f32e375a005d3"
PRODUCT_BINARY_COMMIT = "a99d7d4387740fd2c9ccb183f81ea8004c2e7206"
NAMESPACES = ("inv04-readable-populated", "inv04-readable-empty", "inv04-denied")
MAX_COMMAND_OUTPUT = 2 * 1024 * 1024
MAX_HTTP_BODY = 4 * 1024 * 1024
COMMAND_TIMEOUT = 45
CREATED_OUTPUTS: set[Path] = set()


class CaptureError(RuntimeError):
    pass


class CaptureInterrupted(CaptureError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def owned_cluster_name() -> str:
    # kind appends "-control-plane"; keep the entire DNS label below 64 bytes.
    name = "scout-inv04-rbac-" + datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S") + "-" + uuid.uuid4().hex[:12]
    if len(name + "-control-plane") > 63 or not re.fullmatch(r"scout-inv04-rbac-[0-9a-z-]+", name):
        raise CaptureError("generated node name is invalid")
    return name


def build_manifests() -> bytes:
    """Literal low-risk workload and namespace-scoped read-only observer setup."""
    docs = [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": ns}}
        for ns in NAMESPACES
    ]
    docs.extend([
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "inv04-observer", "namespace": NAMESPACES[0]}},
    ])
    for ns in NAMESPACES[:2]:
        docs.extend([
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
             "metadata": {"name": "inv04-deployment-reader", "namespace": ns},
             "rules": [{"apiGroups": ["apps"], "resources": ["deployments"], "verbs": ["get", "list"]}]},
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
             "metadata": {"name": "inv04-deployment-reader", "namespace": ns},
             "subjects": [{"kind": "ServiceAccount", "name": "inv04-observer", "namespace": NAMESPACES[0]}],
             "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "inv04-deployment-reader"}},
        ])
    for name in ("inv04-api", "inv04-worker"):
        docs.append({
            "apiVersion": "apps/v1", "kind": "Deployment",
            "metadata": {"name": name, "namespace": NAMESPACES[0], "labels": {"app": name}},
            "spec": {"replicas": 1, "selector": {"matchLabels": {"app": name}},
                     "template": {"metadata": {"labels": {"app": name}},
                                  "spec": {"containers": [{"name": name, "image": "registry.k8s.io/pause:3.10"}]}}},
        })
    return ("\n---\n".join(json.dumps(d, sort_keys=True, separators=(",", ":")) for d in docs) + "\n").encode()


def observer_kubeconfig(server: str, ca_data: str, token: str) -> bytes:
    """Build a minimal config that contains no admin cert/key material."""
    if not server.startswith("https://") or not token or not ca_data:
        raise CaptureError("observer connection material is incomplete")
    parsed = urllib.parse.urlparse(server)
    if (parsed.hostname not in ("127.0.0.1", "localhost", "::1") or parsed.username or parsed.password
            or parsed.path or parsed.query or parsed.fragment):
        raise CaptureError("refusing a non-local Kubernetes API endpoint")
    config = {
        "apiVersion": "v1", "kind": "Config", "current-context": "inv04-observer",
        "clusters": [{"name": "inv04-owned", "cluster": {"server": server, "certificate-authority-data": ca_data}}],
        "users": [{"name": "inv04-observer", "user": {"token": token}}],
        "contexts": [{"name": "inv04-observer", "context": {"cluster": "inv04-owned", "user": "inv04-observer", "namespace": NAMESPACES[0]}}],
    }
    return (json.dumps(config, sort_keys=True, separators=(",", ":")) + "\n").encode()


def validate_api_response(scope: str, status: int, body: bytes) -> dict:
    """Validate identity and preserve the distinction between empty and denied."""
    try:
        data = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise CaptureError("API response is not valid JSON") from None
    if not isinstance(data, dict):
        raise CaptureError("API response is not a JSON object")
    if scope == NAMESPACES[0]:
        if status != 200 or data.get("apiVersion") != "apps/v1" or data.get("kind") != "DeploymentList":
            raise CaptureError("populated namespace did not return a DeploymentList")
        items = data.get("items")
        if not isinstance(items, list) or any(not isinstance(item, dict) for item in items):
            raise CaptureError("populated namespace response lacks items list")
        metadata = [(item.get("metadata") or {}) for item in items]
        if any(not isinstance(meta, dict) or not isinstance(meta.get("name"), str) for meta in metadata):
            raise CaptureError("populated namespace response has malformed Deployment identity")
        names = sorted(meta["name"] for meta in metadata)
        if names != ["inv04-api", "inv04-worker"]:
            raise CaptureError("populated namespace response has unexpected Deployment identities")
        for meta in metadata:
            if meta.get("namespace") != scope or not meta.get("uid") or not meta.get("resourceVersion"):
                raise CaptureError("populated Deployment lacks exact namespace/UID/resourceVersion")
        return {"httpStatus": status, "result": "readable_populated", "items": len(items)}
    if scope == NAMESPACES[1]:
        if status != 200 or data.get("apiVersion") != "apps/v1" or data.get("kind") != "DeploymentList" or data.get("items") != []:
            raise CaptureError("empty namespace must be a successful empty DeploymentList")
        if not data.get("metadata", {}).get("resourceVersion"):
            raise CaptureError("empty DeploymentList lacks resourceVersion")
        return {"httpStatus": status, "result": "readable_empty", "items": 0}
    if scope == NAMESPACES[2]:
        if status != 403 or data.get("kind") != "Status" or data.get("reason") != "Forbidden" or data.get("code") != 403:
            raise CaptureError("denied namespace must return an actual Kubernetes Forbidden Status")
        return {"httpStatus": status, "result": "denied", "items": None}
    raise CaptureError("unexpected namespace scope")


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def fetch_raw_list(server: str, ca_pem: bytes, token: str, namespace: str,
                   timeout: float = 15.0, opener_factory: Callable | None = None) -> tuple[int, bytes, str, float]:
    """GET one bounded namespaced List; return the exact response body including HTTP errors."""
    if namespace not in NAMESPACES:
        raise CaptureError("namespace is outside the capture plan")
    url = server.rstrip("/") + "/apis/apps/v1/namespaces/" + namespace + "/deployments"
    request = urllib.request.Request(url, headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
    if opener_factory:
        opener = opener_factory()
    else:
        context = ssl.create_default_context(cadata=ca_pem.decode("ascii"))
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect,
                                             urllib.request.HTTPSHandler(context=context))
    started = time.monotonic()
    deadline = started + timeout
    try:
        with opener.open(request, timeout=timeout) as response:
            status = response.status
            content_type = response.headers.get("Content-Type", "")
            body = _read_response_body(response, deadline)
    except urllib.error.HTTPError as error:
        status = error.code
        content_type = error.headers.get("Content-Type", "")
        try:
            body = _read_response_body(error, deadline)
        finally:
            error.close()
    except CaptureError:
        raise
    except Exception as error:
        raise CaptureError("bounded API request failed: " + type(error).__name__) from None
    elapsed = time.monotonic() - started
    if len(body) > MAX_HTTP_BODY:
        raise CaptureError("API response exceeded the body limit")
    return status, body, content_type, elapsed


def _read_response_body(response, deadline: float) -> bytes:
    """Read in single-recv chunks so a peer cannot extend the total deadline by trickling."""
    data = bytearray()
    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise CaptureError("API response exceeded the request deadline")
        fp = getattr(response, "fp", None)
        raw = getattr(fp, "raw", None)
        sock = getattr(raw, "_sock", None)
        if sock is not None:
            sock.settimeout(remaining)
        reader = getattr(response, "read1", None) or response.read
        chunk = reader(min(65536, MAX_HTTP_BODY + 1 - len(data)))
        if not chunk:
            break
        data.extend(chunk)
        if len(data) > MAX_HTTP_BODY:
            raise CaptureError("API response exceeded the body limit")
    return bytes(data)


def run_bounded(args: list[str], timeout: float, env: dict[str, str] | None = None,
                max_output: int = MAX_COMMAND_OUTPUT) -> tuple[int, bytes, bytes]:
    """Run a command in its own process group with bounded output and teardown."""
    proc = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            env=env, start_new_session=True)
    assert proc.stdout and proc.stderr
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
    selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
    outputs = {"stdout": bytearray(), "stderr": bytearray()}
    deadline = time.monotonic() + timeout
    try:
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise CaptureError("command timed out: " + Path(args[0]).name)
            for key, _ in selector.select(min(remaining, 0.25)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                target = outputs[key.data]
                if len(target) + len(chunk) > max_output:
                    raise CaptureError("command output exceeded limit: " + Path(args[0]).name)
                target.extend(chunk)
            if proc.poll() is not None and not selector.get_map():
                break
        returncode = proc.wait()
        # A helper may exit while leaving descendants that have closed our pipes.
        # The whole command process group is ours, so do not let those children escape.
        _terminate_group(proc.pid, proc, grace=0.1)
        return returncode, bytes(outputs["stdout"]), bytes(outputs["stderr"])
    except BaseException:
        _terminate_group(proc.pid, proc)
        raise
    finally:
        selector.close()
        for stream in (proc.stdout, proc.stderr):
            try:
                stream.close()
            except OSError:
                pass


def _terminate_group(pgid: int, proc: subprocess.Popen, grace: float = 2.0) -> None:
    try:
        os.killpg(pgid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    deadline = time.monotonic() + grace
    while time.monotonic() < deadline:
        proc.poll()
        try:
            os.killpg(pgid, 0)
        except ProcessLookupError:
            break
        time.sleep(0.025)
    else:
        try:
            os.killpg(pgid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    try:
        proc.wait(timeout=2)
    except subprocess.TimeoutExpired:
        pass


def _arm_capture_signals():
    if not hasattr(signal, "setitimer"):
        raise CaptureError("platform lacks bounded capture deadline support")
    prior = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGALRM)}
    prior_timer = signal.getitimer(signal.ITIMER_REAL)
    def interrupt(signum, _frame):
        raise CaptureInterrupted("capture interrupted by " + signal.Signals(signum).name)
    for sig in prior:
        signal.signal(sig, interrupt)
    signal.setitimer(signal.ITIMER_REAL, 12 * 60)
    return prior, prior_timer, time.monotonic()


def _suppress_capture_signals(state) -> None:
    prior, _prior_timer, _started = state
    signal.setitimer(signal.ITIMER_REAL, 0)
    for sig in prior:
        signal.signal(sig, signal.SIG_IGN)


def _restore_capture_signals(state) -> None:
    prior, prior_timer, started = state
    for sig, handler in prior.items():
        signal.signal(sig, handler)
    if prior_timer != (0.0, 0.0):
        delay = max(0.001, prior_timer[0] - (time.monotonic() - started))
        signal.setitimer(signal.ITIMER_REAL, delay, prior_timer[1])


def require_kind_version(version: str) -> None:
    fields = version.split()
    if len(fields) < 2 or fields[0] != "kind" or fields[1] != KIND_VERSION:
        raise CaptureError("kind version does not match pinned capture requirement")


def verified_node_repo_digest(image_inspect: bytes) -> str:
    try:
        inspected = json.loads(image_inspect)
        digests = inspected[0]["RepoDigests"]
    except (json.JSONDecodeError, KeyError, IndexError, TypeError):
        raise CaptureError("pinned kind node image lacks repository digest metadata") from None
    expected = NODE_IMAGE.rsplit("@", 1)[1]
    for ref in digests or []:
        if isinstance(ref, str) and "@" in ref:
            repository, digest = ref.rsplit("@", 1)
            if (repository == "kindest/node" or repository.endswith("/kindest/node")) and digest == expected:
                return ref
    raise CaptureError("local kind node image does not expose the pinned repository digest")


def raw_observations_complete(records: list[dict]) -> bool:
    expected = ["readable_populated", "readable_empty", "denied"]
    results = [((record.get("validation") or {}).get("result")) for record in records]
    return len(records) == 3 and results == expected and not any(record.get("error") for record in records)


def _command(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise CaptureError("required executable not found: " + name)
    return path


def require_local_docker(env: dict[str, str]) -> None:
    if env.get("DOCKER_HOST"):
        raise CaptureError("DOCKER_HOST is set; refusing a potentially remote container engine")
    context_name = _call("docker", ["context", "show"], 10, env).decode("utf-8", "replace").strip()
    if not context_name or not re.fullmatch(r"[A-Za-z0-9_.-]+", context_name):
        raise CaptureError("could not identify the selected Docker context")
    endpoint = _call("docker", ["context", "inspect", context_name,
                                 "--format", "{{json .Endpoints.docker.Host}}"], 10, env)
    try:
        docker_host = json.loads(endpoint)
    except json.JSONDecodeError:
        raise CaptureError("could not validate the selected Docker endpoint") from None
    if not isinstance(docker_host, str) or not docker_host.startswith("unix://"):
        raise CaptureError("refusing a non-local Docker endpoint")


def _call(name: str, args: list[str], timeout: float, env: dict[str, str] | None = None) -> bytes:
    code, out, _err = run_bounded([_command(name), *args], timeout, env)
    if code != 0:
        # Never echo argv, stdout or stderr: those may contain private credentials.
        raise CaptureError("command failed: " + name + " (exit " + str(code) + ")")
    return out


def _write(path: Path, data: bytes, mode: int = 0o600) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def _hash_file_or_empty(path: Path, errors: list[str], label: str) -> str:
    try:
        return sha256(path.read_bytes()) if path.is_file() else ""
    except OSError:
        errors.append(label + " could not be hashed")
        return ""


def _fresh_output(path: Path) -> Path:
    path = path.expanduser().absolute()
    if path.exists() or path.is_symlink():
        raise CaptureError("output directory already exists")
    path = path.resolve()
    repo = Path(__file__).resolve().parents[2]
    if path == repo or repo in path.parents:
        raise CaptureError("output directory must be outside the repository")
    path.mkdir(parents=True, mode=0o700)
    os.chmod(path, 0o700)
    if path.is_symlink():
        raise CaptureError("output directory must not be a symlink")
    CREATED_OUTPUTS.add(path)
    return path


def capture(shared_config: Path, binary: Path, output: Path, after_observation: Callable | None = None) -> int:
    out = _fresh_output(output)
    started = utc_now()
    shared_before = sha256(shared_config.read_bytes())
    repo = Path(__file__).resolve().parents[2]
    git = _command("git")
    revision_code, revision_bytes, _ = run_bounded([git, "-C", str(repo), "rev-parse", "HEAD"], 5, max_output=4096)
    if revision_code:
        raise CaptureError("could not determine source revision")
    source_revision = revision_bytes.decode("ascii", "replace").strip()
    if not re.fullmatch(r"[0-9a-f]{40,64}", source_revision):
        raise CaptureError("source revision is not a full git object ID")
    status_code, status_bytes, _ = run_bounded([git, "-C", str(repo), "status", "--porcelain"], 5, max_output=16384)
    if status_code or status_bytes.strip():
        raise CaptureError("source checkout must be clean before capture")
    preflight_env = dict(os.environ)
    require_local_docker(preflight_env)
    kind = _call("kind", ["version"], 10).decode("utf-8", "replace").strip()
    require_kind_version(kind)
    node_inspect = _call("docker", ["image", "inspect", NODE_IMAGE], 15)
    node_repo_digest = verified_node_repo_digest(node_inspect)
    for name in ("kubectl", "kind", "docker"):
        _command(name)
    binary = binary.expanduser().resolve(strict=True)
    if not os.access(binary, os.X_OK):
        raise CaptureError("cub-scout binary is not executable")
    binary_sha = sha256(binary.read_bytes())
    if binary_sha != PRODUCT_BINARY_SHA256:
        raise CaptureError("cub-scout binary does not match the pinned local build")
    scripts_sha = sha256(Path(__file__).read_bytes())
    cluster = owned_cluster_name()
    temp = Path(tempfile.mkdtemp(prefix="scout-inv04-private-"))
    os.chmod(temp, 0o700)
    admin_config = temp / "admin.kubeconfig"
    observer_config = temp / "observer.kubeconfig"
    env_base = dict(os.environ)
    env_base["KUBECONFIG"] = str(admin_config)
    create_attempted = False
    cleanup_ok = False
    errors: list[str] = []
    records = []
    _write(out / "literal-manifests.yaml", build_manifests())
    admin_initial_hash = ""
    observer_initial_hash = ""
    admin_before_cleanup = ""
    observer_before_cleanup = ""
    admin_after_cleanup = ""
    observer_after_cleanup = ""
    owner_marker_sha = ""
    server_version = {}
    client_version = {}
    signal_state = _arm_capture_signals()
    try:
        existing = _call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
        if cluster in existing:
            raise CaptureError("refusing to reuse an existing kind cluster")
        create_attempted = True
        marker = {"schema": "inv04-owned-cluster-marker.v1", "clusterName": cluster,
                  "ownerPid": os.getpid(), "sourceRevision": source_revision,
                  "privateTempDirectoryName": temp.name,
                  "creationAttemptedAt": utc_now()}
        marker_bytes = (json.dumps(marker, sort_keys=True, indent=2) + "\n").encode()
        _write(out / "owned-cluster-marker.json", marker_bytes)
        owner_marker_sha = sha256(marker_bytes)
        # This command has no token or credential payload in argv/output. Retain
        # kind's setup diagnostics; credential-bearing kubectl calls stay private.
        create_code, create_out, create_err = run_bounded(
            [_command("kind"), "create", "cluster", "--name", cluster, "--image", NODE_IMAGE,
             "--kubeconfig", str(admin_config), "--wait", "120s"], 180, env_base)
        _write(out / "kind-create-stdout.txt", create_out)
        _write(out / "kind-create-stderr.txt", create_err)
        if create_code:
            raise CaptureError("owned kind cluster creation failed (exit " + str(create_code) + ")")
        os.chmod(admin_config, 0o600)
        admin_initial_hash = sha256(admin_config.read_bytes())
        kubectl = _command("kubectl")
        admin_env = dict(env_base, KUBECONFIG=str(admin_config))
        try:
            client_version = json.loads(_call("kubectl", ["version", "--client", "-o", "json"], 15, admin_env))
        except (json.JSONDecodeError, CaptureError):
            raise CaptureError("could not capture kubectl client version") from None
        manifest_path = out / "literal-manifests.yaml"
        code, _stdout, _stderr = run_bounded([kubectl, "--kubeconfig", str(admin_config), "--context", "kind-" + cluster,
            "apply", "-f", str(manifest_path)], 60, admin_env)
        if code:
            raise CaptureError("could not apply capture fixture manifests")
        # Extract only public API endpoint/CA and a short-lived observer token in memory.
        config_raw = _call("kubectl", ["--kubeconfig", str(admin_config), "--context", "kind-" + cluster,
                                        "config", "view", "--minify", "--flatten", "--raw", "-o", "json"], 15, admin_env)
        config = json.loads(config_raw)
        cluster_data = config["clusters"][0]["cluster"]
        server = cluster_data["server"]
        ca64 = cluster_data.get("certificate-authority-data")
        if not ca64:
            raise CaptureError("private cluster config lacks embedded CA")
        ca_pem = base64.b64decode(ca64, validate=True)
        del config_raw, config
        try:
            version_data = json.loads(_call("kubectl", ["--kubeconfig", str(admin_config), "--context", "kind-" + cluster,
                                                         "version", "-o", "json"], 15, admin_env))
            client_version = version_data.get("clientVersion", client_version)
            server_version = version_data.get("serverVersion", {})
        except (json.JSONDecodeError, CaptureError):
            raise CaptureError("could not capture Kubernetes API server version") from None
        token = _call("kubectl", ["--kubeconfig", str(admin_config), "--context", "kind-" + cluster,
                                  "create", "token", "inv04-observer", "-n", NAMESPACES[0], "--duration=900s"], 15, admin_env).decode().strip()
        if not token or any(char.isspace() for char in token):
            raise CaptureError("could not create a bounded observer token")
        _write(observer_config, observer_kubeconfig(server, ca64, token))
        observer_initial_hash = sha256(observer_config.read_bytes())
        os.chmod(observer_config, 0o600)
        for scope in NAMESPACES:
            req_start = utc_now()
            status, body, content_type, elapsed = 0, b"", "", 0.0
            try:
                status, body, content_type, elapsed = fetch_raw_list(server, ca_pem, token, scope)
            except CaptureInterrupted:
                raise
            except CaptureError as error:
                request_error = str(error)
            else:
                request_error = None
            raw_name = scope + ".api-response.json"
            _write(out / raw_name, body)
            validation = None
            if request_error is None:
                try:
                    validation = validate_api_response(scope, status, body)
                except CaptureError as error:
                    request_error = str(error)
            records.append({"namespace": scope, "method": "GET", "path": f"/apis/apps/v1/namespaces/{scope}/deployments",
                            "startedAt": req_start, "endedAt": utc_now(), "httpStatus": status,
                            "contentType": content_type, "elapsedSeconds": elapsed,
                            "rawFile": raw_name, "rawBytes": len(body), "rawSha256": sha256(body),
                            "validation": validation, "error": request_error})
            if request_error:
                errors.append("API observation incomplete for " + scope)
            # Separate product output; errors are captured without using stdout/stderr as evidence text.
            map_args = ["map", "list", "--namespace", scope, "--kind", "Deployment",
                        "--ownership-evidence", "--format", "json", "--kube-context", "inv04-observer"]
            scout_started = utc_now()
            scout_clock = time.monotonic()
            code, stdout, stderr = run_bounded([str(binary), *map_args], COMMAND_TIMEOUT,
                                               dict(env_base, KUBECONFIG=str(observer_config)))
            scout_elapsed = time.monotonic() - scout_clock
            scout_ended = utc_now()
            if token.encode() in stdout or token.encode() in stderr:
                raise CaptureError("Scout output unexpectedly contained observer credentials")
            map_stdout_name = scope + ".scout-map.json"
            map_stderr_name = scope + ".scout-stderr.txt"
            _write(out / map_stdout_name, stdout)
            _write(out / map_stderr_name, stderr)
            records[-1]["scout"] = {"argv": map_args, "exitCode": code, "stdoutFile": map_stdout_name,
                                     "startedAt": scout_started, "endedAt": scout_ended,
                                     "elapsedSeconds": scout_elapsed,
                                     "stdoutSha256": sha256(stdout), "stdoutBytes": len(stdout),
                                     "stderrFile": map_stderr_name, "stderrSha256": sha256(stderr),
                                     "stderrBytes": len(stderr)}
            if after_observation is not None:
                after_observation(out, scope, observer_config, token, records[-1])
    except BaseException as exc:
        # Keep only safe step-level diagnostics; never serialize command output or credential-bearing config.
        errors.append(str(exc) if isinstance(exc, CaptureError) else type(exc).__name__)
    finally:
        _suppress_capture_signals(signal_state)
        admin_before_cleanup = _hash_file_or_empty(admin_config, errors, "private admin kubeconfig before cleanup")
        observer_before_cleanup = _hash_file_or_empty(observer_config, errors, "private observer kubeconfig before cleanup")
        try:
            if create_attempted:
                listed = _call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
                if cluster in listed:
                    _call("kind", ["delete", "cluster", "--name", cluster, "--kubeconfig", str(admin_config)], 90,
                          dict(env_base, KUBECONFIG=str(admin_config)))
                after = _call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
                cleanup_ok = cluster not in after
            else:
                cleanup_ok = True
        except Exception:
            cleanup_ok = False
        try:
            shared_after = sha256(shared_config.read_bytes())
        except OSError:
            shared_after = ""
            errors.append("shared kubeconfig could not be re-read after capture")
        admin_after_cleanup = _hash_file_or_empty(admin_config, errors, "private admin kubeconfig after cleanup")
        observer_after_cleanup = _hash_file_or_empty(observer_config, errors, "private observer kubeconfig after cleanup")
        if admin_initial_hash and admin_initial_hash != admin_before_cleanup:
            errors.append("private admin kubeconfig changed before cleanup")
        if observer_initial_hash and observer_initial_hash != observer_before_cleanup:
            errors.append("private observer kubeconfig changed before cleanup")
        shutil.rmtree(temp, ignore_errors=True)
        _restore_capture_signals(signal_state)
    final = utc_now()
    if not raw_observations_complete(records):
        errors.append("required populated, empty, and denied raw observations were not all validated")
    provenance = {
        "schema": "inv04-rbac-capture.v1", "sourceRevision": source_revision,
        "captureScriptSha256": scripts_sha, "cubScoutBinarySha256": binary_sha,
        "cubScoutBinaryCommit": PRODUCT_BINARY_COMMIT,
        "literalManifestsSha256": sha256((out / "literal-manifests.yaml").read_bytes()),
        "toolBinarySha256": {name: sha256(Path(_command(name)).read_bytes()) for name in ("kubectl", "kind", "docker")},
        "dockerContextLocalUnixEndpoint": True,
        "kindVersion": kind, "nodeImage": NODE_IMAGE, "nodeImageRepoDigest": node_repo_digest,
        "clusterName": cluster, "ownedClusterMarkerSha256": owner_marker_sha,
        "kubectlClientVersion": client_version, "kubernetesServerVersion": server_version,
        "startedAt": started, "endedAt": final, "atomicSnapshot": False,
        "sequence": ["fixture apply", "observer config", *[
            step for record in records for step in
            ("API GET " + record["namespace"], "cub-scout map list " + record["namespace"])
        ], "owned cluster cleanup"],
        "scopes": list(NAMESPACES), "observations": records,
        "sharedKubeconfigSha256": {"before": shared_before, "after": shared_after, "unchanged": shared_before == shared_after},
        "privateKubeconfigSha256": {
            "adminInitial": admin_initial_hash, "adminBeforeCleanup": admin_before_cleanup,
            "adminAfterCleanup": admin_after_cleanup,
            "adminUnchangedBeforeCleanup": bool(admin_initial_hash and admin_initial_hash == admin_before_cleanup),
            "observerInitial": observer_initial_hash, "observerBeforeCleanup": observer_before_cleanup,
            "observerAfterCleanup": observer_after_cleanup,
            "observerUnchanged": bool(observer_initial_hash and observer_initial_hash == observer_before_cleanup),
        },
        "exclusions": ["kubeconfig contents", "bearer tokens", "client keys/certificates", "Secret objects and payloads"],
        "limitations": ["only the three named namespace Deployment List requests are captured",
                        "no cluster-wide inventory or atomic snapshot is claimed",
                        "a denied response does not establish that the namespace is empty"],
        "cleanupVerified": cleanup_ok, "errors": errors,
    }
    _write(out / "provenance.json", (json.dumps(provenance, indent=2, sort_keys=True) + "\n").encode())
    if errors or not cleanup_ok or shared_before != shared_after:
        raise CaptureError("capture incomplete; inspect retained provenance.json")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required acknowledgement for owned-cluster creation")
    parser.add_argument("--shared-kubeconfig", type=Path, help="existing config hashed before/after; never used for API calls")
    parser.add_argument("--cub-scout-binary", type=Path)
    parser.add_argument("--output-dir", type=Path)
    args = parser.parse_args(argv)
    if not args.execute or not args.shared_kubeconfig or not args.cub_scout_binary or not args.output_dir:
        parser.error("--execute, --shared-kubeconfig, --cub-scout-binary and --output-dir are all required")
    try:
        requested_config = args.shared_kubeconfig.expanduser().absolute()
        if requested_config.is_symlink():
            raise CaptureError("shared kubeconfig must not be a symlink")
        config = requested_config.resolve(strict=True)
        if not config.is_file():
            raise CaptureError("shared kubeconfig must be a regular file")
        return capture(config, args.cub_scout_binary, args.output_dir)
    except CaptureError as error:
        out = args.output_dir.expanduser().absolute().resolve()
        if out in CREATED_OUTPUTS and not (out / "provenance.json").exists():
            try:
                _write(out / "preflight-failure.json", (json.dumps({"schema": "inv04-rbac-preflight-failure.v1",
                    "error": str(error), "recordedAt": utc_now()}, sort_keys=True, indent=2) + "\n").encode())
            except OSError:
                pass
        print(str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
