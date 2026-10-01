"""Pure source and mocked-lifecycle guards for PRE-02."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("pre02_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(capture)


NODE_NAME = "scout-pre02-test-control-plane"
POD_UID = "pod-fixture-uid"


def pod(*, scheduled=False, uid=POD_UID, selector_value=capture.LABEL_VALUE, spec_change=None):
    spec = {"restartPolicy": "Never", "nodeSelector": {capture.LABEL_KEY: selector_value},
            "containers": [{"name": capture.CONTAINER_NAME, "image": capture.IMAGE,
                            "imagePullPolicy": "Never"}]}
    if scheduled:
        spec["nodeName"] = NODE_NAME
    if spec_change:
        spec.update(spec_change)
    condition = {"type": "PodScheduled", "status": "True" if scheduled else "False",
                 "reason": "" if scheduled else "Unschedulable",
                 "message": "scheduled" if scheduled else "0/1 nodes are available: 1 node(s) didn't match pod's node affinity/selector."}
    status = {"conditions": [condition], "phase": "Pending" if not scheduled else "Pending"}
    return {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": capture.POD_NAME,
        "namespace": capture.NAMESPACE, "uid": uid, "resourceVersion": "17" if not scheduled else "23"},
        "spec": spec, "status": status}


def nodes(*, labelled=False, names=(NODE_NAME,)):
    return {"apiVersion": "v1", "kind": "NodeList", "metadata": {"resourceVersion": "10" if not labelled else "19"},
        "items": [{"metadata": {"name": name,
            "uid": "node-uid-" + name, "resourceVersion": "9" if not labelled else "18",
            "labels": {"kubernetes.io/hostname": name, **({capture.LABEL_KEY: capture.LABEL_VALUE}
                if labelled and name == NODE_NAME else {})}}} for name in names]}


def events(*, uid=POD_UID, reason="FailedScheduling", message="0/1 nodes are available: 1 node(s) didn't match pod's node affinity/selector."):
    return {"apiVersion": "v1", "kind": "EventList", "metadata": {"resourceVersion": "15"}, "items": [{
        "apiVersion": "v1", "kind": "Event", "metadata": {"name": "schedule-failure.123", "namespace": capture.NAMESPACE},
        "involvedObject": {"apiVersion": "v1", "kind": "Pod", "name": capture.POD_NAME,
                           "namespace": capture.NAMESPACE, "uid": uid},
        "type": "Warning", "reason": reason, "message": message}]}


def encoded(value):
    return (json.dumps(value, separators=(",", ":")) + "\n").encode()


class Pre02ContractTests(unittest.TestCase):
    def test_repository_path_points_at_checkout_root(self):
        self.assertEqual(capture.REPO, Path(__file__).resolve().parents[2])

    def test_authored_fixture_is_single_pod_with_selector_and_no_external_prerequisites(self):
        docs = [json.loads(doc) for doc in capture.MANIFEST.decode().split("\n---\n")]
        pods = [doc for doc in docs if doc.get("kind") == "Pod"]
        self.assertEqual(len(pods), 1)
        pod_doc = pods[0]
        self.assertEqual(pod_doc["metadata"]["namespace"], capture.NAMESPACE)
        self.assertEqual(pod_doc["spec"]["nodeSelector"], {capture.LABEL_KEY: capture.LABEL_VALUE})
        self.assertEqual(pod_doc["spec"]["containers"], [{"name": capture.CONTAINER_NAME,
            "image": capture.IMAGE, "imagePullPolicy": "Never"}])
        self.assertFalse(any(doc.get("kind") in ("Secret", "Service", "Deployment") for doc in docs))
        role = next(doc for doc in docs if doc.get("kind") == "Role")
        self.assertEqual(role["rules"], [
            {"apiGroups": [""], "resources": ["pods"], "verbs": ["get"]},
            {"apiGroups": [""], "resources": ["events"], "verbs": ["list"]},
        ])
        cluster_role = next(doc for doc in docs if doc.get("kind") == "ClusterRole")
        self.assertEqual(cluster_role["rules"], [
            {"apiGroups": [""], "resources": ["nodes"], "verbs": ["list"]},
        ])
        self.assertIn('"resources":["nodes"]', capture.MANIFEST.decode())
        self.assertNotIn('"resources":["secrets"]', capture.MANIFEST.decode())

    def test_before_pod_and_nodes_must_prove_selector_prerequisite(self):
        before = capture.validate_pod(encoded(pod()), "before")
        node_state = capture.validate_nodes(encoded(nodes()), "before")
        self.assertEqual(before["uid"], POD_UID)
        self.assertEqual(node_state["matchingNodeNames"], [])
        malformed = pod()
        malformed["status"]["conditions"][0]["reason"] = "Pending"
        with self.assertRaises(capture.CaptureError):
            capture.validate_pod(encoded(malformed), "before")
        already_match = nodes(labelled=True)
        with self.assertRaises(capture.CaptureError):
            capture.validate_nodes(encoded(already_match), "before")
        with self.assertRaises(capture.CaptureError):
            capture.validate_nodes(encoded(nodes(names=())), "before")

    def test_before_event_must_be_uid_correlated_failed_scheduling_selector_mismatch(self):
        result = capture.validate_events(encoded(events()), POD_UID, "before")
        self.assertEqual(result["failedSchedulingEventCount"], 1)
        for bad in (events(uid="other-pod"), events(reason="FailedMount"),
                    events(message="0/1 nodes are available: insufficient cpu."),
                    events(message="0/1 nodes are available: 1 node(s) had untolerated taint.")):
            with self.subTest(bad=bad), self.assertRaises(capture.CaptureError):
                capture.validate_events(encoded(bad), POD_UID, "before")
        contradictory = events()
        contradictory["items"][0]["involvedObject"]["name"] = "different-pod"
        with self.assertRaises(capture.CaptureError):
            capture.validate_events(encoded(contradictory), POD_UID, "before")

    def test_after_state_preserves_stale_failed_event_without_current_failure_claim(self):
        value = capture.validate_events(encoded(events()), POD_UID, "after")
        self.assertTrue(value["historicalEventsNotTreatedAsCurrentFailure"])
        self.assertEqual(value["staleFailedSchedulingEventCount"], 1)
        pod_state = capture.validate_pod(encoded(pod(scheduled=True)), "after")
        node_state = capture.validate_nodes(encoded(nodes(labelled=True)), "after", NODE_NAME)
        self.assertEqual(pod_state["nodeName"], NODE_NAME)
        self.assertEqual(node_state["matchingNodeNames"], [NODE_NAME])

    def test_transition_requires_same_pod_spec_node_set_and_only_owned_label_change(self):
        before_pod = capture.validate_pod(encoded(pod()), "before")
        before_nodes = capture.validate_nodes(encoded(nodes()), "before")
        after_pod = capture.validate_pod(encoded(pod(scheduled=True)), "after")
        after_nodes = capture.validate_nodes(encoded(nodes(labelled=True)), "after", NODE_NAME)
        result = capture.validate_transition(before_pod, before_nodes, after_pod, after_nodes)
        self.assertTrue(result["podUIDUnchanged"])
        self.assertTrue(result["podSpecUnchangedExceptSchedulerNodeName"])
        self.assertIn("not readiness", result["conclusion"])
        with self.assertRaises(capture.CaptureError):
            capture.validate_transition(before_pod, before_nodes,
                capture.validate_pod(encoded(pod(scheduled=True, uid="replacement")), "after"), after_nodes)
        with self.assertRaises(capture.CaptureError):
            capture.validate_transition(before_pod, before_nodes,
                capture.validate_pod(encoded(pod(scheduled=True, spec_change={"priority": 1})), "after"), after_nodes)
        with self.assertRaises(capture.CaptureError):
            capture.validate_transition(before_pod, before_nodes,
                capture.validate_pod(encoded(pod(scheduled=True,
                    spec_change={"containers": [{"name": capture.CONTAINER_NAME,
                        "image": "different", "imagePullPolicy": "Never"}]})), "after"), after_nodes)
        changed_node = nodes(labelled=True)
        changed_node["items"][0]["metadata"]["labels"]["example.com/other"] = "mutation"
        with self.assertRaises(capture.CaptureError):
            capture.validate_transition(before_pod, before_nodes, after_pod,
                capture.validate_nodes(encoded(changed_node), "after", NODE_NAME))
        with self.assertRaises(capture.CaptureError):
            capture.validate_transition(before_pod, before_nodes, after_pod,
                capture.validate_nodes(encoded(nodes(labelled=True, names=("replacement",))), "after", NODE_NAME))

    def test_kubernetes_shape_edges_return_capture_errors(self):
        for body in (b"[]", b"null", b'"Pod"'):
            with self.subTest(body=body), self.assertRaises(capture.CaptureError):
                capture.validate_pod(body, "before")
            with self.subTest(body=body), self.assertRaises(capture.CaptureError):
                capture.validate_nodes(body, "before")
            with self.subTest(body=body), self.assertRaises(capture.CaptureError):
                capture.validate_events(body, POD_UID, "before")
        malformed_nodes = nodes()
        malformed_nodes["items"][0]["kind"] = "Pod"
        with self.assertRaises(capture.CaptureError):
            capture.validate_nodes(encoded(malformed_nodes), "before")
        malformed_events = events(message=None)
        with self.assertRaises(capture.CaptureError):
            capture.validate_events(encoded(malformed_events), POD_UID, "before")

    def test_prerequisite_wait_retains_bounded_failed_poll_log(self):
        observations = []
        def no_events(server, ca, token, path, timeout=10):
            if path == capture.API_POD:
                return 200, encoded(pod()), 0.001
            return 200, encoded({"apiVersion": "v1", "kind": "EventList",
                "metadata": {"resourceVersion": "18"}, "items": []}), 0.001
        with self.assertRaises(capture.CaptureError):
            capture.wait_for_unschedulable_pod("https://127.0.0.1:6443", b"ca", "token", (),
                api_reader=no_events, deadline=time.monotonic() + 0.01, observations=observations)
        self.assertTrue(observations)
        self.assertIn("FailedScheduling", observations[0]["error"])
        self.assertIn("eventsSha256", observations[0])

    def test_api_allowlist_and_strict_json_reject_ambiguous_shapes(self):
        capture.validate_api_path(capture.API_POD)
        capture.validate_api_path(capture.API_NODES)
        capture.validate_api_path(capture.API_EVENTS_PREFIX + POD_UID, POD_UID)
        for path in ("/api/v1/secrets", "/api/v1/nodes?watch=true", capture.API_EVENTS_PREFIX + "other"):
            with self.subTest(path=path), self.assertRaises(capture.CaptureError):
                capture.validate_api_path(path, POD_UID if "fieldSelector" in path else None)
        for body in (b'{"kind":"Pod","kind":"Node"}', b'{"value":NaN}', b'{"value":1e999}'):
            with self.subTest(body=body), self.assertRaises(capture.CaptureError):
                capture.strict_json(body, "test")

    def test_observer_context_is_loopback_and_has_no_admin_secret_fields(self):
        config = json.loads(capture.observer_config("https://127.0.0.1:6443", "Q0E=", "observer-test-token"))
        self.assertEqual(config["current-context"], "pre02-observer")
        self.assertEqual(config["contexts"][0]["context"]["namespace"], capture.NAMESPACE)
        self.assertNotIn("client-key", json.dumps(config))
        with self.assertRaises(capture.CaptureError):
            capture.observer_config("https://remote.example:6443", "Q0E=", "token")

    def test_caps_and_timing_are_fixed_and_api_fetch_stays_bounded(self):
        self.assertEqual((capture.EXECUTION_SECONDS, capture.CLEANUP_SECONDS, capture.TOTAL_SECONDS), (120, 30, 150))
        self.assertEqual(capture.MAX_BODY, 2 * 1024 * 1024)
        self.assertEqual(capture.MAX_COMMAND_OUTPUT, 2 * 1024 * 1024)
        observed = {}
        class Response:
            status = 200
            def __enter__(self): return self
            def __exit__(self, *_): return None
            def read(self, size=-1):
                observed["requestedBytes"] = size
                if observed.get("read"):
                    return b""
                observed["read"] = True
                return b'{"ok":true}'
        class Opener:
            def open(self, request, timeout):
                observed["timeout"] = timeout
                observed["url"] = request.full_url
                return Response()
        result = capture.fetch_api("https://127.0.0.1:6443", b"ca", "token", capture.API_POD,
                                   timeout=3.0, opener_factory=Opener)
        self.assertEqual(result[0], 200)
        self.assertLessEqual(observed["timeout"], 3.0)
        self.assertIn(capture.API_POD, observed["url"])

    def test_cli_requires_execute_flag_without_invoking_capture(self):
        with patch.object(capture, "capture") as actual:
            with self.assertRaises(SystemExit) as failure:
                capture.main(["--shared-kubeconfig", "/tmp/shared", "--output-dir", "/tmp/new"])
        self.assertEqual(failure.exception.code, 2)
        actual.assert_not_called()


class MockLifecycleTests(unittest.TestCase):
    def test_mocked_capture_runs_bounded_phases_and_cleans_only_owned_cluster(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            shared = root / "shared.kubeconfig"
            shared.write_text("shared bytes\n")
            out = root / "capture"
            binaries = {}
            for name in ("kind", "kubectl", "docker", "git"):
                path = root / name
                path.write_text("#!/bin/sh\nexit 0\n")
                path.chmod(0o700)
                binaries[name] = str(path)
            original_node = capture.inv04.NODE_IMAGE
            node_digest = original_node.rsplit("@", 1)[1]
            existing = set()
            mutation_commands = []
            api_counts = {capture.API_POD: 0, capture.API_NODES: 0, "events": 0}
            admin_kubeconfig = None
            def fake_runner(argv, timeout, env=None, max_output=None):
                self.assertLessEqual(timeout, capture.EXECUTION_SECONDS)
                self.assertLessEqual(max_output, capture.MAX_COMMAND_OUTPUT)
                if Path(argv[0]).name == "git":
                    if "rev-parse" in argv:
                        return 0, b"a" * 40 + b"\n", b""
                    return 0, b"", b""
                name = Path(argv[0]).name
                args = argv[1:]
                if name == "docker" and args[:2] == ["context", "show"]:
                    return 0, b"local\n", b""
                if name == "docker" and args[:2] == ["context", "inspect"]:
                    return 0, b'"unix:///var/run/docker.sock"', b""
                if name == "kind" and args == ["version"]:
                    return 0, ("kind " + capture.inv04.KIND_VERSION + "\n").encode(), b""
                if name == "docker" and args[:2] == ["image", "inspect"]:
                    self.assertEqual(env.get("DOCKER_CONTEXT"), "local")
                    return 0, json.dumps([{"RepoDigests": ["kindest/node@" + node_digest]}]).encode(), b""
                if name == "kind" and args == ["get", "clusters"]:
                    return 0, ("\n".join(sorted(existing)) + "\n").encode(), b""
                if name == "kind" and args[:3] == ["create", "cluster", "--name"]:
                    self.assertEqual(env.get("DOCKER_CONTEXT"), "local")
                    cluster = args[3]
                    existing.add(cluster)
                    kube = Path(args[args.index("--kubeconfig") + 1])
                    nonlocal admin_kubeconfig
                    admin_kubeconfig = kube
                    kube.write_text("admin-private-material")
                    return 0, b"created\n", b""
                if name == "kubectl" and "apply" in args:
                    return 0, b"resources configured\n", b""
                if name == "kubectl" and "config" in args:
                    return 0, json.dumps({"clusters": [{"cluster": {"server": "https://127.0.0.1:6443",
                        "certificate-authority-data": "Q0E="}}], "users": [{"user": {"token": "admin-token"}}]}).encode(), b""
                if name == "kubectl" and "token" in args:
                    return 0, b"observer-secret-token\n", b""
                if name == "kubectl" and "label" in args:
                    mutation_commands.append(args)
                    return 0, b"node labeled\n", b""
                if name == "kubectl" and "wait" in args:
                    return 0, b"pod condition met\n", b""
                if name == "kind" and args[:2] == ["delete", "cluster"]:
                    existing.discard(args[args.index("--name") + 1])
                    admin_kubeconfig.write_text("cleanup rewrote admin context")
                    return 0, b"deleted\n", b""
                raise AssertionError(argv)

            def api_reader(server, ca, token, path, timeout=10):
                self.assertEqual(server, "https://127.0.0.1:6443")
                self.assertLessEqual(timeout, 10)
                if path == capture.API_POD:
                    api_counts[path] += 1
                    return 200, encoded(pod(scheduled=api_counts[path] >= 3)), 0.001
                if path == capture.API_NODES:
                    api_counts[path] += 1
                    return 200, encoded(nodes(labelled=api_counts[path] == 2)), 0.001
                self.assertTrue(path.startswith(capture.API_EVENTS_PREFIX))
                api_counts["events"] += 1
                return 200, encoded(events()), 0.001

            with patch.object(capture.inv04, "_command", side_effect=lambda name: binaries[name]), \
                 patch.object(capture, "cluster_name", return_value="scout-pre02-test-cluster"):
                result = capture.capture(shared, out, runner=fake_runner, api_reader=api_reader)
            if result:
                self.fail(json.loads((out / "provenance.json").read_text()).get("error"))
            self.assertEqual(result, 0)
            self.assertEqual(existing, set())
            self.assertEqual(len(mutation_commands), 1)
            self.assertEqual(api_counts, {capture.API_POD: 3, capture.API_NODES: 2, "events": 3})
            receipt = json.loads((out / "provenance.json").read_text())
            self.assertEqual(receipt["status"], "passed")
            self.assertTrue(receipt["cleanupVerified"])
            self.assertTrue(receipt["transition"]["podSpecUnchangedExceptSchedulerNodeName"])
            self.assertEqual(receipt["claims"]["cloudAPI"], "not observed")
            self.assertTrue(receipt["claims"]["prerequisite"].startswith("verified:"))
            self.assertNotEqual(receipt["privateKubeconfigSha256"]["before"]["admin"],
                                receipt["privateKubeconfigSha256"]["afterCleanup"]["admin"])
            self.assertEqual(receipt["privateKubeconfigSha256"]["before"]["admin"],
                             receipt["privateKubeconfigSha256"]["beforeCleanup"]["admin"])
            self.assertEqual(receipt["sharedKubeconfigSha256"]["before"], receipt["sharedKubeconfigSha256"]["after"])
            self.assertNotIn(b"observer-secret-token", (out / "provenance.json").read_bytes())
            for filename in ("before-pod.json", "before-nodes.json", "before-events.json",
                             "after-pod.json", "after-nodes.json", "after-events.json"):
                self.assertTrue((out / filename).is_file())
            self.assertEqual((out / "fixture.yaml").read_bytes(), capture.MANIFEST)


if __name__ == "__main__":
    unittest.main()
