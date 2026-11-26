#!/usr/bin/env python3
"""Unit tests for evals/scripts/measure_blast_radius.py."""

import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import measure_blast_radius as mbr


class TestMeasureBlastRadius(unittest.TestCase):
    def test_multi_controller_blast_radius_eval_is_deterministic_and_bounded(self):
        repo_root = pathlib.Path(__file__).resolve().parents[2]
        report = mbr.measure_all(repo_root)

        self.assertEqual(report["schema"], "cub-scout-blast-radius-eval.v1")
        self.assertEqual(report["method"]["model_calls"], 0)
        self.assertEqual(report["method"]["credits_spent"], 0)
        self.assertEqual(report["totals"]["cases"], 4)
        self.assertEqual(len(report["cases"]), 4)

        by_id = {row["id"]: row for row in report["cases"]}
        for row in report["cases"]:
            self.assertEqual(row["answer_state"], "explained", row["id"])
            self.assertGreater(row["raw_bytes"], 0, row["id"])
            self.assertGreater(row["answer_bytes"], 0, row["id"])
            self.assertLess(row["answer_bytes"], row["raw_bytes"], row["id"])
            self.assertEqual(row["raw_tokens"], (row["raw_bytes"] + 3) // 4, row["id"])
            self.assertEqual(row["answer_tokens"], (row["answer_bytes"] + 3) // 4, row["id"])

        # 4-hop Flux fixture stays within 1,200-byte compact JSON ceiling.
        flux = by_id["flux-4hop-configmap"]
        self.assertLessEqual(flux["answer_bytes"], 1200)
        self.assertEqual(flux["ancestors_count"], 2)
        self.assertEqual(flux["descendants_count"], 2)
        self.assertEqual(flux["affected_workloads_count"], 1)
        self.assertFalse(flux["truncated"])

        # Argo Secret fan-out finds 3 Deployments + 3 PDBs downstream.
        argo = by_id["argo-secret-fanout"]
        self.assertEqual(argo["ancestors_count"], 1)
        self.assertEqual(argo["descendants_count"], 6)
        self.assertEqual(argo["affected_workloads_count"], 3)
        self.assertFalse(argo["truncated"])

        # Large snapshot enforces 50-node truncation cap.
        large = by_id["large-snapshot-truncation"]
        self.assertTrue(large["truncated"])
        self.assertEqual(large["ancestors_count"] + large["descendants_count"], 50)

        md = mbr.render_markdown(report)
        self.assertIn("Total (4 cases)", md)
        self.assertIn("flux-4hop-configmap", md)
        self.assertIn("argo-secret-fanout", md)
        self.assertIn("large-snapshot-truncation", md)


if __name__ == "__main__":
    unittest.main()
