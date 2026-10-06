"""Owned-cluster acceptance; private setup/logs never become public fixtures."""
import datetime, fcntl, hashlib, json, os, pathlib, pty, select, struct, subprocess, tempfile, time, uuid
source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-watch-identity-'))
root.chmod(0o700)
cfg = root / 'admin.json'
name = 'scout-v214-id-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
steps, resources, failures = [], [], []
created = False

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None

before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
env = {**os.environ, 'KUBECONFIG': str(cfg), 'CUB_SCOUT_OFFLINE': 'true', 'CUB_CONFIG': str(root / 'cub-config'), 'GOTOOLCHAIN': 'go1.24.0'}
(root / 'cub-config').mkdir()

def call(argv, label, input=None, expected=0):
    result = subprocess.run(argv, cwd=root, env=env, input=input, text=True, capture_output=True, timeout=180)
    (root / (label + '.stdout')).write_text(result.stdout)
    (root / (label + '.stderr')).write_text(result.stderr)
    if not label.startswith('private-'):
        steps.append({'name': label, 'exit': result.returncode,
                      'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest(),
                      'stderrSHA256': hashlib.sha256(result.stderr.encode()).hexdigest()})
    assert result.returncode == expected, (label, result.returncode, result.stderr[-500:])
    return result.stdout

try:
    build = subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=env, text=True, capture_output=True, timeout=180)
    assert build.returncode == 0, build.stderr
    binarysha = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg), '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    deployment = {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': {'name': 'api', 'namespace': 'team-a'}, 'spec': {'replicas': 0, 'selector': {'matchLabels': {'app': 'proof'}}, 'template': {'metadata': {'labels': {'app': 'proof'}}, 'spec': {'containers': [{'name': 'app', 'image': 'registry.k8s.io/pause:3.10'}]}}}}
    items = [{'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': 'team-a'}}, deployment,
             {'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': {'name': 'scout-reader', 'namespace': 'team-a'}},
             {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role', 'metadata': {'name': 'scout-reader', 'namespace': 'team-a'}, 'rules': [{'apiGroups': ['apps', ''], 'resources': ['deployments', 'replicasets', 'pods', 'configmaps', 'services', 'events'], 'verbs': ['get', 'list']}]},
             {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding', 'metadata': {'name': 'scout-reader', 'namespace': 'team-a'}, 'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'Role', 'name': 'scout-reader'}, 'subjects': [{'kind': 'ServiceAccount', 'name': 'scout-reader', 'namespace': 'team-a'}]}]
    call(['kubectl', '--kubeconfig', str(cfg), 'apply', '-f', '-'], 'bootstrap-owned-resources', input=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': items}))
    config = json.loads(call(['kubectl', '--kubeconfig', str(cfg), 'config', 'view', '--raw', '-o', 'json'], 'private-admin-config'))
    token = call(['kubectl', '--kubeconfig', str(cfg), '-n', 'team-a', 'create', 'token', 'scout-reader', '--duration=10m'], 'private-reader-token').strip()
    config['contexts'][0]['name'] = 'selected'
    config['contexts'].append({'name': 'restricted', 'context': {'cluster': config['contexts'][0]['context']['cluster'], 'user': 'restricted'}})
    config['users'].append({'name': 'restricted', 'user': {'token': token}})
    config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config)); cfg.chmod(0o600)
    captured_config = sha(cfg)
    for command in ['watch', 'bot']:
        for context in ['selected', 'restricted']:
            output = root / (command + '-' + context + '.jsonl')
            call(['./cub-scout', command, '--kube-context', context, '--cluster-identity', '--namespace', 'team-a', '--once', '--output-file', str(output)], command + '-' + context)
            events = [json.loads(line) for line in output.read_text().splitlines()]
            cycle = next(e for e in events if e['type'] == 'cluster.observed')
            assert cycle['cluster']['cost']['requestsMade'] == 1
            row = next(e for e in events if e['type'] == 'resource.discovered' and e['resource']['kind'] == 'Deployment' and e['resource']['name'] == 'api')
            if context == 'selected':
                assert row['resourceIdentity']['status'] == 'verified'
                assert row['resourceIdentity']['observed']['clusterId'] == cycle['cluster']['id']
                resources.append(row['resourceIdentity'])
            else:
                assert cycle['cluster']['omission'] == 'forbidden'
                assert row['resourceIdentity']['omission'] == 'cluster_identity_unverified'
                assert 'mergeKey' not in row['resourceIdentity']
    assert resources[0] == resources[1]
    report = json.loads(call(['./cub-scout', 'map', 'list', '--kube-context', 'selected', '--cluster-identity', '--namespace', 'team-a', '--kind', 'Deployment', '--format', 'json'], 'map-selected'))
    mapped = next(r for r in report['resources'] if r['name'] == 'api')['resourceIdentity']
    assert mapped == resources[0], 'map/watch/bot identity disagreement'
    for command, arguments in [('graph', ['graph', 'export']), ('snapshot', ['snapshot'])]:
        output = root / (command + '-selected.json')
        call(['./cub-scout', *arguments, '--kube-context', 'selected', '--namespace', 'team-a', '--output', str(output)], command + '-selected')
        exported = json.loads(output.read_text())
        assert exported['cluster'] == 'selected'
        rows = exported['nodes'] if command == 'graph' else exported['entries']
        assert any(row['kind'] == 'Deployment' and row['name'] == 'api' for row in rows)
        for selection in ['', 'missing']:
            bad = root / (command + '-bad-' + (selection or 'blank') + '.json')
            call(['./cub-scout', *arguments, '--kube-context', selection, '--output', str(bad)], command + '-refuses-' + (selection or 'blank'), expected=1)
            assert not bad.exists()
    master, slave = pty.openpty()
    fcntl.ioctl(slave, 0x80087467, struct.pack('HHHH', 80, 400, 0, 0))
    child = subprocess.Popen(['./cub-scout', 'map', '--kube-context', 'selected', '--namespace', 'team-a'], cwd=root, env={**env, 'TERM': 'xterm-256color'}, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    terminal, exported = bytearray(), False
    started, deadline, last_key, phase = time.monotonic(), time.monotonic() + 30, 0, 0
    try:
        while time.monotonic() < deadline and child.poll() is None:
            now = time.monotonic()
            if now - started > 2 and now - last_key > 1:
                os.write(master, b'M' if phase % 2 == 0 else b'E')
                phase += 1; last_key = now
            ready, _, _ = select.select([master], [], [], 0.2)
            if ready:
                try: terminal.extend(os.read(master, 65536))
                except OSError: break
            files = list(root.glob('cub-scout-graph-*.svg'))
            if files:
                content = files[0].read_text()
                assert '<svg' in content and 'api' in content and 'selected' in content
                exported = True; break
    finally:
        if child.poll() is None:
            os.write(master, b'\x03')
        try: child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            child.terminate()
            try: child.wait(timeout=5)
            except subprocess.TimeoutExpired: child.kill(); child.wait(timeout=5)
        os.close(master)
        (root / 'actual-tui-graph.terminal').write_bytes(terminal)
    assert exported, 'actual selected-context TUI graph export failed'
    call(['kubectl', '--kubeconfig', str(cfg), '--context', 'selected', '-n', 'team-a', 'delete', 'deployment', 'api'], 'delete-owned-instance')
    call(['kubectl', '--kubeconfig', str(cfg), '--context', 'selected', 'apply', '-f', '-'], 'recreate-owned-instance', input=json.dumps(deployment))
    output = root / 'recreated.jsonl'
    call(['./cub-scout', 'watch', '--kube-context', 'selected', '--cluster-identity', '-n', 'team-a', '--once', '--output-file', str(output)], 'watch-recreated')
    recreated = next(json.loads(line) for line in output.read_text().splitlines() if json.loads(line)['type'] == 'resource.discovered' and json.loads(line)['resource']['kind'] == 'Deployment')
    assert recreated['resourceIdentity']['mergeKey'] != mapped['mergeKey']
    assert recreated['resourceIdentity']['observed']['clusterId'] == mapped['observed']['clusterId']
    assert sha(cfg) == captured_config
except Exception as exc:
    failures.append(str(exc))
finally:
    cleanup = False
    if created:
        try:
            call(['kind', 'delete', 'cluster', '--name', name, '--kubeconfig', str(cfg)], 'cleanup-owned-cluster')
            cleanup = name not in call(['kind', 'get', 'clusters'], 'after-clusters').splitlines()
        except Exception as exc:
            failures.append('cleanup: ' + str(exc))
    final_head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    clean = not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip()
    proof = {'schema': 'v214-watch-identity-live-proof.v1', 'sourceCommit': head, 'sourceWorktreeCleanBeforeAndAfter': clean, 'sourceCommitUnchanged': head == final_head, 'binarySHA256': locals().get('binarysha'), 'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'passed': not failures and cleanup and before == sha(shared) and clean and head == final_head, 'failures': failures, 'ownedClusterRemoved': cleanup, 'sharedConfigUnchanged': before == sha(shared), 'steps': steps, 'scope': 'Actual watch/bot --once and map identity parity; restricted-reader identity denial; object recreation; graph/snapshot explicit context and invalid-context refusal; actual PTY TUI graph export. One owned Kubernetes 1.35 cluster; identity-only cost, not total cost, full scanner coverage, Target binding or informer age. Build from clean source with hash binding; no compiler VCS stamp claim.'}
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
