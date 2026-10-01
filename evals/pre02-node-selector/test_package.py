"""Offline integrity, strict-answer, and equal-arm guards for PRE-02."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent
FIXTURES = ROOT / "fixtures"
ARCHIVE = ROOT.parent / "results" / "pre02-node-selector-20261001"
RAW_HASHES = {
    "before-pod.json": "dec5c8fae0f71860f1309aee1be0455b355b6ea3abbb921360ebd306343adefd",
    "before-nodes.json": "9808755e4df180b936aa2b68db5ce84aef3bca84b61cc9b2db8a64b007e7374a",
    "before-events.json": "78b187ef42464644a815b8676bf8743fc7eb491f163f35b334d73bccf531f50e",
    "after-pod.json": "f264204dc296d06590bc357691a1b95db08222f4ac68b3c38935d6ef200832c3",
    "after-nodes.json": "263097f6d314cac3a7d673b4db7a38f36c437bd2fa1a2d402bb2c0a0dd807ee9",
    "after-events.json": "4acce317a3a539b80d4fbb7eb7977dc313a888952caa8b64f2096243d40ee3ae",
}
EXPECTED = {
    "prerequisite": "NODE_SELECTOR_LABEL",
    "before_pod_uid": "b5fa45bb-3aa4-4ec2-aa97-c329ea564317",
    "before_scheduling": "FALSE_UNSCHEDULABLE",
    "selector": "scout-pre02-zone=fixture-zone",
    "before_matching_nodes": "0",
    "before_event": "UID_CORRELATED_SELECTOR_MISMATCH",
    "after_pod_uid": "b5fa45bb-3aa4-4ec2-aa97-c329ea564317",
    "after_scheduling": "TRUE",
    "after_node": "scout-pre02-20261001060250-3cb9509cd3-control-plane",
    "after_selector_label": "scout-pre02-zone=fixture-zone",
    "historical_failed_scheduling_event": "RETAINED_NOT_CURRENT_FAILURE",
    "health_scope": "SCHEDULING_ONLY_NO_HEALTH_CONCLUSION",
    "cloud_api": "NOT_OBSERVED",
    "secret": "NOT_OBSERVED",
    "capacity": "NOT_INFERRED",
    "evidence": "capture-scope.json+before-pod.json+before-nodes.json+before-events.json+after-pod.json+after-nodes.json+after-events.json",
}


def file_sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def strict_pattern():
    grader = (ROOT / "graders" / "verified-answer.md").read_text()
    match = re.search(r"(?m)^pattern: '(.*)'$", grader)
    if not match:
        raise AssertionError("strict regex grader pattern is missing")
    return re.compile(match.group(1), re.S)


class Pre02PackageTests(unittest.TestCase):
    def test_capture_scope_report_and_raw_hashes_are_pinned(self):
        scope = json.loads((FIXTURES / "capture-scope.json").read_text())
        report = json.loads((ROOT.parent / "reports" / "2026-10-01-pre02-node-selector.json").read_text())
        self.assertEqual(scope["schema"], "pre02-node-selector-capture-scope.v1")
        self.assertFalse(scope["capture"]["atomicSnapshot"])
        self.assertEqual(scope["capture"]["sourceRevision"], "4b113710948882eda501e14aacca2d5cec1168ae")
        self.assertEqual(scope["capture"]["captureScriptSha256"], file_sha(ROOT / "capture.py"))
        self.assertEqual(set(scope["capture"]["rawFilesSha256"]), set(RAW_HASHES))
        poll = scope["capture"]["preObservationWait"]
        self.assertEqual(poll["attemptCount"], 1)
        self.assertFalse(any(request["bodyRetainedSeparately"] for request in poll["requests"]))
        self.assertEqual(poll["requests"][0]["bodySha256"], RAW_HASHES["before-pod.json"])
        self.assertEqual(poll["requests"][1]["bodySha256"], RAW_HASHES["before-events.json"])
        self.assertEqual(report["capture"]["rawFilesSha256"], RAW_HASHES)
        self.assertEqual(report["capture"]["captureScopeSha256"], file_sha(FIXTURES / "capture-scope.json"))
        self.assertEqual(report["capture"]["captureScriptSha256"], file_sha(ROOT / "capture.py"))
        self.assertEqual(report["capture"]["wrapper"]["exitCode"], 0)
        self.assertTrue(report["capture"]["cleanupVerified"])
        self.assertTrue(report["capture"]["sharedKubeconfigUnchanged"])
        self.assertFalse(report["limits"]["modelRun"])
        self.assertFalse(report["limits"]["benchmarkRun"])
        for name, expected in RAW_HASHES.items():
            with self.subTest(name=name):
                self.assertEqual(file_sha(FIXTURES / name), expected)
                self.assertEqual(file_sha(ARCHIVE / name), expected)
                self.assertEqual((FIXTURES / name).read_bytes(), (ARCHIVE / name).read_bytes())
        self.assertEqual(file_sha(ARCHIVE / "capture-scope.json"), file_sha(FIXTURES / "capture-scope.json"))

    def test_packaged_api_schemas_and_transition_match_capture_scope(self):
        raw = {name: json.loads((FIXTURES / name).read_text()) for name in RAW_HASHES}
        before_pod, after_pod = raw["before-pod.json"], raw["after-pod.json"]
        self.assertEqual((before_pod["apiVersion"], before_pod["kind"]), ("v1", "Pod"))
        self.assertEqual((after_pod["apiVersion"], after_pod["kind"]), ("v1", "Pod"))
        self.assertEqual(before_pod["metadata"]["uid"], after_pod["metadata"]["uid"])
        self.assertEqual(before_pod["spec"]["nodeSelector"], {"scout-pre02-zone": "fixture-zone"})
        self.assertNotIn("nodeName", before_pod["spec"])
        self.assertEqual(after_pod["status"]["conditions"][-1]["type"], "PodScheduled")
        self.assertEqual(after_pod["status"]["conditions"][-1]["status"], "True")
        after_node_name = after_pod["spec"]["nodeName"]
        before_spec, after_spec = dict(before_pod["spec"]), dict(after_pod["spec"])
        after_spec.pop("nodeName")
        self.assertEqual(before_spec, after_spec)
        for name in ("before-nodes.json", "after-nodes.json"):
            self.assertEqual((raw[name]["apiVersion"], raw[name]["kind"]), ("v1", "NodeList"))
        before_nodes = raw["before-nodes.json"]["items"]
        after_nodes = raw["after-nodes.json"]["items"]
        self.assertEqual(len(before_nodes), 1)
        self.assertEqual(len(after_nodes), 1)
        self.assertEqual(before_nodes[0]["metadata"]["uid"], after_nodes[0]["metadata"]["uid"])
        self.assertEqual(before_nodes[0]["metadata"]["name"], after_node_name)
        self.assertNotIn("scout-pre02-zone", before_nodes[0]["metadata"]["labels"])
        self.assertEqual(after_nodes[0]["metadata"]["labels"]["scout-pre02-zone"], "fixture-zone")
        before_events = raw["before-events.json"]
        after_events = raw["after-events.json"]
        self.assertEqual((before_events["apiVersion"], before_events["kind"]), ("v1", "EventList"))
        self.assertEqual((after_events["apiVersion"], after_events["kind"]), ("v1", "EventList"))
        uid = before_pod["metadata"]["uid"]
        self.assertTrue(any(e.get("reason") == "FailedScheduling" and e.get("involvedObject", {}).get("uid") == uid
                            for e in before_events["items"]))
        self.assertTrue(any(e.get("reason") == "FailedScheduling" and e.get("involvedObject", {}).get("uid") == uid
                            for e in after_events["items"]))

    def test_capture_metadata_has_only_sanitized_scope_and_exact_observation_map(self):
        scope_path = FIXTURES / "capture-scope.json"
        scope_bytes = scope_path.read_bytes()
        scope = json.loads(scope_bytes)
        encoded = scope_bytes.lower()
        for private_marker in (b"privatekubeconfig", b"admininitial", b"observerinitial", b"token", b"secretdata"):
            self.assertNotIn(private_marker, encoded)
        observations = scope["capture"]["observations"]
        self.assertEqual(len(observations), 6)
        for observation in observations:
            self.assertEqual(observation["method"], "GET")
            self.assertEqual(observation["httpStatus"], 200)
            raw = (FIXTURES / observation["rawFile"]).read_bytes()
            self.assertEqual(hashlib.sha256(raw).hexdigest(), observation["rawSha256"])
            self.assertEqual(len(raw), observation["rawBytes"])

    def test_strict_grader_accepts_only_exact_canonical_answer(self):
        pattern = strict_pattern()
        good = json.dumps(EXPECTED, separators=(",", ":"))
        self.assertRegex(good, pattern)
        invalid = []
        wrong = dict(EXPECTED)
        wrong["before_pod_uid"] = "other"
        invalid.append(json.dumps(wrong, separators=(",", ":")))
        wrong = dict(EXPECTED)
        wrong["prerequisite"] = "UNKNOWN"
        invalid.append(json.dumps(wrong, separators=(",", ":")))
        for key in ("before_event", "after_selector_label", "health_scope", "cloud_api", "capacity"):
            wrong = dict(EXPECTED)
            wrong[key] = "UNKNOWN"
            invalid.append(json.dumps(wrong, separators=(",", ":")))
        invalid.extend([
            good[:-1],
            good[:-1] + ',"extra":"value"}',
            good[:-1] + ',"prerequisite":"NODE_SELECTOR_LABEL"}',
            good.replace('"before_matching_nodes":"0"', '"before_matching_nodes":0'),
            good.replace('"before_pod_uid":"b5fa45bb-3aa4-4ec2-aa97-c329ea564317",', "", 1),
            json.dumps(dict(reversed(list(EXPECTED.items()))), separators=(",", ":")),
            good.replace(",", ", ", 1),
            "answer: " + good,
            good + "\nextra",
            '{"value":NaN}',
        ])
        for index, answer in enumerate(invalid):
            with self.subTest(index=index, answer=answer):
                self.assertIsNone(pattern.fullmatch(answer))

    def test_prompt_declares_canonical_contract_without_answer_leak(self):
        prompt = (ROOT / "prompt.md").read_text()
        self.assertTrue(prompt.startswith("---\n"))
        body = prompt.split("\n---\n", 1)[1]
        for answer_token in (EXPECTED["before_pod_uid"], EXPECTED["after_node"], EXPECTED["selector"]):
            self.assertNotIn(answer_token, body)
        self.assertIn("no additional keys,", body)
        self.assertIn("`NODE_SELECTOR_LABEL`, `CLOUD_API`, `SECRET`, or `UNKNOWN`", body)
        self.assertIn("`SCHEDULING_ONLY_NO_HEALTH_CONCLUSION`", body)
        self.assertIn("`NOT_INFERRED`", body)

    def test_scaffold_produces_exact_equal_arms_and_keeps_archive_bytes(self):
        expected_names = set(RAW_HASHES) | {"capture-scope.json"}
        outputs = []
        for _ in range(2):
            with tempfile.TemporaryDirectory() as directory:
                env = dict(__import__("os").environ)
                env["KUBECONFIG"] = "/tmp/scout-offline-validation.kubeconfig"
                subprocess.run(["bash", str(ROOT / "scaffold.sh")], cwd=directory,
                               env=env, check=True, capture_output=True)
                output = Path(directory) / "cluster"
                self.assertEqual({p.name for p in output.iterdir()}, expected_names)
                copied = {p.name: p.read_bytes() for p in output.iterdir()}
                for name, data in copied.items():
                    self.assertEqual(data, (FIXTURES / name).read_bytes())
                outputs.append(copied)
        self.assertEqual(outputs[0], outputs[1])


if __name__ == "__main__":
    unittest.main()
