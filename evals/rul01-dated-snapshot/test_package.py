"""Deterministic fixture, binding and answer-contract guards for RUL-01."""
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).parent
spec = importlib.util.spec_from_file_location("rul01_contract", ROOT / "contract.py")
contract = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(contract)


class DatedSnapshotTests(unittest.TestCase):
    def files(self):
        return (contract.RAW.read_bytes(), contract.RECEIPT.read_bytes(), contract.CLOCKS.read_bytes())

    def test_real_pinned_binding_and_authored_ages(self):
        facts = contract.validate()
        self.assertEqual(facts["sha256"], contract.EXPECTED_SHA)
        self.assertEqual(facts["agesSeconds"], [300, 600])
        self.assertEqual(facts["scheduled"], "POD_SCHEDULED_TRUE")
        self.assertEqual(facts["currentState"], "UNKNOWN")

    def test_missing_or_mismatched_time_and_raw_bindings_fail_closed(self):
        raw, receipt, clocks = self.files()
        for bad_receipt in (b"{}", receipt.replace(b"06:03:25Z", b"06:03:26Z"),
                            receipt.replace(b"0.01204633410088718", b"0.11204633410088718"),
                            receipt.replace(contract.EXPECTED_SHA.encode(), b"0"*64)):
            with self.subTest(receipt=bad_receipt[:30]), self.assertRaises(contract.ContractError):
                contract.validate(raw, bad_receipt, clocks)
            self.assertEqual(contract.validate_or_unknown(raw, bad_receipt, clocks)["evidence_binding"], "UNKNOWN")
        with self.assertRaises(contract.ContractError):
            contract.validate(raw + b" ", receipt, clocks)
        self.assertEqual(contract.validate_or_unknown(raw + b" ", receipt, clocks)["sha256"], "UNKNOWN")

    def test_clock_change_wrong_identity_or_duplicate_condition_fails_closed(self):
        raw, receipt, clocks = self.files()
        bad_clocks = clocks.replace(b"06:13:25Z", b"06:13:24Z")
        with self.assertRaises(contract.ContractError): contract.validate(raw, receipt, bad_clocks)
        self.assertEqual(contract.validate_or_unknown(raw, receipt, bad_clocks)["agesSeconds"], ["UNKNOWN", "UNKNOWN"])
        pod = json.loads(raw); pod["metadata"]["uid"] = "other"
        with self.assertRaises(contract.ContractError): contract.validate(json.dumps(pod).encode(), receipt, clocks)
        pod = json.loads(raw); pod["status"]["conditions"].append(dict(next(c for c in pod["status"]["conditions"] if c["type"] == "PodScheduled")))
        with self.assertRaises(contract.ContractError): contract.validate(json.dumps(pod).encode(), receipt, clocks)

    def test_fixture_scaffold_stages_same_hash_bound_bytes(self):
        script = (ROOT / "scaffold.sh").read_text()
        for name, content in zip(("after-pod.json", "capture-time-receipt.json", "test-clocks.json"), self.files()):
            self.assertIn(f'cp "$root/fixtures/{name}" cluster/{name}', script)
            self.assertEqual(hashlib.sha256(content).hexdigest(), hashlib.sha256((ROOT / "fixtures" / name).read_bytes()).hexdigest())
        self.assertIn("same raw Pod", script)

    def test_strict_answer_regex_accepts_only_contract_answer(self):
        grader = (ROOT / "graders" / "verified-answer.md").read_text()
        pattern = re.compile(grader.split("pattern: '", 1)[1].rsplit("'", 1)[0])
        answer = json.dumps({
            "evidence_binding":"VERIFIED", "resource_identity":"v1|Pod|pre02-node-selector|scout-pre02-selector",
            "resource_uid":"b5fa45bb-3aa4-4ec2-aa97-c329ea564317", "recorded_scheduling_state":"POD_SCHEDULED_TRUE",
            "resource_created_at_utc":"2026-10-01T06:03:25Z", "condition_transition_at_utc":"2026-10-01T06:03:25Z",
            "capture_request_started_at_utc":"2026-10-01T06:03:25Z", "capture_request_ended_at_utc":"2026-10-01T06:03:25Z",
            "timestamp_precision":"ONE_SECOND_LOGGED", "capture_time_basis":"REQUEST_END",
            "snapshot_sha256":contract.EXPECTED_SHA, "test_clock_1_utc":"2026-10-01T06:08:25Z",
            "age_at_clock_1_seconds":"300", "test_clock_2_utc":"2026-10-01T06:13:25Z", "age_at_clock_2_seconds":"600",
            "capture_time_reuse":"SAME_SNAPSHOT_NO_REFRESH", "current_live_state":"UNKNOWN",
            "scope":"HISTORICAL_CAPTURE_ONLY", "evidence":"after-pod.json+capture-time-receipt.json+test-clocks.json"
        }, separators=(",", ":"))
        self.assertIsNotNone(pattern.fullmatch(answer))
        for wrong in (answer.replace('"600"', '"0"'), answer.replace('"UNKNOWN"', '"LIVE_PRESENT"'), answer + " prose",
                      answer.replace("2026-10-01T06:03:25Z", "2026-10-01T06:13:25Z", 1),
                      answer.replace(contract.EXPECTED_SHA, "0"*64), answer[:-1] + ',"extra":"x"}'):
            with self.subTest(wrong=wrong[-50:]): self.assertIsNone(pattern.fullmatch(wrong))


if __name__ == "__main__":
    unittest.main()
