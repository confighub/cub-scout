"""Source-bound acceptance of receipt verify context routing with real Flux (#812).

Creates one uniquely named kind cluster with a private kubeconfig, installs
Flux's source and kustomize controllers, and has Flux deliver podinfo. With the
ambient current-context unusable it runs every receipt verify mode with
--kube-context, records how the flux CLI was invoked, and compares the bound
receipt with one produced the legacy way from a second private kubeconfig.
The shared kubeconfig is never written.

Needs kind, kubectl and the flux CLI on PATH, and network access to pull the
Flux images and clone the podinfo repository.
Run from a clean checkout: python3 examples/cluster-identity-cost/verify-live-receipt-context.py
"""
import datetime, hashlib, json, os, pathlib, re, shutil, subprocess, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-receipt-context-'))
root.chmod(0o700)
cfg = root / 'config'
ambient_cfg = root / 'config-legacy'
name = 'scout-v214-rcpt-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
VOLATILE = {'verifiedAt', 'fingerprint', 'observedAt', 'generatedAt'}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


def stable(value):
    """Receipt content without its timestamps and the fingerprint over them."""
    if isinstance(value, dict):
        return {k: stable(v) for k, v in value.items() if k not in VOLATILE}
    if isinstance(value, list):
        return [stable(v) for v in value]
    return value


def documents(text):
    """Every JSON document in the output; aggregate mode prints more than one."""
    decoder, index, found = json.JSONDecoder(), 0, []
    while index < len(text):
        if text[index].isspace():
            index += 1
            continue
        value, index = decoder.raw_decode(text, index)
        found.append(value)
    return found


shared_before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
real_flux, real_kubectl = shutil.which('flux'), shutil.which('kubectl')
assert real_flux and real_kubectl, 'flux and kubectl are required on PATH'
for directory in ['home', 'cub-config', 'bin']:
    (root / directory).mkdir()
# cub-scout sees only this directory: kubectl, and a flux shim that records how
# it was called before running the real flux. No cub, so nothing is connected.
flux_log = root / 'flux-calls.log'
(root / 'bin/flux').write_text('#!/bin/sh\necho "$*" >> "%s"\nexec "%s" "$@"\n' % (flux_log, real_flux))
(root / 'bin/flux').chmod(0o755)
os.symlink(real_kubectl, root / 'bin/kubectl')
host_env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'),
            'GOTOOLCHAIN': 'go1.24.0'}
scout_env = {'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'PATH': str(root / 'bin') + ':/usr/bin:/bin'}
steps, failures = [], []
created, removed = False, False


def call(args, label, content=None, expected=0, env=None, timeout=300):
    result = subprocess.run(args, cwd=root, env=env or host_env, input=content, text=True, capture_output=True,
                            timeout=timeout)
    (root / (label + '.stdout')).write_text(result.stdout)
    (root / (label + '.stderr')).write_text(result.stderr)
    if not label.startswith('private-'):
        steps.append({'name': label, 'exit': result.returncode,
                      'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest(),
                      'stderrSHA256': hashlib.sha256(result.stderr.encode()).hexdigest()})
    assert result.returncode == expected, (label, result.returncode, result.stderr[-400:])
    return result.stdout


def kubectl(*args, label, content=None):
    return call(['kubectl', '--kubeconfig', str(cfg), *args], label, content)


desired, prerequisites = root / 'desired.yaml', root / 'prerequisites.yaml'
MODES = [
    ('single', ['deployment/podinfo', '--namespace', 'team-a']),
    ('aggregate', ['--scope', 'namespace/team-a']),
    ('object-set', ['--file', str(desired), '--predicate', 'object-set-matches']),
    ('workloads-converged', ['--file', str(desired), '--predicate', 'workloads-converged']),
    ('prerequisites', ['--prerequisites', str(prerequisites)]),
]
flux_bound_calls, flux_legacy_calls, equivalent = [], [], None

try:
    subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=host_env,
                   check=True, capture_output=True, timeout=300)
    binary_hash = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg),
          '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    call([real_flux, 'install', '--kubeconfig', str(cfg), '--components=source-controller,kustomize-controller',
          '--timeout', '4m'], 'install-flux')
    kubectl('create', 'namespace', 'team-a', label='create-namespace')
    kubectl('apply', '-f', '-', label='flux-delivers-podinfo', content=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'GitRepository',
         'metadata': {'name': 'podinfo', 'namespace': 'flux-system'},
         'spec': {'interval': '10m', 'url': 'https://github.com/stefanprodan/podinfo', 'ref': {'tag': '6.7.1'}}},
        {'apiVersion': 'kustomize.toolkit.fluxcd.io/v1', 'kind': 'Kustomization',
         'metadata': {'name': 'podinfo', 'namespace': 'flux-system'},
         'spec': {'interval': '10m', 'path': './kustomize', 'prune': True, 'targetNamespace': 'team-a',
                  'sourceRef': {'kind': 'GitRepository', 'name': 'podinfo'}}},
    ]}))
    kubectl('-n', 'flux-system', 'wait', 'kustomization/podinfo', '--for=condition=Ready', '--timeout=240s',
            label='wait-for-flux')
    kubectl('-n', 'team-a', 'rollout', 'status', 'deployment/podinfo', '--timeout=180s', label='wait-for-workload')
    desired.write_text(kubectl('-n', 'team-a', 'get', 'deployment', 'podinfo', '-o', 'yaml', label='private-desired'))
    prerequisites.write_text('requiredNamespaces:\n  - team-a\nrequiredCRDs:\n  - kustomizations.kustomize.toolkit.fluxcd.io\n')

    config = json.loads(kubectl('config', 'view', '--raw', '-o', 'json', label='private-config'))
    config['contexts'][0]['name'] = 'selected'
    config['current-context'] = 'selected'
    ambient_cfg.write_text(json.dumps(config))
    ambient_cfg.chmod(0o600)
    config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config))
    cfg.chmod(0o600)
    captured = sha(cfg)

    scout = ['./cub-scout', 'receipt', 'verify']
    # Control: with no flag the ambient selection is unusable, so the read must fail.
    call([*scout, 'deployment/podinfo', '--namespace', 'team-a', '--format', 'json'], 'control-ambient-unusable',
         expected=1, env=scout_env)

    receipts = {}
    for label, args in MODES:
        out = call([*scout, *args, '--format', 'json', '--kube-context', 'selected'], 'receipt-' + label, env=scout_env)
        receipts[label] = documents(out)
        assert receipts[label], (label, 'no receipt printed')
        for selection in ['', 'missing']:
            call([*scout, *args, '--format', 'json', '--kube-context', selection],
                 'receipt-' + label + '-refuses-' + (selection or 'blank'), expected=1, env=scout_env)
    assert sha(cfg) == captured, 'selected config changed'

    # The tracer subprocess: every flux call made for a bound receipt must name
    # a private kubeconfig, never the shared one.
    flux_bound_calls = flux_log.read_text().splitlines() if flux_log.exists() else []
    assert flux_bound_calls, 'the Flux tracer was not invoked for a Flux-owned workload'
    for line in flux_bound_calls:
        assert '--kubeconfig ' in line and str(cfg) not in line, ('unbound flux call', line)

    # Equivalence: the same receipt produced the legacy way, from a private
    # kubeconfig whose current-context is the same cluster, has the same content.
    flux_log.unlink()
    legacy = documents(call([*scout, 'deployment/podinfo', '--namespace', 'team-a', '--format', 'json'],
                            'receipt-single-legacy', env={**scout_env, 'KUBECONFIG': str(ambient_cfg)}))
    flux_legacy_calls = flux_log.read_text().splitlines() if flux_log.exists() else []
    equivalent = stable(legacy) == stable(receipts['single'])
    assert equivalent, 'the bound receipt differs from the legacy receipt for the same cluster'
    assert flux_legacy_calls and all('--kubeconfig ' not in line for line in flux_legacy_calls), 'legacy flux calls changed'
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
    private = str(root)
    proof = {
        'schema': 'v214-receipt-context-live-proof.v1',
        'sourceCommit': head,
        'sourceCommitUnchanged': head == final,
        'sourceWorktreeCleanBeforeAndAfter': clean,
        'binarySHA256': locals().get('binary_hash'),
        'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),
        'kubernetesNodeImage': 'kindest/node:v1.35.0',
        'fluxComponents': ['source-controller', 'kustomize-controller'],
        'ownedClusterRemoved': removed,
        'sharedKubeconfigUnchanged': sha(shared) == shared_before,
        'modes': [m[0] for m in MODES],
        'fluxCallsBound': [re.sub(r'--kubeconfig \S+', '--kubeconfig <private-child-kubeconfig>', line.replace(private, '<private>')) for line in flux_bound_calls],
        'fluxCallsLegacy': [line.replace(private, '<private>') for line in flux_legacy_calls],
        'boundReceiptEqualsLegacyReceipt': equivalent,
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with real Flux delivering podinfo from a public Git repository. Shows which '
                  'cluster each receipt verify mode and the Flux tracer read, and that a bound receipt has the same '
                  'content as a legacy one for the same cluster. Argo CD is not installed, so the bound Argo tracer '
                  'is covered only by the deterministic test. It does not show cluster identity, read cost or '
                  'Target binding, and it does not show a Git source anchor: see the receipt-anchor defect in the README.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
