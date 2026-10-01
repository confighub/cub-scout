#!/usr/bin/env python3
"""Optional bounded local CLI/MCP preflight. Never launches Claude or contacts a cluster."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import time

EXPECTED_COUNTS = {"ArgoCD": 90, "ConfigHub": 33, "Flux": 120, "Helm": 45, "Native": 12}
DEPLOYMENT_SHA256 = "822716e41eaf59674cec8b52913b2613c76932b578c45d54285f71747dd228b6"
NO_MARKER = ["team-02/auth", "team-03/auth", "team-05/auth", "team-05/cron", "team-05/notify",
             "team-11/api", "team-11/notify", "team-12/web", "team-18/web", "team-19/cron",
             "team-25/search", "team-30/cache"]
MAP_CONTRACT_BASIC = "recorded-map-basic.v1"
MAP_CONTRACT_VIEWS = "recorded-map-views.v1"
MAP_CONTRACTS = (MAP_CONTRACT_BASIC, MAP_CONTRACT_VIEWS)
VIEW_OWNERS = ("Flux", "ArgoCD", "Sveltos", "Modelplane", "Crossplane", "kro", "Helm",
               "Terraform", "ConfigHub", "Kubernetes", "Native")
VIEWS = ("full", "summary", "native", "native-summary")


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class RunInterrupted(BaseException):
    def __init__(self, signum: int):
        self.signum = signum


def stop_owned_group(pgid: int, grace: float = 0.35, process: subprocess.Popen | None = None) -> None:
    try:
        os.killpg(pgid, signal.SIGTERM)
    except ProcessLookupError:
        return
    # Do not probe or signal any group other than the new-session PGID we own.
    # A fixed bounded grace also gives descendants time to exit on TERM.
    time.sleep(grace)
    if process is not None:
        process.poll()  # Reap a terminated leader without abandoning its descendants.
    try:
        os.killpg(pgid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def exact_map_schema(schema: dict, contract: str = MAP_CONTRACT_BASIC) -> bool:
    if not isinstance(schema, dict) or contract not in MAP_CONTRACTS:
        return False
    properties = schema.get("properties")
    names = ({"api_version", "kind", "namespace", "namespace_prefix"}
             if contract == MAP_CONTRACT_BASIC else
             {"api_version", "kind", "namespace", "namespace_prefix", "owner", "summary"})
    required = schema.get("required")
    if (set(schema) != {"type", "properties", "required", "additionalProperties"} or
            schema.get("type") != "object" or
            not isinstance(properties, dict) or set(properties) != names or
            not isinstance(required, list) or len(required) != 2 or
            set(required) != {"api_version", "kind"} or
            schema.get("additionalProperties") is not False):
        return False
    for name, prop in properties.items():
        expected_type = "boolean" if contract == MAP_CONTRACT_VIEWS and name == "summary" else "string"
        if not isinstance(prop, dict) or not set(prop).issubset({"type", "description", "minLength", "enum"}) or prop.get("type") != expected_type:
            return False
        if "description" in prop and not isinstance(prop["description"], str):
            return False
        if "minLength" in prop and (name != "namespace_prefix" or prop["minLength"] != 1 or isinstance(prop["minLength"], bool)):
            return False
    if contract == MAP_CONTRACT_VIEWS:
        enum = properties["owner"].get("enum")
        if (not isinstance(enum, list) or not all(isinstance(value, str) for value in enum) or
                len(enum) != len(VIEW_OWNERS) or set(enum) != set(VIEW_OWNERS)):
            return False
    elif any("enum" in prop for prop in properties.values()):
        return False
    return True


def run_bounded(command: list[str], env: dict[str, str], timeout: float = 15.0,
                max_stdout: int = 8 * 1024 * 1024, max_stderr: int = 16 * 1024) -> tuple[int, bytes, bytes]:
    """Run one owned process group with drained bounded pipes and deadline cleanup."""
    proc = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, env=env, start_new_session=True)
    selector = selectors.DefaultSelector()
    buffers = {"stdout": bytearray(), "stderr": bytearray()}
    limits = {"stdout": max_stdout, "stderr": max_stderr}
    assert proc.stdout is not None and proc.stderr is not None
    selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
    selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
    deadline = time.monotonic() + timeout
    previous = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
    def interrupted(signum, frame):
        raise RunInterrupted(signum)
    try:
        for sig in previous:
            signal.signal(sig, interrupted)
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("owned subprocess timed out")
            for key, _ in selector.select(min(0.1, remaining)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                buf = buffers[key.data]
                remaining_limit = limits[key.data] - len(buf)
                if remaining_limit > 0:
                    buf.extend(chunk[:remaining_limit])
                if key.data == "stdout" and len(buf) >= limits["stdout"]:
                    raise ValueError("subprocess output exceeded bounded stdout limit")
            if proc.poll() is not None and not selector.get_map():
                break
        return proc.wait(), bytes(buffers["stdout"]), bytes(buffers["stderr"])
    finally:
        for sig in previous:
            signal.signal(sig, signal.SIG_IGN)
        # The leader may have exited while descendants still hold inherited pipes.
        stop_owned_group(proc.pid, grace=0.35, process=proc)
        try:
            proc.wait(timeout=1)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
            proc.wait(timeout=1)
        selector.close()
        proc.stdout.close()
        proc.stderr.close()
        for sig, handler in previous.items():
            signal.signal(sig, handler)


def validate_report(report: dict) -> None:
    def require(condition, message):
        if not condition:
            raise ValueError(message)
    require(report.get("schema") == "map-list-recorded.v1", "unexpected recorded map schema")
    provenance = report.get("provenance", {})
    require(provenance.get("objectCount") == 302 and provenance.get("sha256") == DEPLOYMENT_SHA256,
            "recording provenance mismatch")
    require(provenance.get("captureTime") == "unknown" and provenance.get("captureCompleteness") == "unknown",
            "recording must not claim capture time/completeness")
    require(report.get("scope") == {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"},
            "recorded scope mismatch")
    require(report.get("selectedCount") == 300 and report.get("excludedFromScopeCount") == 2, "selection counts mismatch")
    resources = report.get("resources", [])
    require(len(resources) == 300, "resource count mismatch")
    require(report.get("ownerCounts") == EXPECTED_COUNTS, "owner counts mismatch")
    counts, identities = {}, set()
    for resource in resources:
        require(resource.get("apiVersion") == "apps/v1" and resource.get("kind") == "Deployment" and
                isinstance(resource.get("namespace"), str) and resource["namespace"].startswith("team-") and
                isinstance(resource.get("name"), str) and bool(resource["name"]), "resource outside expected scope")
        identity = (resource["namespace"], resource["name"])
        require(identity not in identities, "duplicate selected identity")
        identities.add(identity)
        owner = resource.get("owner")
        require(owner in EXPECTED_COUNTS, "unexpected owner category")
        counts[owner] = counts.get(owner, 0) + 1
    require(counts == EXPECTED_COUNTS, "resource owners disagree with aggregate counts")
    names = sorted(f"{r['namespace']}/{r['name']}" for r in resources if r["owner"] == "Native")
    require(names == sorted(NO_MARKER), "no-marker identities mismatch")


def validate_views_report(report: dict, owner: str | None = None, summary: bool = False) -> None:
    """Validate one explicit opt-in report variant against the pinned input."""
    def require(condition, message):
        if not condition:
            raise ValueError(message)

    if owner not in (None, "Native"):
        raise ValueError("unsupported recorded map owner expectation")
    schema = "map-list-recorded-summary.v1" if summary else "map-list-recorded.v1"
    expected_fields = {"schema", "provenance", "scope", "selectedCount",
                       "excludedFromScopeCount", "ownerCounts"}
    if summary:
        expected_fields.update({"view", "perObjectEvidenceGuide"})
    else:
        expected_fields.add("resources")
    require(isinstance(report, dict) and set(report) == expected_fields,
            "recorded view has unexpected or missing fields")
    require(report.get("schema") == schema, "unexpected recorded view schema")
    if summary:
        require(report.get("view") == "summary", "summary view marker mismatch")
        guide = report.get("perObjectEvidenceGuide")
        require(isinstance(guide, str) and "per-object detector evidence" in guide.lower(),
                "summary lacks per-object evidence guidance")

    provenance = report.get("provenance")
    expected_provenance = {
        "kind": "kubernetes-object-recording", "sha256": DEPLOYMENT_SHA256,
        "bytes": 1031673, "documents": 1, "objectCount": 302,
        "captureTime": "unknown", "captureCompleteness": "unknown",
    }
    require(provenance == expected_provenance, "recorded view provenance mismatch")
    expected_scope = {"apiVersion": "apps/v1", "kind": "Deployment",
                      "namespacePrefix": "team-"}
    if owner is not None:
        expected_scope["owner"] = owner
    require(report.get("scope") == expected_scope, "recorded view scope mismatch")

    selected = 12 if owner == "Native" else 300
    excluded = 302 - selected
    expected_counts = {"Native": 12} if owner == "Native" else EXPECTED_COUNTS
    require(report.get("selectedCount") == selected and
            report.get("excludedFromScopeCount") == excluded,
            "recorded view selection counts mismatch")
    counts = report.get("ownerCounts")
    require(counts == expected_counts, "recorded view owner counts mismatch")
    if summary:
        require("resources" not in report, "summary must omit resource rows, not encode an empty inventory")
        return

    resources = report.get("resources")
    require(isinstance(resources, list) and len(resources) == selected,
            "recorded view resource count mismatch")
    seen = set()
    actual_counts = {}
    for resource in resources:
        require(isinstance(resource, dict) and resource.get("apiVersion") == "apps/v1" and
                resource.get("kind") == "Deployment" and
                isinstance(resource.get("namespace"), str) and resource["namespace"].startswith("team-") and
                isinstance(resource.get("name"), str) and bool(resource["name"]),
                "resource outside expected recorded scope")
        identity = (resource["apiVersion"], resource["kind"], resource["namespace"], resource["name"])
        require(identity not in seen, "duplicate recorded resource identity")
        seen.add(identity)
        actual_owner = resource.get("owner")
        require(actual_owner in expected_counts, "unexpected recorded owner category")
        if owner == "Native":
            detection = resource.get("ownershipDetection")
            require(actual_owner == "Native" and isinstance(detection, dict) and
                    detection.get("status") == "no_known_marker",
                    "Native-filtered row lacks no-marker detector evidence")
        actual_counts[actual_owner] = actual_counts.get(actual_owner, 0) + 1
    require(actual_counts == expected_counts, "resource owners disagree with view counts")
    names = sorted(f"{r['namespace']}/{r['name']}" for r in resources if r["owner"] == "Native")
    if owner == "Native" or owner is None:
        require(names == sorted(NO_MARKER), "no-marker identities mismatch")


def validate_views_reports(reports: dict) -> None:
    expected = {"full", "summary", "native", "native-summary"}
    if not isinstance(reports, dict) or set(reports) != expected:
        raise ValueError("views contract requires full, summary, Native, and Native-summary reports")
    validate_views_report(reports["full"])
    validate_views_report(reports["summary"], summary=True)
    validate_views_report(reports["native"], owner="Native")
    validate_views_report(reports["native-summary"], owner="Native", summary=True)
    for full_name, summary_name in (("full", "summary"), ("native", "native-summary")):
        full, summary = reports[full_name], reports[summary_name]
        for field in ("provenance", "scope", "selectedCount", "excludedFromScopeCount", "ownerCounts"):
            if summary.get(field) != full.get(field):
                raise ValueError(f"{summary_name} does not match full report {field}")


def call_map_stdio(wrapper: Path, kubeconfig: Path, recording: Path,
                   timeout: float = 12.0, contract: str = MAP_CONTRACT_BASIC) -> dict:
    if not wrapper.is_file() or not wrapper.stat().st_mode & 0o111:
        raise ValueError("recorded wrapper is missing or not executable")
    empty_config = b'apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n'
    if not kubeconfig.is_file() or kubeconfig.read_bytes() != empty_config:
        raise ValueError("owned offline kubeconfig is missing, changed, or not empty of credentials")
    original = recording.read_bytes()
    env = {"PATH": "/usr/bin:/bin", "KUBECONFIG": str(kubeconfig)}
    proc = subprocess.Popen([str(wrapper)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, env=env,
                            start_new_session=True)
    selector = selectors.DefaultSelector()
    assert proc.stdout is not None and proc.stdin is not None
    assert proc.stderr is not None
    selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
    selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
    os.set_blocking(proc.stdout.fileno(), False)
    os.set_blocking(proc.stderr.fileno(), False)
    line_buffer = bytearray()
    stderr_excerpt = bytearray()
    max_line = 8 * 1024 * 1024
    deadline = time.monotonic() + timeout
    previous = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
    def interrupted(signum, frame):
        raise RunInterrupted(signum)
    next_id = 1

    def send(obj: dict) -> None:
        proc.stdin.write((json.dumps(obj, separators=(",", ":")) + "\n").encode())
        proc.stdin.flush()

    def receive(expected_id: int) -> dict:
        nonlocal line_buffer
        while time.monotonic() < deadline:
            newline = line_buffer.find(b"\n")
            if newline >= 0:
                line = bytes(line_buffer[:newline])
                del line_buffer[:newline + 1]
                if not line.strip():
                    continue
                message = json.loads(line)
                if message.get("id") == expected_id:
                    return message
                continue
            if len(line_buffer) > max_line:
                raise ValueError("recorded MCP response exceeded bounded line length")
            ready = selector.select(min(0.1, max(0.0, deadline - time.monotonic())))
            for key, _ in ready:
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                elif key.data == "stdout":
                    line_buffer.extend(chunk)
                elif len(stderr_excerpt) < 16 * 1024:
                    stderr_excerpt.extend(chunk[:16 * 1024 - len(stderr_excerpt)])
            if proc.poll() is not None and not line_buffer and not selector.get_map():
                raise RuntimeError("recorded MCP server exited before responding")
        raise TimeoutError("recorded MCP session deadline expired")

    def request(method: str, params: dict) -> dict:
        nonlocal next_id
        ident = next_id
        next_id += 1
        send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
        return receive(ident)

    try:
        for sig in previous:
            signal.signal(sig, interrupted)
        init = request("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                                       "clientInfo": {"name": "recorded-scale-preflight", "version": "1"}})
        if "result" not in init:
            raise ValueError(f"MCP initialize failed: {init}")
        send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        tools_response = request("tools/list", {})
        tools = tools_response.get("result", {}).get("tools", [])
        names = [tool.get("name") for tool in tools]
        if names != ["explain", "map"]:
            raise ValueError(f"recorded plugin must expose exactly explain,map; got {names}")
        map_schema = next(tool["inputSchema"] for tool in tools if tool["name"] == "map")
        if not exact_map_schema(map_schema, contract):
            raise ValueError(f"recorded map tool schema differs from reviewed exact input contract: {map_schema}")

        # Change only this packet's private recording after server startup. The active
        # gateway must retain its already parsed immutable snapshot.
        recording.write_bytes(b"tampered-after-startup\n")
        live_args = request("tools/call", {"name": "map", "arguments": {
            "api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-", "kube_context": "default"}})
        if not live_args.get("result", {}).get("isError") and "error" not in live_args:
            raise ValueError("recorded MCP accepted unsupported live map argument")
        bad_tool = request("tools/call", {"name": "trace", "arguments": {"target": "deploy/api"}})
        if not bad_tool.get("result", {}).get("isError") and "error" not in bad_tool:
            raise ValueError("recorded MCP accepted unsupported live tool")
        variants = ([ ("full", {}) ] if contract == MAP_CONTRACT_BASIC else [
            ("full", {}), ("summary", {"summary": True}),
            ("native", {"owner": "Native"}),
            ("native-summary", {"owner": "Native", "summary": True}),
        ])
        reports = {}
        for name, extra in variants:
            arguments = {"api_version": "apps/v1", "kind": "Deployment",
                         "namespace_prefix": "team-", **extra}
            exact = request("tools/call", {"name": "map", "arguments": arguments})
            result = exact.get("result", {})
            if result.get("isError"):
                raise ValueError(f"recorded MCP {name} call failed: {result}")
            content = result.get("content", [])
            texts = [item.get("text", "") for item in content if item.get("type") == "text"]
            report = None
            for text in texts:
                try:
                    report = json.loads(text)
                    break
                except json.JSONDecodeError:
                    continue
            if report is None:
                raise ValueError(f"recorded MCP {name} call returned no JSON report")
            if contract == MAP_CONTRACT_BASIC:
                validate_report(report)
            reports[name] = report
        if contract == MAP_CONTRACT_VIEWS:
            validate_views_reports(reports)
        return {"tools": names, "unsupportedToolRejected": True,
                "unsupportedLiveArgumentRejected": True, "reports": reports,
                "report": reports["full"]}
    finally:
        for sig in previous:
            signal.signal(sig, signal.SIG_IGN)
        try:
            try:
                recording.write_bytes(original)
            finally:
                try:
                    proc.stdin.close()
                except BrokenPipeError:
                    pass
                # Always clean the owned group, even after a successful leader exit.
                stop_owned_group(proc.pid, grace=0.35, process=proc)
                proc.wait(timeout=1)
        finally:
            selector.close()
            proc.stdout.close()
            proc.stderr.close()
            for sig, handler in previous.items():
                signal.signal(sig, handler)


def verify_prepared(root: Path, binary: Path, expected_hash: str,
                    requested_contract: str | None = None) -> dict:
    root = root.resolve(strict=True)
    facts = json.loads((root / "prepared.json").read_text())
    selected_contract = facts.get("mapInputContract", MAP_CONTRACT_BASIC)
    if selected_contract not in MAP_CONTRACTS:
        raise ValueError("prepared map input contract is missing or unsupported")
    if requested_contract is not None and requested_contract != selected_contract:
        raise ValueError("requested map contract differs from prepared.json")
    binary = binary.expanduser().resolve(strict=True)
    try:
        prepared_binary = Path(facts["binary"]).expanduser().resolve(strict=True)
    except (KeyError, OSError) as exc:
        raise ValueError("prepared binary path is missing or unreadable") from exc
    if prepared_binary != binary:
        raise ValueError("explicit binary path differs from the path embedded in the prepared wrapper")
    if facts.get("binarySha256") != expected_hash or sha256(binary) != expected_hash:
        raise ValueError("explicit binary does not match the prepared pinned hash")
    manifest = root / "source-recording-manifest.json"
    if sha256(manifest) != facts.get("recordingManifestSha256"):
        raise ValueError("preserved source manifest hash mismatch")
    plugin = root / "plugin"
    listed = facts.get("generatedPluginFiles")
    if not isinstance(listed, dict):
        raise ValueError("prepared generated-file inventory is missing")
    actual_paths = set()
    for path in plugin.rglob("*"):
        if path.is_symlink():
            raise ValueError(f"unexpected symlink in generated plugin: {path.relative_to(plugin)}")
        if path.is_file():
            actual_paths.add(path.relative_to(plugin).as_posix())
    if actual_paths != set(listed):
        raise ValueError("generated plugin has missing or unrecorded files")
    for relative, digest in listed.items():
        path = (plugin / relative).resolve(strict=True)
        if plugin.resolve() not in path.parents or sha256(path) != digest:
            raise ValueError(f"generated plugin file hash mismatch: {relative}")
    kubeconfig = root / "kubeconfig-empty"
    expected_config = b'apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n'
    if not kubeconfig.is_file() or kubeconfig.read_bytes() != expected_config:
        raise ValueError("explicit offline kubeconfig is missing or has changed")
    return facts


def call_map_cli(binary: Path, kubeconfig: Path, recording: Path,
                 contract: str = MAP_CONTRACT_BASIC) -> dict:
    if contract not in MAP_CONTRACTS:
        raise ValueError("unsupported map input contract")
    env = {"PATH": "/usr/bin:/bin", "KUBECONFIG": str(kubeconfig)}
    variants = ([ ("full", None, False) ] if contract == MAP_CONTRACT_BASIC else [
        ("full", None, False), ("summary", None, True),
        ("native", "Native", False), ("native-summary", "Native", True),
    ])
    reports = {}
    for name, owner, summary in variants:
        command = [str(binary), "map", "list", "--recording", str(recording),
                   "--api-version", "apps/v1", "--kind", "Deployment",
                   "--namespace-prefix", "team-", "--format", "json"]
        if owner is not None:
            command.extend(["--owner", owner])
        if summary:
            command.append("--summary")
        code, stdout, stderr = run_bounded(command, env, timeout=15)
        if code != 0:
            raise ValueError(f"recorded CLI {name} call failed ({code}): {stderr.decode(errors='replace')[-1000:]}")
        try:
            report = json.loads(stdout)
        except json.JSONDecodeError as exc:
            raise ValueError(f"recorded CLI {name} call returned invalid JSON") from exc
        if contract == MAP_CONTRACT_BASIC:
            validate_report(report)
        else:
            validate_views_report(report, owner=owner, summary=summary)
        reports[name] = report
    if contract == MAP_CONTRACT_VIEWS:
        validate_views_reports(reports)
    return reports


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("prepared", type=Path)
    parser.add_argument("--binary", required=True, type=Path, help="explicit local product binary; never searched for")
    parser.add_argument("--binary-sha256", required=True)
    parser.add_argument("--map-contract", choices=MAP_CONTRACTS,
                        help="Require this recorded map input contract; defaults to prepared.json")
    args = parser.parse_args()
    facts = verify_prepared(args.prepared, args.binary, args.binary_sha256, args.map_contract)
    contract = facts.get("mapInputContract", MAP_CONTRACT_BASIC)
    root = args.prepared.resolve()
    plugin = root / "plugin"
    kubeconfig = root / "kubeconfig-empty"
    recording = plugin / "recordings/deployments.yaml"
    cli_reports = call_map_cli(args.binary.resolve(), kubeconfig, recording, contract)
    report = call_map_stdio(plugin / "bin/recorded-cub-scout", root / "kubeconfig-empty",
                            recording, contract=contract)
    if report["reports"] != cli_reports:
        raise ValueError("recorded CLI and MCP reports differ")
    print(json.dumps({"preflight": "passed", "binarySha256": facts["binarySha256"],
                      "recordingSha256": facts["deploymentRecordingSha256"],
                      "mapInputContract": contract, "viewsChecked": list(cli_reports),
                      "tools": report["tools"], "selected": report["report"]["selectedCount"],
                      "cliMcpEqual": True, "modelRun": False, "liveCluster": False}, sort_keys=True))


if __name__ == "__main__":
    main()
