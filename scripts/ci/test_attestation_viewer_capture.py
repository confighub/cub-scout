"""Guard genuine viewer projection and denial records, not effective coverage."""
import hashlib
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2] / 'test/fixtures/confighub-attestations-v083-viewer'


def read(name):
    return json.loads((ROOT / name).read_text())


class AttestationViewerCaptureTests(unittest.TestCase):
    def test_unedited_files_are_bound_and_credentials_are_absent(self):
        manifest = read('capture-manifest.json')
        self.assertEqual(len(manifest['publicFiles']), 12)
        for name, digest in manifest['publicFiles'].items():
            data = (ROOT / name).read_bytes()
            self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
            for credential in (b'accessToken', b'refreshToken', b'WorkerSecret', b'PRIVATE KEY'):
                self.assertNotIn(credential, data)
        self.assertEqual(manifest['tokenClaimSubset']['role'], 'viewer')
        self.assertEqual(manifest['tokenClaimSubset']['internal_user_id'], read('identity.json')['userID'])
        self.assertFalse(manifest['workspaceDirty'])
        self.assertFalse(manifest['effectiveInheritedCoverageEstablished'])

    def test_cli_mcp_and_receipt_retain_same_exact_direct_claims(self):
        mcp = read('viewer-scout-mcp.stdout')['result']
        self.assertFalse(mcp.get('isError', False))
        evidence = json.loads(mcp['content'][0]['text'])
        self.assertEqual(evidence['coverage'], 'direct-references-only')
        self.assertEqual(len(evidence['attestations']), 3)
        revision = read('viewer-revision.stdout')['Revision']
        self.assertEqual((evidence['spaceId'], evidence['unitId'], evidence['revisionId'], evidence['revisionNum']),
                         (revision['SpaceID'], revision['UnitID'], revision['RevisionID'], revision['RevisionNum']))
        for name in ('viewer-trace.stdout', 'viewer-explain.stdout'):
            self.assertEqual(read(name)['deliveryEvidence']['attestations']['attestations'], evidence['attestations'])
        receipt = read('viewer-receipt.stdout')
        self.assertEqual(receipt['predicate']['evidence']['attestations']['attestations'], evidence['attestations'])
        self.assertEqual(len(receipt['subject']), 2)
        self.assertEqual(receipt['subject'][1]['digest']['confighub-data-sha256'], revision['DataHash'])
        validation = read('receipt-validation.stdout')
        self.assertTrue(validation['valid'])
        self.assertEqual(validation['fingerprint'], receipt['predicate']['fingerprint'])

    def test_write_denied_and_removed_reads_do_not_become_empty_coverage(self):
        denial = (ROOT / 'viewer-write-denied.stderr').read_text()
        self.assertIn('HTTP 403', denial)
        self.assertIn('permission denied', denial)
        self.assertTrue(read('viewer-scout-mcp-denied.stdout')['result']['isError'])
        delivery = read('viewer-explain-denied.stdout')['deliveryEvidence']
        self.assertFalse(delivery.get('attestations'))
        self.assertTrue(any(o['layer'] == 'confighub.attestations' for o in delivery['omissions']))

    def test_failure_history_and_owned_cleanup_remain_visible(self):
        manifest = read('capture-manifest.json')
        self.assertEqual(len(manifest['retainedAttempts']), 3)
        self.assertIn('failed', manifest['retainedAttempts'][0]['status'])
        self.assertEqual(manifest['status'], 'passed')
        self.assertTrue(manifest['ownedSpaceAbsent'])
        self.assertTrue(manifest['unitUnchanged'])
        self.assertTrue(manifest['originalSpaceActivePermissionsRestored'])
        self.assertEqual({c['action'] for c in manifest['cleanup']},
                         {'delete-namespace', 'delete-worker', 'delete-space'})
        self.assertTrue(all(c['exitCode'] == 0 for c in manifest['cleanup']))


if __name__ == '__main__':
    unittest.main()
