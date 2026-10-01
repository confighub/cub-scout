"""Offline acceptance and lifecycle controls; no Kubernetes/container tools."""
import importlib.util
import http.client
import json
import os
from pathlib import Path
import shlex
import ssl
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("trace_context_live_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capture)
proxy_spec = importlib.util.spec_from_file_location("trace_context_live_api_proxy", Path(__file__).with_name("api_proxy.py"))
api_proxy = importlib.util.module_from_spec(proxy_spec)
proxy_spec.loader.exec_module(api_proxy)


def record(phase, command, body=None, *, exit_code=0, stderr="", failure=None):
    return {"phase": phase, "command": command, "exitCode": exit_code,
            "stdout": json.dumps(body) if body is not None else "", "stderr": stderr,
            "failure": failure}


def observations():
    source = "https://example.invalid/owned-fixture.git"
    result = []
    for command in ("normal", "reverse"):
        old_body = ({"target": {"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT},
                     "summary": {"ownerType": "ArgoCD", "source": {"url": source}}, "chain": [{"id": {"kind": "Source", "name": "owned-fixture"},
                                                                                                      "role": "source"}]}
                    if command == "normal" else
                    {"object": {"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT},
                     "owner": "argo", "k8sChain": [{"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT}]})
        result.append(record("old-ambient-" + command, command, old_body))
        result.append(record("old-explicit-" + command, command, exit_code=1,
                             stderr="unknown flag: --kube-context"))
        fixed_body = dict(old_body)
        fixed_body["context"] = "doctor-allowed"
        result.append(record("fixed-allowed-" + command, command, fixed_body))
        denied_error = f'deployments.apps is forbidden: User "{capture.DENIED_USER}" cannot get resource'
        if command == "normal":
            result.append(record("fixed-denied-normal", command, exit_code=1, stderr=denied_error))
        else:
            result.append(record("fixed-denied-reverse", command,
                                 {"object": {"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT},
                                  "owner": "", "error": "failed to get resource: " + denied_error,
                                  "context": "doctor-denied"}))
    native_target = {"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.NATIVE_DEPLOYMENT}
    result.append(record("fixed-allowed-native-reverse", "reverse",
                         {"object": native_target, "owner": "native", "context": "doctor-allowed"}))
    native_error = f'deployments.apps is forbidden: User "{capture.DENIED_USER}" cannot get resource'
    result.append(record("fixed-denied-native-reverse", "reverse",
                         {"object": native_target, "owner": "", "error": native_error,
                          "context": "doctor-denied"}))
    return result


class ObservationAcceptanceTests(unittest.TestCase):
    def test_accepts_complete_paired_observations(self):
        capture.validate_observations(observations())

    def test_rejects_native_or_clean_output_as_denial(self):
        rows = observations()
        rows[7]["stdout"] = json.dumps({"object": {"kind": "Deployment", "namespace": capture.NAMESPACE,
                                                     "name": capture.DEPLOYMENT},
                                        "owner": "native", "context": "doctor-denied"})
        with self.assertRaisesRegex(RuntimeError, "reverse Trace denial"):
            capture.validate_observations(rows)

    def test_rejects_clean_native_normal_trace_as_denial(self):
        rows = observations()
        rows[3].update(exitCode=0, stderr="", stdout=json.dumps({"context": "doctor-denied",
            "target": {"kind": "Deployment", "namespace": capture.NAMESPACE, "name": capture.DEPLOYMENT},
            "summary": {"ownerType": "Native"}}))
        with self.assertRaisesRegex(RuntimeError, "normal Trace denial"):
            capture.validate_observations(rows)

    def test_rejects_wrong_context_target_source_or_missing_pair(self):
        for mutate in (
            lambda rows: rows[6].update(stdout=json.dumps({"target": {"kind": "Deployment", "namespace": capture.NAMESPACE,
                                                                      "name": "other"}, "context": "doctor-allowed"})),
            lambda rows: rows[4].update(stdout=json.dumps({"target": {"kind": "Deployment", "namespace": capture.NAMESPACE,
                                                                      "name": capture.DEPLOYMENT}, "summary": {"ownerType": "ArgoCD"},
                                                       "context": "ambient", "chain": []})),
            lambda rows: rows.pop(),
        ):
            rows = observations()
            mutate(rows)
            with self.assertRaises(RuntimeError):
                capture.validate_observations(rows)


class TUIAcceptanceTests(unittest.TestCase):
    def valid(self):
        target = "/apis/apps/v1/namespaces/" + capture.NAMESPACE + "/deployments/" + capture.DEPLOYMENT
        return {"schema": capture.TUI_SCHEMA, "passed": True,
                "checks": {"allowed-open": True, "allowed-reopen-after-retarget": True,
                           "denied-open": True, "private-config-retarget-stable": True},
                "requests": {"allowed-open": [{"method": "GET", "path": target, "status": 200}],
                             "allowed-reopen-after-retarget": [{"method": "GET", "path": target, "status": 200}],
                             "denied-open": [{"method": "GET", "path": target, "status": 403}]},
                "selectedContexts": {"allowed-open": "doctor-allowed",
                                     "allowed-reopen-after-retarget": "doctor-allowed",
                                     "denied-open": "doctor-denied"},
                "views": {"allowed-open": "Kubernetes context: doctor-allowed scout-context-marker",
                          "allowed-reopen-after-retarget": "Kubernetes context: doctor-allowed scout-context-marker",
                          "denied-open": "Error: forbidden User " + capture.DENIED_USER}}

    def test_accepts_actual_read_only_binding_evidence(self):
        capture.validate_tui(self.valid())

    def test_rejects_non_get_and_false_denial(self):
        for mutate in (
            lambda data: data["requests"]["allowed-open"].append({"method": "POST", "path": "/anything", "status": 200}),
            lambda data: data["requests"]["denied-open"].__setitem__(0, {"method": "GET", "path": "/api/v1/pods", "status": 200}),
            lambda data: data["views"].update({"denied-open": "Kubernetes context: doctor-denied Native"}),
            lambda data: data["checks"].update({"allowed-reopen-after-retarget": False}),
        ):
            data = self.valid()
            mutate(data)
            with self.assertRaises(RuntimeError):
                capture.validate_tui(data)


class RunnerAndCleanupTests(unittest.TestCase):
    def test_timeout_keeps_partial_output_and_stops_process_group(self):
        result = capture.run([sys.executable, "-c", "import time; print('partial', flush=True); time.sleep(5)"],
                             env={"PATH": os.defpath}, timeout=0.2)
        self.assertIn("partial", result["stdout"])
        self.assertIn("deadline", result["failure"])
        self.assertLess(result["elapsedSeconds"], 2)

    def test_output_limit_fails_closed(self):
        result = capture.run([sys.executable, "-c", "print('x' * 10000)"], env={"PATH": os.defpath}, max_output=32)
        self.assertEqual("x" * 32, result["stdout"])
        self.assertIn("output exceeded", result["failure"])

    def test_token_command_receipt_is_redacted(self):
        receipt = {"commands": []}
        result = capture.record_command(receipt, "token", [sys.executable, "-c", "print('private-token')"],
                                        env={"PATH": os.defpath}, deadline=time.monotonic() + 5, secret=True)
        self.assertIn("private-token", result["stdout"])
        self.assertNotIn("private-token", json.dumps(receipt))

    def test_uncertain_create_never_authorizes_delete(self):
        calls = []
        def fake(argv, **_kwargs):
            calls.append(argv)
            return {"argv": argv, "exitCode": 0, "stdout": "node-id\n", "stderr": "", "failure": None,
                    "elapsedSeconds": 0}
        receipt = {"commands": []}
        with patch.object(capture, "run", side_effect=fake):
            errors = capture.cleanup_cluster(receipt, name="scout-proof-owned", created=False, creation_attempted=True,
                                             tools={"docker": "docker", "kind": "kind"}, env={}, deadline=time.monotonic() + 5)
        self.assertTrue(errors)
        self.assertFalse(any("delete" in argv for argv in calls))
        self.assertIn("ownership review", errors[0])

    def test_precluster_build_failure_cleanup_removes_only_created_worktree(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp) / "repo"
            checkout = Path(tmp) / "source-fixed"
            repo.mkdir()
            checkout.mkdir()
            calls = []
            def fake_run(argv, **_kwargs):
                calls.append(argv)
                self.assertEqual("worktree", argv[3])
                self.assertEqual("remove", argv[4])
                self.assertEqual(str(checkout), argv[-1])
                checkout.rmdir()
                return {"argv": argv, "exitCode": 0, "stdout": "", "stderr": "", "failure": None,
                        "elapsedSeconds": 0}
            receipt = {"commands": []}
            with patch.object(capture, "run", side_effect=fake_run):
                errors = capture.cleanup_worktrees(receipt, repo=repo, git="git", worktrees=[checkout], env={})
            self.assertEqual([], errors)
            self.assertEqual(1, len(calls))
            self.assertFalse(checkout.exists())

    def test_main_cleans_worktrees_and_credentials_after_fixed_build_failure(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            root = Path(tmp)
            shared = root / "shared.kubeconfig"
            shared.write_text("opaque shared config\n")
            shared_before = capture.digest(shared)
            output = Path(tmp) / "receipt"
            fake_tools = {}
            for name in ("git", "go", "kind", "kubectl", "docker"):
                path = root / name
                path.write_text("not executed\n")
                path.chmod(0o700)
                fake_tools[name] = str(path)
            created_worktrees = []
            calls = []

            def fake_run(argv, **kwargs):
                argv = [str(item) for item in argv]
                calls.append(argv)
                stdout, exit_code, stderr = "", 0, ""
                if "status" in argv:
                    stdout = ""
                elif "merge-base" in argv:
                    pass
                elif argv[1:3] == ["version"] or (len(argv) > 1 and argv[1] == "version"):
                    stdout = "kind " + capture.KIND_VERSION + "\n"
                elif "context" in argv and "show" in argv:
                    stdout = "default\n"
                elif "context" in argv and "inspect" in argv:
                    stdout = json.dumps("unix:///tmp/owned-docker.sock")
                elif "image" in argv and "inspect" in argv:
                    stdout = json.dumps([{"RepoDigests": ["kindest/node@" + capture.NODE_IMAGE.rsplit("@", 1)[1]]}])
                elif "worktree" in argv and "add" in argv:
                    checkout = Path(argv[-2])
                    (checkout / "cmd" / "cub-scout").mkdir(parents=True)
                    created_worktrees.append(checkout)
                elif "rev-parse" in argv:
                    stdout = capture.OLD_SOURCE + "\n"
                elif "build" in argv:
                    if "source-old" in str(argv):
                        binary = Path(argv[argv.index("-o") + 1])
                        binary.write_bytes(b"synthetic old binary")
                    else:
                        exit_code, stderr = 9, "synthetic fixed build failure"
                elif "worktree" in argv and "remove" in argv:
                    checkout = Path(argv[-1])
                    import shutil as shutil_module
                    shutil_module.rmtree(checkout)
                else:
                    self.fail("unexpected command in pre-cluster failure test: " + repr(argv))
                return {"argv": argv, "exitCode": exit_code, "stdout": stdout, "stderr": stderr,
                        "failure": None, "elapsedSeconds": 0}

            with patch.object(capture.shutil, "which", side_effect=lambda name: fake_tools[name]), \
                 patch.object(capture, "run", side_effect=fake_run), \
                 patch.object(sys, "argv", ["capture.py", "--execute",
                     "--integrity-only-shared-kubeconfig", str(shared), "--output-dir", str(output)]):
                with self.assertRaisesRegex(RuntimeError, "command failed in phase fixed-build"):
                    capture.main()

            receipt = json.loads((output / "receipt.json").read_text())
            self.assertEqual("failed", receipt["acceptance"])
            self.assertFalse(receipt["clusterCreationAttempted"])
            self.assertTrue(receipt["sharedKubeconfigUnchanged"])
            self.assertEqual(shared_before, capture.digest(shared))
            self.assertEqual(2, len(created_worktrees))
            self.assertTrue(all(not path.exists() for path in created_worktrees))
            self.assertEqual(2, sum("worktree" in argv and "remove" in argv for argv in calls))
            self.assertFalse(any("create" in argv and "cluster" in argv for argv in calls))


class IsolationHelperTests(unittest.TestCase):
    def test_fixed_phase_names_are_exact_and_reject_unknown_values(self):
        self.assertEqual("fixed-allowed-normal", capture.fixed_phase(capture.ALLOWED_CONTEXT, "normal"))
        self.assertEqual("fixed-denied-reverse", capture.fixed_phase(capture.DENIED_CONTEXT, "reverse"))
        for context, command in (("ambient", "normal"), (capture.ALLOWED_CONTEXT, "diff")):
            with self.assertRaises(ValueError):
                capture.fixed_phase(context, command)

    def test_observation_environment_is_private_and_minimal(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = capture.observation_env({"PATH": "/ambient", "HOME": "/ambient-home", "TOKEN": "x"},
                private_home=root / "home", shims=root / "shims", kubeconfig=root / "private.kubeconfig")
            self.assertEqual(str(root / "shims"), env["PATH"])
            self.assertEqual(str(root / "home"), env["HOME"])
            self.assertEqual(str(root / "home" / ".config"), env["XDG_CONFIG_HOME"])
            self.assertEqual(str(root / "home" / ".cache"), env["XDG_CACHE_HOME"])
            self.assertEqual(str(root / "home" / ".local" / "share"), env["XDG_DATA_HOME"])
            self.assertEqual(str(root / "private.kubeconfig"), env["KUBECONFIG"])
            self.assertEqual("x", env["TOKEN"])

    def test_argo_shim_allows_only_availability_and_forces_owned_fallback(self):
        with tempfile.TemporaryDirectory() as tmp:
            shims = Path(tmp)
            paths = capture.create_observation_shims(shims, "/not/invoked/kubectl")
            for args, expected in ((["version", "--client"], 0),
                                   (["app", "get", "fixture", "-o", "json"], 1),
                                   (["login", "remote.invalid"], 1)):
                result = capture.run([str(paths["argocd"]), *args], env={"PATH": str(shims)}, timeout=2)
                self.assertEqual(expected, result["exitCode"])
                if expected:
                    self.assertIn("server address unspecified", result["stderr"])

    def test_shims_quote_kubectl_and_deny_unowned_tools(self):
        with tempfile.TemporaryDirectory() as tmp:
            shims = Path(tmp) / "private shims"
            shims.mkdir()
            paths = capture.create_observation_shims(shims, "/opt/kubectl with spaces/kubectl")
            text = paths["kubectl"].read_text()
            self.assertIn("exec " + shlex.quote("/opt/kubectl with spaces/kubectl"), text)
            self.assertIn("refusing unexpected owned kubectl operation", text)
            for name in ("flux", "helm", "cub"):
                self.assertIn("blocked", paths[name].read_text())
            self.assertIn("server address unspecified", paths["argocd"].read_text())


class CombinedProofAcceptanceTests(unittest.TestCase):
    def test_api_allowlist_is_exact_and_read_only(self):
        proxy = object.__new__(api_proxy.ReadOnlyAPIProxy)
        proxy.namespace = "team-a"
        proxy.deployment = "api"
        proxy.application = "api-app"
        proxy.missing_deployment = "missing"
        self.assertTrue(proxy.allowed_path("/apis/apps/v1/namespaces/team-a/deployments/api", {}))
        self.assertTrue(proxy.allowed_path("/apis/argoproj.io/v1alpha1/applications",
                                           {"fieldSelector": ["metadata.name=api-app"]}))
        self.assertFalse(proxy.allowed_path("/api/v1/namespaces/team-a/secrets/x", {}))
        self.assertFalse(proxy.allowed_path("/apis/argoproj.io/v1alpha1/applications",
                                            {"fieldSelector": ["metadata.name=other"]}))
        self.assertFalse(proxy.allowed_path("/apis/apps/v1/namespaces/team-a/deployments/api", {"watch": ["true"]}))

    def test_arbitrary_http_method_is_recorded_and_rejected(self):
        proxy = api_proxy.ReadOnlyAPIProxy(label="allowed", upstream="https://127.0.0.1:1",
            tls=ssl.create_default_context(), namespace="team-a", deployment="api",
            application="api-app", missing_deployment="missing")
        try:
            host, port = proxy.server.server_address
            conn = http.client.HTTPConnection(host, port, timeout=2)
            conn.request("PROPFIND", "/apis/apps/v1/namespaces/team-a/deployments/api")
            response = conn.getresponse()
            self.assertEqual(403, response.status)
            response.read()
            conn.close()
            self.assertEqual([{"endpoint": "allowed", "method": "PROPFIND",
                               "path": "/apis/apps/v1/namespaces/team-a/deployments/api",
                               "status": 403, "disposition": "rejected"}], proxy.snapshot())
        finally:
            proxy.close()

    def test_observation_config_contains_no_upstream_credentials(self):
        config = {"current-context": "doctor-allowed", "clusters": [{"name": "kind", "cluster": {"server": "https://kind", "certificate-authority-data": "secret"}}],
                  "users": [{"name": "allowed", "user": {"client-certificate-data": "private"}},
                            {"name": "denied", "user": {"token": "private"}}],
                  "contexts": [{"name": "doctor-allowed", "context": {"cluster": "kind", "user": "allowed"}},
                               {"name": "doctor-denied", "context": {"cluster": "kind", "user": "denied"}}]}
        projected = api_proxy.observation_kubeconfig(config, allowed_endpoint="http://127.0.0.1:1",
            denied_endpoint="http://127.0.0.1:2", allowed_cluster_name="proof-allowed",
            denied_cluster_name="proof-denied", allowed_context="doctor-allowed",
            denied_context="doctor-denied", namespace="team-a")
        encoded = json.dumps(projected)
        self.assertNotIn("certificate-authority-data", encoded)
        self.assertNotIn("client-certificate-data", encoded)
        self.assertNotIn("token", encoded)
        self.assertEqual("http://127.0.0.1:1", projected["clusters"][0]["cluster"]["server"])
        self.assertEqual("proof-denied", projected["contexts"][1]["context"]["cluster"])

    def test_api_receipt_rejects_wrong_endpoint_identity_missing_read_and_write(self):
        valid = [{"endpoint": "allowed", "method": "GET",
                  "path": "/apis/apps/v1/namespaces/team-a/deployments/api", "status": 200,
                  "disposition": "forwarded"}]
        api_proxy.validate_api_records(valid, endpoint="allowed", target_path=valid[0]["path"], target_status=200)
        for rows in (
            [dict(valid[0], endpoint="denied")],
            [dict(valid[0], path="/apis/apps/v1/namespaces/other/deployments/api")],
            [dict(valid[0], method="PATCH")],
            [dict(valid[0], status=403)],
            [],
        ):
            with self.assertRaises(RuntimeError):
                api_proxy.validate_api_records(rows, endpoint="allowed", target_path=valid[0]["path"], target_status=200)

    def test_event_log_requires_exact_phase_order_and_assigned_requests(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "events.jsonl"
            rows = [
                {"type": "phase", "phase": "cli-read"},
                {"type": "request", "endpoint": "allowed", "method": "GET",
                 "path": "/apis/apps/v1/namespaces/team-a/deployments/api", "status": 200,
                 "disposition": "forwarded"},
                {"type": "phase", "phase": "mcp-read"},
                {"type": "request", "endpoint": "denied", "method": "GET",
                 "path": "/apis/apps/v1/namespaces/team-a/deployments/api", "status": 403,
                 "disposition": "forwarded"},
            ]
            path.write_text("".join(json.dumps(row) + "\n" for row in rows))
            grouped = api_proxy.group_action_events(path, ["cli-read", "mcp-read"])
            self.assertEqual(["allowed"], [row["endpoint"] for row in grouped["cli-read"]])
            self.assertEqual([403], [row["status"] for row in grouped["mcp-read"]])
            for changed, phases in ((rows[1:], ["cli-read", "mcp-read"]),
                                    (rows, ["mcp-read", "cli-read"])):
                path.write_text("".join(json.dumps(row) + "\n" for row in changed))
                with self.assertRaises(RuntimeError):
                    api_proxy.group_action_events(path, phases)
            orphan = [{"type": "request", "endpoint": "allowed", "method": "GET",
                       "path": "/api/v1", "status": 200}]
            path.write_text(json.dumps(orphan[0]) + "\n")
            with self.assertRaises(RuntimeError):
                api_proxy.group_action_events(path, ["cli-read"])

    def test_actual_mcp_tools_call_wrapper_requires_structured_non_pass_evidence(self):
        source_truth = {"context": "doctor-allowed", "status": "BLOCK", "source_truth": "BLOCKED",
                        "collection_errors": ["ConfigHub: recorded unit lookup unavailable in owned proof"]}
        for result in (
            {"structuredContent": source_truth, "isError": False},
            {"content": [{"type": "text", "text": json.dumps(source_truth)}], "isError": False},
        ):
            response = {"jsonrpc": "2.0", "id": 2, "result": result}
            body = api_proxy.mcp_result_json(response)
            api_proxy.validate_source_truth(body, context="doctor-allowed")
        for response in (
            {"jsonrpc": "2.0", "id": 2, "error": {"code": -32000}},
            {"jsonrpc": "2.0", "id": 2, "result": {"isError": True, "content": []}},
        ):
            with self.assertRaises(RuntimeError):
                api_proxy.mcp_result_json(response)
        fake_pass = dict(source_truth, status="PASS", source_truth="MATCH")
        with self.assertRaisesRegex(RuntimeError, "clean source-truth"):
            api_proxy.validate_source_truth(fake_pass, context="doctor-allowed")

    def test_diff_status_validation_keeps_exact_target_and_observation(self):
        def body(status, name="api", context="doctor-allowed"):
            return {"status": status, "context": context,
                    "resource": {"apiVersion": "apps/v1", "kind": "Deployment",
                                 "namespace": "team-a", "name": name},
                    "read": {"reads": {"discovery": 1, "object": 1}}}
        for status in ("matched", "changed", "missing", "inconclusive"):
            api_proxy.validate_diff(body(status), context="doctor-allowed", status=status,
                                    namespace="team-a", name="api")
        with self.assertRaises(RuntimeError):
            api_proxy.validate_diff(body("missing", name="other"), context="doctor-allowed",
                                    status="missing", namespace="team-a", name="api")
        with self.assertRaises(RuntimeError):
            api_proxy.validate_diff(body("inconclusive", context="doctor-denied"), context="doctor-allowed",
                                    status="inconclusive", namespace="team-a", name="api")

    def test_exact_config_hub_shim_contract_fails_unit_read_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            shims = Path(tmp)
            paths = capture.create_observation_shims(shims, "/not/invoked/kubectl", combined=True)
            log = Path(tmp) / "cub-argv.jsonl"
            env = {"PATH": str(shims), "SCOUT_TRACE_CUB_LOG": str(log)}
            auth = capture.run([str(paths["cub"]), "auth", "status"], env=env, timeout=2)
            self.assertEqual(0, auth["exitCode"])
            unit = capture.run([str(paths["cub"]), "unit", "get", capture.CONFIGHUB_UNIT,
                                "-o", "json", "--space", capture.CONFIGHUB_SPACE], env=env, timeout=2)
            self.assertEqual(73, unit["exitCode"])
            self.assertIn("recorded unit lookup unavailable in owned proof", unit["stderr"])
            denied = capture.run([str(paths["cub"]), "unit", "list"], env=env, timeout=2)
            self.assertEqual(97, denied["exitCode"])
            rows = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual([0, 73], [row["exitCode"] for row in rows])
            self.assertNotIn("list", log.read_text())

    def test_empty_combined_source_pin_is_explicitly_unrunnable(self):
        with patch.object(capture, "COMBINED_SOURCE_PIN", ""):
            with self.assertRaisesRegex(RuntimeError, "integrated source pin has not been selected"):
                capture.require_combined_source_pin()
        with patch.object(capture, "COMBINED_SOURCE_PIN", "d" * 40):
            self.assertEqual("d" * 40, capture.require_combined_source_pin())

    def test_combined_mode_pin_gate_precedes_output_creation(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as tmp:
            root = Path(tmp)
            shared = root / "opaque.kubeconfig"
            shared.write_text("not parsed by helper\n")
            output = root / "must-not-exist"
            with patch.object(capture, "COMBINED_SOURCE_PIN", ""), \
                 patch.object(sys, "argv", ["capture.py", "--mode", "source-truth-diff", "--execute",
                     "--integrity-only-shared-kubeconfig", str(shared), "--output-dir", str(output)]):
                with self.assertRaisesRegex(RuntimeError, "integrated source pin has not been selected"):
                    capture.main()
            self.assertFalse(output.exists())

    def test_combined_fixture_has_exact_source_truth_identity_and_local_inputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            fixture = capture._write_fixture(work, source_truth=True)
            text = fixture.read_text()
            self.assertIn("confighub.com/UnitSlug: " + capture.CONFIGHUB_UNIT, text)
            self.assertIn("confighub.com/SpaceName: " + capture.CONFIGHUB_SPACE, text)
            self.assertIn("revision: " + "a" * 40, text)
            rendered = capture._write_rendered_inputs(work)
            self.assertEqual({"matched", "changed", "missing"}, set(rendered))
            self.assertIn("replicas: 0", rendered["matched"].read_text())
            self.assertIn("replicas: 1", rendered["changed"].read_text())
            self.assertIn("name: scout-context-missing", rendered["missing"].read_text())


if __name__ == "__main__":
    unittest.main()
