import copy
import hashlib
import json
from pathlib import Path
import shutil
import tempfile
import unittest

import context_frames as frames
import context_inventory as binding


class ContextInventoryTests(unittest.TestCase):
    def setUp(self):
        self.body, _ = frames.select_frame('rul03-readable', 'GET', frames.REQUEST_PATH)
        self.report = {
            'schema': 'map-list-recorded.v1',
            'provenance': {'kind': 'kubernetes-object-recording',
                'sha256': hashlib.sha256(self.body).hexdigest(), 'bytes': len(self.body),
                'documents': 1, 'objectCount': 1, 'captureTime': 'unknown',
                'captureCompleteness': 'unknown', 'typedListDerivedObjects': 1},
            'scope': dict(binding.SCOPE), 'selectedCount': 1, 'excludedFromScopeCount': 0,
            'ownerCounts': {'Native': 1}, 'resources': [{'apiVersion': 'apps/v1',
                'kind': 'Deployment', 'namespace': 'rul03-proof', 'name': 'rul03-probe',
                'owner': 'Native', 'ownershipDetection': {'status': 'no_known_marker',
                    'reason': 'no_supported_marker_observed_on_returned_object'}}],
        }

    def bind(self, reader, context='rul03-readable', **kwargs):
        return binding.bind_inventory(context, 'GET', frames.REQUEST_PATH, reader, **kwargs)

    def test_readable_exact_bytes_scope_and_provenance(self):
        calls = []
        def reader(body, scope):
            calls.append((body, scope))
            return json.dumps(self.report).encode()
        result = self.bind(reader)
        self.assertEqual(calls, [(self.body, binding.SCOPE)])
        self.assertEqual(result['inventory'], self.report)
        self.assertEqual(result['coverage'], 'recorded-response')
        self.assertEqual(result['provenance']['context'], 'rul03-readable')
        self.assertEqual(result['provenance']['sourceRow'], 1)
        self.assertEqual(result['provenance']['httpStatus'], 200)
        self.assertFalse(result['provenance']['atomicSnapshot'])
        self.assertFalse(result['currentStateEstablished'])
        self.assertFalse(result['runtimeAdmission'])

    def test_denied_never_calls_reader_or_reads_other_context(self):
        def reader(*args):
            self.fail('denial must not invoke inventory reader')
        with tempfile.TemporaryDirectory() as td:
            root = Path(td).resolve() / 'fixtures'
            shutil.copytree(frames.FIXTURES, root)
            (root / 'rul03-readable-deployments.body').unlink()
            result = self.bind(reader, 'rul03-denied', fixture_dir=root)
        self.assertIsNone(result['inventory'])
        self.assertEqual(result['coverage'], 'unreadable')
        self.assertEqual(result['provenance']['context'], 'rul03-denied')
        self.assertEqual(result['provenance']['httpStatus'], 403)
        self.assertEqual(result['provenance']['observerCurrentContext'], 'rul03-readable')
        self.assertNotIn('selectedCount', result)

    def test_foreign_source_scope_identity_and_counts_refuse(self):
        variants = [
            ('provenance', 'sha256', '0' * 64), ('provenance', 'bytes', 1),
            ('provenance', 'documents', True), ('provenance', 'objectCount', False),
            ('provenance', 'typedListDerivedObjects', True),
            ('provenance', 'captureTime', 'now'), ('scope', 'namespace', 'other'),
            ('scope', 'context', 'rul03-denied'), (None, 'selectedCount', True),
            (None, 'excludedFromScopeCount', False), (None, 'schema', 'map-list-recorded-summary.v1'),
            (None, 'resources', []), (None, 'ownerCounts', {'Native': True}),
            (None, 'ownerCounts', {'Native': 0}), (None, 'currentStateEstablished', True),
        ]
        for parent, key, value in variants:
            with self.subTest(parent=parent, key=key, value=value):
                report = copy.deepcopy(self.report)
                (report[parent] if parent else report)[key] = value
                with self.assertRaises(frames.FrameError):
                    self.bind(lambda *_: json.dumps(report).encode())
        for key, value in [('namespace', 'other'), ('name', 'other'), ('kind', 'Pod'),
                           ('owner', []), ('ownershipDetection', None), ('health', 'healthy')]:
            with self.subTest(key=key):
                report = copy.deepcopy(self.report)
                report['resources'][0][key] = value
                with self.assertRaises(frames.FrameError):
                    self.bind(lambda *_: json.dumps(report).encode())

    def test_malformed_bounded_report_refuses(self):
        raw_cases = [b'null', b'[]', b'{', b'{"schema":1,"schema":2}',
            b'{"x":NaN}', b'{"x":1e999}', b'\xff',
            b'x' * (binding.MAX_REPORT_BYTES + 1),
            b'[' * 34 + b'0' + b']' * 34, self.report, b'']
        for raw in raw_cases:
            with self.subTest(raw_type=type(raw), size=len(raw)), self.assertRaises(frames.FrameError):
                self.bind(lambda *_: raw)

    def test_missing_or_failed_reader_no_retry(self):
        with self.assertRaises(frames.FrameError):
            self.bind(None)
        calls = []
        def reader(*args):
            calls.append(args)
            raise RuntimeError('private failure detail')
        with self.assertRaisesRegex(frames.FrameError, '^recorded inventory reader failed$'):
            self.bind(reader)
        self.assertEqual(len(calls), 1)

    def test_bad_selection_or_source_refuses_before_reader(self):
        def reader(*args):
            self.fail('invalid source must not reach reader')
        for context, method, path in [('', 'GET', frames.REQUEST_PATH),
                ('rul03-readable', 'POST', frames.REQUEST_PATH),
                ('rul03-readable', 'GET', frames.REQUEST_PATH + '?limit=1')]:
            with self.assertRaises(frames.FrameError):
                binding.bind_inventory(context, method, path, reader)
        with tempfile.TemporaryDirectory() as td:
            root = Path(td).resolve() / 'fixtures'
            shutil.copytree(frames.FIXTURES, root)
            with (root / 'rul03-readable-deployments.body').open('ab') as out:
                out.write(b' ')
            with self.assertRaises(frames.FrameError):
                self.bind(reader, fixture_dir=root)
            (root / 'rul03-readable-deployments.body').unlink()
            with self.assertRaises(frames.FrameError):
                self.bind(reader, fixture_dir=root)
