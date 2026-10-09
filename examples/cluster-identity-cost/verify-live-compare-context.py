"""Source-bound acceptance of compare context routing with real Flux (#812).

Creates one uniquely named kind cluster with a private kubeconfig, installs
Flux's source and kustomize controllers, and has Flux deliver podinfo. With the
ambient current-context unusable it runs compare <resource> and compare object-set with
--kube-context, records how the flux CLI was invoked, and checks that the Git
source reported is the workload's own, the same as the legacy path reports
from a second private kubeconfig.
The shared kubeconfig is never written.

Needs kind, kubectl and the flux CLI on PATH, and network access to pull the
Flux images and clone the podinfo repository.
Run from a clean checkout: python3 examples/cluster-identity-cost/verify-live-compare-context.py
"""
import datetime, hashlib, json, os, pathlib, re, shutil, subprocess, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-compare-context-'))
root.chmod(0o700)
cfg = root / 'config'
ambient_cfg = root / 'config-legacy'
name = 'scout-v214-cmp-' + uuid.uuid4().hex[:8]
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
            'GOTOOLCHAIN': 'go1.26.9'}
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
    if expected == 'nonzero':
        assert result.returncode != 0, (label, 'expected a refusal', result.stdout[-200:])
    elif expected is not None:
        assert result.returncode == expected, (label, result.returncode, result.stderr[-400:])
    return result.stdout


def kubectl(*args, label, content=None):
    return call(['kubectl', '--kubeconfig', str(cfg), *args], label, content)


desired = root / 'desired.yaml'
PODINFO = 'https://github.com/stefanprodan/podinfo'
MODES = [
    ('resource', ['deployment/podinfo', '--namespace', 'team-a']),
    ('object-set', ['object-set', '--dry-from', str(desired), '--scope', 'namespace/team-a']),
]
flux_bound_calls, flux_legacy_calls, same_git_source = [], [], None


def git_sources(value):
    """Every gitSource object in a compare result."""
    found = []
    if isinstance(value, dict):
        for key, item in value.items():
            if key == 'gitSource' and isinstance(item, dict):
                found.append({k: item.get(k) for k in ('repoUrl', 'revision')})
            found += git_sources(item)
    elif isinstance(value, list):
        for item in value:
            found += git_sources(item)
    return found


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

    config = json.loads(kubectl('config', 'view', '--raw', '-o', 'json', label='private-config'))
    config['contexts'][0]['name'] = 'selected'
    config['current-context'] = 'selected'
    ambient_cfg.write_text(json.dumps(config))
    ambient_cfg.chmod(0o600)
    config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config))
    cfg.chmod(0o600)
    captured = sha(cfg)

    scout = ['./cub-scout', 'compare']
    # Control: with no flag the ambient selection is unusable, so the read must fail.
    call([*scout, 'deployment/podinfo', '--namespace', 'team-a', '--format', 'json'], 'control-ambient-unusable',
         expected='nonzero', env=scout_env)

    results = {}
    for label, args in MODES:
        # compare's exit code reports its verdict, so it is recorded, not asserted.
        out = call([*scout, *args, '--format', 'json', '--kube-context', 'selected'], 'compare-' + label,
                   expected=None, env=scout_env)
        results[label] = documents(out)
        assert results[label], (label, 'no result printed')
        for selection in ['', 'missing']:
            call([*scout, *args, '--format', 'json', '--kube-context', selection],
                 'compare-' + label + '-refuses-' + (selection or 'blank'), expected='nonzero', env=scout_env)
    # The Git/namespace comparison cannot bind all its reads, so it refuses the flag.
    refusal = subprocess.run([*scout, '--namespace', 'team-a', '--kube-context', 'selected'], cwd=root, env=scout_env,
                             text=True, capture_output=True, timeout=120)
    assert refusal.returncode != 0 and '--kube-context applies to compare <resource>' in refusal.stderr, refusal.stderr
    assert sha(cfg) == captured, 'selected config changed'

    bound_sources = git_sources(results['resource'])
    assert bound_sources and all(s['repoUrl'] == PODINFO for s in bound_sources), ('wrong or missing Git source', bound_sources)
    flux_bound_calls = flux_log.read_text().splitlines() if flux_log.exists() else []
    assert flux_bound_calls, 'the Flux tracer was not invoked for a Flux-owned workload'
    for line in flux_bound_calls:
        assert '--kubeconfig ' in line and str(cfg) not in line, ('unbound flux call', line)
        assert line.startswith('trace deployment podinfo'), ('the tracer asked about something other than the workload', line)

    # The legacy path, from a private kubeconfig whose current-context is the
    # same cluster, must report the same Git source.
    flux_log.unlink()
    legacy = documents(call([*scout, 'deployment/podinfo', '--namespace', 'team-a', '--format', 'json'],
                            'compare-resource-legacy', expected=None, env={**scout_env, 'KUBECONFIG': str(ambient_cfg)}))
    flux_legacy_calls = flux_log.read_text().splitlines() if flux_log.exists() else []
    same_git_source = git_sources(legacy) == bound_sources
    assert same_git_source, ('bound and legacy Git sources differ', git_sources(legacy), bound_sources)
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
        'schema': 'v214-compare-context-live-proof.v1',
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
        'gitSource': locals().get('bound_sources'),
        'boundGitSourceEqualsLegacy': same_git_source,
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with real Flux delivering podinfo from a public Git repository. Shows which '
                  'cluster compare <resource>, compare object-set and the Flux tracer read, and that the Git source is '
                  'the workload\'s own repository. Argo CD is not installed. No ConfigHub: the DRY and WET sides are '
                  'absent, so this is not a three-way comparison. It does not show cluster identity or read cost.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
