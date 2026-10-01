"""Offline guard for acceptance of the scoped-request live comparison."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('kind_comparison', Path(__file__).with_name('compare_kind_scope.py'))
comparison = importlib.util.module_from_spec(spec)
spec.loader.exec_module(comparison)


class ComparisonTests(unittest.TestCase):
    def pair(self, scope):
        resources = [{'name': 'inv04-api', 'owner': 'Native'}, {'name': 'inv04-worker', 'owner': 'Native'}] if scope == comparison.capture.NAMESPACES[0] else []
        omissions = [{'apiVersion': 'argoproj.io/v1alpha1', 'resource': 'applicationsets', 'namespace': scope, 'reason': 'forbidden'}]
        if scope == comparison.capture.NAMESPACES[2]:
            omissions.append({'apiVersion': 'apps/v1', 'resource': 'deployments', 'namespace': scope, 'reason': 'forbidden'})
        new = {'schema': 'map-list-ownership-evidence.v1', 'resources': resources, 'collection': {'status': 'partial', 'omissions': omissions}}
        old = copy.deepcopy(new)
        old['collection']['omissions'].append({'apiVersion': 'apps/v1', 'resource': 'statefulsets', 'namespace': scope, 'reason': 'forbidden'})
        return old, new

    def test_preserves_selected_facts_and_exact_scoped_omissions(self):
        for scope in comparison.capture.NAMESPACES:
            old, new = self.pair(scope)
            comparison.validate_comparison(scope, old, new)
            new['collection']['status'] = 'complete'
            with self.assertRaises(comparison.capture.CaptureError):
                comparison.validate_comparison(scope, old, new)

    def test_rejects_changed_owner_and_hidden_denial(self):
        scope = comparison.capture.NAMESPACES[0]
        old, new = self.pair(scope)
        new['resources'][0]['owner'] = 'Flux'
        with self.assertRaises(comparison.capture.CaptureError):
            comparison.validate_comparison(scope, old, new)
        scope = comparison.capture.NAMESPACES[2]
        old, new = self.pair(scope)
        new['collection']['omissions'].pop()
        with self.assertRaises(comparison.capture.CaptureError):
            comparison.validate_comparison(scope, old, new)


if __name__ == '__main__':
    unittest.main()
