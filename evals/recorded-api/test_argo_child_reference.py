import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import argo_child_reference as ref


class DeclaredChildReferenceTests(unittest.TestCase):
    def edited(self, edit):
        tmp = tempfile.TemporaryDirectory(); self.addCleanup(tmp.cleanup)
        root = Path(tmp.name).resolve()
        values = {n: json.loads((ref.FIXTURES / n).read_bytes()) for n in ref.PINS}
        edit(values)
        pins = {}
        for name in ('argocd-core-root.json', 'argocd-core-child.json'):
            body = json.dumps(values[name]).encode(); (root / name).write_bytes(body)
            pins[name] = hashlib.sha256(body).hexdigest()
            values['capture-scope.json']['source_file_sha256'][name] = pins[name]
        body = json.dumps(values['capture-scope.json']).encode()
        (root / 'capture-scope.json').write_bytes(body); pins['capture-scope.json'] = hashlib.sha256(body).hexdigest()
        return root, pins

    def test_host_proof_binds_current_source_and_keeps_admission_unclaimed(self):
        proof = json.loads(Path(ref.__file__).with_name('local-argo-reference-proof.json').read_bytes())
        for name in ('argo_child_reference.py', 'context_frames.py'):
            self.assertEqual(proof['packageFileSha256'][name], hashlib.sha256(Path(ref.__file__).with_name(name).read_bytes()).hexdigest())
        self.assertEqual(proof['argv'][0], '<host-python>')
        self.assertEqual(proof['argv'][1:4], ['-I', '-S', '-B'])
        self.assertFalse(proof['modelOrProviderRun'])
        self.assertFalse(proof['clusterOrContainerRun'])
        self.assertRegex(proof['privateProofSha256'], r'^[0-9a-f]{64}$')

    def test_genuine_pinned_join_keeps_declared_and_observed_type_separate(self):
        result = ref.project_reference()
        self.assertEqual(result['declaredTarget'], {'group': 'argoproj.io', 'version': 'v1alpha1',
                         'kind': 'Application', 'namespace': 'argocd', 'name': 'spark-parity'})
        self.assertEqual(result['parent']['reported']['health']['status'], 'Healthy')
        self.assertEqual(result['child']['reported']['health']['status'], 'Progressing')
        self.assertEqual(result['parent']['observedGVK'], 'unknown')
        self.assertEqual(result['child']['observedGVK'], 'unknown')
        self.assertNotEqual(result['parent']['capturedIdentity']['uid'], result['child']['capturedIdentity']['uid'])
        self.assertEqual(result['uidForeignKey'], 'not supplied')
        self.assertFalse(result['atomicSnapshot']); self.assertFalse(result['fullKubernetesObjectSnapshot'])
        self.assertEqual(result['responseCaptureTime'], 'unknown')
        self.assertEqual(result['currentState'], 'not established')
        self.assertEqual(result['child']['resourceCount'], 10)
        for name, source in result['sources'].items():
            self.assertEqual(source['sha256'], ref.PINS[name])

    def test_missing_drift_and_unavailable_sources_refuse(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            with self.assertRaises(ref.frames.FrameError): ref.project_reference(fixture_dir=root)
            for name in ref.PINS: (root / name).write_bytes((ref.FIXTURES / name).read_bytes())
            (root / 'argocd-core-child.json').write_bytes(b'{}')
            with self.assertRaisesRegex(ref.frames.FrameError, 'source pin differs'):
                ref.project_reference(fixture_dir=root)

    def test_wrong_ambiguous_missing_and_unsupported_target_refuse(self):
        def refs(values): return values['argocd-core-root.json']['status']['resources']
        edits = [lambda v: refs(v).clear(), lambda v: refs(v).append(copy.deepcopy(refs(v)[1])),
                 lambda v: refs(v)[1].update(namespace='wrong'),
                 lambda v: refs(v)[1].update(version='v2'),
                 lambda v: refs(v)[1].pop('kind')]
        # Find the tracked child row rather than depend on capture array order.
        original = json.loads((ref.FIXTURES / 'argocd-core-root.json').read_bytes())
        index = next(i for i,r in enumerate(original['status']['resources']) if r['name'] == 'spark-parity')
        for edit in edits:
            def normalize_then_edit(v):
                rows = refs(v); rows[1], rows[index] = rows[index], rows[1]; edit(v)
            root, pins = self.edited(normalize_then_edit)
            with patch.object(ref, 'PINS', pins), self.assertRaises(ref.frames.FrameError):
                ref.project_reference(fixture_dir=root)

    def test_tracking_uid_and_observed_gvk_overlays_refuse(self):
        edits = [lambda v: v['argocd-core-child.json']['metadata']['annotations'].update({'argocd.argoproj.io/tracking-id': 'wrong'}),
                 lambda v: v['argocd-core-child.json']['metadata'].update(uid=v['argocd-core-root.json']['metadata']['uid']),
                 lambda v: v['argocd-core-child.json'].update(apiVersion='argoproj.io/v1alpha1',kind='Application'),
                 lambda v: v['argocd-core-child.json']['metadata'].pop('namespace')]
        for edit in edits:
            root,pins = self.edited(edit)
            with patch.object(ref,'PINS',pins), self.assertRaises(ref.frames.FrameError): ref.project_reference(fixture_dir=root)

    def test_malformed_and_oversized_reports_refuse(self):
        edits = [lambda v: v['argocd-core-child.json']['metadata'].update(name='\ud800'),
                 lambda v: v['argocd-core-child.json']['status'].update(health='not an object'),
                 lambda v: v['argocd-core-child.json']['status'].update(resources=[None]),
                 lambda v: v['argocd-core-child.json']['metadata'].update(name='x' * 513),
                 lambda v: v['argocd-core-child.json']['status']['resources'][0].pop('name')]
        for edit in edits:
            root, pins = self.edited(edit)
            with patch.object(ref, 'PINS', pins), self.assertRaises(ref.frames.FrameError):
                ref.project_reference(fixture_dir=root)

    def test_resource_bound_order_core_group_and_output_bound(self):
        def edit(v):
            rows=v['argocd-core-child.json']['status']['resources']
            rows.extend(copy.deepcopy(rows) * 4)
            rows[0]['group']=''
        root,pins=self.edited(edit)
        with patch.object(ref,'PINS',pins):
            result=ref.project_reference(fixture_dir=root)
            self.assertEqual(len(result['child']['resources']),32)
            self.assertEqual(result['child']['omittedResourceCount'],18)
            self.assertTrue(any(r.get('group')=='' for r in result['child']['resources']))
        def reverse(v): edit(v);v['argocd-core-child.json']['status']['resources'].reverse()
        other,otherpins=self.edited(reverse)
        with patch.object(ref,'PINS',otherpins):
            self.assertEqual(ref.project_reference(fixture_dir=other)['child']['resources'],result['child']['resources'])
        with patch.object(ref,'MAX_REPORT_BYTES',1), self.assertRaisesRegex(ref.frames.FrameError,'report exceeds'):
            ref.project_reference()


if __name__ == '__main__': unittest.main()
