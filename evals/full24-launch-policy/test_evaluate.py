"""Offline stream/selected-grader controls. Authored traces, never model runs."""
import copy
import json
from pathlib import Path
import re
import sys
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import evaluate
import test_policy

base = test_policy.policy
loader = test_policy.load
preflight = base.REPO / 'evals/full24-pair-preflight'
delivery = loader('terminal_delivery_helpers', preflight / 'test_delivery_graders.py')
remaining = loader('terminal_remaining_helpers', preflight / 'remaining_grader_helpers.py')


def trace(answer, **extra):
    result = {'type': 'result', 'subtype': 'success', 'is_error': False, 'num_turns': 1,
              'result': answer, 'total_cost_usd': 0.0123, 'usage': {'input_tokens': 12}, **extra}
    return (json.dumps(result) + '\n').encode()


class TerminalEvaluationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        test_policy.PolicyTests.setUpClass()
        cls.fixture = test_policy.PolicyTests()
        cls.source = test_policy.PolicyTests.source
        cls.report = test_policy.PolicyTests.report
        cls.node = delivery._find_node()
        if not cls.node:
            raise AssertionError('Existing Node required; install nothing')
        delivery.DeliveryGraderControls.out = cls.source
        cls.answers = remaining.EvidenceAnswers(cls.source, base.REPO, base.stager.prepare._yaml_load)
        cls.graders = {}
        for row in cls.report['cases']:
            path = cls.source / 'oracle' / row['id'] / 'selected-graders' / row['selectedAnswerGraders'][0]['name']
            cls.graders[row['id']] = path.read_bytes()

    @classmethod
    def tearDownClass(cls):
        test_policy.PolicyTests.tearDownClass()

    def answer(self, cid):
        if cid in base.stager.prepare.LEGACY_CASES:
            return base.stager.prepare._strict_modules()['legacy'].CASES[cid]['answer']
        if cid == 'INV-01':
            return 'COUNTS: flux=120 argocd=90 helm=45 confighub=33 unmanaged=12'
        if cid == 'INV-02':
            return 'UNMANAGED: ' + ', '.join(base.stager.prepare._strict_modules()['scale'].UNMANAGED_NAMES)
        if cid.startswith('DEL-'):
            value = delivery.DeliveryGraderControls._answer(cid)
        elif cid == 'PRE-02':
            helper = loader('terminal_pre02_vectors', base.REPO / 'evals/pre02-node-selector/test_package.py')
            value = helper.EXPECTED
        elif cid == 'RUL-01':
            # This fixed literal-regex control is an authored engine vector,
            # not a new factual oracle or model evidence. Its source is pinned.
            pattern = evaluate.definition(self.graders[cid])['pattern']
            return re.sub(r'\\([{}|.+])', r'\1', pattern[1:-1])
        else:
            value = self.answers.answer(cid)
        return json.dumps(value, separators=(',', ':'))

    def test_all24_exact_selected_terminal_answers_and_intermediate_text_exclusion(self):
        self.assertEqual(len(self.graders), 24)
        for cid, grader in self.graders.items():
            with self.subTest(case=cid):
                answer = self.answer(cid)
                positive = evaluate.evaluate_bytes(trace(answer), grader, self.node, max_turns=30)
                self.assertEqual(positive['outcome'], 'pass', positive)
                self.assertEqual(positive['reportedTerminalMetadata']['total_cost_usd'], 0.0123)
                self.assertTrue(all(v is False for v in positive['claims'].values()))
                intermediate = json.dumps({'type': 'assistant', 'message': {'content': [{'type': 'text', 'text': answer}]}}).encode() + b'\n'
                negative = evaluate.evaluate_bytes(intermediate + trace('WRONG'), grader, self.node, max_turns=30)
                self.assertEqual(negative['outcome'], 'fail')

    def test_partial_duplicate_malformed_ambiguous_terminal_streams_stay_unknown(self):
        good = trace(self.answer('ATR-01'))
        bad = [b'', b'not-json', b'[]', b'{}', b'\xff', good + good,
               good + b'{"type":"assistant"}\n',
               good.replace(b'"num_turns": 1', b'"num_turns": true'),
               good.replace(b'"num_turns": 1', b'"num_turns": 0'),
               good.replace(b'"is_error": false', b'"is_error": null'),
               good.replace(b'"subtype": "success", ', b''),
               good.replace(b'"type": "result"', b'"type":"result","type":"result"'),
               b'{"type":"result","total_cost_usd":NaN}',
               good.replace(b'0.0123', b'1e999'), b'x' * (evaluate.MAX_LINE + 1)]
        for raw in bad:
            with self.subTest(raw=raw[:70]):
                result = evaluate.evaluate_bytes(raw, self.graders['ATR-01'], self.node, max_turns=20)
                self.assertEqual(result['outcome'], 'unknown', result)
                self.assertFalse(result['claims']['costReconciled'])

    def test_error_budget_and_grader_failure_retain_metadata_and_never_claim_billing(self):
        grader = self.graders['ATR-01']
        for raw in (trace('', is_error=True, subtype='error_max_turns'),
                    trace(self.answer('ATR-01'), num_turns=21)):
            with patch.object(evaluate, 'node_grade', side_effect=AssertionError('must not grade')):
                result = evaluate.evaluate_bytes(raw, grader, self.node, max_turns=20,
                                                 grade=lambda *args: self.fail('must not grade'))
            self.assertEqual(result['outcome'], 'fail')
            self.assertEqual(result['reportedTerminalMetadata']['usage'], {'input_tokens': 12})
        result = evaluate.evaluate_bytes(trace(self.answer('ATR-01')), grader, self.node, max_turns=20,
                                         grade=lambda *args: 'truthy')
        self.assertEqual(result['outcome'], 'unknown')

    def test_authored_controls_use_unweighted_acceptance_not_original_grader(self):
        for cid in ('DEL-03', 'DEL-04'):
            acceptance = (self.source / 'authored-input-controls' / cid / 'oracle/acceptance.json').read_bytes()
            expected = json.loads(acceptance)
            result = evaluate.evaluate_bytes(trace(json.dumps(expected)), acceptance, self.node, max_turns=20, control=True)
            self.assertEqual(result['outcome'], 'pass')
            self.assertEqual(evaluate.evaluate_bytes(trace(self.answer(cid)), acceptance, self.node, max_turns=20, control=True)['outcome'], 'fail')
            wrong = copy.deepcopy(expected)
            wrong[next(iter(wrong))] = 'WRONG'
            self.assertEqual(evaluate.evaluate_bytes(trace(json.dumps(wrong)), acceptance, self.node, max_turns=20, control=True)['outcome'], 'fail')

    def test_public_adapter_binds_exact_stage_selection_source_and_trace(self):
        stage, launch = self.fixture.stage('ATR-01', 'without')
        trace_path = stage.parent / 'selected-trace.jsonl'
        trace_path.write_bytes(trace(self.answer('ATR-01')))
        binding_path = stage.parent / 'attempt-binding.json'
        binding = {'schema': 'full24-attempt-binding.v1', 'selection': launch['selection'],
                   'sourcePreparationReportSha256': launch['sourcePreparationReportSha256'],
                   'stageReceiptSha256': base.sha((stage / 'receipt.json').read_bytes()),
                   'traceSha256': base.sha(trace_path.read_bytes())}
        binding_path.write_text(json.dumps(binding))
        result = evaluate.evaluate(self.source, stage, 'ATR-01', 'without', trace_path, self.node, binding_path)
        self.assertEqual(result['outcome'], 'pass')
        for field in ('traceSha256', 'sourcePreparationReportSha256', 'stageReceiptSha256'):
            changed = {**binding, field: '0' * 64}
            binding_path.write_text(json.dumps(changed))
            with self.subTest(field=field), self.assertRaises(ValueError):
                evaluate.evaluate(self.source, stage, 'ATR-01', 'without', trace_path, self.node, binding_path)
