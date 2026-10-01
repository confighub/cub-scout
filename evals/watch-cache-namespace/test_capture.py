"""Pure offline tests for the opt-in #735 owned-kind proof harness."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("watch_namespace_capture", HERE / "capture.py")
capture = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(capture)


class WatchNamespaceCaptureTests(unittest.TestCase):
    def fixture_result(self, variant):
        ns_a, ns_b, denied = "scout-watch-ns-a", "scout-watch-ns-b", "scout-watch-ns-denied"
        paths = {
            "a": f"/api/v1/namespaces/{ns_a}/configmaps",
            "b": f"/api/v1/namespaces/{ns_b}/configmaps",
            "denied": f"/api/v1/namespaces/{denied}/configmaps",
            "all": "/api/v1/configmaps",
            "nodes": f"/api/v1/namespaces/{ns_a}/nodes",
        }
        record = lambda path, status, watch=False: {
            "method": "GET", "path": path, "watch": watch, "status": status,
        }
        direct = [record(paths["a"], 200), record(paths["b"], 200), record(paths["all"], 403),
                  record(paths["denied"], 403), record(paths["nodes"], 404)]
        if variant == "before":
            watched = [record(paths["a"], 200), record(paths["a"], 200, True)]
            mismatches = sorted(capture.EXPECTED_OLD_MISMATCHES)
            checks = {"sameNameDifferentUID": True, "teamAExactCacheHit": True}
            watch = {"teamA": [{"namespace": ns_a, "name": "same-name", "uid": "uid-a"}],
                     "teamB": [], "teamAError": "success", "teamBError": "success",
                     "allError": "success", "deniedError": "success",
                     "clusterScopedNamespacedError": "success"}
        else:
            watched = [record(paths["a"], 200), record(paths["a"], 200, True), record(paths["b"], 200),
                       record(paths["all"], 403), record(paths["denied"], 403), record(paths["nodes"], 404)]
            mismatches = []
            checks = {name: True for name in capture.EXPECTED_NEW_CHECKS}
            watch = {"teamA": [{"namespace": ns_a, "name": "same-name", "uid": "uid-a"}],
                     "teamB": [{"namespace": ns_b, "name": "same-name", "uid": "uid-b"}],
                     "teamAError": "success", "teamBError": "success",
                     "allError": "Forbidden", "deniedError": "Forbidden",
                     "clusterScopedNamespacedError": "NotFound"}
        return {
            "schema": "watch-cache-namespace-live-probe.v1", "variant": variant,
            "namespaceA": ns_a, "namespaceB": ns_b, "deniedNamespace": denied,
            "configMapName": "same-name",
            "directRequests": direct, "watchRequests": watched,
            "mismatches": mismatches, "checks": checks,
            "direct": {"teamA": [{"namespace": ns_a, "name": "same-name", "uid": "uid-a"}],
                       "teamAError": "success",
                       "teamB": [{"namespace": ns_b, "name": "same-name", "uid": "uid-b"}],
                       "teamBError": "success", "allError": "Forbidden", "deniedError": "Forbidden",
                       "clusterScopedNamespacedError": "NotFound"},
            "watch": watch,
        }

    def test_old_and_new_acceptance_shapes_require_their_distinct_evidence(self):
        before = self.fixture_result("before")
        after = self.fixture_result("after")
        accepted_before = capture.validate_probe_result("before", 1, before)
        accepted_after = capture.validate_probe_result("after", 0, after)
        self.assertEqual(set(accepted_before["mismatches"]), capture.EXPECTED_OLD_MISMATCHES)
        self.assertEqual(accepted_after["mismatches"], [])

    def test_old_probe_rejects_missing_expected_false_empty_evidence(self):
        result = self.fixture_result("before")
        result["mismatches"].remove("all-namespace-fallback-and-denial")
        with self.assertRaisesRegex(capture.CaptureError, "exactly the expected"):
            capture.validate_probe_result("before", 1, result)
        result = self.fixture_result("before")
        result["mismatches"].append("unexpected-infrastructure-failure")
        with self.assertRaisesRegex(capture.CaptureError, "exactly the expected"):
            capture.validate_probe_result("before", 1, result)

    def test_fixed_probe_rejects_error_exit_unknown_checks_and_wrong_http_status(self):
        result = self.fixture_result("after")
        with self.assertRaisesRegex(capture.CaptureError, "pass every"):
            capture.validate_probe_result("after", 1, result)
        result = self.fixture_result("after")
        result["checks"]["allNamespaceFallbackPreservesDenial"] = False
        with self.assertRaisesRegex(capture.CaptureError, "pass every"):
            capture.validate_probe_result("after", 0, result)
        result = self.fixture_result("after")
        result["watchRequests"] = [r for r in result["watchRequests"] if r["path"] != "/api/v1/configmaps"]
        with self.assertRaisesRegex(capture.CaptureError, "expected fallback status"):
            capture.validate_probe_result("after", 0, result)

    def test_both_variants_require_direct_status_and_cluster_scope_control(self):
        result = self.fixture_result("after")
        result["directRequests"] = [r for r in result["directRequests"] if not r["path"].endswith("/nodes")]
        with self.assertRaisesRegex(capture.CaptureError, "all five exact API paths"):
            capture.validate_probe_result("after", 0, result)
        result = self.fixture_result("after")
        result["directRequests"][-1]["status"] = 500
        with self.assertRaisesRegex(capture.CaptureError, "API error"):
            capture.validate_probe_result("after", 0, result)

    def test_cluster_scope_control_preserves_an_authorization_denial(self):
        result = self.fixture_result("after")
        result["directRequests"][-1]["status"] = 403
        result["watchRequests"][-1]["status"] = 403
        result["direct"]["clusterScopedNamespacedError"] = "Forbidden"
        result["watch"]["clusterScopedNamespacedError"] = "Forbidden"
        self.assertTrue(capture.validate_probe_result("after", 0, result)["checks"])

    def test_validator_rejects_malformed_or_aliased_scope_identity(self):
        result = self.fixture_result("after")
        del result["namespaceA"]
        with self.assertRaisesRegex(capture.CaptureError, "scope identity"):
            capture.validate_probe_result("after", 0, result)
        result = self.fixture_result("after")
        result["deniedNamespace"] = result["namespaceA"]
        with self.assertRaisesRegex(capture.CaptureError, "not distinct"):
            capture.validate_probe_result("after", 0, result)

    def test_old_adapter_changes_only_constructor_and_probe_scope_injection(self):
        source = (HERE / "observation_watch_live_test.go.txt").read_bytes()
        adapted = capture._old_adapter(source)
        self.assertEqual(adapted.count(b"newWatchBackedClient(ctx, watched,"), 1)
        self.assertIn(b"[]schema.GroupVersionResource{configMapGVR}, namespaceA)", adapted)
        self.assertNotIn(b"resourceScope", adapted)
        self.assertIn(b"func cacheableList", (capture.REPO / "cmd/cub-scout/observation_watch.go").read_bytes())

    def test_probe_commands_are_bound_to_each_absolute_source_worktree(self):
        before = Path("/tmp/probe-before").resolve()
        after = Path("/tmp/probe-after").resolve()
        before_argv = capture._go_worktree_argv(before, ["test", "./cmd/cub-scout", "-run", "^$"])
        after_argv = capture._go_worktree_argv(after, ["test", "./cmd/cub-scout", "-run", "^$"])
        self.assertEqual(before_argv[:3], ["go", "-C", str(before)])
        self.assertEqual(after_argv[:3], ["go", "-C", str(after)])
        self.assertNotEqual(before_argv, after_argv)

    def test_compile_and_probe_pass_source_worktree_in_go_argv(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            tree = root / "source"
            tree.mkdir()
            empty = root / "empty.kubeconfig"
            empty.write_bytes(b"")
            with mock.patch.object(capture, "_run", return_value=(0, b"", b"")) as run:
                capture._compile_probe(tree, "before", capture.time.monotonic() + 10,
                                       {"PATH": "/safe", "KUBECONFIG": "/shared"},
                                       root / "cache", empty)
                compile_args, compile_kwargs = run.call_args
                self.assertEqual(compile_args[0][:3], ["go", "-C", str(tree.resolve())])
                self.assertEqual(compile_kwargs["env"]["KUBECONFIG"], str(empty))
                self.assertEqual(compile_kwargs["env"]["GOPROXY"], "off")

                run.reset_mock()
                capture._run_probe(tree, "after", root / "observer.kubeconfig", "kind-own",
                                   ("a", "b", "denied"), "same-name", root / "missing.json",
                                   capture.time.monotonic() + 10, {"PATH": "/safe"}, root / "cache")
                probe_args, probe_kwargs = run.call_args
                self.assertEqual(probe_args[0][:3], ["go", "-C", str(tree.resolve())])
                self.assertEqual(probe_kwargs["env"]["KUBECONFIG"], str(root / "observer.kubeconfig"))

    def test_compile_env_uses_an_explicit_empty_private_kubeconfig(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            config = root / "empty.kubeconfig"
            config.write_bytes(b"")
            env = capture._compile_environment({"PATH": "/safe/bin", "KUBECONFIG": "/shared/config"},
                                               root / "go-cache", config)
            self.assertEqual(env["KUBECONFIG"], str(config))
            self.assertEqual(config.stat().st_size, 0)
            self.assertEqual(env["GOPROXY"], "off")
            self.assertEqual(env["GOSUMDB"], "off")

    def test_private_observer_config_rejects_non_loopback_endpoint(self):
        with self.assertRaisesRegex(capture.CaptureError, "non-loopback"):
            capture._observer_config("https://cluster.example.invalid:6443", "ZmFrZQ==", "fake", "kind-own", "a")
        self.assertIn(b"current-context", capture._observer_config(
            "https://127.0.0.1:6443", "ZmFrZQ==", "fake-token", "kind-owned", "namespace-a"))

    def test_fixture_is_least_privileged_and_never_creates_cluster_role(self):
        documents = [json.loads(part) for part in capture._fixture_manifests(
            ("team-a", "team-b", "denied"), "same-name").decode().split("\n---\n") if part.strip()]
        self.assertEqual([d["metadata"]["name"] for d in documents if d["kind"] == "Namespace"],
                         ["team-a", "team-b", "denied"])
        self.assertEqual(sum(d["kind"] == "ConfigMap" for d in documents), 2)
        self.assertFalse(any(d["kind"] in ("ClusterRole", "ClusterRoleBinding") for d in documents))
        roles = [d for d in documents if d["kind"] == "Role"]
        self.assertEqual({d["metadata"]["namespace"] for d in roles}, {"team-a", "team-b"})
        for role in roles:
            self.assertEqual(role["rules"], [{"apiGroups": [""], "resources": ["configmaps"],
                                              "verbs": ["get", "list", "watch"]}])

    def test_probe_environment_overwrites_shared_kubeconfig_and_disables_downloads(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            private_config = root / "observer.kubeconfig"
            env = capture._probe_environment(
                {"PATH": "/safe/bin", "KUBECONFIG": "/shared/credential/path",
                 "HTTPS_PROXY": "http://proxy.invalid"},
                private_config, "kind-owned", "after", ("a", "b", "denied"),
                "same-name", root / "result.json", root / "go-cache")
            self.assertEqual(env["KUBECONFIG"], str(private_config))
            self.assertEqual(env["SCOUT_WNS_PRIVATE_KUBECONFIG"], str(private_config))
            self.assertEqual(env["SCOUT_WNS_OWNED_CLUSTER"], "owned")
            self.assertEqual(env["GOPROXY"], "off")
            self.assertEqual(env["GOSUMDB"], "off")
            self.assertEqual(env["GOTOOLCHAIN"], "local")
            self.assertNotIn("HTTPS_PROXY", env)

    def test_output_requires_fresh_temporary_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            out = capture._fresh_output(root / "evidence")
            self.assertEqual(out.stat().st_mode & 0o777, 0o700)
            with self.assertRaisesRegex(capture.CaptureError, "fresh"):
                capture._fresh_output(out)

    def test_integrity_hash_is_only_sha256_and_never_parsed(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "private.kubeconfig"
            raw = b"not parsed or selected as a client config\n"
            path.write_bytes(raw)
            self.assertEqual(capture.sha256_file(path), capture.sha256(raw))
            self.assertEqual(json.loads(json.dumps({"sha256": capture.sha256_file(path)}))["sha256"], capture.sha256(raw))


if __name__ == "__main__":
    unittest.main()
