"""Live check that every surface names the same source for a Flux-delivered workload (#819).

Creates one uniquely named kind cluster with a private kubeconfig and installs
Flux's source, kustomize and helm controllers. Flux delivers podinfo twice: from
a Git repository through a Kustomization, and from a chart repository through a
HelmRelease. The check runs in two layouts: the owners as root objects, and the
owners labelled as managed by a parent Kustomization with a different "fleet"
repository, as `flux bootstrap` lays a cluster out.

For each workload and layout it asks trace, explain, receipt verify and compare
(with and without --kube-context) and the MCP trace and explain tools which
source the workload came from. Each must name the workload's own source and
none may name the fleet repository. The shared kubeconfig is never written.

Needs kind, kubectl and the flux CLI on PATH, and network access to pull the
Flux images, clone the repositories and fetch the chart.
Run from a clean checkout: python3 examples/source-agreement/verify-live-flux.py [--without-flux-cli]

With --without-flux-cli the flux CLI is used only to install Flux; cub-scout
itself runs without it and must name the same sources.
"""
import datetime, hashlib, json, os, pathlib, re, shutil, subprocess, sys, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-flux-source-agreement-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-agree-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
FLEET = 'https://github.com/fluxcd/flux2-kustomize-helm-example'
WORKLOADS = [
    {'name': 'podinfo', 'namespace': 'team-a', 'via': 'Kustomization', 'owner': 'kustomization/podinfo',
     'source': 'https://github.com/stefanprodan/podinfo'},
    {'name': 'podinfo-helm', 'namespace': 'team-b', 'via': 'HelmRelease', 'owner': 'helmrelease/podinfo-helm',
     'source': 'https://stefanprodan.github.io/podinfo'},
]
KNOWN_SOURCES = [w['source'] for w in WORKLOADS] + [FLEET]
PARENT_LABELS = ['kustomize.toolkit.fluxcd.io/name=fleet', 'kustomize.toolkit.fluxcd.io/namespace=flux-system']


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


def sources_named(text):
    """Which of the three known repositories the output names."""
    found = set(re.findall(r'https?://[A-Za-z0-9./_-]+', text))
    return sorted(s for s in KNOWN_SOURCES if any(u.rstrip('/') == s for u in found))


shared_before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
real_flux, real_kubectl = shutil.which('flux'), shutil.which('kubectl')
assert real_flux and real_kubectl, 'flux and kubectl are required on PATH'
for directory in ['home', 'cub-config', 'bin']:
    (root / directory).mkdir()
# cub-scout sees kubectl and, unless --without-flux-cli is given, flux. The
# flux CLI is optional: without it cub-scout reads the same objects through the
# Kubernetes API. No cub either way, so nothing is connected.
WITHOUT_FLUX_CLI = '--without-flux-cli' in sys.argv[1:]
if not WITHOUT_FLUX_CLI:
    os.symlink(real_flux, root / 'bin/flux')
os.symlink(real_kubectl, root / 'bin/kubectl')
host_env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'),
            'GOTOOLCHAIN': 'go1.26.9'}
scout_env = {'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'PATH': str(root / 'bin') + ':/usr/bin:/bin'}
steps, failures, observations = [], [], []
created, removed = False, False


def call(args, label, content=None, expected=0, env=None, timeout=300):
    result = subprocess.run(args, cwd=root, env=env or host_env, input=content, text=True, capture_output=True,
                            timeout=timeout)
    (root / (label + '.stdout')).write_text(result.stdout)
    (root / (label + '.stderr')).write_text(result.stderr)
    steps.append({'name': label, 'exit': result.returncode,
                  'stdoutSHA256': hashlib.sha256(result.stdout.encode()).hexdigest(),
                  'stderrSHA256': hashlib.sha256(result.stderr.encode()).hexdigest()})
    if expected is not None:
        assert result.returncode == expected, (label, result.returncode, result.stderr[-400:])
    return result.stdout


def kubectl(*args, label, content=None):
    return call(['kubectl', '--kubeconfig', str(cfg), *args], label, content)


def mcp(tool, workload, label):
    messages = [
        {'jsonrpc': '2.0', 'id': 0, 'method': 'initialize', 'params': {
            'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'agreement', 'version': '0'}}},
        {'jsonrpc': '2.0', 'method': 'notifications/initialized'},
        {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/call', 'params': {'name': tool, 'arguments': {
            'resource': 'deployment/' + workload['name'], 'namespace': workload['namespace']}}},
    ]
    out = call(['./cub-scout', 'mcp', 'serve'], label, ''.join(json.dumps(m) + '\n' for m in messages), env=scout_env)
    for line in out.splitlines():
        reply = json.loads(line)
        if reply.get('id') == 1:
            result = reply['result']
            assert not result.get('isError'), (label, 'MCP tool reported an error')
            return '\n'.join(c.get('text', '') for c in result.get('content', []))
    raise AssertionError((label, 'no MCP reply'))


def observe(layout):
    for workload in WORKLOADS:
        resource, namespace = 'deployment/' + workload['name'], workload['namespace']
        surfaces = {}
        for command in ['trace', 'explain', 'receipt verify', 'compare']:
            for bound in [False, True]:
                surface = command + (' --kube-context' if bound else '')
                args = ['./cub-scout', *command.split(), resource, '--namespace', namespace, '--format', 'json']
                if bound:
                    args += ['--kube-context', context]
                label = '-'.join([layout, workload['name'], surface.replace(' ', '_').replace('--', '')])
                # compare's exit code reports its verdict, so it is not asserted.
                surfaces[surface] = call(args, label, expected=None if command == 'compare' else 0, env=scout_env)
        for tool in ['trace', 'explain']:
            surfaces['mcp ' + tool] = mcp(tool, workload, '-'.join([layout, workload['name'], 'mcp', tool]))
        for surface, text in surfaces.items():
            # Everything here is healthy, so no surface may report a broken delivery.
            assert 'delivery not ready' not in text, (layout, resource, surface, 'reports a healthy delivery as not ready')
            named = sources_named(text)
            observations.append({'layout': layout, 'workload': resource, 'via': workload['via'], 'surface': surface,
                                 'sourcesNamed': named})
            where = (layout, resource, surface)
            assert FLEET not in named, (*where, 'names the parent Kustomization\'s fleet repository')
            assert named == [workload['source']], (*where, 'expected only ' + workload['source'], named)
        listed = json.loads(call(['./cub-scout', 'map', 'list', '--namespace', namespace, '--format', 'json'],
                                 '-'.join([layout, workload['name'], 'map-list']), env=scout_env))
        rows = listed if isinstance(listed, list) else listed.get('resources') or listed.get('entries') or []
        owners = [r.get('owner') for r in rows if r.get('kind') == 'Deployment' and r.get('name') == workload['name']]
        observations.append({'layout': layout, 'workload': resource, 'via': workload['via'], 'surface': 'map list',
                             'owner': owners})
        assert owners == ['Flux'], (layout, resource, 'map list owner', owners)


try:
    subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=host_env,
                   check=True, capture_output=True, timeout=300)
    binary_hash = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg),
          '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    context = 'kind-' + name
    call([real_flux, 'install', '--kubeconfig', str(cfg),
          '--components=source-controller,kustomize-controller,helm-controller', '--timeout', '5m'], 'install-flux')
    for namespace in ['team-a', 'team-b']:
        kubectl('create', 'namespace', namespace, label='create-namespace-' + namespace)
    flux_system = {'namespace': 'flux-system'}
    kubectl('apply', '-f', '-', label='flux-delivers-podinfo', content=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'GitRepository', 'metadata': {'name': 'podinfo', **flux_system},
         'spec': {'interval': '10m', 'url': WORKLOADS[0]['source'], 'ref': {'tag': '6.7.1'}}},
        {'apiVersion': 'kustomize.toolkit.fluxcd.io/v1', 'kind': 'Kustomization', 'metadata': {'name': 'podinfo', **flux_system},
         'spec': {'interval': '10m', 'path': './kustomize', 'prune': True, 'targetNamespace': 'team-a',
                  'sourceRef': {'kind': 'GitRepository', 'name': 'podinfo'}}},
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'HelmRepository', 'metadata': {'name': 'podinfo', **flux_system},
         'spec': {'interval': '10m', 'url': WORKLOADS[1]['source']}},
        {'apiVersion': 'helm.toolkit.fluxcd.io/v2', 'kind': 'HelmRelease', 'metadata': {'name': 'podinfo-helm', **flux_system},
         'spec': {'interval': '10m', 'targetNamespace': 'team-b', 'releaseName': 'podinfo-helm',
                  'chart': {'spec': {'chart': 'podinfo', 'version': '6.7.1',
                                     'sourceRef': {'kind': 'HelmRepository', 'name': 'podinfo'}}}}},
        # The parent of the bootstrap layout. Suspended: it only has to exist and have a source.
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'GitRepository', 'metadata': {'name': 'fleet', **flux_system},
         'spec': {'interval': '10m', 'url': FLEET, 'ref': {'branch': 'main'}}},
        {'apiVersion': 'kustomize.toolkit.fluxcd.io/v1', 'kind': 'Kustomization', 'metadata': {'name': 'fleet', **flux_system},
         'spec': {'interval': '10m', 'path': './unused', 'prune': False, 'suspend': True,
                  'sourceRef': {'kind': 'GitRepository', 'name': 'fleet'}}},
    ]}))
    kubectl('-n', 'flux-system', 'wait', 'kustomization/podinfo', '--for=condition=Ready', '--timeout=240s', label='wait-kustomization')
    kubectl('-n', 'flux-system', 'wait', 'helmrelease/podinfo-helm', '--for=condition=Ready', '--timeout=300s', label='wait-helmrelease')
    kubectl('-n', 'flux-system', 'wait', 'gitrepository/fleet', '--for=condition=Ready', '--timeout=180s', label='wait-fleet-source')
    for workload in WORKLOADS:
        kubectl('-n', workload['namespace'], 'rollout', 'status', 'deployment/' + workload['name'], '--timeout=180s',
                label='wait-' + workload['name'])
    captured = sha(cfg)

    observe('root')
    for workload in WORKLOADS:
        kubectl('-n', 'flux-system', 'label', workload['owner'], *PARENT_LABELS, '--overwrite', label='parent-' + workload['name'])
    observe('parent')
    assert sha(cfg) == captured, 'kubeconfig changed'
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
        'schema': 'flux-source-agreement-live-proof.v1',
        'sourceCommit': head,
        'sourceCommitUnchanged': head == final,
        'sourceWorktreeCleanBeforeAndAfter': clean,
        'binarySHA256': locals().get('binary_hash'),
        'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),
        'kubernetesNodeImage': 'kindest/node:v1.35.0',
        'fluxComponents': ['source-controller', 'kustomize-controller', 'helm-controller'],
        'fluxCLIOnPath': not WITHOUT_FLUX_CLI,
        'fleetRepository': FLEET,
        'ownedClusterRemoved': removed,
        'sharedKubeconfigUnchanged': sha(shared) == shared_before,
        'observations': observations,
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with real Flux. The parent layout is made by labelling the owners as managed by '
                  'a suspended parent Kustomization, not by a real flux bootstrap. Checks which source each surface '
                  'names and the owner map list reports. It does not check revision, drift, freshness or health '
                  'agreement, Argo CD, the plugin form, watch, bot or the TUI.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
