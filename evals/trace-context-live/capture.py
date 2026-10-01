#!/usr/bin/env python3
"""Prepare an opt-in, serial before/after owned-kind Trace context proof.

No cluster, provider, model, or auth operation occurs unless --execute is given.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import selectors
import shlex
import shutil
import signal
import subprocess
import ssl
import tempfile
import time
import uuid

try:
    from . import api_proxy
except ImportError:  # direct script execution
    import api_proxy

REPO = Path(__file__).resolve().parents[2]
OLD_SOURCE = "8cdb27b0bc9db17f5c0d1b59628b67cd3936ed6c"
FIXED_SOURCE = "24074d858e8e2411ce2ba3e94340ec5646395eed"
# Reviewed product candidate supplied by the root integration. Capture still
# requires this commit to be an ancestor of HEAD, so it remains unrunnable
# from this helper branch until the product commit is integrated here.
COMBINED_SOURCE_PIN = "a508e83840c54843a6461727ca71a417e4ccdbad"
CONFIGHUB_UNIT = "scout-context-unit"
CONFIGHUB_SPACE = "scout-context-space"
NAMESPACE = "scout-trace-context-proof"
DEPLOYMENT = "scout-context-marker"
NATIVE_DEPLOYMENT = "scout-native-marker"
APPLICATION = "scout-context-app"
ALLOWED_CONTEXT = "doctor-allowed"
DENIED_CONTEXT = "doctor-denied"
DENIED_USER = "system:serviceaccount:" + NAMESPACE + ":trace-denied"
NODE_IMAGE = "kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661"
KIND_VERSION = "v0.31.0"
MAX_SECONDS = 600
MAX_OUTPUT = 2 * 1024 * 1024
TUI_SCHEMA = "trace-context-owned-tui.v1"
SOURCE_URL = "https://example.invalid/owned-fixture.git"
TARGET_PATH = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{DEPLOYMENT}"
NATIVE_TARGET_PATH = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{NATIVE_DEPLOYMENT}"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(argv: list[str], *, env: dict[str, str], timeout: float = 90,
        deadline: float | None = None, max_output: int = MAX_OUTPUT,
        input_data: bytes | None = None) -> dict:
    """Run only this command group, retaining bounded partial output on failure."""
    started = time.monotonic()
    end = min(started + timeout, deadline) if deadline is not None else started + timeout
    result = {"argv": argv, "exitCode": None, "stdout": "", "stderr": "",
              "elapsedSeconds": 0, "failure": None}
    buffers = {"stdout": bytearray(), "stderr": bytearray()}
    proc = None
    selector = selectors.DefaultSelector()
    try:
        if end <= started:
            raise TimeoutError("overall proof deadline expired before command")
        proc = subprocess.Popen(argv, env=env,
                                stdin=subprocess.PIPE if input_data is not None else subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                start_new_session=True)
        if input_data is not None:
            assert proc.stdin is not None
            proc.stdin.write(input_data)
            proc.stdin.close()
        for name in buffers:
            selector.register(getattr(proc, name), selectors.EVENT_READ, name)
        while selector.get_map() or proc.poll() is None:
            remaining = end - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("command deadline expired")
            for key, _ in selector.select(min(remaining, 0.1)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                available = max_output - sum(len(value) for value in buffers.values())
                buffers[key.data].extend(chunk[:max(0, available)])
                if len(chunk) > available:
                    raise RuntimeError("command output exceeded bound")
        result["exitCode"] = proc.wait()
    except Exception as exc:
        result["failure"] = type(exc).__name__ + ": " + str(exc)
    finally:
        if proc is not None:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.wait()
            if result["exitCode"] is None:
                result["exitCode"] = proc.returncode
            for name in buffers:
                getattr(proc, name).close()
        selector.close()
        result["stdout"] = buffers["stdout"].decode("utf-8", "replace")
        result["stderr"] = buffers["stderr"].decode("utf-8", "replace")
        result["elapsedSeconds"] = round(time.monotonic() - started, 3)
    return result


def succeeded(result: dict) -> bool:
    return result.get("exitCode") == 0 and not result.get("failure")


def record_command(receipt: dict, phase: str, argv: list[str], *, env: dict,
                   deadline: float, timeout: float = 90, secret: bool = False) -> dict:
    result = run(argv, env=env, timeout=timeout, deadline=deadline)
    public = dict(result)
    if secret:
        public["argv"] = [argv[0], "<private credential operation>"]
        public["stdout"] = "<redacted>"
        public["stderr"] = "<redacted>"
        if public["failure"]:
            public["failure"] = "private credential operation failed"
    receipt["commands"].append({"phase": phase, **public})
    if not succeeded(result):
        raise RuntimeError("command failed in phase " + phase + "; inspect retained receipt")
    return result


def record_observation(receipt: dict, phase: str, command: str, argv: list[str], *,
                       env: dict, deadline: float) -> dict:
    result = run(argv, env=env, deadline=deadline, timeout=60)
    receipt["commands"].append({"phase": phase, "command": command, **result})
    if result.get("failure"):
        raise RuntimeError("Trace transport failed in phase " + phase)
    return result


def _identity(body: dict, field: str) -> bool:
    value = body.get(field)
    return (isinstance(value, dict) and value.get("kind") == "Deployment" and
            value.get("namespace") == NAMESPACE and value.get("name") == DEPLOYMENT)


def _native_identity(body: dict, field: str) -> bool:
    value = body.get(field)
    return (isinstance(value, dict) and value.get("kind") == "Deployment" and
            value.get("namespace") == NAMESPACE and value.get("name") == NATIVE_DEPLOYMENT)


def validate_observations(records: list[dict]) -> None:
    phases = {"old-ambient-normal", "old-explicit-normal", "fixed-allowed-normal", "fixed-denied-normal",
              "old-ambient-reverse", "old-explicit-reverse", "fixed-allowed-reverse", "fixed-denied-reverse",
              "fixed-allowed-native-reverse", "fixed-denied-native-reverse"}
    rows = [row for row in records if row.get("phase") in phases]
    actual = [row.get("phase") for row in rows]
    expected_command = {phase: "reverse" if "reverse" in phase else "normal" for phase in phases}
    if len(actual) != len(phases) or set(actual) != phases or any(row.get("command") != expected_command[row["phase"]] for row in rows):
        raise RuntimeError("paired CLI observations are incomplete or duplicated")
    for row in rows:
        phase, command = row["phase"], row["command"]
        if row.get("failure") or type(row.get("exitCode")) is not int:
            raise RuntimeError("Trace transport failure is not observation evidence")
        if phase.startswith("old-explicit-"):
            if row["exitCode"] == 0 or "unknown flag: --kube-context" not in row.get("stderr", ""):
                raise RuntimeError("old source did not reject the explicit selector as expected")
            continue
        if phase == "fixed-denied-normal":
            denial = row.get("stderr", "")
            if row["exitCode"] == 0 or "forbidden" not in denial.lower() or DENIED_USER not in denial:
                raise RuntimeError("normal Trace denial lacks exact selected service-account evidence")
            if row.get("stdout", "").strip():
                raise RuntimeError("denied normal Trace unexpectedly returned an observation")
            continue
        try:
            body = json.loads(row["stdout"])
        except (TypeError, json.JSONDecodeError):
            raise RuntimeError(phase + " did not return structured JSON") from None
        if phase == "fixed-denied-reverse":
            error = body.get("error", "")
            if (row["exitCode"] != 0 or body.get("owner") == "native" or
                    "forbidden" not in error.lower() or DENIED_USER not in error or
                    body.get("context") != DENIED_CONTEXT or not _identity(body, "object")):
                raise RuntimeError("reverse Trace denial was mistaken for native/clean output")
            continue
        if phase.startswith("fixed-") and "-native-reverse" in phase:
            try:
                body = json.loads(row["stdout"])
            except (TypeError, json.JSONDecodeError):
                raise RuntimeError(phase + " did not return structured JSON") from None
            if phase == "fixed-allowed-native-reverse":
                if row["exitCode"] != 0 or body.get("context") != ALLOWED_CONTEXT or body.get("owner") != "native" or not _native_identity(body, "object"):
                    raise RuntimeError("allowed native control was not identified as native")
            else:
                error = body.get("error", "")
                if row["exitCode"] != 0 or body.get("owner") == "native" or "forbidden" not in error.lower() or DENIED_USER not in error or body.get("context") != DENIED_CONTEXT or not _native_identity(body, "object"):
                    raise RuntimeError("native access denial was misreported as unmanaged")
            continue
        if row["exitCode"] != 0:
            raise RuntimeError(phase + " exited nonzero without being the denied observation")
        fixed = phase.startswith("fixed-")
        if fixed and body.get("context") != ALLOWED_CONTEXT:
            raise RuntimeError(phase + " did not retain its selected context label")
        if command == "normal":
            if not _identity(body, "target") or body.get("summary", {}).get("ownerType") != "ArgoCD":
                raise RuntimeError(phase + " lost the exact workload or Argo label identity")
            if fixed:
                source = body.get("summary", {}).get("source", {})
                if not isinstance(source, dict) or source.get("url") != SOURCE_URL:
                    raise RuntimeError(phase + " lost the synthetic Application source identity")
        else:
            if not _identity(body, "object") or body.get("owner") != "argo":
                raise RuntimeError(phase + " lost the exact reverse target or Argo label identity")


def validate_tui(data: dict) -> None:
    phases = ("allowed-open", "allowed-reopen-after-retarget", "denied-open")
    if data.get("schema") != TUI_SCHEMA or data.get("passed") is not True:
        raise RuntimeError("TUI probe lacks a successful versioned result")
    checks = data.get("checks", {})
    required_checks = (*phases, "private-config-retarget-stable")
    if any(checks.get(name) is not True for name in required_checks):
        raise RuntimeError("TUI probe checks are incomplete")
    for phase in phases:
        denied = phase == "denied-open"
        selected = DENIED_CONTEXT if denied else ALLOWED_CONTEXT
        if data.get("selectedContexts", {}).get(phase) != selected:
            raise RuntimeError("TUI model used the wrong captured context label")
        requests = data.get("requests", {}).get(phase, [])
        view = data.get("views", {}).get(phase, "")
        if not requests or any(request.get("method") != "GET" for request in requests):
            raise RuntimeError("TUI probe lacks an exclusively read-only request trace")
        target = [request for request in requests if request.get("path") == TARGET_PATH]
        expected_status = 403 if denied else 200
        if not target or any(request.get("status") != expected_status for request in target):
            raise RuntimeError("TUI target request did not prove the selected credentials")
        if not denied:
            if "Kubernetes context: " + selected not in view or "scout-context-marker" not in view:
                raise RuntimeError("allowed TUI result lost selected context or exact target")
        elif "forbidden" not in view.lower() or DENIED_USER not in view or "native" in view.lower():
            raise RuntimeError("denied TUI result hid access denial or looked unmanaged")


def cleanup_cluster(receipt: dict, *, name: str, created: bool, creation_attempted: bool,
                    tools: dict, env: dict, deadline: float) -> list[str]:
    """Delete only a successfully created invocation-owned cluster."""
    errors = []
    if created:
        deleted = run([tools["kind"], "delete", "cluster", "--name", name], env=env,
                      timeout=90, deadline=deadline)
        receipt["commands"].append({"phase": "cleanup-owned-cluster", **deleted})
        if not succeeded(deleted):
            errors.append("kind delete failed")
        absent = run([tools["docker"], "ps", "-aq", "--filter", "label=io.x-k8s.kind.cluster=" + name],
                     env=env, timeout=15, deadline=deadline)
        receipt["commands"].append({"phase": "verify-owned-nodes-absent", **absent})
        if not succeeded(absent) or absent["stdout"].strip():
            errors.append("owned node absence unverified")
    elif creation_attempted:
        inventory = run([tools["docker"], "ps", "-aq", "--filter", "label=io.x-k8s.kind.cluster=" + name],
                        env=env, timeout=15, deadline=deadline)
        receipt["commands"].append({"phase": "uncertain-create-node-inventory", **inventory})
        if not succeeded(inventory) or inventory["stdout"].strip():
            errors.append("creation failed; possible partial resources require ownership review before removal")
    return errors


def cleanup_worktrees(receipt: dict, *, repo: Path, git: str, worktrees: list[Path], env: dict) -> list[str]:
    """Remove only temporary source worktrees created by this capture."""
    errors = []
    for checkout in reversed(worktrees):
        if not checkout.exists():
            continue
        result = run([git, "-C", str(repo), "worktree", "remove", "--force", str(checkout)],
                     env=env, timeout=60)
        receipt["commands"].append({"phase": "cleanup-source-worktree", **result})
        if not succeeded(result):
            errors.append("temporary source worktree removal failed")
    return errors


def fixed_phase(context: str, command: str) -> str:
    if context not in (ALLOWED_CONTEXT, DENIED_CONTEXT) or command not in ("normal", "reverse"):
        raise ValueError("unknown fixed-source observation selector")
    return ("fixed-allowed" if context == ALLOWED_CONTEXT else "fixed-denied") + "-" + command


def require_combined_source_pin() -> str:
    if not COMBINED_SOURCE_PIN:
        raise RuntimeError("integrated source pin has not been selected")
    if len(COMBINED_SOURCE_PIN) != 40 or any(char not in "0123456789abcdef" for char in COMBINED_SOURCE_PIN):
        raise RuntimeError("integrated source pin must be a lowercase full Git commit")
    return COMBINED_SOURCE_PIN


def observation_env(base: dict[str, str], *, private_home: Path, shims: Path,
                    kubeconfig: Path) -> dict[str, str]:
    """Keep product commands inside a private HOME/XDG and fail-closed PATH."""
    return {**base, "PATH": str(shims), "HOME": str(private_home),
            "XDG_CONFIG_HOME": str(private_home / ".config"),
            "XDG_CACHE_HOME": str(private_home / ".cache"),
            "XDG_DATA_HOME": str(private_home / ".local" / "share"),
            "KUBECONFIG": str(kubeconfig)}


def tui_observation_env(cli_env: dict[str, str], kubeconfig: Path) -> dict[str, str]:
    """Retarget an already sanitized child environment to the private TUI config."""
    env = dict(cli_env)
    env["KUBECONFIG"] = str(kubeconfig)
    return env


def create_observation_shims(shims: Path, kubectl_path: str, *, combined: bool = False) -> dict[str, Path]:
    """Block all external tools except one exact private-config Argo fallback GET."""
    kubectl = shlex.quote(kubectl_path)
    cub_script = (("#!/bin/sh\n"
                   "if [ \"$#\" -eq 2 ] && [ \"$1\" = auth ] && [ \"$2\" = status ]; then\n"
                   "  printf '%s\\n' '{\"type\":\"call\",\"argv\":[\"cub\",\"auth\",\"status\"],\"exitCode\":0}' >> \"$SCOUT_TRACE_CUB_LOG\"\n"
                   "  exit 0\nfi\n"
                   f"if [ \"$#\" -eq 7 ] && [ \"$1\" = unit ] && [ \"$2\" = get ] && [ \"$3\" = {CONFIGHUB_UNIT} ] && [ \"$4\" = -o ] && [ \"$5\" = json ] && [ \"$6\" = --space ] && [ \"$7\" = {CONFIGHUB_SPACE} ]; then\n"
                   f"  printf '%s\\n' '{{\"type\":\"call\",\"argv\":[\"cub\",\"unit\",\"get\",\"{CONFIGHUB_UNIT}\",\"-o\",\"json\",\"--space\",\"{CONFIGHUB_SPACE}\"],\"exitCode\":73,\"result\":\"recorded unit lookup unavailable in owned proof\"}}' >> \"$SCOUT_TRACE_CUB_LOG\"\n"
                   "  echo 'recorded unit lookup unavailable in owned proof' >&2\n  exit 73\nfi\n"
                   "echo 'refusing unexpected ConfigHub command' >&2\nexit 97\n") if combined else
                  "#!/bin/sh\necho 'blocked ConfigHub command' >&2\nexit 97\n")
    specs = {
        "argocd": ("#!/bin/sh\nif [ \"$#\" -eq 2 ] && [ \"$1\" = version ] && [ \"$2\" = --client ]; then\n"
                   "  echo 'owned proof CLI shim'; exit 0\nfi\n"
                   "echo 'FATA[0000] server address unspecified' >&2\nexit 1\n"),
        "kubectl": ("#!/bin/sh\nif [ \"$#\" -eq 5 ] && [ \"$1\" = get ] && [ \"$2\" = applications.argoproj.io ] && "
                    "[ \"$3\" = --all-namespaces ] && [ \"$4\" = -o ] && [ \"$5\" = json ]; then\n"
                    f"  exec {kubectl} \"$@\" --context \"$SCOUT_TRACE_OLD_CONTEXT\"\nfi\n"
                    "echo 'refusing unexpected owned kubectl operation' >&2; exit 97\n"),
        "flux": "#!/bin/sh\necho 'blocked external Flux command' >&2\nexit 97\n",
        "helm": "#!/bin/sh\necho 'blocked external Helm command' >&2\nexit 97\n",
        "cub": cub_script,
    }
    paths = {}
    for name, contents in specs.items():
        path = shims / name
        path.write_text(contents)
        path.chmod(0o700)
        paths[name] = path
    return paths


def _write_fixture(work: Path, *, source_truth: bool = False) -> Path:
    fixture = work / "trace-owned-fixture.yaml"
    fixture.write_text(f'''apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {APPLICATION}
  namespace: {NAMESPACE}
spec:
  project: default
  source:
    repoURL: {SOURCE_URL}
    path: fixture
    targetRevision: owned-kind-proof
  destination:
    server: https://kubernetes.default.svc
    namespace: {NAMESPACE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {DEPLOYMENT}
  namespace: {NAMESPACE}
  labels:
    argocd.argoproj.io/instance: {APPLICATION}
spec:
  replicas: 0
  selector:
    matchLabels:
      app.kubernetes.io/name: {DEPLOYMENT}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {DEPLOYMENT}
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.10
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {NATIVE_DEPLOYMENT}
  namespace: {NAMESPACE}
spec:
  replicas: 0
  selector:
    matchLabels:
      app.kubernetes.io/name: {NATIVE_DEPLOYMENT}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {NATIVE_DEPLOYMENT}
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.10
''')
    if source_truth:
        _install_source_truth_fixture(fixture)
    return fixture


def _install_source_truth_fixture(fixture: Path) -> None:
    text = fixture.read_text()
    text = text.replace(
        f"    argocd.argoproj.io/instance: {APPLICATION}\nspec:",
        f"    argocd.argoproj.io/instance: {APPLICATION}\n"
        f"    confighub.com/UnitSlug: {CONFIGHUB_UNIT}\n"
        f"  annotations:\n    confighub.com/SpaceName: {CONFIGHUB_SPACE}\nspec:", 1)
    text = text.replace(
        f"    namespace: {NAMESPACE}\n---\napiVersion: apps/v1\nkind: Deployment",
        f"    namespace: {NAMESPACE}\n"
        "status:\n"
        "  sync:\n"
        "    revision: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"
        "  health:\n"
        "    status: Healthy\n"
        "  resources:\n"
        "  - group: apps\n"
        "    version: v1\n"
        "    kind: Deployment\n"
        f"    namespace: {NAMESPACE}\n"
        f"    name: {DEPLOYMENT}\n"
        "---\napiVersion: apps/v1\nkind: Deployment", 1)
    fixture.write_text(text)


def _write_rendered_inputs(work: Path) -> dict[str, Path]:
    inputs = work / "rendered"
    inputs.mkdir(mode=0o700)
    paths = {}
    for label, name, replicas in (
            ("matched", DEPLOYMENT, 0), ("changed", DEPLOYMENT, 1),
            ("missing", "scout-context-missing", 0)):
        path = inputs / (label + ".yaml")
        path.write_text(f'''apiVersion: apps/v1
kind: Deployment
metadata:
  name: {name}
  namespace: {NAMESPACE}
spec:
  replicas: {replicas}
''')
        paths[label] = path
    return paths


def _proxy_pair(config: dict, private_dir: Path):
    clusters = {row["name"]: row["cluster"] for row in config.get("clusters", [])}
    users = {row["name"]: row["user"] for row in config.get("users", [])}
    contexts = {row["name"]: row["context"] for row in config.get("contexts", [])}
    try:
        allowed_context = contexts[ALLOWED_CONTEXT]
        denied_context = contexts[DENIED_CONTEXT]
        allowed_cluster = clusters[allowed_context["cluster"]]
        denied_cluster = clusters[denied_context["cluster"]]
        allowed_user = users[allowed_context["user"]]
        denied_user = users[denied_context["user"]]
        if allowed_cluster["server"] != denied_cluster["server"]:
            raise RuntimeError("proof contexts do not target the same owned API")
    except (KeyError, TypeError) as exc:
        raise RuntimeError("owned proof contexts lack exact API binding data") from exc
    cert_paths = []
    try:
        allowed_tls, allowed_auth, cert_paths = api_proxy.upstream_credentials(
            allowed_cluster, {"user": allowed_user}, private_dir=private_dir, label="allowed")
        denied_tls, denied_auth, denied_paths = api_proxy.upstream_credentials(
            denied_cluster, {"user": denied_user}, private_dir=private_dir, label="denied")
        cert_paths.extend(denied_paths)
    except Exception:
        for path in cert_paths:
            path.unlink(missing_ok=True)
        raise
    active_proxies = []
    try:
        allowed = api_proxy.ReadOnlyAPIProxy(label="allowed", upstream=allowed_cluster["server"],
            tls=allowed_tls, authorization=allowed_auth, namespace=NAMESPACE,
            deployment=DEPLOYMENT, application=APPLICATION,
            missing_deployment="scout-context-missing", event_log=private_dir / "api-events.jsonl")
        active_proxies.append(allowed)
        denied = api_proxy.ReadOnlyAPIProxy(label="denied", upstream=denied_cluster["server"],
            tls=denied_tls, authorization=denied_auth, namespace=NAMESPACE,
            deployment=DEPLOYMENT, application=APPLICATION,
            missing_deployment="scout-context-missing", event_log=private_dir / "api-events.jsonl")
        active_proxies.append(denied)
    except Exception:
        for proxy in reversed(active_proxies):
            proxy.close()
        for path in cert_paths + denied_paths:
            path.unlink(missing_ok=True)
        raise
    try:
        projected = api_proxy.observation_kubeconfig(config, allowed_endpoint=allowed.endpoint,
            denied_endpoint=denied.endpoint, allowed_cluster_name="proof-allowed-api",
            denied_cluster_name="proof-denied-api", allowed_context=ALLOWED_CONTEXT,
            denied_context=DENIED_CONTEXT, namespace=NAMESPACE)
    except Exception:
        for proxy in reversed(active_proxies):
            proxy.close()
        for path in cert_paths:
            path.unlink(missing_ok=True)
        raise
    return active_proxies, projected, cert_paths


def _proxy_records(proxies, *, clear: bool = False) -> list[dict]:
    return [row for proxy in proxies for row in proxy.snapshot(clear=clear)]


def _mark_api_phase(path: Path, phase: str) -> None:
    data = json.dumps({"type": "phase", "phase": phase}, separators=(",", ":")).encode() + b"\n"
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)


def _mark_cub_phase(path: Path, phase: str) -> None:
    data = json.dumps({"type": "phase", "phase": phase}, separators=(",", ":")).encode() + b"\n"
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)


def _combined_action(receipt: dict, *, phase: str, command: str, argv: list[str],
                     result: dict, api_records: list[dict], cub_records: list[dict],
                     tool_arguments: dict | None = None) -> None:
    action = {"phase": phase, "command": command, "argv": argv,
              "exitCode": result.get("exitCode"), "failure": result.get("failure"),
              "stdout": result.get("stdout", ""), "stderr": result.get("stderr", ""),
              "apiRequests": api_records, "cubArgv": cub_records}
    if tool_arguments is not None:
        action["mcpToolArguments"] = tool_arguments
    receipt.setdefault("actions", []).append(action)
    if result.get("failure") or result.get("exitCode") != 0:
        raise RuntimeError("combined observation command failed in phase " + phase)


def _new_cub_records(log_path: Path, old_bytes: int) -> list[dict]:
    if not log_path.exists():
        return []
    data = log_path.read_bytes()[old_bytes:].decode("utf-8", "replace")
    rows = []
    for line in data.splitlines():
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            raise RuntimeError("ConfigHub shim wrote malformed argv evidence") from None
        if isinstance(row, dict) and row.get("type") == "phase":
            continue
        if not isinstance(row, dict) or row.get("type") != "call" or not isinstance(row.get("argv"), list):
            raise RuntimeError("ConfigHub shim wrote invalid argv evidence")
        rows.append(row)
    return rows


def _mcp_payload(tool: str, arguments: dict) -> bytes:
    messages = [
        {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
            "protocolVersion": "2024-11-05", "capabilities": {},
            "clientInfo": {"name": "owned-proof", "version": "1"}}},
        {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}},
        {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {
            "name": tool, "arguments": arguments}},
    ]
    payload = "".join(json.dumps(message, separators=(",", ":")) + "\n" for message in messages).encode()
    return payload


def _decode_json_output(result: dict, label: str) -> dict:
    try:
        value = json.loads(result.get("stdout", ""))
    except (TypeError, json.JSONDecodeError):
        raise RuntimeError(label + " did not return a JSON document") from None
    if not isinstance(value, dict):
        raise RuntimeError(label + " returned a non-object JSON value")
    return value


def _mcp_call_result(result: dict) -> dict:
    messages = []
    for line in result.get("stdout", "").splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            messages.append(value)
    response = next((value for value in messages if value.get("id") == 2), None)
    if response is None:
        raise RuntimeError("MCP stdio session omitted tools/call response")
    return api_proxy.mcp_result_json(response)


def _combined_source_truth_action(*, phase: str, body: dict, context: str,
                                  endpoint: str, rows: list[dict], cub_rows: list[dict],
                                  unit_failure: bool) -> None:
    api_proxy.validate_source_truth(body, context=context, require_unit_failure=unit_failure)
    target_path = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{DEPLOYMENT}"
    status = 200 if endpoint == "allowed" else 403
    api_proxy.validate_api_records(rows, endpoint=endpoint, target_path=target_path, target_status=status)
    errors = body.get("collection_errors", [])
    if endpoint == "denied":
        if not any(DENIED_USER in str(error) for error in errors):
            raise RuntimeError(phase + " lost exact denied-user evidence")
        if any("unit" in json.dumps(row).lower() for row in cub_rows):
            raise RuntimeError(phase + " attempted ConfigHub unit access after runtime denial")
    else:
        app_path = f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/{APPLICATION}"
        if not any(row.get("path") == app_path and row.get("status") == 200 for row in rows):
            raise RuntimeError(phase + " did not read the exact Argo Application")
        if not any(row.get("argv") == ["cub", "unit", "get", CONFIGHUB_UNIT,
                                       "-o", "json", "--space", CONFIGHUB_SPACE] and
                   row.get("exitCode") == 73 for row in cub_rows):
            raise RuntimeError(phase + " omitted the explicit failed ConfigHub unit read")
    if not any(row.get("argv") == ["cub", "auth", "status"] and row.get("exitCode") == 0 for row in cub_rows):
        raise RuntimeError(phase + " omitted the exact ConfigHub connected-gate check")


def _validate_cub_auth_only(phase: str, cub_rows: list[dict]) -> None:
    # MCP startup checks once, then the tools/call session check refreshes it.
    expected = [{"argv": ["cub", "auth", "status"], "exitCode": 0}] * 2
    actual = [{"argv": row.get("argv"), "exitCode": row.get("exitCode")} for row in cub_rows]
    if actual != expected:
        raise RuntimeError(phase + " ConfigHub startup calls differed from the exact auth-only contract")


def _run_combined_action_flow(*, receipt: dict, binary: str, tui_binary: str,
                              cli_env: dict, proxies: list,
                              cub_log: Path, event_log: Path, rendered: dict[str, Path],
                              tui_config: Path, deadline: float, output: Path) -> None:
    phases = ["cli-source-truth-allowed", "cli-source-truth-denied",
              "cli-diff-matched", "cli-diff-changed", "cli-diff-missing", "cli-diff-denied",
              "mcp-source-truth", "mcp-diff-matched", "mcp-diff-denied",
              "tui-source-truth-allowed", "tui-source-truth-reopen-after-retarget",
              "tui-diff-allowed", "tui-diff-denied"]
    event_log.write_text("")
    event_log.chmod(0o600)
    cub_log.write_text("")
    cub_log.chmod(0o600)
    receipt["desiredInputs"] = {name: {"path": str(path), "sha256": digest(path),
                                         "role": "local already-rendered Kubernetes input"}
                                for name, path in rendered.items()}

    def invoke(phase: str, command: str, argv: list[str], env: dict,
               *, tool_arguments: dict | None = None,
               mcp_tool: str | None = None) -> tuple[dict, list[dict], list[dict]]:
        _mark_api_phase(event_log, phase)
        _mark_cub_phase(cub_log, phase)
        api_records = _proxy_records(proxies, clear=True)
        if api_records:
            raise RuntimeError("proxy request escaped its recorded action boundary")
        old_bytes = cub_log.stat().st_size if cub_log.exists() else 0
        payload = _mcp_payload(mcp_tool, tool_arguments["arguments"]) if mcp_tool and tool_arguments else None
        result = run(argv, env=env, timeout=90, deadline=deadline, input_data=payload)
        rows = _proxy_records(proxies, clear=True)
        cub_rows = _new_cub_records(cub_log, old_bytes)
        _combined_action(receipt, phase=phase, command=command, argv=argv, result=result,
                         api_records=rows, cub_records=cub_rows, tool_arguments=tool_arguments)
        return result, rows, cub_rows

    target = "Deployment/" + DEPLOYMENT
    for context, endpoint in ((ALLOWED_CONTEXT, "allowed"), (DENIED_CONTEXT, "denied")):
        phase = "cli-source-truth-allowed" if endpoint == "allowed" else "cli-source-truth-denied"
        argv = [binary, "compare", "source-truth", target, "-n", NAMESPACE,
                "--strategy", "git-argo", "--kube-context", context, "--format", "json"]
        result, rows, cub_rows = invoke(phase, "cli", argv, cli_env)
        _combined_source_truth_action(phase=phase, body=_decode_json_output(result, phase),
            context=context, endpoint=endpoint, rows=rows, cub_rows=cub_rows,
            unit_failure=endpoint == "allowed")

    diff_cases = (("matched", ALLOWED_CONTEXT, "allowed", "matched", DEPLOYMENT, 200),
                  ("changed", ALLOWED_CONTEXT, "allowed", "changed", DEPLOYMENT, 200),
                  ("missing", ALLOWED_CONTEXT, "allowed", "missing", "scout-context-missing", 404),
                  ("denied", DENIED_CONTEXT, "denied", "inconclusive", DEPLOYMENT, 403))
    for label, context, endpoint, status, name, api_status in diff_cases:
        phase = "cli-diff-" + label
        argv = [binary, "trace", "deployment/" + name, "-n", NAMESPACE, "--diff",
                "--desired-file", str(rendered["matched" if label == "denied" else label]), "--api-version", "apps/v1",
                "--kube-context", context, "--format", "json"]
        result, rows, cub_rows = invoke(phase, "cli", argv, cli_env)
        api_proxy.validate_diff(_decode_json_output(result, phase), context=context, status=status,
                                namespace=NAMESPACE, name=name)
        path = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{name}"
        api_proxy.validate_api_records(rows, endpoint=endpoint, target_path=path, target_status=api_status)
        if cub_rows:
            raise RuntimeError(phase + " unexpectedly invoked ConfigHub")

    truth_args = {"target": target, "namespace": NAMESPACE, "strategy": "git-argo", "context": ALLOWED_CONTEXT}
    result, rows, cub_rows = invoke("mcp-source-truth", "mcp-stdio", [binary, "mcp", "serve"], cli_env,
                                    tool_arguments={"tool": "compare_source_truth", "arguments": truth_args},
                                    mcp_tool="compare_source_truth")
    body = _mcp_call_result(result)
    _combined_source_truth_action(phase="mcp-source-truth", body=body, context=ALLOWED_CONTEXT,
        endpoint="allowed", rows=rows, cub_rows=cub_rows, unit_failure=True)

    for label, context, endpoint, status, phase in (
            ("matched", ALLOWED_CONTEXT, "allowed", "matched", "mcp-diff-matched"),
            ("denied", DENIED_CONTEXT, "denied", "inconclusive", "mcp-diff-denied")):
        name = DEPLOYMENT
        tool_arguments = {"resource": "Deployment/" + name, "namespace": NAMESPACE,
                          "context": context, "diff": True,
                          "desired_file": str(rendered["matched" if label == "denied" else label]), "api_version": "apps/v1"}
        result, rows, cub_rows = invoke(phase, "mcp-stdio", [binary, "mcp", "serve"], cli_env,
                                        tool_arguments={"tool": "trace", "arguments": tool_arguments}, mcp_tool="trace")
        body = _mcp_call_result(result)
        api_proxy.validate_diff(body, context=context, status=status, namespace=NAMESPACE, name=name)
        target_path = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{name}"
        api_proxy.validate_api_records(rows, endpoint=endpoint, target_path=target_path,
                                       target_status=200 if endpoint == "allowed" else 403)
        _validate_cub_auth_only(phase, cub_rows)

    shutil.copyfile(cli_env["KUBECONFIG"], tui_config)
    tui_config.chmod(0o600)
    tui_result = output / "combined-tui-result.json"
    # cli_env already has a private HOME/PATH and no ambient ConfigHub setup.
    # Only switch its kubeconfig to the TUI's private, retargetable copy.
    tui_env = tui_observation_env(cli_env, tui_config)
    tui_env.update({"SCOUT_TRACE_COMBINED_EXECUTE": "1",
        "SCOUT_TRACE_COMBINED_TUI_CONFIG": str(tui_config),
        "SCOUT_TRACE_COMBINED_TUI_RESULT": str(tui_result),
        "SCOUT_TRACE_COMBINED_DESIRED_FILE": str(rendered["matched"]),
        "SCOUT_TRACE_COMBINED_EVENT_LOG": str(event_log),
        "SCOUT_TRACE_COMBINED_CUB_LOG": str(cub_log)})
    old_bytes = cub_log.stat().st_size if cub_log.exists() else 0
    tui_result_run = run([tui_binary, "-test.run", "^TestCombinedSourceTruthAndTraceDiffOwnedTUI$",
                          "-test.count=1", "-test.v", "-test.timeout=120s"], env=tui_env,
                         timeout=140, deadline=deadline)
    tui_rows = _proxy_records(proxies, clear=True)
    tui_cub_rows = _new_cub_records(cub_log, old_bytes)
    _combined_action(receipt, phase="tui-live", command="tui-update-tests",
        argv=[tui_binary, "-test.run", "^TestCombinedSourceTruthAndTraceDiffOwnedTUI$",
              "-test.count=1", "-test.v", "-test.timeout=120s"], result=tui_result_run,
        api_records=tui_rows, cub_records=tui_cub_rows)
    if not tui_result.exists():
        raise RuntimeError("combined TUI probe did not write its result")
    tui_data = _decode_json_output({"stdout": tui_result.read_text()}, "combined TUI probe")
    if tui_data.get("schema") != "trace-source-truth-diff-owned-tui.v1" or tui_data.get("passed") is not True:
        raise RuntimeError("combined TUI probe failed its actual Update path")
    expected_checks = ("source-truth-allowed", "source-truth-reopen-after-retarget",
                       "diff-allowed", "diff-denied", "private-config-retarget-stable", "denied-is-inconclusive")
    if any(tui_data.get("checks", {}).get(name) is not True for name in expected_checks):
        raise RuntimeError("combined TUI probe omitted a required action")
    statuses = tui_data.get("statuses", {})
    if statuses.get("source-truth-allowed") != "BLOCK" or statuses.get("source-truth-reopen-after-retarget") != "BLOCK":
        raise RuntimeError("combined TUI ConfigHub failure was not retained as BLOCK")
    views = tui_data.get("views", {})
    if any("recorded unit lookup unavailable in owned proof" not in views.get(name, "")
           for name in ("source-truth-allowed", "source-truth-reopen-after-retarget")):
        raise RuntimeError("combined TUI hid the explicit ConfigHub fixture error")
    receipt["tuiResultSha256"] = digest(tui_result)
    receipt["tuiConfigSha256"] = tui_data.get("privateConfigSha256")
    receipt["apiEventLogSha256"] = digest(event_log)
    expected_api = {
        "cli-source-truth-allowed": ("allowed", TARGET_PATH, 200),
        "cli-source-truth-denied": ("denied", TARGET_PATH, 403),
        "cli-diff-matched": ("allowed", TARGET_PATH, 200),
        "cli-diff-changed": ("allowed", TARGET_PATH, 200),
        "cli-diff-missing": ("allowed", f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/scout-context-missing", 404),
        "cli-diff-denied": ("denied", TARGET_PATH, 403),
        "mcp-source-truth": ("allowed", TARGET_PATH, 200),
        "mcp-diff-matched": ("allowed", TARGET_PATH, 200),
        "mcp-diff-denied": ("denied", TARGET_PATH, 403),
        "tui-source-truth-allowed": ("allowed", TARGET_PATH, 200),
        "tui-source-truth-reopen-after-retarget": ("allowed", TARGET_PATH, 200),
        "tui-diff-allowed": ("allowed", TARGET_PATH, 200),
        "tui-diff-denied": ("denied", TARGET_PATH, 403),
    }
    grouped = api_proxy.group_action_events(event_log, phases)
    cub_grouped = api_proxy.group_cub_events(cub_log, phases)
    for phase, (endpoint, path, status) in expected_api.items():
        rows = grouped[phase]
        api_proxy.validate_api_records(rows, endpoint=endpoint, target_path=path, target_status=status)
        allowed_paths = {path, "/version", "/api", "/api/v1", "/apis", "/apis/apps",
                         "/apis/apps/v1", "/apis/argoproj.io", "/apis/argoproj.io/v1alpha1",
                         f"/apis/argoproj.io/v1alpha1/applications",
                         f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/{APPLICATION}"}
        if any(row.get("path") not in allowed_paths for row in rows):
            raise RuntimeError(phase + " read an API route outside its exact proof allowlist")
        if phase in ("cli-source-truth-allowed", "mcp-source-truth",
                     "tui-source-truth-allowed", "tui-source-truth-reopen-after-retarget"):
            app_path = f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/{APPLICATION}"
            if not any(row.get("path") == app_path and row.get("status") == 200 for row in rows):
                raise RuntimeError(phase + " lacks its exact Application GET")
    auth_argv = ["cub", "auth", "status"]
    unit_argv = ["cub", "unit", "get", CONFIGHUB_UNIT, "-o", "json", "--space", CONFIGHUB_SPACE]
    expected_cub = {
        "cli-source-truth-allowed": [(auth_argv, 0), (unit_argv, 73)],
        "cli-source-truth-denied": [(auth_argv, 0)],
        "cli-diff-matched": [], "cli-diff-changed": [], "cli-diff-missing": [], "cli-diff-denied": [],
        "mcp-source-truth": [(auth_argv, 0), (auth_argv, 0), (auth_argv, 0), (unit_argv, 73)],
        "mcp-diff-matched": [(auth_argv, 0), (auth_argv, 0)],
        "mcp-diff-denied": [(auth_argv, 0), (auth_argv, 0)],
        "tui-source-truth-allowed": [(auth_argv, 0), (unit_argv, 73)],
        "tui-source-truth-reopen-after-retarget": [(auth_argv, 0), (unit_argv, 73)],
        "tui-diff-allowed": [], "tui-diff-denied": [],
    }
    for phase, expected in expected_cub.items():
        actual = [(row.get("argv"), row.get("exitCode")) for row in cub_grouped[phase]]
        if actual != expected:
            raise RuntimeError(phase + " ConfigHub child command sequence differed from the exact stub contract")
    receipt["apiEventPhases"] = {phase: grouped[phase] for phase in phases}
    receipt["cubArgvPhases"] = {phase: cub_grouped[phase] for phase in phases}
    receipt["acceptance"] = "passed"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--mode", choices=("trace-context", "source-truth-diff"), default="trace-context")
    parser.add_argument("--integrity-only-shared-kubeconfig", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    combined = args.mode == "source-truth-diff"
    combined_pin = require_combined_source_pin() if combined else ""
    if not args.execute:
        parser.error("refusing to run without --execute")
    shared_arg = args.integrity_only_shared_kubeconfig.expanduser().absolute()
    if shared_arg.is_symlink():
        parser.error("integrity-only kubeconfig must not be a symlink")
    shared = shared_arg.resolve(strict=True)
    if not shared.is_file():
        parser.error("integrity-only kubeconfig must be a regular file")
    output = args.output_dir.expanduser().absolute()
    if output.exists() or output.is_symlink() or ".." in args.output_dir.parts:
        parser.error("output directory must be fresh and must not contain '..'")
    if not any(Path(base).resolve() == output.resolve() or Path(base).resolve() in output.resolve().parents
               for base in ("/tmp", "/var/tmp")):
        parser.error("output directory must be under /tmp or /var/tmp")
    output.mkdir(parents=True, mode=0o700)
    os.chmod(output, 0o700)

    work = None
    private_config = None
    observation_config = None
    tui_config = None
    worktrees: list[Path] = []
    tools: dict[str, str | None] = {}
    env: dict[str, str] = {}
    cluster = None
    created = False
    creation_attempted = False
    signal_handlers = {}
    cleanup_errors: list[str] = []
    proxies = []
    proxy_cert_paths = []
    receipt = {"schema": "trace-source-truth-diff-owned-kind.v1" if combined else "trace-context-owned-kind.v1",
               "oldSource": None if combined else OLD_SOURCE,
               "integratedSource": combined_pin if combined else None,
               "fixedSource": FIXED_SOURCE, "commands": [],
               "sharedKubeconfigSha256Before": digest(shared)}
    try:
        tool_names = ("git", "go", "kind", "kubectl", "docker")
        tools = {name: shutil.which(name) for name in tool_names}
        if any(path is None for path in tools.values()):
            raise RuntimeError("required existing tool missing: git, go, kind, kubectl, docker")
        if os.environ.get("DOCKER_HOST"):
            raise RuntimeError("DOCKER_HOST is set; refusing a remote Docker engine")
        work = Path(tempfile.mkdtemp(prefix="scout-trace-context-", dir="/tmp"))
        os.chmod(work, 0o700)
        private_config, tui_config = work / "kubeconfig", work / "tui.kubeconfig"
        observation_config = work / "observation.kubeconfig" if combined else private_config
        binaries = work / "bin"
        binaries.mkdir(mode=0o700)
        private_home = work / "home"
        private_home.mkdir(mode=0o700)
        env = {key: value for key, value in os.environ.items()
               if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
        env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
                    "CUB_SCOUT_SCAN_PROVIDER": "legacy",
                    "KUBECONFIG": str(private_config)})
        if not combined:
            env["CUB_SCOUT_OFFLINE"] = "true"
        for key in ("CUB_SPACE", "CUB_SCOUT_TEST_TRACE_JSON", "CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON",
                    "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
            env.pop(key, None)
        shims = work / "shims"
        shims.mkdir(mode=0o700)
        observation_env_base = {**env, "SCOUT_TRACE_OLD_CONTEXT": ALLOWED_CONTEXT}
        cub_log = work / "cub-argv.jsonl"
        if combined:
            observation_env_base["SCOUT_TRACE_CUB_LOG"] = str(cub_log)
            cub_log.touch(mode=0o600)
            cub_log.chmod(0o600)
        shim_paths = create_observation_shims(shims, tools["kubectl"], combined=combined)
        cli_env = observation_env(observation_env_base, private_home=private_home, shims=shims, kubeconfig=observation_config)
        receipt["observationShims"] = {name: digest(path) for name, path in shim_paths.items()}
        deadline = time.monotonic() + MAX_SECONDS
        def interrupt(signum, _frame):
            raise InterruptedError("capture interrupted by signal " + str(signum))
        for signum in (signal.SIGINT, signal.SIGTERM):
            signal_handlers[signum] = signal.signal(signum, interrupt)

        clean = record_command(receipt, "source-clean", [tools["git"], "-C", str(REPO), "status", "--porcelain"], env=env, deadline=deadline)
        if clean["stdout"].strip():
            raise RuntimeError("capture checkout must be committed and clean")
        if combined:
            record_command(receipt, "integrated-source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", combined_pin, "HEAD"], env=env, deadline=deadline)
        else:
            record_command(receipt, "fixed-source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", FIXED_SOURCE, "HEAD"], env=env, deadline=deadline)
            record_command(receipt, "old-source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", OLD_SOURCE, FIXED_SOURCE], env=env, deadline=deadline)
        receipt["captureScriptSha256"] = digest(Path(__file__))
        version = record_command(receipt, "kind-version", [tools["kind"], "version"], env=env, deadline=deadline)["stdout"].split()
        if len(version) < 2 or version[:2] != ["kind", KIND_VERSION]:
            raise RuntimeError("kind version differs from pinned requirement")
        docker_context = record_command(receipt, "docker-context", [tools["docker"], "context", "show"], env=env, deadline=deadline)["stdout"].strip()
        endpoint = record_command(receipt, "docker-endpoint", [tools["docker"], "context", "inspect", docker_context, "--format", "{{json .Endpoints.docker.Host}}"], env=env, deadline=deadline)
        if not json.loads(endpoint["stdout"]).startswith("unix://"):
            raise RuntimeError("refusing non-local Docker endpoint")
        env["DOCKER_CONTEXT"] = docker_context
        node = record_command(receipt, "cached-node-image", [tools["docker"], "image", "inspect", NODE_IMAGE], env=env, deadline=deadline)
        image_data = json.loads(node["stdout"])
        expected_digest = NODE_IMAGE.rsplit("@", 1)[1]
        repo_digests = image_data[0].get("RepoDigests", []) if image_data else []
        if not any(value.rsplit("@", 1)[-1] == expected_digest and value.split("@")[0].split("/")[-2:] == ["kindest", "node"] for value in repo_digests):
            raise RuntimeError("pinned Kubernetes node image is not already cached")
        receipt["nodeImage"] = NODE_IMAGE
        receipt["toolPins"] = {name: {"path": path, "sha256": digest(Path(path))} for name, path in tools.items()}

        # Build the exact source(s) and compile the injected TUI probe before
        # asking kind to create anything.
        source_specs = (("combined", combined_pin),) if combined else (("old", OLD_SOURCE), ("fixed", FIXED_SOURCE))
        for label, ref in source_specs:
            checkout = work / ("source-" + label)
            worktrees.append(checkout)
            record_command(receipt, label + "-worktree", [tools["git"], "-C", str(REPO), "worktree", "add", "--detach", str(checkout), ref], env=env, deadline=deadline)
            binary = binaries / ("cub-scout-" + label)
            record_command(receipt, label + "-build", [tools["go"], "-C", str(checkout), "build", "-o", str(binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
            commit = record_command(receipt, label + "-source", [tools["git"], "-C", str(checkout), "rev-parse", "HEAD"], env=env, deadline=deadline)["stdout"].strip()
            if commit != ref:
                raise RuntimeError(label + " binary does not match the exact source pin")
            receipt["integratedSourceCommit" if combined else label + "SourceCommit"] = commit
            receipt["integratedBinarySha256" if combined else label + "BinarySha256"] = digest(binary)
        template = Path(__file__).with_name("combined_live_test.go.txt" if combined else "tui_live_test.go.txt")
        receipt["tuiProbeSha256"] = digest(template)
        source_label = "combined" if combined else "fixed"
        probe_name = "trace_source_truth_diff_owned_live_test.go" if combined else "trace_context_owned_live_test.go"
        probe = work / ("source-" + source_label) / "cmd" / "cub-scout" / probe_name
        with probe.open("x") as stream:
            stream.write(template.read_text())
        tui_binary = binaries / ("trace-source-truth-diff-tui.test" if combined else "trace-context-tui.test")
        record_command(receipt, "tui-build", [tools["go"], "-C", str(work / ("source-" + source_label)), "test", "-c", "-o", str(tui_binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
        receipt["tuiBinarySha256"] = digest(tui_binary)

        cluster = "scout-trace-" + uuid.uuid4().hex[:10]
        existing = record_command(receipt, "cluster-name-check", [tools["kind"], "get", "clusters"], env=env, deadline=deadline)
        if cluster in existing["stdout"].splitlines():
            raise RuntimeError("generated cluster name already exists")
        receipt["ownedCluster"] = cluster
        (output / "owned-cluster-marker.json").write_text(json.dumps({"cluster": cluster, "ownerPid": os.getpid(), "privateDirectory": str(work)}, indent=2) + "\n")
        try:
            creation_attempted = True
            record_command(receipt, "create-owned-cluster", [tools["kind"], "create", "cluster", "--name", cluster,
                           "--image", NODE_IMAGE, "--kubeconfig", str(private_config), "--wait", "90s"],
                           env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline, timeout=150)
            created = True
            os.chmod(private_config, 0o600)
            kube = {**env, "KUBECONFIG": str(private_config)}
            kubectl = tools["kubectl"]
            record_command(receipt, "rename-owned-context", [kubectl, "config", "rename-context", "kind-" + cluster, ALLOWED_CONTEXT], env=kube, deadline=deadline)
            record_command(receipt, "fixture-namespace", [kubectl, "create", "namespace", NAMESPACE, "--context", ALLOWED_CONTEXT], env=kube, deadline=deadline)
            crd_path = work / "application-crd.yaml"
            crd_path.write_text('''apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: applications.argoproj.io
spec:
  group: argoproj.io
  scope: Namespaced
  names:
    plural: applications
    singular: application
    kind: Application
    shortNames: [app]
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
''')
            record_command(receipt, "fixture-application-crd", [kubectl, "apply", "--context", ALLOWED_CONTEXT, "-f", str(crd_path)], env=kube, deadline=deadline)
            record_command(receipt, "fixture-crd-ready", [kubectl, "wait", "--context", ALLOWED_CONTEXT, "--for=condition=Established", "--timeout=30s", "crd/applications.argoproj.io"], env=kube, deadline=deadline, timeout=40)
            fixture = _write_fixture(work, source_truth=combined)
            record_command(receipt, "fixture-apply", [kubectl, "apply", "--context", ALLOWED_CONTEXT, "-f", str(fixture)], env=kube, deadline=deadline)
            record_command(receipt, "fixture-service-account", [kubectl, "-n", NAMESPACE, "create", "serviceaccount", "trace-denied", "--context", ALLOWED_CONTEXT], env=kube, deadline=deadline)
            token = record_command(receipt, "denied-token", [kubectl, "-n", NAMESPACE, "create", "token", "trace-denied", "--duration=10m", "--context", ALLOWED_CONTEXT], env=kube, deadline=deadline, secret=True)
            token_value = token["stdout"].strip()
            if not token_value:
                raise RuntimeError("service-account token response was empty")
            config_output = record_command(receipt, "private-config-read", [kubectl, "config", "view", "--raw", "-o", "json"], env=kube, deadline=deadline, secret=True)
            config = json.loads(config_output["stdout"])
            config.setdefault("users", []).append({"name": "trace-denied-user", "user": {"token": token_value}})
            config.setdefault("contexts", []).append({"name": DENIED_CONTEXT, "context": {"cluster": "kind-" + cluster, "user": "trace-denied-user", "namespace": NAMESPACE}})
            for context in config["contexts"]:
                if context.get("name") == ALLOWED_CONTEXT:
                    context.setdefault("context", {})["namespace"] = NAMESPACE
            config["current-context"] = ALLOWED_CONTEXT
            private_config.write_text(json.dumps(config) + "\n")
            os.chmod(private_config, 0o600)
            for user in config.get("users", []):
                value = user.get("user", {})
                if any(key in value for key in ("exec", "auth-provider", "tokenFile", "client-certificate", "client-key")):
                    raise RuntimeError("private proof kubeconfig contains a helper or credential file reference")
            inventory = record_command(receipt, "fixture-inventory", [kubectl, "--context", ALLOWED_CONTEXT, "-n", NAMESPACE,
                                "get", "deployments,applications.argoproj.io", "-o", "json"], env=kube, deadline=deadline)
            items = json.loads(inventory["stdout"])["items"]
            want = {("Deployment", DEPLOYMENT), ("Deployment", NATIVE_DEPLOYMENT), ("Application", APPLICATION)}
            got = {(item["kind"], item["metadata"]["name"]) for item in items}
            if len(items) != 3 or got != want or any(item["metadata"].get("namespace") != NAMESPACE or not item["metadata"].get("uid") for item in items):
                raise RuntimeError("owned fixture differs from exact Application/Deployment identities")
            receipt["fixtureIdentities"] = [{"kind": item["kind"], "namespace": item["metadata"]["namespace"],
                                              "name": item["metadata"]["name"], "uid": item["metadata"]["uid"]} for item in items]
            receipt["fixtureScope"] = ("synthetic Argo Application CRD/object, Argo-labeled zero-replica Deployment with exact ConfigHub unit metadata, and unlabelled Native control; no Argo controller/reconciliation" if combined else
                "synthetic Argo Application CRD/object, Argo-labeled zero-replica Deployment, and unlabelled zero-replica Native control; no Argo controller/reconciliation")
            if combined:
                workloads = {item["metadata"]["name"]: item for item in items if item["kind"] == "Deployment"}
                target_object = workloads.get(DEPLOYMENT, {})
                labels = target_object.get("metadata", {}).get("labels", {})
                annotations = target_object.get("metadata", {}).get("annotations", {})
                app_object = next((item for item in items if item["kind"] == "Application" and item["metadata"]["name"] == APPLICATION), {})
                if labels.get("confighub.com/UnitSlug") != CONFIGHUB_UNIT or annotations.get("confighub.com/SpaceName") != CONFIGHUB_SPACE:
                    raise RuntimeError("source-truth fixture lacks exact ConfigHub unit identity")
                if app_object.get("status", {}).get("resources") != [{"group": "apps", "version": "v1", "kind": "Deployment", "namespace": NAMESPACE, "name": DEPLOYMENT}]:
                    raise RuntimeError("source-truth fixture lacks the exact managed workload identity")
                if app_object.get("status", {}).get("sync", {}).get("revision") != "a" * 40:
                    raise RuntimeError("source-truth fixture lacks its observed Argo revision")
                proxies, projected, proxy_cert_paths = _proxy_pair(config, work)
                observation_config.write_text(json.dumps(projected) + "\n")
                observation_config.chmod(0o600)
                cli_env["KUBECONFIG"] = str(observation_config)
                receipt["apiBindingMode"] = "two loopback endpoints; credentials held by forwarding proxy; upstream TLS CA verified; GET-only exact route allowlist"
                receipt["proxyEndpoints"] = {proxy.label: proxy.endpoint for proxy in proxies}
                receipt["observationKubeconfigSha256BeforeReads"] = digest(observation_config)
                rendered = _write_rendered_inputs(work)
                event_log = output / "api-events.jsonl"
                binary = str(binaries / "cub-scout-combined")
                _run_combined_action_flow(receipt=receipt, binary=binary, tui_binary=str(tui_binary),
                    cli_env=cli_env, proxies=proxies,
                    cub_log=cub_log, event_log=event_log, rendered=rendered, tui_config=tui_config,
                    deadline=deadline, output=output)
                receipt["observationKubeconfigSha256AfterReads"] = digest(observation_config)
                receipt["privateKubeconfigUnchangedDuringCLI"] = (
                    receipt["observationKubeconfigSha256BeforeReads"] == receipt["observationKubeconfigSha256AfterReads"])
                if not receipt["privateKubeconfigUnchangedDuringCLI"]:
                    raise RuntimeError("combined observation changed its credential-free kubeconfig")
            else:
                receipt["privateKubeconfigSha256BeforeReads"] = digest(private_config)
                for command, args_for_command in (
                    ("normal", ["trace", "deployment/" + DEPLOYMENT, "-n", NAMESPACE, "--format", "json"]),
                    ("reverse", ["trace", "deployment/" + DEPLOYMENT, "-n", NAMESPACE, "--reverse", "--json"]),
                ):
                    binary = str(binaries / "cub-scout-old")
                    record_observation(receipt, "old-ambient-" + command, command, [binary, *args_for_command], env=cli_env, deadline=deadline)
                    record_observation(receipt, "old-explicit-" + command, command,
                                       [binary, *args_for_command, "--kube-context", DENIED_CONTEXT], env=cli_env, deadline=deadline)
                    fixed = str(binaries / "cub-scout-fixed")
                    for context in (ALLOWED_CONTEXT, DENIED_CONTEXT):
                        record_observation(receipt, fixed_phase(context, command), command,
                                           [fixed, *args_for_command, "--kube-context", context], env=cli_env, deadline=deadline)
                fixed = str(binaries / "cub-scout-fixed")
                native_args = ["trace", "deployment/" + NATIVE_DEPLOYMENT, "-n", NAMESPACE, "--reverse", "--format", "json"]
                for context in (ALLOWED_CONTEXT, DENIED_CONTEXT):
                    result_phase = "fixed-allowed-native-reverse" if context == ALLOWED_CONTEXT else "fixed-denied-native-reverse"
                    record_observation(receipt, result_phase, "reverse",
                                       [fixed, *native_args, "--kube-context", context], env=cli_env, deadline=deadline)
                validate_observations(receipt["commands"])
                receipt["privateKubeconfigSha256AfterCLI"] = digest(private_config)
                receipt["privateKubeconfigUnchangedDuringCLI"] = receipt["privateKubeconfigSha256BeforeReads"] == receipt["privateKubeconfigSha256AfterCLI"]
                if not receipt["privateKubeconfigUnchangedDuringCLI"]:
                    raise RuntimeError("Trace CLI changed its private kubeconfig")

                shutil.copyfile(private_config, tui_config)
                tui_config.chmod(0o600)
                tui_result = output / "tui-result.json"
                tui_env = {**observation_env(observation_env_base, private_home=private_home, shims=shims, kubeconfig=tui_config),
                           "SCOUT_TRACE_OLD_CONTEXT": ALLOWED_CONTEXT, "SCOUT_TRACE_TUI_EXECUTE": "1",
                           "SCOUT_TRACE_TUI_CONFIG": str(tui_config), "SCOUT_TRACE_TUI_RESULT": str(tui_result)}
                record_command(receipt, "tui-live", [str(tui_binary), "-test.run", "^TestTraceContextOwnedTUI$",
                               "-test.count=1", "-test.v", "-test.timeout=90s"], env=tui_env, deadline=deadline, timeout=100)
                tui_data = json.loads(tui_result.read_text())
                receipt["tuiResultSha256"] = digest(tui_result)
                validate_tui(tui_data)
                receipt["tuiKubeconfigSha256BeforeAndAfter"] = tui_data.get("privateConfigSha256BeforeAfter")
            receipt["acceptance"] = "passed"
        except BaseException as exc:
            receipt["acceptance"] = "failed"
            receipt["error"] = type(exc).__name__ + ": " + str(exc)
            raise
    except BaseException as exc:
        receipt["acceptance"] = "failed"
        receipt["error"] = type(exc).__name__ + ": " + str(exc)
        raise
    finally:
        for signum, handler in signal_handlers.items():
            signal.signal(signum, handler)
        for proxy in reversed(proxies):
            try:
                proxy.close()
            except Exception:
                cleanup_errors.append("owned loopback API proxy did not stop cleanly")
        cleanup_env = {**env, "KUBECONFIG": str(private_config)} if env and private_config else env
        if cluster is not None and tools.get("kind") and tools.get("docker"):
            cleanup_errors.extend(cleanup_cluster(receipt, name=cluster, created=created,
                                                   creation_attempted=creation_attempted, tools=tools,
                                                   env=cleanup_env, deadline=time.monotonic() + 120))
        if tools.get("git"):
            cleanup_errors.extend(cleanup_worktrees(receipt, repo=REPO, git=tools["git"], worktrees=worktrees, env=env))
        for path in (private_config, observation_config, tui_config, *proxy_cert_paths):
            try:
                if path is not None:
                    path.unlink(missing_ok=True)
            except OSError:
                cleanup_errors.append("private credentials removal failed: " + path.name)
        try:
            receipt["sharedKubeconfigSha256After"] = digest(shared)
            receipt["sharedKubeconfigUnchanged"] = receipt["sharedKubeconfigSha256Before"] == receipt["sharedKubeconfigSha256After"]
        except OSError:
            receipt["sharedKubeconfigUnchanged"] = False
            cleanup_errors.append("shared kubeconfig integrity check failed")
        receipt["clusterCreationAttempted"] = creation_attempted
        receipt["clusterCreationSucceeded"] = created
        if cleanup_errors or not receipt.get("sharedKubeconfigUnchanged") or (created and not receipt.get("privateKubeconfigUnchangedDuringCLI")):
            receipt["acceptance"] = "failed"
        receipt["cleanupErrors"] = cleanup_errors
        receipt["retainedWorkDirectory"] = str(work) if work else None
        (output / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
        os.chmod(output / "receipt.json", 0o600)
    if cleanup_errors or not receipt.get("sharedKubeconfigUnchanged") or not receipt.get("privateKubeconfigUnchangedDuringCLI"):
        raise RuntimeError("proof cleanup or config integrity failed; inspect the receipt")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print("capture failed: " + str(exc), file=__import__("sys").stderr)
        raise SystemExit(1)
