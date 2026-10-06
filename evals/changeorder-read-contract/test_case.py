"""Offline authored-vector controls; no evaluator, model or live commands."""
import copy
import json
from pathlib import Path
import re
import unittest

HERE = Path(__file__).resolve().parent
PIN = "4c8d2fc3885fed0d7af6835f2aac0a24387b6221"


def expected_answer(inputs):
    request = inputs["request"]
    row = inputs["responses_by_space"][request["space"]]["ChangeOrder"]
    assert row["SpaceSlug"] == request["space"] and row["Slug"] == request["order"]
    workflow = row["ChangeWorkflow"]
    limits = inputs["contract_limits"]
    assert not limits["evaluated_outcomes_exposed_by_get"]
    assert not limits["declarations_are_evaluations"]
    assert not limits["completed_is_runtime_health"]
    assert not limits["missing_workflow_proves_ungoverned"]
    assert "ChangeWorkflow" not in inputs["missing_workflow"]["ChangeOrder"]
    assert inputs["denied"]["Error"]["Code"] == 403
    return {
        "selected_space": row["SpaceSlug"],
        "order": row["Slug"],
        "stage": row["Stage"],
        "state": row["State"],
        "stages": [stage["Name"] for stage in workflow["Stages"]],
        "review_prerequisites": workflow["Stages"][0]["Prerequisites"],
        "evaluation": "unknown",
        "approval": "unknown",
        "runtime_health": "unknown",
        "absent_workflow_governance": "unknown",
        "denied_evaluation": "unknown",
        "runtime_server_version": inputs["provenance"]["runtime_server_version"],
        "provenance": inputs["provenance"]["kind"],
        "live_acceptance_proven": limits["genuine_live_acceptance"],
    }


class ChangeOrderReadCase(unittest.TestCase):
    def setUp(self):
        self.inputs = json.loads((HERE / "fixtures/inputs.json").read_text())
        self.answer = expected_answer(self.inputs)
        metadata = dict(line.split(": ", 1) for line in (HERE / "graders/verified-answer.md").read_text().splitlines())
        self.assertEqual(metadata["type"], "regex")
        self.assertEqual(metadata["target"], "last_message")
        self.assertEqual(metadata["flags"], "s")
        self.grader = re.compile(json.loads(metadata["pattern"]), re.DOTALL)

    def accepts(self, text):
        return self.grader.search(text) is not None

    def test_source_fixture_and_metadata(self):
        provenance = self.inputs["provenance"]
        self.assertEqual(provenance["kind"], "authored")
        self.assertIs(provenance["live_capture"], False)
        self.assertEqual(provenance["parser_contract"]["commit"], PIN)
        self.assertEqual(provenance["parser_contract"]["tag"], "v0.6.8")
        prod = self.inputs["responses_by_space"]["prod"]["ChangeOrder"]
        other = self.inputs["responses_by_space"]["staging"]["ChangeOrder"]
        self.assertEqual(prod["Slug"], other["Slug"])
        self.assertNotEqual(prod["SpaceID"], other["SpaceID"])
        self.assertNotEqual(prod["ChangeWorkflow"], other["ChangeWorkflow"])
        self.assertEqual(prod["Stage"], "Completed")
        attestation = prod["ChangeWorkflow"]["AttestationPrerequisites"][0]
        self.assertEqual(attestation["Count"], 0)
        self.assertIs(attestation["AllowAuthors"], False)
        case = (HERE / "case.yaml").read_text()
        for value in ('schema_version: "1.1"', 'name: changeorder-read-contract', 'scaffold_script: scaffold.sh', 'FIXTURE-OWNED-SCAFFOLD'):
            self.assertIn(value, case)
        prompt = (HERE / "prompt.md").read_text()
        for value in ('allowed_tools: [Read, Grep]', 'max_turns: 5', 'timeout_seconds: 120'):
            self.assertIn(value, prompt)
        for key in self.answer:
            self.assertIn('`' + key + '`', prompt)
        script = (HERE / "scaffold.sh").read_text()
        self.assertEqual(script.count('cp --'), 1)
        self.assertIn('"$case_dir/fixtures/inputs.json" cluster/inputs.json', script)
        for command in ('curl ', 'cub ', 'kubectl ', 'graders'):
            self.assertNotIn(command, script)
        self.assertNotIn('changeorder-read-contract', (HERE.parent / 'benchmark-v1.json').read_text())

    def test_canonical_wrong_evidence_types_and_order(self):
        self.assertTrue(self.accepts(json.dumps(self.answer)))
        self.assertTrue(self.accepts(json.dumps(self.answer, indent=2)))
        for key, value in self.answer.items():
            wrong = copy.deepcopy(self.answer)
            wrong[key] = not value if type(value) is bool else ["WRONG"] if isinstance(value, list) else "WRONG"
            self.assertFalse(self.accepts(json.dumps(wrong)), key)
            for wrong_type in (None, 1, {}, str(value) if not isinstance(value, str) else False):
                wrong[key] = wrong_type
                self.assertFalse(self.accepts(json.dumps(wrong)), (key, wrong_type))
        for key in ('stages', 'review_prerequisites'):
            wrong = copy.deepcopy(self.answer)
            wrong[key].reverse()
            self.assertFalse(self.accepts(json.dumps(wrong)), key)
        wrong_space = dict(self.answer, selected_space='staging')
        self.assertFalse(self.accepts(json.dumps(wrong_space)))
        wrong_completed = dict(self.answer, evaluation='passed', approval='approved', runtime_health='healthy')
        self.assertFalse(self.accepts(json.dumps(wrong_completed)))

    def test_missing_duplicate_extra_partial_and_prose(self):
        canonical = json.dumps(self.answer)
        for key in self.answer:
            missing = dict(self.answer)
            del missing[key]
            self.assertFalse(self.accepts(json.dumps(missing)), key)
            duplicate = canonical[:-1] + ', ' + json.dumps(key) + ': ' + json.dumps(self.answer[key]) + '}'
            self.assertFalse(self.accepts(duplicate), key)
        for text in (json.dumps(dict(self.answer, extra='claim')), canonical[:-1], 'Answer: ' + canonical, canonical + '\nAccepted live', '```json\n' + canonical + '\n```', '{}', 'null', json.dumps(list(self.answer.values()))):
            self.assertFalse(self.accepts(text))


if __name__ == "__main__":
    unittest.main()
