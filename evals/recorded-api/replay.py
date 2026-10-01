#!/usr/bin/env python3
"""Serve a bounded, source-pinned PRE-01 Kubernetes API response replay.

This is a local fixture transport for a future isolated runtime check. It does
not contact a cluster, load kubeconfig, forward requests, or claim TLS/API-server
equivalence.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import select
import signal
import socket
import socketserver
import sys
import tempfile
import threading
import time
from datetime import datetime, timezone
from http import HTTPStatus

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
REPORT = REPO / "evals/reports/2026-10-01-pre01-crd.json"
FIXTURES = REPO / "evals/pre01-crd/fixtures"
REPORT_SHA256 = "5585ecd0298e2b6f4201a638cdc0aeab9cdeb0dfab38d3b52e090cc3cb8f5c0a"
SCOPE_SHA256 = "6d39e83612d52f5124bad2dd63b27b30b94cce492eb70223f759248de71bfec4"
SOURCE_REVISION = "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0"
CAPTURE_HELPER_REVISION = "682160f4cf536a8a16e48ebf237976609d3d1c5f"
SCOPE_SCHEMA = "pre01-crd-capture-scope.v1"

MAX_REPORT_BYTES = 128 * 1024
MAX_BODY_BYTES = 128 * 1024
MAX_RESPONSE_BYTES = 256 * 1024
MAX_REQUESTS = 32
MAX_WALL_SECONDS = 120
REQUEST_READ_TIMEOUT = 2.0
MAX_REQUEST_LINE = 4096
MAX_HEADER_LINE = 4096
MAX_HEADER_BYTES = 16 * 1024
MAX_HEADER_COUNT = 32
MAX_REQUEST_BODY = 64 * 1024
ERROR_BODIES = {
    400: b'{"error":"malformed-request"}',
    405: b'{"error":"method-not-allowed"}',
    408: b'{"error":"request-read-timeout"}',
    413: b'{"error":"request-body-too-large"}',
    429: b'{"error":"replay-request-limit-exceeded"}',
    503: b'{"error":"replay-unavailable","reason":"no captured response for exact request"}',
}


class ReplayError(RuntimeError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def _strict_json(data: bytes, label: str):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ReplayError(f"{label} has duplicate JSON keys")
            result[key] = value
        return result
    try:
        return json.loads(data.decode("utf-8", "strict"), object_pairs_hook=pairs,
                          parse_constant=lambda _: (_ for _ in ()).throw(ReplayError(f"{label} has a non-finite number")))
    except ReplayError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ReplayError(f"{label} is not valid UTF-8 JSON") from None


def _read_pinned_file(path: Path, expected_sha: str, limit: int, label: str) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise ReplayError(f"{label} must be a regular non-symlink file")
    try:
        size = path.stat().st_size
        if size > limit:
            raise ReplayError(f"{label} exceeds the byte limit")
        data = path.read_bytes()
    except OSError:
        raise ReplayError(f"{label} could not be read") from None
    if len(data) != size or sha256(data) != expected_sha:
        raise ReplayError(f"{label} does not match its source pin")
    return data


def collapse_observations(observations: list[dict], fixture_dir: Path,
                          *, max_body_bytes: int = MAX_BODY_BYTES) -> list[dict]:
    """Collapse only byte-identical repeated method/path observations."""
    collapsed: dict[tuple[str, str], dict] = {}
    for index, row in enumerate(observations):
        if not isinstance(row, dict):
            raise ReplayError("API observation row is malformed")
        method, path, raw_file, phase = (row.get("method"), row.get("path"),
                                        row.get("rawFile"), row.get("phase"))
        status = row.get("httpStatus")
        if method != "GET" or not isinstance(path, str) or not path.startswith("/") or "#" in path:
            raise ReplayError("API observation method or raw path is unsupported")
        if not isinstance(raw_file, str) or Path(raw_file).name != raw_file or raw_file in ("", ".", ".."):
            raise ReplayError("API observation fixture name is unsafe")
        if type(status) is not int or status not in (200, 404):
            raise ReplayError("API observation status is unsupported")
        if not isinstance(phase, str) or not isinstance(row.get("startedAt"), str) or not isinstance(row.get("endedAt"), str):
            raise ReplayError("API observation source timing is missing")
        expected_hash, expected_size = row.get("rawSha256"), row.get("rawBytes")
        if not isinstance(expected_hash, str) or not re.fullmatch(r"[0-9a-f]{64}", expected_hash):
            raise ReplayError("API observation body hash is malformed")
        if type(expected_size) is not int or expected_size < 0:
            raise ReplayError("API observation body size is invalid")
        if expected_size > max_body_bytes:
            raise ReplayError("API observation body size exceeds byte limit")
        body = _read_pinned_file(fixture_dir / raw_file, expected_hash, max_body_bytes,
                                 f"API fixture {raw_file}")
        if len(body) != expected_size or len(body) > max_body_bytes:
            raise ReplayError("API fixture size differs from source observation")
        key = (method, path)
        source_ref = {"row": index, "phase": phase, "rawFile": raw_file,
                      "startedAt": row["startedAt"], "endedAt": row["endedAt"],
                      "status": status, "bodyBytes": len(body), "bodySha256": expected_hash}
        previous = collapsed.get(key)
        if previous:
            if previous["status"] != status or previous["body"] != body:
                raise ReplayError("conflicting duplicate API observations cannot be replayed as one static route")
            previous["sourceRows"].append(source_ref)
            continue
        collapsed[key] = {"method": method, "path": path, "status": status, "body": body,
                          "bodySha256": expected_hash, "sourceRows": [source_ref]}
    return list(collapsed.values())


def load_routes(phase: str, *, report_path: Path = REPORT,
                fixture_dir: Path = FIXTURES) -> tuple[list[dict], dict]:
    if phase not in ("absent", "present"):
        raise ReplayError("phase must be explicitly absent or present")
    if fixture_dir.is_symlink() or not fixture_dir.is_dir():
        raise ReplayError("PRE-01 fixture directory must be a regular directory")
    report_bytes = _read_pinned_file(report_path, REPORT_SHA256, MAX_REPORT_BYTES, "PRE-01 report")
    scope_bytes = _read_pinned_file(fixture_dir / "capture-scope.json", SCOPE_SHA256,
                                    MAX_REPORT_BYTES, "PRE-01 capture scope")
    report = _strict_json(report_bytes, "PRE-01 report")
    scope = _strict_json(scope_bytes, "PRE-01 capture scope")
    capture = report.get("capture") if isinstance(report, dict) else None
    observations = capture.get("apiObservations") if isinstance(capture, dict) else None
    if (report.get("schema") != "pre01-crd-proof.v1" or report.get("case") != "PRE-01"
            or capture.get("sourceRevision") != SOURCE_REVISION
            or capture.get("captureSourceRevision") != CAPTURE_HELPER_REVISION
            or capture.get("atomicSnapshot") is not False or not isinstance(observations, list)
            or len(observations) != 9):
        raise ReplayError("PRE-01 report identity, scope, or observation set changed")
    if (scope.get("schema") != SCOPE_SCHEMA
            or scope.get("capture", {}).get("sourceRevision") != SOURCE_REVISION
            or scope.get("capture", {}).get("atomicSnapshot") is not False):
        raise ReplayError("PRE-01 source scope identity changed")

    # A fixed replay phase is a static view. The present-before-apply checkpoint
    # is intentionally omitted because it conflicts with the later 200 at that path.
    selected = [row for row in observations
                if row.get("phase") in ({"absent", "absent-after-apply"} if phase == "absent" else {"present"})]
    if not selected:
        raise ReplayError("selected PRE-01 phase has no captured routes")
    routes = collapse_observations(selected, fixture_dir)
    if any(len(route["body"]) > MAX_RESPONSE_BYTES for route in routes):
        raise ReplayError("captured API response exceeds response limit")
    provenance = {
        "schema": "recorded-api-replay-ready.v1",
        "case": "PRE-01",
        "phase": phase,
        "sourceRevision": SOURCE_REVISION,
        "captureHelperRevision": CAPTURE_HELPER_REVISION,
        "reportPath": "evals/reports/2026-10-01-pre01-crd.json",
        "reportSha256": sha256(report_bytes),
        "scopeSha256": sha256(scope_bytes),
        "captureStartedAt": capture.get("startedAt"),
        "captureEndedAt": capture.get("endedAt"),
        "atomicSnapshot": False,
        "scopeNote": "source capture is sequential, non-atomic, historical; no current-state claim",
        "routes": [{k: v for k, v in route.items() if k != "body"} for route in routes],
        "excludedPresentCheckpoint": ("present-before-apply object 404 excluded; this static final-present phase uses the later captured 200"
                                       if phase == "present" else None),
        "transportHeaders": "Content-Type and Content-Length are generated by replay, not retained source response headers",
    }
    return routes, provenance


def _json_bytes(value: dict) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode("utf-8")


def create_output_dir(path: Path, *, repo: Path = REPO) -> Path:
    requested = path.expanduser().absolute()
    if requested.is_symlink() or requested.exists() or ".." in path.parts:
        raise ReplayError("output directory must be a fresh non-symlink path")
    try:
        parent = requested.parent.resolve(strict=True)
        home = Path.home().resolve(strict=True)
        repository = repo.resolve(strict=True)
    except OSError:
        raise ReplayError("output parent is unavailable") from None
    for denied in (home, repository):
        try:
            parent.relative_to(denied)
            raise ReplayError("output must be outside the home directory and source checkout")
        except ValueError:
            pass
    roots = {Path(tempfile.gettempdir()).resolve(), Path("/tmp").resolve(), Path("/var/tmp").resolve()}
    if not any(parent == root or root in parent.parents for root in roots):
        raise ReplayError("output directory must be under a temporary directory")
    try:
        requested.mkdir(mode=0o700)
        os.chmod(requested, 0o700)
    except OSError:
        raise ReplayError("could not create private output directory") from None
    if requested.is_symlink() or requested.stat().st_mode & 0o777 != 0o700:
        raise ReplayError("output directory is not private")
    return requested


def write_new_private(path: Path, payload: bytes) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    try:
        fd = os.open(path, flags, 0o600)
        with os.fdopen(fd, "wb") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
    except OSError:
        raise ReplayError(f"could not write fresh private output {path.name}") from None


class ReplayServer(socketserver.TCPServer):
    allow_reuse_address = False
    request_queue_size = 16

    def __init__(self, routes: list[dict], *, host: str = "127.0.0.1", port: int = 0,
                 max_requests: int = MAX_REQUESTS, read_timeout: float = REQUEST_READ_TIMEOUT,
                 max_request_body: int = MAX_REQUEST_BODY):
        if host != "127.0.0.1":
            raise ReplayError("replay listener must bind IPv4 loopback only")
        if type(max_requests) is not int or not 1 <= max_requests <= MAX_REQUESTS:
            raise ReplayError(f"max requests must be between 1 and {MAX_REQUESTS}")
        if type(read_timeout) not in (int, float) or not 0 < read_timeout <= 10:
            raise ReplayError("request read timeout is outside the allowed range")
        if type(max_request_body) is not int or not 0 <= max_request_body <= MAX_REQUEST_BODY:
            raise ReplayError("request body limit is outside the allowed range")
        self.routes = {(r["method"], r["path"]): r for r in routes}
        self.max_requests = max_requests
        self.read_timeout = float(read_timeout)
        self.max_request_body = max_request_body
        self.records: list[dict] = []
        self.requests_seen = 0
        self.connections_seen = 0
        self.deadline = float("inf")
        self.coverage_failure = False
        self.stop_event = threading.Event()
        self.lock = threading.Lock()
        super().__init__((host, port), ReplayRequestHandler, bind_and_activate=True)
        if self.server_address[0] != "127.0.0.1":
            self.server_close()
            raise ReplayError("listener did not bind exact IPv4 loopback")

    def get_request(self):
        request, address = super().get_request()
        with self.lock:
            self.connections_seen += 1
        return request, address

    def record_request(self) -> tuple[int, bool]:
        with self.lock:
            self.requests_seen += 1
            return self.requests_seen, self.requests_seen > self.max_requests

    def append_record(self, record: dict) -> None:
        with self.lock:
            self.records.append(record)

    def serve_bounded(self, duration: float = 60.0) -> str:
        if type(duration) not in (int, float) or not 0 < duration <= MAX_WALL_SECONDS:
            raise ReplayError(f"wall duration must be between 0 and {MAX_WALL_SECONDS} seconds")
        deadline = time.monotonic() + duration
        self.deadline = deadline
        status = "stopped"
        try:
            while not self.stop_event.is_set():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    status = "wall_limit"
                    break
                if self.requests_seen > self.max_requests:
                    status = "request_limit_exceeded"
                    break
                if self.requests_seen == self.max_requests:
                    status = "request_limit_reached"
                    break
                self.timeout = min(0.1, remaining)
                self.handle_request()
                if time.monotonic() >= deadline:
                    status = "wall_limit"
                    break
            else:
                status = "request_rejected" if self.coverage_failure else "stopped"
        except KeyboardInterrupt:
            status = "interrupted"
        finally:
            self.server_close()
        return status


class _RequestFailure(Exception):
    def __init__(self, status: int, reason: str):
        self.status = status
        self.reason = reason


class _DeadlineReader:
    """Unbuffered bounded line reader with one absolute request deadline."""
    def __init__(self, connection: socket.socket, server: ReplayServer, deadline: float):
        self.connection = connection
        self.server = server
        self.deadline = deadline
        self.buffer = bytearray()

    def readline(self, limit: int) -> bytes:
        while True:
            newline = self.buffer.find(b"\n")
            if newline >= 0:
                if newline + 1 > limit:
                    raise _RequestFailure(400, "line-limit")
                line = bytes(self.buffer[:newline + 1])
                del self.buffer[:newline + 1]
                if not line.endswith(b"\r\n"):
                    raise _RequestFailure(400, "line-terminator")
                return line
            if len(self.buffer) > limit:
                raise _RequestFailure(400, "line-limit")
            if self.server.stop_event.is_set():
                raise _RequestFailure(408, "server-stop-requested")
            remaining = min(self.deadline, self.server.deadline) - time.monotonic()
            if remaining <= 0:
                reason = "overall-wall-limit" if time.monotonic() >= self.server.deadline else "request-read-timeout"
                raise _RequestFailure(408, reason)
            try:
                readable, _, _ = select.select([self.connection], [], [], min(remaining, 0.05))
                if not readable:
                    continue
                chunk = self.connection.recv(min(4096, limit + 1 - len(self.buffer)))
            except (OSError, ValueError):
                raise _RequestFailure(408, "request-read-timeout") from None
            if not chunk:
                if self.buffer:
                    raise _RequestFailure(400, "line-terminator")
                return b""
            self.buffer.extend(chunk)


class ReplayRequestHandler(socketserver.StreamRequestHandler):
    def setup(self):
        self.connection = self.request
        self.rfile = self.wfile = None

    def finish(self):
        return

    def _line(self, limit: int) -> bytes:
        return self.reader.readline(limit)

    def _parse(self) -> tuple[str, str, dict[str, list[str]]]:
        request_line = self._line(MAX_REQUEST_LINE)
        if not request_line:
            raise _RequestFailure(400, "empty-request")
        parts = request_line[:-2].split(b" ")
        if len(parts) != 3 or any(not x for x in parts):
            raise _RequestFailure(400, "request-line")
        try:
            method, target, version = (part.decode("ascii", "strict") for part in parts)
        except UnicodeDecodeError:
            raise _RequestFailure(400, "request-line-encoding") from None
        if not re.fullmatch(r"[!#$%&'*+.^_`|~0-9A-Za-z-]+", method):
            raise _RequestFailure(400, "method-token")
        if version not in ("HTTP/1.0", "HTTP/1.1"):
            raise _RequestFailure(400, "http-version")
        if not target.startswith("/") or "#" in target or any(ord(c) < 33 or ord(c) > 126 for c in target):
            raise _RequestFailure(400, "request-target")
        self._partial_method, self._partial_target = method, target

        headers: dict[str, list[str]] = {}
        used, count = 0, 0
        while True:
            line = self._line(MAX_HEADER_LINE)
            used += len(line)
            if used > MAX_HEADER_BYTES:
                raise _RequestFailure(400, "header-byte-limit")
            if line == b"\r\n":
                break
            if not line or line[:1] in (b" ", b"\t") or b":" not in line:
                raise _RequestFailure(400, "header-syntax")
            count += 1
            if count > MAX_HEADER_COUNT:
                raise _RequestFailure(400, "header-count-limit")
            name, value = line[:-2].split(b":", 1)
            if not re.fullmatch(rb"[!#$%&'*+.^_`|~0-9A-Za-z-]+", name):
                raise _RequestFailure(400, "header-name")
            try:
                key = name.decode("ascii").lower()
                val = value.strip().decode("latin-1")
            except UnicodeDecodeError:
                raise _RequestFailure(400, "header-encoding") from None
            headers.setdefault(key, []).append(val)
        if len(headers.get("content-length", [])) > 1 or headers.get("transfer-encoding"):
            raise _RequestFailure(400, "ambiguous-body-framing")
        lengths = headers.get("content-length", [])
        if lengths:
            if not re.fullmatch(r"[0-9]{1,10}", lengths[0]):
                raise _RequestFailure(400, "content-length")
            length = int(lengths[0])
            if length > self.server.max_request_body:
                raise _RequestFailure(413, "body-limit")
            if length != 0:
                raise _RequestFailure(400, "request-body-not-accepted")
        return method, target, headers

    def _respond(self, status: int, body: bytes, *, content_type: str = "application/json") -> None:
        try:
            reason = HTTPStatus(status).phrase
        except ValueError:
            reason = "Replay Error"
        # Content-Type and Content-Length are replay-generated transport fields,
        # not claims about headers on the original Kubernetes response.
        payload = (f"HTTP/1.1 {status} {reason}\r\n"
                   f"Content-Type: {content_type}\r\n"
                   f"Content-Length: {len(body)}\r\nConnection: close\r\n\r\n").encode("ascii") + body
        offset = 0
        while offset < len(payload):
            if self.server.stop_event.is_set():
                raise _RequestFailure(408, "server-stop-requested")
            remaining = min(self.reader.deadline, self.server.deadline) - time.monotonic()
            if remaining <= 0:
                raise _RequestFailure(408, "response-write-deadline")
            try:
                _, writable, _ = select.select([], [self.connection], [], min(remaining, 0.05))
                if writable:
                    offset += self.connection.send(payload[offset:])
            except (OSError, ValueError):
                raise _RequestFailure(408, "response-write-failed") from None

    def handle(self) -> None:
        remaining = self.server.deadline - time.monotonic()
        if remaining <= 0:
            self.server.stop_event.set()
            self.server.append_record({"requestNumber": None, "method": None, "status": 408,
                                       "reason": "overall-wall-limit-before-request", "path": None,
                                       "responseBodyBytes": len(ERROR_BODIES[408]),
                                       "responseBodySha256": sha256(ERROR_BODIES[408]),
                                       "captured": False, "sourceRows": []})
            return
        self.reader = _DeadlineReader(self.connection, self.server,
                                      min(time.monotonic() + self.server.read_timeout, self.server.deadline))
        request_number, over_limit = self.server.record_request()
        method = target = None
        route = None
        status = 400
        body = ERROR_BODIES[400]
        reason = "malformed-request"
        try:
            method, target, _headers = self._parse()
            if over_limit:
                status, body, reason = 429, ERROR_BODIES[429], "request-limit-exceeded"
            elif method != "GET":
                status, body, reason = 405, ERROR_BODIES[405], "method-not-allowed"
            else:
                route = self.server.routes.get((method, target))
                if route is None:
                    status, body, reason = 503, ERROR_BODIES[503], "replay-unavailable"
                else:
                    status, body, reason = route["status"], route["body"], "captured-response"
        except _RequestFailure as err:
            method = getattr(self, "_partial_method", method)
            target = getattr(self, "_partial_target", target)
            status, body, reason = err.status, ERROR_BODIES[err.status], err.reason
        except (OSError, socket.timeout):
            method = getattr(self, "_partial_method", method)
            target = getattr(self, "_partial_target", target)
            status, body, reason = 408, ERROR_BODIES[408], "request-read-timeout"

        is_captured = route is not None and status == route["status"]
        content_type = "application/json" if is_captured and _looks_json(body) else "text/plain; charset=utf-8" if is_captured else "application/json"
        try:
            self._respond(status, body, content_type=content_type)
        except (OSError, socket.timeout, _RequestFailure) as err:
            write_reason = err.reason if isinstance(err, _RequestFailure) else "response-write-failed"
            reason = reason + ";" + write_reason
        record = {"requestNumber": request_number,
                  "method": method if isinstance(method, str) else None,
                  "status": status, "reason": reason,
                  "requestTargetSha256": sha256(target.encode("ascii")) if isinstance(target, str) else None,
                  "requestTargetBytes": len(target.encode("ascii")) if isinstance(target, str) else None,
                  "responseBodyBytes": len(body), "responseBodySha256": sha256(body),
                  "captured": bool(is_captured), "sourceRows": route["sourceRows"] if is_captured else [],
                  "generatedContentType": content_type}
        if is_captured:
            record["path"] = route["path"]
        self.server.append_record(record)
        if over_limit or not is_captured or reason != "captured-response":
            self.server.coverage_failure = True
            self.server.stop_event.set()


def _looks_json(body: bytes) -> bool:
    try:
        _strict_json(body, "captured response")
        return True
    except ReplayError:
        return False


def _trace(provenance: dict, server: ReplayServer | None, status: str,
           started_at: str, started_mono: float, error: str | None = None) -> dict:
    records = list(server.records) if server else []
    failed = (any(not record["captured"] or record["reason"] != "captured-response" for record in records)
              or bool(server and server.coverage_failure)
              or status in ("wall_limit", "request_limit_reached", "request_limit_exceeded", "request_rejected", "interrupted", "startup_failed"))
    data = {"schema": "recorded-api-replay-trace.v1", "case": "PRE-01",
            "phase": provenance["phase"], "sourceRevision": SOURCE_REVISION,
            "reportSha256": provenance["reportSha256"], "scopeSha256": provenance["scopeSha256"],
            "startedAt": started_at, "endedAt": utc_now(),
            "elapsedSeconds": round(time.monotonic() - started_mono, 6),
            "status": status, "coverageFailure": failed or status not in ("request_limit_reached", "stopped"),
            "coverageComplete": False,
            "coverageAssessment": "NOT_ASSESSED_NO_DECLARED_EXTERNAL_COMMAND_ROUTE_SET",
            "connectionsAccepted": server.connections_seen if server else 0,
            "requestsObserved": server.requests_seen if server else 0,
            "maxRequests": server.max_requests if server else 0,
            "listenerCleanupConfirmed": server is not None and server.fileno() == -1,
            "events": records}
    if error:
        data["error"] = error[:240]
    return data


def _serve_cli(args) -> int:
    routes, ready = load_routes(args.phase)
    out = create_output_dir(Path(args.out_dir))
    ready["listener"] = {"host": "127.0.0.1", "port": None,
                         "transport": "plain local HTTP; not a Kubernetes TLS/API-server claim"}
    server = None
    started_at, began = utc_now(), time.monotonic()
    status, error = "startup_failed", None
    previous_handlers = {}
    trace_write_failed = False
    try:
        server = ReplayServer(routes, port=args.port, max_requests=args.max_requests)
        ready["listener"]["port"] = server.server_address[1]
        ready["startedAt"] = started_at
        ready["wallLimitSeconds"] = args.wall_seconds
        ready["requestLimit"] = args.max_requests
        write_new_private(out / "ready.json", _json_bytes(ready))
        print(json.dumps(ready, sort_keys=True), flush=True)

        def stop_signal(signum, _frame):
            server.stop_event.set()
        for sig in (signal.SIGINT, signal.SIGTERM):
            previous_handlers[sig] = signal.signal(sig, stop_signal)
        status = server.serve_bounded(args.wall_seconds)
        if status == "wall_limit":
            error = "overall wall limit reached"
    except ReplayError as exc:
        error = str(exc)
    except OSError as exc:
        error = f"listener or output error: {type(exc).__name__}"
    finally:
        for sig, handler in previous_handlers.items():
            signal.signal(sig, handler)
        if server is not None and server.fileno() != -1:
            server.server_close()
        if server is not None and server.fileno() == -1:
            # The HTTPServer fileno becomes invalid after server_close; this is
            # the bounded owned listener cleanup fact, not a process-tree claim.
            pass
        trace = _trace(ready, server, status, started_at, began, error)
        try:
            write_new_private(out / "trace.json", _json_bytes(trace))
        except ReplayError as write_err:
            print(str(write_err), file=sys.stderr)
            trace_write_failed = True
    if trace_write_failed:
        return 1
    return 1 if error or status in ("wall_limit", "request_limit_reached", "request_limit_exceeded", "request_rejected", "startup_failed") else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--phase", choices=("absent", "present"), required=True,
                        help="explicit immutable historical view; no automatic phase progression")
    parser.add_argument("--out-dir", required=True, help="new private directory under a temporary root")
    parser.add_argument("--port", type=int, default=0, help="loopback port; 0 asks the OS for an ephemeral port")
    parser.add_argument("--max-requests", type=int, default=MAX_REQUESTS)
    parser.add_argument("--wall-seconds", type=int, default=60)
    args = parser.parse_args(argv)
    if not 0 <= args.port <= 65535 or not 1 <= args.wall_seconds <= MAX_WALL_SECONDS:
        parser.error("port or wall duration is outside the allowed range")
    try:
        return _serve_cli(args)
    except ReplayError as exc:
        print(f"recorded API replay refused: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
