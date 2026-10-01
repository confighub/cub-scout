import importlib.util
import json
import os
from pathlib import Path
import tempfile
import sys
import time
import unittest

SPEC = importlib.util.spec_from_file_location("probe", Path(__file__).with_name("probe.py"))
probe = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(probe)


def request(tools=None, auth="Bearer " + probe.FAKE_KEY):
    body = {"tools": tools or [{"name": "Read"}]}
    return {"body": json.dumps(body), "authorization": auth}


def terminal(subtype="success"):
    return (json.dumps({"type": "result", "subtype": subtype, "result": "offline-probe-terminal"}) + "\n").encode()


class ProbeContracts(unittest.TestCase):
    def test_terminal_and_inventory(self):
        observed = probe.validate_probe(terminal(), [request()])
        self.assertEqual(observed["tool_inventory"], ["Read"])
        self.assertEqual(observed["terminal_subtype"], "success")

    def test_missing_malformed_or_duplicate_terminal_fails(self):
        for payload in (b"", b"not json\n", terminal() + terminal()):
            with self.subTest(payload=payload), self.assertRaises(probe.ProbeError):
                probe.validate_probe(payload, [request()])

    def test_delegation_inventory_and_inventory_drift_fail(self):
        with self.assertRaises(probe.ProbeError):
            probe.validate_probe(terminal(), [request([{"name": "Read"}, {"name": "Agent"}])])
        with self.assertRaises(probe.ProbeError):
            probe.validate_probe(terminal(), [request(), request([{"name": "Read"}, {"name": "Bash"}])])

    def test_duplicate_tool_use_ids_fail(self):
        with self.assertRaises(probe.ProbeError):
            probe.validate_probe(terminal(), [request([{"name": "Read", "id": "x"}, {"name": "Read", "id": "x"}])])

    def test_request_count_and_size_limits(self):
        with self.assertRaises(probe.ProbeError):
            probe.validate_probe(terminal(), [])
        self.assertEqual(probe.MAX_BODY, 2 * 1024 * 1024)
        self.assertEqual(probe.MAX_REQUESTS, 4)

    def test_private_output_refuses_reuse_and_symlink(self):
        with tempfile.TemporaryDirectory() as td:
            target = Path(td) / "out"
            made = probe.private_output(target)
            self.assertEqual(made.stat().st_mode & 0o777, 0o700)
            with self.assertRaises(probe.ProbeError):
                probe.private_output(target)
            alias = Path(td) / "alias"
            alias.symlink_to(made)
            with self.assertRaises(probe.ProbeError):
                probe.private_output(alias)

    def test_environment_is_synthetic_and_no_auth_inheritance(self):
        with tempfile.TemporaryDirectory() as td:
            env = probe.minimal_env(Path(td), 12345)
        self.assertEqual(env["ANTHROPIC_API_KEY"], probe.FAKE_KEY)
        self.assertEqual(env["ANTHROPIC_BASE_URL"], "http://127.0.0.1:12345")
        self.assertNotIn("AWS_ACCESS_KEY_ID", env)
        self.assertNotIn("GOOGLE_APPLICATION_CREDENTIALS", env)
        self.assertNotIn("CLAUDE_CODE_OAUTH_TOKEN", env)

    def test_fake_authorization_is_preserved_as_fixture_only(self):
        self.assertEqual(request()["authorization"], "Bearer " + probe.FAKE_KEY)
        self.assertNotIn("real", probe.FAKE_KEY)

    def test_unsupported_platform_fails_closed(self):
        with self.assertRaises(probe.ProbeError):
            probe.sandbox_prefix_for_test("Linux")

    def test_owned_process_timeout_and_output_limit(self):
        with tempfile.TemporaryDirectory() as td:
            env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin")}
            with self.assertRaises(probe.ProbeError):
                probe.run_bounded([sys.executable, "-c", "import time; time.sleep(10)"], env, Path(td), timeout=0.1)
            with self.assertRaises(probe.ProbeError):
                probe.run_bounded([sys.executable, "-c", "print('x'*1000)"], env, Path(td), timeout=2, max_output=100)


if __name__ == "__main__":
    unittest.main()
