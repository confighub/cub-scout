#!/usr/bin/env python3
"""Verify retained live Helm evidence; this does not rerun a cluster."""
import hashlib
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / "examples/helm-expt/evidence/2026-10-05-lifecycle"


def load(path):
    return json.loads((ROOT / path).read_bytes())


class GenuineHelmLifecycleCapture(unittest.TestCase):
    def test_retained_raw_hashes_and_cleanup(self):
        manifest = load("capture-manifest.json")
        self.assertEqual(manifest["cleanupExit"], 0)
        self.assertFalse(manifest["sourceDirtyAtStart"])
        self.assertEqual((ROOT / "exit-status.txt").read_text().strip(), "0")
        self.assertEqual(manifest["sourceCommit"], load("summary.json")["sourceCommit"])
        self.assertEqual(len(manifest["publicArtifacts"]), 89)
        for item in manifest["publicArtifacts"]:
            with self.subTest(file=item["file"]):
                data = (ROOT / item["file"]).read_bytes()
                self.assertEqual(len(data), item["bytes"])
                self.assertEqual(hashlib.sha256(data).hexdigest(), item["sha256"])

    def test_pins_and_completed_lifecycle(self):
        report = load("summary.json")
        self.assertTrue(report["lifecycleMatrixCompleted"])
        self.assertEqual(report["helm3Version"], "v3.22.0+g144ca65")
        self.assertEqual(report["helm4Version"], "v4.1.4+g05fa379")
        self.assertEqual(report["kindVersion"], "v0.31.0")
        self.assertIn("application-functionality", report["notCovered"])

    def test_actual_success_and_failure_are_different_jobs(self):
        for version in ["helm3", "helm4"]:
            with self.subTest(version=version):
                success = load(f"lifecycle/{version}/hook-success.json")
                failed = load(f"lifecycle/{version}/hook-failed.json")
                self.assertEqual(success["metadata"]["namespace"], f"matrix-{version}-lifecycle")
                self.assertEqual(success["status"]["succeeded"], 1)
                self.assertNotEqual(success["metadata"]["uid"], failed["metadata"]["uid"])
                self.assertTrue(any(c["type"] == "Failed" and c["status"] == "True" for c in failed["status"]["conditions"]))
                self.assertNotEqual((ROOT / f"lifecycle/{version}/failed-upgrade.exit").read_text().strip(), "0")
                self.assertEqual(load(f"lifecycle/{version}/history-after-failure.json")[-1]["status"], "failed")

    def test_rollback_restores_configuration_and_same_deployment(self):
        for version in ["helm3", "helm4"]:
            with self.subTest(version=version):
                before = load(f"releases/{version}-lifecycle-installed/deployment.raw.json")
                after = load(f"releases/{version}-lifecycle-rolled-back/deployment.raw.json")
                self.assertEqual(before["metadata"]["uid"], after["metadata"]["uid"])
                self.assertEqual(after["spec"]["replicas"], 1)
                self.assertEqual(after["status"]["readyReplicas"], 1)
                self.assertEqual(load(f"lifecycle/{version}/dependent-after-upgrade.json")["spec"]["marker"], "second")
                self.assertEqual(load(f"lifecycle/{version}/dependent-restored.json")["spec"]["marker"], "first")
                self.assertEqual(load(f"releases/{version}-lifecycle-rolled-back/release-history.json")[-1]["status"], "deployed")

    def test_crd_identity_stays_separate_and_deleted_hook_is_absent(self):
        for version in ["helm3", "helm4"]:
            with self.subTest(version=version):
                before = load(f"lifecycle/{version}/crd-initial.json")
                after = load(f"lifecycle/{version}/crd-after-upgrade.json")
                self.assertEqual(before["metadata"]["uid"], after["metadata"]["uid"])
                self.assertEqual(after["metadata"]["annotations"]["matrix.scout.test/crd-version"], "first")
                self.assertEqual(load(f"lifecycle/{version}/hooks-after-delete.json")["items"], [])

    def test_real_apply_conflict_preserves_field_then_explicit_recovery(self):
        prefix = "lifecycle/apply-transition/"
        self.assertNotEqual((ROOT / (prefix + "conflict.exit")).read_text().strip(), "0")
        stderr = (ROOT / (prefix + "conflict.stderr")).read_text()
        self.assertIn("matrix-foreign", stderr)
        self.assertIn(".spec.replicas", stderr)
        rejected = load(prefix + "after-conflict.json")
        accepted = load("releases/explicit-server-forced/deployment.raw.json")
        self.assertEqual(rejected["spec"]["replicas"], 5)
        self.assertEqual(accepted["spec"]["replicas"], 2)
        self.assertEqual(accepted["status"]["readyReplicas"], 2)
        self.assertEqual(rejected["metadata"]["uid"], accepted["metadata"]["uid"])

    def test_all_captured_standalone_plugin_trace_projections_agree(self):
        labels = ["fresh-helm3", "fresh-helm4", "pre-upgrade-helm3", "upgraded-helm4-auto",
                  "helm3-lifecycle-installed", "helm3-lifecycle-rolled-back",
                  "helm4-lifecycle-installed", "helm4-lifecycle-rolled-back", "explicit-server-forced"]
        for label in labels:
            with self.subTest(label=label):
                standalone = load(label + "-standalone-trace.json.projection.json")
                plugin = load(label + "-plugin-trace.json.projection.json")
                self.assertEqual(standalone, plugin)
                self.assertEqual(standalone["owner"], "Helm")


if __name__ == "__main__":
    unittest.main()
