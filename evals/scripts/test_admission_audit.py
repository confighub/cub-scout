"""Synthetic, offline tests for admission_audit.py (no harness invocation)."""
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).with_name("admission_audit.py")
SPEC = importlib.util.spec_from_file_location("admission_audit", SCRIPT)
audit_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(audit_module)


class AdmissionAuditTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.out = self.root / "out"
        self.out.mkdir()
        self.result = self.root / "result.json"
        self.records = [
            {"type": "system", "subtype": "init", "model": "claude-haiku-4-5", "tools": ["Read", "Glob"]},
            {"type": "assistant", "message": {"content": [{"type": "text", "text": "answer"}]}},
            {"type": "result", "subtype": "success", "is_error": False, "result": "answer"},
        ]
        self.data = {
            "partial": False,
            "costUsd": 0.4,
            "suite": {"modelOverride": "claude-haiku-4-5"},
            "cases": [{
                "name": "synthetic",
                "runsPerCase": 3,  # Must not override caller plan.
                "maxTurns": 4,
                "timeoutSeconds": 30,
                "arms": {"with": [self.make_run("with.jsonl")], "without": [self.make_run("without.jsonl")]},
            }],
        }
        self.write_traces()
        self.write_result()

    def tearDown(self):
        self.tmp.cleanup()

    def make_run(self, name, **updates):
        return {"tracePath": str(self.out / name), "turns": 2, "durationSeconds": 4,
                "costUsd": 0.2, **updates}

    def write_traces(self):
        for name in ("with.jsonl", "without.jsonl"):
            (self.out / name).write_text("".join(json.dumps(r) + "\n" for r in self.records), encoding="utf-8")

    def write_result(self):
        self.result.write_text(json.dumps(self.data), encoding="utf-8")

    def audit(self, arms=("with", "without"), runs=1):
        return audit_module.audit(self.result, [self.out], list(arms), runs)

    def test_complete_simple_pair_admitted_and_order_independent_inventory(self):
        self.records[0]["tools"] = ["Glob", "Read"]
        self.write_traces()
        report = self.audit()
        self.assertEqual(report["status"], "ADMITTED")
        case = report["cases"][0]
        self.assertEqual(case["ordinaryInventoryComparison"], {"comparable": True, "equal": True})
        self.assertEqual(case["declaredRunsPerCaseIgnored"], 3)
        self.assertEqual(report["producerCostUsdObservedUnreconciled"], 0.4)

    def test_treatment_mcp_inventory_is_reported_not_compared_as_ordinary(self):
        self.records[0]["tools"] = ["Read", "Glob"]
        self.write_traces()
        trace = self.out / "with.jsonl"
        with_records = [dict(r) for r in self.records]
        with_records[0] = dict(with_records[0], tools=["Read", "Glob", "mcp__scout__map"])
        trace.write_text("".join(json.dumps(r) + "\n" for r in with_records), encoding="utf-8")
        report = self.audit()
        self.assertEqual(report["status"], "ADMITTED")
        case = report["cases"][0]
        self.assertTrue(case["ordinaryInventoryComparison"]["equal"])
        self.assertEqual(case["arms"]["with"][0]["inventory"]["mcp"], ["mcp__scout__map"])

    def test_init_model_mismatch_or_declared_model_mismatch_rejected(self):
        self.records[0]["model"] = "other-model"
        self.write_traces()
        report = self.audit()
        self.assertEqual(report["status"], "NOT_ADMITTED")
        self.assertTrue(any("differs from producer-declared" in r for r in report["cases"][0]["reasons"]))

    def test_caller_plan_detects_default_run_metadata_mismatch(self):
        report = self.audit(runs=3)
        self.assertEqual(report["status"], "NOT_ADMITTED")
        self.assertTrue(any("expected 3 run(s)" in r for r in report["cases"][0]["reasons"]))

    def test_missing_terminal_and_duplicate_terminal_or_init_rejected(self):
        for mutation in (lambda: self.records.pop(),
                         lambda: self.records.append(dict(self.records[-1])),
                         lambda: self.records.insert(1, dict(self.records[0]))):
            self.records = [
                {"type": "system", "subtype": "init", "model": "haiku", "tools": ["Read"]},
                {"type": "result", "subtype": "success", "result": "ok"},
            ]
            mutation()
            self.write_traces()
            self.assertEqual(self.audit()["status"], "NOT_ADMITTED")

    def test_timeout_rejected_even_when_partial_false(self):
        self.data["cases"][0]["arms"]["with"][0]["error"] = "timed out after 30s"
        self.write_result()
        report = self.audit()
        self.assertFalse(report["cases"][0]["arms"]["with"][0]["admitted"])
        self.assertEqual(self.data["partial"], False)

    def test_turns_over_declared_max_rejected(self):
        self.data["cases"][0]["arms"]["with"][0]["turns"] = 5
        self.write_result()
        self.assertEqual(self.audit()["status"], "NOT_ADMITTED")

    def test_nested_mcp_progress_without_call_body_is_not_zero_or_admitted(self):
        self.records.insert(1, {"type": "system", "subtype": "task_started", "task_id": "child", "spawn_depth": 3})
        self.records.insert(2, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "spawn_depth": 3, "last_tool_name": "mcp__plugin_example__map",
                                "description": "tool name appeared in this progress record"})
        self.records.insert(3, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "spawn_depth": 3, "last_tool_name": "mcp__plugin_example__map"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertFalse(arm["admitted"])
        self.assertEqual(arm["nestedProgressEvidence"]["mcpMentions"], ["mcp__plugin_example__map"])
        self.assertEqual(len(arm["nestedProgressEvidence"]["mcpProgressEvents"]), 2)
        self.assertEqual(arm["taskDepthMaxObserved"], 3)
        self.assertEqual(arm["unresolvedStartedTaskIds"], ["child"])
        self.assertTrue(any("nested progress mentions MCP" in r for r in arm["reasons"]))

    def test_mcp_like_prose_is_not_normalized_into_progress_tool(self):
        self.records.insert(1, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "description": "The string mcp__plugin_example__map appears in prose."})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertEqual(arm["nestedProgressEvidence"]["mcpMentions"], [])

    def test_task_progress_depth_is_resolved_from_exact_task_id(self):
        self.records.insert(1, {"type": "system", "subtype": "task_started", "task_id": "child",
                                "tool_use_id": "toolu_child", "spawn_depth": 2})
        self.records.insert(2, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "last_tool_name": "Read"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertEqual(arm["taskDepthMaxObserved"], 2)
        self.assertEqual(arm["taskProgressEvents"][0]["spawnDepthResolved"], 2)
        self.assertTrue(any("descendant inventory/cost coverage" in r for r in arm["reasons"]))

    def test_unknown_task_depth_is_none_and_blocks_admission(self):
        self.records.insert(1, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "last_tool_name": "Read"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertIsNone(arm["taskDepthMaxObserved"])
        self.assertIsNone(arm["taskProgressEvents"][0]["spawnDepthResolved"])
        self.assertTrue(any("no matching task_started" in r for r in arm["reasons"]))

    def test_progress_depth_cannot_be_resolved_from_malformed_start_depth(self):
        self.records.insert(1, {"type": "system", "subtype": "task_started", "task_id": "child",
                                "spawn_depth": "one"})
        self.records.insert(2, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "last_tool_name": "Read"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertIsNone(arm["taskDepthMaxObserved"])
        self.assertIsNone(arm["taskProgressEvents"][0]["spawnDepthResolved"])
        self.assertTrue(any("malformed spawn_depth" in r for r in arm["reasons"]))

    def test_wrong_negative_and_conflicting_task_depths_rejected(self):
        for reported, reason in (("two", "malformed"), (-1, "malformed"), (3, "conflicts")):
            with self.subTest(reported=reported):
                self.records = [
                    {"type": "system", "subtype": "init", "model": "claude-haiku-4-5", "tools": ["Read", "Glob"]},
                    {"type": "system", "subtype": "task_started", "task_id": "child", "spawn_depth": 2},
                    {"type": "system", "subtype": "task_progress", "task_id": "child", "spawn_depth": reported,
                     "last_tool_name": "Read"},
                    {"type": "result", "subtype": "success", "is_error": False, "result": "answer"},
                ]
                self.write_traces()
                arm = self.audit()["cases"][0]["arms"]["with"][0]
                self.assertFalse(arm["admitted"])
                self.assertTrue(any(reason in r for r in arm["reasons"]))
                if reported == 3:
                    self.assertIsNone(arm["taskDepthMaxObserved"])

    def test_progress_a_and_completion_b_without_starts_are_not_admitted(self):
        self.records.insert(1, {"type": "system", "subtype": "task_progress", "task_id": "A",
                                "spawn_depth": 1, "last_tool_name": "Read"})
        self.records.insert(2, {"type": "system", "subtype": "task_notification", "task_id": "B",
                                "status": "completed"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertFalse(arm["admitted"])
        self.assertTrue(any("exact task_id" in r for r in arm["reasons"]))
        self.assertTrue(any("no matching task_started/task_progress" in r for r in arm["reasons"]))

    def test_task_completion_does_not_certify_descendant_accounting(self):
        self.records.insert(1, {"type": "system", "subtype": "task_started", "task_id": "child",
                                "spawn_depth": 1})
        self.records.insert(2, {"type": "system", "subtype": "task_progress", "task_id": "child",
                                "spawn_depth": 1, "last_tool_name": "Read"})
        self.records.insert(3, {"type": "system", "subtype": "task_notification", "task_id": "child",
                                "status": "completed"})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertFalse(arm["admitted"])
        self.assertTrue(any("descendant inventory/cost coverage" in r for r in arm["reasons"]))

    def test_malformed_error_values_are_rejected(self):
        for value in (False, 0, "", {}, []):
            with self.subTest(value=value):
                self.data["cases"][0]["arms"]["with"][0]["error"] = value
                self.write_result()
                arm = self.audit()["cases"][0]["arms"]["with"][0]
                self.assertFalse(arm["admitted"])
                self.assertTrue(any("error field is malformed" in r for r in arm["reasons"]))

    def test_fifo_trace_is_rejected_without_blocking(self):
        fifo = self.out / "trace.fifo"
        os.mkfifo(fifo)
        self.data["cases"][0]["arms"]["with"][0]["tracePath"] = str(fifo)
        self.write_result()
        output = self.root / "fifo-audit.json"
        completed = subprocess.run(
            [sys.executable, str(SCRIPT), str(self.result), "--trace-root", str(self.out),
             "--expected-arms", "with,without", "--expected-runs", "1", "--out", str(output)],
            capture_output=True, text=True, timeout=3,
        )
        self.assertEqual(completed.returncode, 2)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertIn("regular non-symlink file", report["cases"][0]["arms"]["with"][0]["reasons"][0])

    def test_linux_repo_path_exception_does_not_allow_nested_home_or_other_home(self):
        linux_repo = Path("/home/runner/work/cub-scout/cub-scout")
        linux_home = Path("/home/runner")
        with patch.object(audit_module, "REPO_ROOT", linux_repo), patch.object(audit_module, "USER_HOME", linux_home):
            self.assertTrue(audit_module.safe_location(linux_repo / "evals/trace-admission-example/out"))
            self.assertFalse(audit_module.safe_location(linux_repo / "evals/home/out"))
            self.assertFalse(audit_module.safe_location(linux_home / ".ssh/out"))
            self.assertFalse(audit_module.safe_location(linux_repo / "sealed/out"))

    def test_source_partial_true_and_bad_cost_block_admission(self):
        self.data["partial"] = True
        self.data["costUsd"] = "unknown"
        self.write_result()
        report = self.audit()
        self.assertEqual(report["status"], "NOT_ADMITTED")
        self.assertIsNone(report["producerCostUsdObservedUnreconciled"])
        self.assertTrue(any("partial flag is true" in r for r in report["cases"][0]["reasons"]))

    def test_wrong_typed_terminal_error_flag_rejected(self):
        self.records[-1]["is_error"] = "false"
        self.write_traces()
        report = self.audit()
        self.assertFalse(report["cases"][0]["arms"]["with"][0]["terminalResultSuccessful"])

    def test_unadvertised_visible_tool_is_not_normalized(self):
        self.records.insert(1, {"type": "assistant", "message": {"content": [
            {"type": "tool_use", "id": "x", "name": "Agent", "input": {}}]}})
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertTrue(any("absent from init inventory: Agent" in r for r in arm["reasons"]))

    def test_missing_or_extra_inventory_cannot_be_compared(self):
        self.records[0].pop("tools")
        self.write_traces()
        case = self.audit()["cases"][0]
        self.assertFalse(case["ordinaryInventoryComparison"]["comparable"])
        self.assertEqual(case["status"], "NOT_ADMITTED")

    def test_inventory_inequality_rejected(self):
        self.records[0]["tools"] = ["Read", "Glob"]
        self.write_traces()
        second = self.out / "without.jsonl"
        alt = [dict(r) for r in self.records]
        alt[0] = dict(alt[0], tools=["Read", "Write"])
        second.write_text("".join(json.dumps(r) + "\n" for r in alt), encoding="utf-8")
        case = self.audit()["cases"][0]
        self.assertFalse(case["ordinaryInventoryComparison"]["equal"])
        self.assertEqual(case["status"], "NOT_ADMITTED")

    def test_nonfinite_source_json_rejected(self):
        for number in ("NaN", "Infinity", "-Infinity", "1e999"):
            with self.subTest(number=number):
                raw = json.dumps(self.data).replace('"costUsd": 0.4', '"costUsd": ' + number, 1)
                self.result.write_text(raw)
                with self.assertRaisesRegex(ValueError, "non-finite JSON number"):
                    self.audit()

    def test_duplicate_source_keys_rejected(self):
        self.result.write_text(json.dumps(self.data).replace('"partial": false',
                               '"partial": true, "partial": false', 1))
        with self.assertRaisesRegex(ValueError, "duplicate JSON object key"):
            self.audit()

    def test_ambiguous_or_nonfinite_trace_json_rejected(self):
        for record in (
            '{"type":"result","subtype":"success","is_error":true,"is_error":false,"result":"answer"}',
            '{"type":"result","subtype":"success","is_error":false,"result":"answer","cost":NaN}',
        ):
            with self.subTest(record=record):
                lines = [json.dumps(r) for r in self.records[:-1]] + [record]
                (self.out / "with.jsonl").write_text("\n".join(lines) + "\n")
                report = self.audit()
                self.assertEqual(report["status"], "NOT_ADMITTED")
                reasons = report["cases"][0]["arms"]["with"][0]["reasons"]
                self.assertTrue(any("JSON" in reason for reason in reasons))

    def test_oversized_integer_cost_is_unknown_without_overflow(self):
        self.data["costUsd"] = 10 ** 400
        self.data["cases"][0]["arms"]["with"][0]["costUsd"] = 10 ** 400
        self.write_result()
        report = self.audit()
        self.assertEqual(report["status"], "NOT_ADMITTED")
        self.assertIsNone(report["producerCostUsdObservedUnreconciled"])

    def test_malformed_trace_record_rejected(self):
        (self.out / "with.jsonl").write_text('{"type":"system"}\nnot json\n', encoding="utf-8")
        report = self.audit()
        self.assertEqual(report["status"], "NOT_ADMITTED")
        self.assertIn("malformed JSON", report["cases"][0]["arms"]["with"][0]["reasons"][0])

    def test_malformed_assistant_message_rejected(self):
        self.records[1] = {"type": "assistant", "message": "not a message object"}
        self.write_traces()
        arm = self.audit()["cases"][0]["arms"]["with"][0]
        self.assertFalse(arm["admitted"])
        self.assertTrue(any("assistant message/content" in r for r in arm["reasons"]))

    def test_outside_or_sealed_path_rejected_without_opening(self):
        self.data["cases"][0]["arms"]["with"][0]["tracePath"] = str(self.root / "not-allowed.jsonl")
        self.write_result()
        self.assertEqual(self.audit()["status"], "NOT_ADMITTED")
        self.assertFalse((self.root / "not-allowed.jsonl").exists())
        self.data["cases"][0]["arms"]["with"][0]["tracePath"] = "/private/tmp/e-example/out/trace.jsonl"
        self.write_result()
        self.assertEqual(self.audit()["status"], "NOT_ADMITTED")

    def test_symlink_escape_rejected(self):
        outside = self.root / "elsewhere"
        outside.mkdir()
        target = outside / "trace.jsonl"
        target.write_text("{}\n", encoding="utf-8")
        link = self.out / "escape.jsonl"
        link.symlink_to(target)
        self.data["cases"][0]["arms"]["with"][0]["tracePath"] = str(link)
        self.write_result()
        self.assertEqual(self.audit()["status"], "NOT_ADMITTED")

    def test_input_bounds_are_enforced(self):
        self.result.write_bytes(b" " * (audit_module.MAX_RESULT_BYTES + 1))
        with self.assertRaisesRegex(ValueError, "byte limit"):
            self.audit()

    def test_trace_input_bound_is_enforced(self):
        trace = self.out / "with.jsonl"
        trace.write_bytes(b" " * (audit_module.MAX_TRACE_BYTES + 1))
        report = self.audit()
        self.assertTrue(any("byte limit" in r for r in report["cases"][0]["arms"]["with"][0]["reasons"]))


if __name__ == "__main__":
    unittest.main()
