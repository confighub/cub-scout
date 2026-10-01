from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("pre01_real_kubectl_prepare", HERE / "prepare.py")
assert spec and spec.loader
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)
payload_spec = importlib.util.spec_from_file_location("pre01_real_kubectl_payload", HERE / "payload.py")
assert payload_spec and payload_spec.loader
payload = importlib.util.module_from_spec(payload_spec)
payload_spec.loader.exec_module(payload)


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def encoded(data: bytes) -> str:
    return base64.b64encode(data).decode("ascii")


def positive_result(expected):
    phases = []
    for phase in ("absent", "present"):
        routes = expected[phase]
        commands, events = [], []
        for i, route in enumerate(routes):
            status = route["status"]
            stdout = route["body"] if status == 200 else b""
            stderr = b"kubectl received a recorded HTTP 404\n" if status == 404 else b""
            commands.append({
                "phase": phase, "index": i, "path": route["path"],
                "expectedHTTPStatus": status, "expectedSourceBodySha256": route["bodySha256"],
                "expectedSourceBodyBytes": len(route["body"]),
                "argv": ["/tools/kubectl", "--kubeconfig", f"/tmp/kubeconfig-{phase}",
                         "--context", "recorded", "get", "--raw", route["path"]],
                "exitCode": 1 if status == 404 else 0, "status": "passed", "error": None,
                "elapsedSeconds": 0.01, "outputTruncated": False,
                "stdoutBytes": len(stdout), "stderrBytes": len(stderr), "stdoutSha256": digest(stdout),
                "stderrSha256": digest(stderr), "stdoutBase64": encoded(stdout), "stderrBase64": encoded(stderr),
            })
            events.append({"requestNumber": i + 1, "method": "GET", "path": route["path"], "status": status,
                           "responseBodySha256": route["bodySha256"], "responseBodyBytes": len(route["body"]),
                           "captured": True, "reason": "captured-response", "sourceRows": route["sourceRows"]})
        trace = {"schema": "recorded-api-replay-trace.v1", "case": "PRE-01", "phase": phase,
                 "sourceRevision": "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0",
                 "reportSha256": prepare.SOURCE_PINS["evals/reports/2026-10-01-pre01-crd.json"][1],
                 "scopeSha256": prepare.SOURCE_PINS["evals/pre01-crd/fixtures/capture-scope.json"][1],
                 "status": "stopped", "coverageFailure": False,
                 "coverageComplete": False, "listenerCleanupConfirmed": True,
                 "requestsObserved": 3, "maxRequests": 12, "events": events}
        phases.append({"phase": phase, "status": "passed", "listenerCleanupConfirmed": True,
                       "startedAt": "2026-10-01T00:00:00Z", "endedAt": "2026-10-01T00:00:01Z",
                       "elapsedSeconds": 1.0,
                       "sourceRows": [{k: v for k, v in route.items() if k != "body"} for route in routes],
                       "transportReady": {"host": "127.0.0.1", "port": 41234,
                           "sourceRevision": "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0",
                           "reportSha256": prepare.SOURCE_PINS["evals/reports/2026-10-01-pre01-crd.json"][1],
                           "scopeSha256": prepare.SOURCE_PINS["evals/pre01-crd/fixtures/capture-scope.json"][1],
                           "atomicSnapshot": False},
                       "commands": commands, "replayTrace": trace})
    return {"schema": "pre01-real-kubectl-gate.v1", "status": "passed",
            "source": {"phaseOrder": ["absent", "present"], "atomicSnapshot": False,
                       "kubectlSha256": prepare.ASSET["sha256"],
                       "replaySha256": prepare.SOURCE_PINS["evals/recorded-api/replay.py"][1],
                       "reportSha256": prepare.SOURCE_PINS["evals/reports/2026-10-01-pre01-crd.json"][1],
                       "scopeSha256": prepare.SOURCE_PINS["evals/pre01-crd/fixtures/capture-scope.json"][1]},
            "limits": {"perCommandSeconds": 5, "perStreamBytes": 65536,
                       "phaseWallSeconds": 24, "totalExecutionSeconds": 60},
            "elapsedSeconds": 1.0, "phases": phases}


class SourceAndRouteTests(unittest.TestCase):
    def test_source_pins_and_phase_routes_are_exact(self):
        sources = prepare.source_inventory()
        self.assertEqual(set(sources), set(prepare.SOURCE_PINS))
        routes = prepare._expected_routes()
        self.assertEqual([r["path"] for r in routes["absent"]], payload.PHASE_PATHS["absent"])
        self.assertEqual([r["path"] for r in routes["present"]], payload.PHASE_PATHS["present"])
        self.assertEqual([r["status"] for r in routes["absent"]], [404, 404, 404])
        self.assertEqual([r["status"] for r in routes["present"]], [200, 200, 200])
        self.assertEqual([r["path"] for r in routes["absent"]], [r["path"] for r in routes["present"]])

    def test_source_hash_mismatch_fails_before_staging(self):
        rel = "evals/recorded-api/replay.py"
        original = prepare.SOURCE_PINS[rel]
        try:
            prepare.SOURCE_PINS[rel] = (original[0], "0" * 64)
            with self.assertRaises(prepare.GateError):
                prepare._read_source(prepare.REPO / rel, rel)
        finally:
            prepare.SOURCE_PINS[rel] = original

    def test_staging_rejects_symlink_and_unexpected_files_on_cleanup(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as td:
            parent = Path(td); stage = parent / "stage"; stage.mkdir()
            (stage / "kubectl").write_bytes(b"pinned")
            expected = {"kubectl"}
            (stage / "extra").write_bytes(b"unexpected")
            with self.assertRaises(prepare.GateError):
                prepare.remove_stage(stage, expected)
            (stage / "extra").unlink()
            link = stage / "link"; link.symlink_to(stage / "kubectl")
            with self.assertRaises(prepare.GateError):
                prepare._walk_stage(stage)
    def test_staging_source_mutation_is_not_accepted(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as td:
            stage = Path(td) / "stage"; stage.mkdir()
            file = stage / "kubectl"; file.write_bytes(b"before")
            before = prepare._walk_stage(stage)
            file.write_bytes(b"after")
            with self.assertRaises(prepare.GateError):
                prepare.require_stage_unchanged(stage, before)


class SecurityAndContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.expected = prepare._expected_routes()

    def test_container_args_are_fixed_network_none_and_readonly(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as td:
            stage = Path(td) / "stage"; stage.mkdir()
            args = prepare.build_container_create_args("scout-pre01-kubectl-0123456789abcdef", "a" * 32, stage)
            for item in ("--pull=never", "--network=none", "--read-only", "65534:65534", "--cap-drop=ALL",
                         "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=1g", "--cpus=1",
                         "type=bind,src=" + str(stage.resolve()) + ",dst=/tools,readonly"):
                self.assertIn(item, args)
            self.assertIn("--entrypoint=python3", args)
            self.assertEqual(args[-1], "/tools/evals/pre01-real-kubectl/payload.py")
            self.assertNotIn("/var/run/docker.sock", " ".join(args))
            with self.assertRaises(prepare.GateError):
                prepare.build_container_create_args("foreign", "a" * 32, stage)

    def test_inspected_container_must_run_the_fixed_payload(self):
        name, owner, ident = "scout-pre01-kubectl-0123456789abcdef", "a" * 32, "b" * 64
        with tempfile.TemporaryDirectory(dir="/tmp") as td:
            stage = Path(td) / "stage"; stage.mkdir()
            obj = {"Id": ident, "Name": "/" + name, "Image": prepare.IMAGE_ID,
                   "Config": {"Image": prepare.IMAGE_ID, "User": "65534:65534", "Entrypoint": ["python3"],
                              "Cmd": ["/tools/evals/pre01-real-kubectl/payload.py"],
                              "Labels": {prepare.OWNER_LABEL: owner}},
                   "HostConfig": {"ReadonlyRootfs": True, "NetworkMode": "none", "CapDrop": ["ALL"],
                                  "SecurityOpt": ["no-new-privileges"], "Privileged": False, "CapAdd": [],
                                  "PidsLimit": 64, "Memory": 1073741824, "NanoCpus": 1000000000,
                                  "Tmpfs": {"/tmp": "rw,nosuid,nodev,size=64m,uid=65534,gid=65534"}},
                   "Mounts": [{"Type": "bind", "Source": str(stage.resolve()), "Destination": "/tools", "RW": False}],
                   "State": {"Status": "created"}}
            self.assertEqual(prepare.validate_container_inspect(json.dumps(obj).encode(), name, owner, stage)["containerId"], ident)
            obj["Config"]["Cmd"] = ["/bin/sh", "-c", "unexpected"]
            with self.assertRaises(prepare.GateError):
                prepare.validate_container_inspect(json.dumps(obj).encode(), name, owner, stage)

    def test_success_requires_exact_six_requests_and_bytes(self):
        result = positive_result(self.expected)
        serialized = json.dumps(result, separators=(",", ":")).encode()
        checked = prepare.validate_payload(serialized, self.expected)
        self.assertEqual(checked["status"], "passed")
        # Source hashes are separately checked by the host-side capture.
        hashes = {k: v[1] for k, v in prepare.SOURCE_PINS.items() if v[1]}
        hashes["evals/pre01-real-kubectl/payload.py"] = digest((HERE / "payload.py").read_bytes())
        # The payload reports only pinned data-source hashes, not its own source hash.
        prepare.validate_payload(serialized, self.expected, hashes)

    def test_missing_extra_duplicate_or_phase_confused_request_fails(self):
        mutations = []
        result = positive_result(self.expected)
        missing = json.loads(json.dumps(result)); missing["phases"][0]["replayTrace"]["events"].pop()
        mutations.append(missing)
        extra = json.loads(json.dumps(result)); extra["phases"][1]["replayTrace"]["events"].append(extra["phases"][1]["replayTrace"]["events"][-1])
        mutations.append(extra)
        duplicate = json.loads(json.dumps(result)); duplicate["phases"][0]["replayTrace"]["events"][1]["requestNumber"] = 1
        mutations.append(duplicate)
        confused = json.loads(json.dumps(result)); confused["phases"][0]["commands"][0]["phase"] = "present"
        mutations.append(confused)
        wrong_route = json.loads(json.dumps(result)); wrong_route["phases"][0]["replayTrace"]["events"][0]["path"] += "?limit=1"
        mutations.append(wrong_route)
        unavailable = json.loads(json.dumps(result)); unavailable["phases"][1]["replayTrace"]["events"][1]["status"] = 503
        mutations.append(unavailable)
        forged_reason = json.loads(json.dumps(result)); forged_reason["phases"][0]["replayTrace"]["events"][2]["reason"] = "unknown"
        mutations.append(forged_reason)
        for candidate in mutations:
            with self.subTest(candidate=candidate["phases"][0]["phase"]):
                with self.assertRaises(prepare.GateError):
                    prepare.validate_payload(json.dumps(candidate).encode(), self.expected)

    def test_404_and_200_cli_semantics_are_independently_required(self):
        result = positive_result(self.expected)
        result["phases"][0]["commands"][0]["exitCode"] = 0
        with self.assertRaises(prepare.GateError):
            prepare.validate_payload(json.dumps(result).encode(), self.expected)
        result = positive_result(self.expected)
        result["phases"][1]["commands"][0]["stdoutBase64"] = encoded(b"wrong body")
        result["phases"][1]["commands"][0]["stdoutBytes"] = len(b"wrong body")
        result["phases"][1]["commands"][0]["stdoutSha256"] = digest(b"wrong body")
        with self.assertRaises(prepare.GateError):
            prepare.validate_payload(json.dumps(result).encode(), self.expected)

    def test_result_schema_and_source_hash_must_be_pinned(self):
        result = positive_result(self.expected)
        result["phases"].append(result["phases"][-1])
        with self.assertRaises(prepare.GateError):
            prepare.validate_payload(json.dumps(result).encode(), self.expected)
        result = positive_result(self.expected)
        result["source"]["replaySha256"] = "0" * 64
        with self.assertRaises(prepare.GateError):
            prepare.validate_payload(json.dumps(result).encode(), self.expected,
                                     {k: v[1] for k, v in prepare.SOURCE_PINS.items() if v[1]})
        for malformed in (b'{"schema":"x","schema":"y"}', b'{"elapsedSeconds":NaN}'):
            with self.subTest(malformed=malformed), self.assertRaises(prepare.GateError):
                prepare.validate_payload(malformed, self.expected)

    def test_cleanup_uncertainty_fails_closed(self):
        def runner(argv, timeout, env=None, max_output=None):
            return 1, b"", b"permission denied\n"
        name, owner = "scout-pre01-kubectl-0123456789abcdef", "a" * 32
        result = prepare.version_helper.cleanup(Path("/usr/bin/docker"), "orbstack", name, owner, {},
                                                time.monotonic() + 2, runner)
        self.assertFalse(result["verifiedAbsent"])
        self.assertTrue(result["errors"])

    def test_fake_cli_output_is_bounded_and_deadline_is_enforced(self):
        # These are authored Python child processes, never kubectl or Docker.
        good = payload.run_kubectl(["/bin/sh", "-c", "printf 'fixture\\n'"], time.monotonic() + 2, "/tmp/owned-kubeconfig", cwd="/tmp")
        self.assertEqual(good["status"], "passed")
        self.assertEqual(base64.b64decode(good["stdoutBase64"], validate=True), b"fixture\n")
        too_large = payload.run_kubectl(["/bin/sh", "-c", "yes x | head -c 70000"], time.monotonic() + 2, "/tmp/owned-kubeconfig", cwd="/tmp")
        self.assertEqual(too_large["status"], "failed")
        self.assertEqual(too_large["error"], "output_limit")
        self.assertTrue(too_large["outputTruncated"])
        timed = payload.run_kubectl(["/bin/sh", "-c", "sleep 1"], time.monotonic() + 0.05, "/tmp/owned-kubeconfig", cwd="/tmp")
        self.assertEqual(timed["status"], "failed")
        self.assertIn(timed["error"], ("timeout", "child_teardown_unverified"))

    def test_loopback_replay_phase_accepts_only_expected_real_http_responses(self):
        replay_path = prepare.REPO / "evals/recorded-api/replay.py"
        replay_spec = importlib.util.spec_from_file_location("pre01_local_loopback_replay", replay_path)
        assert replay_spec and replay_spec.loader
        replay = importlib.util.module_from_spec(replay_spec)
        replay_spec.loader.exec_module(replay)
        original = payload.run_kubectl
        script = ("import http.client,json,os,sys,urllib.parse; "
                  "cfg=json.load(open(os.environ['KUBECONFIG'])); "
                  "u=urllib.parse.urlsplit(cfg['clusters'][0]['cluster']['server']); "
                  "c=http.client.HTTPConnection(u.hostname,u.port,timeout=2); "
                  "c.request('GET',sys.argv[1]); r=c.getresponse(); b=r.read(); "
                  "sys.stdout.buffer.write(b) if r.status==200 else sys.stderr.buffer.write(('kubectl HTTP '+str(r.status)+'\\n').encode()); "
                  "sys.exit(0 if r.status==200 else 1)")

        def fake_kubectl(argv, deadline, kubeconfig_path=None):
            fake_argv = [sys.executable, "-c", script, argv[-1]]
            result = original(fake_argv, deadline, kubeconfig_path, cwd="/tmp")
            result["argv"] = argv
            return result

        payload.run_kubectl = fake_kubectl
        try:
            with tempfile.TemporaryDirectory(dir="/tmp") as td:
                for phase in ("absent", "present"):
                    result = payload._run_phase(replay, phase, time.monotonic() + 20, runtime_dir=Path(td))
                    self.assertEqual(result["status"], "passed", result)
                    self.assertTrue(result["listenerCleanupConfirmed"])
                    self.assertEqual(result["replayTrace"]["requestsObserved"], 3)
                    self.assertEqual([event["status"] for event in result["replayTrace"]["events"]],
                                     [404, 404, 404] if phase == "absent" else [200, 200, 200])
        finally:
            payload.run_kubectl = original


if __name__ == "__main__":
    unittest.main()
