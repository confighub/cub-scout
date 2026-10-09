"""Live check that every surface names the same source for an Argo CD-delivered workload (#819).

Creates one uniquely named kind cluster with a private kubeconfig and installs
Argo CD from the pinned, checksummed manifest the project's CI uses
(scripts/ci/setup-gitops-controllers.sh). Argo CD delivers the guestbook example
through an Application. The check runs in two layouts: that Application as a
root object, and the same Application carrying the tracking id of a parent
Application "fleet" whose source is a different repository (app-of-apps).

For each layout it asks trace, explain, receipt verify and compare (with and
without --kube-context) and the MCP trace and explain tools which source the
workload came from. Each must name the guestbook repository and none may name
the fleet repository. The shared kubeconfig is never written.

Needs kind, kubectl and curl on PATH, and network access to fetch the manifest,
pull the Argo CD images and clone the repositories.
Run from a clean checkout: python3 examples/source-agreement/verify-live-argo.py
"""
import datetime, hashlib, json, os, pathlib, re, shutil, subprocess, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-argo-source-agreement-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-agree-argo-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
FLEET = 'https://github.com/fluxcd/flux2-kustomize-helm-example'
ARGOCD_MANIFEST = 'https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.3/manifests/install.yaml'
ARGOCD_MANIFEST_SHA256 = '7efe2d6bbc03f63623640f1e4198f16c84009d510fb810ef71e56df1b7614ba9'
WORKLOADS = [
    {'name': 'guestbook-ui', 'namespace': 'guestbook', 'via': 'Application',
     'source': 'https://github.com/argoproj/argocd-example-apps'},
]
KNOWN_SOURCES = [w['source'] for w in WORKLOADS] + [FLEET]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


def sources_named(text):
    """Which of the three known repositories the output names."""
    found = set(re.findall(r'https?://[A-Za-z0-9./_-]+', text))
    return sorted(s for s in KNOWN_SOURCES if any(u.rstrip('/') == s for u in found))


shared_before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
real_kubectl = shutil.which('kubectl')
assert real_kubectl and shutil.which('curl'), 'kubectl and curl are required on PATH'
for directory in ['home', 'cub-config', 'bin']:
    (root / directory).mkdir()
# cub-scout sees only kubectl: no argocd CLI and no cub, so Argo evidence comes
# from the Kubernetes API and nothing is connected.
os.symlink(real_kubectl, root / 'bin/kubectl')
host_env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'),
            'GOTOOLCHAIN': 'go1.26.9'}
scout_env = {'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'PATH': str(root / 'bin') + ':/usr/bin:/bin'}
steps, failures, observations, disagreements = [], [], [], []
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
            if 'delivery not ready' in text:
                disagreements.append((layout, resource, surface, 'reports a healthy delivery as not ready'))
            named = sources_named(text)
            observations.append({'layout': layout, 'workload': resource, 'via': workload['via'], 'surface': surface,
                                 'sourcesNamed': named})
            if FLEET in named:
                disagreements.append((layout, resource, surface, 'names the parent Application\'s fleet repository'))
            elif named != [workload['source']]:
                disagreements.append((layout, resource, surface, 'expected only ' + workload['source'], named))
        listed = json.loads(call(['./cub-scout', 'map', 'list', '--namespace', namespace, '--format', 'json'],
                                 '-'.join([layout, workload['name'], 'map-list']), env=scout_env))
        rows = listed if isinstance(listed, list) else listed.get('resources') or listed.get('entries') or []
        owners = [r.get('owner') for r in rows if r.get('kind') == 'Deployment' and r.get('name') == workload['name']]
        observations.append({'layout': layout, 'workload': resource, 'via': workload['via'], 'surface': 'map list',
                             'owner': owners})
        assert owners == ['ArgoCD'], (layout, resource, 'map list owner', owners)


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
    manifest = root / 'argocd.yaml'
    call(['curl', '--fail', '--silent', '--show-error', '--location', '--retry', '3', '--proto', '=https',
          '--proto-redir', '=https', ARGOCD_MANIFEST, '-o', str(manifest)], 'fetch-argocd-manifest')
    assert sha(manifest) == ARGOCD_MANIFEST_SHA256, 'Argo CD manifest checksum mismatch'
    kubectl('create', 'namespace', 'argocd', label='create-namespace-argocd')
    kubectl('apply', '--server-side', '--force-conflicts', '-n', 'argocd', '-f', str(manifest), label='install-argocd')
    kubectl('wait', '--for=condition=Established', 'crd/applications.argoproj.io', '--timeout=120s', label='wait-crd')
    kubectl('-n', 'argocd', 'rollout', 'status', 'deployment/argocd-repo-server', '--timeout=300s', label='wait-repo-server')
    kubectl('-n', 'argocd', 'rollout', 'status', 'statefulset/argocd-application-controller', '--timeout=300s',
            label='wait-application-controller')
    in_cluster = 'https://kubernetes.default.svc'
    kubectl('apply', '-f', '-', label='argo-delivers-guestbook', content=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'Application', 'metadata': {'name': 'guestbook', 'namespace': 'argocd'},
         'spec': {'project': 'default',
                  'source': {'repoURL': WORKLOADS[0]['source'], 'path': 'guestbook', 'targetRevision': 'HEAD'},
                  'destination': {'server': in_cluster, 'namespace': 'guestbook'},
                  'syncPolicy': {'automated': {}, 'syncOptions': ['CreateNamespace=true']}}},
        # The parent of the app-of-apps layout. Never synced: it only has to exist and name a source.
        {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'Application', 'metadata': {'name': 'fleet', 'namespace': 'argocd'},
         'spec': {'project': 'default',
                  'source': {'repoURL': FLEET, 'path': 'clusters/staging', 'targetRevision': 'main'},
                  'destination': {'server': in_cluster, 'namespace': 'argocd'}}},
    ]}))
    kubectl('-n', 'argocd', 'wait', '--for=jsonpath={.status.sync.status}=Synced', 'application/guestbook', '--timeout=300s',
            label='wait-application')
    kubectl('-n', 'guestbook', 'rollout', 'status', 'deployment/guestbook-ui', '--timeout=240s', label='wait-guestbook-ui')
    captured = sha(cfg)

    observe('root')
    kubectl('-n', 'argocd', 'annotate', 'application', 'guestbook',
            'argocd.argoproj.io/tracking-id=fleet:argoproj.io/Application:argocd/guestbook', '--overwrite', label='parent-annotation')
    kubectl('-n', 'argocd', 'label', 'application', 'guestbook', 'argocd.argoproj.io/instance=fleet', '--overwrite',
            label='parent-label')
    observe('app-of-apps')
    assert sha(cfg) == captured, 'kubeconfig changed'
    assert not disagreements, ('surfaces disagree', disagreements)
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
        'schema': 'argo-source-agreement-live-proof.v1',
        'sourceCommit': head,
        'sourceCommitUnchanged': head == final,
        'sourceWorktreeCleanBeforeAndAfter': clean,
        'binarySHA256': locals().get('binary_hash'),
        'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),
        'kubernetesNodeImage': 'kindest/node:v1.35.0',
        'argocdManifest': ARGOCD_MANIFEST,
        'argocdManifestSHA256': ARGOCD_MANIFEST_SHA256,
        'fleetRepository': FLEET,
        'ownedClusterRemoved': removed,
        'sharedKubeconfigUnchanged': sha(shared) == shared_before,
        'observations': observations,
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with real Argo CD v3.5.3 and one Application. The app-of-apps layout is made by '
                  'giving the Application the tracking id and instance label of a parent Application that is never '
                  'synced, not by a real parent sync. No argocd CLI is on PATH, so this covers the Kubernetes-API '
                  'Argo tracer. Checks which source each surface names and the owner map list reports. It does not '
                  'check revision, drift, freshness or health agreement, multi-source Applications or ApplicationSets.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
