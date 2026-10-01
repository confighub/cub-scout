#!/usr/bin/env python3
"""Capture bounded RUL-03 explicit-context evidence from two owned kind clusters."""
from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import signal
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from datetime import datetime, timezone
from typing import Callable

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
INV04_PATH = HERE.parent / "inv04-rbac" / "capture.py"
spec = importlib.util.spec_from_file_location("rul03_inv04_capture", INV04_PATH)
inv04 = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(inv04)

KIND_VERSION = inv04.KIND_VERSION
NODE_IMAGE = inv04.NODE_IMAGE
API_PATH = "/apis/apps/v1/namespaces/rul03-proof/deployments"
NAMESPACE = "rul03-proof"
SERVICE_ACCOUNT = "rul03-observer"
DEPLOYMENT = "rul03-probe"
DENIED_CONTEXT = "rul03-denied"
READABLE_CONTEXT = "rul03-readable"
MAX_HTTP_BODY = 4 * 1024 * 1024
MAX_COMMAND_OUTPUT = 2 * 1024 * 1024
COMMAND_TIMEOUT = 45
OVERALL_TIMEOUT = 240
CLEANUP_TIMEOUT = 90
CREATED_OUTPUTS: set[Path] = set()


class CaptureError(RuntimeError):
    pass


class CaptureInterrupted(CaptureError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def owned_cluster_name(role: str, timestamp: str | None = None, nonce: str | None = None) -> str:
    if role not in ("denied", "readable"):
        raise CaptureError("cluster role is outside the capture plan")
    timestamp = timestamp or datetime.now(timezone.utc).strftime("%y%m%d%H%M%S")
    nonce = nonce or uuid.uuid4().hex[:8]
    name = f"rul03-{role[:3]}-{timestamp}-{nonce}"
    if len(name + "-control-plane") > 63 or not re.fullmatch(r"rul03-[a-z0-9-]+", name):
        raise CaptureError("generated kind node name is invalid")
    return name


def owns_cluster(marker: dict, cluster_name: str, owner_pid: int) -> bool:
    return (isinstance(marker, dict) and marker.get("schema") == "rul03-owned-clusters.v1"
            and marker.get("ownerPid") == owner_pid and cluster_name in marker.get("clusterNames", [])
            and re.fullmatch(r"rul03-[a-z0-9-]+", cluster_name) is not None
            and len(cluster_name + "-control-plane") <= 63)


def build_manifests(readable: bool) -> bytes:
    """Build the same namespace, observer and workload; add a Role only to readable."""
    docs = [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NAMESPACE}},
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": SERVICE_ACCOUNT, "namespace": NAMESPACE}},
        {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": DEPLOYMENT, "namespace": NAMESPACE,
         "labels": {"app": DEPLOYMENT}}, "spec": {"replicas": 1, "selector": {"matchLabels": {"app": DEPLOYMENT}},
         "template": {"metadata": {"labels": {"app": DEPLOYMENT}}, "spec": {"containers": [
             {"name": DEPLOYMENT, "image": "registry.k8s.io/pause:3.10"}]}}}},
    ]
    if readable:
        docs.extend([
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
             "metadata": {"name": "rul03-deployment-reader", "namespace": NAMESPACE},
             "rules": [{"apiGroups": ["apps"], "resources": ["deployments"], "verbs": ["get", "list"]}]},
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
             "metadata": {"name": "rul03-deployment-reader", "namespace": NAMESPACE},
             "subjects": [{"kind": "ServiceAccount", "name": SERVICE_ACCOUNT, "namespace": NAMESPACE}],
             "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "rul03-deployment-reader"}},
        ])
    return ("\n---\n".join(json.dumps(doc, sort_keys=True, separators=(",", ":")) for doc in docs) + "\n").encode()


def _local_server(server: str) -> bool:
    parsed = urllib.parse.urlparse(server)
    return (parsed.scheme == "https" and parsed.hostname in ("127.0.0.1", "localhost", "::1")
            and not parsed.username and not parsed.password and not parsed.path and not parsed.query and not parsed.fragment)


def build_observer_kubeconfig(endpoints: dict[str, dict[str, str]]) -> bytes:
    """Make one minimal two-context config; its default is deliberately readable."""
    if set(endpoints) != {DENIED_CONTEXT, READABLE_CONTEXT}:
        raise CaptureError("observer config requires exactly the denied and readable contexts")
    clusters, users, contexts = [], [], []
    servers = set()
    cas = set()
    for context_name in (DENIED_CONTEXT, READABLE_CONTEXT):
        values = endpoints[context_name]
        server, ca_data, token = values.get("server"), values.get("caData"), values.get("token")
        if not isinstance(server, str) or not _local_server(server) or not ca_data or not token or any(ch.isspace() for ch in token):
            raise CaptureError("observer endpoint is incomplete or not loopback TLS")
        try:
            base64.b64decode(ca_data, validate=True)
        except (ValueError, TypeError):
            raise CaptureError("observer CA data is malformed") from None
        if server in servers:
            raise CaptureError("denied and readable contexts must bind to distinct API servers")
        ca_hash = sha256(base64.b64decode(ca_data))
        if ca_hash in cas:
            raise CaptureError("denied and readable contexts must have distinct API-server CA identities")
        servers.add(server)
        cas.add(ca_hash)
        cluster_name = context_name + "-server"
        user_name = context_name + "-observer"
        clusters.append({"name": cluster_name, "cluster": {"server": server, "certificate-authority-data": ca_data}})
        users.append({"name": user_name, "user": {"token": token}})
        contexts.append({"name": context_name, "context": {"cluster": cluster_name, "user": user_name, "namespace": NAMESPACE}})
    config = {"apiVersion": "v1", "kind": "Config", "current-context": READABLE_CONTEXT,
              "clusters": clusters, "users": users, "contexts": contexts}
    return (json.dumps(config, sort_keys=True, separators=(",", ":")) + "\n").encode()


def resolve_explicit_context(config: bytes | dict, context_name: str) -> tuple[str, bytes, str]:
    """Resolve only the explicitly named context; never consult current-context."""
    if context_name not in (DENIED_CONTEXT, READABLE_CONTEXT):
        raise CaptureError("explicit context is outside the capture plan")
    try:
        data = json.loads(config) if isinstance(config, bytes) else config
        if not isinstance(data, dict):
            raise TypeError
        contexts, clusters, users = data["contexts"], data["clusters"], data["users"]
        matches = [item for item in contexts if isinstance(item, dict) and item.get("name") == context_name]
        if len(matches) != 1 or not isinstance(matches[0].get("context"), dict):
            raise KeyError
        binding = matches[0]["context"]
        cluster_matches = [item for item in clusters if isinstance(item, dict) and item.get("name") == binding.get("cluster")]
        user_matches = [item for item in users if isinstance(item, dict) and item.get("name") == binding.get("user")]
        if len(cluster_matches) != 1 or len(user_matches) != 1:
            raise KeyError
        cluster = cluster_matches[0]["cluster"]
        user = user_matches[0]["user"]
        server, ca_data, token = cluster["server"], cluster["certificate-authority-data"], user["token"]
        if binding.get("namespace") != NAMESPACE or not _local_server(server):
            raise KeyError
        ca_pem = base64.b64decode(ca_data, validate=True)
        if not token or any(ch.isspace() for ch in token):
            raise KeyError
        return server, ca_pem, token
    except (KeyError, TypeError, ValueError, UnicodeDecodeError, json.JSONDecodeError):
        raise CaptureError("explicit observer context is malformed or missing; no fallback is allowed") from None


def validate_observation(context_name: str, status: int, body: bytes) -> dict:
    """Require a typed Forbidden for denied context and the exact DeploymentList for readable."""
    try:
        obj = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise CaptureError("API response is not valid JSON") from None
    if not isinstance(obj, dict):
        raise CaptureError("API response is not a JSON object")
    if context_name == DENIED_CONTEXT:
        if (status != 403 or obj.get("apiVersion") != "v1" or obj.get("kind") != "Status"
                or obj.get("status") != "Failure" or obj.get("reason") != "Forbidden" or obj.get("code") != 403):
            raise CaptureError("denied context did not return a typed Kubernetes Forbidden Status")
        details = obj.get("details")
        message = obj.get("message")
        if (not isinstance(details, dict) or details.get("kind") != "deployments"
                or not isinstance(message, str)
                or 'cannot list resource "deployments"' not in message
                or 'in API group "apps"' not in message
                or 'in the namespace "rul03-proof"' not in message
                or 'system:serviceaccount:rul03-proof:rul03-observer' not in message):
            raise CaptureError("Forbidden response does not identify the observer's namespaced Deployment List denial")
        return {"httpStatus": status, "result": "forbidden", "itemCount": None}
    if context_name == READABLE_CONTEXT:
        if status != 200 or obj.get("apiVersion") != "apps/v1" or obj.get("kind") != "DeploymentList":
            raise CaptureError("readable context did not return an apps/v1 DeploymentList")
        items = obj.get("items")
        if not isinstance(items, list) or len(items) != 1 or not isinstance(items[0], dict):
            raise CaptureError("readable context must return exactly the authored Deployment")
        metadata = items[0].get("metadata")
        if (not isinstance(metadata, dict) or metadata.get("name") != DEPLOYMENT
                or metadata.get("namespace") != NAMESPACE or not isinstance(metadata.get("uid"), str)
                or not metadata.get("uid") or not isinstance(metadata.get("resourceVersion"), str)
                or not metadata.get("resourceVersion")):
            raise CaptureError("readable DeploymentList has the wrong object identity")
        return {"httpStatus": status, "result": "readable_list", "itemCount": 1,
                "deployment": {"name": metadata["name"], "namespace": metadata["namespace"],
                               "uid": metadata["uid"], "resourceVersion": metadata["resourceVersion"]}}
    raise CaptureError("observation context is outside the capture plan")


def kubectl_context_args(kubeconfig: Path, context_name: str, args: list[str]) -> list[str]:
    if not isinstance(context_name, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+", context_name):
        raise CaptureError("kubectl context must be explicit and valid")
    return ["--kubeconfig", str(kubeconfig), "--context", context_name, *args]


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def fetch_explicit_context(config_bytes: bytes, context_name: str, path: str = API_PATH,
                           timeout: float = 15.0, opener_factory: Callable | None = None) -> tuple[int, bytes, str, float]:
    if path != API_PATH:
        raise CaptureError("API path is outside the RUL-03 allowlist")
    server, ca_pem, token = resolve_explicit_context(config_bytes, context_name)
    request = urllib.request.Request(server.rstrip("/") + path,
                                     headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
    if opener_factory:
        opener = opener_factory()
    else:
        try:
            context = ssl.create_default_context(cadata=ca_pem.decode("ascii"))
        except (UnicodeDecodeError, ssl.SSLError):
            raise CaptureError("observer CA is not valid PEM") from None
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect(),
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
    if len(body) > MAX_HTTP_BODY:
        raise CaptureError("API response exceeded the body limit")
    return status, body, content_type, time.monotonic() - started


def _read_response_body(response, deadline: float) -> bytes:
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


def observer_config_before_after(config_bytes: bytes, expected_context: str) -> str:
    try:
        config = json.loads(config_bytes)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError("private observer kubeconfig is not valid JSON") from None
    if config.get("current-context") != expected_context:
        raise CaptureError("private observer current-context changed")
    return expected_context


def _write(path: Path, data: bytes, mode: int = 0o600) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def fresh_output(path: Path) -> Path:
    candidate = path.expanduser().absolute()
    if candidate.exists() or candidate.is_symlink():
        raise CaptureError("output directory must be new and not a symlink")
    repo = Path(__file__).resolve().parents[2]
    resolved = candidate.resolve()
    if resolved == repo or repo in resolved.parents:
        raise CaptureError("output directory must be outside the repository")
    resolved.mkdir(parents=True, mode=0o700)
    os.chmod(resolved, 0o700)
    if resolved.is_symlink():
        raise CaptureError("output directory must not be a symlink")
    CREATED_OUTPUTS.add(resolved)
    return resolved


def _command(binary: Path, args: list[str], timeout: float, env: dict[str, str], deadline: float | None = None) -> tuple[int, bytes, bytes]:
    if deadline is not None:
        timeout = min(timeout, deadline - time.monotonic())
    if timeout <= 0:
        raise CaptureError("overall capture deadline expired")
    return inv04.run_bounded([str(binary), *[str(arg) for arg in args]], timeout, env, MAX_COMMAND_OUTPUT)


def _tool(path: Path) -> Path:
    candidate = path.expanduser().absolute()
    if candidate.is_symlink():
        raise CaptureError("tool binary must not be a symlink")
    resolved = candidate.resolve(strict=True)
    if not resolved.is_file() or not os.access(resolved, os.X_OK):
        raise CaptureError("tool binary is not executable")
    return resolved


def require_local_docker(docker: Path, env: dict[str, str], deadline: float) -> None:
    if env.get("DOCKER_HOST"):
        raise CaptureError("DOCKER_HOST is set; refusing a potentially remote container engine")
    code, out, _err = _command(docker, ["context", "show"], 10, env, deadline)
    if code:
        raise CaptureError("could not identify local Docker context")
    name = out.decode("utf-8", "replace").strip()
    if not name or not re.fullmatch(r"[A-Za-z0-9_.-]+", name):
        raise CaptureError("Docker context name is malformed")
    code, out, _err = _command(docker, ["context", "inspect", name, "--format", "{{json .Endpoints.docker.Host}}"],
                               10, env, deadline)
    if code:
        raise CaptureError("could not inspect Docker endpoint")
    try:
        endpoint = json.loads(out)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError("Docker endpoint response is malformed") from None
    if not isinstance(endpoint, str) or not endpoint.startswith("unix://"):
        raise CaptureError("refusing a non-local Docker endpoint")


def _json_object(data: bytes, message: str) -> dict:
    try:
        value = json.loads(data)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError(message) from None
    if not isinstance(value, dict):
        raise CaptureError(message)
    return value


def validate_admin_deployment(data: bytes) -> dict:
    obj = _json_object(data, "admin setup check did not return a Deployment object")
    meta = obj.get("metadata")
    if ((obj.get("apiVersion"), obj.get("kind")) != ("apps/v1", "Deployment") or not isinstance(meta, dict)
            or (meta.get("name"), meta.get("namespace")) != (DEPLOYMENT, NAMESPACE)
            or not isinstance(meta.get("uid"), str) or not meta["uid"]
            or not isinstance(meta.get("resourceVersion"), str) or not meta["resourceVersion"]):
        raise CaptureError("admin setup check found no exact authored Deployment")
    return {"name": DEPLOYMENT, "namespace": NAMESPACE, "uid": meta["uid"], "resourceVersion": meta["resourceVersion"]}


def cleanup_owned_clusters(kind: Path, attempts: list[dict], marker: dict, owner_pid: int,
                           env: dict[str, str], runner: Callable = _command,
                           timeout: float = CLEANUP_TIMEOUT) -> dict:
    """Delete only newly generated names, including partial creates, within one cleanup budget."""
    deadline = time.monotonic() + timeout
    errors = []
    names = [item.get("name") for item in attempts if isinstance(item, dict) and item.get("attempted")]
    if not names:
        return {"verified": True, "deleted": [], "errors": []}
    if len(names) != len(set(names)):
        return {"verified": False, "deleted": [], "errors": ["duplicate attempted cluster identity"]}
    if any(not isinstance(name, str) or not owns_cluster(marker, name, owner_pid) for name in names):
        return {"verified": False, "deleted": [], "errors": ["cleanup ownership marker did not match generated cluster names"]}
    config_by_name = {item["name"]: Path(item.get("kubeconfigPath", ""))
                      for item in attempts if isinstance(item, dict) and item.get("attempted")}
    # A partial kind create may leave a container before writing kubeconfig.
    # Recover only within the already-created private directory, never through
    # a default config or a symlink. Empty config is sufficient for deletion.
    try:
        for path in config_by_name.values():
            parent = path.parent
            if (not path.is_absolute() or parent.is_symlink() or not parent.is_dir()
                    or parent.stat().st_uid != os.getuid()
                    or parent.stat().st_mode & 0o077 or path.is_symlink()):
                raise CaptureError("private kubeconfig directory is unavailable")
            if not path.exists():
                _write(path, b'{"apiVersion":"v1","kind":"Config","clusters":[],"users":[],"contexts":[],"current-context":""}\n')
            if not path.is_file():
                raise CaptureError("private kubeconfig is not a regular file")
    except (OSError, CaptureError):
        return {"verified": False, "deleted": [], "errors": ["private kubeconfig unavailable; cleanup refused"]}
    cleanup_env = dict(env)
    cleanup_env["KUBECONFIG"] = str(config_by_name[names[0]])
    try:
        code, out, _err = runner(kind, ["get", "clusters"], 10, cleanup_env, deadline)
        if code:
            raise CaptureError("could not list clusters before cleanup")
        existing = out.decode("utf-8", "replace").splitlines()
    except Exception:
        return {"verified": False, "deleted": [], "errors": ["could not list clusters before cleanup"]}
    deleted = []
    for item in reversed(attempts):
        name = item.get("name") if isinstance(item, dict) else None
        if name not in names or name not in existing:
            continue
        if not owns_cluster(marker, name, owner_pid):
            errors.append("refused to delete cluster without owned marker")
            continue
        try:
            private_config = config_by_name[name]
            delete_env = dict(env, KUBECONFIG=str(private_config))
            code, _out, _err = runner(kind, ["delete", "cluster", "--name", name,
                                               "--kubeconfig", str(private_config)], 30, delete_env, deadline)
            if code:
                errors.append("owned cluster deletion failed: " + name)
            else:
                deleted.append(name)
        except Exception:
            errors.append("owned cluster deletion failed: " + name)
    try:
        code, out, _err = runner(kind, ["get", "clusters"], 10, cleanup_env, deadline)
        if code:
            raise CaptureError("could not verify owned cluster cleanup")
        after = out.decode("utf-8", "replace").splitlines()
        remaining = [name for name in names if name in after]
        if remaining:
            errors.append("owned clusters remain after cleanup: " + ",".join(remaining))
    except Exception:
        errors.append("could not verify owned cluster cleanup")
    return {"verified": not errors, "deleted": deleted, "errors": errors}


def _install_capture_deadline(seconds: int):
    if not hasattr(signal, "setitimer") or seconds <= 0:
        raise CaptureError("platform lacks bounded capture deadline support")
    previous = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGALRM)}
    previous_timer = signal.getitimer(signal.ITIMER_REAL)
    def interrupt(signum, _frame):
        raise CaptureInterrupted("capture interrupted by " + signal.Signals(signum).name)
    for sig in previous:
        signal.signal(sig, interrupt)
    signal.setitimer(signal.ITIMER_REAL, seconds)
    return previous, previous_timer, time.monotonic()


def _disable_capture_signals(state):
    previous, _timer, _started = state
    signal.setitimer(signal.ITIMER_REAL, 0)
    for sig in previous:
        signal.signal(sig, signal.SIG_IGN)


def _restore_capture_signals(state):
    previous, previous_timer, started = state
    for sig, handler in previous.items():
        signal.signal(sig, handler)
    if previous_timer != (0.0, 0.0):
        remaining = max(0.001, previous_timer[0] - (time.monotonic() - started))
        signal.setitimer(signal.ITIMER_REAL, remaining, previous_timer[1])


def _get_server_ca(admin_config: Path, context: str, kubectl: Path, env: dict[str, str], deadline: float) -> tuple[str, str]:
    args = kubectl_context_args(admin_config, context,
        ["config", "view", "--minify", "--flatten", "--raw", "-o", "json"])
    code, stdout, _stderr = _command(kubectl, args, COMMAND_TIMEOUT, env, deadline)
    if code:
        raise CaptureError("could not extract private API endpoint and CA")
    config = _json_object(stdout, "private kubeconfig view was malformed")
    try:
        cluster = config["clusters"][0]["cluster"]
        server, ca_data = cluster["server"], cluster["certificate-authority-data"]
        ca_pem = base64.b64decode(ca_data, validate=True)
    except (KeyError, IndexError, TypeError, ValueError):
        raise CaptureError("private kubeconfig lacks an embedded CA or endpoint") from None
    if not isinstance(server, str) or not _local_server(server) or not ca_pem:
        raise CaptureError("private API endpoint is not loopback TLS")
    return server, ca_data


def _create_observer_token(kubectl: Path, admin: Path, admin_context: str, env: dict[str, str], deadline: float) -> str:
    args = kubectl_context_args(admin, admin_context,
        ["create", "token", SERVICE_ACCOUNT, "-n", NAMESPACE, "--duration=900s"])
    code, stdout, _stderr = _command(kubectl, args, COMMAND_TIMEOUT, env, deadline)
    token = stdout.decode("utf-8", "strict").strip() if code == 0 else ""
    if not token or any(char.isspace() for char in token):
        raise CaptureError("could not create bounded observer token")
    return token


def capture(shared_config: Path, output: Path, kind_binary: Path, kubectl_binary: Path, docker_binary: Path) -> int:
    out = fresh_output(output)
    started = utc_now()
    owner_pid = os.getpid()
    shared_before = sha256(shared_config.read_bytes())
    started_clock = time.monotonic()
    deadline = started_clock + OVERALL_TIMEOUT
    errors: list[str] = []
    operations: list[dict] = []
    observations: list[dict] = []
    attempts: list[dict] = []
    temp = Path(tempfile.mkdtemp(prefix="scout-rul03-private-"))
    os.chmod(temp, 0o700)
    kind_private_config = temp / "kind-private.kubeconfig"
    _write(kind_private_config, b"apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n")
    admin_configs = {role: temp / (role + ".admin.kubeconfig") for role in ("denied", "readable")}
    observer_config = temp / "observer.kubeconfig"
    cluster_names = [owned_cluster_name(role) for role in ("denied", "readable")]
    marker = {"schema": "rul03-owned-clusters.v1", "ownerPid": owner_pid, "clusterNames": cluster_names}
    marker_bytes = (json.dumps(marker, sort_keys=True, indent=2) + "\n").encode()
    private_hashes: dict = {}
    observer_default = READABLE_CONTEXT
    observer_initial_hash = ""
    signal_state = None
    cleanup = {"verified": False, "deleted": [], "errors": []}
    endpoint_inputs: dict[str, dict[str, str]] = {}
    deployments: dict[str, dict] = {}
    source_revision = ""
    tool_hashes: dict[str, str] = {}
    kind_version = ""
    node_digest = ""
    docker_local = False
    try:
        if shared_config.is_symlink() or not shared_config.is_file():
            raise CaptureError("shared kubeconfig must be a regular non-symlink file")
        if time.monotonic() >= deadline:
            raise CaptureError("overall capture deadline expired")
        signal_state = _install_capture_deadline(OVERALL_TIMEOUT)
        git = Path(inv04._command("git"))
        code, stdout, _stderr = _command(git, ["-C", str(Path(__file__).resolve().parents[2]), "rev-parse", "HEAD"], 5, dict(os.environ), deadline)
        if code:
            raise CaptureError("could not determine capture source revision")
        source_revision = stdout.decode("ascii", "replace").strip()
        if not re.fullmatch(r"[0-9a-f]{40,64}", source_revision):
            raise CaptureError("capture source revision is not a full git object ID")
        code, stdout, _stderr = _command(git, ["-C", str(Path(__file__).resolve().parents[2]), "status", "--porcelain"], 5, dict(os.environ), deadline)
        if code or stdout.strip():
            raise CaptureError("source checkout must be clean before capture")
        kind_binary, kubectl_binary, docker_binary = map(_tool, (kind_binary, kubectl_binary, docker_binary))
        tool_hashes = {name: sha256(binary.read_bytes()) for name, binary in
                       (("kind", kind_binary), ("kubectl", kubectl_binary), ("docker", docker_binary))}
        for name, binary in (("kind", kind_binary), ("kubectl", kubectl_binary), ("docker", docker_binary)):
            if not binary.is_file():
                raise CaptureError("required local tool is unavailable: " + name)
        docker_env = dict(os.environ)
        docker_env["KUBECONFIG"] = str(kind_private_config)
        require_local_docker(docker_binary, docker_env, deadline)
        docker_local = True
        code, stdout, _stderr = _command(kind_binary, ["version"], 10, docker_env, deadline)
        kind_version = stdout.decode("utf-8", "replace").strip()
        if code:
            raise CaptureError("could not inspect local kind version")
        inv04.require_kind_version(kind_version)
        code, stdout, _stderr = _command(docker_binary, ["image", "inspect", NODE_IMAGE], 15, docker_env, deadline)
        if code:
            raise CaptureError("pinned kind node image is not locally available")
        node_digest = inv04.verified_node_repo_digest(stdout)
        code, stdout, _stderr = _command(kind_binary, ["get", "clusters"], 15, docker_env, deadline)
        if code:
            raise CaptureError("could not list local kind clusters")
        existing = stdout.decode("utf-8", "replace").splitlines()
        if any(name in existing for name in cluster_names) or len(set(cluster_names)) != 2:
            raise CaptureError("generated cluster name already exists or is duplicated")
        _write(out / "owned-clusters-marker.json", marker_bytes)
        for role, cluster_name in zip(("denied", "readable"), cluster_names):
            admin = admin_configs[role]
            attempt = {"name": cluster_name, "role": role, "attempted": False, "kubeconfigPath": str(admin)}
            attempts.append(attempt)
            manifest_path = out / (role + "-manifests.yaml")
            _write(manifest_path, build_manifests(role == "readable"))
            args = ["create", "cluster", "--name", cluster_name, "--image", NODE_IMAGE,
                    "--kubeconfig", str(admin), "--wait", "75s"]
            attempt["attempted"] = True
            op_start = utc_now()
            cmd_clock = time.monotonic()
            code, out_bytes, err_bytes, create_error = None, b"", b"", None
            try:
                create_env = dict(docker_env, KUBECONFIG=str(admin))
                code, out_bytes, err_bytes = _command(kind_binary, args, 90, create_env, deadline)
            except BaseException as error:
                create_error = error
            if admin.exists() and admin.is_file() and not admin.is_symlink():
                os.chmod(admin, 0o600)
                attempt["adminInitialSha256"] = sha256(admin.read_bytes())
            op_end = utc_now()
            op = {"operation": "create-owned-kind", "role": role, "clusterName": cluster_name,
                  "startedAt": op_start, "endedAt": op_end, "elapsedSeconds": time.monotonic() - cmd_clock,
                  "exitCode": code, "stdoutFile": role + "-kind-create.stdout.txt",
                  "stderrFile": role + "-kind-create.stderr.txt", "stdoutSha256": sha256(out_bytes),
                  "stderrSha256": sha256(err_bytes), "stdoutBytes": len(out_bytes), "stderrBytes": len(err_bytes)}
            if create_error is not None:
                op["errorType"] = type(create_error).__name__
            _write(out / op["stdoutFile"], out_bytes)
            _write(out / op["stderrFile"], err_bytes)
            operations.append(op)
            if create_error is not None:
                raise create_error
            if code:
                raise CaptureError("owned " + role + " kind cluster creation failed (exit " + str(code) + ")")
            if not admin.exists() or admin.is_symlink() or not admin.is_file():
                raise CaptureError("kind did not create the expected private admin kubeconfig")
            os.chmod(admin, 0o600)
            admin_env = dict(docker_env, KUBECONFIG=str(admin))
            admin_context = "kind-" + cluster_name
            manifest_args = kubectl_context_args(admin, admin_context, ["apply", "-f", manifest_path])
            apply_start = utc_now()
            apply_clock = time.monotonic()
            apply_code, apply_stdout, apply_stderr = _command(kubectl_binary, manifest_args, COMMAND_TIMEOUT, admin_env, deadline)
            apply_end = utc_now()
            apply_record = {"operation": "apply-capture-fixture", "role": role, "clusterName": cluster_name,
                "startedAt": apply_start, "endedAt": apply_end, "elapsedSeconds": time.monotonic() - apply_clock,
                "exitCode": apply_code, "stdoutFile": role + "-fixture-apply.stdout.txt",
                "stderrFile": role + "-fixture-apply.stderr.txt", "stdoutSha256": sha256(apply_stdout),
                "stderrSha256": sha256(apply_stderr), "stdoutBytes": len(apply_stdout), "stderrBytes": len(apply_stderr)}
            _write(out / apply_record["stdoutFile"], apply_stdout)
            _write(out / apply_record["stderrFile"], apply_stderr)
            operations.append(apply_record)
            if apply_code:
                raise CaptureError("capture fixture apply failed on " + role + " cluster")
            get_args = kubectl_context_args(admin, admin_context, ["get", "deployment", DEPLOYMENT,
                "-n", NAMESPACE, "-o", "json"])
            get_start = utc_now()
            get_clock = time.monotonic()
            get_code, get_stdout, get_stderr = _command(kubectl_binary, get_args, COMMAND_TIMEOUT, admin_env, deadline)
            get_end = utc_now()
            witness = {"operation": "admin-verify-authored-deployment", "role": role, "clusterName": cluster_name,
                "startedAt": get_start, "endedAt": get_end, "elapsedSeconds": time.monotonic() - get_clock,
                "exitCode": get_code, "stdoutSha256": sha256(get_stdout), "stderrSha256": sha256(get_stderr),
                "stdoutBytes": len(get_stdout), "stderrBytes": len(get_stderr)}
            operations.append(witness)
            if get_code:
                raise CaptureError("admin could not verify fixture Deployment on " + role + " cluster")
            deployments[role] = validate_admin_deployment(get_stdout)
            server, ca_data = _get_server_ca(admin, admin_context, kubectl_binary, admin_env, deadline)
            token = _create_observer_token(kubectl_binary, admin, admin_context, admin_env, deadline)
            context_name = DENIED_CONTEXT if role == "denied" else READABLE_CONTEXT
            endpoint_inputs[context_name] = {"server": server, "caData": ca_data, "token": token}
        if not all(name in endpoint_inputs for name in (DENIED_CONTEXT, READABLE_CONTEXT)):
            raise CaptureError("both API endpoints were not prepared")
        if endpoint_inputs[DENIED_CONTEXT]["server"] == endpoint_inputs[READABLE_CONTEXT]["server"]:
            raise CaptureError("two clusters resolved to the same API-server endpoint")
        observer_bytes = build_observer_kubeconfig(endpoint_inputs)
        _write(observer_config, observer_bytes)
        os.chmod(observer_config, 0o600)
        observer_initial_hash = sha256(observer_bytes)
        observer_default = observer_config_before_after(observer_bytes, READABLE_CONTEXT)
        _write(out / "observer-context-map.json", (json.dumps({
            "schema": "rul03-observer-context-map.v1", "currentContext": observer_default,
            "contexts": {context: {"server": values["server"], "caSha256": sha256(base64.b64decode(values["caData"]))}
                         for context, values in endpoint_inputs.items()},
        }, sort_keys=True, indent=2) + "\n").encode())
        for context_name in (DENIED_CONTEXT, READABLE_CONTEXT):
            raw_name = context_name + "-deployments.body"
            req_start = utc_now()
            status, body, content_type, elapsed, request_error = 0, b"", "", 0.0, None
            try:
                status, body, content_type, elapsed = fetch_explicit_context(observer_bytes, context_name)
            except CaptureInterrupted:
                raise
            except CaptureError as error:
                request_error = str(error)
            _write(out / raw_name, body)
            validation = None
            if request_error is None:
                try:
                    validation = validate_observation(context_name, status, body)
                except CaptureError as error:
                    request_error = str(error)
            observations.append({"context": context_name, "explicitContext": True, "currentContext": observer_default,
                "method": "GET", "path": API_PATH, "startedAt": req_start, "endedAt": utc_now(),
                "elapsedSeconds": elapsed, "httpStatus": status, "contentType": content_type,
                "rawFile": raw_name, "rawBytes": len(body), "rawSha256": sha256(body),
                "validation": validation, "error": request_error})
            if request_error:
                errors.append("API observation incomplete for " + context_name)
        observer_after = observer_config.read_bytes()
        observer_context_after = observer_config_before_after(observer_after, observer_default)
        if sha256(observer_after) != observer_initial_hash or observer_context_after != observer_default:
            errors.append("private observer kubeconfig or current-context changed during explicit queries")
    except BaseException as error:
        message = str(error) if isinstance(error, CaptureError) else type(error).__name__
        errors.append(message[:400])
    finally:
        if signal_state is not None:
            _disable_capture_signals(signal_state)
        cleanup_env = dict(os.environ)
        private_hashes["adminBeforeCleanup"] = {}
        try:
            shared_before_cleanup = sha256(shared_config.read_bytes())
        except OSError:
            shared_before_cleanup = ""
            errors.append("shared kubeconfig could not be read before cleanup")
        for role, path in admin_configs.items():
            if path.is_file() and not path.is_symlink():
                current_hash = sha256(path.read_bytes())
                private_hashes.setdefault("adminInitial", {})
                private_hashes["adminBeforeCleanup"][role] = current_hash
                matching_attempt = next((item for item in attempts if item["role"] == role), {})
                if matching_attempt.get("adminInitialSha256") and matching_attempt["adminInitialSha256"] != current_hash:
                    errors.append("private " + role + " admin kubeconfig changed before cleanup")
                if matching_attempt.get("adminInitialSha256"):
                    private_hashes["adminInitial"][role] = matching_attempt["adminInitialSha256"]
        if observer_config.is_file() and not observer_config.is_symlink():
            private_hashes["observerInitial"] = observer_initial_hash
            private_hashes["observerBeforeCleanup"] = sha256(observer_config.read_bytes())
            try:
                private_hashes["observerCurrentContextBeforeCleanup"] = observer_config_before_after(observer_config.read_bytes(), observer_default)
            except CaptureError:
                errors.append("private observer current-context did not remain readable default")
        cleanup = cleanup_owned_clusters(kind_binary, attempts, marker, owner_pid, cleanup_env,
                                          runner=_command, timeout=CLEANUP_TIMEOUT)
        errors.extend(cleanup["errors"])
        try:
            shared_after = sha256(shared_config.read_bytes())
        except OSError:
            shared_after = ""
            errors.append("shared kubeconfig could not be read after cleanup")
        private_hashes["adminAfterCleanup"] = {}
        for role, path in admin_configs.items():
            if path.is_file() and not path.is_symlink():
                private_hashes["adminAfterCleanup"][role] = sha256(path.read_bytes())
        if observer_config.is_file() and not observer_config.is_symlink():
            private_hashes["observerAfterCleanup"] = sha256(observer_config.read_bytes())
        shutil.rmtree(temp, ignore_errors=True)
        if signal_state is not None:
            _restore_capture_signals(signal_state)
    ended = utc_now()
    try:
        shared_after_value = locals().get("shared_after", "")
        provenance = {
            "schema": "rul03-context-capture.v1", "captureSourceRevision": source_revision,
            "captureScriptSha256": sha256(Path(__file__).read_bytes()), "kindVersion": kind_version,
            "nodeImage": NODE_IMAGE, "nodeImageRepoDigest": node_digest, "toolBinarySha256": tool_hashes,
            "dockerContextLocalUnixEndpoint": docker_local, "startedAt": started, "endedAt": ended,
            "elapsedSeconds": time.monotonic() - started_clock, "overallLimitSeconds": OVERALL_TIMEOUT,
            "atomicSnapshot": False, "clusterNames": cluster_names,
            "apiServerIdentities": {context: {"server": values["server"], "caSha256": sha256(base64.b64decode(values["caData"]))}
                                     for context, values in endpoint_inputs.items()},
            "observerCurrentContext": {"before": observer_default, "after": observer_context_after if "observer_context_after" in locals() else ""},
            "deploymentsVerifiedByAdmin": deployments, "operations": operations, "observations": observations,
            "privateKubeconfigSha256": private_hashes,
            "sharedKubeconfigSha256": {"before": shared_before, "beforeCleanup": shared_before_cleanup, "after": shared_after_value,
                                       "unchanged": bool(shared_after_value and shared_before == shared_after_value)},
            "cleanup": cleanup, "errors": errors,
            "exclusions": ["kubeconfig contents", "bearer tokens", "admin client keys/certificates", "Secrets"],
            "limits": ["two owned kind clusters and one explicit namespaced Deployment List request per context",
                       "sequential observations are not an atomic snapshot", "denied inventory remains unknown",
                       "no cross-cluster aggregation, ownership, workload health, or savings claim"],
        }
        _write(out / "provenance.json", (json.dumps(provenance, sort_keys=True, indent=2) + "\n").encode())
    except Exception as error:
        errors.append("could not write final provenance: " + type(error).__name__)
    if errors or not cleanup.get("verified") or not locals().get("shared_after", "") or shared_before != shared_after:
        return 1
    if len(observations) != 2 or [item.get("validation", {}).get("result") if item.get("validation") else None for item in observations] != ["forbidden", "readable_list"]:
        return 1
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required acknowledgement for two owned kind clusters")
    parser.add_argument("--shared-kubeconfig", type=Path, required=True, help="existing file hashed before/after; never used for cluster calls")
    parser.add_argument("--kind-binary", type=Path, required=True)
    parser.add_argument("--kubectl-binary", type=Path, required=True)
    parser.add_argument("--docker-binary", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.execute:
        parser.error("--execute is required; this helper creates and deletes two owned clusters")
    try:
        shared = args.shared_kubeconfig.expanduser().absolute()
        if shared.is_symlink() or not shared.is_file():
            raise CaptureError("shared kubeconfig must be a regular non-symlink file")
        return capture(shared.resolve(strict=True), args.output_dir, args.kind_binary, args.kubectl_binary, args.docker_binary)
    except (CaptureError, OSError, ValueError) as error:
        candidate = args.output_dir.expanduser().absolute().resolve()
        if candidate in CREATED_OUTPUTS and not (candidate / "provenance.json").exists():
            try:
                _write(candidate / "preflight-failure.json", (json.dumps({
                    "schema": "rul03-context-preflight-failure.v1", "error": str(error)[:400],
                    "recordedAt": utc_now()}, sort_keys=True, indent=2) + "\n").encode())
            except OSError:
                pass
        print(str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
