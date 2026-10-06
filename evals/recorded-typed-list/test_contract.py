import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
PIN = 'e0102f91e1417c8554ed377352b50450f2376e69d40632f109ea012c5c1560a2'


class TypedListContract(unittest.TestCase):
    def test_original_response_and_scaffold_are_exact(self):
        raw = (HERE / 'fixtures/deployments.json').read_bytes()
        self.assertEqual(hashlib.sha256(raw).hexdigest(), PIN)
        self.assertEqual(raw, (HERE.parent / 'rul03-context/fixtures/rul03-readable-deployments.body').read_bytes())
        obj = json.loads(raw)
        self.assertEqual((obj['apiVersion'], obj['kind']), ('apps/v1', 'DeploymentList'))
        self.assertTrue(all('apiVersion' not in item and 'kind' not in item for item in obj['items']))
        with tempfile.TemporaryDirectory() as td:
            subprocess.run(['bash', str(HERE / 'scaffold.sh')], cwd=td, check=True,
                           env={'PATH':'/usr/bin:/bin'}, timeout=5)
            self.assertEqual((Path(td) / 'cluster/deployments.json').read_bytes(), raw)
            self.assertEqual([p.name for p in (Path(td) / 'cluster').iterdir()], ['deployments.json'])

    def test_strict_answer_rejects_missing_derivation_and_live_claims(self):
        text = (HERE / 'graders/answer.md').read_text()
        pattern = re.search(r"^pattern: '(.*)'$", text, re.MULTILINE).group(1)
        answer = '{"apiVersion":"apps/v1","kind":"Deployment","namespace":"rul03-proof","name":"rul03-probe","typedListDerivedObjects":1,"currentState":"UNKNOWN"}'
        self.assertIsNotNone(re.fullmatch(pattern, answer))
        for bad in [answer.replace(':1,', ':0,'), answer.replace('UNKNOWN','LIVE'),
                    answer.replace('Deployment"','DeploymentList"'), answer + '\nprose',
                    answer.replace('rul03-proof','other')]:
            self.assertIsNone(re.fullmatch(pattern, bad))


if __name__ == '__main__':
    unittest.main()
