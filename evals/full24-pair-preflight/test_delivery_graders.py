"""Offline DEL-01..04 selected-answer regex controls; no eval runner is called."""
from __future__ import annotations

import copy
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent
PREPARE_SPEC = importlib.util.spec_from_file_location("full24_delivery_prepare", ROOT / "prepare.py")
prepare = importlib.util.module_from_spec(PREPARE_SPEC)
assert PREPARE_SPEC and PREPARE_SPEC.loader
PREPARE_SPEC.loader.exec_module(prepare)

DELIVERY_CASES = ("DEL-01", "DEL-02", "DEL-03", "DEL-04")
NODE_SCRIPT = r"""
const fs = require('fs');
const packet = JSON.parse(fs.readFileSync(0, 'utf8'));
if (packet.target !== 'last_message') throw new Error('unexpected target');
const regex = new RegExp(packet.pattern, packet.flags || '');
process.stdout.write(JSON.stringify(packet.answers.map(answer => regex.test(answer))));
"""
MAX_NODE_INPUT = 256 * 1024
MAX_NODE_OUTPUT = 4096
# A bound on one Node process, there to stop a regex that backtracks without
# end. It has to cover a cold Node start on a loaded shared CI runner: the
# regex work itself is about 0.02s and a start about 0.07s locally, but a
# start exceeded 5s on CI and failed an unrelated PR (#828).
NODE_TIMEOUT_SECONDS = 30


def _find_node() -> str | None:
    node = shutil.which("node")
    if node:
        return node
    fallback = Path("/opt/homebrew/bin/node")
    return str(fallback) if fallback.is_file() else None


def _frontmatter(raw: bytes) -> dict:
    text = raw.decode("utf-8", "strict")
    match = re.fullmatch(r"---\n(.*?)\n---\n?(.*)", text, re.DOTALL)
    if not match:
        raise AssertionError("selected grader frontmatter is malformed")
    return prepare._yaml_load(match.group(1))


def _json(answer: dict) -> str:
    return json.dumps(answer, separators=(",", ":"), ensure_ascii=False)


class DeliveryGraderControls(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.node = _find_node()
        if cls.node is None:
            raise AssertionError("Node.js is required for grader-dialect parity; install nothing from this test")
        cls.temp = tempfile.TemporaryDirectory(prefix="full24-delivery-graders-")
        cls.out = Path(cls.temp.name) / "prepared"
        prepare.prepare(cls.out)
        cls.report = prepare.validate(cls.out)
        cls.cases = {case["id"]: case for case in cls.report["cases"]}
        cls.grader_defs = {}
        for case_id in DELIVERY_CASES:
            cls.grader_defs[case_id] = cls._load_selected_grader(case_id)

    @classmethod
    def tearDownClass(cls):
        if hasattr(cls, "temp"):
            cls.temp.cleanup()

    @classmethod
    def _load_selected_grader(cls, case_id: str) -> dict:
        case = cls.cases[case_id]
        selected = case["selectedAnswerGraders"]
        if len(selected) != 1 or selected[0] != {
                "name": "verified-answer.md", "type": "regex", "weight": 1, "source": "case-source"}:
            raise AssertionError(f"{case_id} does not have the expected selected weight-1 answer regex")
        raw = (cls.out / "oracle" / case_id / "selected-graders" / selected[0]["name"]).read_bytes()
        metadata = prepare._grader_definition(raw)
        definition = _frontmatter(raw)
        if metadata != {"type": "regex", "weight": 1}:
            raise AssertionError(f"{case_id} selected grader metadata changed: {metadata}")
        if definition.get("type") != "regex" or definition.get("target") != "last_message":
            raise AssertionError(f"{case_id} must select a regex over last_message")
        if not isinstance(definition.get("pattern"), str):
            raise AssertionError(f"{case_id} selected regex is missing")
        flags = definition.get("flags", "")
        if not isinstance(flags, str) or any(flag not in "ims" for flag in flags) or len(set(flags)) != len(flags):
            raise AssertionError(f"{case_id} has invalid regex flags")
        definition["_raw_sha256"] = prepare.digest(raw)
        return definition

    @classmethod
    def _input_path(cls, case_id: str, relative: str) -> Path:
        return cls.out / "arms" / "without" / "cases" / case_id / relative

    @classmethod
    def _yaml_input(cls, case_id: str, relative: str) -> dict:
        return prepare._yaml_load(cls._input_path(case_id, relative).read_text())

    @classmethod
    def _answer(cls, case_id: str) -> dict:
        if case_id == "DEL-01":
            projection = cls._yaml_input(case_id, "cluster/flux-applied-digest.yaml")
            lines = {line["source_line"]: line["text"] for line in projection["excerpt_lines"]}
            revision = re.search(r"resumed at (main@sha1:[0-9a-f]+);", lines[71])
            digest = re.search(r"latest@(sha256:[0-9a-f]+),", lines[72])
            if not revision or not digest:
                raise AssertionError("DEL-01 source log no longer binds the expected revision and digest")
            if projection["scope"].get("snapshot") is not False or len(projection["source"]["ranges"]) < 2:
                raise AssertionError("DEL-01 excerpt no longer identifies trimmed, non-snapshot evidence")
            return {
                "ready_apps_digest": digest.group(1),
                "source_revision": revision.group(1),
                "evidence_kind": "recorded_run_log",
                "snapshot_limit": "trimmed_sequential_not_atomic",
            }

        if case_id == "DEL-02":
            projection = cls._yaml_input(case_id, "cluster/argo-publication-lag.yaml")
            text = " ".join(" ".join(line["text"] for line in projection["excerpt_lines"]).split())
            required = ("still unread ninety seconds later", "previous digest", "previous replica count",
                        "hard refresh", "re-resolves the tag to a digest")
            if not all(item in text for item in required):
                raise AssertionError("DEL-02 excerpt no longer supports the publication/consumption distinction")
            if re.search(r"sha256:[0-9a-f]{64}", text) or re.search(r"\b[0-9]+ replicas?\b", text):
                raise AssertionError("DEL-02 source unexpectedly supplies an exact digest or replica count")
            return {
                "consumed_before_refresh": "NO",
                "pre_refresh_state": "PREVIOUS_RELEASE",
                "transition": "HARD_REFRESH_RESOLVES_LATEST",
                "exact_digests_and_counts": "UNKNOWN",
            }

        if case_id == "DEL-03":
            status = cls._input_path(case_id, "cluster/onboard-excerpt.md").read_text()
            behavior = cls._input_path(case_id, "cluster/known-behaviours-excerpt.md").read_text()
            receipt = cls._yaml_input(case_id, "cluster/sveltos-oci-delivery-proof.yaml")["spec"]
            row = re.search(r"(?m)^eu-central-uat1\s+mer-kyverno-eu-central-uat1\s+Synced\s+Healthy\s+(sha256:[0-9a-f]+)\s+yes$", status)
            if not row or "does not report which release it fetched" not in behavior:
                raise AssertionError("DEL-03 status and inference facts are no longer source-bound")
            pilot = next(variant for variant in receipt["variants"] if variant["cluster"] == "hx-sveltos-fleet-pilot")
            match = pilot["records"][0]["delivery"]["profileMatchesApprovedRevision"]
            checkpoint = next(item for item in receipt["checkpoints"] if item["id"] == "after-wave-1")
            observation = next(item["observation"] for item in checkpoint["observations"]
                               if item["logicalCluster"] == "hx-sveltos-fleet-pilot")
            helm_release = observation["helmRelease"]
            if helm_release["chart"] != "kyverno-3.8.1" or helm_release["status"] != "deployed":
                raise AssertionError("DEL-03 receipt no longer binds deployed workload evidence")
            if "eu-central-uat1" == pilot["cluster"] or "runtimeSveltosDigest" in pilot:
                raise AssertionError("DEL-03 fixtures unexpectedly join the status row to a runtime digest")
            return {
                "status_cluster": "eu-central-uat1",
                "status_revision": row.group(1),
                "status_revision_basis": "inferred_from_timestamps",
                "receipt_cluster": pilot["cluster"],
                "receipt_profile_matches_approved_revision": match,
                "receipt_workload_evidence": "helmrelease_chart_deployed",
                "runtime_sveltos_digest": "UNKNOWN",
                "cross_artifact_join": "UNESTABLISHED",
            }

        if case_id == "DEL-04":
            intent = cls._yaml_input(case_id, "evidence/oci-identity-lifecycle/render-intent.yaml")["spec"]
            render = cls._yaml_input(case_id, "evidence/oci-identity-lifecycle/render-receipt.yaml")["spec"]
            publication = cls._yaml_input(case_id, "evidence/oci-identity-lifecycle/installer-publication-receipt.yaml")["spec"]
            chain = cls._yaml_input(case_id, "evidence/oci-identity-lifecycle/oci-evidence-chain.yaml")
            delivery = cls._yaml_input(case_id, "evidence/oci-identity-lifecycle/catalog-delivery-proof.yaml")["spec"]
            layers = delivery["legs"]
            digests = [leg["digest"] for leg in layers.values()]
            references = [leg["workload"]["image"] for leg in layers.values()]
            same_consumed_digest = len(set(digests)) == 1 and digests[0] == delivery["releaseOci"]["digest"]
            if not same_consumed_digest or delivery["releaseOci"]["sameDigestAcrossConsumers"] != "yes":
                raise AssertionError("DEL-04 consumers no longer bind to the same recorded OCI digest")
            if len(set(references)) != 1:
                raise AssertionError("DEL-04 consumer image references diverged")
            intent_inputs = intent["renderInputs"]
            publication_outputs = publication["outputs"]
            receipt_outputs = render["outputs"]
            render_intent_evidence = intent["evidence"]
            lifecycle = intent["lifecycle"]
            renderer_flags = render["renderer"]["flags"]
            policy_limits = " ".join(chain["status"]["limits"] + delivery["limits"])
            expected = {
                "package_oci_reference": intent_inputs["installerPackageOciRef"],
                "package_manifest_digest": publication_outputs["manifestDigest"],
                "package_layer_digest": publication_outputs["layerDigest"],
                "rendered_manifest_sha256": delivery["render"]["manifestSha256"],
                "rendered_object_set_sha256": receipt_outputs["renderedObjectSetSHA256"],
                "confighub_release_id": delivery["releaseOci"]["releaseId"],
                "output_oci_digest": delivery["releaseOci"]["digest"],
                "bundle_digest": delivery["releaseOci"]["bundleDigest"],
                "consumer_digests_match": "YES" if same_consumed_digest else "NO",
                "recorded_consumer_results": ";".join(
                    f"{leg['mode']}={leg['workload']['replicas']}" for leg in layers.values()),
                "recorded_image_reference": references[0],
                "runtime_image_id": "UNKNOWN",
                "current_cluster_state": "UNKNOWN",
                "independent_bundle_verification": "UNKNOWN",
                "recorded_hook_policy": intent_inputs["hookPolicy"],
                "no_hooks_render_flag": next(flag for flag in renderer_flags if flag == "--no-hooks"),
                "lifecycle_observed": render_intent_evidence["lifecycleObserved"],
                "hook_execution": "UNKNOWN",
                "policy_execution": "UNKNOWN",
                "observation_time": delivery["observedAt"],
            }
            chain_delivery = chain["spec"]["boundaries"]["delivery"]
            if chain_delivery["digest"] != expected["output_oci_digest"]:
                raise AssertionError("DEL-04 chain and receipt output identities disagree")
            if not expected["package_oci_reference"].endswith("@" + expected["package_manifest_digest"]):
                raise AssertionError("DEL-04 immutable package reference no longer binds its manifest digest")
            if expected["lifecycle_observed"] != "n/a" or expected["recorded_hook_policy"] != "no-hooks":
                raise AssertionError("DEL-04 lifecycle/hook source facts changed")
            if not any("not policy execution" in limit for limit in delivery["limits"]):
                raise AssertionError("DEL-04 source no longer limits policy execution claims")
            lifecycle_readme = " ".join((ROOT.parent / "oci-identity-lifecycle" / "README.md").read_text().split())
            if "does not include rendered-manifest bytes or output OCI/bundle bytes" not in lifecycle_readme:
                raise AssertionError("DEL-04 documented raw bundle-byte limit changed")
            if "Do not claim raw runtime identity, current health, artifact integrity by recomputation, hook execution" not in lifecycle_readme:
                raise AssertionError("DEL-04 documentation no longer limits hook execution claims")
            if any("imageID" in json.dumps(leg["workload"]) for leg in layers.values()):
                raise AssertionError("DEL-04 fixture unexpectedly supplies raw runtime image identity")
            if any("current" in leg["workload"] for leg in layers.values()):
                raise AssertionError("DEL-04 fixture unexpectedly supplies current-state evidence")
            if not lifecycle["coverage"]["state"]:
                raise AssertionError("DEL-04 lifecycle coverage source is missing")
            return expected

        raise AssertionError(f"unexpected delivery case: {case_id}")

    @classmethod
    def _python_matches(cls, definition: dict, answers: list[str]) -> list[bool]:
        py_flags = 0
        for flag, value in (("i", re.IGNORECASE), ("m", re.MULTILINE), ("s", re.DOTALL)):
            if flag in definition.get("flags", ""):
                py_flags |= value
        compiled = re.compile(definition["pattern"], py_flags)
        return [compiled.search(answer) is not None for answer in answers]

    @classmethod
    def _node_matches(cls, case_id: str, answers: list[str]) -> list[bool]:
        definition = cls.grader_defs[case_id]
        packet = {
            "pattern": definition["pattern"],
            "flags": definition.get("flags", ""),
            "target": definition["target"],
            "answers": answers,
        }
        encoded = json.dumps(packet, ensure_ascii=False).encode("utf-8")
        if len(encoded) > MAX_NODE_INPUT:
            raise AssertionError(f"{case_id} Node regex input exceeds the local bound")
        try:
            proc = subprocess.run(
                [cls.node, "-e", NODE_SCRIPT],
                input=encoded,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                timeout=NODE_TIMEOUT_SECONDS,
                check=False,
                env={"PATH": os.environ.get("PATH", "")},
            )
        except subprocess.TimeoutExpired as error:
            raise AssertionError(f"{case_id} Node regex check exceeded {NODE_TIMEOUT_SECONDS}s") from error
        if proc.returncode != 0:
            raise AssertionError(f"{case_id} Node regex check failed: {proc.stderr[:1000]!r}")
        if len(proc.stdout) > MAX_NODE_OUTPUT:
            raise AssertionError(f"{case_id} Node regex output exceeded the local bound")
        try:
            result = json.loads(proc.stdout)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise AssertionError(f"{case_id} Node regex output is not JSON booleans") from error
        if not isinstance(result, list) or len(result) != len(answers) or any(type(item) is not bool for item in result):
            raise AssertionError(f"{case_id} Node regex returned a malformed result")
        return result

    @classmethod
    def _assert_matches(cls, test: unittest.TestCase, case_id: str,
                        answers: list[str], expected: list[bool]) -> None:
        python_result = cls._python_matches(cls.grader_defs[case_id], answers)
        node_result = cls._node_matches(case_id, answers)
        test.assertEqual(python_result, expected, f"Python result for {case_id}")
        test.assertEqual(node_result, expected, f"Node.js result for {case_id}")
        test.assertEqual(node_result, python_result, f"regex dialect parity for {case_id}")

    def test_selected_graders_are_validated_weight_one_last_message_regexes(self):
        self.assertEqual(set(self.cases) >= set(DELIVERY_CASES), True)
        for case_id in DELIVERY_CASES:
            with self.subTest(case=case_id):
                selection = self.cases[case_id]["selectedAnswerGraders"]
                self.assertEqual(len(selection), 1)
                self.assertEqual(selection[0]["type"], "regex")
                self.assertEqual(selection[0]["weight"], 1)
                self.assertEqual(self.grader_defs[case_id]["target"], "last_message")
                self.assertRegex(self.grader_defs[case_id]["_raw_sha256"], r"^[0-9a-f]{64}$")

    def test_canonical_vectors_are_bound_to_prepared_source_facts(self):
        answers = {case_id: self._answer(case_id) for case_id in DELIVERY_CASES}
        self.assertEqual(answers["DEL-01"]["source_revision"], "main@sha1:2fe4c48d58f86d6fd44d2625444a2daf61d676d7")
        self.assertEqual(answers["DEL-01"]["ready_apps_digest"],
                         "sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac48")
        self.assertEqual(answers["DEL-02"]["consumed_before_refresh"], "NO")
        self.assertEqual(answers["DEL-02"]["exact_digests_and_counts"], "UNKNOWN")
        self.assertEqual(answers["DEL-03"]["receipt_profile_matches_approved_revision"], True)
        self.assertEqual(answers["DEL-03"]["runtime_sveltos_digest"], "UNKNOWN")
        self.assertEqual(answers["DEL-04"]["consumer_digests_match"], "YES")
        self.assertEqual(answers["DEL-04"]["hook_execution"], "UNKNOWN")
        for case_id, answer in answers.items():
            with self.subTest(case=case_id):
                definition = self.grader_defs[case_id]
                self._assert_matches(self, case_id, [_json(answer)], [True])

    def test_canonical_and_declared_whitespace_answers_match(self):
        for case_id in DELIVERY_CASES:
            with self.subTest(case=case_id):
                answer = self._answer(case_id)
                canonical = _json(answer)
                whitespace = " \n\t" + json.dumps(answer, indent=2, ensure_ascii=False) + " \n"
                self._assert_matches(self, case_id, [canonical, whitespace], [True, True])

    def test_every_answer_field_and_json_shape_has_negative_controls(self):
        for case_id in DELIVERY_CASES:
            answer = self._answer(case_id)
            canonical = _json(answer)
            candidates: list[tuple[str, str]] = []
            for key, value in answer.items():
                changed = copy.deepcopy(answer)
                changed[key] = (not value) if type(value) is bool else "NOT_THE_FROZEN_VALUE"
                candidates.append((f"wrong field {key}", _json(changed)))

                missing = copy.deepcopy(answer)
                del missing[key]
                candidates.append((f"missing field {key}", _json(missing)))

                duplicate = canonical[:-1] + "," + json.dumps(key) + ":" + json.dumps(value) + "}"
                candidates.append((f"duplicate field {key}", duplicate))

                wrong_type = copy.deepcopy(answer)
                wrong_type[key] = ("true" if value is True else "false") if type(value) is bool else None
                candidates.append((f"wrong type {key}", _json(wrong_type)))

            extra = copy.deepcopy(answer)
            extra["unrequested_extra"] = "value"
            candidates.extend([
                ("extra field", _json(extra)),
                ("partial JSON", canonical[:-1]),
                ("leading prose", "Answer: " + canonical),
                ("trailing prose", canonical + " unsupported claim"),
            ])
            with self.subTest(case=case_id):
                results = self._python_matches(self.grader_defs[case_id], [candidate for _, candidate in candidates])
                self.assertEqual(results, [False] * len(candidates),
                                 msg="; ".join(label for (label, _), passed in zip(candidates, results) if passed))
                node_results = self._node_matches(case_id, [candidate for _, candidate in candidates])
                self.assertEqual(node_results, [False] * len(candidates),
                                 msg=f"Node accepted invalid {case_id} answer")
                self.assertEqual(node_results, results, f"regex dialect parity for {case_id}")

    def test_delivery_identity_counterclaims_are_rejected(self):
        answers = {case_id: self._answer(case_id) for case_id in DELIVERY_CASES}
        source_digest = answers["DEL-01"]["ready_apps_digest"]
        wrong_digest = copy.deepcopy(answers["DEL-01"])
        wrong_digest["ready_apps_digest"] = "sha256:" + "0" * 64

        publication_is_consumption = copy.deepcopy(answers["DEL-02"])
        publication_is_consumption["consumed_before_refresh"] = "YES"

        inferred_is_exact = copy.deepcopy(answers["DEL-03"])
        inferred_is_exact["status_revision_basis"] = "reported_exactly"
        inferred_is_exact["runtime_sveltos_digest"] = "sha256:" + "a" * 64
        inferred_is_exact["cross_artifact_join"] = "ESTABLISHED"

        mutable_latest_is_current = copy.deepcopy(answers["DEL-04"])
        mutable_latest_is_current["package_oci_reference"] = (
            mutable_latest_is_current["package_oci_reference"].split("@", 1)[0]
        )
        hooks_are_proven = copy.deepcopy(answers["DEL-04"])
        hooks_are_proven["hook_execution"] = "EXECUTED"
        policy_is_proven = copy.deepcopy(answers["DEL-04"])
        policy_is_proven["policy_execution"] = "PASS"

        controls = {
            "DEL-01": [_json(wrong_digest)],
            "DEL-02": [_json(publication_is_consumption)],
            "DEL-03": [_json(inferred_is_exact)],
            "DEL-04": [_json(mutable_latest_is_current), _json(hooks_are_proven), _json(policy_is_proven)],
        }
        self.assertNotEqual(source_digest, wrong_digest["ready_apps_digest"])
        for case_id, candidates in controls.items():
            with self.subTest(case=case_id):
                self._assert_matches(self, case_id, candidates, [False] * len(candidates))

    def test_authored_control_vectors_are_synthetic_not_run_and_not_base_answers(self):
        controls = self.report["authoredInputControls"]
        expected = {
            "DEL-03": {
                "status_target_binding": "UNESTABLISHED",
                "receipt_variant_binding": "UNESTABLISHED",
                "authored_interval_freshness": "STALE",
                "runtime_sveltos_digest": "UNKNOWN",
                "cross_artifact_join": "UNESTABLISHED",
            },
            "DEL-04": {
                "delivery_digest_consistency": "CONFLICT_UNRESOLVED",
                "current_cluster_state": "UNKNOWN",
                "runtime_image_id": "UNKNOWN",
                "independent_bundle_verification": "UNKNOWN",
                "mutable_tag_current_identity": "UNKNOWN",
            },
        }
        for case_id, vector in expected.items():
            control_root = self.out / "authored-input-controls" / case_id
            with self.subTest(case=case_id):
                self.assertEqual(controls[case_id]["status"], "authored-control-not-run")
                self.assertIn("input-control", controls[case_id]["id"])
                acceptance_path = control_root / "oracle/acceptance.json"
                self.assertEqual(json.loads(acceptance_path.read_text()), vector)
                if case_id == "DEL-03":
                    status = (control_root / "input/status-excerpt.md").read_text()
                    marker = json.loads((control_root / "input/stale-control-marker.json").read_text())
                    receipt = prepare._yaml_load((control_root / "input/delivery-receipt.yaml").read_text())
                    self.assertIn("UNKNOWN UNKNOWN", status)
                    self.assertEqual(marker["authoredInterval"]["result"], "STALE")
                    self.assertGreater(marker["authoredInterval"]["elapsedSeconds"],
                                       marker["authoredInterval"]["maxAgeSeconds"])
                    self.assertTrue(all(variant["identity"] == "WITHHELD_BY_CONTROL"
                                        for variant in receipt["spec"]["variants"]))
                else:
                    control = json.loads((control_root / "input/divergence-control.json").read_text())
                    original = prepare._yaml_load((control_root / "input/source-chain-original.yaml").read_text())
                    conflict = prepare._yaml_load((control_root / "input/source-chain-authored-conflict.yaml").read_text())
                    original_delivery = original["spec"]["boundaries"]["delivery"]
                    conflict_delivery = conflict["spec"]["boundaries"]["delivery"]
                    self.assertEqual(control["evidenceKind"], "authored-conflicting-receipt-not-a-capture")
                    self.assertEqual(original_delivery["reference"], conflict_delivery["reference"])
                    self.assertTrue(original_delivery["reference"].endswith(":latest"))
                    self.assertNotEqual(original_delivery["digest"], conflict_delivery["digest"])
                    self.assertEqual(control["authoredConflictingDeliveryDigest"], conflict_delivery["digest"])
                self._assert_matches(self, case_id, [_json(vector)], [False])


if __name__ == "__main__":
    unittest.main()
