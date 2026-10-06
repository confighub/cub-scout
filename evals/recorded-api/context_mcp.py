#!/usr/bin/env python3
"""Eval-only MCP stdio transport for exact captured responses. No live access."""
import copy
import json
from pathlib import Path
import sys

import context_frames as frames

MAX_MESSAGE_BYTES = 128 * 1024
MAX_MESSAGES = 64
MAX_DEPTH = 32
VERSIONS = ('2025-03-26', '2024-11-05', '2025-06-18')
TOOL = {
    'name': 'recorded_response',
    'description': ('Read one exact historical HTTP response by explicit context, GET and raw path. '
                    'Returns original response text and capture provenance, including 403 denial. '
                    'Sequential non-atomic evidence; no current inventory, ownership or health inference.'),
    'annotations': {'readOnlyHint': True, 'destructiveHint': False, 'openWorldHint': False},
    'inputSchema': {
        'type': 'object', 'properties': {
            'context': {'type': 'string', 'enum': sorted(frames.CONTEXTS)},
            'method': {'type': 'string', 'enum': ['GET']},
            'path': {'type': 'string', 'enum': [frames.REQUEST_PATH]},
        }, 'required': ['context', 'method', 'path'], 'additionalProperties': False,
    },
}


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, separators=(',', ':'), allow_nan=False) + '\n').encode('utf-8')


def _bounded(value, depth=0):
    if depth > MAX_DEPTH:
        raise frames.FrameError('request exceeds nesting bound')
    if isinstance(value, dict):
        for item in value.values():
            _bounded(item, depth + 1)
    elif isinstance(value, list):
        for item in value:
            _bounded(item, depth + 1)


def _id(value):
    return ((type(value) is int and -(2**53) < value < 2**53)
            or (isinstance(value, str) and 0 < len(value) <= 256))


def error(identifier, code, message):
    return {'jsonrpc': '2.0', 'id': identifier, 'error': {'code': code, 'message': message}}


class ContextMCP:
    def __init__(self, *, fixture_dir=Path('/evidence/cluster')):
        # Refuse incomplete or changed source before publishing any tools.
        self.frames = {context: frames.select_frame(context, 'GET', frames.REQUEST_PATH,
                                                    fixture_dir=fixture_dir)
                       for context in sorted(frames.CONTEXTS)}
        self.initialized = False

    def handle(self, request):
        identifier = request.get('id') if isinstance(request, dict) else None
        safe_id = identifier if _id(identifier) else None
        if (not isinstance(request, dict) or set(request) - {'jsonrpc', 'id', 'method', 'params'}
                or request.get('jsonrpc') != '2.0' or not isinstance(request.get('method'), str)
                or ('id' in request and not _id(identifier))
                or not isinstance(request.get('params', {}), dict)):
            return error(safe_id, -32600, 'Invalid request')
        method, params = request['method'], request.get('params', {})
        if 'id' not in request:
            # JSON-RPC notifications have no response. Only the conventional
            # initialized notification is recognized; none executes a tool.
            return None
        if method == 'initialize':
            if self.initialized or params.get('protocolVersion') not in VERSIONS:
                return error(safe_id, -32602, 'Unsupported initialization')
            if (set(params) != {'protocolVersion', 'capabilities', 'clientInfo'}
                    or not isinstance(params['capabilities'], dict)
                    or not isinstance(params['clientInfo'], dict)):
                return error(safe_id, -32602, 'Invalid initialization')
            self.initialized = True
            result = {'protocolVersion': params['protocolVersion'], 'capabilities': {'tools': {}},
                      'serverInfo': {'name': 'scout-recorded-response-eval', 'version': '1'}}
        elif not self.initialized:
            return error(safe_id, -32000, 'Initialization required')
        elif method == 'tools/list':
            if params:
                return error(safe_id, -32602, 'Invalid tool list parameters')
            result = {'tools': [copy.deepcopy(TOOL)]}
        elif method == 'tools/call':
            arguments = params.get('arguments')
            if (set(params) != {'name', 'arguments'} or params.get('name') != TOOL['name']
                    or not isinstance(arguments, dict) or set(arguments) != {'context', 'method', 'path'}
                    or not isinstance(arguments.get('context'), str)
                    or arguments['context'] not in self.frames
                    or arguments.get('method') != 'GET' or arguments.get('path') != frames.REQUEST_PATH):
                result = {'isError': True, 'content': [{'type': 'text', 'text': 'No exact recorded request'}]}
            else:
                body, provenance = self.frames[arguments['context']]
                payload = {'responseText': body.decode('utf-8', 'strict'), 'provenance': provenance}
                # HTTP denial is source data; retrieving it succeeded. Never
                # turn it into an empty inventory or a fabricated API failure.
                result = {'isError': False, 'content': [{'type': 'text', 'text': encoded(payload).decode().rstrip('\n')}]}
        else:
            return error(safe_id, -32601, 'Method not found')
        return {'jsonrpc': '2.0', 'id': safe_id, 'result': result}


def serve(stream_in, stream_out, *, fixture_dir=Path('/evidence/cluster')):
    server = ContextMCP(fixture_dir=fixture_dir)
    for _ in range(MAX_MESSAGES):
        raw = stream_in.readline(MAX_MESSAGE_BYTES + 1)
        if not raw:
            return
        if len(raw) > MAX_MESSAGE_BYTES:
            stream_out.write(encoded(error(None, -32600, 'Request exceeds byte bound')))
            stream_out.flush()
            return
        try:
            request = frames._json(raw)
            _bounded(request)
        except frames.FrameError:
            response = error(None, -32700, 'Invalid bounded JSON')
        else:
            response = server.handle(request)
        if response is not None:
            payload = encoded(response)
            if len(payload) > MAX_MESSAGE_BYTES:
                raise frames.FrameError('response exceeds byte bound')
            stream_out.write(payload)
            stream_out.flush()
    # Explicit refusal once the invocation's bounded message budget is spent.
    stream_out.write(encoded(error(None, -32000, 'Message budget exhausted')))
    stream_out.flush()


def main():
    if len(sys.argv) != 1:
        raise frames.FrameError('no command-line overrides accepted')
    serve(sys.stdin.buffer, sys.stdout.buffer)


if __name__ == '__main__':
    try:
        main()
    except (frames.FrameError, OSError, UnicodeError):
        print('recorded response MCP refused', file=sys.stderr)
        raise SystemExit(2)
