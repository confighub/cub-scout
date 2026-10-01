import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
import sys
import os

SPEC = importlib.util.spec_from_file_location("hlt04_replay", Path(__file__).with_name("replay.py"))
replay = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(replay)


class ReplayHelperTests(unittest.TestCase):
    def test_source_pin_and_expected_cases_are_frozen(self):
        self.assertEqual(replay.SOURCE_COMMIT, "8187910f9fe226e109e55c4d9c7c0e21297ff424")
        self.assertIn("internal/onboard/charts.go", replay.SOURCE_HASHES)
        self.assertGreater(len(replay.SOURCE_HASHES), 20)
        self.assertEqual(set(replay.EXPECTED_CASES), {
            "missing-held-report-time", "missing-observed-at-field", "young-held-report", "old-held-report-renewal",
            "malformed-held-report-time", "future-held-report-time", "clock-skew-control",
            "pre-apply-transition-control",
        })
        self.assertFalse(replay.EXPECTED_CASES["clock-skew-control"]["digest_proven"])
        self.assertFalse(replay.EXPECTED_CASES["pre-apply-transition-control"]["lastTransitionTime_is_lastCheckTime"])

    def test_execution_command_is_one_named_test_and_network_denied(self):
        argv = replay.replay_command("/tools/go", "/usr/bin/sandbox-exec", Path("/private/source"))
        self.assertIn("-run", argv)
        self.assertIn("^TestHLT04OfflineReplay$", argv)
        compile_argv = replay.compile_command("/tools/go", "/usr/bin/sandbox-exec", Path("/private/source"))
        self.assertIn("-c", compile_argv)
        self.assertNotIn("-run", compile_argv)
        self.assertIn("(deny network*)", " ".join(argv))
        env = replay.minimal_go_environment("/tools", "/private/tmp", "/private/cache", "/existing/modcache")
        self.assertEqual(env["GOTOOLCHAIN"], "local")
        self.assertEqual(env["GOPROXY"], "off")
        self.assertEqual(env["GOSUMDB"], "off")
        self.assertNotIn("HOME", env)

    def _valid_result_pair(self):
        names = ["missing-held-report-time", "missing-observed-at-field", "young-held-report",
                 "old-held-report-renewal", "malformed-held-report-time", "future-held-report-time",
                 "clock-skew-control", "pre-apply-transition-control"]
        cases = []
        for name in names:
            wrote = name not in ("young-held-report", "future-held-report-time")
            import json
            computed = {"source": "synthetic", "observedAt": "2026-10-01T12:10:00Z",
                        "healthStatus": "Degraded", "syncStatus": "Synced"}
            held_before = json.dumps({key: value for key, value in computed.items() if key != "observedAt"})
            held_after = json.dumps(computed) if wrote else held_before
            case = {"name": name, "wrote": wrote, "evidence": {
                "raw_inputs": {"clusterprofiles": {"items": []}, "clustersummaries": {"items": []},
                               "clusterhealthchecks": {"items": []}, "published_releases": []},
                "held_before": held_before, "held_after": held_after,
                "computed_report": computed, "synthetic_now": computed["observedAt"],
                "write_patch": json.dumps({"Annotations": {"confighub.com/live-status": held_after}}) if wrote else None,
            }}
            if name in ("young-held-report", "future-held-report-time"):
                case["why"] = "unchanged"
            if name == "old-held-report-renewal":
                case["source_input_hashes_unchanged"] = True
                case["synthetic_check_evidence_consumed_by_reporter"] = False
            cases.append(case)
        a = {"schema": "sveltos-hlt04-offline-replay.v1", "source_kind": "synthetic-source-contract-replay", "cases": cases[:6]}
        b = {"schema": "sveltos-hlt04-offline-replay.v1", "source_kind": "synthetic-source-contract-replay", "cases": cases[6:],
             "clock_skew_control": {"release_digest_proven": False},
             "pre_apply_transition_control": {"last_transition_time_is_check_execution_time": False}}
        return ("HLT04_RESULT " + __import__("json").dumps(a) + "\nHLT04_RESULT " +
                __import__("json").dumps(b) + "\n").encode(), [a, b]

    def test_result_parser_checks_exact_named_cases_and_evidence(self):
        valid, parsed = self._valid_result_pair()
        self.assertEqual(replay.parse_result_summaries(valid), parsed)
        import json
        duplicate_key = valid.replace(b'"schema": "sveltos-hlt04-offline-replay.v1"',
                                      b'"schema": "sveltos-hlt04-offline-replay.v1", "schema": "other"', 1)
        with self.assertRaises(replay.ReplayError):
            replay.parse_result_summaries(duplicate_key)
        for mutation in ("drop", "duplicate", "missing_evidence", "wrong_write_type", "bad_patch", "changed_skip", "conflicting_patch", "wrong_source", "duplicate_evidence", "incomplete_inputs", "nonfinite"):
            rows = json.loads(valid.splitlines()[0].split(b" ", 1)[1]), json.loads(valid.splitlines()[1].split(b" ", 1)[1])
            cases = rows[0]["cases"]
            if mutation == "drop":
                cases.pop()
            elif mutation == "duplicate":
                cases.append(dict(cases[0]))
            elif mutation == "missing_evidence":
                del cases[0]["evidence"]
            elif mutation == "wrong_write_type":
                cases[0]["wrote"] = "true"
            elif mutation == "bad_patch":
                cases[0]["evidence"]["write_patch"] = None
            elif mutation == "changed_skip":
                cases[2]["evidence"]["held_after"] = "{}"
            elif mutation == "conflicting_patch":
                cases[0]["evidence"]["write_patch"] = "{}"
            elif mutation == "wrong_source":
                cases[1]["evidence"]["held_before"] = '{"source":"other"}'
            elif mutation == "duplicate_evidence":
                cases[0]["held_after"] = "contradictory"
            elif mutation == "incomplete_inputs":
                cases[0]["evidence"]["raw_inputs"] = {}
            elif mutation == "nonfinite":
                cases[0]["evidence"]["raw_inputs"]["published_releases"] = [float("nan")]
            bad = ("HLT04_RESULT " + json.dumps(rows[0]) + "\nHLT04_RESULT " + json.dumps(rows[1]) + "\n").encode()
            with self.subTest(mutation=mutation), self.assertRaises(replay.ReplayError):
                replay.parse_result_summaries(bad)

    def test_run_owned_bounds_timeout_output_and_confirms_cleanup(self):
        env = {"PATH": os.environ["PATH"]}
        with tempfile.TemporaryDirectory() as tmp:
            cwd = Path(tmp)
            code = "import sys;sys.stdout.write('ok')"
            rc, out, err, status, cleanup = replay.run_owned([sys.executable, "-c", code], env, cwd, 3)
            self.assertEqual((rc, out, status), (0, b"ok", "completed"))
            self.assertTrue(cleanup["cleanup_confirmed"])
            rc, out, err, status, cleanup = replay.run_owned(
                [sys.executable, "-c", "import time;time.sleep(10)"], env, cwd, 0.1)
            self.assertEqual(status, "timeout")
            self.assertTrue(cleanup["parent_reaped"])
            self.assertTrue(cleanup["group_gone_confirmed"])
            old_limit = replay.MAX_OUTPUT_BYTES
            replay.MAX_OUTPUT_BYTES = 32
            try:
                rc, out, err, status, cleanup = replay.run_owned(
                    [sys.executable, "-c", "import sys;sys.stdout.write('x'*10000)"], env, cwd, 3)
                self.assertEqual(status, "output_limit")
                self.assertLessEqual(len(out) + len(err), 32)
                self.assertTrue(cleanup["cleanup_confirmed"])
            finally:
                replay.MAX_OUTPUT_BYTES = old_limit

    def test_archive_path_and_file_types_fail_closed(self):
        def tar_for(name, kind="file"):
            stream = io.BytesIO()
            with tarfile.open(fileobj=stream, mode="w") as archive:
                entry = tarfile.TarInfo(name)
                entry.type = tarfile.REGTYPE if kind == "file" else tarfile.SYMTYPE
                entry.size = 1 if kind == "file" else 0
                archive.addfile(entry, io.BytesIO(b"x") if kind == "file" else None)
            stream.seek(0)
            return tarfile.open(fileobj=stream, mode="r:")

        with tar_for("internal/onboard/status.go") as good:
            self.assertEqual(len(replay.safe_archive_members(good)), 1)
        for name, kind in (("../escape", "file"), ("/absolute", "file"),
                           ("inside/link", "symlink")):
            with tar_for(name, kind) as bad, self.subTest(name=name), self.assertRaises(replay.ReplayError):
                replay.safe_archive_members(bad)

    def test_private_output_refuses_existing_files_and_symlinks(self):
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            existing = parent / "existing"
            existing.write_text("keep")
            with self.assertRaises(replay.ReplayError):
                replay.create_output(existing)
            link = parent / "link"
            link.symlink_to(existing)
            with self.assertRaises(replay.ReplayError):
                replay.create_output(link)
            new = replay.create_output(parent / "fresh")
            self.assertEqual(new.stat().st_mode & 0o777, 0o700)
            self.assertEqual(existing.read_text(), "keep")

    def test_harness_separates_synthetic_check_execution_from_transition_field(self):
        harness = replay.REPLAY_TEST.read_text()
        self.assertIn("not producer input", harness)
        self.assertIn("lastTransitionTime", harness)
        self.assertIn('"projectsveltos.io/cluster-profile-name": "demo-prod"', harness)
        self.assertIn("missing-observed-at-field", harness)
        self.assertIn("synthetic_check_evidence_consumed_by_reporter", harness)
        self.assertNotIn('"lastCheckTime"', harness)


if __name__ == "__main__":
    unittest.main()
