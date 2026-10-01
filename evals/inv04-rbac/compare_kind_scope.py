#!/usr/bin/env python3
"""One owned-cluster old/new comparison; uses the reviewed INV-04 cleanup path."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import time

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('inv04_capture', HERE / 'capture.py')
capture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capture)

NEW_SOURCE = 'eec6d442279955af0f1fc39c637252ac8eb0f081'
NEW_SHA256 = '7d20aa7bb33b477dfd88afb2f53b1a6f773a5a4ffbcff7f81d8dcbfeb5a12bab'


def validate_comparison(scope, old, new):
    if old.get('schema') != 'map-list-ownership-evidence.v1' or new.get('schema') != old['schema']:
        raise capture.CaptureError('unexpected map schema')
    if old.get('resources') != new.get('resources'):
        raise capture.CaptureError('selected resource or ownership facts changed')
    if old.get('collection', {}).get('status') != 'partial' or new.get('collection', {}).get('status') != 'partial':
        raise capture.CaptureError('read-limited observer must remain partial')
    expected = [{'apiVersion': 'argoproj.io/v1alpha1', 'resource': 'applicationsets',
                 'namespace': scope, 'reason': 'forbidden'}]
    if scope == capture.NAMESPACES[2]:
        expected.append({'apiVersion': 'apps/v1', 'resource': 'deployments',
                         'namespace': scope, 'reason': 'forbidden'})
    key = lambda row: (row['apiVersion'], row['resource'], row['namespace'], row['reason'])
    if sorted(new['collection']['omissions'], key=key) != sorted(expected, key=key):
        raise capture.CaptureError('new omissions do not match actual requested scope')
    if not all(row in old['collection']['omissions'] for row in expected):
        raise capture.CaptureError('old output did not contain the same scoped omissions')
    count = 2 if scope == capture.NAMESPACES[0] else 0
    if len(new['resources']) != count:
        raise capture.CaptureError('unexpected selected resource count')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute', action='store_true')
    parser.add_argument('--shared-kubeconfig', type=Path, required=True)
    parser.add_argument('--old-binary', type=Path, required=True)
    parser.add_argument('--new-binary', type=Path, required=True)
    parser.add_argument('--output-dir', type=Path, required=True)
    args = parser.parse_args()
    if not args.execute:
        parser.error('--execute is required')
    if args.shared_kubeconfig.is_symlink() or not args.shared_kubeconfig.is_file():
        parser.error('shared kubeconfig must be a regular file, not a symlink')
    new_binary = args.new_binary.resolve(strict=True)
    if hashlib.sha256(new_binary.read_bytes()).hexdigest() != NEW_SHA256 or not os.access(new_binary, os.X_OK):
        parser.error('new binary does not match reviewed build')
    driver_sha = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()

    def compare(out, scope, observer_config, token, record):
        argv = [str(new_binary), *record['scout']['argv']]
        started = capture.utc_now()
        clock = time.monotonic()
        code, stdout, stderr = capture.run_bounded(argv, capture.COMMAND_TIMEOUT,
            dict(os.environ, KUBECONFIG=str(observer_config)))
        elapsed = time.monotonic() - clock
        ended = capture.utc_now()
        if token.encode() in stdout or token.encode() in stderr:
            raise capture.CaptureError('comparison output contained observer credentials')
        stdout_name, stderr_name = scope + '.new-map.json', scope + '.new-stderr.txt'
        capture._write(out / stdout_name, stdout)
        capture._write(out / stderr_name, stderr)
        record['comparison'] = {
            'driverSha256': driver_sha, 'sourceRevision': NEW_SOURCE,
            'binarySha256': NEW_SHA256, 'argv': record['scout']['argv'],
            'startedAt': started, 'endedAt': ended, 'elapsedSeconds': elapsed,
            'exitCode': code, 'stdoutFile': stdout_name, 'stdoutBytes': len(stdout),
            'stdoutSha256': capture.sha256(stdout), 'stderrFile': stderr_name,
            'stderrSha256': capture.sha256(stderr), 'validated': False,
            'limits': 'Sequential old/new commands on the same observer and cluster; not a randomized latency benchmark or agent-cost result.'}
        if code or record['scout']['exitCode']:
            raise capture.CaptureError('old or new map command failed')
        try:
            old = json.loads((out / record['scout']['stdoutFile']).read_bytes())
            new = json.loads(stdout)
        except (ValueError, UnicodeError):
            raise capture.CaptureError('map output is not JSON') from None
        validate_comparison(scope, old, new)
        record['comparison']['validated'] = True
        record['comparison']['oldOmissions'] = len(old['collection']['omissions'])
        record['comparison']['newOmissions'] = len(new['collection']['omissions'])

    try:
        return capture.capture(args.shared_kubeconfig.resolve(), args.old_binary,
                               args.output_dir, after_observation=compare)
    except capture.CaptureError as error:
        print(str(error))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
