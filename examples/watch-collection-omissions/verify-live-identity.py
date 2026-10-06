"""Owned-cluster acceptance; private setup/logs never become public fixtures."""
import datetime, hashlib, json, os, pathlib, subprocess, tempfile, uuid
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
    report = json.loads(call(['./cub-scout', 'map', 'list', '--kube-context', 'selected', '--cluster-identity', '-n', 'team-a', '--kind', 'Deployment', '--format', 'json'], 'map-selected'))
    mapped = next(r for r in report['resources'] if r['name'] == 'api')['resourceIdentity']
    assert mapped == resources[0], 'map/watch/bot identity disagreement'
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
    clean = not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip()
    proof = {'schema': 'v214-watch-identity-live-proof.v1', 'sourceCommit': head, 'sourceWorktreeCleanBeforeAndAfter': clean, 'binarySHA256': locals().get('binarysha'), 'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'passed': not failures and cleanup and before == sha(shared) and clean, 'failures': failures, 'ownedClusterRemoved': cleanup, 'sharedConfigUnchanged': before == sha(shared), 'steps': steps, 'scope': 'Actual watch/bot --once and map identity parity; restricted-reader identity denial; object recreation. One owned Kubernetes 1.35 cluster; identity-only cost, not total cost, full scanner coverage, Target binding or informer age. Build from clean source with hash binding; no compiler VCS stamp claim.'}
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
