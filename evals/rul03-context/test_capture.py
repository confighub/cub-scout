"""Offline safety and evidence-contract tests for RUL-03 capture."""
import base64
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
import urllib.error
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("rul03_capture", HERE / "capture.py")
capture = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(capture)


def endpoints():
    return {
        capture.DENIED_CONTEXT: {"server": "https://127.0.0.1:6443", "caData": base64.b64encode(b"denied-ca").decode(), "token": "denied-token"},
        capture.READABLE_CONTEXT: {"server": "https://127.0.0.1:7443", "caData": base64.b64encode(b"readable-ca").decode(), "token": "readable-token"},
    }


class Response:
    def __init__(self, status, body):
        self.status = status
        self.headers = {"Content-Type": "application/json"}
        self.body = body

    def read(self, size=-1):
        if size < 0:
            size = len(self.body)
        part, self.body = self.body[:size], self.body[size:]
        return part

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False


class Opener:
    def __init__(self, response=None, error=None):
        self.response, self.error = response, error
        self.request = None

    def open(self, request, timeout):
        self.request, self.timeout = request, timeout
        if self.error:
            raise self.error
        return self.response


class CaptureContractTests(unittest.TestCase):
    def test_two_owned_cluster_names_are_distinct_and_fit_node_label(self):
        denied = capture.owned_cluster_name("denied", "261001010203", "01234567")
        readable = capture.owned_cluster_name("readable", "261001010203", "89abcdef")
        self.assertNotEqual(denied, readable)
        for name in (denied, readable):
            self.assertLessEqual(len(name + "-control-plane"), 63)
            self.assertRegex(name, r"^rul03-[a-z0-9-]+$")
        with self.assertRaises(capture.CaptureError):
            capture.owned_cluster_name("other", "x", "y")

    def test_manifests_create_same_object_but_only_readable_grants_rbac(self):
        denied = [json.loads(doc) for doc in capture.build_manifests(False).decode().split("\n---\n")]
        readable = [json.loads(doc) for doc in capture.build_manifests(True).decode().split("\n---\n")]
        for docs in (denied, readable):
            self.assertEqual([d["kind"] for d in docs].count("Deployment"), 1)
            deployment = next(d for d in docs if d["kind"] == "Deployment")
            self.assertEqual((deployment["metadata"]["namespace"], deployment["metadata"]["name"]),
                             (capture.NAMESPACE, capture.DEPLOYMENT))
            self.assertEqual([d["kind"] for d in docs].count("ServiceAccount"), 1)
            self.assertFalse(any(d["kind"] in ("ClusterRole", "ClusterRoleBinding") for d in docs))
        self.assertFalse(any(d["kind"] in ("Role", "RoleBinding") for d in denied))
        role = next(d for d in readable if d["kind"] == "Role")
        self.assertEqual(role["rules"], [{"apiGroups": ["apps"], "resources": ["deployments"], "verbs": ["get", "list"]}])
        self.assertEqual([d["kind"] for d in readable].count("RoleBinding"), 1)

    def test_observer_config_has_two_unique_loopback_bindings_and_readable_default(self):
        config_bytes = capture.build_observer_kubeconfig(endpoints())
        config = json.loads(config_bytes)
        self.assertEqual(config["current-context"], capture.READABLE_CONTEXT)
        self.assertEqual({row["name"] for row in config["contexts"]}, {capture.DENIED_CONTEXT, capture.READABLE_CONTEXT})
        denied = capture.resolve_explicit_context(config_bytes, capture.DENIED_CONTEXT)
        readable = capture.resolve_explicit_context(config_bytes, capture.READABLE_CONTEXT)
        self.assertEqual((denied[0], denied[2]), ("https://127.0.0.1:6443", "denied-token"))
        self.assertEqual((readable[0], readable[2]), ("https://127.0.0.1:7443", "readable-token"))
        self.assertNotEqual(denied[0], readable[0])
        self.assertNotEqual(capture.sha256(denied[1]), capture.sha256(readable[1]))
        serialized = config_bytes.lower()
        for forbidden in (b"client-key", b"client-certificate", b"exec", b"admin-private"):
            self.assertNotIn(forbidden, serialized)

    def test_explicit_denied_selection_does_not_fall_back_to_readable_default(self):
        cfg = capture.build_observer_kubeconfig(endpoints())
        denied_body = json.dumps({"apiVersion": "v1", "kind": "Status", "status": "Failure",
                                  "reason": "Forbidden", "code": 403}).encode()
        opener = Opener(Response(403, denied_body))
        status, body, content_type, _elapsed = capture.fetch_explicit_context(
            cfg, capture.DENIED_CONTEXT, opener_factory=lambda: opener)
        self.assertEqual((status, body, content_type), (403, denied_body, "application/json"))
        self.assertEqual(opener.request.full_url, "https://127.0.0.1:6443" + capture.API_PATH)
        self.assertEqual(opener.request.get_header("Authorization"), "Bearer denied-token")

    def test_missing_or_malformed_explicit_context_never_uses_current_context(self):
        cfg = json.loads(capture.build_observer_kubeconfig(endpoints()))
        cfg["contexts"] = [ctx for ctx in cfg["contexts"] if ctx["name"] != capture.DENIED_CONTEXT]
        self.assertEqual(cfg["current-context"], capture.READABLE_CONTEXT)
        with self.assertRaises(capture.CaptureError):
            capture.resolve_explicit_context(cfg, capture.DENIED_CONTEXT)
        cfg["contexts"].append({"name": capture.DENIED_CONTEXT, "context": {"cluster": "wrong", "user": "missing", "namespace": capture.NAMESPACE}})
        with self.assertRaises(capture.CaptureError):
            capture.resolve_explicit_context(cfg, capture.DENIED_CONTEXT)

    def test_duplicate_endpoint_or_certificate_authority_is_rejected(self):
        values = endpoints()
        values[capture.READABLE_CONTEXT]["server"] = values[capture.DENIED_CONTEXT]["server"]
        with self.assertRaises(capture.CaptureError):
            capture.build_observer_kubeconfig(values)
        values = endpoints()
        values[capture.READABLE_CONTEXT]["caData"] = values[capture.DENIED_CONTEXT]["caData"]
        with self.assertRaises(capture.CaptureError):
            capture.build_observer_kubeconfig(values)

    def test_remote_endpoint_malformed_ca_and_wrong_namespace_fail_closed(self):
        values = endpoints()
        values[capture.DENIED_CONTEXT]["server"] = "https://example.invalid:6443"
        with self.assertRaises(capture.CaptureError):
            capture.build_observer_kubeconfig(values)
        values = endpoints()
        values[capture.DENIED_CONTEXT]["caData"] = "not base64!"
        with self.assertRaises(capture.CaptureError):
            capture.build_observer_kubeconfig(values)
        config = json.loads(capture.build_observer_kubeconfig(endpoints()))
        config["contexts"][0]["context"]["namespace"] = "default"
        with self.assertRaises(capture.CaptureError):
            capture.resolve_explicit_context(config, capture.DENIED_CONTEXT)

    def test_typed_forbidden_and_actual_list_are_distinct_from_empty_list(self):
        forbidden = {"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403,
                     "details": {"kind": "deployments"}, "message": 'deployments.apps is forbidden: User "system:serviceaccount:rul03-proof:rul03-observer" cannot list resource "deployments" in API group "apps" in the namespace "rul03-proof"'}
        self.assertEqual(capture.validate_observation(capture.DENIED_CONTEXT, 403, json.dumps(forbidden).encode())["result"], "forbidden")
        item = {"metadata": {"name": capture.DEPLOYMENT, "namespace": capture.NAMESPACE, "uid": "uid-1", "resourceVersion": "3"}}
        deployment_list = {"apiVersion": "apps/v1", "kind": "DeploymentList", "metadata": {"resourceVersion": "4"}, "items": [item]}
        result = capture.validate_observation(capture.READABLE_CONTEXT, 200, json.dumps(deployment_list).encode())
        self.assertEqual(result["result"], "readable_list")
        self.assertEqual(result["deployment"]["uid"], "uid-1")
        empty = dict(deployment_list, items=[])
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.DENIED_CONTEXT, 200, json.dumps(empty).encode())
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.READABLE_CONTEXT, 200, json.dumps(empty).encode())
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.DENIED_CONTEXT, 403, json.dumps(empty).encode())
        wrong_scope = dict(forbidden, message='User "system:serviceaccount:rul03-proof:rul03-observer" cannot list resource "secrets" in API group "" in the namespace "rul03-proof"')
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.DENIED_CONTEXT, 403, json.dumps(wrong_scope).encode())

    def test_malformed_or_identity_mismatched_bodies_fail_closed(self):
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.DENIED_CONTEXT, 403, b"not JSON")
        denied = {"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403}
        for mutation in (lambda obj: obj.update(reason="NotFound"), lambda obj: obj.update(code=200),
                         lambda obj: obj.update(kind="DeploymentList")):
            body = dict(denied); mutation(body)
            with self.assertRaises(capture.CaptureError):
                capture.validate_observation(capture.DENIED_CONTEXT, 403, json.dumps(body).encode())
        bad_item = {"metadata": {"name": "other", "namespace": capture.NAMESPACE, "uid": "u", "resourceVersion": "1"}}
        response = {"apiVersion": "apps/v1", "kind": "DeploymentList", "items": [bad_item]}
        with self.assertRaises(capture.CaptureError):
            capture.validate_observation(capture.READABLE_CONTEXT, 200, json.dumps(response).encode())

    def test_raw_api_capture_preserves_http_error_body_and_enforces_exact_path(self):
        cfg = capture.build_observer_kubeconfig(endpoints())
        body = b'{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}'
        http_error = urllib.error.HTTPError("url", 403, "Forbidden", {"Content-Type": "application/json"}, io.BytesIO(body))
        status, actual, _content, _elapsed = capture.fetch_explicit_context(
            cfg, capture.DENIED_CONTEXT, opener_factory=lambda: Opener(error=http_error))
        self.assertEqual((status, actual), (403, body))
        with self.assertRaises(capture.CaptureError):
            capture.fetch_explicit_context(cfg, capture.DENIED_CONTEXT, "/api/v1/secrets", opener_factory=lambda: Opener())

    def test_http_body_cap_and_output_caps_are_explicit(self):
        self.assertEqual(capture.MAX_HTTP_BODY, 4 * 1024 * 1024)
        self.assertEqual(capture.MAX_COMMAND_OUTPUT, 2 * 1024 * 1024)
        cfg = capture.build_observer_kubeconfig(endpoints())
        class LongResponse(Response):
            def __init__(self):
                super().__init__(200, b"x" * (capture.MAX_HTTP_BODY + 1))
        with self.assertRaises(capture.CaptureError):
            capture.fetch_explicit_context(cfg, capture.READABLE_CONTEXT,
                                           opener_factory=lambda: Opener(LongResponse()))

    def test_command_construction_always_pins_private_file_and_explicit_context(self):
        args = capture.kubectl_context_args(Path("observer.json"), capture.DENIED_CONTEXT,
                                             ["get", "--raw", capture.API_PATH])
        self.assertEqual(args[:4], ["--kubeconfig", "observer.json", "--context", capture.DENIED_CONTEXT])
        self.assertIn("--context", args)
        with self.assertRaises(capture.CaptureError):
            capture.kubectl_context_args(Path("config"), "", ["get", "pods"])

    def test_command_wrapper_passes_explicit_output_cap(self):
        with patch.object(capture.inv04, "run_bounded", return_value=(0, b"", b"")) as run:
            code, _stdout, _stderr = capture._command(Path("kubectl"), ["version"], 5, {}, time.monotonic() + 3)
        self.assertEqual(code, 0)
        self.assertEqual(run.call_args.args[3], capture.MAX_COMMAND_OUTPUT)

    def test_generated_output_is_private_new_and_refuses_symlink_or_overwrite(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            output = capture.fresh_output(root / "new")
            self.assertEqual(output.stat().st_mode & 0o777, 0o700)
            with self.assertRaises(capture.CaptureError):
                capture.fresh_output(output)
            alias = root / "alias"
            alias.symlink_to(output, target_is_directory=True)
            with self.assertRaises(capture.CaptureError):
                capture.fresh_output(alias)
            private_file = output / "private"
            capture._write(private_file, b"sensitive")
            self.assertEqual(private_file.stat().st_mode & 0o777, 0o600)

    def test_cli_requires_explicit_execute_before_capture(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            argv = ["--shared-kubeconfig", str(root / "shared"), "--kind-binary", str(root / "kind"),
                    "--kubectl-binary", str(root / "kubectl"), "--docker-binary", str(root / "docker"),
                    "--output-dir", str(root / "out")]
            with patch.object(capture, "capture") as run:
                with self.assertRaises(SystemExit) as error:
                    capture.main(argv)
            self.assertEqual(error.exception.code, 2)
            run.assert_not_called()

    def test_tool_launcher_name_is_preserved_after_regular_target_validation(self):
        with tempfile.TemporaryDirectory() as td:
            target = Path(td) / "docker-tools"
            target.write_bytes(b"synthetic executable"); target.chmod(0o700)
            launcher = Path(td) / "docker"; launcher.symlink_to(target)
            self.assertEqual(capture._tool(launcher), launcher)
            self.assertEqual(capture.sha256(launcher.read_bytes()), capture.sha256(target.read_bytes()))
            target.chmod(0o600)
            with self.assertRaises(capture.CaptureError): capture._tool(launcher)

    def test_cleanup_after_partial_create_deletes_only_marked_attempts(self):
        with tempfile.TemporaryDirectory() as td:
            private = Path(td) / "admin.kubeconfig"
            private.write_text("private")
            name = capture.owned_cluster_name("denied", "261001010203", "11111111")
            state = {"clusters": [name, "unrelated-cluster"], "calls": []}
            marker = {"schema": "rul03-owned-clusters.v1", "ownerPid": 99, "clusterNames": [name]}
            def runner(_binary, args, _timeout, env, _deadline):
                state["calls"].append((args, env.copy()))
                self.assertEqual(env.get("KUBECONFIG"), str(private))
                if args == ["get", "clusters"]:
                    return 0, ("\n".join(state["clusters"]) + "\n").encode(), b""
                if args == ["delete", "cluster", "--name", name, "--kubeconfig", str(private)]:
                    state["clusters"].remove(name)
                    return 0, b"", b""
                raise AssertionError(args)
            result = capture.cleanup_owned_clusters(Path("kind"),
                [{"name": name, "attempted": True, "kubeconfigPath": str(private)}],
                marker, 99, {"KUBECONFIG": "/shared/default"}, runner=runner)
            self.assertTrue(result["verified"])
            self.assertEqual(state["clusters"], ["unrelated-cluster"])
            self.assertEqual(result["deleted"], [name])
            delete_args, delete_env = next(call for call in state["calls"] if call[0][0] == "delete")
            self.assertEqual(delete_args[-2:], ["--kubeconfig", str(private)])
            self.assertEqual(delete_env["KUBECONFIG"], str(private))
            self.assertFalse(any("unrelated-cluster" in args for args, _env in state["calls"]))

    def test_partial_create_without_config_recovers_private_cleanup_only(self):
        with tempfile.TemporaryDirectory() as td:
            private = Path(td) / "admin.kubeconfig"
            name = capture.owned_cluster_name("denied", "261001010203", "11111111")
            existing = [name]
            def runner(_binary, args, _timeout, env, _deadline):
                self.assertEqual(env["KUBECONFIG"], str(private))
                self.assertEqual(json.loads(private.read_bytes())["clusters"], [])
                self.assertEqual(private.stat().st_mode & 0o777, 0o600)
                if args == ["get", "clusters"]:
                    return 0, "\n".join(existing).encode(), b""
                self.assertEqual(args, ["delete", "cluster", "--name", name, "--kubeconfig", str(private)])
                existing.clear()
                return 0, b"", b""
            result = capture.cleanup_owned_clusters(Path("kind"),
                [{"name":name, "attempted":True, "kubeconfigPath":str(private)}],
                {"schema":"rul03-owned-clusters.v1", "ownerPid":99, "clusterNames":[name]},
                99, {"KUBECONFIG":"/shared/default"}, runner=runner)
            self.assertTrue(result["verified"])
            self.assertEqual(existing, [])

    def test_cleanup_never_falls_back_when_private_kubeconfig_is_missing(self):
        name = capture.owned_cluster_name("denied", "261001010203", "44444444")
        marker = {"schema": "rul03-owned-clusters.v1", "ownerPid": 99, "clusterNames": [name]}
        result = capture.cleanup_owned_clusters(Path("kind"),
            [{"name": name, "attempted": True, "kubeconfigPath": "/missing/private-config"}],
            marker, 99, {"KUBECONFIG": "/shared/default"},
            runner=lambda *_args: self.fail("missing private config must refuse before kind query"))
        self.assertFalse(result["verified"])
        self.assertEqual(result["deleted"], [])
        self.assertIn("private kubeconfig unavailable", result["errors"][0])

    def test_cleanup_refuses_mismatched_ownership_marker(self):
        name = capture.owned_cluster_name("denied", "261001010203", "33333333")
        result = capture.cleanup_owned_clusters(Path("kind"), [{"name": name, "attempted": True}],
            {"schema": "wrong", "ownerPid": 99, "clusterNames": [name]}, 99, {},
            runner=lambda *_args: self.fail("must reject before listing/deleting"))
        self.assertFalse(result["verified"])
        self.assertEqual(result["deleted"], [])

    def test_no_creation_attempt_means_cleanup_does_not_query_any_cluster_list(self):
        result = capture.cleanup_owned_clusters(Path("kind"), [],
            {"schema": "rul03-owned-clusters.v1", "ownerPid": 99, "clusterNames": []}, 99, {},
            runner=lambda *_args: self.fail("no kind call is allowed without an attempted owned create"))
        self.assertTrue(result["verified"])
        self.assertEqual(result["deleted"], [])

    def test_interruption_during_second_partial_create_runs_bounded_owned_cleanup(self):
        self._interruption_capture()

    def test_shared_config_hash_is_checked_after_cleanup(self):
        self._interruption_capture(corrupt_shared=True)

    def _interruption_capture(self, corrupt_shared=False):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            shared = root / "shared.kubeconfig"
            shared.write_bytes(b"shared-stable")
            out = root / "capture"
            binaries = {}
            for name in ("git", "kind", "kubectl", "docker"):
                path = root / name
                path.write_text("fake tool")
                path.chmod(0o700)
                binaries[name] = path
            clusters = []
            deleted = []
            names = {"denied": "rul03-den-261001010203-11111111", "readable": "rul03-rea-261001010203-22222222"}
            def fake_command(binary, args, _timeout, _env, _deadline=None):
                tool = Path(binary).name
                if tool == "git":
                    return 0, (b"a" * 40 + b"\n" if "rev-parse" in args else b""), b""
                if tool == "docker" and args == ["context", "show"]:
                    return 0, b"default\n", b""
                if tool == "docker" and args[:2] == ["context", "inspect"]:
                    return 0, b'"unix:///var/run/docker.sock"', b""
                if tool == "docker" and args[:2] == ["image", "inspect"]:
                    digest = capture.NODE_IMAGE.rsplit("@", 1)[1]
                    return 0, json.dumps([{"RepoDigests": ["kindest/node@" + digest]}]).encode(), b""
                if tool == "kind" and args == ["version"]:
                    return 0, b"kind v0.31.0\n", b""
                if tool == "kind" and args == ["get", "clusters"]:
                    self.assertNotEqual(_env.get("KUBECONFIG"), str(shared))
                    self.assertTrue(Path(_env.get("KUBECONFIG", "")).is_file())
                    return 0, ("\n".join(clusters) + ("\n" if clusters else "")).encode(), b""
                if tool == "kind" and args[:2] == ["create", "cluster"]:
                    name = args[args.index("--name") + 1]
                    admin = Path(args[args.index("--kubeconfig") + 1])
                    self.assertEqual(_env.get("KUBECONFIG"), str(admin))
                    self.assertNotEqual(_env.get("KUBECONFIG"), str(shared))
                    admin.write_bytes(b"private-admin-config-" + name.encode())
                    clusters.append(name)
                    if name == names["readable"]:
                        raise capture.CaptureInterrupted("simulated signal during partial create")
                    return 0, b"created\n", b""
                if tool == "kind" and args[:3] == ["delete", "cluster", "--name"]:
                    self.assertIn("--kubeconfig", args)
                    self.assertEqual(_env.get("KUBECONFIG"), args[args.index("--kubeconfig") + 1])
                    name = args[3]
                    deleted.append(name)
                    clusters.remove(name)
                    Path(_env["KUBECONFIG"]).write_bytes(b"mutated-by-private-cleanup")
                    if corrupt_shared: shared.write_bytes(b"unexpected-shared-mutation")
                    return 0, b"", b""
                if tool == "kubectl" and "apply" in args:
                    return 0, b"applied\n", b""
                if tool == "kubectl" and args[-1:] == ["json"] and "get" in args:
                    obj = {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {
                        "name": capture.DEPLOYMENT, "namespace": capture.NAMESPACE, "uid": "uid-1", "resourceVersion": "4"}}
                    return 0, json.dumps(obj).encode(), b""
                if tool == "kubectl" and "config" in args:
                    context = args[args.index("--context") + 1]
                    port = "6443" if "den-" in context else "7443"
                    ca = base64.b64encode(("pem-" + port).encode()).decode()
                    obj = {"clusters": [{"cluster": {"server": "https://127.0.0.1:" + port,
                                                          "certificate-authority-data": ca}}]}
                    return 0, json.dumps(obj).encode(), b""
                if tool == "kubectl" and "token" in args:
                    return 0, ("token-" + args[args.index("--context") + 1]).encode(), b""
                raise AssertionError((tool, args))
            def fake_name(role):
                return names[role]
            with patch.object(capture, "_tool", side_effect=lambda path: Path(path)), \
                 patch.object(capture.inv04, "_command", return_value=str(binaries["git"])), \
                 patch.object(capture, "owned_cluster_name", side_effect=fake_name), \
                 patch.object(capture, "_command", side_effect=fake_command), \
                 patch.dict(capture.os.environ, {"DOCKER_HOST": "", "KUBECONFIG": str(shared)}), \
                 patch.object(capture, "_install_capture_deadline", return_value=object()), \
                 patch.object(capture, "_disable_capture_signals"), patch.object(capture, "_restore_capture_signals"):
                result = capture.capture(shared, out, binaries["kind"], binaries["kubectl"], binaries["docker"])
            self.assertEqual(result, 1)
            self.assertEqual(set(deleted), set(names.values()), (json.loads((out / "provenance.json").read_text())["errors"],
                                                                  json.loads((out / "provenance.json").read_text())["cleanup"]))
            self.assertEqual(clusters, [])
            provenance = json.loads((out / "provenance.json").read_text())
            self.assertTrue(provenance["cleanup"]["verified"])
            self.assertEqual(provenance["privateKubeconfigSha256"]["adminBeforeCleanup"]["denied"],
                             provenance["privateKubeconfigSha256"]["adminInitial"]["denied"])
            self.assertNotEqual(provenance["privateKubeconfigSha256"]["adminAfterCleanup"]["denied"],
                                provenance["privateKubeconfigSha256"]["adminBeforeCleanup"]["denied"])
            self.assertTrue(any(operation.get("errorType") == "CaptureInterrupted" for operation in provenance["operations"]))
            self.assertEqual(provenance["sharedKubeconfigSha256"]["before"], provenance["sharedKubeconfigSha256"]["beforeCleanup"])
            self.assertEqual(provenance["sharedKubeconfigSha256"]["unchanged"], not corrupt_shared)
            if corrupt_shared:
                self.assertNotEqual(provenance["sharedKubeconfigSha256"]["before"], provenance["sharedKubeconfigSha256"]["after"])
            else:
                self.assertEqual(provenance["sharedKubeconfigSha256"]["before"], provenance["sharedKubeconfigSha256"]["after"])
            self.assertEqual((out.stat().st_mode & 0o777), 0o700)


if __name__ == "__main__":
    unittest.main()
