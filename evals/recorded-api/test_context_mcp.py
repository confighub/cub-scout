import io
import json
from pathlib import Path
import shutil
import tempfile
import unittest

import context_frames as frames
import context_mcp as mcp

INIT = {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
    'protocolVersion': '2025-03-26', 'capabilities': {}, 'clientInfo': {'name': 'offline-test', 'version': '1'}}}


def call(context='rul03-denied', **changes):
    args = {'context': context, 'method': 'GET', 'path': frames.REQUEST_PATH}
    args.update(changes)
    return {'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call',
            'params': {'name': 'recorded_response', 'arguments': args}}


def transport(messages, *, raw=None):
    output = io.BytesIO()
    mcp.serve(io.BytesIO(raw if raw is not None else b''.join(mcp.encoded(row) for row in messages)),
              output, fixture_dir=frames.FIXTURES)
    return [json.loads(row) for row in output.getvalue().splitlines()]


class ContextMCPTests(unittest.TestCase):
    def test_exact_response_through_stdio(self):
        for context in sorted(frames.CONTEXTS):
            with self.subTest(context=context):
                rows = transport([INIT, {'jsonrpc': '2.0', 'method': 'notifications/initialized'},
                    {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list'}, call(context)])
                self.assertEqual([row['id'] for row in rows], [1, 2, 3])
                self.assertEqual(rows[1]['result']['tools'], [mcp.TOOL])
                result = rows[2]['result']
                self.assertIs(result['isError'], False)
                payload = json.loads(result['content'][0]['text'])
                raw, provenance = frames.select_frame(context, 'GET', frames.REQUEST_PATH)
                self.assertEqual(payload, {'responseText': raw.decode(), 'provenance': provenance})
                self.assertEqual(payload['provenance']['httpStatus'], 403 if context.endswith('denied') else 200)

    def test_bad_request_cannot_fall_back_or_echo_inputs(self):
        bad = [call(''), call('current-context'), call('rul03-denied', method='POST'),
               call('rul03-denied', path=frames.REQUEST_PATH + '?limit=500'),
               call('rul03-denied', token='must-not-be-echoed')]
        bad.append({'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {'name': 'map', 'arguments': {}}})
        for row in bad:
            with self.subTest(row=row):
                response = transport([INIT, row])[-1]['result']
                self.assertEqual(response, {'isError': True, 'content': [{'type': 'text', 'text': 'No exact recorded request'}]})

    def test_envelope_initialization_and_unsupported_methods(self):
        self.assertEqual(transport([call()])[0]['error']['code'], -32000)
        self.assertEqual(transport([INIT, INIT])[-1]['error']['code'], -32602)
        self.assertEqual(transport([INIT, {'jsonrpc': '2.0', 'id': 4, 'method': 'resources/read'}])[-1]['error']['code'], -32601)
        for bad in [[], {'jsonrpc': '1.0', 'id': 4, 'method': 'tools/list'},
                    {'jsonrpc': '2.0', 'id': True, 'method': 'tools/list'},
                    {'jsonrpc': '2.0', 'id': 4, 'method': 'tools/list', 'params': []}]:
            with self.subTest(bad=bad):
                self.assertEqual(transport([bad])[0]['error']['code'], -32600)

    def test_strict_bounded_transport_and_budget(self):
        for raw in [b'{"id":1,"id":2}\n', b'{"x":NaN}\n', b'{"x":1e999}\n',
                    b'{"x":' + b'9' * 5000 + b'}\n', b'\xff\n',
                    b'{broken}\n', b'[' * 34 + b'0' + b']' * 34 + b'\n']:
            with self.subTest(raw=raw):
                self.assertEqual(transport([], raw=raw)[0]['error']['code'], -32700)
        self.assertEqual(transport([], raw=b' ' * (mcp.MAX_MESSAGE_BYTES + 1))[0]['error']['code'], -32600)
        self.assertEqual(transport([]), [])
        rows = transport([{'jsonrpc': '2.0', 'method': 'notifications/initialized'}] * mcp.MAX_MESSAGES)
        self.assertEqual(rows, [mcp.error(None, -32000, 'Message budget exhausted')])

    def test_startup_refuses_incomplete_or_changed_source(self):
        for name in frames.PINS:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as td:
                root = Path(td).resolve() / 'fixtures'
                shutil.copytree(frames.FIXTURES, root)
                (root / name).write_bytes(b'changed')
                output = io.BytesIO()
                with self.assertRaises(frames.FrameError):
                    mcp.serve(io.BytesIO(mcp.encoded(INIT)), output, fixture_dir=root)
                self.assertEqual(output.getvalue(), b'')


if __name__ == '__main__':
    unittest.main()
