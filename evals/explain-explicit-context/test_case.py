"""Offline authored-vector controls only; no model, server or evaluator runs."""
import copy
import json
from pathlib import Path
import re
import unittest


HERE = Path(__file__).resolve().parent


def expected_answer(inputs):
    request = inputs["request"]
    selected = inputs["contexts"][request["context"]]
    workload = selected["workload"]
    assert workload["metadata"]["namespace"] == request["namespace"]
    assert workload["metadata"]["labels"]["argocd.argoproj.io/instance"] == selected["application"]["metadata"]["name"]
    assert inputs["denied"]["response"]["reason"] == "Forbidden"
    assert inputs["denied"]["response"]["code"] == 403
    limits = inputs["contract_limits"]
    return {
        "selected_context": request["context"],
        "namespace": workload["metadata"]["namespace"],
        "owner": "ArgoCD",
        "source": selected["application"]["spec"]["source"]["repoURL"],
        "ambient_beta_used": request["context"] == inputs["ambient_context"],
        "denied_owner": "unknown",
        "denied_health": "unknown",
        "omitted_context": limits["omitted_context"],
        "stable_cluster_identity": limits["context_is_stable_cluster_identity"],
        "live_acceptance_proven": limits["genuine_live_acceptance"],
    }


class ExplainExplicitContextCase(unittest.TestCase):
    def setUp(self):
        self.inputs = json.loads((HERE / "fixtures/inputs.json").read_text())
        self.answer = expected_answer(self.inputs)
        metadata = dict(line.split(": ", 1) for line in (HERE / "graders/verified-answer.md").read_text().splitlines())
        self.assertEqual(metadata["type"], "regex")
        self.assertEqual(metadata["target"], "last_message")
        self.assertEqual(metadata["flags"], "s")
        self.grader = re.compile(json.loads(metadata["pattern"]), re.DOTALL)

    def accepts(self, value):
        return self.grader.search(value) is not None

    def test_fixture_and_source_metadata(self):
        self.assertEqual(self.inputs["provenance"]["kind"], "authored")
        self.assertIs(self.inputs["provenance"]["live_capture"], False)
        self.assertEqual(self.inputs["contexts"]["alpha-context"]["workload"], self.inputs["contexts"]["beta-context"]["workload"])
        self.assertNotEqual(self.inputs["contexts"]["alpha-context"]["application"]["spec"]["source"], self.inputs["contexts"]["beta-context"]["application"]["spec"]["source"])
        case = (HERE / "case.yaml").read_text()
        self.assertIn('schema_version: "1.1"', case)
        self.assertIn("name: explain-explicit-context", case)
        self.assertIn("scaffold_script: scaffold.sh", case)
        prompt = (HERE / "prompt.md").read_text()
        self.assertIn("allowed_tools: [Read, Grep]", prompt)
        self.assertIn("max_turns: 5", prompt)
        self.assertIn("timeout_seconds: 120", prompt)
        for key in self.answer:
            self.assertIn(f"`{key}`", prompt)
        script = (HERE / "scaffold.sh").read_text()
        self.assertIn('"$case_dir/fixtures/inputs.json" cluster/inputs.json', script)
        self.assertEqual(script.count("cp --"), 1)
        self.assertNotIn("graders", script)
        self.assertNotIn("curl", script)
        self.assertTrue((HERE.parent.parent / self.inputs["provenance"]["product_contract"]).is_file())

    def test_canonical_and_wrong_evidence_per_field(self):
        self.assertTrue(self.accepts(json.dumps(self.answer)))
        self.assertTrue(self.accepts(json.dumps(self.answer, indent=2)))
        for key, value in self.answer.items():
            wrong = copy.deepcopy(self.answer)
            wrong[key] = not value if type(value) is bool else "WRONG"
            self.assertFalse(self.accepts(json.dumps(wrong)), key)
            wrong[key] = str(value) if type(value) is bool else False
            self.assertFalse(self.accepts(json.dumps(wrong)), key)

    def test_missing_duplicate_extra_partial_and_prose(self):
        canonical = json.dumps(self.answer)
        for key in self.answer:
            missing = dict(self.answer)
            del missing[key]
            self.assertFalse(self.accepts(json.dumps(missing)), key)
            duplicate = canonical[:-1] + ", " + json.dumps(key) + ": " + json.dumps(self.answer[key]) + "}"
            self.assertFalse(self.accepts(duplicate), key)
        extra = dict(self.answer, extra="claim")
        for value in (json.dumps(extra), canonical[:-1], "Answer: " + canonical, canonical + "\nAccepted live", "```json\n" + canonical + "\n```", "{}", "null", json.dumps(list(self.answer.values()))):
            self.assertFalse(self.accepts(value))


if __name__ == "__main__":
    unittest.main()
