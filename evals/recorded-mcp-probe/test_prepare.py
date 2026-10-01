#!/usr/bin/env python3
"""No-model tests for preparation refusal and generated wrapper boundaries."""
import hashlib
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('prepare', ROOT / 'prepare.py')
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)

class PreparationGuards(unittest.TestCase):
    def test_pair_runner_stops_owned_descendants_and_refuses_relaunch(self):
        import json
        import time
        runner_spec = importlib.util.spec_from_file_location('run_pair', ROOT / 'run_pair.py')
        runner = importlib.util.module_from_spec(runner_spec)
        runner_spec.loader.exec_module(runner)
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'plugin').mkdir()
            heartbeat = root / 'heartbeat'
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + "); " + "\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            parent = "import subprocess,sys,signal,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); time.sleep(60)"
            self.assertEqual(runner.run_owned([sys.executable, '-c', parent], root, timeout=.4, grace=.1), 124)
            self.assertTrue(heartbeat.exists())
            final = heartbeat.read_bytes()
            time.sleep(.1)
            self.assertEqual(heartbeat.read_bytes(), final)
            self.assertTrue(json.loads((root / 'completion.json').read_text())['timedOut'])
            with self.assertRaises(FileExistsError):
                runner.run_owned([sys.executable, '-c', 'raise SystemExit(99)'], root)
            self.assertEqual(heartbeat.read_bytes(), final)

    def test_pair_runner_metadata_cannot_override_owned_process_record(self):
        runner_spec = importlib.util.spec_from_file_location('run_pair', ROOT / 'run_pair.py')
        runner = importlib.util.module_from_spec(runner_spec)
        runner_spec.loader.exec_module(runner)
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with self.assertRaisesRegex(ValueError, 'cannot override owned-process fields'):
                runner.run_owned([sys.executable, '-c', 'pass'], root,
                                 launch_metadata={'pid': 0})
            self.assertFalse((root / 'launch.json').exists())

    def test_pair_runner_cleans_group_on_external_signals(self):
        import json
        import signal
        import time
        for signum in (signal.SIGINT, signal.SIGTERM):
            with self.subTest(signum=signum), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                (root / 'plugin').mkdir()
                heartbeat = root / 'heartbeat'
                child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + "); " + "\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
                parent = "import subprocess,sys,signal,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); time.sleep(60)"
                driver = "import sys; sys.path.insert(0," + repr(str(ROOT)) + "); from run_pair import run_owned; from pathlib import Path; raise SystemExit(run_owned(" + repr([sys.executable, '-c', parent]) + ",Path(" + repr(str(root)) + "),timeout=5,grace=.1))"
                process = subprocess.Popen([sys.executable, '-c', driver])
                try:
                    deadline = time.monotonic() + 3
                    while not heartbeat.exists() and time.monotonic() < deadline:
                        time.sleep(.02)
                    self.assertTrue(heartbeat.exists())
                    process.send_signal(signum)
                    self.assertEqual(process.wait(timeout=8), 128 + signum)
                    last = heartbeat.read_bytes()
                    time.sleep(.1)
                    self.assertEqual(heartbeat.read_bytes(), last)
                    self.assertEqual(json.loads((root / 'completion.json').read_text())['interruptedBySignal'], signum)
                finally:
                    if process.poll() is None:
                        process.terminate()
                        process.wait(timeout=8)

    def test_purpose_configuration_preserves_plumbing_and_makes_economy_neutral(self):
        import json
        with tempfile.TemporaryDirectory() as tmp:
            plugin = Path(tmp) / 'plugin'
            shutil.copytree(ROOT / 'template/plugin', plugin)
            manifest_template = plugin / '.claude-plugin/plugin.json.in'
            manifest_path = plugin / '.claude-plugin/plugin.json'
            manifest_path.write_bytes(manifest_template.read_bytes())
            case = plugin / 'evals/recorded-explain-mcp'
            original_prompt = (case / 'prompt.md').read_bytes()
            original_tool_grader = (case / 'graders/tool-called.md').read_bytes()
            original_manifest = manifest_template.read_bytes()

            prepare.configure_purpose(plugin, 'plumbing')
            self.assertEqual((case / 'prompt.md').read_bytes(), original_prompt)
            self.assertEqual((case / 'graders/tool-called.md').read_bytes(), original_tool_grader)

            prepare.configure_purpose(plugin, 'economy')
            prompt = (case / 'prompt.md').read_text()
            self.assertIn('max_turns: 8', prompt)
            self.assertIn('timeout_seconds: 90', prompt)
            self.assertIn('no particular tool, file-reading sequence, or amount of reading is required', prompt)
            self.assertNotIn('call the available `explain` MCP tool', prompt)
            self.assertNotIn('Read `cluster/deployments.yaml`', prompt)
            self.assertTrue((case / 'graders/answer.md').is_file())
            self.assertFalse((case / 'graders/tool-called.md').exists())
            manifest = json.loads((plugin / '.claude-plugin/plugin.json').read_text())
            self.assertEqual(manifest['description'], 'Recorded-only evidence for a Kubernetes resource question.')
            self.assertEqual(original_manifest, (ROOT / 'template/plugin/.claude-plugin/plugin.json.in').read_bytes())

    def test_economy_skill_adds_generic_route_without_changing_prompt_or_recording(self):
        import json
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            plugins = {}
            for purpose in ('economy', 'economy-skill'):
                plugin = base / purpose
                shutil.copytree(ROOT / 'template/plugin', plugin)
                manifest_template = plugin / '.claude-plugin/plugin.json.in'
                (plugin / '.claude-plugin/plugin.json').write_bytes(manifest_template.read_bytes())
                prepare.configure_purpose(plugin, purpose)
                case = plugin / 'evals/recorded-explain-mcp'
                fixtures = case / 'fixtures'
                fixtures.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(prepare.SOURCE_FIXTURE, fixtures / 'deployments.yaml')
                (fixtures / 'recording.json').write_text('{"kind":"recorded input"}\n')
                plugins[purpose] = plugin

            plain_case = plugins['economy'] / 'evals/recorded-explain-mcp'
            skill_case = plugins['economy-skill'] / 'evals/recorded-explain-mcp'
            self.assertEqual((plain_case / 'prompt.md').read_bytes(), (skill_case / 'prompt.md').read_bytes())
            self.assertEqual((plain_case / 'case.yaml').read_bytes(), (skill_case / 'case.yaml').read_bytes())
            self.assertEqual((plain_case / 'graders/answer.md').read_bytes(), (skill_case / 'graders/answer.md').read_bytes())
            self.assertFalse((plain_case / 'graders/tool-called.md').exists())
            self.assertFalse((skill_case / 'graders/tool-called.md').exists())
            self.assertEqual((plain_case / 'fixtures/deployments.yaml').read_bytes(), (skill_case / 'fixtures/deployments.yaml').read_bytes())
            self.assertEqual((plain_case / 'fixtures/recording.json').read_bytes(), (skill_case / 'fixtures/recording.json').read_bytes())

            ordinary = plugins['economy'] / 'skills'
            routed = plugins['economy-skill'] / 'skills/recorded-field-attribution/SKILL.md'
            self.assertFalse(ordinary.exists())
            self.assertTrue(routed.is_file())
            skill = routed.read_text()
            self.assertIn('known resource and exact canonical field path', skill)
            self.assertIn('field_path', skill)
            self.assertIn('Raw recorded object evidence may still be read', skill)
            self.assertIn('unknown', skill)
            self.assertIn('latest writer', skill)
            for fixture_specific in ('checkout', 'kubectl-set', 'shop-apps', 'Flux', '305614fa67327ba3'):
                self.assertNotIn(fixture_specific, skill)

            plain_manifest = json.loads((plugins['economy'] / '.claude-plugin/plugin.json').read_text())
            routed_manifest = json.loads((plugins['economy-skill'] / '.claude-plugin/plugin.json').read_text())
            self.assertEqual(plain_manifest, routed_manifest)
            self.assertEqual(routed_manifest['description'], 'Recorded-only evidence for a Kubernetes resource question.')

    def test_pair_runner_accepts_economy_skill_with_same_bounded_policy_without_running(self):
        import json
        spec = importlib.util.spec_from_file_location('run_pair', ROOT / 'run_pair.py')
        runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runner)
        for purpose in ('economy', 'economy-skill'):
            with self.subTest(purpose=purpose), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                plugin = root / 'plugin'
                plugin.mkdir()
                marker = plugin / 'marker'
                marker.write_text(purpose)
                binary = root / 'cub-scout'
                binary.write_text('pinned-test-binary')
                prepared = {
                    'purpose': purpose,
                    'modelRun': False,
                    'binary': str(binary),
                    'binarySha256': prepare.sha256(binary),
                    'generatedPluginFiles': {'marker': prepare.sha256(marker)},
                }
                (root / 'prepared.json').write_text(json.dumps(prepared))
                captured = {}
                def fake_run_owned(command, run_root):
                    captured['command'] = command
                    captured['root'] = run_root
                    return 0
                with mock.patch.object(runner, 'run_owned', side_effect=fake_run_owned), \
                     mock.patch.object(sys, 'argv', ['run_pair.py', str(root)]):
                    self.assertEqual(runner.main(), 0)
                self.assertEqual(captured['root'], root.resolve())
                command = captured['command']
                self.assertIn('--runs', command)
                self.assertEqual(command[command.index('--runs') + 1], '1')
                self.assertEqual(command[command.index('--concurrency') + 1], '1')
                self.assertEqual(command[command.index('--max-cost-usd') + 1], '1')
                self.assertIn('--keep-temp', command)
                self.assertIn('--allow-real-servers', command)

    def test_invalid_purpose_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaisesRegex(ValueError, 'unsupported purpose'):
                prepare.configure_purpose(Path(tmp), 'benchmark')

    def test_hash_mismatch_and_existing_output_are_not_overwritten(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'binary'
            binary.write_text('#!/bin/sh\nexit 71\n')
            binary.chmod(0o700)
            out = root / 'output'
            args = [sys.executable, str(ROOT / 'prepare.py'), '--binary', str(binary), '--out', str(out), '--binary-sha256']
            mismatch = subprocess.run(args + ['0' * 64], text=True, capture_output=True, timeout=10)
            self.assertNotEqual(mismatch.returncode, 0)
            self.assertIn('binary hash mismatch', mismatch.stderr)
            self.assertFalse(out.exists())
            out.mkdir()
            sentinel = out / 'retained'
            sentinel.write_text('existing evidence')
            existing = subprocess.run(args + [prepare.sha256(binary)], text=True, capture_output=True, timeout=10)
            self.assertNotEqual(existing.returncode, 0)
            self.assertIn('refusing to overwrite', existing.stderr)
            self.assertEqual(sentinel.read_text(), 'existing evidence')

    def test_wrapper_quotes_paths_and_clears_inherited_environment(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            unusual = root / "space 'quote' $(touch PWNED)"
            unusual.mkdir()
            binary = unusual / 'binary'
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "${SCOUT_TEST_SECRET-unset}" "$HOME" "$KUBECONFIG" "$@"\n')
            binary.chmod(0o700)
            fixture = unusual / 'recording'
            fixture.write_bytes(prepare.SOURCE_FIXTURE.read_bytes())
            home = unusual / 'home'
            home.mkdir()
            kubeconfig = unusual / 'kubeconfig'
            kubeconfig.write_bytes(b'')
            wrapper = root / 'wrapper'
            text = (ROOT / 'template/plugin/server-wrapper.sh.in').read_text()
            values = {'BINARY_PATH': prepare.shell_quote(str(binary)),
                      'BINARY_SHA256': prepare.sha256(binary),
                      'RECORDING_PATH': prepare.shell_quote(str(fixture)),
                      'SERVER_HOME': prepare.shell_quote(str(home)),
                      'EMPTY_KUBECONFIG': prepare.shell_quote(str(kubeconfig))}
            for key, value in values.items():
                text = text.replace('@' + key + '@', value)
            wrapper.write_text(text)
            wrapper.chmod(0o700)
            env = {**os.environ, 'SCOUT_TEST_SECRET': 'must-not-reach-server'}
            result = subprocess.run([str(wrapper)], cwd=root, env=env, text=True, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), ['unset', str(home), str(kubeconfig), 'mcp', 'serve', '--recording', str(fixture)])
            self.assertFalse((root / 'PWNED').exists())
            fixture.write_bytes(b'changed')
            rejected = subprocess.run([str(wrapper)], cwd=root, env=env, text=True, capture_output=True, timeout=10)
            self.assertNotEqual(rejected.returncode, 0)
            self.assertIn('recording hash mismatch', rejected.stderr)
            self.assertEqual(rejected.stdout, '')

if __name__ == '__main__':
    unittest.main()
