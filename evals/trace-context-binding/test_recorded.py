import hashlib
import http.client
import json
from pathlib import Path
import re
import sys
import threading
import unittest
from http.server import ThreadingHTTPServer

ROOT = Path(__file__).parent
sys.path.insert(0, str(ROOT))
from capture_mcp import fixture_handler


class RecordedTraceContract(unittest.TestCase):
    def test_fixture_records_and_rejects_every_http_verb(self):
        requests = []
        lock = threading.Lock()
        server = ThreadingHTTPServer(('127.0.0.1', 0), fixture_handler('alpha', requests, lock))
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            for method in ('HEAD', 'OPTIONS', 'PROPFIND'):
                connection = http.client.HTTPConnection(*server.server_address)
                connection.request(method, '/unhandled')
                response = connection.getresponse()
                self.assertEqual(response.status, 405, method)
                response.read()
                connection.close()
            self.assertEqual([request['method'] for request in requests], ['HEAD', 'OPTIONS', 'PROPFIND'])
            self.assertTrue(all(request['status'] == 405 for request in requests))
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)
        self.assertFalse(thread.is_alive())

    def test_recorded_output_and_provenance(self):
        allowed = json.loads((ROOT / 'fixtures/trace-allowed.json').read_text())
        denied = json.loads((ROOT / 'fixtures/trace-denied.json').read_text())
        receipt = json.loads((ROOT / 'fixtures/provenance.json').read_text())
        self.assertEqual(allowed['context'], 'alpha')
        self.assertEqual(allowed['summary']['source']['url'], 'https://alpha.example.invalid/team/repo.git')
        self.assertEqual(allowed['summary']['source']['revision'], 'alpha-branch')
        self.assertEqual(allowed['chain'][0]['role'], 'source')
        self.assertTrue(any('Events unavailable' in w for w in allowed['warnings']))
        self.assertTrue(denied['isError'])
        self.assertIn('--kube-context denied', denied['content'][0]['text'])
        for field in ('passed', 'kubeconfigUnchanged', 'privateConfigRemoved', 'listenersStopped'):
            self.assertIs(receipt[field], True)
        self.assertEqual(receipt['modelExecution'], 'not run')
        self.assertIn('does not prove', receipt['buildProvenanceLimit'])
        self.assertEqual(len(receipt['requests']), 4)
        self.assertTrue(all(r['method'] == 'GET' and r['endpoint'] != 'beta' for r in receipt['requests']))
        self.assertEqual(receipt['captureScriptSHA256'], hashlib.sha256((ROOT / 'capture_mcp.py').read_bytes()).hexdigest())

    def test_grader_rejects_each_wrong_or_extra_claim(self):
        pattern_line = next(line for line in (ROOT / 'graders/verified-answer.md').read_text().splitlines() if line.startswith('pattern: '))
        pattern = re.compile(pattern_line[len("pattern: '"):-1], re.S)
        answer = dict(context='alpha', source_url='https://alpha.example.invalid/team/repo.git', source_revision='alpha-branch', revision_role='DECLARED_TARGET', events_coverage='INCOMPLETE', denied_context='denied', denied_inventory='UNKNOWN', stable_cluster_identity='NOT_PROVEN', dollar_savings='NOT_PROVEN')
        self.assertIsNotNone(pattern.fullmatch(json.dumps(answer)))
        for key in answer:
            mutated = dict(answer)
            mutated[key] = 'incorrect'
            self.assertIsNone(pattern.fullmatch(json.dumps(mutated)), key)
        self.assertIsNone(pattern.fullmatch(json.dumps(dict(answer, extra='claim'))))
        self.assertIsNone(pattern.fullmatch(json.dumps(answer) + '\nadditional answer'))


if __name__ == '__main__':
    unittest.main()
