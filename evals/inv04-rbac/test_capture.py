#!/usr/bin/env python3
"""Offline safety and semantic tests for the INV-04 capture helper."""
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


HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("inv04_capture", HERE / "capture.py")
capture = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(capture)


class _Response:
    def __init__(self, status, body, content_type="application/json"):
        self.status = status
        self.headers = {"Content-Type": content_type}
        self._body = body

    def read(self, _limit=-1):
        if _limit < 0:
            _limit = len(self._body)
        chunk, self._body = self._body[:_limit], self._body[_limit:]
        return chunk

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False


class _Opener:
    def __init__(self, response=None, error=None):
        self.response, self.error = response, error
        self.request = None

    def open(self, request, timeout):
        self.request = request
        self.timeout = timeout
        if self.error:
            raise self.error
        return self.response


class CaptureTests(unittest.TestCase):
    def test_kind_version_requires_exact_release_token(self):
        capture.require_kind_version("kind v0.31.0 go1.25.5 darwin/arm64")
        for version in ("kind v0.31.0-debug", "kind v0.31.0-alpha", "kind v0.31.01", "v0.31.0", "other v0.31.0"):
            with self.subTest(version=version), self.assertRaises(capture.CaptureError):
                capture.require_kind_version(version)

    def test_observer_config_contains_only_endpoint_ca_and_token(self):
        config = json.loads(capture.observer_kubeconfig("https://127.0.0.1:6443", "Q0E=", "observer-token"))
        self.assertEqual(config["current-context"], "inv04-observer")
        self.assertEqual(config["clusters"][0]["cluster"]["certificate-authority-data"], "Q0E=")
        self.assertEqual(config["users"], [{"name": "inv04-observer", "user": {"token": "observer-token"}}])
        serialized = json.dumps(config).lower()
        self.assertNotIn("client-certificate", serialized)
        self.assertNotIn("client-key", serialized)
        self.assertNotIn("exec", serialized)
        with self.assertRaises(capture.CaptureError):
            capture.observer_kubeconfig("https://remote.example:6443", "Q0E=", "observer-token")

    def test_local_engine_guard_rejects_remote_configuration(self):
        with self.assertRaises(capture.CaptureError):
            capture.require_local_docker({"DOCKER_HOST": "tcp://example.invalid:2376"})
        calls = []
        original = capture._call
        def fake_call(name, args, timeout, env=None):
            calls.append((name, args))
            if args == ["context", "show"]:
                return b"remote\n"
            return b'"tcp://example.invalid:2376"'
        capture._call = fake_call
        try:
            with self.assertRaises(capture.CaptureError):
                capture.require_local_docker({})
        finally:
            capture._call = original
        self.assertEqual(len(calls), 2)

    def test_expected_repo_digest_is_verified_not_inferred_from_image_id(self):
        expected = capture.NODE_IMAGE.rsplit("@", 1)[1]
        output = json.dumps([{"Id": "sha256:" + "a" * 64,
                              "RepoDigests": ["docker.io/kindest/node@" + expected]}]).encode()
        self.assertEqual(capture.verified_node_repo_digest(output), "docker.io/kindest/node@" + expected)
        exact_repository = json.dumps([{"Id": "sha256:" + "a" * 64,
                                        "RepoDigests": ["kindest/node@" + expected]}]).encode()
        self.assertEqual(capture.verified_node_repo_digest(exact_repository), "kindest/node@" + expected)
        with self.assertRaises(capture.CaptureError):
            capture.verified_node_repo_digest(json.dumps([{"Id": expected, "RepoDigests": ["kindest/node@sha256:" + "b" * 64]}]).encode())

    def test_literal_manifests_grant_only_namespaced_deployment_reads(self):
        docs = [json.loads(part) for part in capture.build_manifests().decode().split("\n---\n")]
        self.assertEqual([d["metadata"]["name"] for d in docs if d["kind"] == "Namespace"], list(capture.NAMESPACES))
        self.assertEqual(len([d for d in docs if d["kind"] == "Deployment"]), 2)
        roles = [d for d in docs if d["kind"] == "Role"]
        self.assertEqual(len(roles), 2)
        for role in roles:
            self.assertEqual(role["rules"], [{"apiGroups": ["apps"], "resources": ["deployments"], "verbs": ["get", "list"]}])
        self.assertFalse(any(d["kind"] in ("ClusterRole", "ClusterRoleBinding") for d in docs))
        self.assertFalse(any(d["kind"] == "RoleBinding" and d["metadata"]["namespace"] == capture.NAMESPACES[2] for d in docs))
        self.assertFalse(any("Secret" in d["kind"] or "secret" in json.dumps(d).lower() for d in docs))

    def test_successful_empty_and_forbidden_are_distinct(self):
        populated = {"apiVersion": "apps/v1", "kind": "DeploymentList", "items": [
            {"metadata": {"name": n, "namespace": capture.NAMESPACES[0], "uid": "uid-" + n, "resourceVersion": "4"}}
            for n in ("inv04-api", "inv04-worker")
        ]}
        empty = {"apiVersion": "apps/v1", "kind": "DeploymentList", "metadata": {"resourceVersion": "5"}, "items": []}
        denied = {"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403}
        self.assertEqual(capture.validate_api_response(capture.NAMESPACES[0], 200, json.dumps(populated).encode())["result"], "readable_populated")
        self.assertEqual(capture.validate_api_response(capture.NAMESPACES[1], 200, json.dumps(empty).encode())["result"], "readable_empty")
        self.assertEqual(capture.validate_api_response(capture.NAMESPACES[2], 403, json.dumps(denied).encode())["result"], "denied")
        with self.assertRaises(capture.CaptureError):
            capture.validate_api_response(capture.NAMESPACES[2], 200, json.dumps(empty).encode())
        with self.assertRaises(capture.CaptureError):
            capture.validate_api_response(capture.NAMESPACES[1], 403, json.dumps(denied).encode())
        with self.assertRaises(capture.CaptureError):
            capture.validate_api_response(capture.NAMESPACES[1], 200, b"not json")

    def test_capture_success_requires_all_three_valid_raw_observations(self):
        records = [{"validation": {"result": value}, "error": None} for value in
                   ("readable_populated", "readable_empty", "denied")]
        self.assertTrue(capture.raw_observations_complete(records))
        invalid = [dict(record) for record in records]
        invalid[2] = {"validation": {"result": "readable_empty"}, "error": "API observation incomplete"}
        self.assertFalse(capture.raw_observations_complete(invalid))
        self.assertFalse(capture.raw_observations_complete(records[:2]))

    def test_raw_success_and_http_error_bodies_are_preserved_exactly(self):
        body = b'{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}'
        opener = _Opener(_Response(200, body))
        status, actual, content_type, _elapsed = capture.fetch_raw_list(
            "https://127.0.0.1:6443", b"unused CA", "token", capture.NAMESPACES[1], opener_factory=lambda: opener)
        self.assertEqual((status, actual, content_type), (200, body, "application/json"))
        self.assertEqual(opener.request.full_url, "https://127.0.0.1:6443/apis/apps/v1/namespaces/inv04-readable-empty/deployments")
        self.assertEqual(opener.request.get_header("Authorization"), "Bearer token")
        denied_body = b'{"apiVersion":"v1","kind":"Status","reason":"Forbidden","code":403}'
        http_error = urllib.error.HTTPError("url", 403, "Forbidden", {"Content-Type": "application/json"}, io.BytesIO(denied_body))
        status, actual, _content_type, _elapsed = capture.fetch_raw_list(
            "https://127.0.0.1:6443", b"unused CA", "token", capture.NAMESPACES[2], opener_factory=lambda: _Opener(error=http_error))
        self.assertEqual((status, actual), (403, denied_body))
        with self.assertRaises(capture.CaptureError):
            capture.fetch_raw_list("https://127.0.0.1:6443", b"ca", "token", "kube-system", opener_factory=lambda: opener)

    def test_raw_response_body_limit_is_enforced(self):
        response = _Response(200, b"x" * (capture.MAX_HTTP_BODY + 1))
        with self.assertRaises(capture.CaptureError):
            capture.fetch_raw_list("https://127.0.0.1:6443", b"unused CA", "token",
                                   capture.NAMESPACES[1], opener_factory=lambda: _Opener(response=response))

    @unittest.skipUnless(hasattr(os, "killpg") and hasattr(os, "fork"), "requires POSIX process groups")
    def test_bounded_command_kills_owned_process_group_on_timeout(self):
        with tempfile.TemporaryDirectory(prefix="inv04-process-test-") as temp:
            heartbeat = Path(temp) / "heartbeat"
            ready = Path(temp) / "ready"
            child = Path(temp) / "child.py"
            child.write_text("import signal, time\nsignal.signal(signal.SIGTERM, signal.SIG_IGN)\nopen(%r, 'w').close()\nwhile True:\n with open(%r, 'a') as f: f.write('x')\n time.sleep(.03)\n" % (str(ready), str(heartbeat)))
            parent = "import os,time; pid=os.fork(); (exec(compile(open(%r).read(), %r, 'exec')) if pid == 0 else time.sleep(30))" % (str(child), str(child))
            with self.assertRaises(capture.CaptureError):
                capture.run_bounded([sys.executable, "-c", parent], 1.0, max_output=4096)
            self.assertTrue(ready.exists(), "child must start before timeout cleanup is considered exercised")
            count = heartbeat.stat().st_size if heartbeat.exists() else 0
            time.sleep(0.15)
            self.assertEqual(heartbeat.stat().st_size if heartbeat.exists() else 0, count)

    @unittest.skipUnless(hasattr(os, "killpg") and hasattr(os, "fork"), "requires POSIX process groups")
    def test_bounded_command_cleans_descendant_after_normal_leader_exit(self):
        with tempfile.TemporaryDirectory(prefix="inv04-normal-exit-test-") as temp:
            heartbeat, ready, child = (Path(temp) / name for name in ("heartbeat", "ready", "child.py"))
            child.write_text("import os, signal, time\nos.close(1); os.close(2)\nsignal.signal(signal.SIGTERM, signal.SIG_IGN)\nopen(%r, 'w').close()\nwhile True:\n with open(%r, 'a') as f: f.write('x')\n time.sleep(.03)\n" % (str(ready), str(heartbeat)))
            parent = "import os,time\npid=os.fork()\nif pid == 0: exec(compile(open(%r).read(), %r, 'exec'))\ndeadline=time.time()+3\nwhile not os.path.exists(%r) and time.time()<deadline: time.sleep(.01)\nif not os.path.exists(%r): raise SystemExit(9)\n" % (str(child), str(child), str(ready), str(ready))
            code, _stdout, _stderr = capture.run_bounded([sys.executable, "-c", parent], 5, max_output=4096)
            self.assertEqual(code, 0)
            self.assertTrue(ready.exists())
            count = heartbeat.stat().st_size if heartbeat.exists() else 0
            time.sleep(0.15)
            self.assertEqual(heartbeat.stat().st_size if heartbeat.exists() else 0, count)


if __name__ == "__main__":
    unittest.main()
