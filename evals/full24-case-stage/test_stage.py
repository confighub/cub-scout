"""Offline tests for the single-case full24 model-stage copier."""
from __future__ import annotations

import copy
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("full24_case_stage", HERE / "stage.py")
stage_module = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(stage_module)
prepare = stage_module.prepare


def chmod_writable(root: Path) -> None:
    if not root.exists():
        return
    for path in root.rglob("*"):
        if path.is_dir() and not path.is_symlink():
            os.chmod(path, 0o700)
        elif path.is_file() and not path.is_symlink():
            os.chmod(path, 0o600)
    os.chmod(root, 0o700)


class CaseStageTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix="full24-case-stage-")
        cls.root = Path(cls.temp.name).resolve(strict=True)
        cls.source = cls.root / "source-prep"
        cls.source.mkdir()
        cls.report = prepare.prepare(cls.source)
        cls.report_bytes = (cls.source / "preflight.json").read_bytes()
        cls.output = cls.root / "stages"
        cls.output.mkdir()

    @classmethod
    def tearDownClass(cls):
        chmod_writable(cls.root)
        cls.temp.cleanup()

    def _validated_stage(self, name, case_id, arm, control=None):
        # Reuse the one full source validation performed in setUpClass; public
        # stage() performs the same validation on every standalone invocation.
        out = self.output / name
        report = copy.deepcopy(self.report)
        report["_source_root"] = str(self.source)
        return stage_module._stage_validated(
            self.source, out, case_id, arm, control, report, self.report_bytes
        )

    def _rewrite(self, path: Path, data: bytes, action):
        original = path.read_bytes() if path.is_file() else None
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
            with self.assertRaises(stage_module.StageError):
                action()
        finally:
            if original is None:
                path.unlink(missing_ok=True)
            else:
                path.write_bytes(original)

    def test_all_frozen_cases_stage_with_byte_equal_common_files(self):
        treatment_delta = set(self.report["arms"]["with"]["treatmentFiles"])
        self.assertEqual(len(prepare.FROZEN_CASE_IDS), 24)
        for case_id in prepare.FROZEN_CASE_IDS:
            with self.subTest(case=case_id):
                without = self._validated_stage(case_id + "-without", case_id, "without")
                with_arm = self._validated_stage(case_id + "-with", case_id, "with")
                left = without["stagedFiles"]
                right = with_arm["stagedFiles"]
                self.assertEqual({k: v for k, v in left.items()},
                                 {k: v for k, v in right.items() if k not in treatment_delta})
                self.assertEqual(set(right) - set(left), treatment_delta)
                self.assertEqual(without["status"], "not_admitted_not_run")
                self.assertEqual(with_arm["claims"]["mcpUsable"], False)
                self.assertEqual(with_arm["treatmentMcp"]["status"], "pending_inert_marker_retained")
                without_report = copy.deepcopy(self.report)
                with_report = copy.deepcopy(self.report)
                self.assertEqual(stage_module._verify_validated_stage(
                    self.output / (case_id + "-without"), without, self.source,
                    without_report, self.report_bytes), without)
                self.assertEqual(stage_module._verify_validated_stage(
                    self.output / (case_id + "-with"), with_arm, self.source,
                    with_report, self.report_bytes), with_arm)

    def test_authored_controls_replace_case_evidence_and_never_copy_oracle(self):
        controls = self.report["authoredInputControls"]
        for case_id in ("DEL-03", "DEL-04"):
            control_id = controls[case_id]["id"]
            receipts = {}
            for arm in stage_module.ARMS:
                with self.subTest(case=case_id, arm=arm):
                    receipt = self._validated_stage(case_id + "-" + arm + "-control", case_id, arm, control_id)
                    receipts[arm] = receipt
                    model = self.output / (case_id + "-" + arm + "-control") / "model-stage"
                    self.assertEqual(receipt["selection"]["authoredInputControl"], control_id)
                    self.assertIn("ordinary-tool-grant.json", receipt["stagedFiles"])
                    self.assertIn("prompt.md", receipt["stagedFiles"])
                    paths = set(receipt["stagedFiles"])
                    case_paths = paths - set(self.report["arms"]["with"]["treatmentFiles"])
                    self.assertFalse(any("oracle" in p.lower() or "grader" in p.lower() or "reference" in p.lower() for p in case_paths))
                    original_case_files = {
                        Path(name).relative_to(Path("cases") / case_id).as_posix()
                        for name in self.report["arms"]["without"]["files"]
                        if Path(name).is_relative_to(Path("cases") / case_id)
                    }
                    self.assertFalse((paths - {"ordinary-tool-grant.json", "prompt.md"}) & original_case_files)
                    control_files = set(controls[case_id]["inputFiles"]) | {"prompt.md"}
                    self.assertTrue(control_files.issubset(paths))
                    self.assertTrue(model.is_dir())
            self.assertEqual(
                {k: v for k, v in receipts["without"]["stagedFiles"].items()},
                {k: v for k, v in receipts["with"]["stagedFiles"].items()
                 if k not in self.report["arms"]["with"]["treatmentFiles"]},
            )

    def test_invalid_selection_and_control_are_rejected(self):
        for args in (("NOPE", "without", None), ("INV-01", "treatment", None),
                     ("DEL-03", "with", "DEL-04-control"), ("INV-01", "without", "control")):
            with self.subTest(args=args), self.assertRaises(stage_module.StageError):
                stage_module.stage(self.source, self.output / ("invalid-" + str(len(list(self.output.iterdir())))), *args)

    def test_public_stager_revalidates_missing_extra_and_tampered_source(self):
        case_path = self.source / "arms/without/cases/INV-03/ordinary-tool-grant.json"
        self._rewrite(case_path, b'["Read"]', lambda: stage_module.stage(
            self.source, self.output / "bad-tampered", "INV-03", "without"))
        extra = self.source / "arms/without/cases/INV-03/unexpected.txt"
        self._rewrite(extra, b"unexpected", lambda: stage_module.stage(
            self.source, self.output / "bad-extra", "INV-03", "without"))
        missing = self.source / "arms/without/cases/INV-03/prompt.md"
        original = missing.read_bytes()
        try:
            missing.unlink()
            with self.assertRaises(stage_module.StageError):
                stage_module.stage(self.source, self.output / "bad-missing", "INV-03", "without")
        finally:
            missing.write_bytes(original)

    def test_fresh_destination_overlap_and_symlink_guards(self):
        existing = self.output / "preexisting-destination"
        existing.mkdir()
        symlink_out = self.output / "out-link"
        symlink_out.symlink_to(existing, target_is_directory=True)
        with self.assertRaises(stage_module.StageError):
            stage_module.stage(self.source, symlink_out, "INV-03", "without")
        with self.assertRaises(stage_module.StageError):
            stage_module.stage(self.source, existing, "INV-03", "without")
        with self.assertRaises(stage_module.StageError):
            stage_module.stage(self.source, self.source / "nested-stage", "INV-03", "without")
        parent_link = self.root / "parent-link"
        parent_link.symlink_to(self.output, target_is_directory=True)
        with self.assertRaises(stage_module.StageError):
            stage_module.stage(self.source, parent_link / "new", "INV-03", "without")
        source_link = self.root / "source-link"
        source_link.symlink_to(self.source, target_is_directory=True)
        with self.assertRaises(stage_module.StageError):
            stage_module.stage(source_link, self.output / "linked-source", "INV-03", "without")
        file_link = self.source / "arms/without/cases/INV-03/source-link"
        file_link.symlink_to(self.source / "preflight.json")
        try:
            with self.assertRaises(stage_module.StageError):
                stage_module.stage(self.source, self.output / "linked-file-source", "INV-03", "without")
        finally:
            file_link.unlink()

    def test_verifier_reconstructs_receipt_and_enforces_requested_selection(self):
        self._validated_stage("receipt-target", "DEL-03", "with")
        root = self.output / "receipt-target"
        receipt_path = root / "receipt.json"
        original = json.loads(receipt_path.read_bytes())
        changes = (
            lambda value: value.update(modelStageStatus="mounted"),
            lambda value: value["prompt"].update(sha256="0" * 64),
            lambda value: value["ordinaryToolGrant"].update(sha256="0" * 64),
            lambda value: value["sourcePins"].update({"prompt.md": "0" * 64}),
            lambda value: value["claims"].update(mcpUsable=True),
            lambda value: value["selection"].update(arm="bogus"),
            lambda value: value["selection"].update(authoredInputControl="wrong-control"),
            lambda value: value["sourcePreparation"].update(path=str(self.output)),
            lambda value: value.update(status="admitted"),
        )
        for change in changes:
            altered = copy.deepcopy(original)
            change(altered)
            receipt_path.write_text(json.dumps(altered))
            with self.assertRaises(stage_module.StageError):
                stage_module.verify(root, self.source, "DEL-03", "with", None)
        receipt_path.write_bytes((json.dumps(original, sort_keys=True, indent=2) + "\n").encode())
        os.chmod(receipt_path, 0o600)
        stage_module.verify(root, self.source, "DEL-03", "with", None)
        for requested in ((self.source, "DEL-04", "with", None),
                          (self.source, "DEL-03", "without", None),
                          (self.source, "DEL-03", "with", "wrong-control")):
            with self.subTest(requested=requested), self.assertRaises(stage_module.StageError):
                stage_module.verify(root, *requested)

    def test_malformed_receipt_shapes_and_special_files_fail_closed(self):
        receipt = self._validated_stage("malformed-receipt", "INV-03", "without")
        root = self.output / "malformed-receipt"
        receipt_path = root / "receipt.json"
        malformed = [[], None, "receipt", 7]
        for section in ("sourcePreparation", "selection", "claims", "caseBinding",
                        "answerContract", "prompt", "ordinaryToolGrant", "stagedFiles",
                        "sourcePins", "treatmentMcp"):
            for value in (None, [], "wrong"):
                altered = copy.deepcopy(receipt)
                altered[section] = value
                malformed.append(altered)
        for path_value in (None, [], {}, ""):
            altered = copy.deepcopy(receipt)
            altered["sourcePreparation"]["path"] = path_value
            malformed.append(altered)
        for value in malformed:
            with self.subTest(value=value):
                receipt_path.write_text(json.dumps(value))
                with self.assertRaises(stage_module.StageError):
                    stage_module.verify(root)
        receipt_path.write_text(json.dumps(receipt))
        model = root / "model-stage"
        os.chmod(model, 0o755)
        special = model / "untracked-fifo"
        os.mkfifo(special)
        try:
            with self.assertRaises(stage_module.StageError):
                stage_module.verify(root)
        finally:
            special.unlink()
            os.chmod(model, 0o555)
        source_special = self.source / "untracked-fifo"
        os.mkfifo(source_special)
        try:
            with self.assertRaises(stage_module.StageError):
                stage_module.stage(self.source, self.output / "special-source", "INV-03", "without")
        finally:
            source_special.unlink()

    def test_verifier_rejects_stage_drift_oracle_sibling_and_extra_files(self):
        self._validated_stage("verify-target", "DEL-03", "with")
        root = self.output / "verify-target"
        model = root / "model-stage"
        prompt = model / "prompt.md"
        os.chmod(prompt, 0o644)
        prompt.write_bytes(prompt.read_bytes() + b"drift")
        with self.assertRaises(stage_module.StageError):
            stage_module.verify(root)
        prompt.write_bytes((self.source / "arms/with/cases/DEL-03/prompt.md").read_bytes())
        os.chmod(prompt, 0o444)
        oracle = model / "oracle" / "reference.json"
        os.chmod(model, 0o755)
        oracle.parent.mkdir(mode=0o700)
        oracle.write_text("{}")
        os.chmod(oracle.parent, 0o555)
        os.chmod(model, 0o555)
        with self.assertRaises(stage_module.StageError):
            stage_module.verify(root)
        os.chmod(model, 0o755)
        os.chmod(oracle.parent, 0o700)
        shutil.rmtree(oracle.parent)
        sibling = model / "cases" / "INV-02" / "prompt.md"
        sibling.parent.mkdir(parents=True)
        sibling.write_text("sibling")
        with self.assertRaises(stage_module.StageError):
            stage_module.verify(root)


if __name__ == "__main__":
    unittest.main()
