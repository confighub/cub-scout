"""Offline acceptance controls for the opt-in #753 owned-kind proof."""
import importlib.util
import contextlib
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("gitops_status_live_capture", HERE / "capture_live.py")
capture = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = capture
spec.loader.exec_module(capture)


def status(context, *, denied=None):
    app = {"kind": "Application", "name": "scout-context-app", "ready": True,
           "healthStatus": "Healthy", "podTotal": 0, "runtimeIssues": []}
    coverage = [{"family": "Modelplane", "status": "found", "omissions": []}]
    if denied == "pods":
        app["runtimeOmission"] = {"resource": "pods", "reason": "forbidden"}
    elif denied == "controller":
        coverage[0] = {"family": "Modelplane", "status": "unreadable",
                       "omissions": [{"resource": "modelplane.ai/modeldeployments", "reason": "forbidden",
                                      "message": 'User "' + capture.DENIED_USER + '" cannot list resource'}]}
    return {"context": context, "deployers": [app], "controllerCoverage": coverage}


class StatusValidationTests(unittest.TestCase):
    def test_accepts_allowed_and_exact_partial_or_runtime_denial(self):
        capture.validate_status_json(json.dumps(status(capture.ALLOWED)), context=capture.ALLOWED, result="allowed")
        capture.validate_status_json(json.dumps(status(capture.CONTROLLER_DENIED, denied="controller")),
                                     context=capture.CONTROLLER_DENIED, result="controller-denied")
        capture.validate_status_json(json.dumps(status(capture.PODS_DENIED, denied="pods")),
                                     context=capture.PODS_DENIED, result="pods-denied")

    def test_denial_cannot_be_reported_as_absence_or_healthy_zero_pods(self):
        bad = status(capture.CONTROLLER_DENIED)
        with self.assertRaisesRegex(RuntimeError, "hidden or treated as healthy"):
            capture.validate_status_json(json.dumps(bad), context=capture.CONTROLLER_DENIED, result="controller-denied")
        bad = status(capture.PODS_DENIED)
        bad["deployers"][0]["podTotal"] = 4
        with self.assertRaisesRegex(RuntimeError, "runtimeOmission"):
            capture.validate_status_json(json.dumps(bad), context=capture.PODS_DENIED, result="pods-denied")

    def test_wrong_context_and_missing_application_fail(self):
        with self.assertRaisesRegex(RuntimeError, "context label"):
            capture.validate_status_json(json.dumps(status(capture.ALLOWED)), context="other", result="allowed")
        bad = status(capture.ALLOWED)
        bad["deployers"] = []
        with self.assertRaisesRegex(RuntimeError, "omitted the owned"):
            capture.validate_status_json(json.dumps(bad), context=capture.ALLOWED, result="allowed")

    def test_only_allowlisted_gets_and_exact_forbidden_path_count(self):
        path = "/apis/modelplane.ai/v1alpha1/namespaces/" + capture.NAMESPACE + "/modeldeployments"
        rows = [{"context": capture.CONTROLLER_DENIED, "method": "GET", "path": path,
                 "status": 403, "disposition": "forwarded"}]
        capture.validate_api_records(rows, expected_context=capture.CONTROLLER_DENIED, expected_denial=path)
        for bad in (dict(rows[0], method="POST"), dict(rows[0], path="/api/v1/secrets"),
                    dict(rows[0], context=capture.ALLOWED), dict(rows[0], status=200)):
            with self.assertRaises(RuntimeError):
                capture.validate_api_records([bad], expected_context=capture.CONTROLLER_DENIED, expected_denial=path)

    def test_tui_requires_production_context_label_and_model_actions(self):
        for result, context, suffix in (("allowed", capture.ALLOWED, ""),
                ("controller-denied", capture.CONTROLLER_DENIED, "Modelplane | unreadable | forbidden"),
                ("pods-denied", capture.PODS_DENIED, "pods: forbidden")):
            data = {"schema": "gitops-status-context-tui.v1", "passed": True, "context": context,
                    "view": f"- Kubernetes context label: `{context}` (not a stable cluster ID)\n{suffix} Healthy",
                    "checks": {"resize": True, "scroll": True, "quit": True}}
            capture.validate_tui_probe(data, result=result)
        bad = {"schema": "gitops-status-context-tui.v1", "passed": True, "context": capture.ALLOWED,
               "view": "Kubernetes context: doctor-allowed", "checks": {"resize": True, "scroll": True, "quit": True}}
        with self.assertRaises(RuntimeError):
            capture.validate_tui_probe(bad, result="allowed")

    def test_mcp_stdio_request_contains_typed_context(self):
        request = capture._mcp_call("gitops_status", {"context": capture.ALLOWED, "namespace": capture.NAMESPACE})
        rows = [json.loads(line) for line in request.splitlines()]
        self.assertEqual("initialize", rows[0]["method"])
        self.assertEqual("notifications/initialized", rows[1]["method"])
        self.assertEqual("tools/call", rows[2]["method"])
        self.assertEqual({"context": capture.ALLOWED, "namespace": capture.NAMESPACE}, rows[2]["params"]["arguments"])

    def test_live_execution_is_unconditionally_not_admitted(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            shared = Path(tmp) / "shared-never-read"
            output = Path(tmp) / "must-not-be-created"
            argv = ["capture_live.py", "--execute", "--integrity-only-shared-kubeconfig", str(shared),
                    "--output-dir", str(output)]
            with patch.object(sys, "argv", argv), patch.object(capture.shutil, "which") as which:
                stderr = io.StringIO()
                with contextlib.redirect_stderr(stderr), self.assertRaises(SystemExit):
                    capture.main()
                self.assertIn("NOT ADMITTED", stderr.getvalue())
            which.assert_not_called()
            self.assertFalse(shared.exists())
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
