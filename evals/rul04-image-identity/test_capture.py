#!/usr/bin/env python3
"""Offline semantic guards for RUL-04 raw image-identity capture."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("rul04_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(capture)


def statefulset():
    return {"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":capture.WORKLOAD,
        "namespace":capture.NAMESPACE,"uid":"ss-uid","resourceVersion":"42","generation":1},
        "spec":{"replicas":1,"selector":{"matchLabels":{"app":capture.WORKLOAD}},
            "template":{"spec":{"containers":[{"name":"pause","image":capture.IMAGE}]}}},
        "status":{"observedGeneration":1,"readyReplicas":1,"currentReplicas":1,"replicas":1}}


def pod(image_id="docker-pullable://pause@sha256:"+"a"*64):
    return {"apiVersion":"v1","kind":"PodList","metadata":{"resourceVersion":"43"},"items":[{
        "metadata":{"name":capture.WORKLOAD+"-0","namespace":capture.NAMESPACE,"uid":"pod-uid",
            "resourceVersion":"43","ownerReferences":[{"apiVersion":"apps/v1","kind":"StatefulSet",
                "name":capture.WORKLOAD,"uid":"ss-uid","controller":True}]},
        "spec":{"containers":[{"name":"pause","image":capture.IMAGE}]},
        "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],
            "containerStatuses":[{"name":"pause","image":capture.IMAGE,"imageID":image_id,"ready":True,
                "state":{"running":{"startedAt":"2026-10-01T00:00:00Z"}}}]}}]}


class CaptureValidationTests(unittest.TestCase):
    def test_boolean_counters_and_missing_collection_version_are_rejected(self):
        for section,key in (("spec","replicas"),("status","observedGeneration"),("status","readyReplicas"),("status","currentReplicas"),("status","replicas")):
            obj=statefulset(); obj[section][key]=True
            with self.subTest(section=section,key=key),self.assertRaises(capture.inv04.CaptureError):
                capture.validate_statefulset(json.dumps(obj).encode())
        obj=pod(); del obj["metadata"]["resourceVersion"]
        with self.assertRaises(capture.inv04.CaptureError):
            capture.validate_pods(json.dumps(obj).encode(),{"uid":"ss-uid"})

    def test_empty_stderr_is_not_a_credential_leak(self):
        self.assertTrue(capture.credentials_absent(b"", "token", (b"private",)))

    def test_plain_digest_runtime_id_is_supported(self):
        self.assertTrue(capture.valid_image_id("sha256:"+"a"*64))

    def test_empty_diagnostics_are_safe_and_individual_private_keys_are_not(self):
        import base64
        key=b"PRIVATE TEST KEY CONTENT"
        encoded=base64.b64encode(key)
        config=b"users:\n- user:\n    client-key-data: "+encoded+b"\n"
        material=capture.private_material({},config)
        self.assertTrue(capture.credentials_absent(b"", "token", material))
        self.assertFalse(capture.credentials_absent(key, "token", material))
        self.assertFalse(capture.credentials_absent(encoded, "token", material))
        self.assertTrue(capture.valid_image_id("sha256:"+"a"*64))
        malformed=pod(); malformed["items"][0]["metadata"]=None
        with self.assertRaises(capture.inv04.CaptureError):
            capture.validate_pods(json.dumps(malformed).encode(),{"uid":"ss-uid"})

    def test_intent_requires_tag_only_and_records_no_digest(self):
        info=capture.validate_statefulset(json.dumps(statefulset()).encode())
        self.assertFalse(info["intendedImageHasImmutableDigest"])
        self.assertEqual(info["intendedImage"],capture.IMAGE)

    def test_digest_intent_is_rejected(self):
        obj=statefulset(); obj["spec"]["template"]["spec"]["containers"][0]["image"] += "@sha256:"+"a"*64
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_statefulset(json.dumps(obj).encode())

    def test_pod_runtime_image_id_must_be_preserved(self):
        ss=capture.validate_statefulset(json.dumps(statefulset()).encode())
        info=capture.validate_pods(json.dumps(pod()).encode(),ss)
        self.assertEqual(info["runtimeImageID"],pod()["items"][0]["status"]["containerStatuses"][0]["imageID"])
        self.assertIn("does not identify",info["identityConclusion"])

    def test_missing_image_id_rejects_capture_instead_of_fabricating_absence(self):
        ss=capture.validate_statefulset(json.dumps(statefulset()).encode())
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_pods(json.dumps(pod("")).encode(),ss)

    def test_uid_mismatch_or_not_ready_pod_is_rejected(self):
        ss=capture.validate_statefulset(json.dumps(statefulset()).encode())
        wrong=pod(); wrong["items"][0]["metadata"]["ownerReferences"][0]["uid"]="other"
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_pods(json.dumps(wrong).encode(),ss)
        not_ready=pod(); not_ready["items"][0]["status"]["conditions"][0]["status"]="False"
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_pods(json.dumps(not_ready).encode(),ss)

    def test_cluster_name_is_owned_and_below_node_label_limit(self):
        name=capture.owned_cluster_name()
        self.assertRegex(name,r"^scout-rul04-[0-9a-z-]+$")
        self.assertLessEqual(len(name+"-control-plane"),63)
        marker={"schema":"rul04-owned-cluster-marker.v1","clusterName":name,"ownerPid":55}
        self.assertTrue(capture.owns_cluster(marker,name,55))
        self.assertFalse(capture.owns_cluster(marker,"some-shared-cluster",55))
        self.assertFalse(capture.owns_cluster({**marker,"ownerPid":56},name,55))

    def test_output_must_be_new_and_non_symlink(self):
        with tempfile.TemporaryDirectory() as td:
            root=Path(td); fresh=root/"fresh"
            made=capture.prepare_output(fresh)
            self.assertTrue(made.is_dir())
            with self.assertRaises(capture.inv04.CaptureError): capture.prepare_output(fresh)
            alias=root/"alias"; alias.symlink_to(made,target_is_directory=True)
            with self.assertRaises(capture.inv04.CaptureError): capture.prepare_output(alias)

    def test_api_host_and_exact_paths_are_allowlisted(self):
        capture.check_server("https://127.0.0.1:6443")
        for server in ("http://127.0.0.1:6443","https://example.com:6443","https://127.0.0.1:6443/path"):
            with self.subTest(server=server),self.assertRaises(capture.inv04.CaptureError): capture.check_server(server)
        for path in (capture.API_PREFIXES[0],capture.API_PREFIXES[1]): capture.check_path(path)
        with self.assertRaises(capture.inv04.CaptureError): capture.check_path("/api/v1/secrets")

    def test_observer_context_is_rul04_scoped_and_minimal(self):
        cfg=json.loads(capture.observer_config("https://127.0.0.1:6443","Q0E=","observer-secret-token"))
        self.assertEqual(cfg["current-context"],"rul04-observer")
        self.assertEqual(cfg["contexts"][0]["context"]["namespace"],capture.NAMESPACE)
        self.assertEqual(cfg["users"],[{"name":"rul04-observer","user":{"token":"observer-secret-token"}}])
        self.assertNotIn("client-key",json.dumps(cfg))

    def test_hash_and_credential_guards(self):
        with tempfile.TemporaryDirectory() as td:
            f=Path(td)/"binary"; f.write_bytes(b"pinned")
            self.assertEqual(capture.verify_hash(f,capture.inv04.sha256(b"pinned")),capture.inv04.sha256(b"pinned"))
            with self.assertRaises(capture.inv04.CaptureError): capture.verify_hash(f,"0"*64)
        self.assertTrue(capture.credentials_absent(b"safe output","secret",(b"private",)))
        self.assertFalse(capture.credentials_absent(b"secret", "secret",()))
        self.assertFalse(capture.credentials_absent(b"private", "",(b"private",)))
        self.assertTrue(capture.valid_image_id("containerd://sha256:"+"a"*64))
        self.assertFalse(capture.valid_image_id("image:latest"))

    def test_failed_http_response_body_and_status_are_preserved_in_provenance(self):
        records=[]
        with tempfile.TemporaryDirectory() as td:
            out=Path(td)
            body=json.dumps({"kind":"Status","code":403,"reason":"Forbidden"}).encode()
            with patch.object(capture,"request",return_value=(403,body,0.01)):
                with self.assertRaises(capture.inv04.CaptureError):
                    capture.capture_raw_observation("statefulset.json",capture.API_PREFIXES[0],"https://127.0.0.1:6443",b"ca","observer-secret-token",None,records,out,())
            self.assertEqual(records[0]["httpStatus"],403)
            self.assertIn("error",records[0])
            self.assertEqual((out/"statefulset.json").read_bytes(),body)

    def test_statefulset_requires_complete_ready_status_and_pod_type_state(self):
        obj=statefulset(); obj["status"]["observedGeneration"]=0
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_statefulset(json.dumps(obj).encode())
        ss=capture.validate_statefulset(json.dumps(statefulset()).encode())
        bad=pod(); bad["items"][0]["spec"]["containers"][0]["image"]="pause:latest"
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_pods(json.dumps(bad).encode(),ss)
        bad=pod(); bad["items"][0]["status"]["containerStatuses"][0]["state"]={"waiting":{}}
        with self.assertRaises(capture.inv04.CaptureError): capture.validate_pods(json.dumps(bad).encode(),ss)

    def test_scout_must_return_expected_machine_readable_unknown(self):
        good={"runningImage":{"verdict":"unknown","workloads":[{"reason":"workload-ownership-unsupported"}]}}
        self.assertEqual(capture.validate_scout_result(0,json.dumps(good).encode())["verdict"],"unknown")
        for code,out in ((1,json.dumps(good).encode()),(0,b"{}"),(0,json.dumps({"runningImage":{"verdict":"match","workloads":[]}}).encode())):
            with self.subTest(code=code),self.assertRaises(capture.inv04.CaptureError): capture.validate_scout_result(code,out)


if __name__ == "__main__":
    unittest.main()
