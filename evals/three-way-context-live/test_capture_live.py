"""Offline acceptance controls; never launch a cluster or call real services."""
import argparse
import copy
import importlib.util
import contextlib
import io
import http.client
import json
import os
from pathlib import Path
import ssl
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("three_way_live_capture", HERE / "capture_live.py")
capture = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = capture
spec.loader.exec_module(capture)


def report(context, result="allowed"):
    live = {"source": "cluster", "apiVersion": "apps/v1", "kind": "Deployment", "name": capture.DEPLOYMENT,
        "namespace": capture.NAMESPACE, "unitSlug": capture.UNIT, "spaceName": capture.SPACE, "replicas": 0}
    omissions = [{"phase": "comparison-sides", "reason": reason} for reason in ("dry_unavailable", "wet_unavailable")]
    if result == "source-denied":
        omissions += [{"phase": "live-source-link", "reason": "forbidden"}, {"phase": "source-coverage", "reason": "source_anchor_unavailable"}]
    else:
        live["gitSource"] = {"repoUrl": "https://example.invalid/owned-proof.git", "revision": "proof-revision", "path": "proof/manifests"}
    if result == "pods-denied":
        omissions += [{"phase": "current-change", "reason": "forbidden"}]
    for row in omissions:
        row.update(resource="Deployment/" + capture.DEPLOYMENT, namespace=capture.NAMESPACE)
    return {"context": context, "resources": [{"result": {"resource": "Deployment/" + capture.DEPLOYMENT,
        "namespace": capture.NAMESPACE, "connected": True, "live": live,
        "notes": ["recorded unit lookup unavailable in owned proof"]}, "currentChange": {"resource": {"apiVersion": "apps/v1", "kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT}}}], "omissions": omissions,
        "summary": {"totalResources": 1, "agreement": {"state": "partial", "sources": {
            "confighub": 0, "deployer": 0 if result == "source-denied" else 1, "cluster": 1, "total": 1}}}}


def api_rows(context, result="allowed", collections=1):
    return [{"endpoint": context, "method": "GET", "path": path, "query": capture.REQUESTS[path],
             "status": status, "disposition": "forwarded"}
            for path, status in capture.expected_api_sequence(result) * collections]


def mcp(body):
    return {"jsonrpc": "2.0", "id": 2, "result": {"isError": False,
        "structuredContent": {"data": body}, "content": [{"type": "text", "text": json.dumps(body)}]}}


class ReportControls(unittest.TestCase):
    def test_correct_context_and_partial_source_runtime_results(self):
        for result, context in (("allowed", capture.ALLOWED), ("source-denied", capture.SOURCE_DENIED), ("pods-denied", capture.PODS_DENIED)):
            capture.validate_three_way_json(json.dumps(report(context, result)), context=context, result=result)
            capture.validate_api_records(api_rows(context, result), expected_context=context, result=result)
            capture.validate_api_records(api_rows(context, result, 3), expected_context=context, result=result, collections=3)

    def test_false_convergence_hidden_denial_wrong_labels_and_source_fail(self):
        for result, context in (("allowed", capture.ALLOWED), ("source-denied", capture.SOURCE_DENIED), ("pods-denied", capture.PODS_DENIED)):
            baseline = report(context, result)
            bad = []
            item = copy.deepcopy(baseline); item["context"] = "ambient"; bad.append(item)
            item = copy.deepcopy(baseline); item["summary"]["agreement"]["state"] = "full"; bad.append(item)
            item = copy.deepcopy(baseline); item["resources"][0]["result"]["live"]["unitSlug"] = "foreign"; bad.append(item)
            item = copy.deepcopy(baseline); item["resources"][0]["result"]["dry"] = {}; bad.append(item)
            item = copy.deepcopy(baseline); item["resources"][0]["result"]["notes"] = []; bad.append(item)
            item = copy.deepcopy(baseline); item["resources"][0].pop("currentChange"); bad.append(item)
            if result != "allowed":
                item = copy.deepcopy(baseline); item["omissions"] = [x for x in item["omissions"] if x["reason"] != "forbidden"]; bad.append(item)
            if result == "source-denied":
                item = copy.deepcopy(baseline); item["resources"][0]["result"]["live"]["gitSource"] = {"repoUrl": "fabricated"}; bad.append(item)
            else:
                item = copy.deepcopy(baseline); item["resources"][0]["result"]["live"].pop("gitSource"); bad.append(item)
            for item in bad:
                with self.assertRaises(RuntimeError): capture.validate_three_way_json(json.dumps(item), context=context, result=result)

    def test_exact_query_sequence_count_status_disposition_endpoint(self):
        baseline = api_rows(capture.SOURCE_DENIED, "source-denied")
        for mutation in (dict(method="POST"), dict(endpoint=capture.ALLOWED), dict(disposition="rejected"),
                dict(disposition="upstream-error"), dict(path="/version"), dict(status=404), dict(status=500), dict(status=True), dict(query={"watch": ["true"]})):
            rows = copy.deepcopy(baseline); rows[0].update(mutation)
            with self.assertRaises(RuntimeError): capture.validate_api_records(rows, expected_context=capture.SOURCE_DENIED, result="source-denied")
        for rows in ([], baseline[:-1], baseline + baseline[:1], list(reversed(baseline)), api_rows(capture.SOURCE_DENIED, "allowed")):
            with self.assertRaises(RuntimeError): capture.validate_api_records(rows, expected_context=capture.SOURCE_DENIED, result="source-denied")

    def test_mcp_text_structured_consistency_and_errors(self):
        body = report(capture.ALLOWED)
        response = mcp(body)
        capture.validate_mcp_result(response, context=capture.ALLOWED, result="allowed")
        for bad in ({"error": {}}, {"result": {"isError": True}},
                {"result": {"structuredContent": {"data": []}}},
                {"result": {"content": response["result"]["content"]}},
                {"result": {**response["result"], "content": [{"type": "text", "text": "{}"}]}}):
            with self.assertRaises(RuntimeError): capture.validate_mcp_result(bad, context=capture.ALLOWED, result="allowed")
        request = [json.loads(line) for line in capture._mcp_call("compare_three_way", {"context": capture.ALLOWED, "scope": "deploy/" + capture.DEPLOYMENT, "namespace": capture.NAMESPACE}).splitlines()]
        self.assertEqual(["initialize", "notifications/initialized", "tools/call"], [row["method"] for row in request])
        self.assertEqual(capture.ALLOWED, request[-1]["params"]["arguments"]["context"])
        self.assertEqual(response, capture._last_json_line(json.dumps({"id": 1}) + "\n" + json.dumps(response)))
        with self.assertRaises(RuntimeError): capture._last_json_line('{"id":1}')

    def test_old_negative_control_requires_actual_ambient_and_zero_selected(self):
        body = report(capture.SOURCE_DENIED, "source-denied"); body.pop("context")
        rows = [row for row in api_rows(capture.SOURCE_DENIED, "source-denied") if row["path"] != capture.APPLICATION_LIST_PATH]
        capture.validate_old_behavior(mcp(body), rows, ambient=capture.SOURCE_DENIED)
        for bad in ([], [dict(rows[0], endpoint=capture.ALLOWED), *rows[1:]], rows[:-1],
                    [dict(rows[0], status=403), *rows[1:]], rows[::-1]):
            with self.assertRaises(RuntimeError): capture.validate_old_behavior(mcp(body), bad, ambient=capture.SOURCE_DENIED)
        body["context"] = capture.ALLOWED
        with self.assertRaises(RuntimeError): capture.validate_old_behavior(mcp(body), rows, ambient=capture.SOURCE_DENIED)

    def test_live_probe_checks_cannot_be_omitted(self):
        body = report(capture.ALLOWED)
        data = {"schema": "three-way-context-tui.v1", "passed": True, "context": capture.ALLOWED,
            "report": body, "refreshedReport": body, "editedReport": body,
            "view": "Kubernetes context label: " + capture.ALLOWED + " " + capture.DEPLOYMENT, "rendered": "PARTIAL",
            "checks": dict.fromkeys(("resize", "scroll", "scrollBack", "quit", "immutableSnapshot", "immutableConfig", "refresh", "scopeEdit"), True)}
        capture.validate_tui_probe(data, context=capture.ALLOWED, result="allowed")
        for name in data["checks"]:
            bad = copy.deepcopy(data); bad["checks"][name] = False
            with self.assertRaises(RuntimeError): capture.validate_tui_probe(bad, context=capture.ALLOWED, result="allowed")
        if directory := os.environ.get("SCOUT_THREE_WAY_VIEWPORT_RESULTS"):
            for result in ("allowed", "source-denied", "pods-denied"):
                artifact = json.loads((Path(directory) / (result + ".json")).read_text())
                self.assertEqual("three-way-context-viewport.v1", artifact["schema"])
                capture.validate_viewport_probe(artifact, context="three-way-" + result, result=result)


class IsolationControls(unittest.TestCase):
    def test_refusal_and_pending_pin_before_tools_or_config_reads(self):
        with patch.object(capture, "FIXED_SOURCE", "ADMISSION_PENDING"), patch.object(capture.shutil, "which") as which, patch.object(Path, "read_bytes") as read:
            with self.assertRaisesRegex(RuntimeError, "admission pending"): capture.main()
            with self.assertRaisesRegex(RuntimeError, "admission pending"): capture._admitted_main()
            with self.assertRaisesRegex(RuntimeError, "admission pending"): capture._execute_capture(None, None)
        which.assert_not_called(); read.assert_not_called()
        with patch.object(capture, "FIXED_SOURCE", "a" * 40), patch.object(sys, "argv", ["capture", "--helper-source", "b" * 40, "--tool-pin", "git=" + "c" * 64,
                "--integrity-only-shared-kubeconfig", "/nonexistent", "--output-dir", "/tmp/nonexistent"]), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            capture._admitted_main()

    def test_pins_are_complete_strict_and_no_duplicate(self):
        capture.full_commit("a" * 40)
        for value in ("HEAD", "ADMISSION_PENDING", "A" * 40, "a" * 39):
            with self.assertRaises(RuntimeError): capture.full_commit(value)
        values = [name + "=" + "a" * 64 for name in ("git", "go", "kind", "kubectl", "docker")]
        self.assertEqual(5, len(capture.parse_tool_pins(values)))
        for bad in (values[:-1], values + values[:1], ["git=bad"], values + ["cub=" + "a" * 64]):
            with self.assertRaises(RuntimeError): capture.parse_tool_pins(bad)

    def test_private_environment_and_real_shim_exact_sequences(self):
        with tempfile.TemporaryDirectory() as tmp, patch.dict(os.environ, {"CUB_TOKEN": "secret", "HOME": "/ambient", "PATH": "/ambient", "HTTPS_PROXY": "https://foreign", "CUB_SCOUT_TEST_GITOPS_JSON": "hook"}):
            root = Path(tmp); shims = root / "shims"; shims.mkdir(); log = root / "calls"
            env = capture.observation_env(private_home=root / "home", shims=shims, kubeconfig=root / "config", cub_log=log)
            self.assertEqual(str(shims), env["PATH"]); self.assertEqual(str(root / "home"), env["HOME"])
            for name in ("CUB_TOKEN", "HTTPS_PROXY", "CUB_SCOUT_TEST_GITOPS_JSON", "CUB_PLUGIN", "KUBERNETES_SERVICE_HOST"):
                self.assertNotIn(name, env)
            shim = capture.create_observation_shims(shims)
            for action, expected in capture.CUB_SEQUENCES.items():
                size = log.stat().st_size if log.exists() else 0
                for row in expected:
                    actual = capture.run([str(shim), *row["argv"][1:]], env=env, timeout=5)
                    self.assertEqual(row["exitCode"], actual["exitCode"])
                capture.validate_cub_records(log, old_bytes=size, action=action)
            size = log.stat().st_size
            for argv in (["auth", "status", "--quiet"], ["auth", "get-token"], ["unit", "get", "token-secret"], ["unit", "data", capture.UNIT]):
                self.assertEqual(97, capture.run([str(shim), *argv], env=env, timeout=5)["exitCode"])
            self.assertNotIn("token-secret", log.read_text())
            with self.assertRaises(RuntimeError): capture.validate_cub_records(log, old_bytes=size, action="cli")

    def test_immutable_configs_all_distinct_bindings_and_ambient(self):
        proxies = [type("Proxy", (), {"label": label, "endpoint": f"http://127.0.0.1:{1234+i}"})() for i, label in enumerate((capture.ALLOWED, capture.SOURCE_DENIED, capture.PODS_DENIED))]
        with tempfile.TemporaryDirectory() as tmp:
            for selected in (capture.ALLOWED, capture.SOURCE_DENIED, capture.PODS_DENIED):
                config = capture._proxy_kubeconfig(selected, proxies)
                self.assertNotEqual(selected, config["current-context"])
                self.assertEqual({p.label for p in proxies}, {row["name"] for row in config["contexts"]})
                self.assertEqual({}, config["users"][0]["user"])
            path = Path(tmp) / "config"; capture.write_private(path, json.dumps(config), immutable=True)
            self.assertEqual(0o400, path.stat().st_mode & 0o777)
            before = capture.digest(path); capture.verify_immutable_configs({}, {path: before})
            path.chmod(0o600); path.write_text("changed")
            with self.assertRaises(RuntimeError): capture.verify_immutable_configs({}, {path: before})
        for bad in (proxies[:1], [proxies[0], proxies[0], proxies[2]]):
            with self.assertRaises(RuntimeError): capture._proxy_kubeconfig(capture.ALLOWED, bad)
        proxies[1].endpoint = proxies[0].endpoint
        with self.assertRaises(RuntimeError): capture._proxy_kubeconfig(capture.ALLOWED, proxies)

    def test_proxy_queries_mutations_and_foreign_upstreams_fail_closed(self):
        for path, query in capture.REQUESTS.items():
            self.assertTrue(capture.allowed_three_way_request(path, query))
            for bad in ({"watch": ["true"]}, {"labelSelector": ["foreign"]}, {**query, "timeout": ["32s"]}):
                self.assertFalse(capture.allowed_three_way_request(path, bad))
        for path in ("/version", "/api/v1/secrets", capture.APPLICATION_PATH, capture.DEPLOYMENT_PATH.replace(capture.NAMESPACE, "shared")):
            self.assertFalse(capture.allowed_three_way_request(path, {}))
        kwargs = dict(label=capture.ALLOWED, tls=ssl.create_default_context(), namespace=capture.NAMESPACE,
            deployment=capture.DEPLOYMENT, application=capture.APPLICATION, missing_deployment="unused")
        for upstream in ("https://foreign.example.invalid", "http://127.0.0.1:1"):
            with self.assertRaises(RuntimeError): capture.ThreeWayAPIProxy(upstream=upstream, **kwargs)
        proxy = capture.ThreeWayAPIProxy(upstream="https://127.0.0.1:1", **kwargs)
        try:
            for method, route in (("POST", capture.DEPLOYMENT_PATH), ("GET", capture.PODS_PATH + "?watch=true"), ("GET", "/api/v1/secrets")):
                conn = http.client.HTTPConnection("127.0.0.1", proxy.server.server_address[1], timeout=5)
                try:
                    conn.request(method, route); response = conn.getresponse(); self.assertEqual(403, response.status); response.read()
                finally: conn.close()
            self.assertEqual(3, len(proxy.snapshot()))
            self.assertTrue(all(row["disposition"] == "rejected" for row in proxy.snapshot()))
        finally: proxy.close()
        self.assertFalse(proxy.thread.is_alive())

    def test_fixture_and_rbac_are_minimal_exact_and_inert(self):
        fixture = capture._fixture_yaml()
        for text in ("kind: Deployment", "replicas: 0", "argocd.argoproj.io/instance: " + capture.APPLICATION,
                "confighub.com/UnitSlug: " + capture.UNIT, "confighub.com/SpaceName: " + capture.SPACE,
                "repoURL: https://example.invalid/owned-proof.git", "targetRevision: proof-revision", "path: proof/manifests"):
            self.assertIn(text, fixture)
        self.assertNotIn("modelplane", fixture.lower()); self.assertNotIn("kind: Pod", fixture)
        source = capture._rbac_yaml("source-denied", deny_source=True)
        pods = capture._rbac_yaml("pods-denied", deny_pods=True)
        self.assertNotIn('"applications"', source); self.assertIn('"pods"', source)
        self.assertIn('"applications"', pods); self.assertNotIn('"pods"', pods)
        for rbac in (source, pods):
            self.assertNotIn('"secrets"', rbac); self.assertNotIn('"*"', rbac); self.assertIn('verbs: ["get", "list"]', rbac)

    def test_mcp_transport_reuses_bounded_runner(self):
        with patch.object(capture._lifecycle, "run", return_value={"failure": "timeout"}) as run:
            result = capture._run_mcp("/private/scout", "{}\n", env={"PATH": "/private"}, deadline=123)
        self.assertEqual("timeout", result["failure"])
        self.assertEqual(b"{}\n", run.call_args.kwargs["input_data"])
        self.assertEqual(90, run.call_args.kwargs["timeout"])

    def test_failure_receipt_and_private_cleanup_after_exception(self):
        # All commands/proxies are mocked. Exercise failure finalization directly.
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            root = Path(tmp)
            shared = root / "integrity-only"
            shared.write_text("dummy integrity bytes")
            output = root / "receipt"
            args = argparse.Namespace(integrity_only_shared_kubeconfig=shared, output_dir=output, execute=True, helper_source="b" * 40, tool_pin=[name + "=" + "a" * 64 for name in ("git", "go", "kind", "kubectl", "docker")])
            for failure in (RuntimeError("offline injected failure"), KeyboardInterrupt()):
                if output.exists(): capture.shutil.rmtree(output)
                with patch.object(capture, "FIXED_SOURCE", "a" * 40), patch.object(capture.shutil, "which", return_value="/fake/tool"), patch.object(capture, "digest", side_effect=lambda path: "a" * 64 if str(path) == "/fake/tool" else capture._shared.digest(path)), \
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

    def test_final_snapshot_failure_cannot_skip_owned_cleanup(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            root = Path(tmp)
            shared = root / "integrity-only"
            shared.write_text("dummy integrity bytes")
            output = root / "receipt"
            args = argparse.Namespace(integrity_only_shared_kubeconfig=shared, output_dir=output, execute=True, helper_source="b" * 40, tool_pin=[name + "=" + "a" * 64 for name in ("git", "go", "kind", "kubectl", "docker")])
            with patch.object(capture, "FIXED_SOURCE", "a" * 40), patch.object(capture.shutil, "which", return_value="/fake/tool"), patch.object(capture, "digest", side_effect=lambda path: "a" * 64 if str(path) == "/fake/tool" else capture._shared.digest(path)), \
                 patch.object(capture, "record_command", side_effect=RuntimeError("offline stop")), \
                 patch.object(capture, "snapshot_proxies", side_effect=RuntimeError("offline snapshot failure")), \
                 patch.object(capture._lifecycle, "cleanup_cluster", return_value=[]) as cluster, \
                 patch.object(capture._lifecycle, "cleanup_worktrees", return_value=[]) as worktrees:
                with self.assertRaisesRegex(RuntimeError, "offline stop"):
                    capture._execute_capture(args, argparse.ArgumentParser())
            cluster.assert_called_once()
            worktrees.assert_called_once()
            receipt = json.loads((output / "receipt.json").read_text())
            self.assertEqual("failed", receipt["acceptance"])
            self.assertIn("final API snapshot failed", receipt["cleanupErrors"])
            self.assertTrue(receipt["privateDirectoryRemoved"])


    def test_fixture_storage_readiness_only_bounded_setup_get(self):
        transient = {"exitCode": 1, "failure": None, "stderr": "storage is (re)initializing", "stdout": ""}
        ready = {"exitCode": 0, "failure": None, "stdout": json.dumps({"kind": "ApplicationList", "items": []})}
        with patch.object(capture, "record_observation", side_effect=[transient, ready]) as observe, patch.object(capture.time, "sleep") as sleep:
            capture.wait_fixture_api_ready({}, kubectl="/private/kubectl", env={}, deadline=time.monotonic()+30)
        self.assertEqual(2, observe.call_count); sleep.assert_called_once_with(1)
        for call in observe.call_args_list:
            self.assertEqual(["/private/kubectl", "--context", capture.ALLOWED, "get", "--raw"], call.args[2][:5])
        for failure, count in ((transient, 5), ({**transient, "stderr": "Forbidden"}, 1)):
            with patch.object(capture, "record_observation", return_value=failure) as observe, patch.object(capture.time, "sleep"):
                with self.assertRaises(RuntimeError): capture.wait_fixture_api_ready({}, kubectl="/private/kubectl", env={}, deadline=time.monotonic()+30)
            self.assertEqual(count, observe.call_count)

    def test_wrong_helper_source_and_tool_pin_refuse_before_setup(self):
        for wrong_tool in (False, True):
            with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
                root = Path(tmp); shared = root / "integrity"; shared.write_text("synthetic integrity bytes")
                output = root / "receipt"
                args = argparse.Namespace(execute=True, integrity_only_shared_kubeconfig=shared, output_dir=output,
                    helper_source="b" * 40, tool_pin=[name+"="+"a"*64 for name in ("git", "go", "kind", "kubectl", "docker")])
                def command(receipt, phase, argv, **kwargs):
                    receipt["commands"].append({"phase": phase, "argv": argv})
                    return {"stdout": "" if phase == "source-clean" else "c"*40, "exitCode": 0, "failure": None}
                def digest(path):
                    return ("c" if wrong_tool else "a")*64 if str(path) == "/fake/tool" else capture._shared.digest(path)
                with patch.object(capture, "FIXED_SOURCE", "a"*40), patch.object(capture.shutil, "which", return_value="/fake/tool"), \
                     patch.object(capture, "digest", side_effect=digest), patch.object(capture, "record_command", side_effect=command) as run:
                    with self.assertRaisesRegex(RuntimeError, "sha256 pin" if wrong_tool else "helper source"):
                        capture._execute_capture(args, argparse.ArgumentParser())
                self.assertEqual(0 if wrong_tool else 2, run.call_count)
                receipt = json.loads((output / "receipt.json").read_text())
                self.assertEqual("failed", receipt["acceptance"])
                self.assertFalse(receipt["clusterCreationAttempted"])
                self.assertTrue(receipt["privateDirectoryRemoved"])

    def test_reuses_owned_uncertain_creation_and_deadline_cleanup(self):
        receipt = {"commands": []}
        with patch.object(capture._lifecycle, "run", return_value={"exitCode": 0, "stdout": "partial-node", "failure": None}) as run:
            errors = capture._lifecycle.cleanup_cluster(receipt, name="private-owned", created=False, creation_attempted=True,
                tools={"docker": "/fake/docker", "kind": "/fake/kind"}, env={}, deadline=123)
        self.assertTrue(errors); self.assertNotIn("delete", run.call_args.args[0])
        with tempfile.TemporaryDirectory() as tmp, patch.object(capture._lifecycle, "run", return_value={"exitCode": 0, "stdout": "", "failure": None}) as run:
            capture.cleanup_worktrees_bounded({"commands": []}, repo=Path(tmp), git="/fake/git", worktrees=[Path(tmp)], env={}, deadline=123)
            self.assertIs(capture._lifecycle.run, run)
        self.assertEqual(123, run.call_args.kwargs["deadline"])


if __name__ == "__main__":
    unittest.main()
