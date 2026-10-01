#!/usr/bin/env python3
"""Offline semantic guards for RUL-04 raw image-identity capture."""
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("rul04_capture", Path(__file__).with_name("capture.py"))
capture = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(capture)


def statefulset():
    return {"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":capture.WORKLOAD,
        "namespace":capture.NAMESPACE,"uid":"ss-uid","resourceVersion":"42","generation":1},
        "spec":{"replicas":1,"selector":{"matchLabels":{"app":capture.WORKLOAD}},
            "template":{"spec":{"containers":[{"name":"pause","image":capture.IMAGE}]}}},
        "status":{"observedGeneration":1,"readyReplicas":1}}


def pod(image_id="docker-pullable://pause@sha256:"+"a"*64):
    return {"apiVersion":"v1","kind":"PodList","metadata":{"resourceVersion":"43"},"items":[{
        "metadata":{"name":capture.WORKLOAD+"-0","namespace":capture.NAMESPACE,"uid":"pod-uid",
            "resourceVersion":"43","ownerReferences":[{"apiVersion":"apps/v1","kind":"StatefulSet",
                "name":capture.WORKLOAD,"uid":"ss-uid","controller":True}]},
        "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],
            "containerStatuses":[{"name":"pause","image":capture.IMAGE,"imageID":image_id,"ready":True}]}}]}


class CaptureValidationTests(unittest.TestCase):
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


if __name__ == "__main__":
    unittest.main()
