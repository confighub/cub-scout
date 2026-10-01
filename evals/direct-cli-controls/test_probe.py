import base64
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("probe", Path(__file__).with_name("probe.py"))
probe = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(probe)


def request(body=None, auth=None, path="/v1/messages", method="POST", headers=None):
    payload = body if isinstance(body, bytes) else json.dumps(body or {"tools": [{"name": "Read"}], "stream": True}).encode()
    values = {"content-length": str(len(payload)), "content-type": "application/json",
              "x-api-key": probe.FAKE_KEY if auth is None else auth}
    values.update(headers or {})
    recs, count, lock = [], [0], threading.Lock()
    result = probe.handle_request(method, path, values, payload, recs, lock, count)
    return result, recs, count


def terminal(subtype="success", text="offline-probe-terminal", **extra):
    value = {"type": "result", "subtype": subtype, "result": text}
    value.update(extra)
    return (json.dumps(value) + "\n").encode()


class OfflineContracts(unittest.TestCase):
    def test_environment_uses_only_fake_auth_and_correct_controls(self):
        env = probe.minimal_env(Path("/private/config"), 1234)
        self.assertNotIn("HOME", env)
        self.assertEqual(env["CLAUDE_CONFIG_DIR"], "/private/config")
        self.assertEqual(env["ANTHROPIC_API_KEY"], probe.FAKE_KEY)
        self.assertEqual(env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"], "1")
        self.assertEqual(env["CLAUDE_CODE_DISABLE_FAST_MODE"], "1")
        self.assertNotIn("CLAUDE_CODE_SIMPLE", env)

    def test_cli_pin_binds_path_version_hash(self):
        self.assertEqual(probe.PINNED_VERSION, "2.1.274")
        self.assertEqual(probe.PINNED_SHA256, "3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a")
        with self.assertRaises(probe.ProbeError):
            probe.sandbox_prefix_for_test("Linux", "/fake/sandbox", 123)

    def test_parsing_accepts_one_exact_success_and_inventory(self):
        _, records, _ = request()
        observed = probe.validate_probe(terminal(), records, 0)
        self.assertEqual(observed["tool_inventory"], ["Read"])
        self.assertEqual(observed["terminal_text"], "offline-probe-terminal")

    def test_terminal_truth_rejects_rc_error_max_turns_text_and_is_error(self):
        _, records, _ = request()
        cases = [(terminal(), 1), (terminal("error_max_turns"), 0),
                 (terminal(text="near miss"), 0), (terminal(is_error=True), 0),
                 (terminal()+terminal(), 0), (b"", 0), (b"{bad}\n", 0)]
        for payload, rc in cases:
            with self.subTest(payload=payload, rc=rc), self.assertRaises(probe.ProbeError):
                probe.validate_probe(payload, records, rc)

    def test_inventory_must_be_exact_read_only_list(self):
        for tools in ([], [{"name": "Bash"}], [{"name": "Read"}, {"name": "Read"}], [{"name": "Read"}, {"name": "Agent"}]):
            _, records, _ = request({"tools": tools, "stream": True})
            with self.subTest(tools=tools), self.assertRaises(probe.ProbeError):
                probe.validate_probe(terminal(), records, 0)

    def test_strict_json_rejects_duplicate_keys(self):
        with self.assertRaises(ValueError):
            probe.strict_json('{"stream":true,"stream":false}')

    def test_body_auth_path_method_and_stream_validation(self):
        good_body = b'{"tools":[{"name":"Read"}],"stream":true}'
        result, records, _ = request(good_body)
        self.assertEqual(result[0], 200)
        self.assertEqual(records[0]["body_base64"], base64.b64encode(good_body).decode())
        self.assertNotIn("authorization", records[0])
        for args, expected in [
            ((good_body, "wrong", "/v1/messages", "POST", None), 401),
            ((good_body, None, "/elsewhere", "POST", None), 404),
            ((good_body, None, "/v1/messages", "GET", None), 405),
            ((b'{"stream":"true"}', None, "/v1/messages", "POST", None), 400),
            ((b'{"stream":true,"stream":false}', None, "/v1/messages", "POST", None), 400),
        ]:
            with self.subTest(args=args):
                self.assertEqual(request(*args)[0][0], expected)
        oversized = b"x" * (probe.MAX_BODY + 1)
        self.assertEqual(request(oversized)[0][0], 413)
        # Malformed length rejected before body can be interpreted or retained.
        result, retained, _ = request(good_body, headers={"content-length": "1e3"})
        self.assertEqual(result[0], 400)
        self.assertEqual(retained, [])

    def test_body_reader_enforces_size_completeness_and_deadline(self):
        self.assertEqual(probe.read_bounded_body(lambda n: b"x"[:n], 1), b"x")
        with self.assertRaises(ValueError): probe.read_bounded_body(lambda n: b"", 1)
        with self.assertRaises(ValueError): probe.read_bounded_body(lambda n: b"", probe.MAX_BODY+1)
        def slow(n):
            import time
            time.sleep(0.02)
            return b"x"[:n]
        with self.assertRaises(TimeoutError): probe.read_bounded_body(slow, 1, timeout=0.001)

    def test_response_stream_mode_comes_from_body(self):
        _, records, _ = request({"stream": False, "tools": [{"name": "Read"}]})
        kind, payload = probe.response_for(False)
        self.assertEqual(kind, "application/json")
        self.assertIn(b"offline-probe-terminal", payload)
        self.assertFalse(json.loads(records[0]["body"])["stream"])
        self.assertTrue(probe.response_for(True)[0] == "text/event-stream")

    def test_request_limit_is_atomic_under_concurrency(self):
        records, count, lock = [], [0], threading.Lock()
        body = b'{"tools":[{"name":"Read"}],"stream":true}'
        headers = {"x-api-key": probe.FAKE_KEY, "content-length": str(len(body))}
        results = []
        def invoke():
            results.append(probe.handle_request("POST", "/v1/messages", headers, body, records, lock, count)[0])
        threads = [threading.Thread(target=invoke) for _ in range(20)]
        for t in threads: t.start()
        for t in threads: t.join()
        self.assertEqual(count[0], 20)
        self.assertEqual(len(records), probe.MAX_REQUESTS)
        self.assertEqual(results.count(200), probe.MAX_REQUESTS)
        self.assertTrue(all(r == 429 for r in results if r != 200))

    def test_server_arrival_counter_saturates_and_bounds_rejected_attempts(self):
        count, lock, values = [0], threading.Lock(), []
        workers = [threading.Thread(target=lambda: values.append(probe.reserve_request(count, lock))) for _ in range(100)]
        for worker in workers: worker.start()
        for worker in workers: worker.join()
        self.assertEqual(count[0], probe.MAX_REQUESTS+1)
        self.assertEqual(values.count(probe.MAX_REQUESTS+1), 100-probe.MAX_REQUESTS)

    def test_private_output_and_failure_trace_persistence(self):
        with tempfile.TemporaryDirectory() as td:
            root = probe.private_output(Path(td)/"out")
            probe.persist_trace(root, {"run_status":"timeout"}, b"partial", b"diagnostic", [{"body":"fixture"}])
            self.assertEqual((root/"stdout.bin").read_bytes(), b"partial")
            self.assertEqual((root/"stderr.bin").read_bytes(), b"diagnostic")
            self.assertEqual(json.loads((root/"provenance.json").read_text())["run_status"], "timeout")
            self.assertEqual((root/"provenance.json").stat().st_mode & 0o777, 0o600)
            self.assertEqual(json.loads((root/"requests.json").read_text())[0]["body"], "fixture")
            with self.assertRaises(probe.ProbeError): probe.private_output(root)
            alias = Path(td)/"alias"; alias.symlink_to(root)
            with self.assertRaises(probe.ProbeError): probe.private_output(alias)

    def test_process_timeout_keeps_partial_output_and_cleans_owned_group(self):
        with tempfile.TemporaryDirectory() as td:
            code = "import subprocess,sys,time; c=subprocess.Popen([sys.executable,'-c','import time; time.sleep(5)']); print('child='+str(c.pid),flush=True); time.sleep(5)"
            original = os.killpg
            called = []
            def observed(pgid, sig):
                called.append((pgid, sig))
                return original(pgid, sig)
            with mock.patch.object(probe.os, "killpg", side_effect=observed):
                with self.assertRaises(probe.ProcessFailure) as caught:
                    probe.run_bounded([sys.executable, "-c", code], {"PATH": os.environ["PATH"]}, Path(td), time.monotonic()+0.1)
            self.assertEqual(caught.exception.status, "timeout")
            self.assertIn(b"child=", caught.exception.stdout)
            self.assertTrue(called)
            self.assertTrue(all(pgid == called[0][0] for pgid, _ in called))
            child_pid = int(caught.exception.stdout.decode().split("child=")[1].splitlines()[0])
            # A bounded check confirms the descendant in the owned group is gone.
            with self.assertRaises(ProcessLookupError): os.kill(child_pid, 0)

    def test_output_overflow_is_structured_and_preserves_partial(self):
        with tempfile.TemporaryDirectory() as td:
            with self.assertRaises(probe.ProcessFailure) as caught:
                probe.run_bounded([sys.executable, "-c", "print('x'*1000)"], {"PATH": os.environ["PATH"]}, Path(td), time.monotonic()+2, max_output=100)
            self.assertEqual(caught.exception.status, "output_limit")
            self.assertLessEqual(len(caught.exception.stdout), 100)


if __name__ == "__main__": unittest.main()
