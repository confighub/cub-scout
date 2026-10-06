#!/usr/bin/env python3
"""Explicit source-bound host stdio preflight, not a model/tool admission."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time

import policy
import recorded_server

HELPERS = policy.REPO / 'evals/recorded-api'
ADAPTER_PINS = {
    'context_frames.py': '63a259fd0e741dc199f6c17ecf44700923f6c1f4cf8e1973c15b623e9353a46d',
    'context_mcp.py': 'befb30fb7c21afdf7e6d3e6c543dcae6c9e67908bc645d5f0d1668c255741050',
}
# Import only the reviewed sources, before trusting their fixture constants.
for _name, _pin in ADAPTER_PINS.items():
    _raw = recorded_server.read_regular(HELPERS / _name, 128 * 1024)
    if hashlib.sha256(_raw).hexdigest() != _pin:
        raise ValueError('unreviewed adapter source')
sys.path.insert(0, str(HELPERS))
import context_frames as frames
import context_mcp as mcp

BOOTSTRAP = b'''import sys
from pathlib import Path
root = Path(sys.argv[1])
sys.path.insert(0, str(root))
import context_mcp
try:
    context_mcp.serve(sys.stdin.buffer, sys.stdout.buffer, fixture_dir=root / 'cluster')
except (ValueError, OSError, UnicodeError):
    sys.stderr.write('recorded-response preflight refused\\n')
    raise SystemExit(2)
'''
MAX_OUTPUT = 512 * 1024


def encoded(value):
    return (json.dumps(value, indent=2, sort_keys=True) + '\n').encode()


def _exact(actual, expected):
    # Python equality treats 0/False and 1/True as equal. Protocol proof must
    # preserve JSON types as well as values, while ignoring object key order.
    return encoded(actual) == encoded(expected)


def _selection(receipt):
    if receipt.get('selection') != {'case': 'RUL-03', 'arm': 'with', 'authoredInputControl': None}:
        raise ValueError('only original RUL-03 treatment preflight is reviewed')
    expected = {'cluster/' + name: pin for name, pin in frames.PINS.items()}
    files = receipt.get('stagedFiles', {})
    if {name: pin for name, pin in files.items() if name.startswith('cluster/')} != expected:
        raise ValueError('selected context evidence differs')
    return expected


def build(source, stage):
    receipt = policy.stager.verify(stage, source, 'RUL-03', 'with')
    evidence = _selection(receipt)
    root = stage / 'model-stage/cluster'
    for context in sorted(frames.CONTEXTS):
        frames.select_frame(context, 'GET', frames.REQUEST_PATH, fixture_dir=root)
    assets = {}
    for name, pin in ADAPTER_PINS.items():
        raw = recorded_server.read_regular(HELPERS / name, 128 * 1024)
        if policy.sha(raw) != pin:
            raise ValueError('adapter source differs from reviewed pin')
        assets[name] = raw
    assets['bootstrap.py'] = BOOTSTRAP
    for name, pin in evidence.items():
        raw = frames._read(stage / 'model-stage' / name, pin)
        assets[name] = raw
    plan = {'schema': 'recorded-context-host-preflight.v1', 'selection': receipt['selection'],
            'sourcePreparationReportSha256': receipt['sourcePreparation']['reportSha256'],
            'stageReceiptSha256': policy.sha(recorded_server.read_regular(stage / 'receipt.json', 128 * 1024)),
            'files': {name: policy.sha(raw) for name, raw in sorted(assets.items())},
            'preflightTools': [mcp.TOOL['name']], 'frozenLaunchToolGranted': False,
            'productScoutCapability': False, 'runtimeAssetsAdmitted': False,
            'paidRunAdmitted': False, 'qualityOrSavingsMeasured': False}
    return plan, assets


def requests():
    init = {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
        'protocolVersion': '2025-03-26', 'capabilities': {},
        'clientInfo': {'name': 'source-bound-host-preflight', 'version': '1'}}}
    rows = [init, {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list'}]
    for identifier, context in ((3, 'rul03-denied'), (4, 'rul03-readable'), (5, 'current-context')):
        rows.append({'jsonrpc': '2.0', 'id': identifier, 'method': 'tools/call', 'params': {
            'name': 'recorded_response', 'arguments': {'context': context, 'method': 'GET', 'path': frames.REQUEST_PATH}}})
    rows.append({'jsonrpc': '2.0', 'id': 6, 'method': 'tools/call', 'params': {'name': 'map', 'arguments': {}}})
    return b''.join(mcp.encoded(row) for row in rows)


def check(raw, evidence_root):
    if len(raw) > MAX_OUTPUT:
        raise ValueError('preflight output exceeds bound')
    rows = [frames._json(line) for line in raw.splitlines()]
    if (len(rows) != 6 or any(not isinstance(row, dict) for row in rows)
            or [row.get('id') for row in rows] != list(range(1, 7))
            or any(type(row.get('id')) is not int for row in rows)
            or any(row.get('jsonrpc') != '2.0' or set(row) != {'jsonrpc', 'id', 'result'} for row in rows)):
        raise ValueError('preflight replies differ')
    if not _exact(rows[0]['result'], {'protocolVersion': '2025-03-26', 'capabilities': {'tools': {}},
                                    'serverInfo': {'name': 'scout-recorded-response-eval', 'version': '1'}}):
        raise ValueError('preflight initialization differs')
    if not _exact(rows[1]['result'], {'tools': [mcp.TOOL]}):
        raise ValueError('preflight catalog differs')
    results = []
    for index, context in ((2, 'rul03-denied'), (3, 'rul03-readable')):
        body, provenance = frames.select_frame(context, 'GET', frames.REQUEST_PATH, fixture_dir=evidence_root)
        expected = {'isError': False, 'content': [{'type': 'text', 'text':
                    mcp.encoded({'responseText': body.decode(), 'provenance': provenance}).decode().rstrip('\n')}]}
        if not _exact(rows[index]['result'], expected):
            raise ValueError('preflight response bytes/status/provenance differ')
        results.append({'context': context, 'httpStatus': provenance['httpStatus'],
                        'responseSha256': policy.sha(body), 'sourceRow': provenance['sourceRow']})
    refused = {'isError': True, 'content': [{'type': 'text', 'text': 'No exact recorded request'}]}
    if any(not _exact(row['result'], refused) for row in rows[4:]):
        raise ValueError('preflight unsupported selection did not refuse')
    return {'responses': results, 'unsupportedContextRefused': True, 'mapSubstitutionRefused': True}


def _new_output(output, source, stage):
    output = policy.stager._no_symlink_components(output)
    roots = {Path(tempfile.gettempdir()).resolve(), Path('/tmp').resolve(), Path('/var/tmp').resolve()}
    if not any(output.parent == root or root in output.parent.parents for root in roots):
        raise ValueError('output must be under a temporary directory')
    for denied in (source.resolve(), stage.resolve(), policy.REPO.resolve(), Path.home().resolve()):
        if output == denied or denied in output.parents or output in denied.parents:
            raise ValueError('output overlaps protected input')
    if output.exists():
        raise ValueError('output must be fresh')
    output.mkdir(mode=0o700)
    output.chmod(0o700)
    return output


def run(source, stage, python, python_sha, output):
    if not isinstance(python_sha, str) or not re.fullmatch('[0-9a-f]{64}', python_sha):
        raise ValueError('explicit interpreter pin required')
    binary = recorded_server.read_regular(python, 128 * 1024 * 1024)
    if policy.sha(binary) != python_sha or not os.access(python, os.X_OK):
        raise ValueError('interpreter does not match selected host pin')
    plan, assets = build(source, stage)
    output = _new_output(output, source, stage)
    package = output / 'package'
    package.mkdir(mode=0o700)
    for name, raw in sorted(assets.items()):
        policy.stager._write_new(package / name, raw, 0o444)
    policy.stager._freeze_tree(package)
    home = output / 'home'
    home.mkdir(mode=0o700)
    policy.stager._write_new(home / 'empty-kubeconfig', b'apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n', 0o600)
    policy.stager._write_new(output / 'plan.json', encoded(plan), 0o600)
    policy.stager._write_new(output / 'stdin.jsonl', requests(), 0o600)
    argv = [str(python), '-I', '-S', str(package / 'bootstrap.py'), str(package)]
    result = {'schema': 'recorded-context-host-proof.v1', 'status': 'failed', 'plan': plan,
              'argv': argv, 'pythonSha256': python_sha, 'exitCode': None, 'timedOut': False,
              'stdinSha256': policy.sha(requests()),
              'hostProbeSourceSha256': policy.sha(recorded_server.read_regular(Path(__file__), 128 * 1024)),
              'claims': {'hostStdioUsable': False, 'runtimeAssetsAdmitted': False,
                         'sourceFilesystemInaccessible': False, 'networkIsolationProved': False,
                         'processAccountingComplete': False, 'frozenLaunchToolGranted': False,
                         'paidRunAdmitted': False, 'qualityOrSavingsMeasured': False}}
    started = time.monotonic()
    try:
        with (output / 'stdin.jsonl').open('rb') as stream_in, (output / 'stdout.jsonl').open('xb') as stream_out, (output / 'stderr.txt').open('xb') as stream_err:
            os.chmod(output / 'stdout.jsonl', 0o600)
            os.chmod(output / 'stderr.txt', 0o600)
            child = subprocess.Popen(argv, stdin=stream_in, stdout=stream_out, stderr=stream_err,
                cwd=home, env={'PATH': '/usr/bin:/bin', 'HOME': str(home),
                              'KUBECONFIG': str(home / 'empty-kubeconfig'), 'LANG': 'C.UTF-8'})
            try:
                result['exitCode'] = child.wait(timeout=15)
            except subprocess.TimeoutExpired:
                result['timedOut'] = True
                child.kill()
                result['exitCode'] = child.wait(timeout=5)
        result['elapsedSeconds'] = time.monotonic() - started
        # Revalidate original source, the child package and interpreter after it exits.
        if build(source, stage)[0] != plan or policy.stager._hash_tree(package) != plan['files']:
            raise ValueError('preflight source/package changed')
        if any(p.stat().st_mode & 0o222 for p in (package, *package.rglob('*'))):
            raise ValueError('preflight package became writable')
        if policy.sha(recorded_server.read_regular(python, 128 * 1024 * 1024)) != python_sha:
            raise ValueError('host interpreter changed')
        if policy.sha(recorded_server.read_regular(Path(__file__), 128 * 1024)) != result['hostProbeSourceSha256']:
            raise ValueError('host probe source changed')
        stdout = recorded_server.read_regular(output / 'stdout.jsonl', MAX_OUTPUT)
        stderr = recorded_server.read_regular(output / 'stderr.txt', MAX_OUTPUT)
        result['stdoutSha256'], result['stderrSha256'] = policy.sha(stdout), policy.sha(stderr)
        if result['timedOut'] or result['exitCode'] != 0 or stderr:
            raise ValueError('host child failed or timed out')
        result['checks'] = check(stdout, package / 'cluster')
        result['claims']['hostStdioUsable'] = True
        result['status'] = 'host_preflight_passed_not_runtime_admitted'
    except (ValueError, OSError, subprocess.SubprocessError):
        result['failure'] = 'host preflight refused; retained attempt artifacts require review'
    finally:
        result['elapsedSeconds'] = time.monotonic() - started
        policy.stager._write_new(output / 'proof.json', encoded(result), 0o600)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-prep', type=Path, required=True)
    parser.add_argument('--stage', type=Path, required=True)
    parser.add_argument('--python', type=Path, required=True)
    parser.add_argument('--python-sha256', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = run(args.source_prep, args.stage, args.python, args.python_sha256, args.output)
    print(json.dumps({'status': result['status'], 'proof': str(args.output / 'proof.json')}))
    return 0 if result['claims']['hostStdioUsable'] else 2


if __name__ == '__main__':
    raise SystemExit(main())
