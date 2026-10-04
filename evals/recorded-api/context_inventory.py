"""Eval-only binding of one captured request to Scout recorded inventory.

No executable, listener, live client or context default. The injected reader is
an offline recorded-map boundary, not proof of binary/runtime admission.
"""
import hashlib

import context_frames as frames

MAX_REPORT_BYTES = 128 * 1024
MAX_DEPTH = 32
SCOPE = {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'namespace': 'rul03-proof'}
OWNERS = frozenset(('Flux', 'ArgoCD', 'Sveltos', 'Modelplane', 'Crossplane', 'kro',
                    'Helm', 'Terraform', 'ConfigHub', 'Kubernetes', 'Native'))


def _bounded(value, depth=0):
    if depth > MAX_DEPTH:
        raise frames.FrameError('inventory exceeds nesting bound')
    if isinstance(value, dict):
        for child in value.values():
            _bounded(child, depth + 1)
    elif isinstance(value, list):
        for child in value:
            _bounded(child, depth + 1)


def _validate(raw, body):
    if not isinstance(raw, bytes) or len(raw) > MAX_REPORT_BYTES:
        raise frames.FrameError('inventory must be bounded JSON bytes')
    report = frames._json(raw)
    _bounded(report)
    source = frames._json(body)
    identities = sorted((source['apiVersion'], 'Deployment', item['metadata']['namespace'],
                         item['metadata']['name']) for item in source['items'])
    count = len(identities)
    derived = sum('apiVersion' not in item or 'kind' not in item for item in source['items'])
    expected_provenance = {
        'kind': 'kubernetes-object-recording', 'sha256': hashlib.sha256(body).hexdigest(),
        'bytes': len(body), 'documents': 1, 'objectCount': count,
        'captureTime': 'unknown', 'captureCompleteness': 'unknown',
    }
    if derived:
        expected_provenance['typedListDerivedObjects'] = derived
    if (not isinstance(report, dict) or set(report) != {
            'schema', 'provenance', 'scope', 'selectedCount', 'excludedFromScopeCount',
            'ownerCounts', 'resources'}
            or report['schema'] != 'map-list-recorded.v1' or report['scope'] != SCOPE
            or report['provenance'] != expected_provenance
            or type(report['selectedCount']) is not int or report['selectedCount'] != count
            or type(report['excludedFromScopeCount']) is not int or report['excludedFromScopeCount'] != 0
            or not isinstance(report['resources'], list) or len(report['resources']) != count
            or not isinstance(report['ownerCounts'], dict)):
        raise frames.FrameError('inventory source or scope differs')
    # bool is not an acceptable integer count, even though Python compares it
    # equal to 0/1. Provenance numeric fields follow the same exact rule.
    for key in ('bytes', 'documents', 'objectCount', 'typedListDerivedObjects'):
        if key in report['provenance'] and type(report['provenance'][key]) is not int:
            raise frames.FrameError('inventory count type differs')
    actual, owners = [], {}
    for row in report['resources']:
        if (not isinstance(row, dict) or set(row) - {
                'apiVersion', 'kind', 'namespace', 'name', 'owner', 'ownerDetails', 'ownershipDetection'}
                or not {'apiVersion', 'kind', 'namespace', 'name', 'owner', 'ownershipDetection'} <= set(row)
                or not isinstance(row['owner'], str) or row['owner'] not in OWNERS
                or not isinstance(row['ownershipDetection'], dict)):
            raise frames.FrameError('inventory resource shape differs')
        identity = tuple(row[key] for key in ('apiVersion', 'kind', 'namespace', 'name'))
        if not all(isinstance(value, str) for value in identity):
            raise frames.FrameError('inventory resource identity differs')
        actual.append(identity)
        owners[row['owner']] = owners.get(row['owner'], 0) + 1
    if (actual != identities or report['ownerCounts'] != owners
            or any(type(value) is not int for value in report['ownerCounts'].values())):
        raise frames.FrameError('inventory identities or owner counts differ')
    return report


def bind_inventory(context, method, path, map_reader=None, *, fixture_dir=frames.FIXTURES):
    """Bind exactly one historical response; never join contexts.

    map_reader(body_bytes, scope_dict) must return the product's full recorded
    map JSON as bytes. Source selection precedes that callback. No denial calls
    the callback, and errors never create empty inventory or trigger fallback.
    Ownership fields are retained product output, not independently re-detected
    here. Runtime admission must separately pin and enforce the reader.
    """
    body, provenance = frames.select_frame(context, method, path, fixture_dir=fixture_dir)
    payload = {
        'schema': 'recorded-context-inventory.v1', 'provenance': provenance,
        'coverage': 'unreadable', 'inventory': None,
        'currentStateEstablished': False, 'runtimeAdmission': False,
    }
    if provenance['httpStatus'] == 403:
        return payload
    if provenance['httpStatus'] != 200 or not callable(map_reader):
        raise frames.FrameError('recorded inventory reader unavailable')
    try:
        raw = map_reader(body, dict(SCOPE))
    except Exception:
        raise frames.FrameError('recorded inventory reader failed') from None
    payload['inventory'] = _validate(raw, body)
    payload['coverage'] = 'recorded-response'
    return payload
