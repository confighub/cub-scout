"""Pure local tests for the source-pinned PRE-01 API replay."""
import hashlib
import http.client
import importlib.util
import json
from pathlib import Path
import shutil
import socket
import tempfile
import threading
import time
import unittest
from types import SimpleNamespace

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("recorded_api_replay", HERE / "replay.py")
replay = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(replay)


class ReplayTests(unittest.TestCase):
    def setUp(self):
        self.absent, self.absent_ready = replay.load_routes("absent")
        self.present, self.present_ready = replay.load_routes("present")

    def start(self, routes, max_requests=12, read_timeout=0.5, duration=4):
        server = replay.ReplayServer(routes, max_requests=max_requests, read_timeout=read_timeout)
        result = {}
        thread = threading.Thread(target=lambda: result.setdefault("status", server.serve_bounded(duration)), daemon=True)
        thread.start()
        return server, thread, result

    def request(self, port, method, path, body=None, headers=None, timeout=2):
        conn = http.client.HTTPConnection("127.0.0.1", port, timeout=timeout)
        conn.request(method, path, body=body, headers=headers or {})
        response = conn.getresponse()
        data = response.read()
        result = response.status, data, dict(response.getheaders())
        conn.close()
        return result

    def stop(self, server, thread, result):
        server.stop_event.set()
        thread.join(timeout=2)
        self.assertFalse(thread.is_alive(), "owned replay listener thread did not stop")
        self.assertEqual(server.fileno(), -1, "listener socket was not closed")
        return result["status"]

    def test_source_report_scope_and_every_selected_body_are_pinned(self):
        self.assertEqual(self.absent_ready["reportSha256"], replay.REPORT_SHA256)
        self.assertEqual(self.absent_ready["scopeSha256"], replay.SCOPE_SHA256)
        self.assertEqual(self.absent_ready["sourceRevision"], replay.SOURCE_REVISION)
        self.assertFalse(self.absent_ready["atomicSnapshot"])
        self.assertEqual(len(self.absent), 3)
        self.assertEqual(len(self.present), 3)
        for routes in (self.absent, self.present):
            for route in routes:
                self.assertEqual(hashlib.sha256(route["body"]).hexdigest(), route["bodySha256"])
                self.assertGreaterEqual(len(route["sourceRows"]), 1)
                for source_row in route["sourceRows"]:
                    self.assertTrue(source_row["startedAt"])
                    self.assertTrue(source_row["endedAt"])

    def test_present_phase_excludes_conflicting_pre_apply_checkpoint(self):
        self.assertIn("present-before-apply", self.present_ready["excludedPresentCheckpoint"])
        route = next(route for route in self.present if route["path"].endswith("servicemonitors/kube-prometheus-stack-kube-state-metrics"))
        self.assertEqual(route["status"], 200)
        self.assertEqual(route["sourceRows"][0]["phase"], "present")

    def test_loopback_returns_exact_absent_and_present_fixtures(self):
        for routes, path, status in (
            (self.absent, "/apis/monitoring.coreos.com/v1", 404),
            (self.present, "/apis/monitoring.coreos.com/v1", 200),
        ):
            with self.subTest(status=status):
                server, thread, result = self.start(routes)
                got_status, body, headers = self.request(server.server_address[1], "GET", path)
                expected = next(item for item in routes if item["path"] == path)
                self.assertEqual(got_status, status)
                self.assertEqual(body, expected["body"])
                self.assertEqual(headers["Connection"], "close")
                self.assertEqual(self.stop(server, thread, result), "stopped")
                record = server.records[0]
                self.assertTrue(record["captured"])
                self.assertEqual(record["responseBodySha256"], expected["bodySha256"])
                self.assertIn("sourceRows", record)

    def test_known_typed_404_is_exact_but_unknown_route_is_unavailable(self):
        path = "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/servicemonitors.monitoring.coreos.com"
        server, thread, result = self.start(self.absent)
        status, body, _ = self.request(server.server_address[1], "GET", path)
        expected = next(item for item in self.absent if item["path"] == path)
        self.assertEqual((status, body), (404, expected["body"]))
        self.assertEqual(self.request(server.server_address[1], "GET", path + "?limit=1")[0], 503)
        self.assertEqual(server.records[-1]["reason"], "replay-unavailable")
        self.assertFalse(server.records[-1]["captured"])
        self.stop(server, thread, result)

    def test_methods_body_and_query_variants_fail_closed_without_headers_in_trace(self):
        for method, path, expected, reason in (
            ("POST", "/apis/monitoring.coreos.com/v1", 405, "method-not-allowed"),
            ("GET", "/apis/monitoring.coreos.com/v1?x=y", 503, "replay-unavailable"),
        ):
            server, thread, result = self.start(self.absent)
            self.assertEqual(self.request(server.server_address[1], method, path)[0], expected)
            self.assertEqual(self.stop(server, thread, result), "request_rejected")
            self.assertEqual(server.records[0]["reason"], reason)
            self.assertFalse(server.records[0]["captured"])
        server, thread, result = self.start(self.absent)
        conn = http.client.HTTPConnection("127.0.0.1", server.server_address[1], timeout=2)
        path = self.absent[0]["path"]
        conn.request("GET", path, headers={"Authorization": "Bearer fake-never-log"})
        response = conn.getresponse()
        response.read()
        conn.close()
        self.stop(server, thread, result)
        encoded = json.dumps(server.records)
        self.assertNotIn("fake-never-log", encoded)
        self.assertFalse(any("headers" in item for item in server.records))

    def test_concurrent_requests_are_bounded_and_listener_cleanup_is_owned(self):
        server, thread, result = self.start(self.present, max_requests=4)
        paths = [route["path"] for route in self.present]
        errors = []
        def fetch(path):
            try:
                self.request(server.server_address[1], "GET", path)
            except Exception as exc:  # surfaced after join
                errors.append(exc)
        clients = [threading.Thread(target=fetch, args=(paths[i % len(paths)],)) for i in range(4)]
        for client in clients:
            client.start()
        for client in clients:
            client.join(timeout=3)
        self.assertTrue(all(not client.is_alive() for client in clients))
        self.assertEqual(errors, [])
        thread.join(timeout=2)
        self.assertFalse(thread.is_alive())
        self.assertEqual(result["status"], "request_limit_reached")
        self.assertEqual(server.requests_seen, 4)
        self.assertEqual(len(server.records), 4)
        self.assertEqual(server.fileno(), -1)

    def test_partial_request_times_out_and_is_recorded_without_raw_target(self):
        server, thread, result = self.start(self.absent, read_timeout=0.15)
        conn = socket.create_connection(server.server_address, timeout=1)
        conn.sendall(b"GET /incomplete HTTP/1.1\r\n")
        response = conn.recv(200)
        conn.close()
        self.assertEqual(response, b"")  # deadline has elapsed, so no late response is written
        self.stop(server, thread, result)
        event = server.records[0]
        self.assertTrue(event["reason"].startswith("request-read-timeout"))
        self.assertNotIn("path", event)
        self.assertEqual(event["requestTargetSha256"], hashlib.sha256(b"/incomplete").hexdigest())

    def test_slow_trickle_cannot_extend_absolute_request_deadline(self):
        server, thread, result = self.start(self.absent, read_timeout=0.22, duration=3)
        conn = socket.create_connection(server.server_address, timeout=1)
        conn.settimeout(1)
        began = time.monotonic()
        conn.sendall(b"GET /incomplete HTTP/1.1\r\nX-Slow: ")
        try:
            while time.monotonic() - began < 0.7:
                conn.sendall(b"a")
                time.sleep(0.035)  # every inter-byte gap is below read_timeout
        except OSError:
            pass
        elapsed = time.monotonic() - began
        conn.close()
        thread.join(timeout=1)
        self.assertFalse(thread.is_alive(), "slow-trickle handler outlived its absolute deadline")
        self.assertLess(elapsed, 0.7)
        self.assertEqual(server.fileno(), -1)
        self.assertTrue(server.records)
        self.assertFalse(server.records[0]["captured"])
        self.assertIn(server.records[0]["reason"].split(";")[0], ("request-read-timeout", "response-write-failed"))

    def test_wall_deadline_applies_during_slow_header_and_stop_interrupts_read(self):
        server, thread, result = self.start(self.absent, read_timeout=2, duration=0.22)
        conn = socket.create_connection(server.server_address, timeout=1)
        try:
            conn.sendall(b"GET /incomplete HTTP/1.1\r\nX-Slow: a")
            began = time.monotonic()
            thread.join(timeout=1)
            elapsed = time.monotonic() - began
            self.assertFalse(thread.is_alive(), "wall deadline did not interrupt active read")
            self.assertLess(elapsed, 0.7)
            self.assertEqual(result["status"], "wall_limit")
            self.assertEqual(server.fileno(), -1)
        finally:
            conn.close()

        server, thread, result = self.start(self.absent, read_timeout=3, duration=3)
        conn = socket.create_connection(server.server_address, timeout=1)
        conn.sendall(b"GET /incomplete HTTP/1.1\r\nX-Wait: ")
        time.sleep(0.04)
        began = time.monotonic()
        server.stop_event.set()
        thread.join(timeout=0.5)
        elapsed = time.monotonic() - began
        self.assertFalse(thread.is_alive(), "stop request left an owned handler running")
        self.assertLess(elapsed, 0.3)
        self.assertEqual(server.fileno(), -1)
        conn.close()

    def test_wall_limit_and_rejected_request_are_not_reported_as_complete(self):
        server, thread, result = self.start(self.absent, duration=0.15)
        thread.join(timeout=2)
        self.assertFalse(thread.is_alive())
        self.assertEqual(result["status"], "wall_limit")
        trace = replay._trace(self.absent_ready, server, result["status"], replay.utc_now(), time.monotonic())
        self.assertTrue(trace["coverageFailure"])
        self.assertFalse(trace["coverageComplete"])
        self.assertTrue(trace["listenerCleanupConfirmed"])

        server, thread, result = self.start(self.absent)
        self.assertEqual(self.request(server.server_address[1], "GET", "/unrecorded")[0], 503)
        self.assertEqual(self.stop(server, thread, result), "request_rejected")
        trace = replay._trace(self.absent_ready, server, result["status"], replay.utc_now(), time.monotonic())
        self.assertTrue(trace["coverageFailure"])
        self.assertFalse(trace["coverageComplete"])

    def test_request_line_header_and_body_limits_are_enforced(self):
        attempts = [
            lambda port: self._raw(port, b"G" * (replay.MAX_REQUEST_LINE + 2) + b"\r\n"),
            lambda port: self.request(port, "POST", "/", body=b"", headers={"Content-Length": str(replay.MAX_REQUEST_BODY + 1)})[0],
            lambda port: self._raw(port, b"GET / HTTP/1.1\r\n" + b"X-Fill: " + b"a" * (replay.MAX_HEADER_BYTES + 1) + b"\r\n\r\n"),
        ]
        expected = (400, 413, 400)
        for send, want in zip(attempts, expected):
            server, thread, result = self.start(self.absent)
            self.assertEqual(send(server.server_address[1]), want)
            self.assertEqual(self.stop(server, thread, result), "request_rejected")
            self.assertEqual(len(server.records), 1)
            self.assertFalse(server.records[0]["captured"])

    def _raw(self, port, payload):
        conn = socket.create_connection(("127.0.0.1", port), timeout=1)
        conn.sendall(payload)
        response = conn.recv(200)
        conn.close()
        return int(response.split(b" ", 2)[1])

    def test_source_hash_mismatch_duplicate_conflict_and_file_bounds_reject(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            report_copy = tmp_path / "report.json"
            report_copy.write_bytes(replay.REPORT.read_bytes() + b" ")
            with self.assertRaisesRegex(replay.ReplayError, "source pin"):
                replay.load_routes("absent", report_path=report_copy)

            fixture_copy = tmp_path / "fixtures"
            shutil.copytree(replay.FIXTURES, fixture_copy)
            target = fixture_copy / "absent-crd.json"
            target.write_bytes(target.read_bytes() + b"x")
            with self.assertRaisesRegex(replay.ReplayError, "source pin"):
                replay.load_routes("absent", fixture_dir=fixture_copy)

            shutil.rmtree(fixture_copy)
            shutil.copytree(replay.FIXTURES, fixture_copy)
            rows = json.loads(replay.REPORT.read_bytes())["capture"]["apiObservations"][:1]
            duplicate = dict(rows[0], rawFile="other.body")
            (fixture_copy / "other.body").write_bytes(b"different")
            duplicate["rawBytes"] = len(b"different")
            duplicate["rawSha256"] = hashlib.sha256(b"different").hexdigest()
            with self.assertRaisesRegex(replay.ReplayError, "conflicting duplicate"):
                replay.collapse_observations(rows + [duplicate], fixture_copy)

            with self.assertRaisesRegex(replay.ReplayError, "byte limit"):
                replay.collapse_observations(rows, replay.FIXTURES, max_body_bytes=10)

    def test_output_directory_and_files_are_fresh_private_regular_outputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            out = replay.create_output_dir(parent / "fresh")
            self.assertEqual(out.stat().st_mode & 0o777, 0o700)
            target = out / "ready.json"
            replay.write_new_private(target, b"{}\n")
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(replay.ReplayError):
                replay.write_new_private(target, b"overwrite")
            with self.assertRaises(replay.ReplayError):
                replay.create_output_dir(out)
            link = parent / "linked"
            link.symlink_to(out, target_is_directory=True)
            with self.assertRaises(replay.ReplayError):
                replay.create_output_dir(link)

    def test_response_backpressure_cannot_exceed_absolute_deadline(self):
        sender, receiver = socket.socketpair()
        sender.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 1024)
        handler = object.__new__(replay.ReplayRequestHandler)
        handler.request = sender
        handler.setup()
        deadline = time.monotonic() + 0.1
        handler.server = SimpleNamespace(stop_event=threading.Event(), deadline=deadline)
        handler.reader = SimpleNamespace(deadline=deadline)
        # This owned timer also releases a regressed blocking writer, so the
        # negative test fails within a bound rather than leaking a thread.
        release = threading.Timer(0.6, receiver.close)
        release.start()
        started = time.monotonic()
        try:
            with self.assertRaises(replay._RequestFailure):
                handler._respond(200, b"x" * replay.MAX_BODY_BYTES)
            self.assertLess(time.monotonic() - started, 0.4)
            self.assertFalse(sender.getblocking())
        finally:
            receiver.close()
            sender.close()
            release.cancel()
            release.join(timeout=1)
            self.assertFalse(release.is_alive())

    def test_bad_phase_and_request_caps_reject(self):
        with self.assertRaisesRegex(replay.ReplayError, "explicitly absent or present"):
            replay.load_routes("both")
        with self.assertRaisesRegex(replay.ReplayError, "max requests"):
            replay.ReplayServer(self.absent, max_requests=replay.MAX_REQUESTS + 1)


if __name__ == "__main__":
    unittest.main()
