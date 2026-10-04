"""Eval-only exact-context map protocol. Runtime/tool admission is separate."""
import copy
from pathlib import Path

import context_frames as frames
import context_inventory as binding
import context_mcp as protocol

TOOL = {
    'name': 'map',
    'description': ('Inspect one exact captured namespaced Deployment request by context. '
                    'Returns historical inventory with capture provenance, or unreadable coverage '
                    'and unknown inventory for a recorded denial. No current state or cross-context join.'),
    'annotations': {'readOnlyHint': True, 'destructiveHint': False, 'openWorldHint': False},
    'inputSchema': {'type': 'object', 'properties': {
        'context': {'type': 'string', 'enum': sorted(frames.CONTEXTS)}},
        'required': ['context'], 'additionalProperties': False},
}


class ContextInventoryMCP(protocol.ContextMCP):
    def __init__(self, map_reader, *, fixture_dir=Path('/evidence/cluster')):
        if not callable(map_reader):
            raise frames.FrameError('recorded inventory reader unavailable')
        # Validate all original pinned source files before publishing a tool.
        # This startup check is not a fallback: each call selects one request.
        super().__init__(fixture_dir=fixture_dir)
        self.fixture_dir = fixture_dir
        self.map_reader = map_reader

    def handle(self, request):
        # Reuse the reviewed envelope/initialization contract without permitting
        # its raw-response tool. The substitute call is always a protocol-only
        # refusal; validate the original map arguments below after that gate.
        gate = request
        if isinstance(request, dict) and request.get('method') == 'tools/call':
            gate = dict(request)
            if isinstance(request.get('params', {}), dict):
                gate['params'] = {'name': '__protocol_only__', 'arguments': {}}
        response = super().handle(gate)
        if response is None or 'error' in response:
            return response
        method = request['method']
        if method == 'initialize':
            response['result']['serverInfo'] = {'name': 'scout-context-inventory-eval', 'version': '1'}
        elif method == 'tools/list':
            response['result']['tools'] = [copy.deepcopy(TOOL)]
        elif method == 'tools/call':
            params = request.get('params', {})
            arguments = params.get('arguments')
            result = {'isError': True, 'content': [{'type': 'text', 'text': 'Exact recorded inventory request refused'}]}
            if (set(params) == {'name', 'arguments'} and params.get('name') == TOOL['name']
                    and isinstance(arguments, dict) and set(arguments) == {'context'}
                    and isinstance(arguments['context'], str) and arguments['context'] in frames.CONTEXTS):
                try:
                    payload = binding.bind_inventory(arguments['context'], 'GET', frames.REQUEST_PATH,
                                                     self.map_reader, fixture_dir=self.fixture_dir)
                    text = protocol.encoded(payload).decode('utf-8').rstrip('\n')
                    candidate = {'isError': False, 'content': [{'type': 'text', 'text': text}]}
                    # Include the JSON-RPC envelope in the outgoing byte bound.
                    if len(protocol.encoded({**response, 'result': candidate})) > protocol.MAX_MESSAGE_BYTES:
                        raise frames.FrameError('inventory response exceeds byte bound')
                    result = candidate
                except (frames.FrameError, OSError, UnicodeError):
                    pass
            response['result'] = result
        return response


def serve(stream_in, stream_out, map_reader, *, fixture_dir=Path('/evidence/cluster')):
    """Bounded byte-stream transport; injected reader is never runtime admission."""
    server = ContextInventoryMCP(map_reader, fixture_dir=fixture_dir)
    for _ in range(protocol.MAX_MESSAGES):
        raw = stream_in.readline(protocol.MAX_MESSAGE_BYTES + 1)
        if not raw:
            return
        if len(raw) > protocol.MAX_MESSAGE_BYTES:
            stream_out.write(protocol.encoded(protocol.error(None, -32600, 'Request exceeds byte bound')))
            stream_out.flush()
            return
        try:
            request = frames._json(raw)
            protocol._bounded(request)
        except frames.FrameError:
            response = protocol.error(None, -32700, 'Invalid bounded JSON')
        else:
            response = server.handle(request)
        if response is not None:
            raw_response = protocol.encoded(response)
            if len(raw_response) > protocol.MAX_MESSAGE_BYTES:
                raise frames.FrameError('response exceeds byte bound')
            stream_out.write(raw_response)
            stream_out.flush()
    stream_out.write(protocol.encoded(protocol.error(None, -32000, 'Message budget exhausted')))
    stream_out.flush()
