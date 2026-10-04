"""Source-only runtime overlay contracts; nothing is executed or mounted."""
import json
from pathlib import Path
import unittest

import overlay
import test_policy


class OverlayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        test_policy.PolicyTests.setUpClass()
        cls.fixture = test_policy.PolicyTests()
        cls.source = test_policy.PolicyTests.source
        cls.root = test_policy.PolicyTests.root

    @classmethod
    def tearDownClass(cls):
        test_policy.PolicyTests.tearDownClass()

    def test_all24_arm_overlay_boundaries_and_explicit_unsupported_cases(self):
        for cid in overlay.policy.stager.prepare.FROZEN_CASE_IDS:
            for arm in ('without', 'with'):
                stage, plan = self.fixture.stage(cid, arm)
                if plan['status'] != 'candidate_not_executed':
                    with self.assertRaises(ValueError):
                        overlay.render(plan, stage / 'model-stage')
                    continue
                files = overlay.render(plan, stage / 'model-stage')
                self.assertFalse(any('oracle' in p or 'cases/' in p or p.startswith('cluster/') for p in files))
                if arm == 'without':
                    self.assertEqual(set(files), {'mcp.json', 'launch-policy.json', 'dispatch_guard.py',
                                                 'guard-settings.json', 'dispatch-binding.json'})
                    self.assertEqual(json.loads(files['mcp.json']), {'mcpServers': {}})
                    self.assertEqual(json.loads(files['dispatch-binding.json'])['recordedTools'], [])
                else:
                    plugin = json.loads(files['plugin/.claude-plugin/plugin.json'])
                    self.assertNotIn('mcpServers', plugin)
                    self.assertNotIn('recorded-mcp-binding.json', files)
                    self.assertEqual(overlay.policy.sha(files['recorded_server.py']), plan['runtimeAssetAdmission']['wrapper']['sourceSha256'])
                    skills = [p for p in files if p.startswith('plugin/skills/')]
                    self.assertEqual(len(skills), len([p for p in plan['stageFiles'] if p.startswith('skills/')]))
                    for path in skills:
                        self.assertEqual(overlay.policy.sha(files[path]), plan['stageFiles'][path[len('plugin/'):]])
                    self.assertEqual(json.loads(files['recorded-binding.json']), overlay.policy.exec_binding(plan))

    def test_materialization_is_new_readonly_source_bound_and_drift_refuses(self):
        stage, plan = self.fixture.stage('ATR-01', 'with')
        output = self.root / 'runtime-overlay'
        receipt = overlay.build(self.source, stage, 'ATR-01', 'with', output)
        self.assertEqual(overlay.verify(self.source, stage, 'ATR-01', 'with', output), receipt)
        self.assertTrue(all(value is False for value in receipt['claims'].values()))
        for path in output.rglob('*'):
            self.assertEqual(path.stat().st_mode & 0o222, 0)
        with self.assertRaises(ValueError):
            overlay.build(self.source, stage, 'ATR-01', 'with', output)
        with self.assertRaises(ValueError):
            overlay.build(self.source, stage, 'ATR-01', 'with', stage / 'model-stage/overlay')
        target = output / 'mcp.json'
        target.chmod(0o600)
        target.write_text('{"mcpServers":{"live":{"command":"cub-scout"}}}')
        with self.assertRaises(ValueError):
            overlay.verify(self.source, stage, 'ATR-01', 'with', output)
