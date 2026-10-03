"""Offline controls for exactly eleven remaining full24 selected answer graders."""
from __future__ import annotations

import copy
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent


def _load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


# Reuse only the DEL packet's engine utilities, never its TestCase or tests.
delivery = _load("remaining_delivery_utilities", ROOT / "test_delivery_graders.py")
helpers = _load("remaining_source_helpers", ROOT / "remaining_grader_helpers.py")
prepare = delivery.prepare
CASES = ("HLT-01", "HLT-02", "HLT-03", "HLT-04", "INV-04", "PRE-01", "PRE-03", "PRE-04", "RUL-02", "RUL-03", "RUL-04")
COMPACT_ONLY = {"HLT-04", "PRE-03"}


def _json(value):
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False)


def _leaves(value, path=()):
    if isinstance(value, dict):
        for key, child in value.items():
            yield from _leaves(child, path + (key,))
    elif isinstance(value, list):
        for idx, child in enumerate(value):
            yield from _leaves(child, path + (idx,))
    else:
        yield path, value


def _parent(value, path):
    for key in path[:-1]:
        value = value[key]
    return value


def _duplicate_json(value, path):
    """Insert a duplicate at its exact containing object, including nested records."""
    def serialize(item, current=()):
        if isinstance(item, dict):
            fields = []
            for key, child in item.items():
                field = _json(key) + ":" + serialize(child, current + (key,))
                fields.append(field)
                if current + (key,) == path:
                    fields.append(field)
            return "{" + ",".join(fields) + "}"
        if isinstance(item, list):
            return "[" + ",".join(serialize(child, current + (idx,)) for idx, child in enumerate(item)) + "]"
        return _json(item)
    return serialize(value)


class RemainingGraderControls(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.node = delivery._find_node()
        if cls.node is None:
            raise AssertionError("Existing Node.js required for authored-vector dialect checks; install nothing")
        cls.temp = tempfile.TemporaryDirectory(prefix="full24-remaining-graders-")
        cls.addClassCleanup(cls.temp.cleanup)
        cls.out = Path(cls.temp.name) / "prepared"
        prepare.prepare(cls.out)
        cls.report = prepare.validate(cls.out)
        rows = {row["id"]: row for row in cls.report["cases"]}
        cls.definitions = {}
        for case in CASES:
            selected = rows[case]["selectedAnswerGraders"]
            if len(selected) != 1 or selected[0]["type"] != "regex" or selected[0]["weight"] != 1 or selected[0]["source"] != "case-source":
                raise AssertionError(f"{case}: expected one positive-weight source answer regex")
            name = selected[0]["name"]
            raw = (cls.out / "oracle" / case / "selected-graders" / name).read_bytes()
            source = prepare.REPO / rows[case]["sourceCase"] / "graders" / name
            if raw != source.read_bytes():
                raise AssertionError(f"{case}: selected oracle grader bytes differ from frozen source")
            text = raw.decode("utf-8", "strict")
            if text.startswith("---\n"):
                text = text.split("---\n", 2)[1]
            definition = prepare._yaml_load(text)
            if prepare._grader_definition(raw) != {"type": "regex", "weight": 1}:
                raise AssertionError(f"{case}: grader metadata changed")
            if definition["type"] != "regex" or definition["target"] != "last_message" or not isinstance(definition["pattern"], str):
                raise AssertionError(f"{case}: unexpected answer target or pattern")
            flags = definition.get("flags", "")
            if not isinstance(flags, str) or any(flag not in "ims" for flag in flags) or len(set(flags)) != len(flags):
                raise AssertionError(f"{case}: invalid declared flags")
            definition["_sha256"] = prepare.digest(raw)
            cls.definitions[case] = definition
        cls.answers = helpers.EvidenceAnswers(cls.out, prepare.REPO, prepare._yaml_load)

    def assert_vectors(self, case, candidates, expected):
        definition = self.definitions[case]
        flags = sum(value for flag, value in (("i", re.I), ("m", re.M), ("s", re.S)) if flag in definition.get("flags", ""))
        compiled = re.compile(definition["pattern"], flags)
        python_results = [compiled.search(answer) is not None for answer in candidates]
        packet = dict(pattern=definition["pattern"], flags=definition.get("flags", ""), target=definition["target"], answers=candidates)
        encoded = json.dumps(packet, ensure_ascii=False).encode()
        self.assertLessEqual(len(encoded), delivery.MAX_NODE_INPUT, f"{case}: bounded Node stdin")
        proc = subprocess.run([self.node, "-e", delivery.NODE_SCRIPT], input=encoded, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, timeout=delivery.NODE_TIMEOUT_SECONDS, check=False,
                              env={"PATH": os.environ.get("PATH", "")})
        self.assertEqual(proc.returncode, 0, proc.stderr[:1000])
        self.assertLessEqual(len(proc.stdout), delivery.MAX_NODE_OUTPUT)
        node_results = json.loads(proc.stdout)
        self.assertIsInstance(node_results, list)
        self.assertEqual(len(node_results), len(candidates))
        self.assertTrue(all(type(item) is bool for item in node_results))
        self.assertEqual(python_results, expected, f"{case}: Python authored vectors")
        self.assertEqual(node_results, expected, f"{case}: Node authored vectors")
        self.assertEqual(node_results, python_results, f"{case}: parity limited to authored vectors")

    def test_exact_selected_case_set_and_frozen_oracle_metadata(self):
        self.assertEqual(set(self.definitions), set(CASES))
        self.assertEqual(len(self.definitions), 11)
        self.assertEqual(self.report["caseCount"], 24)
        for case in CASES:
            with self.subTest(case=case):
                self.assertRegex(self.definitions[case]["_sha256"], r"^[0-9a-f]{64}$")
                self.assertEqual(self.definitions[case]["target"], "last_message")

    def test_source_bound_canonical_and_declared_whitespace_vectors(self):
        for case in CASES:
            with self.subTest(case=case):
                answer = self.answers.answer(case)
                pretty = " \n\t" + json.dumps(answer, indent=2, ensure_ascii=False) + " \n"
                self.assert_vectors(case, [_json(answer), pretty], [True, case not in COMPACT_ONLY])

    def test_each_leaf_rejects_wrong_missing_duplicate_and_wrong_type(self):
        for case in CASES:
            answer = self.answers.answer(case)
            for path, value in _leaves(answer):
                with self.subTest(case=case, field=path):
                    wrong, missing = (copy.deepcopy(answer) for _ in range(2))
                    _parent(wrong, path)[path[-1]] = "WRONG_FROZEN_EVIDENCE"
                    del _parent(missing, path)[path[-1]]
                    candidates = [_json(wrong), _json(missing), _duplicate_json(answer, path)]
                    for value in (None, 0, True, [], {}):
                        wrong_type = copy.deepcopy(answer)
                        _parent(wrong_type, path)[path[-1]] = value
                        candidates.append(_json(wrong_type))
                    self.assert_vectors(case, candidates, [False] * len(candidates))

    def test_extra_partial_prose_and_nested_structure_controls(self):
        for case in CASES:
            with self.subTest(case=case):
                answer = self.answers.answer(case)
                canonical = _json(answer)
                extra = copy.deepcopy(answer)
                extra["unrequested_extra"] = "value"
                candidates = [_json(extra), canonical[:-1], "Answer: " + canonical, canonical + " unsupported claim",
                              "```json\n" + canonical + "\n```", _json({}), _json([])]
                if "records" in answer:
                    missing_record, duplicate_record, extra_field = (copy.deepcopy(answer) for _ in range(3))
                    missing_record["records"].pop()
                    duplicate_record["records"].append(copy.deepcopy(answer["records"][0]))
                    extra_field["records"][0]["extra"] = "value"
                    candidates += [_json(missing_record), _json(duplicate_record), _json(extra_field), _duplicate_json(answer, ("records",))]
                    without_records = copy.deepcopy(answer)
                    del without_records["records"]
                    candidates.append(_json(without_records))
                    for value in (None, 0, True, "records", {}):
                        wrong_records = copy.deepcopy(answer)
                        wrong_records["records"] = value
                        candidates.append(_json(wrong_records))
                self.assert_vectors(case, candidates, [False] * len(candidates))

    def test_unknown_degraded_and_correlation_counterclaims(self):
        controls = {
            "HLT-01": {"report_health": "Healthy", "release_identity": "PROVEN"},
            "HLT-02": {"ready_proves_workload_healthy": "YES", "observation_scope": "ATOMIC", "current_time_claim": "CURRENT"},
            "HLT-03": {"residual_cause": "CONTROLLER_FAILURE", "observation_scope": "CURRENT_ATOMIC"},
            "HLT-04": {"renewal_proves_new_check": "YES", "future_timestamp_proves_freshness": "YES", "check_receipt_consumed": "YES"},
            "INV-04": {"denied_deployment_count": "0", "denied_ownership": "Native", "readable_objects_orphan_status": "ORPHAN"},
            "PRE-01": {"health_claim": "HEALTHY", "object_before_successful_apply": "ABSENT_BECAUSE_ROUTE_404"},
            "PRE-03": {"parent_status_proves_child_health": "YES", "crossplane_evidence": "ESTABLISHED", "cleanup_status": "CONFIRMED"},
            "PRE-04": {"cert_manager_health": "Healthy", "unknown_interpretation": "unmanaged", "spoke_argo_live_observation": "INSTALLED"},
            "RUL-02": {"changed_response_before_refresh_served": "YES", "automatic_invalidation": "DEMONSTRATED", "freshness_guarantee": "ESTABLISHED"},
            "RUL-03": {"denied_inventory": "EMPTY", "denied_context": "rul03-readable"},
            "RUL-04": {"image_identity_verdict": "MATCH", "applied_source_binding": "PROVEN"},
        }
        for case, changes in controls.items():
            candidates = []
            for field, value in changes.items():
                changed = self.answers.answer(case)
                changed[field] = value
                candidates.append(_json(changed))
            with self.subTest(case=case):
                self.assert_vectors(case, candidates, [False] * len(candidates))


if __name__ == "__main__":
    unittest.main()
