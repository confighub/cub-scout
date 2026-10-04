import copy
import io
import json
from pathlib import Path
import shutil
import tempfile
import unittest

import context_frames as frames
import context_mcp as protocol
import context_inventory_mcp as mcp
import test_context_inventory as fixture_tests

INIT = {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
    'protocolVersion': '2025-03-26', 'capabilities': {}, 'clientInfo': {'name': 'offline-test', 'version': '1'}}}


def call(context='rul03-denied', name='map', **extra):
    return {'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call',
            'params': {'name': name, 'arguments': {'context': context, **extra}}}


class ContextInventoryMCPTests(unittest.TestCase):
    def setUp(self):
        fixture = fixture_tests.ContextInventoryTests()
        fixture.setUp()
        self.report = fixture.report
        self.body = fixture.body
        self.calls = []

    def reader(self, body, scope):
        self.calls.append((body, scope))
        return json.dumps(self.report).encode()

    def transport(self, messages=(), *, reader=None, raw=None, fixture_dir=frames.FIXTURES):
        output = io.BytesIO()
        mcp.serve(io.BytesIO(raw if raw is not None else b''.join(protocol.encoded(row) for row in messages)),
                  output, reader or self.reader, fixture_dir=fixture_dir)
        return [json.loads(row) for row in output.getvalue().splitlines()]

    def test_catalog_readable_denied_and_notification(self):
        rows = self.transport([INIT, {'jsonrpc': '2.0', 'method': 'notifications/initialized'},
            {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list'}, call(),
            {**call('rul03-readable'), 'id': 4},
            {key: value for key, value in call('rul03-readable').items() if key != 'id'}])
        self.assertEqual([row['id'] for row in rows], [1, 2, 3, 4])
        self.assertEqual(rows[0]['result']['serverInfo']['name'], 'scout-context-inventory-eval')
        self.assertEqual(rows[1]['result']['tools'], [mcp.TOOL])
        denied = json.loads(rows[2]['result']['content'][0]['text'])
        self.assertFalse(rows[2]['result']['isError'])
        self.assertIsNone(denied['inventory'])
        self.assertEqual(denied['coverage'], 'unreadable')
        self.assertEqual(denied['provenance']['httpStatus'], 403)
        readable = json.loads(rows[3]['result']['content'][0]['text'])
        self.assertEqual(readable['inventory'], self.report)
        self.assertEqual(readable['provenance']['context'], 'rul03-readable')
        self.assertFalse(readable['runtimeAdmission'])
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(self.calls[0][0], self.body)

    def test_strict_arguments_no_fallback_or_unsupported_tools(self):
        for request in [call(''), call(None), call('current-context'), call(context=['rul03-denied']),
                call(method='GET'), call(path=frames.REQUEST_PATH), call(namespace='other'),
                call(token='private-value'), call(name='recorded_response'), call(name='explain'),
                call(name='doctor'), {**call(), 'params': {}},
                {**call(), 'params': {'name': 'map', 'arguments': {}}},
                {**call(), 'params': {'name': 'map', 'arguments': {'context': 'rul03-readable'}, 'extra': True}}]:
            with self.subTest(request=request):
                response = self.transport([INIT, request])[-1]['result']
                self.assertTrue(response['isError'])
                self.assertNotIn('private-value', json.dumps(response))
        self.assertEqual(self.calls, [])

    def test_envelope_and_initialization_refuse(self):
        self.assertEqual(self.transport([call()])[0]['error']['code'], -32000)
        self.assertEqual(self.transport([INIT, INIT])[-1]['error']['code'], -32602)
        for request in [[], {**call(), 'id': True}, {**call(), 'jsonrpc': '1.0'},
                {**call(), 'params': []}, {**call(), 'extra': True}]:
            with self.subTest(request=request):
                self.assertEqual(self.transport([INIT, request])[-1]['error']['code'], -32600)
        self.assertEqual(self.calls, [])

    def test_reader_failure_foreign_output_and_outgoing_limit(self):
        def fail(*args):
            raise RuntimeError('private-detail')
        foreign = copy.deepcopy(self.report)
        foreign['provenance']['sha256'] = '0' * 64
        large = copy.deepcopy(self.report)
        large['resources'][0]['ownershipDetection']['reason'] = '"' * 40000
        for reader in [fail, lambda *_: json.dumps(foreign).encode(), lambda *_: json.dumps(large).encode()]:
            with self.subTest(reader=reader):
                response = self.transport([INIT, call('rul03-readable')], reader=reader)[-1]['result']
                self.assertTrue(response['isError'])
                self.assertNotIn('private-detail', json.dumps(response))
        self.assertEqual(self.calls, [])

    def test_strict_transport_and_message_budget(self):
        for raw in [b'{"id":1,"id":2}\n', b'{"x":NaN}\n', b'{"x":1e999}\n',
                b'\xff\n', b'[' * 34 + b'0' + b']' * 34 + b'\n']:
            with self.subTest(raw=raw):
                self.assertEqual(self.transport(raw=raw)[0]['error']['code'], -32700)
        self.assertEqual(self.transport(raw=b' ' * (protocol.MAX_MESSAGE_BYTES + 1))[0]['error']['code'], -32600)
        rows = self.transport([{'jsonrpc': '2.0', 'method': 'notifications/initialized'}] * protocol.MAX_MESSAGES)
        self.assertEqual(rows, [protocol.error(None, -32000, 'Message budget exhausted')])
        self.assertEqual(self.calls, [])

    def test_startup_source_and_reader_refusal(self):
        for name in frames.PINS:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as td:
                root = Path(td).resolve() / 'fixtures'
                shutil.copytree(frames.FIXTURES, root)
                (root / name).write_bytes(b'drift')
                output = io.BytesIO()
                with self.assertRaises(frames.FrameError):
                    mcp.serve(io.BytesIO(protocol.encoded(INIT)), output, self.reader, fixture_dir=root)
                self.assertEqual(output.getvalue(), b'')
        with self.assertRaises(frames.FrameError):
            mcp.ContextInventoryMCP(None, fixture_dir=frames.FIXTURES)
        self.assertEqual(self.calls, [])
