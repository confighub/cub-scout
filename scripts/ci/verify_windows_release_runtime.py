#!/usr/bin/env python3
"""Inspect and execute configured native Windows snapshot builds, without publishing."""
import argparse
import hashlib
import json
import platform
import subprocess
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--preflight', action='store_true')
    parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    parser.add_argument('--source', required=True)
    parser.add_argument('--dist', type=Path, default=Path('.goreleaser-dist'))
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if platform.system() != 'Windows':
        raise SystemExit('Native Windows is required; cross-build inspection is insufficient')
    host = subprocess.check_output(['go', 'env', 'GOHOSTARCH'], text=True).strip()
    if host != args.arch:
        raise SystemExit(f'Native Go host architecture {host} differs from {args.arch}')
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if source != args.source:
        raise SystemExit('Checked-out source differs from the dispatched source')
    status = subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip()
    if status:
        diff = subprocess.check_output(['git', 'diff', '--', 'go.mod', 'go.sum'], text=True)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps({'status': 'refused-dirty-source',
            'sourceRevision': source, 'gitStatus': status, 'dependencyDiff': diff}, indent=2) + '\n')
        raise SystemExit('Build hooks or outputs dirtied the source checkout:\n' + status + '\n' + diff)
    if args.preflight:
        inputs = []
        for filename in ('go.mod', 'go.sum'):
            observed = Path(filename).read_bytes()
            indexed = subprocess.check_output(['git', 'show', 'HEAD:' + filename])
            if observed != indexed:
                raise SystemExit(f'{filename} checkout bytes differ from the Git source')
            inputs.append({'path': filename, 'sha256': hashlib.sha256(observed).hexdigest()})
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps({'schema': 'scout-native-windows-inputs.v1',
            'sourceRevision': source, 'architecture': args.arch, 'inputs': inputs,
            'claims': {'canonicalSourceInputsVerified': True, 'runtimeAccepted': False}}, indent=2) + '\n')
        print('Native Windows canonical source inputs verified')
        return
    metadata = json.loads((args.dist / 'metadata.json').read_text())
    if metadata['commit'] != source or metadata['version'] != 'v2.13.0-next':
        raise SystemExit('Snapshot metadata does not match the required source/version')
    records = []
    for build, filename in [('cub-scout', 'cub-scout.exe'),
                            ('kubectl-cub-scout', 'kubectl-cub_scout.exe')]:
        matches = list(args.dist.glob(f'{build}_windows_{args.arch}_*/{filename}'))
        if len(matches) != 1:
            raise SystemExit(f'Expected exactly one configured {build} binary')
        binary = matches[0].resolve()
        info = subprocess.check_output(['go', 'version', '-m', str(binary)], text=True)
        # The toolchain go.mod selects; setup-go installs it from there.
        required = [': go1.26.9', 'github.com/confighub/cub-scout/v2',
                    'vcs.revision=' + source, 'vcs.modified=false',
                    'CGO_ENABLED=0', 'GOOS=windows', 'GOARCH=' + args.arch]
        if any(value not in info for value in required):
            raise SystemExit(f'Invalid embedded release identity: {filename}')
        executions = []
        for command in [['version'], ['help', 'trace']]:
            result = subprocess.run([str(binary), *command], capture_output=True, text=True)
            expected = 'v2.13.0-next' if command == ['version'] else 'cub-scout trace'
            if result.returncode or expected not in result.stdout:
                raise SystemExit(f'Native execution failed: {filename} {command}: {result.stderr}')
            executions.append({'args': command, 'exit': result.returncode,
                               'stdout': result.stdout, 'stderr': result.stderr})
        records.append({'path': str(binary.relative_to(args.dist.resolve())),
                        'sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                        'bytes': binary.stat().st_size, 'buildInfo': info,
                        'executions': executions})
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({'schema': 'scout-native-windows-runtime.v1',
        'sourceRevision': source, 'architecture': args.arch, 'hostMachine': platform.machine(),
        'metadata': metadata, 'binaries': records,
        'claims': {'nativeConfiguredSnapshotSmokePassed': True, 'published': False,
                   'clusterBehaviorAccepted': False, 'publicInstallAccepted': False}}, indent=2) + '\n')
    print(f'Native Windows {args.arch}: both configured binaries pass identity/version/help checks')


if __name__ == '__main__':
    main()
