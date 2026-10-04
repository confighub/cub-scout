import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

import probe_context as probe


class ContextPreflightTests(unittest.TestCase):
    def receipt(self):
        return {'selection': {'case': 'RUL-03', 'arm': 'with', 'authoredInputControl': None},
                'stagedFiles': {'cluster/' + name: pin for name, pin in probe.frames.PINS.items()}}

    def replies(self):
        server = probe.mcp.ContextMCP(fixture_dir=probe.frames.FIXTURES)
        return [server.handle(json.loads(line)) for line in probe.requests().splitlines()]

    def test_exact_selection_refuses_cross_arm_case_control_or_evidence(self):
        self.assertEqual(probe._selection(self.receipt()), self.receipt()['stagedFiles'])
        mutations = []
        for key, value in [('case', 'INV-04'), ('arm', 'without'), ('authoredInputControl', 'control')]:
            row = self.receipt(); row['selection'][key] = value; mutations.append(row)
        row = self.receipt(); row['stagedFiles']['cluster/sibling.json'] = 'a' * 64; mutations.append(row)
        row = self.receipt(); row['stagedFiles']['cluster/capture-scope.json'] = 'a' * 64; mutations.append(row)
        for row in mutations:
            with self.subTest(row=row), self.assertRaises(ValueError):
                probe._selection(row)

    def test_all_exact_transport_results_are_checked(self):
        raw = b''.join(probe.mcp.encoded(row) for row in self.replies())
        checks = probe.check(raw, probe.frames.FIXTURES)
        self.assertEqual([(row['context'], row['httpStatus'], row['sourceRow']) for row in checks['responses']],
                         [('rul03-denied', 403, 0), ('rul03-readable', 200, 1)])
        self.assertTrue(checks['unsupportedContextRefused'])
        self.assertTrue(checks['mapSubstitutionRefused'])

    def test_forged_missing_reordered_duplicate_or_inferred_results_refuse(self):
        variants = []
        rows = self.replies(); variants.extend([rows[:-1], list(reversed(rows)), rows + [rows[-1]]])
        for field, value in [('httpStatus', 200), ('context', 'rul03-readable'), ('atomicSnapshot', True)]:
            rows = self.replies()
            payload = json.loads(rows[2]['result']['content'][0]['text'])
            payload['provenance'][field] = value
            rows[2]['result']['content'][0]['text'] = probe.mcp.encoded(payload).decode().rstrip('\n')
            variants.append(rows)
        rows = self.replies(); rows[2]['result'] = rows[3]['result']; variants.append(rows)
        rows = self.replies(); rows[2]['result']['isError'] = True; variants.append(rows)
        rows = self.replies(); rows[2]['result']['isError'] = 0; variants.append(rows)
        rows = self.replies(); rows[2]['id'] = 3.0; variants.append(rows)
        rows = self.replies(); rows[4]['result']['isError'] = False; variants.append(rows)
        rows = self.replies(); rows[1]['result']['tools'].append({'name': 'map'}); variants.append(rows)
        for rows in variants:
            with self.subTest(rows=rows), self.assertRaises(ValueError):
                probe.check(b''.join(probe.mcp.encoded(row) for row in rows), probe.frames.FIXTURES)
        for raw in (b'[]\n', b'{"id":1,"id":2}\n', b'x' * (probe.MAX_OUTPUT + 1)):
            with self.subTest(raw=raw[:20]), self.assertRaises(ValueError):
                probe.check(raw, probe.frames.FIXTURES)

    def test_output_guards_and_interpreter_refusal_precede_execution(self):
        with tempfile.TemporaryDirectory(dir=Path('/tmp').resolve()) as td:
            root = Path(td).resolve()
            source = root / 'source'; source.mkdir()
            stage = root / 'stage'; stage.mkdir()
            with self.assertRaises(ValueError): probe._new_output(source / 'child', source, stage)
            with self.assertRaises(ValueError): probe._new_output(root, source, stage)
            alias = root / 'alias'; alias.symlink_to(stage, target_is_directory=True)
            with self.assertRaises(ValueError): probe._new_output(alias / 'out', source, stage)
            out = probe._new_output(root / 'out', source, stage)
            self.assertEqual(out.stat().st_mode & 0o777, 0o700)
            fake = root / 'python'; fake.write_bytes(b'not python'); fake.chmod(0o700)
            with patch.object(probe.subprocess, 'Popen') as launch:
                with self.assertRaises(ValueError):
                    probe.run(source, stage, fake, '0' * 64, root / 'unused')
                launch.assert_not_called()
            self.assertFalse((root / 'unused').exists())

    def test_failed_or_timed_out_child_retains_attempt_without_success_claim(self):
        assets = {name: (probe.HELPERS / name).read_bytes() for name in probe.ADAPTER_PINS}
        assets['bootstrap.py'] = probe.BOOTSTRAP
        assets.update({'cluster/' + name: (probe.frames.FIXTURES / name).read_bytes()
                       for name in probe.frames.PINS})
        plan = {'files': {name: probe.policy.sha(raw) for name, raw in assets.items()}}
        for variant in ('exit', 'timeout', 'malformed'):
            with self.subTest(variant=variant), tempfile.TemporaryDirectory(dir=Path('/tmp').resolve()) as td:
                root = Path(td).resolve()
                source = root / 'source'; source.mkdir()
                stage = root / 'stage'; stage.mkdir()
                python = root / 'python'; python.write_bytes(b'fake interpreter'); python.chmod(0o700)
                child = MagicMock()
                child.wait.side_effect = ([probe.subprocess.TimeoutExpired('fixed-child', 15), -9]
                                          if variant == 'timeout' else [1 if variant == 'exit' else 0])
                def launcher(_argv, **kwargs):
                    kwargs['stdout'].write(b'broken\n' if variant == 'malformed' else
                        b''.join(probe.mcp.encoded(row) for row in self.replies()))
                    return child
                with patch.object(probe, 'build', return_value=(plan, assets)), \
                     patch.object(probe.subprocess, 'Popen', side_effect=launcher):
                    result = probe.run(source, stage, python, probe.policy.sha(python.read_bytes()), root / 'out')
                self.assertEqual(result['status'], 'failed')
                self.assertTrue(all(value is False for value in result['claims'].values()))
                for name in ('stdin.jsonl', 'stdout.jsonl', 'stderr.txt', 'proof.json'):
                    self.assertTrue((root / 'out' / name).is_file())
                self.assertEqual(json.loads((root / 'out/proof.json').read_bytes()), result)
                self.assertEqual(result['timedOut'], variant == 'timeout')
                if variant == 'timeout': child.kill.assert_called_once()
                else: child.kill.assert_not_called()


if __name__ == '__main__':
    unittest.main()
