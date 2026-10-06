"""Check the retained native Windows receipts, without claiming current runtime."""
import hashlib
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2] / 'docs/releases/v2.13-windows-runtime'


class NativeWindowsCaptureTests(unittest.TestCase):
    root = ROOT

    def test_unedited_public_records_match_manifest(self):
        manifest = json.loads((self.root / 'capture-manifest.json').read_text())
        self.assertEqual(len(manifest['publicRecords']), 6)
        for record in manifest['publicRecords']:
            data = (self.root / record['path']).read_bytes()
            self.assertEqual(len(data), record['bytes'])
            self.assertEqual(hashlib.sha256(data).hexdigest(), record['sha256'])

    def test_exact_native_source_metadata_and_module_identity(self):
        manifest = json.loads((self.root / 'capture-manifest.json').read_text())
        for arch in ('amd64', 'arm64'):
            runtime = json.loads((self.root / arch / 'runtime.json').read_text())
            metadata = json.loads((self.root / arch / 'metadata.json').read_text())
            self.assertEqual(runtime['sourceRevision'], manifest['sourceRevision'])
            self.assertEqual(runtime['metadata'], metadata)
            self.assertEqual(metadata['commit'], manifest['sourceRevision'])
            self.assertEqual(metadata['runtime'], {'goos': 'windows', 'goarch': arch})
            self.assertEqual(metadata['version'], 'v2.13.0-next')
            self.assertEqual(len(runtime['binaries']), 2)
            for binary in runtime['binaries']:
                for expected in ('go1.24.0', 'github.com/confighub/cub-scout/v2',
                                 'vcs.revision=' + manifest['sourceRevision'],
                                 'vcs.modified=false', 'CGO_ENABLED=0',
                                 'GOARCH=' + arch, 'GOOS=windows'):
                    self.assertIn(expected, binary['buildInfo'])
                matches = [r for r in manifest['privateArtifactInventory']
                           if r['path'].endswith(binary['path'].replace('\\', '/'))]
                self.assertEqual(len(matches), 1)
                self.assertEqual(matches[0]['sha256'], binary['sha256'])
                self.assertEqual(matches[0]['bytes'], binary['bytes'])

    def test_all_eight_actual_commands_pass_without_publication_claim(self):
        count = 0
        for arch in ('amd64', 'arm64'):
            runtime = json.loads((self.root / arch / 'runtime.json').read_text())
            self.assertFalse(runtime['claims']['published'])
            self.assertFalse(runtime['claims']['clusterBehaviorAccepted'])
            self.assertFalse(runtime['claims']['publicInstallAccepted'])
            for binary in runtime['binaries']:
                self.assertEqual([e['args'] for e in binary['executions']],
                                 [['version'], ['help', 'trace']])
                for execution in binary['executions']:
                    self.assertEqual(execution['exit'], 0)
                    expected = 'v2.13.0-next' if execution['args'] == ['version'] else 'cub-scout trace'
                    self.assertIn(expected, execution['stdout'])
                    count += 1
        self.assertEqual(count, 8)

    def test_canonical_input_receipts_agree_across_native_architectures(self):
        amd64 = json.loads((self.root / 'amd64/source-inputs.json').read_text())
        arm64 = json.loads((self.root / 'arm64/source-inputs.json').read_text())
        self.assertEqual(amd64['inputs'], arm64['inputs'])
        self.assertEqual(amd64['sourceRevision'], arm64['sourceRevision'])
        self.assertEqual({r['path'] for r in amd64['inputs']}, {'go.mod', 'go.sum'})
        for record in (amd64, arm64):
            self.assertTrue(record['claims']['canonicalSourceInputsVerified'])
            self.assertFalse(record['claims']['runtimeAccepted'])

class LatestNativeWindowsCaptureTests(NativeWindowsCaptureTests):
    root = ROOT.parent / 'v2.13-windows-e94ce9cf'


if __name__ == '__main__':
    unittest.main()
