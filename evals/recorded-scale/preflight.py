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
    if process is not None and process.poll() is not None:
        return
    try:
        os.killpg(pgid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def exact_map_schema(schema: dict) -> bool:
    properties = schema.get("properties")
    return (isinstance(properties, dict) and
            set(properties) == {"api_version", "kind", "namespace", "namespace_prefix"} and
            set(schema.get("required", [])) == {"api_version", "kind"} and
            schema.get("additionalProperties") is False and
            all(isinstance(prop, dict) and prop.get("type") == "string"
                for prop in properties.values()))


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
        if proc.poll() is None or selector.get_map():
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
    assert report["schema"] == "map-list-recorded.v1", report.get("schema")
    assert report["provenance"]["objectCount"] == 302, report["provenance"]
    assert report["provenance"]["sha256"] == DEPLOYMENT_SHA256, report["provenance"]
    assert report["scope"] == {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"}, report["scope"]
    assert report["selectedCount"] == 300 and report["excludedFromScopeCount"] == 2
    assert len(report["resources"]) == 300
    assert report["ownerCounts"] == EXPECTED_COUNTS, report["ownerCounts"]
    names = sorted(f"{r['namespace']}/{r['name']}" for r in report["resources"] if r["owner"] == "Native")
    assert names == sorted(NO_MARKER), names


def call_map_stdio(wrapper: Path, kubeconfig: Path, recording: Path,
                   timeout: float = 12.0) -> dict:
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

    completed = False
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
        if not exact_map_schema(map_schema):
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
        exact = request("tools/call", {"name": "map", "arguments": {
            "api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-"}})
        result = exact.get("result", {})
        if result.get("isError"):
            raise ValueError(f"recorded MCP exact call failed: {result}")
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
            raise ValueError("recorded MCP exact call returned no JSON report")
        validate_report(report)
        completed = True
        return {"tools": names, "unsupportedToolRejected": True,
                "unsupportedLiveArgumentRejected": True, "report": report}
    finally:
        recording.write_bytes(original)
        for sig in previous:
            signal.signal(sig, signal.SIG_IGN)
        try:
            proc.stdin.close()
        except BrokenPipeError:
            pass
        if completed:
            try:
                proc.wait(timeout=1)
            except subprocess.TimeoutExpired:
                stop_owned_group(proc.pid, grace=0.35, process=proc)
                proc.wait(timeout=1)
        else:
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


def verify_prepared(root: Path, binary: Path, expected_hash: str) -> dict:
    root = root.resolve(strict=True)
    facts = json.loads((root / "prepared.json").read_text())
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


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("prepared", type=Path)
    parser.add_argument("--binary", required=True, type=Path, help="explicit local product binary; never searched for")
    parser.add_argument("--binary-sha256", required=True)
    args = parser.parse_args()
    facts = verify_prepared(args.prepared, args.binary, args.binary_sha256)
    root = args.prepared.resolve()
    plugin = root / "plugin"
    env = {"PATH": "/usr/bin:/bin", "KUBECONFIG": str(root / "kubeconfig-empty")}
    code, stdout, stderr = run_bounded([str(args.binary.resolve()), "map", "list", "--recording",
                          str(plugin / "recordings/deployments.yaml"), "--api-version", "apps/v1",
                          "--kind", "Deployment", "--namespace-prefix", "team-", "--format", "json"], env, timeout=15)
    if code != 0:
        raise ValueError(f"recorded CLI call failed ({code}): {stderr.decode(errors='replace')[-1000:]}")
    cli_report = json.loads(stdout)
    validate_report(cli_report)
    report = call_map_stdio(plugin / "bin/recorded-cub-scout", root / "kubeconfig-empty",
                            plugin / "recordings/deployments.yaml")
    if report["report"] != cli_report:
        raise ValueError("recorded CLI and MCP reports differ")
    print(json.dumps({"preflight": "passed", "binarySha256": facts["binarySha256"],
                      "recordingSha256": facts["deploymentRecordingSha256"],
                      "tools": report["tools"], "selected": report["report"]["selectedCount"],
                      "cliMcpEqual": True, "modelRun": False, "liveCluster": False}, sort_keys=True))


if __name__ == "__main__":
    main()
