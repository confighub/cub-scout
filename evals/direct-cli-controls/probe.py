#!/usr/bin/env python3
"""Bounded, API-only direct Claude CLI wiring probe against a loopback fixture.

No invocation occurs without --execute and reviewed binary pins. This is a
diagnostic harness, not provider/billing enforcement or benchmark evidence.
"""
from __future__ import annotations

import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import selectors
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import uuid

MAX_BODY = 2 * 1024 * 1024
MAX_OUTPUT = 2 * 1024 * 1024
MAX_REQUESTS = 4
CLI_TIMEOUT = 60
FAKE_KEY = "sk-ant-api03-cub-scout-offline-fake-key"
PROMPT = "Return exactly: offline-probe-terminal"


class ProbeError(RuntimeError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def private_output(path: Path) -> Path:
    path = path.absolute()
    if path.exists() or path.is_symlink():
        raise ProbeError("output path already exists (including symlinks)")
    path.mkdir(mode=0o700, parents=True)
    os.chmod(path, 0o700)
    return path


def minimal_env(config_dir: Path, port: int) -> dict[str, str]:
    """Construct a child environment from scratch; never inherit caller auth."""
    return {
        "PATH": "/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        "HOME": str(config_dir), "CLAUDE_CONFIG_DIR": str(config_dir),
        "ANTHROPIC_API_KEY": FAKE_KEY,
        "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
        "CLAUDE_CODE_DISABLE_NON_ESSENTIAL_TRAFFIC": "1",
        "CLAUDE_CODE_SIMPLE": "1", "CI": "1", "NO_COLOR": "1",
        "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "",
        "http_proxy": "", "https_proxy": "", "all_proxy": "",
    }


def sandbox_prefix(port: int) -> list[str]:
    return sandbox_prefix_for_test(platform.system(), shutil.which("sandbox-exec"), port)


def sandbox_prefix_for_test(system: str, executable: str | None = "/usr/bin/sandbox-exec", port: int = 1) -> list[str]:
    if system != "Darwin" or not executable:
        raise ProbeError("macOS sandbox-exec unavailable; refusing to run CLI")
    profile = f'(version 1) (allow default) (deny network*) (allow network-outbound (remote ip "localhost:{port}"))'
    return [executable, "-p", profile]


def network_preflight(port: int) -> None:
    """Verify local allowance plus denied other-loopback and non-loopback paths."""
    prefix = sandbox_prefix(port)
    code = ("import errno,socket,sys; s=socket.socket(); s.settimeout(1); "
            "\ntry: s.connect((sys.argv[1],int(sys.argv[2]))); sys.exit(8)"
            "\nexcept OSError as e: sys.exit(0 if e.errno==errno.EPERM else 9)")
    for host, other in (("127.0.0.1", port + 1), ("203.0.113.1", 443)):
        result = subprocess.run(prefix + [sys.executable, "-c", code, host, str(other)],
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=3, env={"PATH": "/usr/bin:/bin"})
        if result.returncode != 0:
            raise ProbeError("sandbox denial check failed closed (expected EPERM)")


def validate_probe(stdout: bytes, requests: list[dict]) -> dict:
    """Require one actual terminal stream result and exact observed tool inventory."""
    if not requests or len(requests) > MAX_REQUESTS:
        raise ProbeError("missing or excessive provider request count")
    inventories = []
    ids = set()
    for request in requests:
        try:
            body = json.loads(request["body"])
        except Exception:
            raise ProbeError("malformed captured request") from None
        tools = body.get("tools", [])
        if not isinstance(tools, list):
            raise ProbeError("malformed tool inventory")
        names = [tool.get("name") for tool in tools if isinstance(tool, dict)]
        if len(names) != len(tools) or any(not isinstance(n, str) for n in names):
            raise ProbeError("malformed tool inventory")
        inventories.append(names)
        for tool in tools:
            if "id" in tool:
                if tool["id"] in ids:
                    raise ProbeError("duplicate tool-use ID")
                ids.add(tool["id"])
    if any(inv != inventories[0] for inv in inventories[1:]):
        raise ProbeError("provider request inventory changed across turns")
    names = inventories[0]
    if "Agent" in names or "Task" in names or any(n.lower() in ("agent", "task") for n in names):
        raise ProbeError("delegation tool appeared in actual provider inventory")
    try:
        events = [json.loads(line) for line in stdout.splitlines() if line.strip()]
    except Exception:
        raise ProbeError("CLI stream-json output is malformed") from None
    terminals = [e for e in events if isinstance(e, dict) and e.get("type") == "result"]
    if len(terminals) != 1 or terminals[0].get("subtype") not in ("success", "error_max_turns"):
        raise ProbeError("missing, duplicate, or malformed terminal result")
    return {"terminal_type": terminals[0]["type"], "terminal_subtype": terminals[0]["subtype"],
            "tool_inventory": names}


def response_for(stream: bool) -> bytes:
    if stream:
        events = [
            {"type": "message_start", "message": {"id": "msg_offline_fixture", "type": "message", "role": "assistant", "model": "offline-fixture", "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}},
            {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "offline-probe-terminal"}},
            {"type": "content_block_stop", "index": 0},
            {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 1}},
            {"type": "message_stop"},
        ]
        return b"".join(b"event: " + e["type"].encode() + b"\ndata: " + json.dumps(e).encode() + b"\n\n" for e in events)
    return json.dumps({"id": "msg_offline_fixture", "type": "message", "role": "assistant", "model": "offline-fixture", "content": [{"type": "text", "text": "offline-probe-terminal"}], "stop_reason": "end_turn", "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}).encode()


class FixtureServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = False


def handler_type(requests: list[dict]):
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_POST(self):
            if self.path != "/v1/messages" or len(requests) >= MAX_REQUESTS:
                self.send_error(404)
                return
            length = int(self.headers.get("Content-Length", "-1"))
            if length < 0 or length > MAX_BODY:
                self.send_error(413)
                return
            body = self.rfile.read(length)
            try:
                parsed = json.loads(body)
            except Exception:
                self.send_error(400)
                return
            requests.append({"path": self.path, "authorization": self.headers.get("Authorization", ""),
                             "content_type": self.headers.get("Content-Type", ""),
                             "body": body.decode("utf-8", "replace"), "tool_inventory": parsed.get("tools", [])})
            payload = response_for(self.headers.get("Accept", "").find("text/event-stream") >= 0)
            stream = self.headers.get("Accept", "").find("text/event-stream") >= 0
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream" if stream else "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, *_):
            pass
    return Handler


def run_bounded(argv: list[str], env: dict[str, str], cwd: Path, timeout: int = CLI_TIMEOUT,
                max_output: int = MAX_OUTPUT):
    proc = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            start_new_session=True)
    assert proc.stdout and proc.stderr
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ, "out")
    selector.register(proc.stderr, selectors.EVENT_READ, "err")
    captured = {"out": bytearray(), "err": bytearray()}
    deadline = time.monotonic() + timeout
    try:
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise ProbeError("owned CLI process group exceeded deadline")
            for key, _ in selector.select(min(remaining, 0.2)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                output = captured[key.data]
                if len(output) + len(chunk) > max_output:
                    raise ProbeError("CLI output exceeded configured bound")
                output.extend(chunk)
        return proc.wait(), bytes(captured["out"]), bytes(captured["err"])
    except BaseException:
        try:
            os.killpg(proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.wait()
        raise
    finally:
        selector.close()
        for stream in (proc.stdout, proc.stderr):
            if not stream.closed:
                stream.close()


def execute(output: Path, cli: Path, expected_hash: str, expected_version: str) -> dict:
    if platform.system() != "Darwin":
        raise ProbeError("unsupported platform; fail closed")
    if not cli.is_absolute() or not cli.is_file() or cli.is_symlink():
        raise ProbeError("CLI must be an absolute regular executable path")
    binary_hash = sha256(cli.read_bytes())
    if binary_hash != expected_hash:
        raise ProbeError("CLI SHA-256 does not match reviewed pin")
    # Version check is read-only and still sandboxed. Do not persist its output.
    if expected_version != "2.1.274":
        raise ProbeError("this probe implementation is pinned to reviewed CLI 2.1.274")
    root = private_output(output)
    config = root / "config"
    config.mkdir(mode=0o700)
    os.chmod(config, 0o700)
    requests: list[dict] = []
    server = FixtureServer(("127.0.0.1", 0), handler_type(requests))
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    proc_record = {"started_at": time.time(), "cli_path": str(cli), "cli_version_pin": expected_version,
                   "cli_sha256": binary_hash, "python": sys.version, "platform": platform.platform(),
                   "cwd": str(root), "fake_key_used": True, "sandbox": "macOS sandbox-exec", "requests": requests}
    try:
        # Probe the allowed endpoint under the exact same boundary first.
        prefix = sandbox_prefix(port)
        probe = subprocess.run(prefix + [sys.executable, "-c",
            "import socket,sys; s=socket.create_connection(('127.0.0.1',int(sys.argv[1])),1); s.close()",
            str(port)], env={"PATH": "/usr/bin:/bin"}, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL, timeout=3)
        if probe.returncode:
            raise ProbeError("designated loopback sandbox check failed")
        network_preflight(port)
        env = minimal_env(config, port)
        mcp = root / "empty-mcp.json"
        mcp.write_text('{"mcpServers":{}}\n')
        os.chmod(mcp, 0o600)
        argv = prefix + [str(cli), "--bare", "--print", "--verbose", "--output-format", "stream-json",
            "--no-session-persistence", "--tools", "Read", "--disallowedTools", "Task,Agent",
            "--strict-mcp-config", "--mcp-config", str(mcp), "--setting-sources", "", "--max-turns", "1",
            "--max-budget-usd", "0.05", PROMPT]
        rc, stdout, stderr = run_bounded(argv, env, root)
        (root / "stdout.bin").write_bytes(stdout)
        (root / "stderr.bin").write_bytes(stderr)
        os.chmod(root / "stdout.bin", 0o600); os.chmod(root / "stderr.bin", 0o600)
        (root / "requests.json").write_text(json.dumps(requests, indent=2) + "\n")
        os.chmod(root / "requests.json", 0o600)
        proc_record.update({"return_code": rc, "stdout_sha256": sha256(stdout), "stderr_sha256": sha256(stderr),
                            "request_count": len(requests), "result": "wiring_only"})
        try:
            proc_record["observed"] = validate_probe(stdout, requests)
        except ProbeError as exc:
            proc_record["validation_error"] = str(exc)
            (root / "provenance.json").write_text(json.dumps(proc_record, indent=2) + "\n")
            os.chmod(root / "provenance.json", 0o600)
            raise
        (root / "provenance.json").write_text(json.dumps(proc_record, indent=2) + "\n")
        os.chmod(root / "provenance.json", 0o600)
        return proc_record
    finally:
        server.shutdown(); server.server_close(); thread.join(timeout=2)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cli", type=Path, default=Path("/opt/homebrew/bin/claude"))
    parser.add_argument("--expected-sha256")
    parser.add_argument("--expected-version")
    parser.add_argument("--execute", action="store_true", help="perform the one bounded mock-provider probe")
    args = parser.parse_args()
    if not args.execute:
        parser.error("refusing to run without explicit --execute")
    if not args.expected_sha256 or not args.expected_version:
        parser.error("--expected-sha256 and --expected-version are required reviewed pins")
    try:
        result = execute(args.output, args.cli, args.expected_sha256, args.expected_version)
    except (ProbeError, OSError, subprocess.SubprocessError) as exc:
        print(f"probe refused/failed: {exc}", file=sys.stderr)
        return 2
    print(json.dumps({k: result[k] for k in ("return_code", "request_count", "result", "cli_version_pin", "cli_sha256")}, indent=2))
    return 0 if result["return_code"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
