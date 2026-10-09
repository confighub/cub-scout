"""Source-bound acceptance of context routing for debug, drift, graph explain, patterns and context-pack (#809).

Creates one uniquely named kind cluster with a private kubeconfig, leaves the
ambient current-context unusable, runs each of those commands with
--kube-context, and removes the cluster. The shared kubeconfig is never
written. Run from a clean checkout: python3 examples/cluster-identity-cost/verify-live-command-context.py
"""
import datetime, hashlib, json, os, pathlib, subprocess, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-command-context-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-v214-cmd-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


shared_before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
for directory in ['home', 'cub-config']:
    (root / directory).mkdir()
env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'),
       'GOTOOLCHAIN': 'go1.26.9'}
# Standalone: hide cub so no command enters connected mode from the host's session.
env['PATH'] = os.pathsep.join(d for d in env.get('PATH', '').split(os.pathsep)
                              if not os.access(os.path.join(d, 'cub'), os.X_OK))
steps, failures = [], []
created, removed = False, False


def call(args, label, content=None, expected=0):
    result = subprocess.run(args, cwd=root, env=env, input=content, text=True, capture_output=True, timeout=180)
    (root / (label + '.stdout')).write_text(result.stdout)
    (root / (label + '.stderr')).write_text(result.stderr)
    if not label.startswith('private-'):
        steps.append({'name': label, 'exit': result.returncode,
                      'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest(),
                      'stderrSHA256': hashlib.sha256(result.stderr.encode()).hexdigest()})
    assert result.returncode == expected, (label, result.returncode, result.stderr[-400:])
    return result.stdout


# Each command: label, arguments, expected exit code, and a string its output
# must contain. patterns detect exits 4 when no pattern matches; that is its
# documented result on a cluster with no GitOps controllers.
desired = root / 'desired.yaml'
COMMANDS = [
    ('debug', ['debug', 'deployment/api', '--namespace', 'team-a', '--non-interactive', '--format', 'json'], 0, '"name": "api"'),
    ('drift', ['drift', '--file', str(desired)], 0, 'Drift Report'),
    ('compare-drift', ['compare', 'drift', '--file', str(desired)], 0, 'Drift Report'),
    ('graph-explain', ['graph', 'explain', 'Deployment/api', '--namespace', 'team-a'], 0,
     'Target: selected/team-a/Deployment/api'),
    ('patterns-detect', ['patterns', 'detect'], 4, 'PATTERNS DETECT'),
    ('patterns-explain', ['patterns', 'explain', 'delivery.bridge.confighub_oci'], 0, 'PATTERN EXPLAIN'),
    ('context-pack', ['context-pack'], 0, '"source": "cluster"'),
]

try:
    subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=env,
                   check=True, capture_output=True, timeout=300)
    binary_hash = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg),
          '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    pod = {'metadata': {'labels': {'app': 'proof'}},
           'spec': {'containers': [{'name': 'pause', 'image': 'registry.k8s.io/pause:3.10'}]}}
    call(['kubectl', '--kubeconfig', str(cfg), 'apply', '-f', '-'], 'bootstrap-owned-resources', json.dumps({
        'apiVersion': 'v1', 'kind': 'List', 'items': [
            {'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': 'team-a'}},
            {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': {'name': 'api', 'namespace': 'team-a'},
             'spec': {'replicas': 1, 'selector': {'matchLabels': {'app': 'proof'}}, 'template': pod}},
            {'apiVersion': 'batch/v1', 'kind': 'CronJob', 'metadata': {'name': 'nightly', 'namespace': 'team-a'},
             'spec': {'schedule': '0 3 * * *', 'suspend': True, 'jobTemplate': {'spec': {'template': {
                 'spec': {'restartPolicy': 'Never', 'containers': pod['spec']['containers']}}}}}},
        ]}))
    call(['kubectl', '--kubeconfig', str(cfg), '-n', 'team-a', 'rollout', 'status', 'deployment/api', '--timeout=120s'],
         'wait-for-workload')
    desired.write_text(call(['kubectl', '--kubeconfig', str(cfg), '-n', 'team-a', 'get', 'deployment', 'api', '-o', 'yaml'],
                            'private-desired'))
    config = json.loads(call(['kubectl', '--kubeconfig', str(cfg), 'config', 'view', '--raw', '-o', 'json'],
                             'private-config'))
    config['contexts'][0]['name'] = 'selected'
    config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config))
    cfg.chmod(0o600)
    captured = sha(cfg)

    # Control: with no flag the ambient selection is unusable, so the read must fail.
    call(['./cub-scout', 'debug', 'deployment/api', '--namespace', 'team-a', '--non-interactive'],
         'control-ambient-unusable', expected=1)

    for label, args, exit_code, expect in COMMANDS:
        out = call(['./cub-scout', *args, '--kube-context', 'selected'], label, expected=exit_code)
        assert expect in out, (label, 'output does not contain ' + expect)
        for selection in ['', 'missing']:
            call(['./cub-scout', *args, '--kube-context', selection],
                 label + '-refuses-' + (selection or 'blank'), expected=1)
    assert sha(cfg) == captured, 'selected config changed'
except Exception as exc:
    failures.append(repr(exc))
finally:
    if created:
        try:
            call(['kind', 'delete', 'cluster', '--name', name, '--kubeconfig', str(cfg)], 'cleanup-owned-cluster')
            removed = name not in call(['kind', 'get', 'clusters'], 'after-clusters').splitlines()
        except Exception as exc:
            failures.append('cleanup: ' + repr(exc))
    final = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    clean = not subprocess.check_output(['git', 'status', '--porcelain'], cwd=source, text=True).strip()
    if created and not removed:
        failures.append('owned cluster not removed')
    if sha(shared) != shared_before:
        failures.append('shared kubeconfig changed')
    proof = {
        'schema': 'v214-command-context-live-proof.v1',
        'sourceCommit': head,
        'sourceCommitUnchanged': head == final,
        'sourceWorktreeCleanBeforeAndAfter': clean,
        'binarySHA256': locals().get('binary_hash'),
        'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),
        'kubernetesNodeImage': 'kindest/node:v1.35.0',
        'ownedClusterRemoved': removed,
        'sharedKubeconfigUnchanged': sha(shared) == shared_before,
        'commands': [c[0] for c in COMMANDS],
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with authored workloads and no GitOps controllers. Shows which context each '
                  'command reads and labels, and that missing/blank selections refuse. It does not show cluster identity, '
                  'read cost, complete inventory, Target binding or six-surface agreement.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
