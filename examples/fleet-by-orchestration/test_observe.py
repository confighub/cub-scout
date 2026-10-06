# Copyright (C) ConfigHub, Inc.
# SPDX-License-Identifier: MIT
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('observe', Path(__file__).with_name('observe.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def packet(cluster_id, uid='object-a'):
    ref = dict(clusterIdSource='v1/Namespace/kube-system', clusterId=cluster_id,
               apiVersion='apps/v1', group='apps', kind='Deployment', namespace='team-a', name='api', uid=uid, scope='namespaced')
    key = json.dumps([ref[k] for k in ['clusterIdSource', 'clusterId', 'group', 'kind', 'namespace', 'name', 'uid']], separators=(',', ':'))
    return {'schema': 'map-list-cluster-identity.v1', 'cluster': {'identity': 'verified', 'id': cluster_id,
            'idSource': ref['clusterIdSource'], 'observedAt': '2026-10-06T00:00:00Z'},
            'clusterCostScope': 'identity-reader', 'collection': {'status': 'complete', 'omissions': []},
            'resources': [{'kind': 'Deployment', 'name': 'api', 'namespace': 'team-a',
                           'resourceIdentity': {'status': 'verified', 'observed': ref, 'mergeKey': key}}]}


class ObserveTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        binary = self.root / 'cub-scout'
        binary.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
assert os.environ['CUB_SCOUT_OFFLINE'] == 'true'
assert sys.argv[1:] == ['map','list','--cluster-identity','--format','json','--kube-context','same-label','--kind','Deployment','--namespace','team-a']
p = pathlib.Path(os.environ['KUBECONFIG'])
with (p.parent/'calls').open('a') as f: f.write(p.name+'\\n')
if p.read_text() == 'exit7':
    sys.stderr.write('secret sentinel')
    sys.exit(7)
sys.stdout.write(p.read_text())
''')
        binary.chmod(0o700)

    def scope(self, label, data):
        path = self.root / (label + '.json')
        path.write_text(data if isinstance(data, str) else json.dumps(data))
        return {'label': label, 'context': 'same-label', 'kubeconfig': str(path), 'namespace': 'team-a'}

    def test_two_clusters_denial_and_failure_are_retained(self):
        denied = packet('a')
        denied['cluster'] = {'identity': 'unverified', 'omission': 'forbidden'}
        scopes = [self.scope('a', packet('a')), self.scope('b', packet('b')),
                  self.scope('denied', denied), self.scope('unreachable', 'exit7'), self.scope('invalid', 'invalid JSON')]
        result = module.observe(scopes, self.root)
        self.assertEqual(result['status'], 'partial')
        self.assertEqual(len(result['observations']), 5)
        self.assertEqual(len(result['instances']), 2)
        self.assertEqual(len(result['observations'][2]['data']['resources']), 1)
        self.assertIn('cluster_identity_unverified', result['observations'][2]['omissions'])
        self.assertEqual((self.root/'calls').read_text().splitlines(), ['a.json','b.json','denied.json','unreachable.json','invalid.json'])
        self.assertNotIn('secret sentinel', json.dumps(result))
        reverse = module.observe(list(reversed(scopes)), self.root)
        self.assertEqual(result['instances'], reverse['instances'])

    def test_conflicts_and_unknown_scope_never_index(self):
        for alter in [lambda p: p['cluster'].update(id='wrong'),
                      lambda p: p['resources'][0]['resourceIdentity'].update(mergeKey='[]'),
                      lambda p: p['resources'][0]['resourceIdentity']['observed'].update(scope='unknown'),
                      lambda p: p['resources'][0].update(name='wrong'),
                      lambda p: p['resources'][0].update(resourceIdentity=[]),
                      lambda p: p['resources'][0]['resourceIdentity'].update(observed=[])]:
            data = packet('a'); alter(data)
            result = module.observe([self.scope('a', data)], self.root)
            self.assertEqual(result['instances'], [])
            self.assertEqual(result['status'], 'partial')

    def test_duplicate_keys_keep_every_observation(self):
        result = module.observe([self.scope('a', packet('a')), self.scope('duplicate', packet('a'))], self.root)
        self.assertEqual(len(result['instances']), 1)
        self.assertEqual([x['label'] for x in result['instances'][0]['observations']], ['a','duplicate'])
        self.assertEqual(result['status'], 'complete')
        with self.assertRaises(ValueError):
            module.observe([self.scope('same', packet('a'))]*2, self.root)

    def test_missing_process_retains_scope_without_error_details(self):
        (self.root/'cub-scout').unlink()
        result = module.observe([self.scope('a', packet('a'))], self.root)
        self.assertEqual(result['observations'][0]['omissions'], ['command_unavailable'])
        self.assertNotIn(str(self.root), json.dumps(result))


if __name__ == '__main__':
    unittest.main()
