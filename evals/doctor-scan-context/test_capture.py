"""Offline failure-control tests; never invoke cluster/container/provider tools."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
import shutil

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("doctor_scan_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capture)


class CommandEvidenceTests(unittest.TestCase):
    def command(self, source, **kwargs):
        return capture.run([sys.executable, "-c", source], env={"PATH": os.defpath}, **kwargs)

    def test_nonzero_output_retained_before_raise(self):
        receipt = {"commands": []}
        with self.assertRaisesRegex(RuntimeError, "build"):
            capture.record_command(receipt, "build", [sys.executable, "-c", "import sys; print('build evidence'); sys.exit(7)"], env={}, deadline=time.monotonic() + 5)
        self.assertEqual(7, receipt["commands"][0]["exitCode"])
        self.assertIn("build evidence", receipt["commands"][0]["stdout"])

    def test_timeout_preserves_partial_output(self):
        result = self.command("import time; print('before timeout', flush=True); time.sleep(5)", timeout=0.3)
        self.assertIn("before timeout", result["stdout"])
        self.assertIn("deadline", result["failure"])
        self.assertFalse(capture.succeeded(result))
        self.assertLess(result["elapsedSeconds"], 2)

    def test_output_bound_preserves_prefix(self):
        result = self.command("import sys; sys.stdout.write('x' * 100000); sys.stdout.flush()", max_output=128)
        self.assertEqual("x" * 128, result["stdout"])
        self.assertIn("output exceeded", result["failure"])
        self.assertFalse(capture.succeeded(result))

    def test_expired_overall_deadline_starts_no_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            marker = Path(tmp) / "must-not-exist"
            result = self.command("from pathlib import Path; Path(" + repr(str(marker)) + ").touch()", deadline=time.monotonic() - 1)
            self.assertFalse(marker.exists())
            self.assertIn("before command", result["failure"])
            self.assertIsNone(result["exitCode"])

    def test_secret_command_redacts_receipt_but_returns_private_data(self):
        receipt = {"commands": []}
        result = capture.record_command(receipt, "token", [sys.executable, "-c", "print('private-token-value')"], env={}, deadline=time.monotonic() + 5, secret=True)
        self.assertIn("private-token-value", result["stdout"])
        self.assertNotIn("private-token-value", json.dumps(receipt))
        self.assertTrue(capture.succeeded(receipt["commands"][0]))

    def test_missing_executable_is_recorded_failure(self):
        receipt = {"commands": []}
        with self.assertRaises(RuntimeError):
            capture.record_command(receipt, "missing-tool", ["/nonexistent/scout-proof-tool"], env={}, deadline=time.monotonic() + 5)
        self.assertIn("FileNotFoundError", receipt["commands"][0]["failure"])


    def test_observation_transport_failure_stops_after_retaining_evidence(self):
        receipt = {"commands": []}
        failure = {"argv": ["owned-command"], "exitCode": -9, "stdout": "partial evidence", "stderr": "", "failure": "InterruptedError"}
        with patch.object(capture, "run", return_value=failure):
            with self.assertRaisesRegex(RuntimeError, "observation transport failed"):
                capture.record_observation(receipt, "fixed-doctor-denied", ["owned-command"], env={}, deadline=time.monotonic()+5)
        self.assertEqual("partial evidence", receipt["commands"][0]["stdout"])


class ObservationAcceptanceTests(unittest.TestCase):
    def observations(self):
        records = []
        denial = 'deployments.apps is forbidden: User "system:serviceaccount:scout-context-proof:doctor-denied" cannot list resource'
        for phase in ("old-ambient-allowed", "old-explicit-denial", "fixed-doctor-allowed", "fixed-doctor-denied"):
            for command in ("doctor", "scan"):
                denied = phase == "fixed-doctor-denied"
                body = {"namespace": "scout-context-proof", "resources": {"total": 0 if denied else 2},
                        "kubernetesContext": "doctor-denied" if denied else "doctor-allowed", "warnings": [denial] if denied else []}
                if command == "scan":
                    body = {"state": {"summary": {}, "warnings": [denial] if denied else []}}
                records.append({"phase": phase, "argv": ["/private/cub-scout", command],
                                "exitCode": 1 if phase == "old-explicit-denial" else 0,
                                "stdout": json.dumps(body), "stderr": "unknown flag: --kube-context" if phase == "old-explicit-denial" else "",
                                "failure": None})
        return records

    def test_complete_structured_controls(self):
        capture.validate_observations(self.observations(), 2)

    def test_clean_empty_denied_result_is_not_accepted(self):
        rows = self.observations()
        rows[-1]["stdout"] = json.dumps({"state": {"summary": {}, "warnings": []}})
        with self.assertRaisesRegex(RuntimeError, "denied service-account"):
            capture.validate_observations(rows, 2)

    def test_unrelated_warning_is_not_denial_proof(self):
        rows = self.observations()
        rows[-1]["stdout"] = json.dumps({"state": {"summary": {}, "warnings": ["warning: optional API missing"]}})
        with self.assertRaises(RuntimeError):
            capture.validate_observations(rows, 2)

    def test_timeout_is_not_a_denied_result(self):
        rows = self.observations()
        rows[-1].update(exitCode=-9, failure="TimeoutError", stderr="forbidden")
        with self.assertRaisesRegex(RuntimeError, "transport failure"):
            capture.validate_observations(rows, 2)

    def test_wrong_context_and_fixture_count_fail(self):
        for field, value in (("kubernetesContext", "ambient"), ("resources", {"total": 0})):
            rows = self.observations()
            body = json.loads(rows[4]["stdout"])
            body[field] = value
            rows[4]["stdout"] = json.dumps(body)
            with self.assertRaises(RuntimeError):
                capture.validate_observations(rows, 2)

    def test_missing_or_duplicated_commands_fail(self):
        for rows in (self.observations()[:-1], self.observations() + [self.observations()[0]]):
            with self.assertRaisesRegex(RuntimeError, "incomplete or duplicated"):
                capture.validate_observations(rows, 2)


class LifecycleEvidenceTests(unittest.TestCase):
    def invoke(self, *, missing_tools=False, create_ok=False):
        with tempfile.TemporaryDirectory(dir="/tmp", prefix="scout-proof-unit-") as tmp:
            shared = Path(tmp) / "shared"
            shared.write_text("hash-only fixture")
            output = Path(tmp) / "output"
            seen = []

            def fake_run(argv, **_kwargs):
                seen.append(argv)
                args = argv[1:]
                result = {"argv": argv, "exitCode": 0, "stdout": "", "stderr": "", "failure": None}
                if args == ["version"]:
                    result["stdout"] = "kind " + capture.KIND_VERSION
                elif args == ["context", "show"]:
                    result["stdout"] = "desktop-linux"
                elif args[:2] == ["context", "inspect"]:
                    result["stdout"] = json.dumps("unix:///owned/test.sock")
                elif args[:2] == ["image", "inspect"]:
                    result["stdout"] = json.dumps([{"RepoDigests": ["kindest/node@" + capture.NODE_IMAGE.rsplit("@", 1)[1]]}])
                elif "rev-parse" in args:
                    result["stdout"] = capture.OLD_SOURCE if "source-old" in args[1] else "a" * 40
                elif args[:2] == ["create", "cluster"]:
                    if create_ok:
                        Path(args[args.index("--kubeconfig") + 1]).write_text("owned-private-config")
                    else:
                        result["exitCode"] = 1
                elif args[:2] == ["config", "rename-context"]:
                    result["exitCode"] = 2  # Stop before any token or API command.
                elif args[:2] == ["delete", "cluster"]:
                    result["exitCode"] = 9  # Cleanup failure must remain visible.
                elif args[:2] == ["ps", "-aq"]:
                    result["stdout"] = "possible-partial-node"
                return result

            argv = ["capture.py", "--execute", "--integrity-only-shared-kubeconfig", str(shared), "--output-dir", str(output)]
            with patch.object(capture, "FIXED_SOURCE", "a" * 40), patch.object(sys, "argv", argv), patch.object(capture.shutil, "which", return_value=None if missing_tools else sys.executable), patch.object(capture, "run", side_effect=fake_run), patch.dict(os.environ, {"DOCKER_HOST": ""}):
                with self.assertRaises(RuntimeError):
                    capture.main()
            receipt = json.loads((output / "receipt.json").read_text())
            self.assertEqual("failed", receipt["acceptance"])
            self.assertEqual("hash-only fixture", shared.read_text())
            work = receipt.get("retainedWorkDirectory")
            if work:
                self.assertFalse((Path(work) / "kubeconfig").exists())
                shutil.rmtree(work)  # Only the exact disposable tree returned by this test.
            return receipt, seen

    def test_missing_tools_leave_setup_failure_receipt(self):
        receipt, seen = self.invoke(missing_tools=True)
        self.assertEqual("setup", receipt["phase"])
        self.assertFalse(receipt["clusterCreationAttempted"])
        self.assertFalse(seen)

    def test_failed_creation_never_deletes_uncertain_cluster(self):
        receipt, seen = self.invoke()
        self.assertFalse(any(args[1:3] == ["delete", "cluster"] for args in seen))
        self.assertTrue(receipt["clusterCreationAttempted"])
        self.assertFalse(receipt["clusterCreationSucceeded"])
        self.assertTrue(any("ownership review" in error for error in receipt["cleanupErrors"]))
        self.assertTrue(receipt["sharedKubeconfigUnchanged"])

    def test_failed_cleanup_retains_failure_and_integrity(self):
        receipt, seen = self.invoke(create_ok=True)
        self.assertTrue(any(args[1:3] == ["delete", "cluster"] for args in seen))
        self.assertTrue(receipt["clusterCreationSucceeded"])
        self.assertIn("kind delete failed", receipt["cleanupErrors"])
        self.assertTrue(receipt["sharedKubeconfigUnchanged"])


if __name__ == "__main__":
    unittest.main()
