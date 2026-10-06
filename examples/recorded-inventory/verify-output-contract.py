"""Actual binary/schema/budget controls; no cluster or ConfigHub access."""
import argparse, copy, hashlib, http.server, json, os, pathlib, subprocess, tempfile, threading
from jsonschema import Draft202012Validator

parser = argparse.ArgumentParser()
binary_mode = parser.add_mutually_exclusive_group(required=True)
binary_mode.add_argument('--binary')
binary_mode.add_argument('--build', action='store_true', help='Build from clean source and retain exact source/hash binding')
parser.add_argument('--cub', help='Optional actual cub plugin host; installs only in private HOME/CUB_CONFIG')
args = parser.parse_args()
source = pathlib.Path(__file__).resolve().parents[2]
fixture = source / 'examples/recorded-inventory/pagination.yaml'
schema = json.loads((source / 'docs/reference/schemas/recorded-inventory.v1.schema.json').read_text())
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema)
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-recorded-contract-'))
root.chmod(0o700)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
if args.build:
    assert not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip(), 'clean build source required'
    binary = root / 'cub-scout'
    build = subprocess.run(['go', 'build', '-o', str(binary), './cmd/cub-scout'], cwd=source, env={**os.environ, 'GOTOOLCHAIN': 'go1.24.0'}, text=True, capture_output=True, timeout=180)
    (root / 'build.stderr').write_text(build.stderr)
    assert build.returncode == 0, 'isolated source build failed'
else:
    binary = pathlib.Path(args.binary).resolve()
requests = []

class Trap(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        requests.append(self.path); self.send_response(500); self.end_headers()
    do_POST = do_GET
    def log_message(self, *unused): pass

server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Trap)
thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
kubeconfig = root / 'kubeconfig.json'
kubeconfig.write_text(json.dumps({'apiVersion': 'v1', 'kind': 'Config', 'clusters': [{'name': 'trap', 'cluster': {'server': 'http://127.0.0.1:' + str(server.server_port)}}], 'users': [{'name': 'trap', 'user': {}}], 'contexts': [{'name': 'trap', 'context': {'cluster': 'trap', 'user': 'trap'}}], 'current-context': 'trap'}))
env = {**os.environ, 'KUBECONFIG': str(kubeconfig), 'CUB_CONFIG': str(root / 'cub-config'), 'HOME': str(root / 'home'), 'CUB_SCOUT_OFFLINE': 'true'}
pathlib.Path(env['HOME']).mkdir(); pathlib.Path(env['CUB_CONFIG']).mkdir()
steps = []

def call(argv, name, expected=0, input=None):
    result = subprocess.run(argv, cwd=source, env=env, input=input, text=True, capture_output=True, timeout=30)
    (root / (name + '.stdout')).write_text(result.stdout)
    (root / (name + '.stderr')).write_text(result.stderr)
    steps.append({'name': name, 'exit': result.returncode, 'stdoutBytes': len(result.stdout.encode()), 'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest()})
    assert result.returncode == expected, (name, result.returncode, result.stderr)
    return result.stdout

base = ['map', 'list', '--recording', str(fixture), '--api-version', 'apps/v1', '--kind', 'Deployment', '--namespace-prefix', 'team-', '--format', 'json']
failures = []
try:
    full = json.loads(call([str(binary), *base], 'cli-full')); validator.validate(full)
    summary = json.loads(call([str(binary), *base, '--summary'], 'cli-summary')); validator.validate(summary)
    recorded = source / 'evals/fixtures/scale/cluster/deployments.yaml'
    scale = json.loads(call([str(binary), 'map', 'list', '--recording', str(recorded), '--api-version', 'apps/v1', '--kind', 'Deployment', '--format', 'json'], 'cli-pinned-scale-recording'))
    validator.validate(scale)
    assert scale['provenance']['captureTime'] == 'unknown' and scale['provenance']['captureCompleteness'] == 'unknown'
    assert summary['selectedCount'] == full['selectedCount'] and 'resources' not in summary
    rows, cursor, pages = [], None, []
    while True:
        extra = ['--page-size', '2'] + (['--cursor', cursor] if cursor else [])
        page = json.loads(call([str(binary), *base, *extra], 'cli-page-' + str(len(pages))))
        validator.validate(page); pages.append(page); rows.extend(page['resources'])
        assert page['pagination']['returnedCount'] == len(page['resources'])
        cursor = page['pagination'].get('nextCursor')
        if not cursor: break
    assert rows == full['resources'] and len(pages) == 3
    # Reconstruct the compact Go JSON data size from pretty JSON using Go's
    # standard escaping. This fixture contains no floats or U+2028/U+2029.
    canonical = json.dumps(full, separators=(',', ':'), ensure_ascii=False).replace('&', '\\u0026').replace('<', '\\u003c').replace('>', '\\u003e').encode()
    budget = len(canonical)
    equal = json.loads(call([str(binary), *base, '--max-report-json-bytes', str(budget)], 'cli-budget-equal'))
    assert equal == full
    call([str(binary), *base, '--max-report-json-bytes', str(budget - 1)], 'cli-budget-refusal', expected=1)
    call([str(binary), *base, '--page-size', '2', '--cursor', 'bad'], 'cli-cursor-refusal', expected=1)
    broken = root / 'malformed.yaml'; broken.write_text('apiVersion: [')
    call([str(binary), 'map', 'list', '--recording', str(broken), '--api-version', 'apps/v1', '--kind', 'Deployment', '--format', 'json'], 'cli-malformed-refusal', expected=1)
    calls = [{'jsonrpc': '2.0', 'id': 1, 'method': 'tools/list'}, {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/call', 'params': {'name': 'map', 'arguments': {'api_version': 'apps/v1', 'kind': 'Deployment', 'namespace_prefix': 'team-', 'max_report_json_bytes': budget}}}, {'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {'name': 'doctor', 'arguments': {}}}, {'jsonrpc': '2.0', 'id': 4, 'method': 'tools/call', 'params': {'name': 'map', 'arguments': {'api_version': 'apps/v1', 'kind': 'Deployment', 'context': 'trap'}}}, {'jsonrpc': '2.0', 'id': 5, 'method': 'tools/call', 'params': {'name': 'map', 'arguments': {'api_version': 'apps/v1', 'kind': 'Deployment', 'namespace_prefix': 'team-', 'page_size': 2}}}]
    out = call([str(binary), 'mcp', 'serve', '--recording', str(fixture)], 'actual-mcp', input=''.join(json.dumps(c) + '\n' for c in calls))
    replies = {r['id']: r for r in map(json.loads, out.splitlines())}
    assert all(t['annotations']['readOnlyHint'] for t in replies[1]['result']['tools'])
    result = replies[2]['result']; assert not result['isError']
    report = json.loads(result['content'][0]['text']); validator.validate(report); assert report == full
    assert 'structuredContent' not in result, 'legacy full-recording MCP shape changed'
    page_result = replies[5]['result']; assert not page_result['isError']
    page_report = page_result['structuredContent']['data']; validator.validate(page_report)
    assert page_report == pages[0] and json.loads(page_result['content'][0]['text']) == page_report
    assert json.loads(result['content'][0]['text']) == report
    assert replies[3]['result']['isError'] and replies[4]['result']['isError']
    for mutation in ['age', 'rows', 'pagination', 'version', 'type', 'extra', 'summary-rows']:
        bad = copy.deepcopy(summary if mutation == 'summary-rows' else full)
        if mutation == 'age': bad['provenance']['captureTime'] = '2026-10-06T00:00:00Z'
        elif mutation == 'rows': del bad['resources']
        elif mutation == 'pagination': bad['schema'] = 'map-list-recorded-page.v1'
        elif mutation == 'version': bad['schema'] = 'invented'
        elif mutation == 'type': bad['selectedCount'] = '5'
        elif mutation == 'extra': bad['cluster'] = {'identity': 'verified'}
        else: bad['resources'] = []
        assert list(validator.iter_errors(bad)), 'schema admitted ' + mutation
    if args.cub:
        plugin = root / 'plugin'; plugin.mkdir()
        (plugin / 'main').write_bytes(binary.read_bytes()); (plugin / 'main').chmod(0o755)
        call([args.cub, 'plugin', 'install', str(plugin), '--name', 'scout'], 'private-plugin-install')
        plugin_report = json.loads(call([args.cub, 'scout', *base], 'actual-cub-plugin'))
        validator.validate(plugin_report); assert plugin_report == full
    assert not requests, 'recorded commands contacted live trap endpoint'
except Exception as exc:
    failures.append(repr(exc))
finally:
    server.shutdown(); server.server_close(); thread.join(timeout=5)
    final_head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    source_clean = not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip()
    if args.build and (head != final_head or not source_clean): failures.append('source changed during acceptance')
    proof = {'schema': 'recorded-output-contract-proof.v1', 'sourceCommit': head, 'sourceCommitUnchanged': head == final_head, 'sourceWorktreeClean': source_clean, 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'fixtureSHA256': hashlib.sha256(fixture.read_bytes()).hexdigest(), 'passed': not failures, 'failures': failures, 'trapRequests': len(requests), 'actualPluginChecked': bool(args.cub), 'canonicalReportBytes': locals().get('budget'), 'mcpFullResultBytes': len(json.dumps(locals().get('result', {}), separators=(',', ':'), ensure_ascii=False).encode()), 'mcpDuplicatedPageResultBytes': len(json.dumps(locals().get('page_result', {}), separators=(',', ':'), ensure_ascii=False).encode()), 'binaryBuildBinding': 'Isolated Go 1.24 build from the clean captured source; executable SHA256 retained, no implicit compiler VCS stamp claim.' if args.build else 'Externally supplied binary hash; source HEAD does not prove build provenance.', 'steps': steps, 'limits': 'Authored fixture, not a genuine cluster recording. Data JSON budget excludes duplicated MCP result, transport framing and tokens. No paid/model or six-surface acceptance claim.'}
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
