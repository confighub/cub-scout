"""Pure source-only package tests. No CLI, network, model, or grader execution."""
from __future__ import annotations

import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("full24_prepare", ROOT / "prepare.py")
prepare = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(prepare)


class Full24PreparationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix="full24-pair-preflight-test-")
        cls.out = Path(cls.temp.name) / "package"
        cls.report = prepare.prepare(cls.out)

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def _mutate(self, path: Path, new: bytes, action):
        old = path.read_bytes() if path.exists() else None
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(new)
            with self.assertRaises(prepare.PreparationError):
                action()
        finally:
            if old is None:
                path.unlink(missing_ok=True)
            else:
                path.write_bytes(old)

    def _mutate_many(self, replacements: dict[Path, bytes], action):
        old = {path: path.read_bytes() if path.exists() else None for path in replacements}
        try:
            for path, data in replacements.items():
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
            with self.assertRaises(prepare.PreparationError):
                action()
        finally:
            for path, data in old.items():
                if data is None:
                    path.unlink(missing_ok=True)
                else:
                    path.write_bytes(data)

    def test_full_pair_has_exact_frozen24_and_blinded_oracles(self):
        report = prepare.validate(self.out)
        self.assertEqual(report["caseCount"], 24)
        self.assertEqual([g["weight"] for g in report["groups"]], [0.1666666667] * 6)
        self.assertEqual(report["scaleDataset"]["parsed"], 302)
        self.assertEqual(report["scaleDataset"]["selected"], 300)
        self.assertEqual(report["scaleDataset"]["excluded"], 2)
        self.assertEqual(report["status"], "prepared_source_only_not_run_not_admitted")
        for case in report["cases"]:
            self.assertEqual(len(case["selectedAnswerGraders"]), 1)
            self.assertGreater(case["selectedAnswerGraders"][0]["weight"], 0)
            self.assertEqual(len(case["ordinaryToolGrant"]), len(set(case["ordinaryToolGrant"])))
            for arm in ("without", "with"):
                root = self.out / "arms" / arm / "cases" / case["id"]
                text = (root / "prompt.md").read_text()
                self.assertNotIn("expected_outcome:", text)
                self.assertFalse(any("grader" in p.name.lower() for p in root.rglob("*")))
                self.assertFalse(any(p.name in {"case.yaml", "reference.json"} for p in root.rglob("*")))
        self.assertIn("recorded-scale-answer-line.v1", {c["answerContract"] for c in report["cases"]})
        self.assertIn("legacy-answer-line.v1", {c["answerContract"] for c in report["cases"]})

    def test_case_set_rejects_missing_duplicate_and_extra(self):
        path = self.out / "preflight.json"
        original = path.read_bytes()
        value = json.loads(original)
        for change in (
            lambda x: x["cases"].pop(),
            lambda x: x["cases"].__setitem__(1, copy.deepcopy(x["cases"][0])),
            lambda x: x["cases"].append(copy.deepcopy(x["cases"][0]) | {"id": "EXTRA"}),
        ):
            changed = copy.deepcopy(value)
            change(changed)
            self._mutate(path, json.dumps(changed).encode(), lambda: prepare.validate(self.out))
        self.assertEqual(path.read_bytes(), original)

    def test_frozen_question_reference_control_and_tool_grant_are_source_bound(self):
        path = self.out / "preflight.json"
        original = path.read_bytes()
        value = json.loads(original)
        changed = copy.deepcopy(value)
        changed["cases"][0]["question"] += " altered"
        self._mutate(path, json.dumps(changed).encode(), lambda: prepare.validate(self.out))
        changed = copy.deepcopy(value)
        changed["cases"][0]["reference"] += " altered"
        self._mutate(path, json.dumps(changed).encode(), lambda: prepare.validate(self.out))
        grant = self.out / "arms/without/cases/INV-01/ordinary-tool-grant.json"
        self._mutate(grant, b'["Bash"]', lambda: prepare.validate(self.out))
        self.assertEqual(path.read_bytes(), original)

    def test_pair_evidence_prompt_grant_and_oracle_leaks_fail(self):
        prompt = self.out / "arms/with/cases/DEL-01/prompt.md"
        self._mutate(prompt, prompt.read_bytes() + b" drift", lambda: prepare.validate(self.out))
        evidence = self.out / "arms/without/cases/PRE-03/cluster/events.txt"
        self._mutate(evidence, evidence.read_bytes() + b" truncated", lambda: prepare.validate(self.out))
        leak = self.out / "arms/without/cases/HLT-01/graders/answer.md"
        self._mutate(leak, b"oracle", lambda: prepare.validate(self.out))

    def test_private_selected_graders_and_reference_are_checked(self):
        grader = self.out / "oracle/DEL-04/selected-graders/verified-answer.md"
        self._mutate(grader, grader.read_bytes() + b" drift", lambda: prepare.validate(self.out))
        reference = self.out / "oracle/DEL-04/reference.json"
        self._mutate(reference, b'{"reference":"wrong"}', lambda: prepare.validate(self.out))

    def test_yaml_duplicate_keys_and_malformed_evidence_fail_closed(self):
        with self.assertRaises(prepare.PreparationError):
            prepare._frontmatter(b"---\nallowed_tools: [Read]\nallowed_tools: [Bash]\n---\nquestion\n")
        path = self.out / "arms/with/cases/INV-02/cluster/deployments.yaml"
        self._mutate(path, b"apiVersion: apps/v1\nkind: Deployment\nmetadata: [broken\n", lambda: prepare.validate(self.out))

    def test_del03_and_del04_authored_control_inference_guards(self):
        del03 = self.out / "authored-input-controls/DEL-03/input/status-excerpt.md"
        self._mutate(del03, del03.read_bytes() + b"\neu-central-uat1 injected\n", lambda: prepare.validate(self.out))
        report_path = self.out / "preflight.json"
        report = json.loads(report_path.read_bytes())
        altered_status = del03.read_bytes() + b"\nidentity altered\n"
        for path in (self.out / "authored-input-controls/DEL-03/arms/without/status-excerpt.md",
                     self.out / "authored-input-controls/DEL-03/arms/with/status-excerpt.md"):
            self.assertEqual(path.read_bytes(), del03.read_bytes())
        changed = copy.deepcopy(report)
        changed["authoredInputControls"]["DEL-03"]["inputFiles"]["status-excerpt.md"] = prepare.digest(altered_status)
        self._mutate_many({
            del03: altered_status,
            self.out / "authored-input-controls/DEL-03/arms/without/status-excerpt.md": altered_status,
            self.out / "authored-input-controls/DEL-03/arms/with/status-excerpt.md": altered_status,
            report_path: json.dumps(changed).encode(),
        }, lambda: prepare.validate(self.out))
        del04 = self.out / "authored-input-controls/DEL-04/input/source-chain-authored-conflict.yaml"
        original = prepare._yaml_load((self.out / "authored-input-controls/DEL-04/input/source-chain-original.yaml").read_text())
        conflict = prepare._yaml_load(del04.read_text())
        conflict["spec"]["boundaries"]["delivery"]["digest"] = original["spec"]["boundaries"]["delivery"]["digest"]
        self._mutate(del04, prepare.yaml.safe_dump(conflict, sort_keys=False).encode(), lambda: prepare.validate(self.out))

    def test_treatment_mcp_is_inert_and_source_pins_are_exact(self):
        plugin = json.loads((self.out / "arms/with/.claude-plugin/plugin.json").read_bytes())
        self.assertEqual(plugin["mcpServers"]["cub-scout"]["command"], "__RECORDED_MCP_BINDING_PENDING__")
        self.assertTrue((self.out / "arms/with/skills/cub-scout/SKILL.md").is_file())
        self.assertFalse((self.out / "arms/without/skills").exists())
        self.assertEqual(self.report["treatmentMcpBinding"]["execution"], "not-invoked")
        self.assertEqual(self.report["officialScaffoldWorkflow"]["status"], "reproducibility-metadata-only-never-invoked")

    def test_report_cannot_authorize_extra_answer_skill_or_live_plugin_files(self):
        report_path = self.out / "preflight.json"
        report = json.loads(report_path.read_bytes())
        extra = b"answers.txt"
        changed = copy.deepcopy(report)
        changed["arms"]["without"]["files"]["answers.txt"] = prepare.digest(extra)
        changed["arms"]["with"]["commonFiles"]["answers.txt"] = prepare.digest(extra)
        self._mutate_many({self.out / "arms/without/answers.txt": extra,
                           self.out / "arms/with/answers.txt": extra,
                           report_path: json.dumps(changed).encode()}, lambda: prepare.validate(self.out))

        skill_rel = "skills/cub-scout/SKILL.md"
        skill = self.out / "arms/with" / skill_rel
        mutated_skill = skill.read_bytes() + b" changed"
        changed = copy.deepcopy(report)
        changed["arms"]["with"]["treatmentFiles"][skill_rel] = prepare.digest(mutated_skill)
        skill_source_map = {p.relative_to(prepare.REPO / "skills").as_posix(): prepare.digest(p.read_bytes())
                            for p in sorted((prepare.REPO / "skills").rglob("*")) if p.is_file()}
        skill_source_map["cub-scout/SKILL.md"] = prepare.digest(mutated_skill)
        changed["skillsSourceTreeSha256"] = prepare.digest(json.dumps(skill_source_map, sort_keys=True, separators=(",", ":")).encode())
        self._mutate_many({skill: mutated_skill, report_path: json.dumps(changed).encode()}, lambda: prepare.validate(self.out))

        plugin_rel = ".claude-plugin/plugin.json"
        plugin = self.out / "arms/with" / plugin_rel
        plugin_value = json.loads(plugin.read_bytes())
        plugin_value["mcpServers"]["cub-scout"]["command"] = "cub-scout"
        mutated_plugin = json.dumps(plugin_value, indent=2).encode() + b"\n"
        changed = copy.deepcopy(report)
        changed["arms"]["with"]["treatmentFiles"][plugin_rel] = prepare.digest(mutated_plugin)
        self._mutate_many({plugin: mutated_plugin, report_path: json.dumps(changed).encode()}, lambda: prepare.validate(self.out))


if __name__ == "__main__":
    unittest.main()
