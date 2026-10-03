"""Offline source/launch-policy and exec-guard contracts; no CLI is executed."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


policy = load('full24_policy', HERE / 'policy.py')
server = load('full24_server', HERE / 'recorded_server.py')


class PolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='full24-launch-', dir=Path('/tmp').resolve())
        cls.root = Path(cls.temp.name).resolve()
        cls.source = cls.root / 'source'
        cls.source.mkdir()
        cls.report = policy.stager.prepare.prepare(cls.source)
        policy.stager.prepare.validate(cls.source)
        cls.raw_report = (cls.source / 'preflight.json').read_bytes()
        cls.counter = 0

    @classmethod
    def tearDownClass(cls):
        for path in cls.root.rglob('*'):
            if not path.is_symlink():
                path.chmod(0o700 if path.is_dir() else 0o600)
        cls.temp.cleanup()

    def stage(self, cid, arm, control=None):
        type(self).counter += 1
        out = self.root / ('stage-' + str(self.counter))
        report = copy.deepcopy(self.report)
        report['_source_root'] = str(self.source)
        receipt = policy.stager._stage_validated(self.source, out, cid, arm, control, report, self.raw_report)
        # Exact reconstruction still runs; source was validated once for this
        # immutable test fixture. Public build() revalidates it on every call.
        policy.stager._verify_validated_stage(out, receipt, self.source, copy.deepcopy(self.report), self.raw_report)
        return out, policy._build(receipt, self.report, out / 'model-stage')

    def test_all24_both_arms_keep_prompts_grants_and_budgets(self):
        supported = blocked = 0
        for case in self.report['cases']:
            cid = case['id']
            _, baseline = self.stage(cid, 'without')
            _, treatment = self.stage(cid, 'with')
            self.assertEqual(baseline['status'], 'candidate_not_executed')
            for plan in (baseline, treatment):
                self.assertEqual(plan['ordinaryToolGrant'], case['ordinaryToolGrant'])
                self.assertEqual(plan['budget']['source'], case['budgets'])
                self.assertTrue(all(v is False for v in plan['claims'].values()))
                self.assertTrue(plan['runtimeAssetAdmission']['status'].startswith('blocked_'))
                for tool in ('Bash', 'Edit', 'Write', 'Task', 'Agent', 'mcp__unknown__map', '*', ''):
                    self.assertFalse(policy.authorize_tool(plan, tool))
            self.assertEqual(baseline['prompt'], treatment['prompt'])
            self.assertEqual(baseline['budget'], treatment['budget'])
            self.assertEqual(baseline['mcpConfig'], {'mcpServers': {}})
            self.assertNotIn('--plugin-dir', baseline['argv'])
            for tool in case['ordinaryToolGrant']:
                self.assertTrue(policy.authorize_tool(baseline, tool))
            if cid in policy.RECORDED_CASES:
                supported += 1
                self.assertEqual(treatment['status'], 'candidate_not_executed')
                self.assertEqual(treatment['argv'][treatment['argv'].index('--tools') + 1], ','.join(case['ordinaryToolGrant']))
                self.assertEqual(treatment['argv'][treatment['argv'].index('--allowedTools') + 1], ','.join(case['ordinaryToolGrant'] + policy.MCP_TOOLS))
                self.assertIn('--strict-mcp-config', treatment['argv'])
                self.assertEqual(treatment['argv'][treatment['argv'].index('--setting-sources') + 1], '')
                self.assertTrue(policy.authorize_tool(treatment, policy.MCP_TOOLS[0]))
                self.assertEqual(policy.exec_binding(treatment)['binarySha256'], server.LINUX_SCOUT_PIN)
                self.assertIsNone(treatment['runtimeAssetAdmission']['python']['trustedSha256'])
                self.assertEqual(treatment['runtimeAssetAdmission']['wrapper']['sourceSha256'], policy.sha((HERE / 'recorded_server.py').read_bytes()))
            else:
                blocked += 1
                self.assertEqual(treatment['status'], 'blocked_recorded_mcp_binding')
                self.assertIsNone(treatment['argv'])
                self.assertIsNone(treatment['mcpConfig'])
                with self.assertRaises(ValueError):
                    policy.exec_binding(treatment)
        self.assertEqual((supported, blocked), (11, 13))
        self.assertEqual(policy.RECORDINGS, server.RECORDINGS)
        for cid, recording in policy.RECORDINGS.items():
            _, plan = self.stage(cid, 'with')
            self.assertEqual(plan['recordedMcp']['relativePath'], recording)
            self.assertEqual(plan['recordedMcp']['sha256'], plan['stageFiles'][recording])

    def test_authored_controls_stay_case_bound_without_inherited_original_evidence(self):
        for cid in ('DEL-03', 'DEL-04'):
            control = self.report['authoredInputControls'][cid]['id']
            _, original = self.stage(cid, 'without')
            _, left = self.stage(cid, 'without', control)
            _, right = self.stage(cid, 'with', control)
            self.assertEqual(left['prompt'], right['prompt'])
            self.assertNotEqual(left['prompt']['sha256'], original['prompt']['sha256'])
            self.assertEqual(left['ordinaryToolGrant'], original['ordinaryToolGrant'])
            self.assertEqual(left['budget'], original['budget'])
            self.assertFalse(any('oracle' in path for path in left['stageFiles']))
            self.assertEqual(right['status'], 'blocked_recorded_mcp_binding')

    def test_policy_reconstruction_rejects_any_material_drift(self):
        root, original = self.stage('ATR-01', 'with')
        mutations = [lambda p: p['argv'].extend(['--dangerously-skip-permissions']),
                     lambda p: p['ordinaryToolGrant'].append('Bash'),
                     lambda p: p['recordedMcp'].update(case='ATR-02'),
                     lambda p: p['runtimePins']['cub-scout'].update(sha256='0' * 64),
                     lambda p: p['environment'].update(CUB_AUTH_TOKEN='unexpected'),
                     lambda p: p['budget'].update(timeout_seconds=9999),
                     lambda p: p['claims'].update(ordinaryToolGrantEnforced=True),
                     lambda p: p['prompt'].update(bodySha256='0' * 64)]
        with patch.object(policy, 'build', return_value=original):
            for change in mutations:
                altered = copy.deepcopy(original)
                change(altered)
                with self.assertRaises(ValueError):
                    policy.verify(altered, root, self.source, 'ATR-01', 'with')
            self.assertEqual(policy.verify(copy.deepcopy(original), root, self.source, 'ATR-01', 'with'), original)

    def test_public_builder_rejects_selector_and_source_corruption(self):
        root, original = self.stage('ATR-01', 'with')
        self.assertEqual(policy.build(root, self.source, 'ATR-01', 'with'), original)
        with self.assertRaises(ValueError):
            policy.build(root, self.source, 'ATR-02', 'with')
        prompt = root / 'model-stage/prompt.md'
        prompt.chmod(0o600)
        prompt.write_bytes(b'changed')
        with self.assertRaises(ValueError):
            policy.build(root, self.source, 'ATR-01', 'with')

    def test_exec_guard_rejects_wrong_case_pin_paths_and_nonregular_files(self):
        recording, binary = self.root / 'recording.yaml', self.root / 'binary'
        recording.write_bytes(b'fixture')
        binary.write_bytes(b'executable')
        binary.chmod(0o700)
        binding = {'schema': 'full24-recorded-mcp-exec.v1', 'case': 'ATR-01',
                   'recording': str(recording), 'recordingSha256': policy.sha(recording.read_bytes()),
                   'binary': str(binary), 'binarySha256': policy.sha(binary.read_bytes())}
        def validate(value=binding):
            return server.validate(value, 'ATR-01', recording_path=str(recording), binary_path=str(binary),
                                   trusted_binary_sha256=policy.sha(b'executable'))
        self.assertEqual(validate(), [str(binary), 'mcp', 'serve', '--recording', str(recording)])
        for key, value in [('case', 'ATR-02'), ('case', []), ('binarySha256', '0' * 64),
                           ('recordingSha256', '0' * 64), ('binary', '/usr/bin/sh'),
                           ('recording', str(self.source / 'oracle/answer.md')), ('extra', True)]:
            with self.assertRaises(ValueError):
                validate({**binding, key: value})
        with self.assertRaises(ValueError):
            server.unique([('case', 'ATR-01'), ('case', 'ATR-01')])
        with self.assertRaises(ValueError):
            server.validate(binding, 'ATR-01', recording_path=str(recording), binary_path=str(binary))
        alias = self.root / 'alias'
        alias.symlink_to(recording)
        with self.assertRaises(ValueError):
            server.read_regular(alias, 1024)
        fifo = self.root / 'fifo'
        os.mkfifo(fifo)
        # Reject FIFO before open: opening one read-only can otherwise block.
        with self.assertRaises(ValueError):
            server.read_regular(fifo, 1024)


if __name__ == '__main__':
    unittest.main()
