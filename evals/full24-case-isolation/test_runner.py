"""Offline isolation guards. Docker responses are authored; no daemon is used."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import sys
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location('full24_isolation', HERE / 'run_pair.py')
runtime = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runtime)


def writable(root):
    for path in sorted(root.rglob('*'), reverse=True):
        if not path.is_symlink():
            os.chmod(path, 0o700 if path.is_dir() else 0o600)
    root.chmod(0o700)


class FakeDocker:
    def __init__(self, *, drift=None, final_drift=None, fail=None, cleanup_missing=False, probe_drift=False, mutate=None, cleanup_mutate=None):
        self.calls = []
        self.objects = {}
        self.drift = drift
        self.final_drift = final_drift
        self.fail = fail
        self.cleanup_missing = cleanup_missing
        self.probe_drift = probe_drift
        self.mutate = mutate
        self.cleanup_mutate = cleanup_mutate

    def __call__(self, argv, seconds, *, env, max_output):
        self.assert_profile(env, seconds, max_output)
        args = argv[3:]
        self.calls.append(args)
        if args[:2] == ['context', 'inspect']:
            return 0, b'"unix:///fake-local.sock"', b''
        if args[:2] == ['image', 'inspect']:
            return 0, runtime.linux.IMAGE_ID.encode() + b'\n', b''
        action = args[1]
        if action == 'create':
            name = args[args.index('--name') + 1]
            owner = args[args.index('--label') + 1].split('=', 1)[1]
            root = Path(args[args.index('--mount') + 1].split('src=', 1)[1].split(',dst=', 1)[0])
            ident = ('a' if not self.objects else 'b') * 64
            self.objects[ident] = {'name': name, 'owner': owner, 'root': root, 'started': False, 'removed': False}
            if self.fail == 'create-lost':
                raise TimeoutError('lost create response')
            return 0, ident.encode() + b'\n', b''
        target = args[-1]
        item = self.objects.get(target)
        if item is None:
            item = next((v for v in self.objects.values() if v['name'] == target), None)
            if item is not None:
                target = next(k for k, v in self.objects.items() if v is item)
        if action == 'inspect':
            if item is None or item['removed']:
                if self.cleanup_missing:
                    return 1, b'', b'connection refused'
                return 1, b'', ('Error response from daemon: No such container: ' + target + '\n').encode()
            obj = self.inspect_object(target, item)
            if self.drift:
                self.drift(obj)
            if self.final_drift and item['started']:
                self.final_drift(obj)
            return 0, json.dumps(obj).encode(), b''
        if action == 'rm':
            if self.cleanup_mutate:
                self.cleanup_mutate(item['root'])
            item['removed'] = True
            return 0, target.encode() + b'\n', b''
        if self.fail == action:
            return 1, b'', b'authored command failure'
        if self.fail == 'interrupt' and action == 'wait':
            raise KeyboardInterrupt('authored interrupt')
        if self.fail == 'wait-timeout' and action == 'wait':
            raise TimeoutError('authored wait timeout')
        if action == 'start':
            item['started'] = True
            return 0, target.encode() + b'\n', b''
        if action == 'wait':
            return 0, b'0\n', b''
        if action == 'logs':
            files = runtime.stager._hash_tree(item['root'])
            value = {'schema': 'full24-case-isolation-probe.v1', 'files': files,
                     'bytes': sum((item['root'] / p).stat().st_size for p in files),
                     'uid': 65534, 'gid': 65534, 'interfaces': ['lo'],
                     'writeDenied': ['/tools/.write-control', '/tmp/stage-alias/.write-control', '/root-write-control']}
            if self.probe_drift:
                value['files']['oracle/answer.md'] = '0' * 64
            if self.mutate:
                self.mutate(item['root'])
            return 0, json.dumps(value).encode(), b''
        raise AssertionError('unexpected fake Docker operation: ' + str(args))

    @staticmethod
    def assert_profile(env, seconds, max_output):
        assert set(env) <= {'PATH', 'HOME', 'DOCKER_CONFIG', 'TMPDIR', 'LANG'}
        assert seconds > 0 and max_output <= runtime.CAP

    @staticmethod
    def inspect_object(ident, item):
        return {'Id': ident, 'Name': '/' + item['name'], 'Image': runtime.linux.IMAGE_ID,
                'Config': {'Image': runtime.linux.IMAGE_ID, 'User': '65534:65534', 'Cmd': runtime.command(),
                           'Entrypoint': None, 'Labels': {runtime.linux.OWNER_LABEL: item['owner']}},
                'HostConfig': {'ReadonlyRootfs': True, 'NetworkMode': 'none', 'CapDrop': ['ALL'],
                               'CapAdd': None, 'PidMode': '', 'IpcMode': 'private',
                               'SecurityOpt': ['no-new-privileges'], 'PidsLimit': 64,
                               'Memory': 1073741824, 'NanoCpus': 1000000000, 'Privileged': False,
                               'Tmpfs': {'/tmp': 'rw,nosuid,nodev,size=64m,uid=65534,gid=65534'}},
                'Mounts': [{'Type': 'bind', 'Source': str(item['root']), 'Destination': '/tools', 'RW': False}],
                'State': {'Status': 'exited' if item['started'] else 'created', 'ExitCode': 0, 'OOMKilled': False}}


class IsolationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='full24-isolation-', dir=Path('/tmp').resolve())
        cls.root = Path(cls.temp.name).resolve()
        cls.source = cls.root / 'source'
        cls.source.mkdir()
        cls.report = runtime.stager.prepare.prepare(cls.source)
        runtime.stager.prepare.validate(cls.source)
        cls.report_bytes = (cls.source / 'preflight.json').read_bytes()
        cls.counter = 0

    @classmethod
    def tearDownClass(cls):
        writable(cls.root)
        cls.temp.cleanup()

    def run_fake(self, docker, case='INV-04', control=None):
        type(self).counter += 1
        out = self.root / ('run-' + str(self.counter))
        # Existing stager tests cover full source revalidation. Cache that one
        # immutable report here; exact stage verification still runs for every arm.
        def stage(source, dest, cid, arm, ctl):
            report = copy.deepcopy(self.report)
            report['_source_root'] = str(source)
            return runtime.stager._stage_validated(source, dest, cid, arm, ctl, report, self.report_bytes)

        def verify(root, source, cid, arm, ctl):
            receipt = json.loads((root / 'receipt.json').read_bytes())
            self.assertEqual(receipt['selection'], {'case': cid, 'arm': arm, 'authoredInputControl': ctl})
            return runtime.stager._verify_validated_stage(root, receipt, source, copy.deepcopy(self.report), self.report_bytes)

        with patch.object(runtime.stager.prepare, 'validate', return_value=copy.deepcopy(self.report)), \
             patch.object(runtime.stager, 'stage', side_effect=stage), patch.object(runtime.stager, 'verify', side_effect=verify):
            result = runtime.run_pair(self.source, case, control, out, Path(sys.executable), 'offline', runner=docker)
        self.assertEqual(json.loads((out / 'receipt.json').read_bytes()), result)
        return result, out

    def test_all_cases_and_controls_mount_only_exact_selected_tree(self):
        for cid in runtime.stager.prepare.FROZEN_CASE_IDS:
            controls = [None]
            if cid in ('DEL-03', 'DEL-04'):
                controls.append(self.report['authoredInputControls'][cid]['id'])
            for control in controls:
                docker = FakeDocker()
                result, out = self.run_fake(docker, cid, control)
                self.assertEqual(result['status'], 'isolation_pair_passed_not_admitted')
                self.assertTrue(all(value is False for value in result['claims'].values()))
                creates = [c for c in docker.calls if c[:2] == ['container', 'create']]
                self.assertEqual(len(creates), 2)
                for arm, args in zip(runtime.stager.ARMS, creates):
                    self.assertEqual(args.count('--mount'), 1)
                    self.assertEqual(args[args.index('--mount') + 1], 'type=bind,src=' + str(out / (arm + '-stage/model-stage')) + ',dst=/tools,readonly')
                    self.assertIn('--pull=never', args)
                    self.assertEqual(args[args.index(runtime.linux.IMAGE_ID) + 1:], runtime.command())

    def test_inspect_drift_never_starts_container(self):
        mutations = [lambda x: x['Mounts'].append({'Type': 'bind', 'Source': str(self.source), 'Destination': '/oracle', 'RW': False}),
                     lambda x: x['Mounts'][0].update(RW=True),
                     lambda x: x['HostConfig'].update(NetworkMode='host'),
                     lambda x: x['HostConfig']['Tmpfs'].update({'/extra': 'rw'}),
                     lambda x: x['Config'].update(Cmd=['sh']),
                     lambda x: x['Config'].update(Entrypoint=['sh']),
                     lambda x: x['HostConfig'].update(Devices=[{'PathOnHost': '/dev/sda'}]),
                     lambda x: x.update(Id='f' * 64)]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                docker = FakeDocker(drift=mutation)
                result, _ = self.run_fake(docker)
                self.assertEqual(result['status'], 'failed')
                self.assertFalse(any(c[:2] == ['container', 'start'] for c in docker.calls))

    def test_failures_interruptions_and_lost_create_retain_cleanup(self):
        for action in ('create-lost', 'start', 'wait', 'wait-timeout', 'logs', 'interrupt'):
            with self.subTest(action=action):
                docker = FakeDocker(fail=action)
                result, out = self.run_fake(docker)
                self.assertEqual(result['status'], 'failed')
                self.assertEqual(set(result['arms']), {'without'})
                self.assertTrue(result['arms']['without']['cleanup']['verifiedAbsent'])
                self.assertTrue((out / 'without-receipt.json').is_file())

    def test_final_inspection_rejects_drift_after_start(self):
        mutations = [lambda x: x['HostConfig'].update(NetworkMode='host'),
                     lambda x: x['Mounts'][0].update(RW=True),
                     lambda x: x['Config'].update(Cmd=['sh']),
                     lambda x: x['State'].update(OOMKilled=True),
                     lambda x: x['State'].update(ExitCode=3),
                     lambda x: x['State'].update(Status='running')]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                docker = FakeDocker(final_drift=mutation)
                result, _ = self.run_fake(docker)
                self.assertEqual(result['status'], 'failed')
                self.assertTrue(any(c[:2] == ['container', 'start'] for c in docker.calls))
                self.assertFalse(any(c[:2] == ['container', 'logs'] for c in docker.calls))
                self.assertTrue(result['arms']['without']['cleanup']['verifiedAbsent'])

    def test_uncertain_cleanup_or_changed_probe_cannot_pass(self):
        for docker in (FakeDocker(cleanup_missing=True), FakeDocker(probe_drift=True)):
            result, _ = self.run_fake(docker)
            self.assertEqual(result['status'], 'failed')

    def test_input_mutation_is_failed_even_with_pre_mutation_probe(self):
        def mutate(root):
            path = root / 'prompt.md'
            path.chmod(0o600)
            path.write_bytes(b'x' * path.stat().st_size)
            path.chmod(0o444)
        result, _ = self.run_fake(FakeDocker(mutate=mutate))
        self.assertEqual(result['status'], 'failed')
        self.assertIn('StageError', result['arms']['without']['error'])

    def test_invalid_selection_fails_before_docker(self):
        for cid, control in (('UNKNOWN', None), ('INV-04', 'not-a-control')):
            docker = FakeDocker()
            with self.assertRaises(ValueError):
                runtime.run_pair(self.source, cid, control, self.root / 'bad', Path(sys.executable), 'offline', runner=docker)
            self.assertEqual(docker.calls, [])

    def test_unsafe_output_paths_fail_before_docker(self):
        symlink = self.root / 'source-alias'
        symlink.symlink_to(self.source, target_is_directory=True)
        for source, dest in ((self.source, self.source / 'nested'),
                             (self.source, self.root),
                             (symlink, self.root / 'unused'),
                             (self.source, self.root / 'comma,path')):
            docker = FakeDocker()
            with self.assertRaises((ValueError, runtime.linux.CaptureError)):
                runtime.run_pair(source, 'INV-04', None, dest, Path(sys.executable), 'offline', runner=docker)
            self.assertEqual(docker.calls, [])

    def test_cleanup_artifacts_and_post_cleanup_integrity(self):
        result, out = self.run_fake(FakeDocker())
        self.assertEqual(result['status'], 'isolation_pair_passed_not_admitted')
        for arm in result['arms'].values():
            self.assertEqual([op['argv'][1] for op in arm['operations'][-3:]], ['inspect', 'rm', 'inspect'])
            for op in arm['operations'][-3:]:
                self.assertEqual(runtime.linux.sha((out / op['stdout']).read_bytes()), op['stdoutSha256'])
                self.assertEqual(runtime.linux.sha((out / op['stderr']).read_bytes()), op['stderrSha256'])

        def mutate(root):
            path = root / 'prompt.md'
            path.chmod(0o600)
            path.write_bytes(b'x' * path.stat().st_size)
            path.chmod(0o444)
        result, _ = self.run_fake(FakeDocker(cleanup_mutate=mutate))
        self.assertEqual(result['status'], 'failed')
        self.assertTrue(result['arms']['without']['cleanup']['verifiedAbsent'])
        self.assertIn('StageError', result['arms']['without']['error'])

    def test_invalid_source_fails_before_docker(self):
        report = self.source / 'preflight.json'
        original = report.read_bytes()
        docker = FakeDocker()
        try:
            report.write_bytes(b'{}')
            with self.assertRaises(ValueError):
                runtime.run_pair(self.source, 'INV-04', None, self.root / 'bad-source', Path(sys.executable), 'offline', runner=docker)
            self.assertEqual(docker.calls, [])
        finally:
            report.write_bytes(original)


if __name__ == '__main__':
    unittest.main()
