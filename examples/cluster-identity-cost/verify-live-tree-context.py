"""Source-bound acceptance of tree context routing on one owned kind cluster."""
import datetime, hashlib, json, os, pathlib, subprocess, tempfile, uuid
source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-tree-context-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-v214-tree-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None
before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
for directory in ['home', 'cub-config']: (root / directory).mkdir()
env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'), 'GOTOOLCHAIN': 'go1.24.0'}
steps, failures = [], []
created, removed = False, False

def call(args, label, content=None, expected=0):
    result = subprocess.run(args, cwd=root, env=env, input=content, text=True, capture_output=True, timeout=180)
    (root / (label + '.stdout')).write_text(result.stdout)
    (root / (label + '.stderr')).write_text(result.stderr)
    if not label.startswith('private-'):
        steps.append({'name': label, 'exit': result.returncode, 'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest(), 'stderrSHA256': hashlib.sha256(result.stderr.encode()).hexdigest()})
    assert result.returncode == expected, (label, result.returncode)
    return result.stdout

try:
    subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=env, check=True, capture_output=True, timeout=180)
    binary_hash = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg), '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    call(['kubectl', '--kubeconfig', str(cfg), 'apply', '-f', '-'], 'bootstrap-owned-resources', json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': 'team-a'}},
        {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': {'name': 'api', 'namespace': 'team-a'}, 'spec': {'replicas': 0, 'selector': {'matchLabels': {'app': 'proof'}}, 'template': {'metadata': {'labels': {'app': 'proof'}}, 'spec': {'containers': [{'name': 'app', 'image': 'registry.k8s.io/pause:3.10'}]}}}}
    ]}))
    config = json.loads(call(['kubectl', '--kubeconfig', str(cfg), 'config', 'view', '--raw', '-o', 'json'], 'private-config'))
    config['contexts'][0]['name'] = 'selected'
    config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config)); cfg.chmod(0o600)
    captured = sha(cfg)
    views = ['runtime', 'ownership', 'composition', 'workloads', 'git', 'patterns', 'suggest']
    for view in views:
        out = call(['./cub-scout', 'tree', view, '--kube-context', 'selected', '--namespace', 'team-a', '--format', 'json'], 'tree-' + view)
        if view in ['runtime', 'ownership', 'workloads']: assert 'api' in out, view
        if view == 'ownership': assert json.loads(out)['context']['cluster'] == 'selected'
        for selection in ['', 'missing']:
            call(['./cub-scout', 'tree', view, '--kube-context', selection], 'tree-' + view + '-refuses-' + (selection or 'blank'), expected=1)
    call(['./cub-scout', 'tree', 'config', '--kube-context', 'selected'], 'config-refuses-kubernetes-selector', expected=1)
    assert sha(cfg) == captured, 'selected config changed'
except Exception as exc:
    failures.append(repr(exc))
finally:
    if created:
        try:
            call(['kind', 'delete', 'cluster', '--name', name, '--kubeconfig', str(cfg)], 'cleanup-owned-cluster')
            removed = name not in call(['kind', 'get', 'clusters'], 'after-clusters').splitlines()
        except Exception as exc: failures.append('cleanup: ' + repr(exc))
    final = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    clean = not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip()
    proof = {'schema': 'v214-tree-context-live-proof.v1', 'sourceCommit': head, 'sourceCommitUnchanged': head == final, 'sourceWorktreeCleanBeforeAndAfter': clean, 'binarySHA256': locals().get('binary_hash'), 'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'passed': not failures and removed and before == sha(shared) and clean and head == final, 'failures': failures, 'ownedClusterRemoved': removed, 'sharedConfigUnchanged': before == sha(shared), 'steps': steps, 'scope': 'Seven actual standalone tree views with an unusable ambient context; blank/missing selectors and ConfigHub-only view refuse. One owned Kubernetes 1.35 cluster and private HOME/config. No physical identity, whole-command cost, completeness, Target, connected/fleet or six-surface proof. Clean isolated build bound by source checkpoint and binary hash; no compiler VCS-stamp claim.'}
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
