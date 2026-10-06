#!/usr/bin/env python3
"""Exercise verification control flow in Bash; these are not live receipts."""
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOURCE = (ROOT / "test/prove-it-works.sh").read_text()
HELPERS = SOURCE.split("# Helper functions\n", 1)[1].split("# Start\n", 1)[0]


class VerificationControlFlow(unittest.TestCase):
    def run_bash(self, body, verbose=0):
        return subprocess.run(
            ["bash", "-c", "set -eo pipefail\nPASSED=0; FAILED=0; SKIPPED=0\n"
             + f"VERBOSE={verbose}; LEVEL=full\n" + HELPERS + body],
            capture_output=True, text=True,
        )

    def test_multiple_successful_checks_reach_completion(self):
        for verbose in (0, 1):
            with self.subTest(verbose=verbose):
                result = self.run_bash(
                    'run_test first true\nrun_test second true\n'
                    'test "$PASSED" = 2\necho COMPLETED\n', verbose)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn("COMPLETED", result.stdout)

    def test_failure_stops_required_run(self):
        for verbose in (0, 1):
            with self.subTest(verbose=verbose):
                result = self.run_bash('run_test failure false\necho FALSE_PASS\n', verbose)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("FALSE_PASS", result.stdout)

    def test_failed_pipeline_input_cannot_be_hidden_by_consumer(self):
        result = self.run_bash('run_test pipeline "false | true"\necho FALSE_PASS\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("FALSE_PASS", result.stdout)

    def test_missing_prerequisite_cannot_pass(self):
        result = self.run_bash('skip_test Connected "authentication expired"\necho FALSE_PASS\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("authentication expired", result.stdout)
        self.assertNotIn("FALSE_PASS", result.stdout)

    def test_optional_provider_is_required_for_full_acceptance(self):
        result = self.run_bash(
            'optional_test_unavailable Scan "provider missing"\necho FALSE_PASS\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("FALSE_PASS", result.stdout)

    def test_optional_provider_is_explicit_in_integration(self):
        result = self.run_bash(
            'LEVEL=integration\noptional_test_unavailable Scan "provider missing"\n'
            'test "$SKIPPED" = 1\necho COMPLETED\n')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("optional check unavailable", result.stdout)
        self.assertIn("COMPLETED", result.stdout)

    def test_expired_context_is_not_authentication(self):
        result = self.run_bash(
            'cub() { if [[ "$1 $2" == "context get" ]]; then return 0; fi; return 1; }\n'
            'check_confighub\necho FALSE_PASS\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("FALSE_PASS", result.stdout)

    def test_unknown_level_is_rejected_before_build(self):
        result = subprocess.run(["bash", str(ROOT / "test/prove-it-works.sh"),
                                 "--level=unknown"], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Unknown level", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
