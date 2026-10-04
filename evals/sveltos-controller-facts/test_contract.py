import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent


class ReportedFactsContract(unittest.TestCase):
    def test_authored_scaffold_and_frozen_exclusion(self):
        fixture = (ROOT / 'fixtures/cluster/observations.yaml').read_bytes()
        self.assertIn(b'Authored contract fixture, not a live capture.', fixture)
        self.assertNotIn('sveltos-controller-facts', (ROOT.parent / 'benchmark-v1.json').read_text())
        with tempfile.TemporaryDirectory() as td:
            subprocess.run(['bash', str(ROOT / 'scaffold.sh')], cwd=td,
                env={'PATH': '/usr/bin:/bin'}, check=True, capture_output=True, timeout=5)
            files = [p.relative_to(td).as_posix() for p in Path(td).rglob('*') if p.is_file()]
            self.assertEqual(files, ['cluster/observations.yaml'])
            self.assertEqual((Path(td) / files[0]).read_bytes(), fixture)

    def test_strict_answer_does_not_infer_health_freshness_or_join(self):
        line = next(line for line in (ROOT / 'graders/answer.md').read_text().splitlines() if line.startswith('pattern: '))
        pattern = line[len('pattern: '):].strip("'")
        expected = {'deliveryStatus': 'Provisioned', 'conditionStatus': 'False',
            'workloadHealth': 'UNKNOWN', 'checkFreshness': 'UNKNOWN', 'joined': False}
        compact = lambda value: json.dumps(value, separators=(',', ':'))
        self.assertIsNotNone(re.fullmatch(pattern, compact(expected)))
        for key, value in [('workloadHealth', 'Healthy'), ('workloadHealth', 'Degraded'),
                           ('checkFreshness', 'fresh'), ('joined', True)]:
            changed = dict(expected)
            changed[key] = value
            self.assertIsNone(re.fullmatch(pattern, compact(changed)))
        self.assertIsNone(re.fullmatch(pattern, 'Result: ' + compact(expected)))
