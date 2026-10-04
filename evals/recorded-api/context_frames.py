#!/usr/bin/env python3
"""Read one exact RUL-03 historical response; no transport or context fallback."""
import hashlib
import json
import os
from pathlib import Path
import stat

FIXTURES = Path(__file__).resolve().parents[1] / 'rul03-context/fixtures'
MAX_BYTES = 128 * 1024
REQUEST_PATH = '/apis/apps/v1/namespaces/rul03-proof/deployments'
PINS = {
    'capture-scope.json': '9b05333216fd5daa609266c67c5fee6bdd5ba82487563ff39be26fa24131dc35',
    'observer-context-map.json': '19a2b434661e8ed3d52934c69340cb8773fbd78a8416ff9e245a72e057fc7c09',
    'rul03-denied-deployments.body': 'fc5cad6498d731f454f2b859a528b00ab69450481804c89a42939512b0dc6978',
    'rul03-readable-deployments.body': 'e0102f91e1417c8554ed377352b50450f2376e69d40632f109ea012c5c1560a2',
}
CONTEXTS = frozenset(('rul03-denied', 'rul03-readable'))


class FrameError(ValueError):
    pass


def _read(path, expected):
    path = Path(path)
    try:
        if not path.is_absolute() or '..' in path.parts or any(p.is_symlink() for p in (path, *path.parents)):
            raise FrameError('unsafe source path')
        if not stat.S_ISREG(path.lstat().st_mode):
            raise FrameError('source is not a regular file')
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_BYTES:
                raise FrameError('source exceeds regular-file bound')
            raw = stream.read(MAX_BYTES + 1)
        if len(raw) > MAX_BYTES or hashlib.sha256(raw).hexdigest() != expected:
            raise FrameError('source pin differs')
        return raw
    except OSError:
        raise FrameError('source unavailable') from None


def _json(raw):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise FrameError('duplicate source property')
            result[key] = value
        return result
    def constant(_):
        raise FrameError('non-finite source value')
    try:
        return json.loads(raw.decode('utf-8', 'strict'), object_pairs_hook=unique, parse_constant=constant)
    except (UnicodeError, json.JSONDecodeError, RecursionError):
        raise FrameError('invalid source JSON') from None


def select_frame(context, method, path, *, fixture_dir=FIXTURES):
    """Return (raw response bytes, provenance) for an explicit captured request.

    fixture_dir is an offline test/source-directory override, never a network
    destination. Metadata and selected bytes must match the reviewed pins.
    Unselected bodies are not read; they cannot supply a fallback response.
    """
    if not isinstance(context, str) or context not in CONTEXTS:
        raise FrameError('exact recorded context required')
    if method != 'GET' or path != REQUEST_PATH:
        raise FrameError('request has no exact captured frame')
    root = Path(fixture_dir)
    scope = _json(_read(root / 'capture-scope.json', PINS['capture-scope.json']))
    context_map = _json(_read(root / 'observer-context-map.json', PINS['observer-context-map.json']))
    # Pins bind the complete reviewed metadata. Still check the semantic join
    # explicitly so a future pin refresh cannot silently change selection.
    if (scope.get('schema') != 'rul03-context-capture-scope.v1'
            or scope.get('atomicSnapshot') is not False
            or context_map.get('schema') != 'rul03-observer-context-map.v1'
            or set(context_map.get('contexts', {})) != CONTEXTS):
        raise FrameError('source scope differs')
    matches = [(index, row) for index, row in enumerate(scope['observations'])
               if (row.get('context'), row.get('method'), row.get('path')) == (context, method, path)]
    if len(matches) != 1:
        raise FrameError('captured request is absent or ambiguous')
    index, row = matches[0]
    filename = ('rul03-denied-deployments.body' if context == 'rul03-denied'
                else 'rul03-readable-deployments.body')
    status = 403 if context == 'rul03-denied' else 200
    if (row.get('rawFile') != filename or row.get('rawSha256') != PINS[filename]
            or row.get('httpStatus') != status or row.get('explicitContext') is not True):
        raise FrameError('response identity differs')
    body = _read(root / filename, PINS[filename])
    if len(body) != row.get('rawBytes'):
        raise FrameError('response size differs')
    return body, {
        'schema': 'recorded-context-frame.v1', 'case': 'RUL-03',
        'context': context, 'method': method, 'path': path, 'httpStatus': status,
        'endpoint': dict(context_map['contexts'][context]),
        'observerCurrentContext': context_map['currentContext'],
        'sourceRow': index, 'sourceObservation': dict(row),
        'scopeSha256': PINS['capture-scope.json'],
        'contextMapSha256': PINS['observer-context-map.json'],
        'atomicSnapshot': False, 'currentStateEstablished': False,
        'runtimeAdmission': False,
    }
