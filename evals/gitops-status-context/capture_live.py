#!/usr/bin/env python3
"""Opt-in owned-kind #753 GitOps context proof.

This packet shares the doctor-scan-context command runner and evidence helpers.
It is intentionally a separate mode so the existing #743 receipt contract and
fixture remain unchanged. Live execution requires --execute.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import sys
import tempfile
import time
import uuid
from urllib.parse import urlsplit

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
FIXED_SOURCE = "00e1375e0380ccf809ee4ef6f22abd47ca3119d1"
OLD_SOURCE = "4d3f28e9c4ab912736811c3b4896781f0a55b56b"
NAMESPACE = "scout-gitops-context-proof"
ALLOWED = "doctor-allowed"
CONTROLLER_DENIED = "doctor-controller-denied"
PODS_DENIED = "doctor-pods-denied"
DENIED_USER = "system:serviceaccount:" + NAMESPACE + ":controller-denied"
PODS_DENIED_USER = "system:serviceaccount:" + NAMESPACE + ":pods-denied"
MAX_SECONDS = 600

# Import the already-reviewed bounded runner, receipt writer primitives and
# pinned local tool requirements from the sibling owned-kind proof.
_spec = importlib.util.spec_from_file_location(
    "doctor_scan_capture_shared", REPO / "evals/doctor-scan-context/capture.py")
if _spec is None or _spec.loader is None:
    raise RuntimeError("shared doctor-scan capture helpers are unavailable")
_shared = importlib.util.module_from_spec(_spec)
sys.modules[_spec.name] = _shared
_spec.loader.exec_module(_shared)
run = _shared.run
succeeded = _shared.succeeded
record_command = _shared.record_command
record_observation = _shared.record_observation
digest = _shared.digest
KIND_VERSION = _shared.KIND_VERSION
NODE_IMAGE = _shared.NODE_IMAGE

# Exact dynamic-client reads at FIXED_SOURCE for this inert Application fixture.
# No discovery or arbitrary selectors are used by this status collector.
CONTROLLER_PATHS = tuple(
    f"/apis/{group}/{version}/namespaces/{NAMESPACE}/{resource}"
    for group, version, resources in (
        ("helm.toolkit.fluxcd.io", "v2", ("helmreleases",)),
        ("kustomize.toolkit.fluxcd.io", "v1", ("kustomizations",)),
        ("argoproj.io", "v1alpha1", ("applications",)),
        ("fluxcd.controlplane.io", "v1", ("fluxinstances", "fluxreports", "resourcesets",
         "resourcesetinputproviders", "externalartifacts", "artifactgenerators")),
        ("config.projectsveltos.io", "v1beta1", ("profiles", "clustersummaries",
         "clusterconfigurations", "clusterreports")),
        ("lib.projectsveltos.io", "v1beta1", ("healthcheckreports", "eventreports")),
        ("modelplane.ai", "v1alpha1", ("inferencegateways", "inferenceclasses", "inferenceclusters",
         "modeldeployments", "modelservices", "modelendpoints", "modelcaches", "modelreplicas")),
        ("infrastructure.modelplane.ai", "v1alpha1", ("eksclusters", "gkeclusters", "servingstacks")),
        ("apps", "v1", ("deployments",)),
    ) for resource in resources
) + tuple(f"/apis/{group}/v1beta1/{resource}" for group, resource in (
    ("config.projectsveltos.io", "clusterprofiles"),
    ("config.projectsveltos.io", "clusterpromotions"),
    ("lib.projectsveltos.io", "eventsources"),
    ("lib.projectsveltos.io", "eventtriggers"),
    ("lib.projectsveltos.io", "clusterhealthchecks"),
)) + (
    f"/api/v1/namespaces/{NAMESPACE}/pods",
    f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/scout-context-app",
    f"/apis/source.toolkit.fluxcd.io/v1/namespaces/{NAMESPACE}/gitrepositories/scout-context-app",
)
_trace_spec = importlib.util.spec_from_file_location(
    "trace_context_api_proxy", REPO / "evals/trace-context-live/api_proxy.py")
if _trace_spec is None or _trace_spec.loader is None:
    raise RuntimeError("shared owned-kind read-only proxy is unavailable")
_trace_proxy = importlib.util.module_from_spec(_trace_spec)
sys.modules[_trace_spec.name] = _trace_proxy
_trace_spec.loader.exec_module(_trace_proxy)

_lifecycle_spec = importlib.util.spec_from_file_location(
    "trace_context_capture_lifecycle", REPO / "evals/trace-context-live/capture.py")
if _lifecycle_spec is None or _lifecycle_spec.loader is None:
    raise RuntimeError("reviewed lifecycle helpers are unavailable")
_lifecycle = importlib.util.module_from_spec(_lifecycle_spec)
sys.modules[_lifecycle_spec.name] = _lifecycle
# The reviewed script has a direct-script fallback import. Bind its dependency
# only while loading it, without inheriting or extending an ambient PATH.
_previous_proxy = sys.modules.get("api_proxy")
sys.modules["api_proxy"] = _trace_proxy
try:
    _lifecycle_spec.loader.exec_module(_lifecycle)
finally:
    if _previous_proxy is None:
        sys.modules.pop("api_proxy", None)
    else:
        sys.modules["api_proxy"] = _previous_proxy


def observation_env(*, private_home: Path, shims: Path, kubeconfig: Path, cub_log: Path) -> dict:
    # Explicit allowlist: no ambient HOME, token, plugin, kube, proxy or hook env.
    return _lifecycle.observation_env(
        {"LANG": "C", "TMPDIR": str(private_home), "SCOUT_GITOPS_CUB_LOG": str(cub_log)},
        private_home=private_home, shims=shims, kubeconfig=kubeconfig)


def create_observation_shims(shims: Path) -> Path:
    # Unexpected arguments are never echoed: they may contain a secret. A
    # rejection marker is sufficient to fail the exact per-action contract.
    path = shims / "cub"
    path.write_text("""#!/bin/sh
if [ "$#" -eq 2 ] && [ "$1" = auth ] && [ "$2" = status ]; then
  printf '%s\\n' '{"type":"call","argv":["cub","auth","status"],"exitCode":73}' >> "$SCOUT_GITOPS_CUB_LOG"
  echo 'synthetic unauthenticated session in owned proof' >&2
  exit 73
fi
printf '%s\\n' '{"type":"rejected","argvRedacted":true,"exitCode":97}' >> "$SCOUT_GITOPS_CUB_LOG"
exit 97
""")
    path.chmod(0o700)
    return path


def validate_cub_records(path: Path, *, old_bytes: int, expected_calls: int) -> list[dict]:
    raw = path.read_bytes() if path.exists() else b""
    if len(raw) > 1024 * 1024 or old_bytes > len(raw):
        raise RuntimeError("cub argv log exceeded bound or was truncated")
    try:
        rows = [json.loads(line) for line in raw[old_bytes:].splitlines()]
    except (ValueError, UnicodeError):
        raise RuntimeError("cub argv log is malformed") from None
    expected = {"type": "call", "argv": ["cub", "auth", "status"], "exitCode": 73}
    if rows != [expected] * expected_calls:
        raise RuntimeError("cub calls differed from exact synthetic auth-status contract")
    return rows


def snapshot_proxies(proxies: list, *, clear: bool = False) -> list[dict]:
    return [row for proxy in proxies for row in proxy.snapshot(clear=clear)]


def allowed_status_request(path: str, query: dict) -> bool:
    return path in _allowed_paths() and not query


class StatusAPIProxy(_trace_proxy.ReadOnlyAPIProxy):
    def allowed_path(self, path: str, query: dict) -> bool:
        return allowed_status_request(path, query)


def write_private(path: Path, text: str, *, immutable: bool = False) -> None:
    with path.open("x") as stream:
        os.chmod(path, 0o600)
        stream.write(text)
    if immutable:
        path.chmod(0o400)


def verify_immutable_configs(receipt: dict, configs: dict[Path, str]) -> None:
    receipt["observationConfigs"] = {path.name: {"before": before, "after": digest(path)}
                                     for path, before in configs.items()}
    if any(row["before"] != row["after"] for row in receipt["observationConfigs"].values()):
        raise RuntimeError("observation kubeconfig changed during reads")


def validate_status_json(raw: str, *, context: str, result: str) -> dict:
    try:
        body = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        raise RuntimeError("GitOps observation is not JSON") from None
    if not isinstance(body, dict) or body.get("context") != context:
        raise RuntimeError("GitOps status lost the selected context label")
    deployers = body.get("deployers")
    if not isinstance(deployers, list):
        raise RuntimeError("GitOps status omitted deployer rows")
    app = next((item for item in deployers if isinstance(item, dict) and
                item.get("kind") == "Application" and item.get("name") == "scout-context-app" and
                item.get("namespace") == NAMESPACE), None)
    if app is None:
        raise RuntimeError("GitOps status omitted the owned synthetic Application")
    if result == "allowed":
        if app.get("healthStatus") != "Healthy" or app.get("ready") is not True:
            raise RuntimeError("allowed controller evidence did not retain Healthy Application state")
    elif result == "controller-denied":
        coverage = body.get("controllerCoverage")
        if not isinstance(coverage, list):
            raise RuntimeError("controller-denied result omitted coverage")
        modelplane = next((item for item in coverage if isinstance(item, dict) and
                           item.get("family") == "Modelplane"), None)
        if modelplane is None or modelplane.get("status") != "unreadable" or not any(
                isinstance(omission, dict) and omission.get("reason") == "forbidden"
                for omission in modelplane.get("omissions", [])):
            raise RuntimeError("controller denial was hidden or treated as healthy absence")
        if not any(isinstance(omission, dict) and DENIED_USER in str(omission.get("message", ""))
                   for omission in modelplane.get("omissions", [])):
            raise RuntimeError("controller omission did not identify the selected denied principal")
    elif result == "pods-denied":
        omission = app.get("runtimeOmission")
        if app.get("healthStatus") != "Healthy" or app.get("ready") is not True:
            raise RuntimeError("pod denial overwrote controller-reported Application health")
        if not isinstance(omission, dict) or omission.get("resource") != "pods" or omission.get("reason") != "forbidden":
            raise RuntimeError("pod denial was not surfaced as runtimeOmission")
        if app.get("podTotal", 0) != 0 or app.get("runtimeIssues"):
            raise RuntimeError("denied Pod read was misrepresented as observed runtime state")
    else:
        raise RuntimeError("unknown GitOps observation result")
    return body


def validate_api_records(rows: list[dict], *, expected_context: str,
                         expected_denial: str | None = None) -> None:
    if not rows:
        raise RuntimeError("GitOps action has no captured Kubernetes API requests")
    for row in rows:
        if row.get("context") != expected_context or row.get("method") != "GET":
            raise RuntimeError("Kubernetes request escaped the selected read-only context")
        if row.get("disposition") != "forwarded":
            raise RuntimeError("Kubernetes request was refused by the proof proxy")
        path = row.get("path")
        if path not in _allowed_paths():
            raise RuntimeError("Kubernetes request path is outside the GitOps allowlist")
        status = row.get("status")
        if type(status) is not int or status not in (200, 403, 404):
            raise RuntimeError("Kubernetes request returned an unexpected status")
        if status == 403 and path != expected_denial:
            raise RuntimeError("unexpected forbidden Kubernetes request")
    required = {
        f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications": 200,
        f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/scout-context-app": 200,
        f"/api/v1/namespaces/{NAMESPACE}/pods": 200,
    }
    if expected_denial:
        required[expected_denial] = 403
    for path, status in required.items():
        matches = [row for row in rows if row.get("path") == path]
        if not matches or any(row.get("status") != status for row in matches):
            raise RuntimeError("exact Application/runtime read evidence is missing or inconsistent")
    if expected_denial:
        if not any(row.get("path") == expected_denial and row.get("status") == 403 for row in rows):
            raise RuntimeError("expected exact RBAC denial was not recorded")


def validate_mcp_result(response: dict, *, context: str, result: str) -> dict:
    if not isinstance(response, dict) or response.get("error") is not None:
        raise RuntimeError("MCP tools/call returned a JSON-RPC error")
    payload = response.get("result")
    if not isinstance(payload, dict) or payload.get("isError") is True:
        raise RuntimeError("MCP tools/call returned an error result")
    return validate_status_json(json.dumps(_trace_proxy.mcp_result_json(response)), context=context, result=result)


def validate_tui_probe(data: dict, *, context: str, result: str) -> None:
    if data.get("schema") != "gitops-status-context-tui.v1" or data.get("passed") is not True:
        raise RuntimeError("status TUI probe did not report a versioned pass")
    if data.get("context") != context or context not in (ALLOWED, CONTROLLER_DENIED, PODS_DENIED):
        raise RuntimeError("status TUI probe has an unknown selected context")
    view = data.get("view", "")
    if f"- Kubernetes context label: ` {context} ` (not a stable cluster ID)" not in view:
        raise RuntimeError("status TUI view omitted its selected context label")
    if result == "controller-denied" and ("forbidden" not in view.lower() or "unreadable" not in view.lower()):
        raise RuntimeError("status TUI hid controller API denial")
    if result == "pods-denied" and ("pods: forbidden" not in view or "Healthy" not in view):
        raise RuntimeError("status TUI hid runtime omission or controller health")
    if not all(data.get("checks", {}).get(name) is True for name in ("resize", "scroll", "scrollBack", "quit", "immutableSummary")):
        raise RuntimeError("status TUI probe omitted actual model actions")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--integrity-only-shared-kubeconfig", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    if not args.execute:
        parser.error("refusing to run without --execute")
    return _execute_capture(args, parser)


def _execute_capture(args, parser) -> int:
    """Reviewed opt-in lifecycle; offline controls inject tools without live reads."""
    requested = args.integrity_only_shared_kubeconfig.expanduser().absolute()
    if requested.is_symlink():
        parser.error("integrity-only kubeconfig must not be a symlink")
    shared = requested.resolve(strict=True)
    if not shared.is_file():
        parser.error("integrity-only kubeconfig must be a regular file")
    output = args.output_dir.expanduser().absolute()
    if output.exists() or output.is_symlink() or ".." in args.output_dir.parts or not any(
            Path(base).resolve() == output.resolve() or Path(base).resolve() in output.resolve().parents
            for base in ("/tmp", "/var/tmp")):
        parser.error("output directory must be fresh and under /tmp or /var/tmp")
    output.mkdir(parents=True, mode=0o700)
    os.chmod(output, 0o700)

    tools = {name: shutil.which(name) for name in ("git", "go", "kind", "kubectl", "docker")}
    missing = [name for name, path in tools.items() if path is None]
    receipt = {"schema": "gitops-status-context-owned-kind.v1", "fixedSource": FIXED_SOURCE,
               "oldSource": OLD_SOURCE, "commands": [], "acceptance": "failed",
               "sharedKubeconfigSha256Before": hashlib.sha256(shared.read_bytes()).hexdigest()}
    cluster = "scout-gitops-" + uuid.uuid4().hex[:10]
    worktrees: list[Path] = []
    proxies = []
    proxy_cert_paths = []
    created = attempted = False
    cleanup_errors: list[str] = []
    signal_handlers = {}
    if missing:
        receipt["error"] = "required existing tools missing: " + ", ".join(missing)
        write_private(output / "receipt.json", json.dumps(receipt, indent=2) + "\n")
        raise RuntimeError(receipt["error"])
    deadline = time.monotonic() + MAX_SECONDS
    temp_parent = None
    private_config = None
    env = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME", "LANG", "LC_ALL")}
    env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "PYTHONDONTWRITEBYTECODE": "1"})
    configs: dict[Path, str] = {}
    receipt["observations"] = []
    try:
        temp_parent = Path(tempfile.mkdtemp(prefix="scout-gitops-context-", dir="/tmp"))
        os.chmod(temp_parent, 0o700)
        private_config = temp_parent / "kubeconfig"
        env["KUBECONFIG"] = str(private_config)
        private_home = temp_parent / "observation-home"
        private_home.mkdir(mode=0o700)
        shims = temp_parent / "shims"
        shims.mkdir(mode=0o700)
        cub_log = output / "cub-shim.jsonl"
        shim = create_observation_shims(shims)
        receipt["observationShimSha256"] = digest(shim)
        receipt["helperHashes"] = {str(path.relative_to(REPO)): digest(path) for path in (
            Path(__file__), REPO / "evals/doctor-scan-context/capture.py",
            REPO / "evals/trace-context-live/capture.py", REPO / "evals/trace-context-live/api_proxy.py")}
        def interrupt(signum, _frame):
            raise InterruptedError("capture interrupted by signal " + str(signum))
        for signum in (signal.SIGINT, signal.SIGTERM):
            signal_handlers[signum] = signal.signal(signum, interrupt)
        clean = record_command(receipt, "source-clean", [tools["git"], "-C", str(REPO), "status", "--porcelain"], env=env, deadline=deadline)
        if clean["stdout"].strip():
            raise RuntimeError("capture checkout must be committed and clean")
        record_command(receipt, "fixed-source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", FIXED_SOURCE, "HEAD"], env=env, deadline=deadline)
        record_command(receipt, "old-source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", OLD_SOURCE, FIXED_SOURCE], env=env, deadline=deadline)
        if os.environ.get("DOCKER_HOST"):
            raise RuntimeError("DOCKER_HOST is set; refusing a remote Docker engine")
        version = record_command(receipt, "kind-version", [tools["kind"], "version"], env=env, deadline=deadline)["stdout"].split()
        if len(version) < 2 or version[:2] != ["kind", KIND_VERSION]:
            raise RuntimeError("kind version differs from pinned requirement")
        docker_context = record_command(receipt, "docker-context", [tools["docker"], "context", "show"], env=env, deadline=deadline)["stdout"].strip()
        endpoint = record_command(receipt, "docker-endpoint", [tools["docker"], "context", "inspect", docker_context, "--format", "{{json .Endpoints.docker.Host}}"], env=env, deadline=deadline)
        if not json.loads(endpoint["stdout"]).startswith("unix://"):
            raise RuntimeError("refusing non-local Docker endpoint")
        image = record_command(receipt, "cached-node-image", [tools["docker"], "image", "inspect", NODE_IMAGE], env=env, deadline=deadline)
        digests = json.loads(image["stdout"])[0].get("RepoDigests", [])
        if not any(item.rsplit("@", 1)[-1] == NODE_IMAGE.rsplit("@", 1)[-1] for item in digests):
            raise RuntimeError("pinned Kubernetes node image is not already cached")
        receipt["captureScriptSha256"] = digest(Path(__file__))
        receipt["toolPins"] = {name: {"path": path, "sha256": digest(Path(path))} for name, path in tools.items()}
        for label, ref in (("old", OLD_SOURCE), ("fixed", FIXED_SOURCE)):
            checkout = temp_parent / ("source-" + label)
            worktrees.append(checkout)
            record_command(receipt, label + "-worktree", [tools["git"], "-C", str(REPO), "worktree", "add", "--detach", str(checkout), ref], env=env, deadline=deadline)
            binary = temp_parent / ("cub-scout-" + label)
            record_command(receipt, label + "-build", [tools["go"], "-C", str(checkout), "build", "-o", str(binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
            source = record_command(receipt, label + "-source", [tools["git"], "-C", str(checkout), "rev-parse", "HEAD"], env=env, deadline=deadline)["stdout"].strip()
            if source != ref:
                raise RuntimeError("built source differs from product pin")
            receipt[label + "SourceCommit"] = source
            receipt[label + "BinarySha256"] = digest(binary)
        # Fixed-source test binary drives the real status collector and summary viewport.
        probe = HERE / "gitops_status_tui_live_test.go.txt"
        receipt["tuiProbeSha256"] = digest(probe)
        target = worktrees[-1] / "cmd/cub-scout" / "gitops_status_context_live_test.go"
        target.write_text(probe.read_text())
        tui_binary = temp_parent / "gitops-status-tui.test"
        record_command(receipt, "tui-build", [tools["go"], "-C", str(worktrees[-1]), "test", "-c", "-o", str(tui_binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)

        receipt["tuiBinarySha256"] = digest(tui_binary)

        record_command(receipt, "cluster-name-check", [tools["kind"], "get", "clusters"], env=env, deadline=deadline)
        receipt["ownedCluster"] = cluster
        attempted = True
        record_command(receipt, "create-owned-cluster", [tools["kind"], "create", "cluster", "--name", cluster,
                       "--image", NODE_IMAGE, "--kubeconfig", str(private_config), "--wait", "90s"],
                       env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline, timeout=150)
        created = True
        os.chmod(private_config, 0o600)
        receipt["clusterCreationSucceeded"] = True

        # Fixture and role creation are the only cluster mutations. The app has
        # no workload Pods; this proof reads synthetic controller state only.
        kubectl = tools["kubectl"]
        record_command(receipt, "rename-context", [kubectl, "config", "rename-context", "kind-" + cluster, ALLOWED], env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline)
        record_command(receipt, "fixture-namespace", [kubectl, "create", "namespace", NAMESPACE, "--context", ALLOWED], env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline)
        fixture = temp_parent / "fixture.yaml"
        crd_file = temp_parent / "application-crd.yaml"
        crd_file.write_text(f'''apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {{name: applications.argoproj.io}}
spec:
  group: argoproj.io
  scope: Namespaced
  names: {{plural: applications, singular: application, kind: Application}}
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema: {{openAPIV3Schema: {{type: object, x-kubernetes-preserve-unknown-fields: true}}}}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {{name: modeldeployments.modelplane.ai}}
spec:
  group: modelplane.ai
  scope: Namespaced
  names: {{plural: modeldeployments, singular: modeldeployment, kind: ModelDeployment}}
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema: {{openAPIV3Schema: {{type: object, x-kubernetes-preserve-unknown-fields: true}}}}
''')
        kube_env = {**env, "KUBECONFIG": str(private_config)}
        record_command(receipt, "fixture-crd", [kubectl, "apply", "--context", ALLOWED, "-f", str(crd_file)], env=kube_env, deadline=deadline)
        record_command(receipt, "fixture-crd-established", [kubectl, "wait", "--context", ALLOWED, "--for=condition=Established", "--timeout=45s", "crd/applications.argoproj.io"], env=kube_env, deadline=deadline, timeout=55)
        record_command(receipt, "modelplane-crd-established", [kubectl, "wait", "--context", ALLOWED, "--for=condition=Established", "--timeout=45s", "crd/modeldeployments.modelplane.ai"], env=kube_env, deadline=deadline, timeout=55)
        fixture = temp_parent / "fixture.yaml"
        write_private(fixture, _fixture_yaml(include_crd=False))
        receipt["fixtureSha256"] = digest(fixture)
        receipt["crdSha256"] = digest(crd_file)
        record_command(receipt, "fixture-apply", [kubectl, "apply", "--context", ALLOWED, "-f", str(fixture)], env=kube_env, deadline=deadline)
        raw_config = record_command(receipt, "private-context-bindings", [kubectl, "config", "view", "--raw", "-o", "json"], env=kube_env, deadline=deadline, secret=True)
        config = json.loads(raw_config["stdout"])
        config.setdefault("users", [])
        config.setdefault("contexts", [])
        # Short-lived ServiceAccount tokens are captured privately and every
        # token-producing command is redacted from the public receipt.
        for sa, role_yaml in (("controller-denied", _rbac_yaml("controller-denied", deny_modelplane=True)),
                              ("pods-denied", _rbac_yaml("pods-denied", deny_pods=True))):
            role_file = temp_parent / (sa + ".yaml")
            write_private(role_file, role_yaml)
            receipt.setdefault("rbacHashes", {})[sa] = digest(role_file)
            record_command(receipt, sa + "-rbac", [kubectl, "apply", "--context", ALLOWED, "-f", str(role_file)], env=kube_env, deadline=deadline)
            token = record_command(receipt, sa + "-token", [kubectl, "-n", NAMESPACE, "create", "token", sa, "--duration=10m", "--context", ALLOWED], env=kube_env, deadline=deadline, secret=True)["stdout"].strip()
            if not token or "\n" in token:
                raise RuntimeError("owned ServiceAccount token output was malformed")
            config["users"].append({"name": sa + "-user", "user": {"token": token}})
            config["contexts"].append({"name": CONTROLLER_DENIED if sa == "controller-denied" else PODS_DENIED, "context": {"cluster": "kind-" + cluster,
                                    "user": sa + "-user", "namespace": NAMESPACE}})
        # Credentials are created for this invocation-owned cluster, kept in
        # process memory, and written directly to its private file. Tokens
        # never appear in command arguments, environment or receipts.
        private_config.write_text(json.dumps(config) + "\n")
        private_config.chmod(0o600)
        clusters = {row["name"]: row["cluster"] for row in config.get("clusters", [])}
        users = {row["name"]: row["user"] for row in config.get("users", [])}
        contexts = {row["name"]: row["context"] for row in config.get("contexts", [])}
        for user in users.values():
            if any(key in user for key in ("exec", "auth-provider", "tokenFile", "client-certificate", "client-key")):
                raise RuntimeError("owned upstream config contains unsupported credential mechanism")
        for context_name, label in ((ALLOWED, "allowed"), (CONTROLLER_DENIED, "controller-denied"), (PODS_DENIED, "pods-denied")):
            binding = contexts[context_name]
            tls, authorization, certs = _trace_proxy.upstream_credentials(
                clusters[binding["cluster"]], {"user": users[binding["user"]]}, private_dir=temp_parent, label=label)
            proxy_cert_paths.extend(certs)
            upstream = clusters[binding["cluster"]]["server"]
            if urlsplit(upstream).hostname not in ("127.0.0.1", "localhost", "::1"):
                raise RuntimeError("owned kind API is not loopback-bound")
            proxy = StatusAPIProxy(label=context_name, upstream=upstream, tls=tls,
                authorization=authorization, namespace=NAMESPACE, deployment="scout-context-marker",
                application="scout-context-app", missing_deployment="scout-context-missing", event_log=output / "api-events.jsonl")
            proxies.append(proxy)
        receipt["privateKubeconfigSha256BeforeReads"] = digest(private_config)

        phases = receipt["observations"]
        old_env = observation_env(private_home=private_home, shims=shims, kubeconfig=temp_parent / "unused-old-config", cub_log=cub_log)
        old_cub_bytes = cub_log.stat().st_size if cub_log.exists() else 0
        old = record_observation(receipt, "old-explicit-selector", [str(temp_parent / "cub-scout-old"),
            "gitops", "status", "--namespace", NAMESPACE, "--kube-context", ALLOWED, "--format", "json"],
            env=old_env, deadline=deadline)
        validate_cub_records(cub_log, old_bytes=old_cub_bytes, expected_calls=0)
        if old["exitCode"] == 0 or "unknown flag: --kube-context" not in old["stderr"]:
            raise RuntimeError("old product did not reject the unsupported GitOps selector")

        for label, context, typ in (("allowed", ALLOWED, "allowed"),
                                    ("controller-denied", CONTROLLER_DENIED, "controller-denied"),
                                    ("pods-denied", PODS_DENIED, "pods-denied")):
            binary = str(temp_parent / "cub-scout-fixed")
            obs_config = temp_parent / (label + ".kubeconfig")
            write_private(obs_config, json.dumps(_proxy_kubeconfig(context, proxies)) + "\n", immutable=True)
            configs[obs_config] = digest(obs_config)
            obs_env = observation_env(private_home=private_home, shims=shims, kubeconfig=obs_config, cub_log=cub_log)
            proxy = next(item for item in proxies if item.label == context)
            snapshot_proxies(proxies, clear=True)
            _lifecycle._mark_api_phase(output / "api-events.jsonl", "cli-" + label)
            cub_bytes = cub_log.stat().st_size if cub_log.exists() else 0
            cli = record_observation(receipt, "cli-" + label, [binary, "gitops", "status", "--namespace", NAMESPACE, "--kube-context", context, "--format", "json"], env=obs_env, deadline=deadline)
            cub_rows = validate_cub_records(cub_log, old_bytes=cub_bytes, expected_calls=1)
            if not succeeded(cli):
                raise RuntimeError("CLI observation failed; inspect retained receipt")
            validate_status_json(cli["stdout"], context=context, result=typ)
            cli_rows = snapshot_proxies(proxies, clear=True)
            expected_denial = f"/apis/modelplane.ai/v1alpha1/namespaces/{NAMESPACE}/modeldeployments" if typ == "controller-denied" else f"/api/v1/namespaces/{NAMESPACE}/pods" if typ == "pods-denied" else None
            validate_api_records([{**row, "context": row.get("endpoint")} for row in cli_rows], expected_context=context, expected_denial=expected_denial)
            phases.append({"name": "cli-" + label, "context": context, "result": typ, "stdout": cli["stdout"], "apiRequests": cli_rows, "cubCalls": cub_rows})
            mcp_request = _mcp_call("gitops_status", {"context": context, "namespace": NAMESPACE})
            snapshot_proxies(proxies, clear=True)
            _lifecycle._mark_api_phase(output / "api-events.jsonl", "mcp-" + label)
            cub_bytes = cub_log.stat().st_size if cub_log.exists() else 0
            mcp = _run_mcp(binary, mcp_request, env=obs_env, deadline=deadline)
            receipt["commands"].append({"phase": "mcp-" + label, **mcp})
            cub_rows = validate_cub_records(cub_log, old_bytes=cub_bytes, expected_calls=3)
            if not succeeded(mcp):
                raise RuntimeError("MCP observation failed; inspect retained receipt")
            validate_mcp_result(_last_json_line(mcp["stdout"]), context=context, result=typ)
            mcp_rows = snapshot_proxies(proxies, clear=True)
            validate_api_records([{**row, "context": row.get("endpoint")} for row in mcp_rows], expected_context=context, expected_denial=expected_denial)
            phases.append({"name": "mcp-" + label, "context": context, "result": typ, "stdout": mcp["stdout"], "apiRequests": mcp_rows, "cubCalls": cub_rows})
        # TUI probe uses the same command collector and real summary model, then
        # sends resize, scroll and quit events. It does not claim terminal UX.
        for label, context, typ in (("allowed", ALLOWED, "allowed"),
                                    ("controller-denied", CONTROLLER_DENIED, "controller-denied"),
                                    ("pods-denied", PODS_DENIED, "pods-denied")):
            obs_config = temp_parent / (label + ".kubeconfig")
            tui_result_path = output / ("tui-" + label + ".json")
            probe_env = {**observation_env(private_home=private_home, shims=shims, kubeconfig=obs_config, cub_log=cub_log), "SCOUT_GITOPS_TUI_CONTEXT": context,
                         "SCOUT_GITOPS_TUI_RESULT": str(tui_result_path)}
            proxy = next(item for item in proxies if item.label == context)
            snapshot_proxies(proxies, clear=True)
            _lifecycle._mark_api_phase(output / "api-events.jsonl", "tui-" + label)
            cub_bytes = cub_log.stat().st_size if cub_log.exists() else 0
            record_command(receipt, "tui-" + label, [str(tui_binary), "-test.run", "^TestGitOpsStatusOwnedTUI$", "-test.count=1", "-test.v"], env=probe_env, deadline=deadline, timeout=120)
            data = json.loads(tui_result_path.read_text())
            cub_rows = validate_cub_records(cub_log, old_bytes=cub_bytes, expected_calls=1)
            validate_tui_probe(data, context=context, result=typ)
            tui_rows = snapshot_proxies(proxies, clear=True)
            expected_denial = f"/apis/modelplane.ai/v1alpha1/namespaces/{NAMESPACE}/modeldeployments" if typ == "controller-denied" else f"/api/v1/namespaces/{NAMESPACE}/pods" if typ == "pods-denied" else None
            validate_api_records([{**row, "context": row.get("endpoint")} for row in tui_rows], expected_context=context, expected_denial=expected_denial)
            phases.append({"name": "tui-" + label, "context": context, "result": typ, "view": data["view"], "apiRequests": tui_rows, "cubCalls": cub_rows, "tuiResultSha256": digest(tui_result_path)})
        verify_immutable_configs(receipt, configs)
        receipt["cubLogSha256"] = digest(cub_log)
        receipt["apiLogSha256"] = digest(output / "api-events.jsonl")
        receipt["acceptance"] = "passed"
    except BaseException as exc:
        receipt["acceptance"] = "failed"
        receipt["error"] = type(exc).__name__ + ": " + str(exc)
        raise
    finally:
        for signum, handler in signal_handlers.items():
            try:
                signal.signal(signum, handler)
            except BaseException:
                cleanup_errors.append("signal handler restoration failed")
        receipt["clusterCreationAttempted"] = attempted
        receipt["clusterCreationSucceeded"] = created
        cleanup_deadline = time.monotonic() + 120
        for proxy in reversed(proxies):
            try:
                proxy.close()
            except BaseException:
                cleanup_errors.append("owned read-only API proxy did not stop cleanly")
        # Stop handlers before the final snapshot/hash, and never let evidence
        # finalization errors skip owned-cluster or credential cleanup.
        try:
            receipt["pendingAPIRequests"] = snapshot_proxies(proxies)
            if receipt["pendingAPIRequests"]:
                cleanup_errors.append("unvalidated Kubernetes traffic remained after observations")
        except BaseException:
            cleanup_errors.append("final API snapshot failed")
        for log in ("api-events.jsonl", "cub-shim.jsonl"):
            try:
                if (output / log).exists():
                    receipt.setdefault("retainedLogHashes", {})[log] = digest(output / log)
            except BaseException:
                cleanup_errors.append("retained log hashing failed")
        # kind delete edits its own admin kubeconfig. Seal read integrity before
        # that intentional setup/cleanup mutation, after all readers have stopped.
        try:
            receipt["sharedKubeconfigSha256After"] = digest(shared)
            receipt["sharedKubeconfigUnchanged"] = receipt["sharedKubeconfigSha256Before"] == receipt["sharedKubeconfigSha256After"]
            receipt["privateKubeconfigSha256AfterReads"] = digest(private_config) if private_config and private_config.exists() else None
            receipt["privateKubeconfigUnchangedDuringReads"] = receipt.get("privateKubeconfigSha256BeforeReads") == receipt.get("privateKubeconfigSha256AfterReads")
        except OSError:
            receipt["sharedKubeconfigUnchanged"] = False
            receipt["privateKubeconfigUnchangedDuringReads"] = False
            cleanup_errors.append("kubeconfig integrity check failed")
        for cleanup in (
            lambda: _lifecycle.cleanup_cluster(receipt, name=cluster, created=created, creation_attempted=attempted,
                tools=tools, env=env, deadline=cleanup_deadline),
            lambda: _lifecycle.cleanup_worktrees(receipt, repo=REPO, git=tools["git"], worktrees=worktrees, env=env),
        ):
            try:
                cleanup_errors.extend(cleanup())
            except BaseException:
                cleanup_errors.append("owned lifecycle cleanup raised an exception")
        try:
            verify_immutable_configs(receipt, configs)
        except (OSError, RuntimeError):
            cleanup_errors.append("observation kubeconfig integrity failed")
        if temp_parent is not None:
            try:
                # Delete only this invocation's fresh private directory. Public
                # command/failure receipts and TUI artifacts remain in output.
                shutil.rmtree(temp_parent)
                receipt["privateDirectoryRemoved"] = not temp_parent.exists()
            except OSError:
                cleanup_errors.append("private directory removal failed")
        receipt["cleanupErrors"] = cleanup_errors
        if cleanup_errors or receipt.get("sharedKubeconfigUnchanged") is not True or (created and receipt.get("privateKubeconfigUnchangedDuringReads") is not True):
            receipt["acceptance"] = "failed"
        receipt["retainedWorkDirectory"] = str(temp_parent) if temp_parent and temp_parent.exists() else None
        write_private(output / "receipt.json", json.dumps(receipt, indent=2) + "\n")
    if receipt.get("acceptance") != "passed":
        raise RuntimeError("owned GitOps proof failed; inspect receipt.json")
    return 0


def _mcp_call(name: str, arguments: dict) -> str:
    initialize = {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "owned-proof", "version": "1"}}}
    notification = {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
    call = {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": name, "arguments": arguments}}
    return "\n".join(json.dumps(item) for item in (initialize, notification, call)) + "\n"


def _run_mcp(binary: str, input_data: str, *, env: dict, deadline: float) -> dict:
    """Use the reviewed bounded runner, including process-group cleanup."""
    return _lifecycle.run([binary, "mcp", "serve"], env=env, deadline=deadline,
                          timeout=90, input_data=input_data.encode())


def _last_json_line(raw: str) -> dict:
    for line in reversed(raw.splitlines()):
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict) and value.get("id") == 2:
            return value
    raise RuntimeError("MCP stdio output omitted tools/call response")


def _fixture_yaml(include_crd: bool = True) -> str:
    prefix = f'''apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {{name: applications.argoproj.io}}
spec:
  group: argoproj.io
  scope: Namespaced
  names: {{plural: applications, singular: application, kind: Application}}
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema: {{openAPIV3Schema: {{type: object, x-kubernetes-preserve-unknown-fields: true}}}}
---
''' if include_crd else ""
    return prefix + f'''apiVersion: v1
kind: Namespace
metadata: {{name: {NAMESPACE}}}
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: {{name: scout-context-app, namespace: {NAMESPACE}}}
spec:
  destination: {{namespace: {NAMESPACE}}}
  source: {{repoURL: https://example.invalid/owned-proof.git}}
status:
  sync: {{status: Synced}}
  health: {{status: Healthy}}
'''


def _allowed_paths() -> frozenset[str]:
    return frozenset(CONTROLLER_PATHS)


def _proxy_kubeconfig(context: str, proxies: list) -> dict:
    labels = [proxy.label for proxy in proxies]
    if len(labels) < 2 or len(set(labels)) != len(labels) or context not in labels:
        raise RuntimeError("selector proof requires distinct selected and ambient bindings")
    ambient = next(label for label in labels if label != context)
    return {"apiVersion": "v1", "kind": "Config", "current-context": ambient,
            "clusters": [{"name": proxy.label, "cluster": {"server": proxy.endpoint}} for proxy in proxies],
            "users": [{"name": "anonymous-proof-proxy", "user": {}}],
            "contexts": [{"name": proxy.label, "context": {"cluster": proxy.label,
                "user": "anonymous-proof-proxy", "namespace": NAMESPACE}} for proxy in proxies]}


def _rbac_yaml(service_account: str, *, deny_modelplane=False, deny_pods=False) -> str:
    groups = {
        "": ["pods", "events"], "apps": ["deployments"],
        "argoproj.io": ["applications"], "helm.toolkit.fluxcd.io": ["helmreleases"],
        "kustomize.toolkit.fluxcd.io": ["kustomizations"], "source.toolkit.fluxcd.io": ["gitrepositories"], "fluxcd.controlplane.io": [
            "fluxinstances", "fluxreports", "resourcesets", "resourcesetinputproviders", "externalartifacts", "artifactgenerators"],
        "config.projectsveltos.io": ["clusterprofiles", "profiles", "clustersummaries", "clusterconfigurations", "clusterreports", "clusterpromotions"],
        "lib.projectsveltos.io": ["eventsources", "eventtriggers", "clusterhealthchecks", "healthcheckreports", "eventreports"],
        "modelplane.ai": ["inferencegateways", "inferenceclasses", "inferenceclusters", "modeldeployments", "modelservices", "modelendpoints", "modelcaches", "modelreplicas"],
        "infrastructure.modelplane.ai": ["eksclusters", "gkeclusters", "servingstacks"],
    }
    if deny_pods:
        groups[""] = ["events"]
    if deny_modelplane:
        groups["modelplane.ai"].remove("modeldeployments")
    rules = "\n".join("  - apiGroups: [" + (f'"{group}"' if group else '""') + "]\n    resources: [" + ", ".join('"' + item + '"' for item in resources) + "]\n    verbs: [" + '"get", "list"' + "]" for group, resources in groups.items())
    return f'''apiVersion: v1
kind: ServiceAccount
metadata: {{name: {service_account}, namespace: {NAMESPACE}}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {{name: {service_account}}}
rules:
{rules}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {{name: {service_account}}}
roleRef: {{apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: {service_account}}}
subjects: [{{kind: ServiceAccount, name: {service_account}, namespace: {NAMESPACE}}}]
'''


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print("capture failed: " + str(exc), file=sys.stderr)
        raise SystemExit(1)
