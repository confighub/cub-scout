#!/usr/bin/env python3
"""Prepare an opt-in, serial before/after owned-kind Trace context proof.

No cluster, provider, model, or auth operation occurs unless --execute is given.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import shlex
import shutil
import signal
import subprocess
import tempfile
import time
import uuid

REPO = Path(__file__).resolve().parents[2]
OLD_SOURCE = "8cdb27b0bc9db17f5c0d1b59628b67cd3936ed6c"
FIXED_SOURCE = "3a056e5e72d7236badc76d6d0090cbcb1752aba0"
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
        deadline: float | None = None, max_output: int = MAX_OUTPUT) -> dict:
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
        proc = subprocess.Popen(argv, env=env, stdin=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                start_new_session=True)
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


def observation_env(base: dict[str, str], *, private_home: Path, shims: Path,
                    kubeconfig: Path) -> dict[str, str]:
    """Keep product commands inside a private HOME/XDG and fail-closed PATH."""
    return {**base, "PATH": str(shims), "HOME": str(private_home),
            "XDG_CONFIG_HOME": str(private_home / ".config"),
            "XDG_CACHE_HOME": str(private_home / ".cache"),
            "XDG_DATA_HOME": str(private_home / ".local" / "share"),
            "KUBECONFIG": str(kubeconfig)}


def create_observation_shims(shims: Path, kubectl_path: str) -> dict[str, Path]:
    """Block all external tools except one exact private-config Argo fallback GET."""
    kubectl = shlex.quote(kubectl_path)
    specs = {
        "argocd": "#!/bin/sh\necho 'FATA[0000] server address unspecified' >&2\nexit 1\n",
        "kubectl": ("#!/bin/sh\nif [ \"$#\" -eq 5 ] && [ \"$1\" = get ] && [ \"$2\" = applications.argoproj.io ] && "
                    "[ \"$3\" = --all-namespaces ] && [ \"$4\" = -o ] && [ \"$5\" = json ]; then\n"
                    f"  exec {kubectl} \"$@\" --context \"$SCOUT_TRACE_OLD_CONTEXT\"\nfi\n"
                    "echo 'refusing unexpected owned kubectl operation' >&2; exit 97\n"),
        "flux": "#!/bin/sh\necho 'blocked external Flux command' >&2\nexit 97\n",
        "helm": "#!/bin/sh\necho 'blocked external Helm command' >&2\nexit 97\n",
        "cub": "#!/bin/sh\necho 'blocked ConfigHub command' >&2\nexit 97\n",
    }
    paths = {}
    for name, contents in specs.items():
        path = shims / name
        path.write_text(contents)
        path.chmod(0o700)
        paths[name] = path
    return paths


def _write_fixture(work: Path) -> Path:
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
    return fixture


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--integrity-only-shared-kubeconfig", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
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
    tui_config = None
    worktrees: list[Path] = []
    tools: dict[str, str | None] = {}
    env: dict[str, str] = {}
    cluster = None
    created = False
    creation_attempted = False
    signal_handlers = {}
    cleanup_errors: list[str] = []
    receipt = {"schema": "trace-context-owned-kind.v1", "oldSource": OLD_SOURCE,
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
        binaries = work / "bin"
        binaries.mkdir(mode=0o700)
        private_home = work / "home"
        private_home.mkdir(mode=0o700)
        env = {key: value for key, value in os.environ.items()
               if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
        env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
                    "CUB_SCOUT_OFFLINE": "true", "CUB_SCOUT_SCAN_PROVIDER": "legacy",
                    "KUBECONFIG": str(private_config)})
        for key in ("CUB_SPACE", "CUB_SCOUT_TEST_TRACE_JSON", "CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON",
                    "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
            env.pop(key, None)
        shims = work / "shims"
        shims.mkdir(mode=0o700)
        observation_env_base = {**env, "SCOUT_TRACE_OLD_CONTEXT": ALLOWED_CONTEXT}
        shim_paths = create_observation_shims(shims, tools["kubectl"])
        cli_env = observation_env(observation_env_base, private_home=private_home, shims=shims, kubeconfig=private_config)
        receipt["observationShims"] = {name: digest(path) for name, path in shim_paths.items()}
        deadline = time.monotonic() + MAX_SECONDS
        def interrupt(signum, _frame):
            raise InterruptedError("capture interrupted by signal " + str(signum))
        for signum in (signal.SIGINT, signal.SIGTERM):
            signal_handlers[signum] = signal.signal(signum, interrupt)

        clean = record_command(receipt, "source-clean", [tools["git"], "-C", str(REPO), "status", "--porcelain"], env=env, deadline=deadline)
        if clean["stdout"].strip():
            raise RuntimeError("capture checkout must be committed and clean")
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

        # Build both exact product sources and compile the source-injected TUI probe
        # before asking kind to create anything.
        for label, ref in (("old", OLD_SOURCE), ("fixed", FIXED_SOURCE)):
            checkout = work / ("source-" + label)
            worktrees.append(checkout)
            record_command(receipt, label + "-worktree", [tools["git"], "-C", str(REPO), "worktree", "add", "--detach", str(checkout), ref], env=env, deadline=deadline)
            binary = binaries / ("cub-scout-" + label)
            record_command(receipt, label + "-build", [tools["go"], "-C", str(checkout), "build", "-o", str(binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
            commit = record_command(receipt, label + "-source", [tools["git"], "-C", str(checkout), "rev-parse", "HEAD"], env=env, deadline=deadline)["stdout"].strip()
            if commit != ref:
                raise RuntimeError(label + " binary does not match the exact source pin")
            receipt[label + "SourceCommit"] = commit
            receipt[label + "BinarySha256"] = digest(binary)
        template = Path(__file__).with_name("tui_live_test.go.txt")
        receipt["tuiProbeSha256"] = digest(template)
        probe = work / "source-fixed" / "cmd" / "cub-scout" / "trace_context_owned_live_test.go"
        with probe.open("x") as stream:
            stream.write(template.read_text())
        tui_binary = binaries / "trace-context-tui.test"
        record_command(receipt, "tui-build", [tools["go"], "-C", str(work / "source-fixed"), "test", "-c", "-o", str(tui_binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
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
            fixture = _write_fixture(work)
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
            receipt["fixtureScope"] = "synthetic Argo Application CRD/object, Argo-labeled zero-replica Deployment, and unlabelled zero-replica Native control; no Argo controller/reconciliation"
            receipt["privateKubeconfigSha256BeforeReads"] = digest(private_config)

            for command, args_for_command in (
                ("normal", ["trace", "deployment/" + DEPLOYMENT, "-n", NAMESPACE, "--format", "json"]),
                ("reverse", ["trace", "deployment/" + DEPLOYMENT, "-n", NAMESPACE, "--reverse", "--format", "json"]),
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
        cleanup_env = {**env, "KUBECONFIG": str(private_config)} if env and private_config else env
        if cluster is not None and tools.get("kind") and tools.get("docker"):
            cleanup_errors.extend(cleanup_cluster(receipt, name=cluster, created=created,
                                                   creation_attempted=creation_attempted, tools=tools,
                                                   env=cleanup_env, deadline=time.monotonic() + 120))
        if tools.get("git"):
            cleanup_errors.extend(cleanup_worktrees(receipt, repo=REPO, git=tools["git"], worktrees=worktrees, env=env))
        for path in (private_config, tui_config):
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
