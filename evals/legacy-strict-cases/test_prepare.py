"""Pure offline tests for generated strict answers; no eval runner is called."""
import importlib.util
import json
from pathlib import Path
import re
import shutil
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("legacy_strict_prepare", HERE / "prepare.py")
prepare = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(prepare)
REPORT_SPEC = importlib.util.spec_from_file_location("eval_report", HERE.parent / "scripts/report.py")
report = importlib.util.module_from_spec(REPORT_SPEC)
assert REPORT_SPEC and REPORT_SPEC.loader
REPORT_SPEC.loader.exec_module(report)


class StrictLegacyPacketTests(unittest.TestCase):
    def test_contract_requires_explicit_opt_in_and_historical_inputs_stay_pinned(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(prepare.PacketError, "explicit --contract"):
                prepare.prepare(Path(temp) / "implicit")
            facts = prepare.verify_source()
            self.assertEqual(set(facts["cases"]), set(prepare.CASES))
            self.assertEqual(facts["manifestSha256"], prepare.MANIFEST_SHA256)
            self.assertEqual(facts["negativeControls"], ["evals/owner-unlabelled", "ATR-03 changed-by-payments"])

    def test_canonical_answers_match_and_contradictions_or_extra_content_fail(self):
        for case_id, spec in prepare.CASES.items():
            with self.subTest(case=case_id):
                grader = prepare.grader_bytes(spec["answer"]).decode()
                match = re.search(r"(?m)^pattern: '(.*)'$", grader)
                self.assertIsNotNone(match)
                pattern = re.compile(match.group(1), re.DOTALL)
                self.assertRegex(spec["answer"], pattern)
                self.assertRegex(spec["answer"] + "\n", pattern)
                for bad in (
                    "Answer: " + spec["answer"],
                    spec["answer"] + "\nActually, that conclusion is wrong.",
                    spec["answer"] + " with extra unsupported detail",
                ):
                    with self.subTest(bad=bad[:90]):
                        self.assertNotRegex(bad, pattern)

    def test_answer_contracts_bind_field_manager_person_and_scope(self):
        mutations = {
            "INV-03": ["UnitSlug=inventory", "recorded evidence only"],
            "ATR-01": ["MANAGER: kubectl-set", "FIELD: spec.template.spec.containers[name=checkout].image",
                       "HUMAN: UNKNOWN", "COMMAND: UNKNOWN", "SCOPE: recorded evidence only"],
            "ATR-02": ["MANAGER: kubectl", "FIELD: spec.replicas", "SUBRESOURCE: scale",
                       "HUMAN: UNKNOWN", "COMMAND: UNKNOWN", "SCOPE: recorded evidence only"],
            "ATR-03": ["MANAGER: helm", "HUMAN: UNKNOWN", "COMMAND: UNKNOWN", "SCOPE: recorded evidence only"],
            "ATR-04": ["APPLICATION: payments", "tracking-id payments:", "annotation tracking",
                       "instance label storefront", "SCOPE: recorded evidence only"],
        }
        for case_id, required in mutations.items():
            spec = prepare.CASES[case_id]
            pattern = re.compile(prepare.canonical_pattern(spec["answer"]), re.DOTALL)
            for item in required:
                with self.subTest(case=case_id, item=item):
                    self.assertIn(item, spec["answer"])
                    replacement = item.replace("UNKNOWN", "operator-17") if "UNKNOWN" in item else "incorrect-evidence"
                    self.assertNotRegex(spec["answer"].replace(item, replacement), pattern)
        self.assertNotIn("payments", prepare.CASES["ATR-04"]["prompt_suffix"])
        self.assertNotIn("ConfigHub", prepare.CASES["INV-03"]["prompt_suffix"])

    def test_actual_report_schema_counts_one_default_weight_regex_not_tool_indicators(self):
        # The harness report schema uses the embedded definition's default
        # weight=1 and explicitly skips tool_used definitions.
        definitions = [
            {"name": "verified-answer", "type": "regex"},
            {"name": "skill-fired", "type": "tool_used", "weight": 100},
        ]
        self.assertTrue(report.verified_answer(
            {"graders": [{"name": "verified-answer", "passed": True}]}, definitions))
        self.assertFalse(report.verified_answer(
            {"graders": [{"name": "verified-answer", "passed": False}]}, definitions))
        self.assertIsNone(report.verified_answer(
            {"graders": [{"name": "verified-answer", "passed": True},
                          {"name": "verified-answer", "passed": True}]}, definitions))
        self.assertIsNone(report.verified_answer(
            {"graders": [{"name": "skill-fired", "passed": True}]}, definitions))

    def test_prepared_arms_are_exact_byte_equal_and_metadata_is_pinned(self):
        with tempfile.TemporaryDirectory() as temp:
            out = Path(temp) / "packet"
            facts = prepare.prepare(out, prepare.CONTRACT)
            verified = prepare.verify_prepared(out, prepare.CONTRACT)
            self.assertEqual(facts, verified)
            self.assertEqual(facts["caseIds"], list(prepare.CASES))
            self.assertTrue(facts["armInputsByteEqual"])
            for case_id, spec in prepare.CASES.items():
                with_root = out / "arms/with/evals/legacy-strict" / spec["directory"]
                without_root = out / "arms/without/evals/legacy-strict" / spec["directory"]
                self.assertEqual(prepare._file_hashes(with_root), prepare._file_hashes(without_root))
                self.assertEqual((with_root / "case.yaml").read_bytes(),
                                 (prepare.REPO / "evals" / spec["directory"] / "case.yaml").read_bytes())
                self.assertEqual((with_root / "scaffold.sh").read_bytes(),
                                 (prepare.REPO / "evals" / spec["directory"] / "scaffold.sh").read_bytes())
                staged_prompt = (with_root / "prompt.md").read_bytes()
                source_prompt = (prepare.REPO / "evals" / spec["directory"] / "prompt.md").read_bytes()
                self.assertEqual(staged_prompt, prepare.strict_prompt(source_prompt, spec["prompt_suffix"]))
                self.assertNotIn(spec["answer"].encode(), staged_prompt)
                self.assertFalse((with_root / "graders" / spec["answer_grader"]).exists())
                self.assertTrue((with_root / "graders/verified-answer.md").is_file())
                graders = list((with_root / "graders").glob("*.md"))
                correctness = [p for p in graders if "type: regex" in p.read_text()]
                self.assertEqual([p.name for p in correctness], ["verified-answer.md"])
                self.assertIn("flags: s\ntarget: last_message", correctness[0].read_text())
                self.assertNotIn("weight:", correctness[0].read_text())

    def test_staged_missing_extra_modified_and_symlinked_files_reject(self):
        with tempfile.TemporaryDirectory() as temp:
            out = Path(temp) / "packet"
            prepare.prepare(out, prepare.CONTRACT)
            case = out / "arms/with/evals/legacy-strict/changed-by-checkout"
            prompt = case / "prompt.md"
            prompt.write_bytes(prompt.read_bytes() + b"\ncontradiction")
            with self.assertRaisesRegex(prepare.PacketError, "staged with files differ"):
                prepare.verify_prepared(out, prepare.CONTRACT)

            # A fresh packet also rejects unexpected files and symlink substitution.
            shutil.rmtree(out)
            prepare.prepare(out, prepare.CONTRACT)
            (out / "unexpected.txt").write_text("extra")
            with self.assertRaisesRegex(prepare.PacketError, "extra"):
                prepare.verify_prepared(out, prepare.CONTRACT)
            (out / "unexpected.txt").unlink()
            case = out / "arms/without/evals/legacy-strict/owner-confighub"
            scaffold = case / "scaffold.sh"
            scaffold.unlink()
            scaffold.symlink_to(prepare.REPO / "evals/owner-confighub/scaffold.sh")
            with self.assertRaisesRegex(prepare.PacketError, "symlink"):
                prepare.verify_prepared(out, prepare.CONTRACT)

    def test_source_tampering_and_wrong_contract_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            fake_repo = Path(temp) / "repo"
            (fake_repo / "evals").mkdir(parents=True)
            shutil.copy2(prepare.REPO / prepare.MANIFEST_PATH, fake_repo / prepare.MANIFEST_PATH)
            for spec in prepare.CASES.values():
                shutil.copytree(prepare.REPO / "evals" / spec["directory"],
                                fake_repo / "evals" / spec["directory"])
            shutil.copytree(prepare.REPO / "evals/owner-unlabelled",
                            fake_repo / "evals/owner-unlabelled")
            original_repo = prepare.REPO
            try:
                prepare.REPO = fake_repo
                self.assertEqual(set(prepare.verify_source()["cases"]), set(prepare.CASES))
                changed = fake_repo / "evals/changed-by-cart/scaffold.sh"
                changed.write_bytes(changed.read_bytes() + b"# changed\n")
                with self.assertRaisesRegex(prepare.PacketError, "source file set or hash changed"):
                    prepare.verify_source()
                with self.assertRaisesRegex(prepare.PacketError, "requires explicit"):
                    prepare.prepare(Path(temp) / "unused", "legacy")
            finally:
                prepare.REPO = original_repo


if __name__ == "__main__":
    unittest.main()
