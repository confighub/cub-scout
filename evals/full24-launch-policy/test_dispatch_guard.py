"""Authored hook events only; never execute a model or requested tool."""
import json
from pathlib import Path
import subprocess
import sys
import unittest

import dispatch_guard as guard
import policy
import test_policy


class DispatchGuardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        test_policy.PolicyTests.setUpClass()
        cls.fixture = test_policy.PolicyTests()
        cls.root = test_policy.PolicyTests.root

    @classmethod
    def tearDownClass(cls):
        test_policy.PolicyTests.tearDownClass()

    def documents(self, cid='ATR-01', arm='with'):
        _, plan = self.fixture.stage(cid, arm)
        binding = guard.binding_for(plan)
        selection_sha = guard.digest(guard.encoded(plan['selection']))
        return plan, binding, selection_sha

    def call(self, plan, binding, selection_sha, tool, **extra):
        event = {'hook_event_name': 'PreToolUse', 'cwd': '/evidence',
                 'tool_name': tool, 'tool_input': {}, 'tool_use_id': 'authored-1', **extra}
        return guard.decide(guard.encoded(binding), guard.encoded(plan), guard.encoded(event),
                            plan['selection']['case'], plan['selection']['arm'], selection_sha)

    def test_all24_both_arm_exact_grants_and_no_admission_claim(self):
        for cid in policy.stager.prepare.FROZEN_CASE_IDS:
            for arm in ('with', 'without'):
                plan, binding, selection_sha = self.documents(cid, arm)
                for tool in plan['ordinaryToolGrant'] + guard.MCP + ['Bash', 'Write', 'Edit', 'Task', 'Agent', '*',
                    'mcp__cub-scout__doctor', 'mcp__other__map', 'mcp__cub-scout__map ', 'read']:
                    if plan['status'] != 'candidate_not_executed':
                        with self.assertRaises(ValueError):
                            self.call(plan, binding, selection_sha, tool)
                    else:
                        expected = tool in plan['ordinaryToolGrant'] or arm == 'with' and tool in guard.MCP
                        self.assertEqual(self.call(plan, binding, selection_sha, tool), expected, (cid, arm, tool))
                self.assertFalse(plan['runtimeAssetAdmission']['dispatchGuard']['runtimeInvoked'])
                self.assertIsNone(plan['runtimeAssetAdmission']['python']['trustedSha256'])
                self.assertTrue(all(v is False for v in plan['claims'].values()))
                settings = policy.dispatch_settings(plan['selection'])
                self.assertEqual(settings['hooks']['PreToolUse'][0]['matcher'], '.*')
                self.assertIn(selection_sha, settings['hooks']['PreToolUse'][0]['hooks'][0]['command'])

    def test_binding_selection_policy_and_control_corruption_refuse(self):
        plan, binding, selection_sha = self.documents()
        for key, value in [('schema', 'unknown'), ('selection', {'case': 'ATR-02', 'arm': 'with'}),
                           ('launchPolicySha256', '0' * 64), ('ordinaryToolGrant', ['Bash']),
                           ('recordedTools', ['mcp__cub-scout__doctor']), ('extra', True)]:
            with self.assertRaises(ValueError):
                self.call(plan, {**binding, key: value}, selection_sha, 'Read')
        for key, value in [('ordinaryToolGrant', ['Read', 'Bash']), ('schema', 'unknown'),
                           ('status', 'blocked_recorded_mcp_binding'), ('budget', {})]:
            changed = {**plan, key: value}
            with self.assertRaises(ValueError):
                self.call(changed, binding, selection_sha, 'Read')
        for selection in ({**plan['selection'], 'case': 'ATR-02'},
                          {**plan['selection'], 'arm': 'without'},
                          {**plan['selection'], 'authoredInputControl': 'substituted-control'}):
            changed = {**plan, 'selection': selection}
            with self.assertRaises(ValueError):
                self.call(changed, guard.binding_for(changed), selection_sha, 'Read')
        for cid in ('DEL-03', 'DEL-04'):
            control = self.fixture.report['authoredInputControls'][cid]['id']
            _, authored = self.fixture.stage(cid, 'without', control)
            authored_sha = guard.digest(guard.encoded(authored['selection']))
            self.assertTrue(self.call(authored, guard.binding_for(authored), authored_sha, 'Read'))
            _, _, original_sha = self.documents(cid, 'without')
            with self.assertRaises(ValueError):
                self.call(authored, guard.binding_for(authored), original_sha, 'Read')

    def test_malformed_duplicate_nonfinite_and_incomplete_events_refuse(self):
        plan, binding, selection_sha = self.documents()
        for raw in (b'{', b'null', b'[]', b'\xff', b' ' * (guard.MAX_EVENT + 1),
                    b'{"hook_event_name":"PreToolUse","tool_name":"Read","tool_name":"Bash"}',
                    b'{"value":NaN}', b'{"value":1e9999}', b'[' * 2000 + b']' * 2000):
            with self.assertRaises((ValueError, UnicodeError)):
                guard.decide(guard.encoded(binding), guard.encoded(plan), raw, 'ATR-01', 'with', selection_sha)
        for extra in ({'hook_event_name': 'PostToolUse'}, {'cwd': '/sibling'}, {'tool_input': []},
                      {'tool_use_id': ''}, {'tool_name': None}):
            with self.assertRaises(ValueError):
                self.call(plan, binding, selection_sha, 'Read', **extra)

    def test_denial_stdio_never_echoes_input_and_never_asks_or_allows(self):
        secret = 'authored-private-input-do-not-echo'
        child = subprocess.run([sys.executable, str(Path(guard.__file__).resolve())],
                               input=json.dumps({'tool_name': 'Bash', 'tool_input': {'command': secret}}).encode(),
                               capture_output=True, timeout=5, env={'PATH': '/usr/bin:/bin', 'PYTHONDONTWRITEBYTECODE': '1'})
        self.assertEqual(child.returncode, 0)
        self.assertEqual(child.stderr, b'')
        self.assertNotIn(secret.encode(), child.stdout)
        self.assertEqual(json.loads(child.stdout), guard.denial())
        self.assertEqual(guard.denial()['hookSpecificOutput']['permissionDecision'], 'deny')


if __name__ == '__main__':
    unittest.main()
