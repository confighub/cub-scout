#!/usr/bin/env python3
"""Bounded real MCP/child-process proof against owned local HTTP fixtures.

No real cluster, ConfigHub session, model execution, or downloads are used.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def fixture_handler(marker, requests, lock):
    class Handler(BaseHTTPRequestHandler):
        def _handle_fixture_request(self):
            denied = marker == 'denied'
            with lock:
                request = {'endpoint': marker, 'method': self.command, 'path': self.path, 'denied': denied}
                requests.append(request)
            status = 200
            if self.command != 'GET':
                status = 405
                body = {'kind': 'Status', 'apiVersion': 'v1', 'status': 'Failure', 'reason': 'MethodNotAllowed', 'code': status}
            elif denied or self.path.startswith('/api/v1/namespaces/delivery/events'):
                status = 403
                body = {'kind': 'Status', 'apiVersion': 'v1', 'status': 'Failure', 'reason': 'Forbidden', 'code': status, 'message': 'fixture read denied'}
            elif self.path == '/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api':
                body = {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'Application', 'metadata': {'name': 'api', 'namespace': 'delivery', 'uid': marker + '-application'}, 'spec': {'source': {'repoURL': 'https://' + marker + '.example.invalid/team/repo.git', 'targetRevision': marker + '-branch', 'path': '.'}}, 'status': {'sync': {'status': 'Synced', 'revision': marker + '-revision'}, 'health': {'status': 'Healthy'}}}
            else:
                status = 404
                body = {'kind': 'Status', 'apiVersion': 'v1', 'status': 'Failure', 'reason': 'NotFound', 'code': status}
            request['status'] = status
            payload = json.dumps(body).encode()
            self.send_response(status)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            if self.command != 'HEAD':
                self.wfile.write(payload)

        def do_GET(self):
            self._handle_fixture_request()

        def __getattr__(self, name):
            if name.startswith('do_'):
                return self._handle_fixture_request
            raise AttributeError(name)

        def log_message(self, *args):
            pass
    return Handler


def capture(binary, output):
    output.mkdir(parents=True, exist_ok=False)
    started = time.monotonic()
    config = None
    requests = []
    lock = threading.Lock()
    servers = []
    threads = []

    def start(marker):
        server = ThreadingHTTPServer(('127.0.0.1', 0), fixture_handler(marker, requests, lock))
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        servers.append(server)
        threads.append(thread)
        return 'http://127.0.0.1:' + str(server.server_port)

    receipt = {'kind': 'local-http-mcp-process-proof', 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'passed': False, 'captureScriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'productCommit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=binary.parent, text=True).strip(), 'buildProvenanceLimit': 'productCommit records checkout HEAD and binarySHA256 pins the executable; this receipt does not prove the binary was built from a clean tree at that commit'}
    try:
        alpha, beta, denied = start('alpha'), start('beta'), start('denied')
        with tempfile.TemporaryDirectory(prefix='scout-trace-mcp-') as private:
            private = Path(private)
            config = private / 'config.json'
            data = {'apiVersion': 'v1', 'kind': 'Config', 'current-context': 'beta', 'clusters': [{'name': 'alpha', 'cluster': {'server': alpha}}, {'name': 'beta', 'cluster': {'server': beta}}, {'name': 'denied', 'cluster': {'server': denied}}], 'users': [{'name': name, 'user': {'token': 'fixture-' + name}} for name in ('alpha', 'beta', 'denied')], 'contexts': [{'name': name, 'context': {'cluster': name, 'user': name}} for name in ('alpha', 'beta', 'denied')]}
            before = json.dumps(data, sort_keys=True).encode()
            config.write_bytes(before)
            config.chmod(0o600)
            calls = []
            for i, context in enumerate(('alpha', '', 'missing', 'denied'), 1):
                calls.append({'jsonrpc': '2.0', 'id': i, 'method': 'tools/call', 'params': {'name': 'trace', 'arguments': {'resource': 'application/api', 'namespace': 'delivery', 'context': context}}})
            env = {'PATH': str(private), 'KUBECONFIG': str(config), 'NO_COLOR': '1'}
            completed = subprocess.run([str(binary), 'mcp', 'serve'], input=''.join(json.dumps(c) + '\n' for c in calls), text=True, capture_output=True, env=env, timeout=45)
            (output / 'mcp.stdout').write_text(completed.stdout)
            (output / 'mcp.stderr').write_text(completed.stderr)
            assert completed.returncode == 0, completed.stderr
            responses = [json.loads(line) for line in completed.stdout.splitlines() if line.strip()]
            assert [r['id'] for r in responses] == [1, 2, 3, 4]
            first = responses[0]['result']
            assert not first.get('isError', False), first
            observed = json.loads(first['content'][0]['text'])
            assert observed['context'] == 'alpha', observed
            assert observed['summary']['source']['url'] == 'https://alpha.example.invalid/team/repo.git'
            assert observed['summary']['source']['revision'] == 'alpha-branch'
            assert observed['chain'][0]['role'] == 'source'
            assert any('Events unavailable' in w for w in observed['warnings'])
            for response, expected in zip(responses[1:], ('non-empty', 'missing', 'denied')):
                result = response['result']
                assert result.get('isError'), result
                assert expected in json.dumps(result), result
            assert not any(r['endpoint'] == 'beta' for r in requests), requests
            assert len(requests) == 4, requests
            assert all(r['method'] == 'GET' for r in requests), requests
            assert sum(r['denied'] for r in requests) == 1, requests
            assert sum(r['endpoint'] == 'alpha' and r['path'] == '/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api' and r['status'] == 200 for r in requests) == 2, requests
            assert sum(r['endpoint'] == 'alpha' and r['path'].startswith('/api/v1/namespaces/delivery/events?') and r['status'] == 403 for r in requests) == 1, requests
            assert config.read_bytes() == before
            assert 'fixture-alpha' not in completed.stdout and 'fixture-denied' not in completed.stdout
            (output / 'trace-allowed.json').write_text(json.dumps(observed, indent=2) + '\n')
            (output / 'trace-denied.json').write_text(json.dumps(responses[3]['result'], indent=2) + '\n')
            receipt.update(passed=True, calls=4, requests=requests, kubeconfigUnchanged=True, scope='real Scout MCP stdio and subprocesses; local HTTP fixtures, not a live cluster', modelExecution='not run')
    finally:
        for server in servers:
            server.shutdown()
            server.server_close()
        for thread in threads:
            thread.join(timeout=2)
        receipt['elapsedSeconds'] = round(time.monotonic() - started, 3)
        receipt['privateConfigRemoved'] = config is not None and not config.exists()
        receipt['requests'] = requests
        receipt['listenersStopped'] = all(not thread.is_alive() for thread in threads)
        receipt['passed'] = receipt['passed'] and receipt['listenersStopped'] and receipt['privateConfigRemoved']
        (output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    return receipt


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = capture(args.binary.resolve(strict=True), args.output)
    print(json.dumps(result, indent=2))
