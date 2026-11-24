#!/usr/bin/env python3
"""Unit tests for evals/scripts/measure_bytes_tokens.py."""

import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import measure_bytes_tokens as mbt


class TestMeasureBytesTokens(unittest.TestCase):
    def test_all_24_cases_measured_deterministically(self):
        repo_root = pathlib.Path(__file__).resolve().parents[2]
        report = mbt.measure_all(repo_root)

        self.assertEqual(report["schema"], "cub-scout-bytes-tokens-benchmark.v1")
        self.assertEqual(report["method"]["model_calls"], 0)
        self.assertEqual(report["method"]["credits_spent"], 0)
        self.assertEqual(report["totals"]["cases"], 24)
        self.assertEqual(len(report["cases"]), 24)
        self.assertEqual(len(report["groups"]), 6)

        for row in report["cases"]:
            self.assertGreater(row["raw_bytes"], 0, row["id"])
            self.assertGreater(row["files"], 0, row["id"])
            self.assertEqual(row["raw_tokens"], (row["raw_bytes"] + 3) // 4, row["id"])
            self.assertIn(row["tool"], mbt.ANSWER_BLOCK_CEILINGS, row["id"])
            self.assertGreater(row["answer_ceiling_bytes"], 0, row["id"])

        md = mbt.render_markdown(report)
        self.assertIn("Total (24 cases)", md)
        self.assertIn("INV-01", md)
        self.assertIn("RUL-04", md)


if __name__ == "__main__":
    unittest.main()
