#!/usr/bin/env python3
"""Reviewed, explicitly gated owned-kind #755 three-way context proof.

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
import threading
import uuid
from urllib.parse import urlsplit

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
FIXED_SOURCE = "478b1095359c6325d38e89c02e837861fc0f4410"
OLD_SOURCE = "94e6edf339e0cd410994563fb7a7c57c52e18c09"
NAMESPACE = "scout-three-way-context-proof"
ALLOWED = "three-way-allowed"
SOURCE_DENIED = "three-way-source-denied"
PODS_DENIED = "three-way-pods-denied"
SOURCE_DENIED_USER = "system:serviceaccount:" + NAMESPACE + ":source-denied"
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

# Exact reads for one Deployment and its explicitly labelled inert Application.
DEPLOYMENT = "scout-context-marker"
APPLICATION = "scout-context-app"
UNIT = "scout-proof-unit"
SPACE = "scout-proof-space"
DEPLOYMENT_PATH = f"/apis/apps/v1/namespaces/{NAMESPACE}/deployments/{DEPLOYMENT}"
APPLICATION_LIST_PATH = "/apis/argoproj.io/v1alpha1/applications"
APPLICATION_PATH = f"/apis/argoproj.io/v1alpha1/namespaces/{NAMESPACE}/applications/{APPLICATION}"
PODS_PATH = f"/api/v1/namespaces/{NAMESPACE}/pods"
REQUESTS = {
    DEPLOYMENT_PATH: {},
    APPLICATION_LIST_PATH: {"fieldSelector": ["metadata.name=" + APPLICATION]},
    APPLICATION_PATH: {},
    PODS_PATH: {"labelSelector": ["app=" + DEPLOYMENT]},
}
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
        {"LANG": "C", "TMPDIR": str(private_home), "SCOUT_THREE_WAY_CUB_LOG": str(cub_log)},
        private_home=private_home, shims=shims, kubeconfig=kubeconfig)


AUTH_CALL = {"type": "call", "argv": ["cub", "auth", "status"], "exitCode": 0}
UNIT_CALL = {"type": "call", "argv": ["cub", "unit", "get", UNIT, "-o", "json", "--quiet", "--space", SPACE],
             "exitCode": 73}
CUB_SEQUENCES = {
    "cli": [AUTH_CALL, UNIT_CALL],
    "mcp": [AUTH_CALL, AUTH_CALL, AUTH_CALL, UNIT_CALL],
    "old": [AUTH_CALL, AUTH_CALL, AUTH_CALL, UNIT_CALL],
    # Each visible TUI request refreshes the cached ConfigHub gate once.
    "tui": [AUTH_CALL, UNIT_CALL] * 3,
}


def create_observation_shims(shims: Path) -> Path:
    # Reject and redact every unexpected argv. No real cub is on child PATH.
    path = shims / "cub"
    auth_record = json.dumps(AUTH_CALL, separators=(",", ":"))
    unit_record = json.dumps(UNIT_CALL, separators=(",", ":"))
    path.write_text(f"""#!/bin/sh
if [ "$#" -eq 2 ] && [ "$1" = auth ] && [ "$2" = status ]; then
  printf '%s\\n' '{auth_record}' >> "$SCOUT_THREE_WAY_CUB_LOG"
  exit 0
fi
if [ "$#" -eq 8 ] && [ "$1" = unit ] && [ "$2" = get ] && [ "$3" = {UNIT} ] && [ "$4" = -o ] && [ "$5" = json ] && [ "$6" = --quiet ] && [ "$7" = --space ] && [ "$8" = {SPACE} ]; then
  printf '%s\\n' '{unit_record}' >> "$SCOUT_THREE_WAY_CUB_LOG"
  echo 'recorded unit lookup unavailable in owned proof' >&2
  exit 73
fi
printf '%s\\n' '{{"type":"rejected","argvRedacted":true,"exitCode":97}}' >> "$SCOUT_THREE_WAY_CUB_LOG"
exit 97
""")
    path.chmod(0o700)
    return path


def validate_cub_records(path: Path, *, old_bytes: int, action: str) -> list[dict]:
    raw = path.read_bytes() if path.exists() else b""
    if len(raw) > 1024 * 1024 or old_bytes < 0 or old_bytes > len(raw):
        raise RuntimeError("cub argv log exceeded bound or was truncated")
    try:
        rows = [json.loads(line) for line in raw[old_bytes:].splitlines()]
    except (ValueError, UnicodeError):
        raise RuntimeError("cub argv log is malformed") from None
    if action not in CUB_SEQUENCES or rows != CUB_SEQUENCES[action]:
        raise RuntimeError("cub calls differed from exact synthetic auth/unit-get sequence")
    return rows


def snapshot_proxies(proxies: list, *, clear: bool = False) -> list[dict]:
    return [row for proxy in proxies for row in proxy.snapshot(clear=clear)]


def allowed_three_way_request(path: str, query: dict) -> bool:
    return path in REQUESTS and query == REQUESTS[path]


class ThreeWayAPIProxy(_trace_proxy.ReadOnlyAPIProxy):
    def __init__(self, **kwargs):
        upstream = urlsplit(kwargs["upstream"])
        if upstream.scheme != "https" or upstream.hostname not in ("127.0.0.1", "localhost", "::1"):
            raise RuntimeError("owned kind API must be HTTPS loopback")
        self._request_query = threading.local()
        super().__init__(**kwargs)

    def allowed_path(self, path: str, query: dict) -> bool:
        self._request_query.value = query
        return allowed_three_way_request(path, query)

    def record(self, method: str, path: str, status: int, disposition: str) -> None:
        # The reviewed base proxy records only path. Retain exact selectors too
        # so an accepted report must match actual request query evidence.
        row = {"type": "request", "endpoint": self.label, "method": method,
               "path": path, "query": getattr(self._request_query, "value", {}),
               "status": status, "disposition": disposition}
        with self._lock:
            if len(self.requests) >= 512:
                self.overflow = True
                return
            self.requests.append({k: v for k, v in row.items() if k != "type"})
            if self.event_log is not None:
                data = json.dumps(row, separators=(",", ":")).encode() + b"\n"
                if self._event_bytes + len(data) > 1024 * 1024:
                    self.overflow = True
                    return
                fd = os.open(self.event_log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
                try:
                    os.write(fd, data)
                finally:
                    os.close(fd)
                self._event_bytes += len(data)


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


def validate_three_way_json(raw: str, *, context: str, result: str) -> dict:
    try:
        body = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        raise RuntimeError("three-way observation is not JSON") from None
    if not isinstance(body, dict) or body.get("context") != context:
        raise RuntimeError("three-way report lost the selected context label")
    if result not in ("allowed", "source-denied", "pods-denied"):
        raise RuntimeError("unknown three-way observation result")
    entries = body.get("resources")
    if not isinstance(entries, list) or len(entries) != 1 or not isinstance(entries[0], dict):
        raise RuntimeError("three-way report must retain exactly one workload")
    resource = entries[0].get("result", {})
    live = resource.get("live", {})
    if (resource.get("resource"), resource.get("namespace")) != ("Deployment/" + DEPLOYMENT, NAMESPACE):
        raise RuntimeError("three-way report lost exact workload scope")
    if (live.get("source"), live.get("apiVersion"), live.get("kind"), live.get("name"), live.get("namespace"),
            live.get("unitSlug"), live.get("spaceName"), live.get("replicas")) != (
            "cluster", "apps/v1", "Deployment", DEPLOYMENT, NAMESPACE, UNIT, SPACE, 0):
        raise RuntimeError("three-way report lost observed workload identity or ConfigHub labels")
    if type(live.get("replicas")) is not int:
        raise RuntimeError("LIVE replica count is not an observed integer")
    if resource.get("connected") is not True or resource.get("dry") is not None or resource.get("wet") is not None:
        raise RuntimeError("synthetic unit failure fabricated comparison sides")
    if not any("recorded unit lookup unavailable in owned proof" in note for note in resource.get("notes", [])):
        raise RuntimeError("synthetic unit-get failure was hidden")
    omissions = body.get("omissions", [])
    if not isinstance(omissions, list) or not all(isinstance(row, dict) for row in omissions):
        raise RuntimeError("three-way report omitted structured omissions")
    for reason in ("dry_unavailable", "wet_unavailable"):
        if not any(row.get("phase") == "comparison-sides" and row.get("reason") == reason for row in omissions):
            raise RuntimeError("missing comparison sides were hidden")
    forbidden = {(row.get("phase"), row.get("reason")) for row in omissions if row.get("reason") == "forbidden"}
    expected = {("live-source-link", "forbidden")} if result == "source-denied" else {("current-change", "forbidden")} if result == "pods-denied" else set()
    if any(row.get("reason") == "forbidden" and (row.get("resource"), row.get("namespace")) != ("Deployment/" + DEPLOYMENT, NAMESPACE) for row in omissions):
        raise RuntimeError("source/runtime denial named a different workload")
    if forbidden != expected:
        raise RuntimeError("source/runtime denial was hidden or unexpected")
    anchor = live.get("gitSource")
    if result == "source-denied":
        if anchor is not None or not any(row.get("phase") == "source-coverage" and row.get("reason") == "source_anchor_unavailable" for row in omissions):
            raise RuntimeError("denied source read was fabricated as observed provenance")
    elif not isinstance(anchor, dict) or (anchor.get("repoUrl"), anchor.get("revision"), anchor.get("path")) != (
            "https://example.invalid/owned-proof.git", "proof-revision", "proof/manifests"):
        raise RuntimeError("allowed source evidence was not retained")
    change = entries[0].get("currentChange")
    if not isinstance(change, dict) or change.get("resource") != {"apiVersion": "apps/v1", "kind": "Deployment", "namespace": NAMESPACE, "name": DEPLOYMENT}:
        raise RuntimeError("workload current-change evidence was discarded")
    summary = body.get("summary", {})
    if summary.get("totalResources") != 1 or summary.get("agreement", {}).get("state") != "partial":
        raise RuntimeError("incomplete sides falsely established complete convergence")
    sources = summary.get("agreement", {}).get("sources", {})
    if sources != {"confighub": 0, "deployer": 0 if result == "source-denied" else 1, "cluster": 1, "total": 1}:
        raise RuntimeError("source coverage disagrees with partial evidence")
    return body


def expected_api_sequence(result: str) -> list[tuple[str, int]]:
    sequence = [(DEPLOYMENT_PATH, 200), (APPLICATION_LIST_PATH, 403 if result == "source-denied" else 200)]
    if result != "source-denied":
        sequence.append((APPLICATION_PATH, 200))
    return sequence + [(DEPLOYMENT_PATH, 200), (PODS_PATH, 403 if result == "pods-denied" else 200)]


def validate_api_records(rows: list[dict], *, expected_context: str, result: str, collections: int = 1) -> None:
    if result not in ("allowed", "source-denied", "pods-denied") or collections not in (1, 3):
        raise RuntimeError("unknown API observation contract")
    actual = []
    for row in rows:
        if row.get("endpoint") != expected_context or row.get("method") != "GET":
            raise RuntimeError("Kubernetes request escaped the selected read-only context")
        if row.get("disposition") != "forwarded":
            raise RuntimeError("Kubernetes request was refused by the proof proxy")
        path, query = row.get("path"), row.get("query")
        if not isinstance(path, str) or not allowed_three_way_request(path, query):
            raise RuntimeError("Kubernetes request path/query escaped the exact allowlist")
        if type(row.get("status")) is not int or row["status"] not in (200, 403):
            raise RuntimeError("Kubernetes request returned an unexpected status")
        actual.append((path, row["status"]))
    if actual != expected_api_sequence(result) * collections:
        raise RuntimeError("exact workload/source/runtime request sequence or counts differed")


def validate_mcp_payload(response: dict) -> dict:
    body = _trace_proxy.mcp_result_json(response)
    payload = response["result"]
    texts = [block.get("text") for block in payload.get("content", []) if isinstance(block, dict) and block.get("type") == "text"]
    try:
        parsed = [json.loads(text) for text in texts]
    except (TypeError, json.JSONDecodeError):
        raise RuntimeError("MCP text content is not JSON") from None
    if parsed != [body] or not isinstance(payload.get("structuredContent"), dict) or payload["structuredContent"].get("data") != body:
        raise RuntimeError("MCP text and structured JSON disagree")
    return body


def validate_mcp_result(response: dict, *, context: str, result: str) -> dict:
    return validate_three_way_json(json.dumps(validate_mcp_payload(response)), context=context, result=result)


def validate_old_behavior(response: dict, rows: list[dict], *, ambient: str) -> None:
    body = validate_mcp_payload(response)
    if body.get("context") == ALLOWED:
        raise RuntimeError("old behavioral control unexpectedly honored selected context")
    entries = body.get("resources", [])
    if len(entries) != 1 or entries[0].get("result", {}).get("live", {}).get("name") != DEPLOYMENT:
        raise RuntimeError("old behavioral control failed to read actual ambient workload")
    if not rows or any(row.get("endpoint") != ambient or row.get("method") != "GET" or row.get("disposition") != "forwarded" or row.get("status") not in (200, 403) or not allowed_three_way_request(row.get("path"), row.get("query")) for row in rows):
        raise RuntimeError("old behavioral control did not prove exclusive actual ambient reads")
    # The old ambient source collector has no Argo/Flux child on the private
    # PATH. It still reads LIVE and rollout facts directly through that binding.
    if [(row.get("path"), row.get("status")) for row in rows] != [
            (DEPLOYMENT_PATH, 200), (DEPLOYMENT_PATH, 200), (PODS_PATH, 200)]:
        raise RuntimeError("old behavioral control successful workload/runtime reads differed")


def validate_tui_probe(data: dict, *, context: str, result: str) -> None:
    if data.get("schema") != "three-way-context-tui.v1" or data.get("passed") is not True or data.get("context") != context:
        raise RuntimeError("three-way TUI probe did not report a versioned selected-context pass")
    if context not in (ALLOWED, SOURCE_DENIED, PODS_DENIED):
        raise RuntimeError("three-way TUI probe has an unknown selected context")
    for name in ("report", "refreshedReport", "editedReport"):
        validate_three_way_json(json.dumps(data.get(name)), context=context, result=result)
    validate_viewport_probe(data, context=context, result=result)
    for name in ("immutableConfig", "refresh", "scopeEdit"):
        if data.get("checks", {}).get(name) is not True:
            raise RuntimeError("three-way TUI probe omitted actual model action " + name)


def validate_viewport_probe(data: dict, *, context: str, result: str) -> None:
    if data.get("schema") not in ("three-way-context-tui.v1", "three-way-context-viewport.v1") or data.get("context") != context:
        raise RuntimeError("viewport artifact schema/context is invalid")
    validate_three_way_json(json.dumps(data.get("report")), context=context, result=result)
    view, rendered = data.get("view", ""), data.get("rendered", "")
    if "Kubernetes context label: " + context not in view or DEPLOYMENT not in view or "PARTIAL" not in rendered:
        raise RuntimeError("three-way TUI view lost selected scope or partial agreement")
    if result != "allowed" and "forbidden" not in rendered:
        raise RuntimeError("three-way TUI rendering hid source/runtime denial")
    for name in ("resize", "scroll", "scrollBack", "quit", "immutableSnapshot"):
        if data.get("checks", {}).get(name) is not True:
            raise RuntimeError("three-way TUI probe omitted actual model action " + name)



def wait_fixture_api_ready(receipt: dict, *, kubectl: str, env: dict, deadline: float) -> None:
    # CRD Established can precede storage initialization. These are setup GETs
    # only; observation receipts must still reject every unexpected status.
    for group, resource, kind in (("argoproj.io", "applications", "ApplicationList"),):
        route = f"/apis/{group}/v1alpha1/namespaces/{NAMESPACE}/{resource}"
        for attempt in range(1, 6):
            result = record_observation(receipt, f"fixture-api-ready-{resource}-{attempt}",
                [kubectl, "--context", ALLOWED, "get", "--raw", route], env=env, deadline=deadline)
            if succeeded(result):
                body = json.loads(result["stdout"])
                if not isinstance(body, dict) or body.get("kind") != kind or not isinstance(body.get("items"), list):
                    raise RuntimeError("fixture API readiness returned an unexpected list")
                break
            if "storage is (re)initializing" not in result.get("stderr", "") or attempt == 5:
                raise RuntimeError("synthetic fixture API did not become readable during bounded setup")
            if time.monotonic() + 1 >= deadline:
                raise RuntimeError("fixture API readiness exceeded proof deadline")
            time.sleep(1)


def main() -> int:
    return _admitted_main()


def _admitted_main() -> int:
    require_source_pins()
    parser = argparse.ArgumentParser()
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--helper-source", required=True, type=full_commit)
    parser.add_argument("--tool-pin", action="append", required=True, help="reviewed tool name=sha256; one each for git/go/kind/kubectl/docker")
    parser.add_argument("--integrity-only-shared-kubeconfig", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    if not args.execute:
        parser.error("refusing to run without --execute")
    return _execute_capture(args, parser)


def _execute_capture(args, parser) -> int:
    """Pinned lifecycle; offline controls inject tools without live reads."""
    require_source_pins()
    if args.execute is not True:
        raise RuntimeError("refusing to run without --execute")
    full_commit(args.helper_source)
    tool_pins = parse_tool_pins(args.tool_pin)
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
    receipt = {"schema": "three-way-context-owned-kind.v1", "fixedSource": FIXED_SOURCE,
               "oldSource": OLD_SOURCE, "commands": [], "acceptance": "failed",
               "sharedKubeconfigSha256Before": hashlib.sha256(shared.read_bytes()).hexdigest()}
    cluster = "scout-three-way-" + uuid.uuid4().hex[:10]
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
    global_deadline = time.monotonic() + MAX_SECONDS
    deadline = global_deadline - 240  # reserve reviewed owned cleanup within total bound
    temp_parent = None
    private_config = None
    env = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME", "LANG", "LC_ALL")}
    env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "PYTHONDONTWRITEBYTECODE": "1"})
    configs: dict[Path, str] = {}
    receipt["observations"] = []
    try:
        temp_parent = Path(tempfile.mkdtemp(prefix="scout-three-way-context-", dir="/tmp"))
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
        receipt["toolPins"] = {name: {"path": path, "sha256": digest(Path(path))} for name, path in tools.items()}
        if any(receipt["toolPins"][name]["sha256"] != expected for name, expected in tool_pins.items()):
            raise RuntimeError("existing tool binary differs from admitted sha256 pin")
        clean = record_command(receipt, "source-clean", [tools["git"], "-C", str(REPO), "status", "--porcelain"], env=env, deadline=deadline)
        if clean["stdout"].strip():
            raise RuntimeError("capture checkout must be committed and clean")
        helper = record_command(receipt, "helper-source", [tools["git"], "-C", str(REPO), "rev-parse", "HEAD"], env=env, deadline=deadline)["stdout"].strip()
        if helper != args.helper_source:
            raise RuntimeError("helper/probe checkout differs from admitted helper source")
        receipt["helperSourceCommit"] = helper
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
        # Fixed-source test binary drives the real three-way collector and scoped viewport.
        probe = HERE / "three_way_tui_live_test.go.txt"
        receipt["tuiProbeSha256"] = digest(probe)
        target = worktrees[-1] / "cmd/cub-scout" / "three_way_context_live_test.go"
        target.write_text(probe.read_text())
        tui_binary = temp_parent / "three-way-tui.test"
        record_command(receipt, "tui-build", [tools["go"], "-C", str(worktrees[-1]), "test", "-c", "-o", str(tui_binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)

        receipt["tuiBinarySha256"] = digest(tui_binary)

        existing_clusters = record_command(receipt, "cluster-name-check", [tools["kind"], "get", "clusters"], env=env, deadline=deadline)["stdout"].splitlines()
        if cluster in existing_clusters:
            raise RuntimeError("invocation-owned cluster name already exists")
        receipt["ownedCluster"] = cluster
        attempted = True
        record_command(receipt, "create-owned-cluster", [tools["kind"], "create", "cluster", "--name", cluster,
                       "--image", NODE_IMAGE, "--kubeconfig", str(private_config), "--wait", "90s"],
                       env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline, timeout=240)
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
        write_private(crd_file, _crd_yaml())
        kube_env = {**env, "KUBECONFIG": str(private_config)}
        record_command(receipt, "fixture-crd", [kubectl, "apply", "--context", ALLOWED, "-f", str(crd_file)], env=kube_env, deadline=deadline)
        record_command(receipt, "fixture-crd-established", [kubectl, "wait", "--context", ALLOWED, "--for=condition=Established", "--timeout=45s", "crd/applications.argoproj.io"], env=kube_env, deadline=deadline, timeout=55)
        fixture = temp_parent / "fixture.yaml"
        write_private(fixture, _fixture_yaml(include_crd=False))
        receipt["fixtureSha256"] = digest(fixture)
        receipt["crdSha256"] = digest(crd_file)
        wait_fixture_api_ready(receipt, kubectl=kubectl, env=kube_env, deadline=deadline)
        record_command(receipt, "fixture-apply", [kubectl, "apply", "--context", ALLOWED, "-f", str(fixture)], env=kube_env, deadline=deadline)
        raw_config = record_command(receipt, "private-context-bindings", [kubectl, "config", "view", "--raw", "-o", "json"], env=kube_env, deadline=deadline, secret=True)
        config = json.loads(raw_config["stdout"])
        config.setdefault("users", [])
        config.setdefault("contexts", [])
        # Short-lived ServiceAccount tokens are captured privately and every
        # token-producing command is redacted from the public receipt.
        for sa, role_yaml in (("source-denied", _rbac_yaml("source-denied", deny_source=True)),
                              ("pods-denied", _rbac_yaml("pods-denied", deny_pods=True))):
            role_file = temp_parent / (sa + ".yaml")
            write_private(role_file, role_yaml)
            receipt.setdefault("rbacHashes", {})[sa] = digest(role_file)
            record_command(receipt, sa + "-rbac", [kubectl, "apply", "--context", ALLOWED, "-f", str(role_file)], env=kube_env, deadline=deadline)
            token = record_command(receipt, sa + "-token", [kubectl, "-n", NAMESPACE, "create", "token", sa, "--duration=10m", "--context", ALLOWED], env=kube_env, deadline=deadline, secret=True)["stdout"].strip()
            if not token or "\n" in token:
                raise RuntimeError("owned ServiceAccount token output was malformed")
            config["users"].append({"name": sa + "-user", "user": {"token": token}})
            config["contexts"].append({"name": SOURCE_DENIED if sa == "source-denied" else PODS_DENIED, "context": {"cluster": "kind-" + cluster,
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
        for context_name, label in ((ALLOWED, "allowed"), (SOURCE_DENIED, "source-denied"), (PODS_DENIED, "pods-denied")):
            binding = contexts[context_name]
            tls, authorization, certs = _trace_proxy.upstream_credentials(
                clusters[binding["cluster"]], {"user": users[binding["user"]]}, private_dir=temp_parent, label=label)
            proxy_cert_paths.extend(certs)
            upstream = clusters[binding["cluster"]]["server"]
            if urlsplit(upstream).hostname not in ("127.0.0.1", "localhost", "::1"):
                raise RuntimeError("owned kind API is not loopback-bound")
            proxy = ThreeWayAPIProxy(label=context_name, upstream=upstream, tls=tls,
                authorization=authorization, namespace=NAMESPACE, deployment="scout-context-marker",
                application="scout-context-app", missing_deployment="scout-context-missing", event_log=output / "api-events.jsonl")
            proxies.append(proxy)
        receipt["privateKubeconfigSha256BeforeReads"] = digest(private_config)

        phases = receipt["observations"]
        # Each config includes every proxy binding and a different ambient context.
        for context in (ALLOWED, SOURCE_DENIED, PODS_DENIED):
            obs_config = temp_parent / (context + ".kubeconfig")
            write_private(obs_config, json.dumps(_proxy_kubeconfig(context, proxies)) + "\n", immutable=True)
            configs[obs_config] = digest(obs_config)
        old_config = temp_parent / (ALLOWED + ".kubeconfig")
        ambient = _proxy_kubeconfig(ALLOWED, proxies)["current-context"]
        old_env = observation_env(private_home=private_home, shims=shims, kubeconfig=old_config, cub_log=cub_log)
        snapshot_proxies(proxies, clear=True)
        _lifecycle._mark_api_phase(output / "api-events.jsonl", "old-mcp-ignored-context")
        old_bytes = cub_log.stat().st_size if cub_log.exists() else 0
        old = _run_mcp(str(temp_parent / "cub-scout-old"), _mcp_call("compare_three_way", {
            "context": ALLOWED, "namespace": NAMESPACE, "scope": "deploy/" + DEPLOYMENT}), env=old_env, deadline=deadline)
        receipt["commands"].append({"phase": "old-mcp-ignored-context", **old})
        if not succeeded(old):
            raise RuntimeError("old MCP behavioral control failed; inspect retained receipt")
        cub_rows = validate_cub_records(cub_log, old_bytes=old_bytes, action="old")
        old_rows = snapshot_proxies(proxies, clear=True)
        validate_old_behavior(_last_json_line(old["stdout"]), old_rows, ambient=ambient)
        phases.append({"name": "old-mcp-ignored-context", "requestedContext": ALLOWED,
            "actualAmbientContext": ambient, "selectedRequests": 0, "stdout": old["stdout"],
            "apiRequests": old_rows, "cubCalls": cub_rows})

        for context, typ in ((ALLOWED, "allowed"), (SOURCE_DENIED, "source-denied"), (PODS_DENIED, "pods-denied")):
            binary = str(temp_parent / "cub-scout-fixed")
            obs_config = temp_parent / (context + ".kubeconfig")
            obs_env = observation_env(private_home=private_home, shims=shims, kubeconfig=obs_config, cub_log=cub_log)
            for action in ("cli", "mcp", "tui"):
                phase = action + "-" + typ
                snapshot_proxies(proxies, clear=True)
                _lifecycle._mark_api_phase(output / "api-events.jsonl", phase)
                cub_bytes = cub_log.stat().st_size if cub_log.exists() else 0
                artifact = None
                if action == "cli":
                    command = record_observation(receipt, phase, [binary, "compare", "three-way", "--scope", "deploy/" + DEPLOYMENT,
                        "-n", NAMESPACE, "--kube-context", context, "--format", "json"], env=obs_env, deadline=deadline)
                    if not succeeded(command):
                        raise RuntimeError("CLI observation failed; inspect retained receipt")
                    validate_three_way_json(command["stdout"], context=context, result=typ)
                elif action == "mcp":
                    command = _run_mcp(binary, _mcp_call("compare_three_way", {
                        "context": context, "namespace": NAMESPACE, "scope": "deploy/" + DEPLOYMENT}), env=obs_env, deadline=deadline)
                    receipt["commands"].append({"phase": phase, **command})
                    if not succeeded(command):
                        raise RuntimeError("MCP observation failed; inspect retained receipt")
                    validate_mcp_result(_last_json_line(command["stdout"]), context=context, result=typ)
                else:
                    artifact = output / (phase + ".json")
                    probe_env = {**obs_env, "SCOUT_THREE_WAY_TUI_CONTEXT": context, "SCOUT_THREE_WAY_TUI_RESULT": str(artifact)}
                    command = record_command(receipt, phase, [str(tui_binary), "-test.run", "^TestThreeWayOwnedTUI$", "-test.count=1", "-test.v"], env=probe_env, deadline=deadline, timeout=120)
                    data = json.loads(artifact.read_text())
                    validate_tui_probe(data, context=context, result=typ)
                cub_rows = validate_cub_records(cub_log, old_bytes=cub_bytes, action=action)
                rows = snapshot_proxies(proxies, clear=True)
                validate_api_records(rows, expected_context=context, result=typ, collections=3 if action == "tui" else 1)
                phases.append({"name": phase, "context": context, "result": typ, "stdout": command["stdout"],
                    "apiRequests": rows, "cubCalls": cub_rows, **({"tuiResultSha256": digest(artifact)} if artifact else {})})
        # Validate raw phase logs against the exact rows retained in the receipt.
        grouped = _trace_proxy.group_action_events(output / "api-events.jsonl", [phase["name"] for phase in phases])
        if any(grouped[phase["name"]] != phase["apiRequests"] for phase in phases):
            raise RuntimeError("raw API events and receipt traffic disagree")
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
        cleanup_deadline = global_deadline
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
            lambda: cleanup_worktrees_bounded(receipt, repo=REPO, git=tools["git"], worktrees=worktrees, env=env, deadline=cleanup_deadline),
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
        raise RuntimeError("owned three-way proof failed; inspect receipt.json")
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


def _crd_yaml() -> str:
    return """apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: applications.argoproj.io}
spec:
  group: argoproj.io
  scope: Namespaced
  names: {plural: applications, singular: application, kind: Application}
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema: {openAPIV3Schema: {type: object, x-kubernetes-preserve-unknown-fields: true}}
"""


def _fixture_yaml(include_crd: bool = True) -> str:
    return (_crd_yaml() + "---\n" if include_crd else "") + f"""apiVersion: v1
kind: Namespace
metadata: {{name: {NAMESPACE}}}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {DEPLOYMENT}
  namespace: {NAMESPACE}
  labels:
    argocd.argoproj.io/instance: {APPLICATION}
    confighub.com/UnitSlug: {UNIT}
    confighub.com/SpaceName: {SPACE}
spec:
  replicas: 0
  selector: {{matchLabels: {{app: {DEPLOYMENT}}}}}
  template:
    metadata: {{labels: {{app: {DEPLOYMENT}}}}}
    spec:
      containers: [{{name: inert, image: example.invalid/never-pulled:proof}}]
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: {{name: {APPLICATION}, namespace: {NAMESPACE}}}
spec:
  destination: {{namespace: {NAMESPACE}}}
  source: {{repoURL: https://example.invalid/owned-proof.git, targetRevision: proof-revision, path: proof/manifests}}
status:
  sync: {{status: Synced, revision: proof-revision}}
  health: {{status: Healthy}}
"""


def _allowed_paths() -> frozenset[str]:
    return frozenset(REQUESTS)


def parse_tool_pins(values: list[str]) -> dict[str, str]:
    pins = {}
    for value in values:
        name, separator, sha = value.partition("=")
        if not separator or name in pins or len(sha) != 64 or any(c not in "0123456789abcdef" for c in sha):
            raise RuntimeError("tool pin must be a unique name=lowercase sha256")
        pins[name] = sha
    if set(pins) != {"git", "go", "kind", "kubectl", "docker"}:
        raise RuntimeError("tool pins must cover exactly git/go/kind/kubectl/docker")
    return pins


def cleanup_worktrees_bounded(receipt: dict, *, deadline: float, **kwargs) -> list[str]:
    # Reuse reviewed ownership/removal policy. Its command runner lacks a global
    # deadline parameter at this boundary; add only a deadline to real commands.
    original = _lifecycle.run
    def bounded(argv, **options):
        options["deadline"] = min(options.get("deadline", deadline), deadline)
        return original(argv, **options)
    _lifecycle.run = bounded
    try:
        return _lifecycle.cleanup_worktrees(receipt, **kwargs)
    finally:
        _lifecycle.run = original


def full_commit(value: str) -> str:
    if not isinstance(value, str) or len(value) != 40 or any(char not in "0123456789abcdef" for char in value):
        raise RuntimeError("source pin must be a full lowercase Git commit")
    return value


def require_source_pins() -> None:
    if FIXED_SOURCE == "ADMISSION_PENDING":
        raise RuntimeError("live execution disabled: product source pin is admission pending")
    full_commit(FIXED_SOURCE)
    full_commit(OLD_SOURCE)


def _proxy_kubeconfig(context: str, proxies: list) -> dict:
    labels = [proxy.label for proxy in proxies]
    if set(labels) != {ALLOWED, SOURCE_DENIED, PODS_DENIED} or len(set(labels)) != len(labels) or context not in labels:
        raise RuntimeError("selector proof requires distinct selected and ambient bindings")
    endpoints = [proxy.endpoint for proxy in proxies]
    if len(set(endpoints)) != len(endpoints) or any(urlsplit(endpoint).scheme != "http" or urlsplit(endpoint).hostname != "127.0.0.1" or urlsplit(endpoint).username or urlsplit(endpoint).query or urlsplit(endpoint).path for endpoint in endpoints):
        raise RuntimeError("observation proxy endpoints must be distinct credential-free HTTP loopback")
    ambient = next(label for label in labels if label != context)
    return {"apiVersion": "v1", "kind": "Config", "current-context": ambient,
            "clusters": [{"name": proxy.label, "cluster": {"server": proxy.endpoint}} for proxy in proxies],
            "users": [{"name": "anonymous-proof-proxy", "user": {}}],
            "contexts": [{"name": proxy.label, "context": {"cluster": proxy.label,
                "user": "anonymous-proof-proxy", "namespace": NAMESPACE}} for proxy in proxies]}


def _rbac_yaml(service_account: str, *, deny_source=False, deny_pods=False) -> str:
    groups = {"apps": ["deployments"]}
    if not deny_source:
        groups["argoproj.io"] = ["applications"]
    if not deny_pods:
        groups[""] = ["pods"]
    rules = "\n".join("  - apiGroups: [" + json.dumps(group) + "]\n    resources: " + json.dumps(resources) + '\n    verbs: ["get", "list"]' for group, resources in groups.items())
    return f"""apiVersion: v1
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
"""


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print("capture failed: " + str(exc), file=sys.stderr)
        raise SystemExit(1)
