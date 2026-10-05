"""Evaluate the workflow's job-selection graph without running any job."""
import ast
from pathlib import Path
import unittest

import yaml

WORKFLOW = Path(__file__).resolve().parents[2] / '.github/workflows/ci.yaml'


def condition(source, event, level):
    if source is None:
        return True
    if source == 'always()':
        return True
    tree = ast.parse(source.replace('&&', ' and ').replace('||', ' or '), mode='eval')
    values = {'github.event_name': event, 'github.event.inputs.level': level}

    def visit(node):
        if isinstance(node, ast.Expression):
            return visit(node.body)
        if isinstance(node, ast.Constant) and isinstance(node.value, (str, bool)):
            return node.value
        if isinstance(node, ast.Attribute):
            parts = []
            while isinstance(node, ast.Attribute):
                parts.insert(0, node.attr)
                node = node.value
            if not isinstance(node, ast.Name):
                raise ValueError('unsupported workflow variable')
            return values['.'.join([node.id] + parts)]
        if isinstance(node, ast.BoolOp):
            if isinstance(node.op, ast.And):
                return all(visit(value) for value in node.values)
            if isinstance(node.op, ast.Or):
                return any(visit(value) for value in node.values)
        if isinstance(node, ast.Compare) and len(node.ops) == len(node.comparators) == 1:
            left, right = visit(node.left), visit(node.comparators[0])
            if isinstance(node.ops[0], ast.Eq):
                return left == right
            if isinstance(node.ops[0], ast.NotEq):
                return left != right
        raise ValueError('unsupported workflow condition syntax')

    return bool(visit(tree))


def scheduled(jobs, event, level):
    outcomes = {}
    while len(outcomes) < len(jobs):
        previous = len(outcomes)
        for name, job in jobs.items():
            needs = job.get('needs', [])
            needs = [needs] if isinstance(needs, str) else needs
            if name in outcomes or any(dependency not in outcomes for dependency in needs):
                continue
            selected = condition(job.get('if'), event, level)
            outcomes[name] = selected and (job.get('if') == 'always()' or all(outcomes[n] for n in needs))
        if len(outcomes) == previous:
            raise ValueError('unresolved workflow dependency graph')
    return {name for name, selected in outcomes.items() if selected}


class OfflineWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = yaml.safe_load(WORKFLOW.read_text())
        cls.jobs = cls.workflow['jobs']

    def test_manual_offline_and_unknown_levels_never_schedule_cluster_jobs(self):
        for level in ('smoke', 'unit', '', 'typo'):
            with self.subTest(level=level):
                self.assertEqual(scheduled(self.jobs, 'workflow_dispatch', level), {'unit', 'proof-artifact'})

    def test_existing_live_levels_and_automatic_events_keep_their_jobs(self):
        base = {'unit', 'integration', 'gitops', 'proof-artifact'}
        for event in ('push', 'pull_request'):
            self.assertEqual(scheduled(self.jobs, event, ''), base)
        for level in ('integration', 'gitops', 'demos', 'connected', 'full'):
            extra = {'demos'} if level == 'demos' else {'connected'} if level == 'connected' else {'demos', 'connected', 'full'} if level == 'full' else set()
            with self.subTest(level=level):
                self.assertEqual(scheduled(self.jobs, 'workflow_dispatch', level), base | extra)

    def test_dispatch_choices_are_explicit_and_unit_remains_available(self):
        # PyYAML's YAML 1.1 resolver treats the Actions key 'on' as boolean True.
        trigger = self.workflow.get('on', self.workflow.get(True))
        option = trigger['workflow_dispatch']['inputs']['level']
        self.assertEqual(option['type'], 'choice')
        self.assertEqual(set(option['options']), {'smoke', 'unit', 'integration', 'gitops', 'demos', 'connected', 'full'})
        self.assertEqual(option['default'], 'integration')

    def test_required_connected_tests_cannot_be_conditional_on_a_secret(self):
        steps = self.jobs['connected']['steps']
        for name in ('Provision authenticated disposable ConfigHub', 'Run import round-trip tests'):
            step = next(s for s in steps if s.get('name') == name)
            self.assertNotIn('if', step)
            self.assertNotIn('continue-on-error', step)
        roundtrip = next(s for s in steps if s.get('name') == 'Run import round-trip tests')['run']
        self.assertIn('require_test_passes.py', roundtrip)
        self.assertIn('-count=1', roundtrip)
        for test in ('TestImportFullRoundTrip', 'TestImportIdempotent', 'TestImportCleanup'):
            self.assertIn(test, roundtrip)

    def test_full_acceptance_requires_all_providers_before_execution(self):
        steps = self.jobs['full']['steps']
        acceptance = next(i for i, step in enumerate(steps) if step.get('name') == 'Run prove-it-works')
        for provider in ('setup-scan-provider.sh', 'setup-gitops-controllers.sh', 'setup-connected-server.sh'):
            index = next(i for i, step in enumerate(steps) if provider in step.get('run', ''))
            self.assertLess(index, acceptance)
            self.assertNotIn('if', steps[index])
            self.assertNotIn('continue-on-error', steps[index])

    def test_gitops_and_demo_acceptance_do_not_mask_failure(self):
        for job in ('gitops', 'demos'):
            steps = self.jobs[job]['steps']
            self.assertTrue(any('setup-gitops-controllers.sh' in s.get('run', '') for s in steps))
            for step in steps:
                with self.subTest(job=job, step=step.get('name')):
                    self.assertNotIn('continue-on-error', step)
                    self.assertNotRegex(step.get('run', ''), r'\|\|\s*(true|echo)\b')
                    self.assertNotIn('./cub-scout demo ', step.get('run', ''))


if __name__ == '__main__':
    unittest.main()
