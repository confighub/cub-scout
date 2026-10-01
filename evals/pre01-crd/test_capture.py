"""Offline source and semantic guards for PRE-01's bounded CRD capture."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("pre01_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(capture)


class SourceAndEvidenceTests(unittest.TestCase):
    def test_pinned_sources_parse_one_exact_authored_service_monitor_and_crd(self):
        value = os.environ.get("HELM_EXPT_SOURCE")
        if not value:
            self.skipTest("set HELM_EXPT_SOURCE to the clean pinned read-only source checkout")
        checkout = Path(value)
        sources = capture.extract_pinned_sources(checkout, capture.SOURCE_REVISION, capture.SOURCE_HASHES)
        self.assertEqual(sources["selectedServiceMonitor"]["namespace"], "monitoring")
        self.assertEqual(sources["selectedServiceMonitor"]["name"], capture.RESOURCE_NAME)
        self.assertEqual(sources["selectedCRD"]["name"], capture.CRD_NAME)
        self.assertEqual(sources["sourceFiles"][capture.CRD_PATH]["bytes"], 3870160)
        self.assertTrue(sources["selectedCRD"]["normalizedYamlSha256"])
        self.assertIn("changes formatting", sources["normalization"])

    def test_variant_record_requires_typed_external_crd_path(self):
        entry = {"name": capture.CRD_NAME, "type": "requiredCRDs", "required": True,
                 "details": {"name": capture.CRD_NAME, "packagePath": capture.CRD_PATH}}
        capture.validate_variant_record([{"spec": {"inputs": {"installTime": [entry]}}}])
        deceptive = {"spec": {"notes": "mentions " + capture.CRD_NAME,
            "inputs": {"installTime": [{"name": capture.CRD_NAME, "type": "requiredCRDs", "required": False,
                "details": {"name": capture.CRD_NAME, "packagePath": "other.yaml"}}]}}}
        with self.assertRaises(capture.CaptureError): capture.validate_variant_record([deceptive])

    def test_service_monitor_wrong_identity_or_duplicate_is_rejected(self):
        obj = {"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
               "metadata": {"name": "wrong", "namespace": "monitoring"}}
        with self.assertRaises(capture.CaptureError):
            capture.validate_service_monitor(obj)
        obj["metadata"]["name"] = capture.RESOURCE_NAME
        obj["apiVersion"] = "monitoring.coreos.com/v2"
        with self.assertRaises(capture.CaptureError):
            capture.validate_service_monitor(obj)
        obj["apiVersion"] = "monitoring.coreos.com/v1"
        with self.assertRaises(capture.CaptureError):
            capture.select_service_monitor([obj, obj])
        with self.assertRaises(capture.CaptureError):
            capture.extract_pinned_sources(Path("/does/not/matter"), capture.SOURCE_REVISION,
                {**capture.SOURCE_HASHES, capture.SERVICE_MONITOR_PATH: "0" * 64})
        with self.assertRaises(capture.CaptureError):
            capture.extract_pinned_sources(Path("/does/not/matter"), "0" * 40, capture.SOURCE_HASHES)

    def test_crd_must_be_real_matching_v1_and_established(self):
        crd = {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
               "metadata": {"name": capture.CRD_NAME},
               "spec": {"group": "monitoring.coreos.com", "scope": "Namespaced",
                        "names": {"kind": "ServiceMonitor", "plural": "servicemonitors"},
                        "versions": [{"name": "v1", "served": True, "storage": True}]}}
        capture.validate_crd(crd)
        bad = json.loads(json.dumps(crd)); bad["metadata"]["name"] = "other"
        with self.assertRaises(capture.CaptureError): capture.validate_crd(bad)
        raw = {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
               "metadata": {"name": capture.CRD_NAME, "uid": "crd-uid", "resourceVersion": "12"},
               "spec": crd["spec"], "status": {"conditions": [{"type": "Established", "status": "False"}]}}
        body = json.dumps(raw).encode()
        with self.assertRaises(capture.CaptureError): capture.validate_raw(capture.API_PATHS[0], 200, body, "present")
        raw["status"]["conditions"][0]["status"] = "True"
        self.assertTrue(capture.validate_raw(capture.API_PATHS[0], 200, json.dumps(raw).encode(), "present")["established"])

    def test_404_is_absence_but_403_or_present_phase_404_is_unknown(self):
        not_found = json.dumps({"apiVersion": "v1", "kind": "Status", "status": "Failure",
                                "reason": "NotFound", "code": 404}).encode()
        self.assertEqual(capture.validate_raw(capture.API_PATHS[0], 404, not_found, "absent")["status"], "absent")
        for code, phase in ((403, "absent"), (404, "present")):
            with self.subTest(code=code, phase=phase), self.assertRaises(capture.CaptureError):
                capture.validate_raw(capture.API_PATHS[0], code, not_found, phase)
        with self.assertRaises(capture.CaptureError):
            capture.validate_raw(capture.API_PATHS[0], 200, b"{}", "absent")
        self.assertEqual(capture.validate_raw(capture.API_PATHS[2], 404, not_found, "present-before-apply")["status"], "absent")

    def test_discovery_and_object_identity_are_exact(self):
        absent = {"kind": "APIResourceList", "groupVersion": "monitoring.coreos.com/v1", "resources": []}
        self.assertEqual(capture.validate_raw(capture.API_PATHS[1], 200, json.dumps(absent).encode(), "absent")["status"], "absent")
        absent["resources"] = [{"name": "servicemonitors", "kind": "ServiceMonitor"}]
        with self.assertRaises(capture.CaptureError): capture.validate_raw(capture.API_PATHS[1], 200, json.dumps(absent).encode(), "absent")
        present = {"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
                   "metadata": {"name": capture.RESOURCE_NAME, "namespace": capture.NAMESPACE, "uid": "sm-uid", "resourceVersion": "9"}}
        self.assertEqual(capture.validate_raw(capture.API_PATHS[2], 200, json.dumps(present).encode(), "present")["uid"], "sm-uid")
        present["metadata"]["uid"] = ""
        with self.assertRaises(capture.CaptureError): capture.validate_raw(capture.API_PATHS[2], 200, json.dumps(present).encode(), "present")

    def test_typed_scout_receipt_validates_only_exact_crd_fact(self):
        statement = {"predicate": {"predicateName": "prerequisites-met", "verdict": "BLOCK",
            "evidence": {"prerequisites": {"facts": [{"kind": "CRD", "name": capture.CRD_NAME, "status": "missing"}],
                "summary": {"required": 1, "present": 0, "missing": 1, "inconclusive": 0}}}}}
        self.assertEqual(capture.prerequisite_receipt_valid(0, json.dumps(statement).encode(), "missing")["verdict"], "BLOCK")
        for code in (1, -15, True):
            with self.subTest(exit_code=code), self.assertRaises(capture.CaptureError):
                capture.prerequisite_receipt_valid(code, json.dumps(statement).encode(), "missing")
        for malformed in ([], {"predicate": []}, {"predicate": {"predicateName": "prerequisites-met", "evidence": {"prerequisites": []}}}):
            with self.subTest(shape=malformed), self.assertRaises(capture.CaptureError):
                capture.prerequisite_receipt_valid(0, json.dumps(malformed).encode(), "missing")
        summary = statement["predicate"]["evidence"]["prerequisites"]["summary"]
        for malformed in (
            {"required": True, "present": 0, "missing": 1, "inconclusive": 0},
            {"required": 1, "present": 1, "missing": 1, "inconclusive": 0},
            {"required": 1, "present": 0, "missing": 0, "inconclusive": 1},
            {"required": 1, "present": 0, "missing": 1},
            [],
        ):
            statement["predicate"]["evidence"]["prerequisites"]["summary"] = malformed
            with self.subTest(summary=malformed), self.assertRaises(capture.CaptureError):
                capture.prerequisite_receipt_valid(0, json.dumps(statement).encode(), "missing")
        statement["predicate"]["evidence"]["prerequisites"]["summary"] = summary
        statement["predicate"]["evidence"]["prerequisites"]["facts"].append({"kind": "Secret", "name": "x", "status": "present"})
        with self.assertRaises(capture.CaptureError): capture.prerequisite_receipt_valid(0, json.dumps(statement).encode(), "missing")

    def test_observer_is_loopback_scoped_and_api_paths_are_allowlisted(self):
        config = json.loads(capture.observer_kubeconfig("https://127.0.0.1:6443", "Q0E=", "test-token"))
        self.assertEqual(config["current-context"], "pre01-observer")
        self.assertEqual(config["contexts"][0]["context"]["namespace"], "monitoring")
        self.assertNotIn("client-key", json.dumps(config))
        with self.assertRaises(capture.CaptureError): capture.observer_kubeconfig("https://example.com:6443", "Q0E=", "token")
        with self.assertRaises(capture.CaptureError): capture.check_api_path("/api/v1/secrets")

    def test_credential_and_binary_integrity_checks_fail_closed(self):
        self.assertFalse(capture.credentials_absent(b"observer-token", "observer-token", ()))
        self.assertFalse(capture.credentials_absent(b"private-key-bytes", "", (b"private-key-bytes",)))
        self.assertTrue(capture.credentials_absent(b"safe", "token", (b"private",)))
        with tempfile.TemporaryDirectory() as td:
            binary = Path(td) / "scout"
            binary.write_bytes(b"unreviewed"); binary.chmod(0o700)
            with self.assertRaises(capture.CaptureError):
                capture.verify_scout_binary(binary, "0" * 64, "eec6d442279955af0f1fc39c637252ac8eb0f081",
                                            "eec6d442279955af0f1fc39c637252ac8eb0f081")
            # The right bytes/digest cannot bless a caller-selected source revision.
            pinned_sha = "7d20aa7bb33b477dfd88afb2f53b1a6f773a5a4ffbcff7f81d8dcbfeb5a12bab"
            with patch.object(capture, "sha256", return_value=pinned_sha):
                with self.assertRaises(capture.CaptureError):
                    capture.verify_scout_binary(binary, pinned_sha, "a" * 40, "a" * 40)
        key = b"test-private-key-material"
        import base64
        encoded = base64.b64encode(key).decode()
        material = capture.private_material({"users": [{"user": {"client-key-data": encoded}}]}, b"config")
        self.assertFalse(capture.credentials_absent(key, "", material))

    def test_new_output_and_owned_cleanup_marker_are_exact(self):
        with tempfile.TemporaryDirectory() as td:
            output = Path(td) / "out"
            self.assertTrue(capture.prepare_output(output).is_dir())
            with self.assertRaises(capture.CaptureError): capture.prepare_output(output)
            alias = Path(td) / "alias"; alias.symlink_to(output, target_is_directory=True)
            with self.assertRaises(capture.CaptureError): capture.prepare_output(alias)
        name = capture.owned_cluster_name()
        marker = {"schema": "pre01-owned-cluster-marker.v1", "clusterName": name, "ownerPid": 31}
        self.assertTrue(capture.owns_cluster(marker, name, 31))
        self.assertFalse(capture.owns_cluster(marker, name, 32))
        self.assertFalse(capture.owns_cluster(marker, "shared-cluster", 31))

    def test_failed_raw_acceptance_retains_exact_status_and_body(self):
        body = json.dumps({"apiVersion": "v1", "kind": "Status", "reason": "Forbidden", "code": 403}).encode()
        with tempfile.TemporaryDirectory() as td:
            output = Path(td); records = []
            with patch.object(capture, "api_get", return_value=(403, body, 0.01)):
                with self.assertRaises(capture.CaptureError):
                    capture.capture_api("absent", capture.API_PATHS[0], "crd.json", "https://127.0.0.1:6443",
                                        b"ca", "token", (), output, records)
            self.assertEqual((output / "crd.json").read_bytes(), body)
            self.assertEqual(records[0]["httpStatus"], 403)
            self.assertIn("error", records[0])

    def test_private_response_body_is_never_written(self):
        with tempfile.TemporaryDirectory() as td:
            output = Path(td); records = []
            with patch.object(capture, "api_get", return_value=(200, b"observer-token", 0.01)):
                with self.assertRaises(capture.CaptureError):
                    capture.capture_api("absent", capture.API_PATHS[0], "private.json", "https://127.0.0.1:6443",
                                        b"ca", "observer-token", (), output, records)
            self.assertFalse((output / "private.json").exists())
            self.assertIn("error", records[0])

    def test_scout_receipt_uses_explicit_private_kubeconfig_without_unsupported_context_flag(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td); output = root / "out"; output.mkdir()
            scout = root / "scout"; scout.write_bytes(b"test")
            observer = root / "observer.kubeconfig"; observer.write_bytes(b"private-config")
            stmt = {"predicate": {"predicateName": "prerequisites-met", "verdict": "BLOCK",
                "evidence": {"prerequisites": {"facts": [{"kind": "CRD", "name": capture.CRD_NAME, "status": "missing"}],
                    "summary": {"required": 1, "present": 0, "missing": 1, "inconclusive": 0}}}}}
            with patch.object(capture.inv04, "run_bounded", return_value=(0, json.dumps(stmt).encode(), b"")) as run:
                result = capture.run_scout_receipt(scout, root / "admin", observer, "absent",
                    "eec6d442279955af0f1fc39c637252ac8eb0f081", "missing", output, "token", (b"admin-private",))
            argv = run.call_args.args[0]
            env = run.call_args.args[2]
            self.assertNotIn("--context", argv)
            self.assertEqual(env["KUBECONFIG"], str(observer))
            self.assertEqual(result["validation"]["factStatus"], "missing")

    def test_dependent_apply_success_is_registration_not_health(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td); output = root / "out"; output.mkdir()
            manifest = output / "sm.yaml"; manifest.write_text("{}\n")
            outputs = (("absent", 1, b"", b'no matches for kind "ServiceMonitor" in version "monitoring.coreos.com/v1"'),
                       ("present", 0, b"servicemonitor.monitoring.coreos.com/kube-prometheus-stack-kube-state-metrics created\n", b""))
            for phase, code, stdout, stderr in outputs:
                with self.subTest(phase=phase), patch.object(capture.inv04, "run_bounded", return_value=(code, stdout, stderr)):
                    record = capture.run_apply("kubectl", root / "admin", "owned", manifest,
                        root / ("cache-" + phase), output, phase, "token", ())
                self.assertTrue(record["accepted"])
                self.assertFalse(record["healthProven"])
                self.assertIn("registration only", record["meaning"])
            denied_output = root / "denied-output"; denied_output.mkdir()
            with patch.object(capture.inv04, "run_bounded", return_value=(1, b"", b"Error: Forbidden")):
                denied = capture.run_apply("kubectl", root / "admin", "owned", manifest,
                    root / "cache-denied", denied_output, "absent", "token", ())
            self.assertFalse(denied["accepted"], "arbitrary apply failure cannot prove the missing-CRD cause")

    def test_raw_request_body_limit_is_enforced(self):
        class Response:
            status = 200
            def __init__(self): self.remaining = capture.MAX_BODY + 1
            def __enter__(self): return self
            def __exit__(self, *_args): return False
            def read1(self, size):
                count = min(size, self.remaining)
                self.remaining -= count
                return b"x" * count
        class Opener:
            def open(self, *_args, **_kwargs): return Response()
        with patch.object(capture.ssl, "create_default_context", return_value=object()), \
             patch.object(capture.urllib.request, "build_opener", return_value=Opener()):
            with self.assertRaises(capture.inv04.CaptureError):
                capture.api_get("https://127.0.0.1:6443", b"unused", "token", capture.API_PATHS[0])

    def test_full_lifecycle_with_mocked_commands_and_api(self):
        """Exercise both serial phases, private config handling, receipts and owned cleanup offline."""
        import types
        with tempfile.TemporaryDirectory() as td:
            root = Path(td); output = root / "output"; output.mkdir()
            shared = root / "shared.kubeconfig"; shared.write_bytes(b"shared-stable")
            scout = root / "cub-scout"; scout.write_bytes(b"pinned test executable")
            scout.chmod(0o700)
            sm = {"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
                  "metadata": {"name": capture.RESOURCE_NAME, "namespace": capture.NAMESPACE}}
            crd = {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
                   "metadata": {"name": capture.CRD_NAME},
                   "spec": {"group": "monitoring.coreos.com", "scope": "Namespaced",
                            "names": {"kind": "ServiceMonitor", "plural": "servicemonitors"},
                            "versions": [{"name": "v1", "served": True, "storage": True}]}}
            sources = {"sourceRevision": capture.SOURCE_REVISION,
                "sourceFiles": {"record": {"sha256": "a" * 64, "bytes": 1}},
                "selectedServiceMonitor": {"apiVersion": sm["apiVersion"], "kind": sm["kind"],
                    "namespace": capture.NAMESPACE, "name": capture.RESOURCE_NAME,
                    "sourceObjectSha256": "b" * 64, "normalizedYamlSha256": "c" * 64,
                    "normalizedYaml": capture.yaml.safe_dump(sm, sort_keys=False).encode()},
                "selectedCRD": {"apiVersion": crd["apiVersion"], "kind": crd["kind"], "name": capture.CRD_NAME,
                    "sourceObjectSha256": "d" * 64, "normalizedYamlSha256": "e" * 64,
                    "normalizedYaml": capture.yaml.safe_dump(crd, sort_keys=False).encode()},
                "normalization": "normalized for mock", "normalizer": {"name": "PyYAML", "version": capture.yaml.__version__},
                "prerequisiteRecordChecked": True}
            raw_crd = json.loads(json.dumps(crd))
            raw_crd["metadata"].update({"uid": "crd-uid", "resourceVersion": "12"})
            raw_crd["status"] = {"conditions": [{"type": "Established", "status": "True"}]}
            raw_sm = {"apiVersion": sm["apiVersion"], "kind": sm["kind"],
                "metadata": {"name": capture.RESOURCE_NAME, "namespace": capture.NAMESPACE,
                             "uid": "sm-uid", "resourceVersion": "42"}}
            not_found = json.dumps({"apiVersion": "v1", "kind": "Status", "status": "Failure",
                                    "reason": "NotFound", "code": 404}).encode()
            bodies = {
                capture.API_PATHS[0]: [(404, not_found), (404, not_found), (200, json.dumps(raw_crd).encode())],
                capture.API_PATHS[1]: [(404, not_found), (200, json.dumps({"kind": "APIResourceList",
                    "groupVersion": "monitoring.coreos.com/v1", "resources": [{"name": "servicemonitors", "kind": "ServiceMonitor"}]}).encode())],
                capture.API_PATHS[2]: [(404, not_found), (404, not_found), (404, not_found), (200, json.dumps(raw_sm).encode())],
            }
            state = {"created": False, "deleted": False}
            cluster_name = "scout-pre01-mock-20261001000000-12345678"
            def fake_call(name, args, timeout, env=None):
                if name == "kind" and args[:2] == ["get", "clusters"]:
                    return (cluster_name + "\n").encode() if state["created"] else b""
                if name == "kind" and args[:2] == ["delete", "cluster"]:
                    state["created"] = False; state["deleted"] = True; return b""
                if name == "kind" and args == ["version"]: return b"kind v0.31.0 test\n"
                if name == "kubectl" and "config" in args: return json.dumps({"clusters": [{"cluster": {
                    "server": "https://127.0.0.1:6443", "certificate-authority-data": "Q0E="}}],
                    "users": []}).encode()
                if name == "kubectl" and "token" in args: return b"observer-token\n"
                return b""
            def fake_run(args, timeout, env=None, max_output=capture.inv04.MAX_COMMAND_OUTPUT):
                args = [str(a) for a in args]
                if "rev-parse" in args: return 0, b"a" * 40 + b"\n", b""
                if "status" in args: return 0, b"", b""
                if args[0] == "kind" and "create" in args:
                    state["created"] = True
                    admin_path = Path(args[args.index("--kubeconfig") + 1]); admin_path.write_bytes(b"private-admin-config")
                    return 0, b"created\n", b""
                if args[0] == "kubectl" and "apply" in args and "-f" in args:
                    manifest = args[args.index("-f") + 1]
                    if manifest.endswith("observer-setup.jsonl"): return 0, b"", b""
                    if any("cache-absent" in value for value in args):
                        return 1, b"", b'no matches for kind "ServiceMonitor" in version "monitoring.coreos.com/v1"'
                    if any("cache-present" in value for value in args):
                        return 0, b"servicemonitor.monitoring.coreos.com/kube-prometheus-stack-kube-state-metrics created\n", b""
                if args[0] == "kubectl" and "create" in args and "-f" in args: return 0, b"created\n", b""
                if args[0] == "kubectl" and "wait" in args: return 0, b"condition met\n", b""
                return 0, b"", b""
            def fake_api(_server, _ca, _token, path, timeout=15.0):
                status, body = bodies[path].pop(0)
                return status, body, 0.001
            args = types.SimpleNamespace(source_checkout=root, source_revision=capture.SOURCE_REVISION,
                                         scout_source_revision="eec6d442279955af0f1fc39c637252ac8eb0f081",
                                         expected_scout_sha256=capture.sha256(scout.read_bytes()))
            with patch.object(capture.inv04, "require_local_docker"), \
                 patch.object(capture.inv04, "require_kind_version"), \
                 patch.object(capture.inv04, "verified_node_repo_digest"), \
                 patch.object(capture.inv04, "_command", side_effect=lambda name: name), \
                 patch.object(capture, "owned_cluster_name", return_value=cluster_name), \
                 patch.object(capture.inv04, "_call", side_effect=fake_call), \
                 patch.object(capture.inv04, "run_bounded", side_effect=fake_run), \
                 patch.object(capture.inv04, "_arm_capture_signals", return_value=object()), \
                 patch.object(capture.inv04, "_suppress_capture_signals"), \
                 patch.object(capture.inv04, "_restore_capture_signals"), \
                 patch.object(capture, "api_get", side_effect=fake_api), \
                 patch.object(capture, "run_scout_receipt", side_effect=lambda _s, _a, _o, phase, *_args: {"phase": phase, "isRawModelEvidence": False}):
                try:
                    result = capture.capture(args, sources, output, shared, scout)
                except capture.CaptureError:
                    print((output / "provenance.json").read_text())
                    raise
                self.assertEqual(result, 0)
            provenance = json.loads((output / "provenance.json").read_bytes())
            self.assertTrue(provenance["cleanupVerified"])
            self.assertEqual(provenance["sharedKubeconfigSha256"]["before"], provenance["sharedKubeconfigSha256"]["after"])
            self.assertEqual([op["accepted"] for op in provenance["operations"] if op.get("phase") in ("absent", "present")], [True, True])
            self.assertEqual([record["phase"] for record in provenance["apiObservations"] if record["path"] == capture.API_PATHS[2]],
                             ["absent", "absent", "present-before-apply", "present"])
            self.assertEqual(len(bodies[capture.API_PATHS[0]]), 0)


if __name__ == "__main__":
    unittest.main()
