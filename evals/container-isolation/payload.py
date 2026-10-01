#!/usr/bin/env python3
"""Small in-container proof payload; invoked inline, not mounted into the container."""
from __future__ import annotations

import errno
import http.server
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import threading

FIXTURE = b"cub-scout-container-isolation-fixture-v1\n"
HTTP_REPLY = b"loopback-only-ok\n"
HOST_MARKER = os.environ.get("SCOUT_SYNTHETIC_HOST_MARKER", "/scout-host-marker-unmounted")


def main() -> int:
    assertions: dict[str, bool] = {}
    try:
        assertions["fixture_read_exact"] = Path("/fixture/input.txt").read_bytes() == FIXTURE
    except OSError:
        assertions["fixture_read_exact"] = False
    try:
        Path("/fixture/input.txt").write_bytes(b"mutation\n")
        assertions["fixture_write_denied"] = False
    except OSError as error:
        assertions["fixture_write_denied"] = error.errno in (errno.EROFS, errno.EACCES, errno.EPERM)
    try:
        Path("/scout-outside-tmp-write").write_text("must-not-write")
        assertions["outside_tmp_write_denied"] = False
    except OSError as error:
        assertions["outside_tmp_write_denied"] = error.errno in (errno.EROFS, errno.EACCES, errno.EPERM)
    tmp_path = Path("/tmp/scout-isolation-probe")
    try:
        tmp_path.write_text("writable tmpfs")
        assertions["tmp_write_allowed"] = tmp_path.read_text() == "writable tmpfs"
        tmp_path.unlink(missing_ok=True)
    except OSError:
        assertions["tmp_write_allowed"] = False
    assertions["synthetic_host_marker_unavailable"] = not Path(HOST_MARKER).exists()

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):  # noqa: N802 - stdlib callback API
            self.send_response(200)
            self.send_header("Content-Length", str(len(HTTP_REPLY)))
            self.end_headers()
            self.wfile.write(HTTP_REPLY)

        def log_message(self, *_args):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    server.timeout = 3
    worker = threading.Thread(target=server.handle_request, daemon=True)
    worker.start()
    try:
        with socket.create_connection(server.server_address, timeout=2) as client:
            client.sendall(b"GET /proof HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n")
            reply = bytearray()
            while len(reply) < 4096:
                chunk = client.recv(4096)
                if not chunk:
                    break
                reply.extend(chunk)
        assertions["loopback_http_ok"] = b"\r\n\r\n" + HTTP_REPLY in bytes(reply)
    except OSError:
        assertions["loopback_http_ok"] = False
    finally:
        server.server_close()
        worker.join(timeout=4)

    child = (
        "import errno,socket,sys; s=socket.socket(); s.settimeout(2); "
        "\ntry: s.connect(('203.0.113.1', 9)); sys.exit(13) "
        "\nexcept OSError as e: sys.exit(0 if e.errno in (errno.ENETUNREACH, errno.EHOSTUNREACH, errno.EADDRNOTAVAIL) else 14) "
        "\nfinally: s.close()"
    )
    try:
        child_result = subprocess.run([sys.executable, "-c", child], stdin=subprocess.DEVNULL,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                      timeout=4, check=False)
        assertions["non_loopback_child_network_denied"] = child_result.returncode == 0
        child_spawned = True
    except (OSError, subprocess.TimeoutExpired):
        assertions["non_loopback_child_network_denied"] = False
        child_spawned = False

    expected = {
        "fixture_read_exact", "fixture_write_denied", "outside_tmp_write_denied",
        "tmp_write_allowed", "synthetic_host_marker_unavailable", "loopback_http_ok",
        "non_loopback_child_network_denied",
    }
    report = {"schema": "container-isolation-payload.v1", "assertions": assertions,
              "childProcessAttempted": child_spawned}
    print(json.dumps(report, sort_keys=True, separators=(",", ":")))
    return 0 if set(assertions) == expected and all(value is True for value in assertions.values()) and child_spawned else 1


if __name__ == "__main__":
    raise SystemExit(main())
