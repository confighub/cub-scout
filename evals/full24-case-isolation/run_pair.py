#!/usr/bin/env python3
"""Serial, owned, non-model selected-case container isolation diagnostic."""
from __future__ import annotations

import argparse
import importlib.util
import json
import re
import signal
import time
import uuid
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError('reviewed helper unavailable')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


stager = load('isolation_stager', REPO / 'evals/full24-case-stage/stage.py')
combined = load('isolation_combined', REPO / 'evals/combined-recorded-runtime/run_pair.py')
linux = combined.linux
SCHEMA = 'full24-case-isolation.v1'
TOTAL_SECONDS = 180
CAP = 512 * 1024
PROBE = (HERE / 'probe.py').read_bytes()


def command():
    return ['/usr/bin/env', '-i', 'PATH=/usr/bin:/bin', 'HOME=/tmp/private-home',
            'LANG=C.UTF-8', 'PYTHONDONTWRITEBYTECODE=1', '/usr/local/bin/python3',
            '-c', PROBE.decode('utf-8')]


def create_args(name, owner, model_stage):
    # Preserve the previously reviewed profile; replace only its fixed payload.
    if not re.fullmatch(r'scout-combined-isolation-[0-9a-f]{16}', name):
        raise ValueError('invalid isolation container name')
    if not re.fullmatch(r'[0-9a-f]{32}', owner):
        raise ValueError('invalid owner')
    root = stager._no_symlink_components(model_stage)
    if not root.is_dir() or root.name != 'model-stage' or ',' in str(root):
        raise ValueError('unsafe model-stage mount')
    args = combined.custom_create_args(name, owner, root, plugin=None)
    index = args.index(linux.IMAGE_ID)
    return args[:index + 1] + command()


def inspect(raw, name, owner, model_stage, known_id, *, finished=False):
    ident = combined.inspect_runtime(raw, name, owner, model_stage, None, finished=finished)
    obj = linux._one(raw)
    config, host = obj['Config'], obj['HostConfig']
    if ident != known_id or config.get('Cmd') != command() or config.get('Entrypoint') not in (None, []):
        raise ValueError('container ID or fixed command differs')
    if set(host['Tmpfs']) != {'/tmp'} or host.get('Devices') not in (None, []) or host.get('DeviceRequests') not in (None, []):
        raise ValueError('unexpected tmpfs or device access')
    if finished and (obj['State'].get('ExitCode') != 0 or obj['State'].get('OOMKilled') is not False):
        raise ValueError('probe did not finish successfully')
    return ident


def validate_probe(raw, receipt, model_stage):
    value = linux.strict_json(raw, 'isolation probe')
    expected = {'schema', 'files', 'bytes', 'uid', 'gid', 'writeDenied', 'interfaces'}
    if not isinstance(value, dict) or set(value) != expected:
        raise ValueError('malformed probe output')
    if (value['schema'] != 'full24-case-isolation-probe.v1'
        or value['files'] != receipt['stagedFiles']
        or type(value['uid']) is not int or value['uid'] != 65534
        or type(value['gid']) is not int or value['gid'] != 65534
        or value['writeDenied'] != ['/tools/.write-control', '/tmp/stage-alias/.write-control', '/root-write-control']
        or value['interfaces'] != ['lo']
        or type(value['bytes']) is not int
        or value['bytes'] != sum((model_stage / name).stat().st_size for name in receipt['stagedFiles'])):
        raise ValueError('probe result differs from selected stage or isolation controls')
    return value


def run_arm(docker, context, env, source, stage_root, receipt, output, deadline, runner):
    owner = uuid.uuid4().hex
    name = 'scout-combined-isolation-' + owner[:16]
    arm = receipt['selection']['arm']
    model_stage = stage_root / 'model-stage'
    result = {'selection': receipt['selection'], 'status': 'failed', 'name': name,
              'owner': owner, 'operations': [], 'stageReceiptSha256': linux.sha((stage_root / 'receipt.json').read_bytes()),
              'probeSha256': linux.sha(PROBE), 'containerId': None}
    known_id = ''

    def retain(args, code, stdout, stderr, began):
        if type(code) is not int or not isinstance(stdout, bytes) or not isinstance(stderr, bytes) or len(stdout) + len(stderr) > CAP:
            raise ValueError('malformed or oversized Docker command output')
        prefix = f'{arm}-op-{len(result["operations"]):02d}'
        linux.write_new(output / (prefix + '-stdout.bin'), stdout)
        linux.write_new(output / (prefix + '-stderr.bin'), stderr)
        result['operations'].append({'argv': args, 'exitCode': code, 'elapsedSeconds': time.monotonic() - began,
                                     'stdout': prefix + '-stdout.bin', 'stderr': prefix + '-stderr.bin',
                                     'stdoutSha256': linux.sha(stdout), 'stderrSha256': linux.sha(stderr)})
        return code, stdout, stderr

    def call(args, seconds):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError('execution deadline expired')
        began = time.monotonic()
        code, stdout, stderr = runner([str(docker), '--context', context, *args],
                                      min(seconds, remaining), env=env, max_output=CAP)
        retain(args, code, stdout, stderr, began)
        if code != 0:
            raise RuntimeError('Docker command failed: ' + args[0] + ' ' + args[1])
        return stdout, stderr

    def cleanup_runner(argv, seconds, *, env, max_output):
        began = time.monotonic()
        code, stdout, stderr = runner(argv, seconds, env=env, max_output=max_output)
        return retain(argv[3:], code, stdout, stderr, began)

    try:
        # Validate immediately before create; the host receipt is never mounted.
        stager.verify(stage_root, source, receipt['selection']['case'], arm,
                      receipt['selection']['authoredInputControl'])
        raw, _ = call(create_args(name, owner, model_stage), 10)
        candidate = raw.decode('ascii', 'strict').strip()
        if not re.fullmatch(r'[0-9a-f]{64}', candidate):
            raise ValueError('container create ID malformed')
        known_id = candidate
        result['containerId'] = known_id
        raw, _ = call(['container', 'inspect', '--format', '{{json .}}', known_id], 10)
        inspect(raw, name, owner, model_stage, known_id)
        call(['container', 'start', known_id], 10)
        raw, _ = call(['container', 'wait', known_id], 40)
        if raw.strip() != b'0':
            raise ValueError('container wait did not report zero')
        raw, _ = call(['container', 'inspect', '--format', '{{json .}}', known_id], 10)
        inspect(raw, name, owner, model_stage, known_id, finished=True)
        raw, err = call(['container', 'logs', known_id], 10)
        if err:
            raise ValueError('probe emitted unexpected stderr')
        result['probe'] = validate_probe(raw, receipt, model_stage)
        stager.verify(stage_root, source, receipt['selection']['case'], arm,
                      receipt['selection']['authoredInputControl'])
        result['status'] = 'probe_passed_cleanup_pending'
    except BaseException as exc:
        # Preserve interruption/failure; cleanup runs even if create output was lost.
        result['error'] = type(exc).__name__ + ': ' + str(exc)[:300]
    finally:
        result['cleanup'] = linux.cleanup(docker, context, name, owner, env,
                                          time.monotonic() + 30, cleanup_runner, known_id)
        if result['cleanup']['verifiedAbsent'] is not True:
            result['status'] = 'failed'
            result['error'] = result.get('error', 'owned cleanup unverified')
        elif result['status'] == 'probe_passed_cleanup_pending':
            try:
                stager.verify(stage_root, source, receipt['selection']['case'], arm,
                              receipt['selection']['authoredInputControl'])
                result['status'] = 'isolation_probe_passed'
            except BaseException as exc:
                result['status'] = 'failed'
                result['error'] = type(exc).__name__ + ': ' + str(exc)[:300]
        linux.write_new(output / (arm + '-receipt.json'), (json.dumps(result, indent=2, sort_keys=True) + '\n').encode())
    return result


def run_pair(source, case_id, control, output, docker_path, context, *, runner=None):
    linux.validate_context(context)
    docker, _ = linux.docker_launcher(docker_path)
    # Validate inputs before any Docker command/output mutation.
    source = stager._no_symlink_components(source)
    report = stager.prepare.validate(source)
    if case_id not in stager.prepare.FROZEN_CASE_IDS:
        raise ValueError('unknown frozen case')
    if control is not None and report.get('authoredInputControls', {}).get(case_id, {}).get('id') != control:
        raise ValueError('unknown authored input control')
    out = stager._no_symlink_components(output)
    if ',' in str(out) or out == source or out.is_relative_to(source) or source.is_relative_to(out):
        raise ValueError('unsafe runtime output/source overlap')
    out = linux.safe_output(out)
    deadline = time.monotonic() + TOTAL_SECONDS
    run = runner or linux._inv04.run_bounded
    env = linux.docker_env()
    result = {'schema': SCHEMA, 'status': 'failed', 'case': case_id, 'authoredInputControl': control,
              'imageId': linux.IMAGE_ID, 'arms': {}, 'claims': {
                  'modelExecuted': False, 'ordinaryToolGrantEnforced': False, 'mcpUsable': False,
                  'officialGraderExecuted': False, 'processAccountingComplete': False,
                  'paidRunAdmitted': False, 'qualityOrSavingsMeasured': False}}
    previous = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}

    def interrupted(signum, frame):
        raise KeyboardInterrupt(signal.Signals(signum).name)

    try:
        for sig in previous:
            signal.signal(sig, interrupted)
        for tag, args in (('context', ['context', 'inspect', '--format', '{{json .Endpoints.docker.Host}}', context]),
                          ('image', ['image', 'inspect', '--format', '{{.Id}}', linux.IMAGE_ID])):
            code, stdout, stderr = run([str(docker), '--context', context, *args], 10, env=env, max_output=CAP)
            linux.write_new(out / (tag + '-stdout.bin'), stdout)
            linux.write_new(out / (tag + '-stderr.bin'), stderr)
            if type(code) is not int or code != 0:
                raise ValueError('runtime preflight failed: ' + tag)
            if tag == 'context':
                result['localDockerEndpoint'] = linux.validate_local_endpoint(stdout)
            else:
                linux.validate_image_inspect(stdout)
        for arm in stager.ARMS:
            root = out / (arm + '-stage')
            receipt = stager.stage(source, root, case_id, arm, control)
            result['arms'][arm] = run_arm(docker, context, env, source, root, receipt, out, deadline, run)
            if result['arms'][arm]['status'] != 'isolation_probe_passed':
                break  # Preserve failure and stop; never silently retry or start another arm.
        if set(result['arms']) == set(stager.ARMS) and all(a['status'] == 'isolation_probe_passed' for a in result['arms'].values()):
            left, right = (result['arms'][a]['probe']['files'] for a in stager.ARMS)
            delta = set(report['arms']['with']['treatmentFiles'])
            if left != {k: v for k, v in right.items() if k not in delta} or set(right) - set(left) != delta:
                raise ValueError('observed arm parity or treatment delta differs')
            result['status'] = 'isolation_pair_passed_not_admitted'
    except BaseException as exc:
        result['error'] = type(exc).__name__ + ': ' + str(exc)[:300]
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)
        linux.write_new(out / 'receipt.json', (json.dumps(result, indent=2, sort_keys=True) + '\n').encode())
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-prep', type=Path, required=True)
    parser.add_argument('--case', required=True)
    parser.add_argument('--control')
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--docker', type=Path, default=Path('/usr/local/bin/docker'))
    parser.add_argument('--docker-context', required=True)
    args = parser.parse_args()
    try:
        result = run_pair(args.source_prep, args.case, args.control, args.out, args.docker, args.docker_context)
    except (ValueError, OSError, linux.CaptureError, stager.StageError) as exc:
        parser.exit(2, str(exc) + '\n')
    print(json.dumps({'status': result['status'], 'receipt': str(args.out / 'receipt.json')}))
    return 0 if result['status'] == 'isolation_pair_passed_not_admitted' else 1


if __name__ == '__main__':
    raise SystemExit(main())
