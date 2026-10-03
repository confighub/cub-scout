#!/usr/bin/env python3
"""Explicit offline host stdio probe; never starts a model or live client.

Uses the locally built Scout, not the pinned Linux runtime. This does not prove
container isolation, provider access, actual Claude grants or paid admission.
"""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

import yaml

import policy
import recorded_server


def payload(result):
    if result.get('isError'):
        raise ValueError('recorded tool returned an error')
    content = result['content']
    if len(content) != 1 or content[0].get('type') != 'text':
        raise ValueError('unexpected recorded tool content')
    return json.loads(content[0]['text'])


def check(stdout, recording_sha, identity, object_count, selected_count):
    replies = [json.loads(line) for line in stdout.splitlines()]
    if [r.get('id') for r in replies] != [1, 2, 3, 4, 5]:
        raise ValueError('missing, duplicate or reordered reply')
    if any('error' in r for r in replies[:4]):
        raise ValueError('initialization or supported tool failed')
    tools = replies[1]['result']['tools']
    if sorted(t['name'] for t in tools) != ['explain', 'map']:
        raise ValueError('unexpected recorded tool inventory')
    if any(t.get('annotations', {}).get('readOnlyHint') is not True for t in tools):
        raise ValueError('recorded tool lacks read-only declaration')
    summary = payload(replies[2]['result'])
    explain = payload(replies[3]['result'])
    provenance = summary['provenance']
    if (provenance['sha256'] != recording_sha or provenance['captureTime'] != 'unknown'
        or provenance['captureCompleteness'] != 'unknown'):
        raise ValueError('recorded provenance differs')
    expected_identity = {'apiVersion': identity['api_version'], **{k: identity[k] for k in ('kind', 'namespace', 'name')}}
    recorded = explain.get('recordedInput', {})
    if recorded.get('sha256') != recording_sha or recorded.get('identity') != expected_identity:
        raise ValueError('explain recording or exact object identity differs')
    if provenance['objectCount'] != object_count or recorded.get('objectCount') != object_count:
        raise ValueError('recorded object count differs')
    if (type(summary.get('selectedCount')) is not int or summary['selectedCount'] != selected_count
        or type(summary.get('excludedFromScopeCount')) is not int
        or summary['excludedFromScopeCount'] != object_count - selected_count):
        raise ValueError('map scope totals differ from supplied objects')
    owners = summary.get('ownerCounts')
    if (not isinstance(owners, dict) or any(type(v) is not int or v < 0 for v in owners.values())
        or sum(owners.values()) != selected_count):
        raise ValueError('map owner counts do not account for selected objects')
    if 'error' not in replies[4] and not replies[4].get('result', {}).get('isError'):
        raise ValueError('unsupported live tool did not refuse')
    return {'tools': ['explain', 'map'], 'selectedCount': summary['selectedCount'],
            'excludedFromScopeCount': summary['excludedFromScopeCount'],
            'ownerCounts': summary['ownerCounts'], 'provenance': provenance,
            'explainIdentity': expected_identity, 'unsupportedToolRefused': True}


def run(source, binary, output):
    source = source.resolve()
    binary = binary.absolute()
    report = policy.stager.prepare.validate(source)
    binary_sha = policy.sha(recorded_server.read_regular(binary, 128 * 1024 * 1024))
    output.mkdir(parents=True, exist_ok=False)
    results = []
    for cid in sorted(policy.RECORDED_CASES):
        recording = source / 'arms/with/cases' / cid / policy.RECORDING
        raw = recorded_server.read_regular(recording, 4 * 1024 * 1024)
        recording_sha = policy.sha(raw)
        objects = []
        for document in yaml.safe_load_all(raw):
            objects.extend(document['items'] if document.get('kind') == 'List' else [document])
        obj = next(o for o in objects if o['apiVersion'] == 'apps/v1' and o['kind'] == 'Deployment')
        identity = {'api_version': obj['apiVersion'], 'kind': obj['kind'],
                    'namespace': obj['metadata']['namespace'], 'name': obj['metadata']['name']}
        binding = {'schema': 'full24-recorded-mcp-exec.v1', 'case': cid,
                   'recording': str(recording), 'recordingSha256': recording_sha,
                   'binary': str(binary), 'binarySha256': binary_sha}
        argv = recorded_server.validate(binding, cid, recording_path=str(recording),
                                        binary_path=str(binary), trusted_binary_sha256=binary_sha)
        messages = [
            {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
                'protocolVersion': '2025-03-26', 'capabilities': {},
                'clientInfo': {'name': 'offline-recorded-probe', 'version': '1'}}},
            {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list', 'params': {}},
            {'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {
                'name': 'map', 'arguments': {'api_version': 'apps/v1', 'kind': 'Deployment', 'summary': True}}},
            {'jsonrpc': '2.0', 'id': 4, 'method': 'tools/call', 'params': {'name': 'explain', 'arguments': identity}},
            {'jsonrpc': '2.0', 'id': 5, 'method': 'tools/call', 'params': {'name': 'doctor', 'arguments': {}}}]
        request = ''.join(json.dumps(m) + '\n' for m in messages).encode()
        (output / (cid + '.stdin')).write_bytes(request)
        with tempfile.TemporaryDirectory(prefix='scout-recorded-probe-') as home:
            config = Path(home) / 'empty-kubeconfig'
            config.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            env = {'PATH': '/usr/bin:/bin', 'HOME': home, 'KUBECONFIG': str(config), 'LANG': 'C.UTF-8'}
            try:
                child = subprocess.run(argv, input=request, capture_output=True, timeout=30, env=env, cwd=home)
            except subprocess.TimeoutExpired as exc:
                (output / (cid + '.stdout')).write_bytes(exc.stdout or b'')
                (output / (cid + '.stderr')).write_bytes(exc.stderr or b'')
                raise
        (output / (cid + '.stdout')).write_bytes(child.stdout)
        (output / (cid + '.stderr')).write_bytes(child.stderr)
        if child.returncode != 0:
            raise ValueError('recorded server exited nonzero: ' + cid)
        selected_count = sum(o.get('apiVersion') == 'apps/v1' and o.get('kind') == 'Deployment' for o in objects)
        observed = check(child.stdout, recording_sha, identity, len(objects), selected_count)
        if recording_sha != policy.sha(recorded_server.read_regular(recording, 4 * 1024 * 1024)):
            raise ValueError('recording changed during probe')
        results.append({'case': cid, 'binding': binding, 'argv': argv, 'exitCode': child.returncode,
                        'requestSha256': policy.sha(request), 'stdoutSha256': policy.sha(child.stdout),
                        'stderrSha256': policy.sha(child.stderr), 'observed': observed})
    if report != policy.stager.prepare.validate(source):
        raise ValueError('source preparation changed during probe')
    if binary_sha != policy.sha(recorded_server.read_regular(binary, 128 * 1024 * 1024)):
        raise ValueError('local executable changed during probe')
    result = {'schema': 'full24-local-recorded-stdio-probe.v1', 'cases': results,
              'scope': 'Seven selected source recordings, real locally built Scout; host stdio only.',
              'claims': {'hostRecordedToolsProbed': True, 'linuxRuntimePinProbed': False,
                         'containerIsolationProbed': False, 'claudeExecuted': False,
                         'actualToolGrantsEnforced': False, 'paidRunAdmitted': False}}
    (output / 'proof.json').write_text(json.dumps(result, indent=2, sort_keys=True) + '\n')
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-prep', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = run(args.source_prep, args.binary, args.output)
    print(json.dumps({'casesPassed': len(result['cases']), 'claims': result['claims']}, sort_keys=True))
