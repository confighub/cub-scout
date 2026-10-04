"""PRE-03 declared child reference from pinned fields, never an observed GVK."""
import hashlib
import json
from pathlib import Path

import context_frames as frames

FIXTURES = Path(__file__).resolve().parents[1] / 'pre03-argo-child-failure/fixtures'
PINS = {
    'capture-scope.json': 'a1b64893b8df6ab27200e2b0f08e6bbb1f4413ac8694bfbdb6a24c6f485c5878',
    'argocd-core-root.json': '5f2fe7e6f3200327567b50abd695a8bbeb85f7c6d5f4e92976798bb5a8e36fff',
    'argocd-core-child.json': 'c8e5c7476a8b0f3a7941587957b63caaa651247715ed537be3a6f3795f0e3ffe',
}
RESOURCE_LIMIT = 32
STRING_LIMIT = 512
MAX_REPORT_BYTES = 64 * 1024


def text(values, key, *, required=False, allow_empty=False):
    value = values.get(key)
    if value is None and not required:
        return None
    if not isinstance(value, str) or (not value.strip() and not (allow_empty and value == '')):
        raise frames.FrameError('invalid captured string field')
    try:
        size = len(value.encode('utf-8', 'strict'))
    except UnicodeError:
        raise frames.FrameError('invalid captured string encoding') from None
    if size > STRING_LIMIT:
        raise frames.FrameError('captured string exceeds bound')
    return value


def identity(obj):
    metadata = obj.get('metadata')
    if not isinstance(metadata, dict):
        raise frames.FrameError('captured identity unavailable')
    return {key: text(metadata, key, required=True) for key in ('namespace', 'name', 'uid')}


def state(obj):
    status = obj.get('status')
    if not isinstance(status, dict):
        raise frames.FrameError('captured status unavailable')
    reported = {}
    for key in ('sync', 'health'):
        values = status.get(key)
        if not isinstance(values, dict):
            raise frames.FrameError('captured controller report unavailable')
        reported[key] = {field: value for field in ('status', 'revision', 'lastTransitionTime')
                         if (value := text(values, field)) is not None}
    return reported


def project_reference(*, fixture_dir=FIXTURES):
    root = Path(fixture_dir)
    raw = {name: frames._read(root / name, pin) for name, pin in PINS.items()}
    values = {name: frames._json(body) for name, body in raw.items()}
    scope = values['capture-scope.json']
    parent, child = values['argocd-core-root.json'], values['argocd-core-child.json']
    if not all(isinstance(value, dict) for value in values.values()):
        raise frames.FrameError('captured objects unavailable')
    source_hashes = scope.get('source_file_sha256')
    if (scope.get('schema') != 'pre03-argo-child-evidence.v1' or not isinstance(source_hashes, dict) or
            source_hashes.get('argocd-core-root.json') != PINS['argocd-core-root.json'] or
            source_hashes.get('argocd-core-child.json') != PINS['argocd-core-child.json']):
        raise frames.FrameError('captured source scope differs')
    # Neither response supplied served GVK. Even declared references or tracking
    # metadata cannot supply those missing observed fields.
    if any(key in obj for obj in (parent, child) for key in ('apiVersion', 'kind')):
        raise frames.FrameError('captured response contract differs')
    pi, ci = identity(parent), identity(child)
    if pi['uid'] == ci['uid']:
        raise frames.FrameError('distinct captured instances required')
    parent_state, child_state = state(parent), state(child)
    refs = parent['status'].get('resources')
    if not isinstance(refs, list):
        raise frames.FrameError('declared references unavailable')
    matches = [ref for ref in refs if isinstance(ref, dict) and
               (ref.get('namespace'), ref.get('name')) == (ci['namespace'], ci['name'])]
    if len(matches) != 1:
        raise frames.FrameError('declared target absent or ambiguous')
    target = {key: text(matches[0], key, required=True)
              for key in ('group', 'version', 'kind', 'namespace', 'name')}
    if (target['group'], target['version'], target['kind']) != ('argoproj.io', 'v1alpha1', 'Application'):
        raise frames.FrameError('unsupported declared target')
    expected_tracking = pi['name'] + ':argoproj.io/Application:' + ci['namespace'] + '/' + ci['name']
    annotations = child['metadata'].get('annotations')
    if not isinstance(annotations, dict) or annotations.get('argocd.argoproj.io/tracking-id') != expected_tracking:
        raise frames.FrameError('captured tracking identity differs')
    resources = child['status'].get('resources')
    if not isinstance(resources, list) or any(not isinstance(r, dict) for r in resources):
        raise frames.FrameError('child resource reports unavailable')
    # Keep separate raw controller rows; no workload fetch, tree/Pod join, health
    # decision, atomicity or UID foreign-key is manufactured from these fields.
    rows = []
    for resource in resources:
        row = {key: value for key in ('group', 'version', 'kind', 'namespace', 'name', 'status')
               if (value := text(resource, key, required=key in ('kind', 'name'), allow_empty=key == 'group')) is not None}
        health = resource.get('health')
        if health is not None:
            if not isinstance(health, dict):
                raise frames.FrameError('resource health report unavailable')
            row['health'] = {key: value for key in ('status', 'message', 'lastTransitionTime')
                             if (value := text(health, key)) is not None}
        rows.append(row)
    rows.sort(key=lambda row: json.dumps(row, sort_keys=True))
    report = {
        'schema': 'pre03.declared-child-reference.v1',
        'sources': {name: {'sha256': hashlib.sha256(body).hexdigest(), 'bytes': len(body)}
                    for name, body in raw.items()},
        'parent': {'capturedIdentity': pi, 'observedGVK': 'unknown', 'reported': parent_state},
        'declaredTarget': target,
        'child': {'capturedIdentity': ci, 'observedGVK': 'unknown', 'trackingId': expected_tracking,
                  'reported': child_state, 'resourceCount': len(rows),
                  'resources': rows[:RESOURCE_LIMIT], 'omittedResourceCount': max(0, len(rows) - RESOURCE_LIMIT)},
        'matchBasis': ['exact namespace/name declared reference', 'child tracking-id names captured parent'],
        'uidForeignKey': 'not supplied', 'atomicSnapshot': False,
        'responseCaptureTime': 'unknown', 'currentState': 'not established',
        'fullKubernetesObjectSnapshot': False,
    }
    if len(json.dumps(report, sort_keys=True).encode()) > MAX_REPORT_BYTES:
        raise frames.FrameError('declared reference report exceeds bound')
    return report
