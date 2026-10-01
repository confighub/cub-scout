#!/usr/bin/env python3
"""Offline-only direct Claude CLI control probe. Runtime mode requires review pins."""
from __future__ import annotations

import argparse
import base64
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import re
import secrets
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import threading
import time

MAX_BODY = 2 * 1024 * 1024
MAX_OUTPUT = 2 * 1024 * 1024
MAX_REQUESTS = 4
OVERALL_TIMEOUT = 90.0
CLI_TIMEOUT = 60.0
PINNED_VERSION = "2.1.274"
PINNED_SHA256 = "3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a"
PINNED_CLI = "/opt/homebrew/Caskroom/claude-code/2.1.274/claude"
FAKE_KEY = "sk-ant-api03-cub-scout-offline-fake-key"
PROMPT = "Return exactly: offline-probe-terminal"
READ_TIMEOUT = 2.0


class ProbeError(RuntimeError):
    pass


class ProcessFailure(ProbeError):
    def __init__(self, message, status, stdout=b"", stderr=b""):
        super().__init__(message)
        self.status, self.stdout, self.stderr = status, stdout, stderr


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024*1024): digest.update(chunk)
    return digest.hexdigest()


def strict_json(data: bytes | str):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate JSON key")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=pairs)


def private_output(path: Path) -> Path:
    path = path.absolute()
    if path.exists() or path.is_symlink():
        raise ProbeError("output path already exists (including symlinks)")
    path.mkdir(mode=0o700, parents=True)
    os.chmod(path, 0o700)
    return path


def minimal_env(config_dir: Path, port: int) -> dict[str, str]:
    return {
        "PATH": "/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        "CLAUDE_CONFIG_DIR": str(config_dir),
        "ANTHROPIC_API_KEY": FAKE_KEY,
        "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{port}",
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
        "CLAUDE_CODE_DISABLE_FAST_MODE": "1",
        "CI": "1", "NO_COLOR": "1",
        "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "",
        "http_proxy": "", "https_proxy": "", "all_proxy": "",
    }


def sandbox_prefix_for_test(system: str, executable: str | None, port: int) -> list[str]:
    if system != "Darwin" or not executable:
        raise ProbeError("macOS sandbox-exec unavailable; refusing to run")
    profile = f'(version 1) (allow default) (deny network*) (allow network-outbound (remote ip "localhost:{port}"))'
    return [executable, "-p", profile]


def sandbox_prefix(port: int) -> list[str]:
    return sandbox_prefix_for_test(platform.system(), shutil.which("sandbox-exec"), port)


def validate_probe(stdout: bytes, requests: list[dict], return_code: int) -> dict:
    if type(return_code) is not int or return_code != 0:
        raise ProbeError("CLI did not exit successfully")
    if not requests or len(requests) > MAX_REQUESTS:
        raise ProbeError("missing or excessive provider request count")
    inventories = []
    for record in requests:
        body = strict_json(record["body"])
        if not isinstance(body, dict):
            raise ProbeError("provider request JSON is not an object")
        tools = body.get("tools")
        if not isinstance(tools, list) or any(not isinstance(t, dict) for t in tools):
            raise ProbeError("provider request lacks a valid tool inventory")
        names = [t.get("name") for t in tools]
        if names != ["Read"]:
            raise ProbeError("observed tool inventory is not exactly ['Read']")
        inventories.append(names)
    if any(item != inventories[0] for item in inventories[1:]):
        raise ProbeError("provider request inventory changed")
    try:
        events = [strict_json(line) for line in stdout.splitlines() if line.strip()]
    except (ValueError, TypeError):
        raise ProbeError("CLI stream-json output is malformed") from None
    terminals = [e for e in events if isinstance(e, dict) and e.get("type") == "result"]
    if len(terminals) != 1:
        raise ProbeError("expected exactly one terminal result")
    terminal = terminals[0]
    if (terminal.get("subtype") != "success" or terminal.get("is_error") is True
            or terminal.get("result") != "offline-probe-terminal"):
        raise ProbeError("terminal result did not match the expected successful fixture")
    return {"terminal_type": "result", "terminal_subtype": "success",
            "terminal_text": "offline-probe-terminal", "tool_inventory": ["Read"]}


def response_for(stream: bool) -> tuple[str, bytes]:
    if stream:
        events = [
            {"type": "message_start", "message": {"id": "msg_offline_fixture", "type": "message", "role": "assistant", "model": "offline-fixture", "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}},
            {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "offline-probe-terminal"}},
            {"type": "content_block_stop", "index": 0},
            {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 1}},
            {"type": "message_stop"},
        ]
        return "text/event-stream", b"".join(b"event: " + e["type"].encode() + b"\ndata: " + json.dumps(e).encode() + b"\n\n" for e in events)
    value = {"id": "msg_offline_fixture", "type": "message", "role": "assistant", "model": "offline-fixture", "content": [{"type": "text", "text": "offline-probe-terminal"}], "stop_reason": "end_turn", "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}
    return "application/json", json.dumps(value).encode()


def reserve_request(count: list[int], lock: threading.Lock) -> int:
    """Count valid and rejected arrivals, saturating after one over-limit marker."""
    with lock:
        count[0] = min(MAX_REQUESTS+1, count[0]+1)
        return count[0]


def handle_request(method: str, path: str, headers: dict, body: bytes, records: list[dict], lock: threading.Lock,
                   count: list[int], reserved_ordinal: int | None = None) -> tuple[int, str, bytes]:
    """Pure bounded request decision; only safe metadata/raw valid body is retained."""
    if reserved_ordinal is not None:
        ordinal = reserved_ordinal
    else:
        with lock:
            count[0] += 1
            ordinal = count[0]
    if ordinal > MAX_REQUESTS:
        return 429, "application/json", b'{"error":"request limit"}'
    if method != "POST":
        return 405, "application/json", b'{"error":"POST required"}'
    auth = headers.get("x-api-key", "")
    bearer = headers.get("authorization", "")
    if not ((auth == FAKE_KEY and not bearer) or (bearer == "Bearer " + FAKE_KEY and not auth)):
        return 401, "application/json", b'{"error":"invalid synthetic auth"}'
    if path not in ("/v1/messages", "/v1/messages?beta=true"):
        return 404, "application/json", b'{"error":"unexpected path"}'
    length = headers.get("content-length", "")
    if not re.fullmatch(r"[0-9]+", length):
        return 400, "application/json", b'{"error":"invalid content length"}'
    if int(length) != len(body) or len(body) > MAX_BODY:
        return 413, "application/json", b'{"error":"invalid or oversized body"}'
    try:
        parsed = strict_json(body)
    except (ValueError, TypeError):
        return 400, "application/json", b'{"error":"malformed JSON"}'
    if not isinstance(parsed, dict) or type(parsed.get("stream")) is not bool:
        return 400, "application/json", b'{"error":"stream must be boolean"}'
    # No auth header/value is retained. Accepted body is kept byte-for-byte.
    with lock:
        records.append({"path": path, "auth_kind": "x-api-key" if auth else "bearer",
                        "content_type": headers.get("content-type", ""),
                        "body_base64": base64.b64encode(body).decode("ascii"),
                        "body": body.decode("utf-8")})
    kind, payload = response_for(parsed["stream"])
    return 200, kind, payload


class FixtureServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = False
    request_queue_size = 8
    def __init__(self, *args, request_count: list[int], request_lock: threading.Lock, **kwargs):
        self._worker_slots = threading.BoundedSemaphore(MAX_REQUESTS)
        self._worker_lock = threading.Lock()
        self._workers = set()
        self._request_count = request_count
        self._request_lock = request_lock
        self._request_ordinals = {}
        self._thread_ordinal = threading.local()
        super().__init__(*args, **kwargs)
    def process_request(self, request, client_address):
        ordinal = reserve_request(self._request_count, self._request_lock)
        if ordinal > MAX_REQUESTS:
            self.shutdown_request(request)
            return
        if not self._worker_slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        with self._worker_lock: self._request_ordinals[id(request)] = ordinal
        try:
            super().process_request(request, client_address)
        except BaseException:
            with self._worker_lock: self._request_ordinals.pop(id(request), None)
            self._worker_slots.release()
            raise
    def process_request_thread(self, request, client_address):
        current = threading.current_thread()
        with self._worker_lock:
            self._workers.add(current)
            self._thread_ordinal.value = self._request_ordinals.pop(id(request))
        try:
            super().process_request_thread(request, client_address)
        finally:
            with self._worker_lock:
                self._workers.discard(current)
                del self._thread_ordinal.value
            self._worker_slots.release()
    def request_ordinal(self, request):
        return self._thread_ordinal.value
    def join_workers(self, timeout: float):
        deadline = time.monotonic()+timeout
        while time.monotonic() < deadline:
            with self._worker_lock: workers = list(self._workers)
            if not workers: return True
            for worker in workers: worker.join(min(0.1, max(0.0, deadline-time.monotonic())))
        with self._worker_lock: return not self._workers


def read_bounded_body(reader, length: int, timeout: float = READ_TIMEOUT) -> bytes:
    if length < 0 or length > MAX_BODY: raise ValueError("body length outside limit")
    deadline = time.monotonic()+timeout
    data = bytearray()
    while len(data) < length:
        if time.monotonic() >= deadline: raise TimeoutError("request body read deadline exceeded")
        chunk = reader(min(65536, length-len(data)))
        if time.monotonic() > deadline: raise TimeoutError("request body read deadline exceeded")
        if not chunk: raise ValueError("incomplete request body")
        data.extend(chunk)
    return bytes(data)


def handler_type(records: list[dict], lock: threading.Lock, count: list[int], preflight_path: str):
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        def setup(self):
            super().setup()
            self.request.settimeout(READ_TIMEOUT)
        def _dispatch(self):
            ordinal = self.server.request_ordinal(self.request)
            if self.command == "GET" and self.path == preflight_path:
                with lock: count[0] = max(0, count[0]-1)
                self.send_response(204); self.send_header("Content-Length", "0")
                self.send_header("Connection", "close"); self.end_headers(); return
            if (len(self.headers.get_all("x-api-key", [])) > 1
                    or len(self.headers.get_all("authorization", [])) > 1
                    or len(self.headers.get_all("content-length", [])) != 1
                    or self.headers.get_all("transfer-encoding")):
                status, kind, payload = 400, "application/json", b'{"error":"ambiguous request headers"}'
                self.send_response(status); self.send_header("Content-Type", kind)
                self.send_header("Content-Length", str(len(payload))); self.send_header("Connection", "close")
                self.end_headers(); self.wfile.write(payload); return
            raw_length = self.headers.get("Content-Length", "")
            if not re.fullmatch(r"[0-9]+", raw_length) or int(raw_length) > MAX_BODY:
                status, kind, payload = 400, "application/json", b'{"error":"invalid length"}'
            else:
                try:
                    body = read_bounded_body(self.rfile.read, int(raw_length))
                except TimeoutError:
                    body = b""
                    status, kind, payload = 408, "application/json", b'{"error":"body read timeout"}'
                except ValueError:
                    status, kind, payload = 400, "application/json", b'{"error":"incomplete body"}'
                else:
                    status, kind, payload = handle_request(self.command, self.path, {k.lower(): v for k, v in self.headers.items()}, body, records, lock, count, reserved_ordinal=ordinal)
            self.send_response(status)
            self.send_header("Content-Type", kind)
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Connection", "close")
            self.end_headers()
            try:
                self.wfile.write(payload)
            except (BrokenPipeError, ConnectionResetError):
                pass
        def do_POST(self): self._dispatch()
        def __getattr__(self, name):
            if name.startswith("do_"): return self._dispatch
            raise AttributeError(name)
        def log_message(self, *_): pass
    return Handler


def _kill_owned_group(pgid: int, grace: float = 0.4):
    """Signal only the dedicated group created by this helper's Popen."""
    try:
        os.killpg(pgid, signal.SIGTERM)
    except ProcessLookupError:
        return
    except PermissionError:
        pass
    time.sleep(grace)
    try:
        os.killpg(pgid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass


def run_bounded(argv: list[str], env: dict[str, str], cwd: Path, deadline: float,
                max_output: int = MAX_OUTPUT):
    proc = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    assert proc.stdout and proc.stderr
    pgid = proc.pid
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ, "out")
    selector.register(proc.stderr, selectors.EVENT_READ, "err")
    captured = {"out": bytearray(), "err": bytearray()}
    status = "running"
    try:
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                status = "timeout"
                raise ProcessFailure("CLI exceeded deadline", status)
            for key, _ in selector.select(min(remaining, 0.2)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj); key.fileobj.close(); continue
                target = captured[key.data]
                room = max_output - len(target)
                if len(chunk) > room:
                    target.extend(chunk[:max(0, room)])
                    status = "output_limit"
                    raise ProcessFailure("CLI output exceeded bound", status)
                target.extend(chunk)
        rc = proc.wait()
        if rc:
            status = "nonzero_exit"
            raise ProcessFailure("CLI exited nonzero", status)
        status = "success"
        return rc, bytes(captured["out"]), bytes(captured["err"]), status
    except ProcessFailure as exc:
        exc.stdout, exc.stderr = bytes(captured["out"]), bytes(captured["err"])
        raise
    except BaseException as exc:
        status = "interrupted" if isinstance(exc, (KeyboardInterrupt, SystemExit)) else "runner_error"
        raise ProcessFailure(type(exc).__name__, status, bytes(captured["out"]), bytes(captured["err"])) from None
    finally:
        # Always clean the process group, even when its original parent exited.
        _kill_owned_group(pgid)
        try:
            if proc.poll() is None: proc.wait(timeout=0.5)
        except subprocess.TimeoutExpired:
            pass
        selector.close()
        for stream in (proc.stdout, proc.stderr):
            if not stream.closed: stream.close()


def write_private(root: Path, name: str, data: bytes):
    path = root / name
    with path.open("xb") as stream:
        os.chmod(path, 0o600)
        stream.write(data)


def persist_trace(root: Path, record: dict, stdout: bytes, stderr: bytes, requests: list[dict]):
    """Persist bounded partial artifacts and provenance without following/replacing files."""
    for name, payload in (("stdout.bin", stdout), ("stderr.bin", stderr),
                          ("requests.json", json.dumps(requests, indent=2).encode()+b"\n")):
        if (root / name).is_symlink():
            raise ProbeError("refusing symlinked trace artifact")
        if not (root / name).exists():
            write_private(root, name, payload)
    path = root / "provenance.json"
    if path.is_symlink(): raise ProbeError("refusing symlinked provenance")
    encoded = json.dumps(record, indent=2).encode()+b"\n"
    if path.exists():
        path.write_bytes(encoded)
        os.chmod(path, 0o600)
    else:
        write_private(root, "provenance.json", encoded)


def execute(output: Path, cli: Path, expected_hash: str, expected_version: str) -> dict:
    started = time.monotonic()
    wall_started = time.time()
    deadline = started + OVERALL_TIMEOUT - 3.0  # Reserve bounded teardown within the 90-second wall.
    root = private_output(output)
    config = root / "config"
    records: list[dict] = []
    count, lock = [0], threading.Lock()
    server = thread = None
    argv = []
    stdout = stderr = b""
    record = {"started_at": wall_started, "helper_sha256": sha256(Path(__file__).read_bytes()),
              "python": sys.version, "platform": platform.platform(), "output_dir": str(root),
              "config_dir_retained_private": str(config), "cli_version_pin": PINNED_VERSION,
              "cli_sha256_pin": PINNED_SHA256, "cli_argv": [], "request_count": 0}
    try:
        config.mkdir(mode=0o700)
        os.chmod(config, 0o700)
        if platform.system() != "Darwin": raise ProbeError("unsupported platform; fail closed")
        # Bind executable, hash, and version together; caller values must match all pins.
        if (str(cli) != PINNED_CLI or expected_hash != PINNED_SHA256 or expected_version != PINNED_VERSION
                or not cli.is_absolute() or cli.is_symlink() or not cli.is_file()
                or not stat.S_ISREG(cli.lstat().st_mode) or not os.access(cli, os.X_OK)):
            raise ProbeError("CLI path/version/hash must match the fixed reviewed executable pin")
        actual_hash = sha256_file(cli)
        if actual_hash != PINNED_SHA256: raise ProbeError("executable hash does not match fixed pin")
        record["cli_path"] = str(cli)
        record["cli_sha256_actual"] = actual_hash
        preflight_path = "/__offline_preflight_" + secrets.token_hex(16)
        server = FixtureServer(("127.0.0.1", 0), handler_type(records, lock, count, preflight_path), request_count=count, request_lock=lock)
        server.timeout = 0.2
        port = server.server_address[1]
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
        thread.start()
        prefix = sandbox_prefix(port)
        for host, other in (("127.0.0.1", port + 1), ("203.0.113.1", 443)):
            remaining = deadline-time.monotonic()
            if remaining <= 0: raise ProbeError("overall probe deadline exceeded")
            code = "import errno,socket,sys; s=socket.socket(); s.settimeout(1);\ntry: s.connect((sys.argv[1],int(sys.argv[2]))); sys.exit(8)\nexcept OSError as e: sys.exit(0 if e.errno==errno.EPERM else 9)"
            result = subprocess.run(prefix + [sys.executable, "-c", code, host, str(other)],
                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                timeout=min(3, remaining), env={"PATH": "/usr/bin:/bin"})
            if result.returncode: raise ProbeError("sandbox denial check failed; expected EPERM")
        remaining = deadline-time.monotonic()
        if remaining <= 0: raise ProbeError("overall probe deadline exceeded")
        preflight_code = ("import socket,sys; s=socket.create_connection(('127.0.0.1',int(sys.argv[1])),1); "
            f"s.sendall(b'GET {preflight_path} HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n'); "
            "r=s.recv(512); s.close(); sys.exit(0 if b'204' in r else 1)")
        allowed = subprocess.run(prefix + [sys.executable, "-c", preflight_code, str(port)],
            env={"PATH":"/usr/bin:/bin"}, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL, timeout=min(3, remaining))
        if allowed.returncode: raise ProbeError("designated loopback allowance check failed")
        env = minimal_env(config, port)
        mcp = root / "empty-mcp.json"
        mcp.write_text('{"mcpServers":{}}\n'); os.chmod(mcp, 0o600)
        argv = prefix + [str(cli), "--bare", "--print", "--verbose", "--output-format", "stream-json",
            "--no-session-persistence", "--tools", "Read", "--disallowedTools", "Task,Agent",
            "--strict-mcp-config", "--mcp-config", str(mcp), "--setting-sources", "", "--max-turns", "1",
            "--max-budget-usd", "0.05", PROMPT]
        record["cli_argv"] = argv
        rc, stdout, stderr, status = run_bounded(argv, env, root, min(deadline, time.monotonic()+CLI_TIMEOUT))
        write_private(root, "stdout.bin", stdout); write_private(root, "stderr.bin", stderr)
        write_private(root, "requests.json", json.dumps(records, indent=2).encode()+b"\n")
        record.update({"return_code": rc, "run_status": status, "stdout_sha256": sha256(stdout),
                       "stderr_sha256": sha256(stderr), "request_count": count[0],
                       "observed": validate_probe(stdout, records, rc)})
    except BaseException as exc:
        record["run_status"] = getattr(exc, "status", "error")
        record["error"] = str(exc)
        stdout = getattr(exc, "stdout", stdout)
        stderr = getattr(exc, "stderr", stderr)
        record["stdout_sha256"] = sha256(stdout)
        record["stderr_sha256"] = sha256(stderr)
        raise
    finally:
        cleanup = {"server_shutdown_attempted": server is not None, "server_thread_joined": False}
        if server is not None:
            try:
                server.shutdown(); cleanup["server_shutdown_complete"] = True
            except BaseException as cleanup_error:
                cleanup["server_shutdown_error"] = type(cleanup_error).__name__
            try:
                server.server_close()
                cleanup["handlers_joined"] = server.join_workers(READ_TIMEOUT + 0.5)
            except BaseException as cleanup_error:
                cleanup["server_cleanup_error"] = type(cleanup_error).__name__
        if thread is not None:
            thread.join(timeout=1.0); cleanup["server_thread_joined"] = not thread.is_alive()
        record["cleanup"] = cleanup
        record["ended_at"] = time.time()
        record["elapsed_seconds"] = round(time.monotonic()-started, 3)
        with lock:
            record["request_count"] = count[0]
            request_snapshot = list(records)
        persist_trace(root, record, stdout, stderr, request_snapshot)
    return record


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cli", type=Path, default=Path(PINNED_CLI))
    parser.add_argument("--expected-sha256", default=PINNED_SHA256)
    parser.add_argument("--expected-version", default=PINNED_VERSION)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    if not args.execute: parser.error("refusing to run without explicit --execute")
    try:
        result = execute(args.output, args.cli, args.expected_sha256, args.expected_version)
    except (ProbeError, OSError, subprocess.SubprocessError) as exc:
        print(f"probe refused/failed: {exc}", file=sys.stderr); return 2
    print(json.dumps({k: result.get(k) for k in ("return_code", "request_count", "run_status", "cli_version_pin", "cli_sha256_pin")}, indent=2))
    return 0


if __name__ == "__main__": raise SystemExit(main())
