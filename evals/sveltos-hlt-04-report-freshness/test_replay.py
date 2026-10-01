import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("hlt04_replay", Path(__file__).with_name("replay.py"))
replay = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(replay)


class ReplayHelperTests(unittest.TestCase):
    def test_source_pin_and_expected_cases_are_frozen(self):
        self.assertEqual(replay.SOURCE_COMMIT, "8187910f9fe226e109e55c4d9c7c0e21297ff424")
        self.assertEqual(len(replay.SOURCE_HASHES), 2)
        self.assertEqual(set(replay.EXPECTED_CASES), {
            "missing-held-report-time", "young-held-report", "old-held-report-renewal",
            "malformed-held-report-time", "future-held-report-time", "clock-skew-control",
            "pre-apply-transition-control",
        })
        self.assertFalse(replay.EXPECTED_CASES["clock-skew-control"]["digest_proven"])
        self.assertFalse(replay.EXPECTED_CASES["pre-apply-transition-control"]["lastTransitionTime_is_lastCheckTime"])

    def test_execution_command_is_one_named_test_and_network_denied(self):
        argv = replay.replay_command("/tools/go", "/usr/bin/sandbox-exec", Path("/private/source"))
        self.assertIn("-run", argv)
        self.assertIn("^TestHLT04OfflineReplay$", argv)
        self.assertIn("(deny network*)", " ".join(argv))
        env = replay.minimal_go_environment("/tools", "/private/tmp", "/private/cache", "/existing/modcache")
        self.assertEqual(env["GOTOOLCHAIN"], "local")
        self.assertEqual(env["GOPROXY"], "off")
        self.assertEqual(env["GOSUMDB"], "off")
        self.assertNotIn("HOME", env)

    def test_result_parser_requires_two_valid_structured_summaries(self):
        valid = b"HLT04_RESULT {\"schema\":\"sveltos-hlt04-offline-replay.v1\"}\n"
        prefixed = b"    hlt04_replay_test.go:42: " + valid
        self.assertEqual(len(replay.parse_result_summaries(prefixed + valid)), 2)
        for output in (b"", valid, valid + valid + valid,
                       valid + b"HLT04_RESULT {bad}\n"):
            with self.subTest(output=output), self.assertRaises(replay.ReplayError):
                replay.parse_result_summaries(output)

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
        self.assertIn("synthetic_check_evidence_consumed_by_reporter", harness)
        self.assertNotIn('"lastCheckTime"', harness)


if __name__ == "__main__":
    unittest.main()
