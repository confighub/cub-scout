#!/usr/bin/env python3
"""Run one reviewed economy diagnostic pair; never retry or overwrite a run."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time


def stop_owned_group(pgid, grace):
    try:
        os.killpg(pgid, signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + grace
    while time.monotonic() < deadline:
        try:
            os.killpg(pgid, 0)
        except ProcessLookupError:
            return
        time.sleep(min(0.05, max(0, deadline - time.monotonic())))
    try:
        os.killpg(pgid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def run_owned(command, root, timeout=210, grace=10):
    # Exclusive launch marker also rejects concurrent launches in this output.
    with (root / 'launch.json').open('x') as launch:
        if (root / 'result.json').exists():
            raise ValueError('refusing to overwrite an existing result')
        env = {**os.environ, 'CLAUDE_CODE_DISABLE_FAST_MODE': '1'}
        started = time.monotonic()
        with (root / 'run.stdout').open('x') as stdout, (root / 'run.stderr').open('x') as stderr:
            process = subprocess.Popen(command, cwd=root / 'plugin', env=env,
                                       stdout=stdout, stderr=stderr, start_new_session=True)
            json.dump({'pid': process.pid, 'pgid': process.pid, 'command': command,
                       'timeoutSeconds': timeout, 'terminationGraceSeconds': grace,
                       'fastModeDisabled': True}, launch)
            launch.flush()
            timed_out = False
            try:
                code = process.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                timed_out = True
                code = 124
            finally:
                # Includes surviving descendants after the launcher exits. This
                # addresses this owned group, not detached/global processes.
                stop_owned_group(process.pid, grace)
                process.wait()
        (root / 'completion.json').write_text(json.dumps({
            'exit': code, 'timedOut': timed_out, 'seconds': round(time.monotonic()-started, 3),
            'resultExists': (root / 'result.json').exists(),
        }, indent=2) + '\n')
        return code


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('prepared_directory', type=Path)
    args = parser.parse_args()
    root = args.prepared_directory.resolve(strict=True)
    prepared = json.loads((root / 'prepared.json').read_text())
    if prepared.get('purpose') != 'economy' or prepared.get('modelRun') is not False:
        parser.error('requires an unrun economy preparation')
    for name, expected in prepared['generatedPluginFiles'].items():
        path = (root / 'plugin' / name).resolve(strict=True)
        if not path.is_relative_to(root / 'plugin') or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            parser.error('prepared plugin hash mismatch')
    if hashlib.sha256(Path(prepared['binary']).read_bytes()).hexdigest() != prepared['binarySha256']:
        parser.error('pinned Scout binary changed')
    command = ['claude', 'plugin', 'eval', '.', '--scaffold', '--case', 'recorded-explain-mcp',
               '--runs', '1', '--concurrency', '1', '--ablation', 'with-without', '--mocks', 'record',
               '--allow-real-servers', '--allow-tools', 'mcp__plugin_recorded-mcp-probe_cub-scout__explain',
               '--model', 'claude-haiku-4-5-20251001', '--max-cost-usd', '1', '--no-publish',
               '--trust-plugin', '--keep-temp', '--json', str(root / 'result.json')]
    return run_owned(command, root)


if __name__ == '__main__':
    raise SystemExit(main())
