"""Pure source and mocked-lifecycle guards; these tests never invoke Docker."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("container_isolation_prepare", HERE / "prepare.py")
prepare = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(prepare)

CONTAINER_ID = "a" * 64
OWNER = "b" * 32


def inspect_object(*, state="created", owner=OWNER):
    fixture = prepare.FIXTURE.resolve()
    return {
        "Id": CONTAINER_ID,
        "Name": "/scout-isolation-0123456789abcdef",
        "Image": prepare.IMAGE_ID,
        "Config": {"Image": prepare.IMAGE_ID, "User": "65534:65534",
                    "Labels": {prepare.OWNER_LABEL: owner}},
        "HostConfig": {"ReadonlyRootfs": True, "NetworkMode": "none", "CapDrop": ["ALL"],
                       "SecurityOpt": ["no-new-privileges:true"], "PidsLimit": 32,
                       "Memory": 268435456, "NanoCpus": 500000000,
                       "Privileged": False, "CapAdd": [], "PidMode": "private", "IpcMode": "private",
                       "Tmpfs": {"/tmp": "rw,noexec,nosuid,size=16777216,uid=65534,gid=65534"}},
        "Mounts": [{"Type": "bind", "Source": str(fixture), "Destination": "/fixture/input.txt", "RW": False}],
        "State": {"Status": state, "ExitCode": 0 if state == "exited" else 0},
    }


def good_payload():
    return json.dumps({"schema": "container-isolation-payload.v1",
                       "assertions": {key: True for key in sorted(prepare.PAYLOAD_ASSERTIONS)},
                       "childProcessAttempted": True}, separators=(",", ":")).encode()


class IsolationSourceTests(unittest.TestCase):
    def test_reviewed_image_pin_and_explicit_context_are_strict(self):
        prepare.validate_image_pin(prepare.IMAGE_ID)
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_image_pin("python:3.11-slim")
        prepare.validate_context_name("local-dev")
        for context in ("", "remote/context", "../default", "x" * 64, "bad\ncontext"):
            with self.subTest(context=context), self.assertRaises(prepare.CaptureError):
                prepare.validate_context_name(context)
        prepare.validate_local_endpoint(b'"unix:///var/run/docker.sock"')
        for endpoint in (b'"tcp://remote:2376"', b'"ssh://host"', b'null', b'not json'):
            with self.subTest(endpoint=endpoint), self.assertRaises(prepare.CaptureError):
                prepare.validate_local_endpoint(endpoint)

    def test_launcher_validation_preserves_requested_argv0(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            target = root / "docker-tools"
            target.write_text("#!/bin/sh\nexit 0\n")
            target.chmod(0o700)
            launcher = root / "docker"
            launcher.symlink_to(target)
            selected = prepare.docker_launcher(launcher)
            self.assertEqual(str(selected), str(launcher.absolute()))
            self.assertNotEqual(selected.name, target.name)

    def test_environment_strips_context_overrides_and_secret_mount_hints(self):
        source = {"PATH": "/bin", "HOME": "/tmp/home", "DOCKER_CONFIG": "/tmp/docker",
                  "DOCKER_CONTEXT": "remote", "DOCKER_HOST": "tcp://remote", "DOCKER_TLS_VERIFY": "1",
                  "DOCKER_CERT_PATH": "/secret", "KUBECONFIG": "/secret/kubeconfig", "SSH_AUTH_SOCK": "/secret/sock"}
        self.assertEqual(prepare.docker_env(source), {"PATH": "/bin", "HOME": "/tmp/home", "DOCKER_CONFIG": "/tmp/docker"})

    def test_output_is_new_private_temporary_path_and_rejects_symlink(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            repo = root / "repo"
            repo.mkdir()
            safe = root / "capture"
            created = prepare._safe_output(safe, repo_root=repo)
            self.assertEqual(created.stat().st_mode & 0o777, 0o700)
            with self.assertRaises(prepare.CaptureError):
                prepare._safe_output(safe, repo_root=repo)
            link = root / "link"
            link.symlink_to(root / "elsewhere")
            with self.assertRaises(prepare.CaptureError):
                prepare._safe_output(link, repo_root=repo)
            with self.assertRaises(prepare.CaptureError):
                prepare._safe_output(repo / "inside", repo_root=repo)

    def test_create_command_has_only_expected_mount_and_explicit_bounds(self):
        with tempfile.TemporaryDirectory() as td:
            marker = Path(td) / "marker.txt"
            marker.write_text("host only")
            args = prepare.build_container_args("scout-isolation-0123456789abcdef", OWNER,
                                                prepare.FIXTURE, str(marker.resolve()))
        self.assertEqual(args[:2], ["container", "create"])
        self.assertIn("--pull=never", args)
        self.assertIn("--network=none", args)
        self.assertIn("--read-only", args)
        self.assertIn("65534:65534", args)
        self.assertIn("--cap-drop=ALL", args)
        self.assertIn("--security-opt=no-new-privileges", args)
        self.assertIn("--pids-limit=32", args)
        self.assertIn("--memory=256m", args)
        self.assertIn("--cpus=0.5", args)
        self.assertEqual(args.count("--mount"), 1)
        self.assertIn("dst=/fixture/input.txt,readonly", args[args.index("--mount") + 1])
        self.assertIn("--tmpfs", args)
        self.assertNotIn("--rm", args)
        self.assertNotIn("/var/run/docker.sock", " ".join(args))
        self.assertNotIn(".kube/config", " ".join(args))
        self.assertNotIn("--volume", args)
        self.assertEqual(args[args.index("python") + 1], "-c")
        with self.assertRaises(prepare.CaptureError):
            prepare.build_container_args("unsafe-name", OWNER, prepare.FIXTURE, "/scout-host-marker-" + OWNER)

    def test_inspect_checks_identity_mounts_and_configured_vs_demonstrated_bounds(self):
        obj = inspect_object()
        ident, selected = prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                                          owner=OWNER, fixture=prepare.FIXTURE)
        self.assertEqual(ident, CONTAINER_ID)
        self.assertEqual(selected["networkMode"], "none")
        self.assertEqual(selected["pidsLimit"], 32)
        obj["Config"]["Labels"][prepare.OWNER_LABEL] = "not-this-run"
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                           owner=OWNER, fixture=prepare.FIXTURE)
        obj = inspect_object()
        obj["Mounts"].append({"Type": "bind", "Source": "/host/home", "Destination": "/home", "RW": True})
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                           owner=OWNER, fixture=prepare.FIXTURE)
        obj = inspect_object()
        obj["Image"] = "sha256:" + "0" * 64
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                           owner=OWNER, fixture=prepare.FIXTURE)
        for field, value in (("Privileged", True), ("CapAdd", ["SYS_ADMIN"]),
                             ("PidMode", "host"), ("IpcMode", "host")):
            obj = inspect_object()
            obj["HostConfig"][field] = value
            with self.subTest(field=field), self.assertRaises(prepare.CaptureError):
                prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                               owner=OWNER, fixture=prepare.FIXTURE)
        for tmpfs in ("rw,noexec,nosuid,size=16777216,uid=65534,gid=65534,fake=noexec",
                      "rw,noexec,nosuid,size=16777216,uid=65534,gid=65534,uid=0"):
            obj = inspect_object()
            obj["HostConfig"]["Tmpfs"]["/tmp"] = tmpfs
            with self.subTest(tmpfs=tmpfs), self.assertRaises(prepare.CaptureError):
                prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                               owner=OWNER, fixture=prepare.FIXTURE)
        obj = inspect_object(state="exited")
        obj["State"]["ExitCode"] = False
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_owned_inspect(json.dumps(obj).encode(), name="scout-isolation-0123456789abcdef",
                                           owner=OWNER, fixture=prepare.FIXTURE, require_running_state=True)

    def test_payload_contract_requires_all_true_assertions_and_no_extra_fields(self):
        parsed = prepare.validate_payload(good_payload())
        self.assertEqual(set(parsed), prepare.PAYLOAD_ASSERTIONS)
        for value in (
            b'{"schema":"container-isolation-payload.v1","assertions":{},"childProcessAttempted":true}',
            good_payload().replace(b"true", b"false", 1),
            good_payload()[:-1] + b',"extra":true}',
            good_payload() + b"prose",
        ):
            with self.subTest(value=value[:80]), self.assertRaises(prepare.CaptureError):
                prepare.validate_payload(value)
        with self.assertRaises(prepare.CaptureError):
            prepare.validate_payload(good_payload(), b"unexpected stderr")
        for bad in (b'{"schema":"container-isolation-payload.v1","schema":"other"}',
                    b'{"value":1e999}', b'{"value":NaN}'):
            with self.subTest(bad=bad), self.assertRaises(prepare.CaptureError):
                prepare.validate_payload(bad)

    def test_payload_is_python_source_and_generated_marker_is_not_a_mount(self):
        with tempfile.TemporaryDirectory() as td:
            marker = Path(td) / "host-marker.txt"
            marker.write_text("host only")
            args = prepare.build_container_args("scout-isolation-0123456789abcdef", OWNER,
                                                prepare.FIXTURE, str(marker.resolve()))
        source = args[args.index("python") + 2]
        compile(source, "container-payload", "exec")
        self.assertIn(str(marker.resolve()), source)
        self.assertIn("synthetic_host_marker_unavailable", source)
        self.assertIn("203.0.113.1", source)
        self.assertEqual(args.count("--mount"), 1)

    def test_limits_and_cleanup_budget_are_explicit(self):
        self.assertEqual((prepare.EXECUTION_TIMEOUT, prepare.CLEANUP_TIMEOUT, prepare.TOTAL_TIMEOUT), (30, 15, 60))
        self.assertEqual(prepare.MAX_OUTPUT, 1024 * 1024)

    def test_mocked_capture_proves_configuration_and_owned_cleanup_only(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            repo = root / "repo"
            repo.mkdir()
            # Use the real source root for helper cleanliness; only tools are mocked.
            docker_file = root / "docker"
            docker_file.write_text("fake launcher")
            docker_file.chmod(0o700)
            output = root / "capture"
            removed = []
            commands = []
            state = "created"
            obj = inspect_object()
            expected = good_payload()
            created_name = ""
            def runner(argv, timeout, env=None, max_output=None):
                nonlocal state, created_name
                self.assertLessEqual(timeout, 30, (argv, timeout))
                self.assertLessEqual(max_output, 1024 * 1024, (argv, max_output))
                commands.append(argv)
                if Path(argv[0]).name == "git":
                    if "rev-parse" in argv:
                        return 0, b"a" * 40 + b"\n", b""
                    return 0, b"", b""
                self.assertEqual(max_output, 1024 * 1024, (argv, max_output))
                self.assertNotIn("KUBECONFIG", env, env)
                self.assertNotIn("DOCKER_HOST", env, env)
                self.assertEqual(argv[0], str(docker_file.absolute()))
                self.assertEqual(argv[1:3], ["--context", "local"])
                tail = argv[3:]
                if tail == ["--version"]:
                    return 0, b"Docker fake\n", b""
                if tail[:2] == ["context", "inspect"]:
                    return 0, b'"unix:///var/run/docker.sock"', b""
                if tail[:2] == ["image", "inspect"]:
                    return 0, (prepare.IMAGE_ID + "\n").encode(), b""
                if tail[:2] == ["container", "create"]:
                    self.assertIn("--pull=never", tail)
                    self.assertIn("--network=none", tail)
                    created_name = tail[tail.index("--name") + 1]
                    owner_value = tail[tail.index("--label") + 1].split("=", 1)[1]
                    obj["Name"] = "/" + created_name
                    obj["Config"]["Labels"][prepare.OWNER_LABEL] = owner_value
                    return 0, (CONTAINER_ID + "\n").encode(), b""
                if tail[:2] == ["container", "inspect"]:
                    if tail[-1] in (CONTAINER_ID, created_name) and state == "removed":
                        return 1, b"", b"Error: No such object: " + CONTAINER_ID.encode()
                    return 0, json.dumps(obj | {"State": {"Status": state, "ExitCode": 0}}).encode(), b""
                if tail[:2] == ["start", "--attach"]:
                    state = "exited"
                    return 0, expected, b""
                if tail[:3] == ["container", "rm", "--force"]:
                    removed.append(tail[-1])
                    state = "removed"
                    return 0, b"", b""
                raise AssertionError(argv)
            # source clean/revision commands use the real repo path, runner is mock-only.
            # Patch REPO only for git metadata; no git or Docker command is executed.
            with patch.object(prepare, "REPO", root):
                result = prepare.capture(docker_file, "local", prepare.IMAGE_ID, output, runner=runner)
            self.assertEqual(result, 0, (json.loads((output / "receipt.json").read_text()) if (output / "receipt.json").exists() else "no receipt"))
            self.assertEqual(removed, [CONTAINER_ID])
            receipt = json.loads((output / "receipt.json").read_text())
            self.assertEqual(receipt["status"], "passed")
            self.assertTrue(receipt["containerCleanup"]["verifiedAbsent"])
            self.assertTrue(all(receipt["demonstratedAssertions"].values()))
            self.assertTrue(any("--context" in command for command in commands))
            self.assertFalse((output / "inspect-owned-container.stdout.bin").exists())
            self.assertFalse((output / "inspect-container-result.stdout.bin").exists())
            self.assertFalse(receipt["commands"][2]["outputFilesRetained"])
            marker_path = Path(receipt["hostMarkerContainerPath"])
            self.assertTrue(marker_path.is_file())
            self.assertEqual(receipt["hostMarkerSha256"], receipt["hostMarkerAfterSha256"])
            self.assertEqual(receipt["hostMarkerUnavailableInContainer"], True)
            self.assertEqual(receipt["dockerLauncher"]["sha256"], prepare.sha256(docker_file.read_bytes()))
            self.assertEqual(receipt["inv04Runner"]["sha256"], prepare.sha256(prepare.INV04.read_bytes()))
            create = next(command for command in commands if command[3:5] == ["container", "create"])
            self.assertEqual(create.count("--mount"), 1)
            mount_value = create[create.index("--mount") + 1]
            self.assertNotIn(str(marker_path), mount_value)
            self.assertEqual(receipt["injectedPayloadSha256"], prepare.sha256(create[create.index("python") + 2].encode()))
            # A lost receipt is a failed capture even if the mocked payload passed.
            state = "created"
            obj["State"] = {"Status": "created", "ExitCode": 0}
            failed_receipt_output = root / "capture-no-receipt"
            original_write = prepare._write
            def fail_receipt(path, content):
                if path.name == "receipt.json":
                    raise OSError("synthetic receipt write failure")
                return original_write(path, content)
            with patch.object(prepare, "_write", side_effect=fail_receipt):
                failed_result = prepare.capture(docker_file, "local", prepare.IMAGE_ID,
                                                failed_receipt_output, runner=runner)
            self.assertEqual(failed_result, 1)
            self.assertFalse((failed_receipt_output / "receipt.json").exists())

    def test_uncertain_cleanup_is_failure_and_never_deletes_unowned_container(self):
        wrong = inspect_object(owner="someone-else")
        calls = []
        def runner(argv, timeout, env=None, max_output=None):
            calls.append(argv)
            return 0, json.dumps(wrong).encode(), b""
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local",
            "scout-isolation-0123456789abcdef", OWNER, prepare.FIXTURE, {}, time.monotonic() + 10,
            runner=runner)
        self.assertFalse(result["verifiedAbsent"])
        self.assertTrue(result["errors"])
        self.assertEqual(len(calls), 1)
        self.assertFalse(any("rm" in argv for argv in calls))

    def test_partial_create_cleanup_uses_owned_name_for_discovery_then_exact_id_for_remove(self):
        obj = inspect_object()
        state = "present"
        calls = []
        def runner(argv, timeout, env=None, max_output=None):
            nonlocal state
            calls.append(argv)
            if argv[-3:-1] == ["rm", "--force"]:
                self.assertEqual(argv[-1], CONTAINER_ID)
                state = "removed"
                return 0, b"", b""
            if state == "removed":
                return 1, b"", b"Error: No such object: " + argv[-1].encode()
            return 0, json.dumps(obj).encode(), b""
        name = "scout-isolation-0123456789abcdef"
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local", name,
            OWNER, prepare.FIXTURE, {}, time.monotonic() + 10, runner=runner)
        self.assertTrue(result["verifiedAbsent"])
        self.assertEqual(calls[0][-1], name)
        self.assertEqual(calls[1][-1], CONTAINER_ID)
        self.assertEqual(calls[2][-1], CONTAINER_ID)
        self.assertEqual([op["operation"] for op in result["operations"]],
                         ["inspect-before-remove", "remove-owned-container", "verify-container-absent"])

    def test_cleanup_timeout_is_uncertain_and_does_not_claim_success(self):
        def timed_out(argv, timeout, env=None, max_output=None):
            self.assertLessEqual(timeout, 5)
            raise subprocess.TimeoutExpired(argv, timeout)
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local",
            "scout-isolation-0123456789abcdef", OWNER, prepare.FIXTURE, {}, time.monotonic() + 10,
            runner=timed_out)
        self.assertFalse(result["verifiedAbsent"])
        self.assertIn("TimeoutExpired", result["errors"][0])
        self.assertFalse(prepare._is_exact_missing(1, b"", b"no such object", "scout-isolation-name"))

    def test_missing_container_error_must_be_exact_and_uncontradicted(self):
        target = "scout-isolation-review-missing-20261001"
        exact = b"\nError response from daemon: No such container: " + target.encode() + b"\n"
        self.assertTrue(prepare._is_exact_missing(1, b"", exact, target))
        self.assertTrue(prepare._is_exact_missing(1, b"\n", exact, target))
        for code, stdout, stderr in (
            (1, b"\n\n", exact), (1, b" ", exact),
            (0, b"", exact), (1, b"partial", exact), (1, b"", exact + b"\nother failure\n"),
            (1, b"", b"Error response from daemon: No such container: " + target.encode() + b"suffix"),
            (1, b"", b"Error response from daemon: No such container: " + target.encode() + b"-other"),
            (True, b"", exact), (1, b"", exact.replace(b"No such container", b"no such container")),
        ):
            with self.subTest(code=code, stderr=stderr):
                self.assertFalse(prepare._is_exact_missing(code, stdout, stderr, target))

    def test_owned_container_with_bad_config_is_still_removed(self):
        obj = inspect_object()
        obj["HostConfig"]["Privileged"] = True
        state = "present"
        calls = []
        def runner(argv, timeout, env=None, max_output=None):
            nonlocal state
            calls.append(argv)
            if argv[-4:-2] == ["container", "rm"]:
                state = "removed"
                return 0, b"", b""
            if state == "removed":
                target = argv[-1]
                return 1, b"", b"Error response from daemon: No such container: " + target.encode()
            return 0, json.dumps(obj).encode(), b""
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local",
            "scout-isolation-0123456789abcdef", OWNER, prepare.FIXTURE, {}, time.monotonic() + 10,
            runner=runner, known_id=CONTAINER_ID)
        self.assertTrue(result["verifiedAbsent"])
        self.assertEqual([command[-1] for command in calls if command[-4:-2] == ["container", "rm"]], [CONTAINER_ID])

    def test_cleanup_exact_absence_and_wrong_id_are_distinguished(self):
        name = "scout-isolation-0123456789abcdef"
        absent_calls = []
        def absent(argv, timeout, env=None, max_output=None):
            absent_calls.append(argv)
            return 1, b"", b"\nError response from daemon: No such container: " + name.encode() + b"\n"
        absent_result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local", name,
            OWNER, prepare.FIXTURE, {}, time.monotonic() + 10, runner=absent)
        self.assertTrue(absent_result["verifiedAbsent"])
        self.assertEqual(len(absent_calls), 1)

        wrong_id = inspect_object()
        wrong_id["Id"] = "c" * 64
        calls = []
        def inspect_wrong_id(argv, timeout, env=None, max_output=None):
            calls.append(argv)
            return 0, json.dumps(wrong_id).encode(), b""
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local", name,
            OWNER, prepare.FIXTURE, {}, time.monotonic() + 10, runner=inspect_wrong_id,
            known_id=CONTAINER_ID)
        self.assertFalse(result["verifiedAbsent"])
        self.assertTrue(result["errors"])
        self.assertEqual(len(calls), 1)
        self.assertFalse(any(command[-4:-2] == ["container", "rm"] for command in calls))

    def test_failed_remove_with_container_still_present_is_not_cleanup_success(self):
        obj = inspect_object()
        calls = []
        def runner(argv, timeout, env=None, max_output=None):
            calls.append(argv)
            if argv[-3:-1] == ["rm", "--force"]:
                return 1, b"", b"removal failed"
            return 0, json.dumps(obj).encode(), b""
        result = prepare.cleanup_owned_container(Path("/usr/local/bin/docker"), "local",
            "scout-isolation-0123456789abcdef", OWNER, prepare.FIXTURE, {}, time.monotonic() + 10,
            runner=runner, known_id=CONTAINER_ID)
        self.assertFalse(result["verifiedAbsent"])
        self.assertTrue(result["errors"])
        rm_args = next(args for args in calls if "rm" in args)
        self.assertEqual(rm_args[-1], CONTAINER_ID)

    def test_execute_flag_is_mandatory(self):
        with patch.object(prepare, "capture") as capture:
            with self.assertRaises(SystemExit) as failure:
                prepare.main(["--docker-binary", "/usr/local/bin/docker", "--docker-context", "local",
                              "--image-id", prepare.IMAGE_ID, "--output-dir", "/tmp/unused"])
        self.assertEqual(failure.exception.code, 2)
        capture.assert_not_called()


if __name__ == "__main__":
    unittest.main()
