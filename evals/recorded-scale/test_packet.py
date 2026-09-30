#!/usr/bin/env python3
"""Offline tests for recorded scale staging and preflight validation."""
import hashlib
import copy
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("recorded_scale_prepare", ROOT / "prepare.py")
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)
spec = importlib.util.spec_from_file_location("recorded_scale_preflight", ROOT / "preflight.py")
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)


def expected_report():
    names = preflight.NO_MARKER
    resources = [{"namespace": item.split("/", 1)[0], "name": item.split("/", 1)[1], "owner": "Native"}
                 for item in names]
    resources.extend({"namespace": f"team-{i:02d}", "name": f"managed-{i}", "owner": "Flux"}
                     for i in range(288))
    return {
        "schema": "map-list-recorded.v1",
        "provenance": {"objectCount": 302, "sha256": preflight.DEPLOYMENT_SHA256},
        "scope": {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"},
        "selectedCount": 300, "excludedFromScopeCount": 2,
        "ownerCounts": copy.deepcopy(preflight.EXPECTED_COUNTS),
        "resources": resources,
    }


class RecordedScalePacket(unittest.TestCase):
    def test_staged_pair_files_are_byte_identical_and_match_pinned_sources(self):
        with tempfile.TemporaryDirectory() as tmp:
            plugin = Path(tmp) / "plugin"
            plugin.mkdir()
            facts = prepare.stage_case(plugin, "scale-ownership-counts")
            self.assertEqual(set(facts), {Path(p).name for p in prepare.FILES})
            for name, metadata in facts.items():
                raw = (plugin / "evals/recorded-scale/scale-ownership-counts/fixtures" / name).read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), metadata["sha256"])
            # The generated shell scaffold is exercised twice and compares every byte.
            self.assertEqual((plugin / "evals/recorded-scale/scale-ownership-counts/prompt.md").read_bytes(),
                             (prepare.REPO / "evals/scale/scale-ownership-counts/prompt.md").read_bytes())

    def test_scope_counts_and_all_twelve_no_marker_identities(self):
        preflight.validate_report(expected_report())
        self.assertTrue(preflight.exact_map_schema({
            "type": "object", "properties": {key: {"type": "string"} for key in
                ("api_version", "kind", "namespace", "namespace_prefix")},
            "required": ["api_version", "kind"], "additionalProperties": False}))
        invalid_schema = {"type": "object", "properties": {key: {"type": "string"} for key in
                          ("api_version", "kind", "namespace", "namespace_prefix", "context")},
                          "required": ["api_version", "kind"], "additionalProperties": False}
        self.assertFalse(preflight.exact_map_schema(invalid_schema))
        for mutation in ("counts", "native-names", "scope"):
            report = expected_report()
            if mutation == "counts":
                report["ownerCounts"]["Native"] = 11
            elif mutation == "native-names":
                report["resources"].pop()
            else:
                report["scope"]["namespacePrefix"] = "team"
            with self.subTest(mutation=mutation), self.assertRaises(AssertionError):
                preflight.validate_report(report)

    def test_prepared_file_tampering_and_binary_hash_mismatch_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            plugin = root / "plugin"
            plugin.mkdir()
            marker = plugin / "marker"
            marker.write_text("pinned")
            binary = root / "binary"
            binary.write_text("binary")
            facts = {"binarySha256": preflight.sha256(binary),
                     "binary": str(binary),
                     "recordingManifestSha256": "",
                     "generatedPluginFiles": {"marker": preflight.sha256(marker)}}
            (root / "source-recording-manifest.json").write_text("manifest")
            facts["recordingManifestSha256"] = preflight.sha256(root / "source-recording-manifest.json")
            (root / "kubeconfig-empty").write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            (root / "prepared.json").write_text(json.dumps(facts))
            preflight.verify_prepared(root, binary, facts["binarySha256"])
            marker.write_text("changed")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                preflight.verify_prepared(root, binary, facts["binarySha256"])
            marker.write_text("pinned")
            with self.assertRaisesRegex(ValueError, "pinned hash"):
                preflight.verify_prepared(root, binary, "0" * 64)
            other = root / "other-binary"
            other.write_bytes(binary.read_bytes())
            with self.assertRaisesRegex(ValueError, "binary path differs"):
                preflight.verify_prepared(root, other, facts["binarySha256"])
            kubeconfig = root / "kubeconfig-empty"
            kubeconfig.write_text("apiVersion: v1\nkind: Config\nclusters: [{server: https://unexpected}]\n")
            with self.assertRaisesRegex(ValueError, "kubeconfig"):
                preflight.verify_prepared(root, binary, facts["binarySha256"])

    def test_configuration_keeps_full_plugin_and_limits_recorded_server_tooling(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "binary"
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "${HOME-unset}" "$KUBECONFIG" "$@"\n')
            binary.chmod(0o700)
            plugin = root / "plugin"
            plugin.mkdir()
            (plugin / ".claude-plugin").mkdir()
            (plugin / ".claude-plugin/plugin.json").write_text(json.dumps({
                "name": "cub-scout", "skills": ["retained"], "mcpServers": {"other": {"command": "x"}}}))
            record = root / "recording.yaml"
            record.write_bytes(b"recording")
            kubeconfig = root / "empty-kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            prepare.generate_wrapper(plugin, binary, prepare.sha256(binary), record, prepare.sha256(record), kubeconfig)
            manifest = json.loads((plugin / ".claude-plugin/plugin.json").read_text())
            self.assertEqual(manifest["name"], "cub-scout")
            self.assertEqual(manifest["skills"], ["retained"])
            self.assertEqual(list(manifest["mcpServers"]), ["cub-scout"])
            self.assertEqual(manifest["mcpServers"]["cub-scout"]["args"], [])
            wrapper = (plugin / "bin/recorded-cub-scout").read_text()
            self.assertIn("env -i", wrapper)
            self.assertIn(str(kubeconfig), wrapper)
            self.assertIn(prepare.sha256(record), wrapper)
            self.assertNotIn("HOME=", wrapper)
            self.assertIn("mcp serve --recording", wrapper)
            result = subprocess.run([str(plugin / "bin/recorded-cub-scout")], text=True,
                                    capture_output=True, timeout=5,
                                    env={"PATH": "/usr/bin:/bin", "HOME": "must-not-be-forwarded",
                                         "KUBECONFIG": "must-not-be-forwarded"})
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), ["unset", str(kubeconfig), "mcp", "serve",
                                                          "--recording", str(record)])
            record.write_bytes(b"changed after preparation")
            rejected = subprocess.run([str(plugin / "bin/recorded-cub-scout")], text=True,
                                      capture_output=True, timeout=5,
                                      env={"PATH": "/usr/bin:/bin", "KUBECONFIG": str(kubeconfig)})
            self.assertNotEqual(rejected.returncode, 0)
            self.assertEqual(rejected.stdout, "")

    def test_full_prepare_is_offline_and_keeps_manifest_outside_case_inputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "cub-scout"
            binary.write_text("#!/bin/sh\nexit 0\n")
            binary.chmod(0o700)
            out = root / "prepared"
            prepare.prepare(binary, prepare.sha256(binary), out)
            facts = json.loads((out / "prepared.json").read_text())
            self.assertFalse(facts["modelRun"])
            self.assertEqual(facts["preflight"], "not run by preparation; invoke separately with explicit --binary")
            self.assertTrue(facts["allSevenFilesByteEqualAcrossArms"])
            self.assertEqual((out / "source-recording-manifest.json").read_bytes(), prepare.MANIFEST.read_bytes())
            a = out / "plugin/evals/recorded-scale/scale-ownership-counts/fixtures"
            b = out / "plugin/evals/recorded-scale/scale-unmanaged/fixtures"
            self.assertEqual({p.name: p.read_bytes() for p in a.iterdir()},
                             {p.name: p.read_bytes() for p in b.iterdir()})
            self.assertFalse((a / "recording-2026-09-30.json").exists())
            for case in prepare.CASES:
                graders = out / "plugin/evals/recorded-scale" / case / "graders"
                self.assertTrue(any(graders.glob("*.md")), "correctness grader retained")
                self.assertFalse(list(graders.glob("used-cub-scout-*.md")))
            preflight.verify_prepared(out, binary, prepare.sha256(binary))

    def test_mcp_rejects_unsupported_tool_and_live_args_and_holds_startup_snapshot(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            report = expected_report()
            fake = root / "fake-mcp"
            # This tiny stdio fixture parses the immutable report before serving requests.
            fake.write_text("#!" + sys.executable + "\n" + """
import json,sys
report = json.loads(%r)
for line in sys.stdin:
 m=json.loads(line); i=m.get('id'); method=m.get('method'); p=m.get('params',{})
 if i is None: continue
 if method == 'initialize': result={'protocolVersion':'2024-11-05'}
 elif method == 'tools/list': result={'tools':[{'name':'explain'},{'name':'map','inputSchema':{'type':'object','properties':{'api_version':{'type':'string'},'kind':{'type':'string'},'namespace':{'type':'string'},'namespace_prefix':{'type':'string'}},'required':['api_version','kind'],'additionalProperties':False}}]}
 elif method == 'tools/call' and p.get('name') not in ('map','explain'): result={'isError':True,'content':[]}
 elif method == 'tools/call' and p.get('name') == 'map' and 'kube_context' in p.get('arguments',{}): result={'isError':True,'content':[]}
 elif method == 'tools/call' and p.get('name') == 'map': result={'content':[{'type':'text','text':json.dumps(report)}]}
 else: result={}
 print(json.dumps({'jsonrpc':'2.0','id':i,'result':result}),flush=True)
""" % json.dumps(report))
            fake.chmod(0o700)
            wrapper = fake
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original bytes")
            result = preflight.call_map_stdio(wrapper, kubeconfig, recording)
            self.assertEqual(result["tools"], ["explain", "map"])
            self.assertTrue(result["unsupportedToolRejected"] and result["unsupportedLiveArgumentRejected"])
            self.assertEqual(recording.read_bytes(), b"original bytes")

    def test_timeout_drains_stderr_bounds_partial_line_and_kills_owned_descendant(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            script = ("#!" + sys.executable + "\nimport signal,subprocess,sys,time\n"
                      "signal.signal(signal.SIGTERM, signal.SIG_IGN)\n"
                      "subprocess.Popen([sys.executable,'-c'," + repr(child) + "])\n"
                      "sys.stderr.write('e'*200000); sys.stderr.flush()\n"
                      "sys.stdout.write('{\\\"jsonrpc\\\":'); sys.stdout.flush()\n"
                      "time.sleep(30)\n")
            server = root / "fake-server"
            server.write_text(script)
            server.chmod(0o700)
            wrapper = server
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original")
            started = __import__("time").monotonic()
            with self.assertRaises(TimeoutError):
                preflight.call_map_stdio(wrapper, kubeconfig, recording, timeout=0.3)
            self.assertLess(__import__("time").monotonic() - started, 3)
            self.assertEqual(recording.read_bytes(), b"original")
            self.assertTrue(heartbeat.exists())
            value = heartbeat.read_bytes()
            __import__("time").sleep(0.12)
            self.assertEqual(heartbeat.read_bytes(), value)

    def test_run_bounded_timeout_cleans_inherited_pipe_descendants(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            parent = ("import signal,subprocess,sys,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); "
                      "subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); "
                      "sys.stderr.write('x'*200000); sys.stderr.flush(); time.sleep(30)")
            started = __import__("time").monotonic()
            with self.assertRaisesRegex(TimeoutError, "timed out"):
                preflight.run_bounded([sys.executable, "-c", parent], {"PATH": "/usr/bin:/bin"}, timeout=0.3)
            self.assertLess(__import__("time").monotonic() - started, 3)
            self.assertTrue(heartbeat.exists())
            value = heartbeat.read_bytes()
            __import__("time").sleep(0.12)
            self.assertEqual(heartbeat.read_bytes(), value)

    def test_cancellation_restores_private_recording_and_cleans_owned_group(self):
        import signal
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            server = root / "fake-server"
            server.write_text("#!" + sys.executable + "\nimport signal,subprocess,sys,time\n"
                              "signal.signal(signal.SIGTERM,signal.SIG_IGN)\n"
                              "subprocess.Popen([sys.executable,'-c'," + repr(child) + "])\n"
                              "sys.stdout.write('{\\\"jsonrpc\\\":'); sys.stdout.flush()\n"
                              "time.sleep(30)\n")
            server.chmod(0o700)
            wrapper = server
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original")
            module = str(ROOT / "preflight.py")
            driver = ("import importlib.util,sys,pathlib; spec=importlib.util.spec_from_file_location('pf'," + repr(module) + "); "
                      "m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m); "
                      "m.call_map_stdio(pathlib.Path(" + repr(str(wrapper)) + "),pathlib.Path(" + repr(str(kubeconfig)) + "),pathlib.Path(" + repr(str(recording)) + "),timeout=20)")
            process = subprocess.Popen([sys.executable, "-c", driver], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                       env={"PATH": "/usr/bin:/bin"}, start_new_session=True)
            try:
                deadline = __import__("time").monotonic() + 3
                while not heartbeat.exists() and __import__("time").monotonic() < deadline:
                    __import__("time").sleep(0.02)
                self.assertTrue(heartbeat.exists())
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=3)
                self.assertNotEqual(process.returncode, 0)
                self.assertEqual(recording.read_bytes(), b"original")
                value = heartbeat.read_bytes()
                __import__("time").sleep(0.12)
                self.assertEqual(heartbeat.read_bytes(), value)
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=3)


if __name__ == "__main__":
    unittest.main()
