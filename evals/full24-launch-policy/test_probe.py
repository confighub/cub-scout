"""Validate probe assertions with authored protocol replies; no process starts."""
import copy
import json
import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import probe_recorded


class ProbeTests(unittest.TestCase):
    def test_identity_provenance_inventory_and_refusal_are_required(self):
        pin = 'a' * 64
        identity = {'api_version': 'apps/v1', 'kind': 'Deployment', 'namespace': 'fixture', 'name': 'exact'}
        summary = {'scope': {'apiVersion': 'apps/v1', 'kind': 'Deployment'},
                   'provenance': {'sha256': pin, 'captureTime': 'unknown', 'captureCompleteness': 'unknown', 'objectCount': 1},
                   'selectedCount': 1, 'excludedFromScopeCount': 0, 'ownerCounts': {'Native': 1}}
        explain = {'recordedInput': {'sha256': pin, 'objectCount': 1,
                                     'identity': {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'namespace': 'fixture', 'name': 'exact'}}}
        def content(value):
            return {'content': [{'type': 'text', 'text': json.dumps(value)}], 'isError': False}
        replies = [{'id': 1, 'result': {}}, {'id': 2, 'result': {'tools': [
            {'name': n, 'annotations': {'readOnlyHint': True}} for n in ('explain', 'map')]}},
            {'id': 3, 'result': content(summary)}, {'id': 4, 'result': content(explain)},
            {'id': 5, 'error': {'code': -32602, 'message': 'unsupported'}}]
        def check(value):
            return probe_recorded.check(b'\n'.join(json.dumps(r).encode() for r in value), pin, identity, 1, 1)
        self.assertTrue(check(replies)['unsupportedToolRefused'])
        for mutation in ('missing', 'duplicate', 'extra_tool', 'failed_tool', 'wrong_identity', 'wrong_sha', 'wrong_count', 'selected_count', 'excluded_count', 'owner_count', 'scope', 'live_success'):
            bad = copy.deepcopy(replies)
            if mutation == 'missing': bad.pop(3)
            elif mutation == 'duplicate': bad[3]['id'] = 3
            elif mutation == 'extra_tool': bad[1]['result']['tools'].append({'name': 'doctor'})
            elif mutation == 'failed_tool': bad[2]['result']['isError'] = True
            elif mutation.startswith('wrong_'):
                obj = copy.deepcopy(explain)
                if mutation == 'wrong_identity': obj['recordedInput']['identity']['namespace'] = 'other'
                elif mutation == 'wrong_sha': obj['recordedInput']['sha256'] = 'b' * 64
                else: obj['recordedInput']['objectCount'] = 0
                bad[3]['result'] = content(obj)
            elif mutation in ('selected_count', 'excluded_count', 'owner_count', 'scope'):
                obj = copy.deepcopy(summary)
                if mutation == 'selected_count': obj['selectedCount'] = 0
                elif mutation == 'excluded_count': obj['excludedFromScopeCount'] = 1
                elif mutation == 'owner_count': obj['ownerCounts'] = {'Native': 0}
                else: obj['scope']['apiVersion'] = 'v1'
                bad[2]['result'] = content(obj)
            else: bad[4] = {'id': 5, 'result': content({})}
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                check(bad)


if __name__ == '__main__':
    unittest.main()
