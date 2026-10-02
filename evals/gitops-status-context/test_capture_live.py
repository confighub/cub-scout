"""Offline acceptance controls for the opt-in #753 owned-kind proof."""
import importlib.util
import contextlib
import io
import argparse
import http.client
import ssl
import time
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
    app = {"kind": "Application", "name": "scout-context-app", "namespace": capture.NAMESPACE, "ready": True,
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
        rows = [{"context": capture.CONTROLLER_DENIED, "method": "GET", "path": route,
                 "status": 403 if route == path else 200, "disposition": "forwarded"}
                for route in (path, f"/apis/argoproj.io/v1alpha1/namespaces/{capture.NAMESPACE}/applications",
                    f"/apis/argoproj.io/v1alpha1/namespaces/{capture.NAMESPACE}/applications/scout-context-app",
                    f"/api/v1/namespaces/{capture.NAMESPACE}/pods")]
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
                    "checks": {"resize": True, "scroll": True, "scrollBack": True, "quit": True, "immutableSummary": True}}
            capture.validate_tui_probe(data, context=context, result=result)
        bad = {"schema": "gitops-status-context-tui.v1", "passed": True, "context": capture.ALLOWED,
               "view": "Kubernetes context: doctor-allowed", "checks": {"resize": True, "scroll": True, "scrollBack": True, "quit": True, "immutableSummary": True}}
        with self.assertRaises(RuntimeError):
            capture.validate_tui_probe(bad, context=capture.ALLOWED, result="allowed")

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


class IsolationTests(unittest.TestCase):
    def test_private_environment_and_restricted_path(self):
        with tempfile.TemporaryDirectory() as tmp, patch.dict(os.environ, {
                "CUB_TOKEN": "secret", "HOME": "/ambient", "PATH": "/ambient/bin",
                "HTTPS_PROXY": "https://proxy", "CUB_SCOUT_TEST_GITOPS_JSON": "hook"}):
            root = Path(tmp)
            env = capture.observation_env(private_home=root / "home", shims=root / "shims",
                kubeconfig=root / "config", cub_log=root / "calls")
            self.assertEqual(str(root / "shims"), env["PATH"])
            self.assertEqual(str(root / "home"), env["HOME"])
            for key in ("XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"):
                self.assertTrue(env[key].startswith(env["HOME"] + "/"))
            for key in ("CUB_TOKEN", "CUB_PLUGIN", "HTTPS_PROXY", "CUB_SCOUT_TEST_GITOPS_JSON"):
                self.assertNotIn(key, env)
            self.assertEqual(str(root / "config"), env["KUBECONFIG"])

    def test_auth_only_shim_exact_count_and_secret_free_rejections(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            shim = capture.create_observation_shims(root)
            log = root / "calls"
            env = capture.observation_env(private_home=root, shims=root, kubeconfig=root / "config", cub_log=log)
            result = capture.run([str(shim), "auth", "status"], env=env, timeout=5)
            self.assertEqual(73, result["exitCode"])
            capture.validate_cub_records(log, old_bytes=0, expected_calls=1)
            with self.assertRaises(RuntimeError):
                capture.validate_cub_records(log, old_bytes=0, expected_calls=2)
            size = log.stat().st_size
            for argv in (["auth", "status", "--quiet"], ["auth", "get-token"], ["unit", "get", "token-secret"]):
                self.assertEqual(97, capture.run([str(shim), *argv], env=env, timeout=5)["exitCode"])
            self.assertNotIn("token-secret", log.read_text())
            with self.assertRaises(RuntimeError):
                capture.validate_cub_records(log, old_bytes=size, expected_calls=0)

    def test_immutable_credential_free_observation_config(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            proxy = type("Proxy", (), {"label": capture.ALLOWED, "endpoint": "http://127.0.0.1:1234"})()
            config = capture._proxy_kubeconfig(capture.ALLOWED, [proxy])
            self.assertEqual({}, config["users"][0]["user"])
            path = root / "config"
            capture.write_private(path, json.dumps(config), immutable=True)
            self.assertEqual(0o400, path.stat().st_mode & 0o777)
            before = capture.digest(path)
            capture.verify_immutable_configs({}, {path: before})
            path.chmod(0o600)
            path.write_text("changed")
            with self.assertRaisesRegex(RuntimeError, "changed"):
                capture.verify_immutable_configs({}, {path: before})

    def test_exact_proxy_routes_queries_and_mutations_fail_closed(self):
        route = f"/api/v1/namespaces/{capture.NAMESPACE}/pods"
        self.assertTrue(capture.allowed_status_request(route, {}))
        for query in ({"watch": ["true"]}, {"timeout": ["32s"]}, {"labelSelector": ["x=y"]}):
            self.assertFalse(capture.allowed_status_request(route, query))
        for path in ("/version", "/api/v1/secrets", route + "/other", route.replace(capture.NAMESPACE, "shared")):
            self.assertFalse(capture.allowed_status_request(path, {}))
        proxy = capture.StatusAPIProxy(label=capture.ALLOWED, upstream="https://127.0.0.1:1",
            tls=ssl.create_default_context(), namespace=capture.NAMESPACE, deployment="unused",
            application="scout-context-app", missing_deployment="unused")
        try:
            port = proxy.server.server_address[1]
            for method, path in (("POST", route), ("GET", route + "?watch=true"), ("GET", "/api/v1/secrets")):
                conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
                try:
                    conn.request(method, path)
                    response = conn.getresponse()
                    self.assertEqual(403, response.status)
                    response.read()
                finally:
                    conn.close()
            rows = proxy.snapshot()
            self.assertEqual(3, len(rows))
            self.assertTrue(all(row["disposition"] == "rejected" for row in rows))
        finally:
            proxy.close()
        self.assertFalse(proxy.thread.is_alive())

    def test_mcp_dispatch_uses_actual_result_and_rejects_errors(self):
        body = status(capture.ALLOWED)
        response = {"jsonrpc": "2.0", "id": 2, "result": {"isError": False,
            "structuredContent": {"data": body}, "content": [{"type": "text", "text": "guide"}]}}
        capture.validate_mcp_result(response, context=capture.ALLOWED, result="allowed")
        text_response = {"id": 2, "result": {"content": [{"type": "text", "text": "guide"},
            {"type": "text", "text": json.dumps(body)}]}}
        capture.validate_mcp_result(text_response, context=capture.ALLOWED, result="allowed")
        for bad in ({"error": {}}, {"result": {"isError": True}},
                    {"result": {"structuredContent": {"data": []}}}):
            with self.assertRaises(RuntimeError):
                capture.validate_mcp_result(bad, context=capture.ALLOWED, result="allowed")
        self.assertEqual(response, capture._last_json_line(json.dumps({"id": 1}) + "\n" + json.dumps(response)))
        with self.assertRaises(RuntimeError):
            capture._last_json_line(json.dumps({"id": 1}))

    def test_mcp_transport_reuses_bounded_runner(self):
        with patch.object(capture._lifecycle, "run", return_value={"failure": "timeout"}) as run:
            result = capture._run_mcp("/private/scout", "{}\n", env={"PATH": "/private"}, deadline=123)
        self.assertEqual("timeout", result["failure"])
        self.assertEqual(b"{}\n", run.call_args.kwargs["input_data"])
        self.assertEqual(90, run.call_args.kwargs["timeout"])

    def test_failure_receipt_and_private_cleanup_after_exception(self):
        # All commands/proxies are mocked. This exercises dormant lifecycle
        # finalization directly; main's unconditional admission guard is intact.
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            root = Path(tmp)
            shared = root / "integrity-only"
            shared.write_text("dummy integrity bytes")
            output = root / "receipt"
            args = argparse.Namespace(integrity_only_shared_kubeconfig=shared, output_dir=output)
            for failure in (RuntimeError("offline injected failure"), KeyboardInterrupt()):
                if output.exists(): capture.shutil.rmtree(output)
                with patch.object(capture.shutil, "which", return_value="/fake/tool"), \
                     patch.object(capture, "record_command", side_effect=failure), \
                     patch.object(capture._lifecycle, "cleanup_cluster", side_effect=RuntimeError("cleanup injected")), \
                     patch.object(capture._lifecycle, "cleanup_worktrees", return_value=[]) as cleanup:
                    with self.assertRaises(type(failure)):
                        capture._execute_capture(args, argparse.ArgumentParser())
                cleanup.assert_called_once()
                receipt = json.loads((output / "receipt.json").read_text())
                self.assertEqual("failed", receipt["acceptance"])
                self.assertIn(type(failure).__name__, receipt["error"])
                self.assertTrue(receipt["privateDirectoryRemoved"])
                self.assertTrue(receipt["sharedKubeconfigUnchanged"])
                self.assertIsNone(receipt["retainedWorkDirectory"])
                self.assertIn("owned lifecycle cleanup raised an exception", receipt["cleanupErrors"])
                self.assertEqual(0o600, (output / "receipt.json").stat().st_mode & 0o777)

    def test_cleanup_reuses_owned_cluster_rules_for_uncertain_creation(self):
        receipt = {"commands": []}
        with patch.object(capture._lifecycle, "run", return_value={"exitCode": 0, "stdout": "partial-node", "failure": None}) as run:
            errors = capture._lifecycle.cleanup_cluster(receipt, name="private-owned", created=False,
                creation_attempted=True, tools={"docker": "/fake/docker", "kind": "/fake/kind"}, env={}, deadline=time.monotonic()+5)
        self.assertTrue(errors)
        self.assertEqual("/fake/docker", run.call_args.args[0][0])
        self.assertNotIn("delete", run.call_args.args[0])


if __name__ == "__main__":
    unittest.main()
