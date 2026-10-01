import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).parent


class RecordedSlugContract(unittest.TestCase):
    def test_trace_and_candidate_rows_agree_on_safe_omission(self):
        trace = json.loads((ROOT / "fixtures/trace-evidence.json").read_text())
        rows = json.loads((ROOT / "fixtures/connected-rows.json").read_text())
        self.assertEqual(trace["fixtureKind"], "synthetic-shared-trace-model")
        self.assertEqual(rows["fixtureKind"], "mocked-bounded-connected-input")
        self.assertEqual(trace["trace"]["correlation"]["unitSlug"], "PaymentsAPI")
        self.assertEqual(trace["trace"]["correlation"]["target"], "West")
        self.assertEqual(rows["rows"]["unitEvents"][0]["unit"], "paymentsapi")
        self.assertEqual(rows["rows"]["releases"][0]["target"], "west")
        evidence = trace["trace"]["deliveryEvidence"]
        self.assertEqual(evidence["unitEvents"], [])
        self.assertEqual(evidence["releases"], [])
        self.assertEqual({entry["layer"] for entry in evidence["omissions"]},
                         {"confighub.releases", "confighub.unitEvents"})
        self.assertTrue(all("no " in entry["reason"] for entry in evidence["omissions"]))

    def test_grader_requires_exact_conservative_answer(self):
        grader = (ROOT / "graders/verified-answer.md").read_text()
        pattern = re.search(r"^pattern: '(.*)'$", grader, re.M).group(1)
        answer = ("{\"unit_slug\":\"PaymentsAPI\",\"target_slug\":\"West\","
                  "\"wrong_case_unit_event\":\"EXCLUDED\",\"wrong_case_release\":\"EXCLUDED\","
                  "\"unit_event_evidence\":\"NONE_WITH_NO_MATCH_OMISSION\","
                  "\"release_evidence\":\"NONE_WITH_NO_MATCH_OMISSION\","
                  "\"identity_rule\":\"SLUGS_CASE_SENSITIVE_IDS_STRONGER\","
                  "\"uuid_id_case\":\"NORMALIZED\"}")
        self.assertIsNotNone(re.fullmatch(pattern, answer))
        for wrong in (answer.replace("EXCLUDED", "MATCHED", 1),
                      answer.replace("NORMALIZED", "CASE_SENSITIVE"),
                      answer[:-1] + ',"extra":"claim"}'):
            self.assertIsNone(re.fullmatch(pattern, wrong))


if __name__ == "__main__":
    unittest.main()
