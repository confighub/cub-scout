import importlib.util
from pathlib import Path
import shutil
import tempfile
import unittest
import os

spec = importlib.util.spec_from_file_location('context_frames', Path(__file__).with_name('context_frames.py'))
frames = importlib.util.module_from_spec(spec)
spec.loader.exec_module(frames)


class ContextFrameTests(unittest.TestCase):
    def test_exact_context_status_bytes_and_identity(self):
        responses = {}
        for context, status in [('rul03-denied', 403), ('rul03-readable', 200)]:
            body, provenance = frames.select_frame(context, 'GET', frames.REQUEST_PATH)
            self.assertEqual(body, (frames.FIXTURES / (context + '-deployments.body')).read_bytes())
            self.assertEqual(provenance['httpStatus'], status)
            self.assertEqual(provenance['context'], context)
            self.assertEqual(provenance['sourceObservation']['context'], context)
            self.assertEqual(provenance['observerCurrentContext'], 'rul03-readable')
            self.assertFalse(provenance['atomicSnapshot'])
            self.assertFalse(provenance['currentStateEstablished'])
            self.assertFalse(provenance['runtimeAdmission'])
            responses[context] = provenance
        self.assertNotEqual(responses['rul03-denied']['endpoint'], responses['rul03-readable']['endpoint'])
        self.assertEqual(responses['rul03-denied']['sourceRow'], 0)
        self.assertEqual(responses['rul03-readable']['sourceRow'], 1)

    def test_unknown_context_method_and_query_refuse_before_reading(self):
        for context, method, path in [('', 'GET', frames.REQUEST_PATH), (None, 'GET', frames.REQUEST_PATH),
                ('current-context', 'GET', frames.REQUEST_PATH), ('RUL03-DENIED', 'GET', frames.REQUEST_PATH),
                ('rul03-denied', 'POST', frames.REQUEST_PATH),
                ('rul03-denied', 'GET', frames.REQUEST_PATH + '?limit=500'),
                ('rul03-denied', 'GET', frames.REQUEST_PATH + '/')]:
            with self.subTest(context=context, method=method, path=path), self.assertRaises(frames.FrameError):
                frames.select_frame(context, method, path, fixture_dir=Path('/missing'))

    def test_selected_denial_never_reads_readable_body(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td).resolve() / 'fixtures'
            shutil.copytree(frames.FIXTURES, root)
            (root / 'rul03-readable-deployments.body').unlink()
            body, provenance = frames.select_frame('rul03-denied', 'GET', frames.REQUEST_PATH, fixture_dir=root)
            self.assertEqual(provenance['httpStatus'], 403)
            self.assertIn(b'Forbidden', body)
            (root / 'rul03-denied-deployments.body').unlink()
            with self.assertRaises(frames.FrameError):
                frames.select_frame('rul03-denied', 'GET', frames.REQUEST_PATH, fixture_dir=root)

    def test_each_selected_source_pin_rejects_drift(self):
        for filename in ('capture-scope.json', 'observer-context-map.json', 'rul03-denied-deployments.body'):
            with self.subTest(filename=filename), tempfile.TemporaryDirectory() as td:
                root = Path(td).resolve() / 'fixtures'
                shutil.copytree(frames.FIXTURES, root)
                with (root / filename).open('ab') as stream:
                    stream.write(b' ')
                with self.assertRaises(frames.FrameError):
                    frames.select_frame('rul03-denied', 'GET', frames.REQUEST_PATH, fixture_dir=root)

    def test_unsafe_and_oversized_sources_refuse(self):
        for variant in ('symlink', 'parent-symlink', 'fifo', 'oversized'):
            with self.subTest(variant=variant), tempfile.TemporaryDirectory() as td:
                parent = Path(td).resolve()
                root = parent / 'fixtures'
                shutil.copytree(frames.FIXTURES, root)
                target = root / 'rul03-denied-deployments.body'
                if variant == 'parent-symlink':
                    alias = parent / 'alias'
                    alias.symlink_to(root, target_is_directory=True)
                    root = alias
                else:
                    target.unlink()
                    if variant == 'symlink':
                        target.symlink_to(frames.FIXTURES / target.name)
                    elif variant == 'fifo':
                        os.mkfifo(target)
                    else:
                        target.write_bytes(b'x' * (frames.MAX_BYTES + 1))
                with self.assertRaises(frames.FrameError):
                    frames.select_frame('rul03-denied', 'GET', frames.REQUEST_PATH, fixture_dir=root)


if __name__ == '__main__':
    unittest.main()
