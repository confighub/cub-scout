"""Offline source/answer controls; never executes an agent or cluster command."""
import hashlib
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).parent
EXPECTED = {'allowed_doctor_context': 'doctor-allowed', 'allowed_scan_context': 'doctor-allowed', 'allowed_observed_resources': '3', 'denied_doctor_context': 'doctor-denied', 'denied_scan_context': 'doctor-denied', 'denied_inventory': 'UNKNOWN', 'denied_scan_coverage': 'INCOMPLETE', 'denied_healthy': 'NOT_PROVEN', 'stable_cluster_identity': 'NOT_PROVEN', 'dollar_savings': 'NOT_PROVEN'}

class RecordedCaseTests(unittest.TestCase):
    def test_recorded_bytes_and_product_contract(self):
        provenance = json.loads((ROOT / "fixtures/provenance.json").read_text())
        for name, metadata in provenance["files"].items():
            data = (ROOT / "fixtures" / name).read_bytes()
            self.assertEqual(metadata["sha256"], hashlib.sha256(data).hexdigest())
            self.assertEqual(0, metadata["exitCode"])
            parsed = json.loads(data)
            context = "doctor-denied" if "denied" in name else "doctor-allowed"
            self.assertEqual(context, parsed["kubernetesContext"])
            if name == "doctor-allowed.json":
                self.assertEqual(3, parsed["resources"]["total"])
            if name == "doctor-denied.json":
                self.assertEqual(0, parsed["resources"]["total"])
                self.assertTrue(any("forbidden" in w.lower() for w in parsed["warnings"]))
            if name == "scan-denied.json":
                self.assertEqual(6, len(parsed["state"]["warnings"]))
                self.assertTrue(all("Access denied" in w for w in parsed["state"]["warnings"]))

    def test_grader_accepts_correct_and_rejects_each_wrong_claim(self):
        line = next(line for line in (ROOT / "graders/verified-answer.md").read_text().splitlines() if line.startswith("pattern: "))
        pattern = line[len("pattern: '"):-1]
        self.assertIsNotNone(re.fullmatch(pattern, json.dumps(EXPECTED), re.S))
        for key in EXPECTED:
            bad = dict(EXPECTED)
            bad[key] = "WRONG"
            self.assertIsNone(re.fullmatch(pattern, json.dumps(bad), re.S), key)
        bad = dict(EXPECTED, extra="unjustified")
        self.assertIsNone(re.fullmatch(pattern, json.dumps(bad), re.S))

if __name__ == "__main__":
    unittest.main()
