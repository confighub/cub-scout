"""Guard the retained Python checkpoint without admitting a model runtime."""
import hashlib
import json
from pathlib import Path
import unittest

REPO = Path(__file__).resolve().parents[2]
ROOT = REPO / 'evals/python-runtime-substrate'


class PythonSubstrateCaptureTests(unittest.TestCase):
    def setUp(self):
        self.report = json.loads((ROOT / 'report.json').read_text())

    def test_all_attempts_and_unedited_outputs_are_retained(self):
        self.assertEqual([a['attempt'] for a in self.report['attempts']], [1, 2, 3, 4])
        for attempt in self.report['attempts']:
            directory = ROOT / 'evidence' / f"attempt{attempt['attempt']}"
            self.assertEqual(set(attempt['files']), {p.name for p in directory.iterdir()})
            for name, expected in attempt['files'].items():
                self.assertEqual(hashlib.sha256((directory / name).read_bytes()).hexdigest(), expected)
            proof = json.loads((directory / 'proof.json').read_text())
            self.assertFalse(proof['sourceDirty'])
            self.assertFalse(proof['modelOrProviderRun'])
            self.assertEqual(proof['sourceCommit'], self.report['sourceCommit'])
            self.assertTrue(proof['cleanup']['verifiedAbsent'])
            self.assertEqual(proof['cleanup']['errors'], [])
            for operation in proof['operations']:
                for stream in ('stdout', 'stderr'):
                    name = operation['label'] + '.' + stream
                    if name in attempt['files']:
                        self.assertEqual(operation[stream + 'SHA256'], attempt['files'][name])

    def test_failures_are_not_relabelled_as_dependency_acceptance(self):
        first = json.loads((ROOT / 'evidence/attempt1/proof.json').read_text())
        self.assertNotIn('runtime', first)
        self.assertFalse(any(o['label'] == 'python-execution' for o in first['operations']))
        second = json.loads((ROOT / 'evidence/attempt2/proof.json').read_text())
        self.assertFalse(second['runtime']['yamlAvailable'])
        self.assertEqual(second['status'], 'stdlib_passed_required_pyyaml_missing')

    def test_executed_assets_match_unchanged_static_inventory(self):
        inventory = json.loads((REPO / 'evals/full24-launch-policy/runtime-python-candidate.json').read_text())
        runtime = self.report['runtime']
        self.assertEqual(self.report['imageIndexDigest'], inventory['imageIndexDigest'])
        for path, digest in runtime['assetSHA256'].items():
            self.assertEqual(digest, inventory['pythonAssets'][path.lstrip('/')]['sha256'])
        final = json.loads((ROOT / 'evidence/attempt4/python-execution.stdout').read_text())
        self.assertEqual(final, runtime)
        self.assertEqual((runtime['uid'], runtime['isolated'], runtime['noSite']), (65534, 1, 1))
        self.assertEqual(runtime['child'], {'ok': True, 'uid': 65534})
        self.assertTrue(runtime['yamlCompiledExtension'])
        self.assertEqual(runtime['yamlVersion'], '6.0.3')
        self.assertFalse(inventory['containerStarted'])  # Earlier static proof stays static.

    def test_dependency_and_helpers_remain_bound(self):
        acquisition = self.report['acquisition']
        self.assertEqual(hashlib.sha256((REPO / acquisition['requirementsFile']).read_bytes()).hexdigest(),
                         acquisition['requirementsSHA256'])
        helper = self.report['harnessSource']
        self.assertEqual(hashlib.sha256((REPO / helper['limitsHelper']).read_bytes()).hexdigest(),
                         helper['limitsHelperSHA256'])
        for number in (3, 4):
            proof = json.loads((ROOT / f'evidence/attempt{number}/proof.json').read_text())
            self.assertEqual(proof['wheelAcquisition'], acquisition)
            self.assertTrue(proof['wheelFilesUnchanged'])

    def test_final_inspection_has_required_bounds_and_no_model_claim(self):
        before = json.loads((ROOT / 'evidence/attempt4/before.stdout').read_text())
        after = json.loads((ROOT / 'evidence/attempt4/after.stdout').read_text())
        self.assertEqual(before['Id'], after['Id'])
        self.assertEqual(before['Image'], self.report['imageIndexDigest'])
        self.assertEqual(before['Args'], ['-I', '-S', '/tools/payload.py'])
        self.assertEqual(before['Config']['User'], '65534:65534')
        host = before['HostConfig']
        self.assertEqual(host['NetworkMode'], 'none')
        self.assertTrue(host['ReadonlyRootfs'])
        self.assertFalse(host['Privileged'])
        self.assertEqual(host['CapDrop'], ['ALL'])
        self.assertIn('no-new-privileges', host['SecurityOpt'])
        self.assertEqual((host['PidsLimit'], host['Memory'], host['NanoCpus']),
                         (64, 1073741824, 1000000000))
        self.assertEqual(len(before['Mounts']), 1)
        self.assertEqual(before['Mounts'][0]['Destination'], '/tools')
        self.assertFalse(before['Mounts'][0]['RW'])
        self.assertEqual(after['State']['ExitCode'], 0)
        self.assertFalse(after['State']['Running'])
        self.assertFalse(self.report['modelOrProviderRun'])
        self.assertFalse(self.report['frozenCasesPromptsEvidenceGrantsBudgetsGradersChanged'])
        self.assertEqual(self.report['runtimeAssetAdmission'], 'still blocked')

    def test_linux_recorded_checkpoint_retains_selected_source_bindings(self):
        latest = json.loads((REPO / 'evals/linux-recorded-substrate/report.json').read_text())
        historical = json.loads((REPO / 'evals/full24-launch-policy/local-recorded-proof.json').read_text())
        packaging = json.loads((REPO / 'docs/releases/v2.13-goreleaser-e94ce9cf.json').read_text())['artifacts']
        self.assertEqual(latest['binarySourceCommit'], packaging['sourceRevision'])
        binaries = [b for b in packaging['binaries'] if b['path'] == 'cub-scout_linux_arm64_v8.0/cub-scout']
        self.assertEqual(len(binaries), 1)
        self.assertEqual(latest['binarySHA256'], binaries[0]['sha256'])
        self.assertEqual(latest['imageIndexDigest'], self.report['imageIndexDigest'])
        self.assertEqual(latest['status'], 'passed')
        self.assertFalse(latest['workspaceDirty'])
        for claim in ('modelOrProviderRun', 'frozenInputsChanged', 'fullRuntimeAdmitted'):
            self.assertFalse(latest[claim])
        original = {row['case']: row for row in historical['cases']}
        self.assertEqual({row['case'] for row in latest['cases']}, set(original))
        self.assertEqual(len(latest['cases']), 11)
        wrapper_sha = hashlib.sha256((REPO / 'evals/full24-launch-policy/recorded_server.py').read_bytes()).hexdigest()
        for row in latest['cases']:
            prior = original[row['case']]
            self.assertEqual(list(row['selectedRecording'].values()), [prior['recordingSha256']])
            self.assertEqual(row['requestSHA256'], prior['requestSha256'])
            self.assertEqual(row['wrapperSHA256'], wrapper_sha)
            self.assertEqual(row['observed'], prior['observed'])
            self.assertTrue(row['ownedContainerAbsent'])
            self.assertTrue(row['stagedFilesUnchanged'])
            self.assertEqual(row['cleanupErrors'], [])


if __name__ == '__main__':
    unittest.main()
