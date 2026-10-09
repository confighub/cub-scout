"""Live check of gitops settings against real Argo CD and Flux objects (#839).

Creates one uniquely named kind cluster with a private kubeconfig, installs
Flux and the pinned, checksummed Argo CD manifest the project's CI uses, and
creates Applications, Kustomizations and HelmReleases whose specs declare
different sync, correction and prune settings. With the ambient current-context
unusable it runs gitops settings with --kube-context in every format, grouping
and filter, and as two restricted identities that may list only some kinds.
The objects the API server returned are saved under recorded/ beside the proof.
The shared kubeconfig is never written.

Needs kind, kubectl, curl and the flux CLI on PATH, and network access to fetch
the manifest and pull the controller images.
Run from a clean checkout: python3 examples/delivery-settings/verify-live.py
"""
import datetime, hashlib, json, os, pathlib, shutil, subprocess, tempfile, uuid

source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-delivery-settings-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-settings-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
ARGOCD_MANIFEST = 'https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.3/manifests/install.yaml'
ARGOCD_MANIFEST_SHA256 = '7efe2d6bbc03f63623640f1e4198f16c84009d510fb810ef71e56df1b7614ba9'
ARGOCD_URL = 'https://argocd.example.test'
GUESTBOOK = 'https://github.com/argoproj/argocd-example-apps'
PODINFO = 'https://github.com/stefanprodan/podinfo'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


shared_before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
real_flux = shutil.which('flux')
assert real_flux and shutil.which('kubectl') and shutil.which('curl'), 'flux, kubectl and curl are required on PATH'
for directory in ['home', 'cub-config', 'recorded']:
    (root / directory).mkdir()
host_env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'),
            'GOTOOLCHAIN': 'go1.24.0'}
# cub-scout sees no kubectl, flux, argocd or cub: every fact comes from the Kubernetes API.
scout_env = {'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'PATH': '/usr/bin:/bin'}
steps, failures, observations = [], [], {}
created, removed, ambient_unusable = False, False, False


def call(args, label, content=None, expected=0, env=None, timeout=420):
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
    # Once the ambient current-context is made unusable, setup names its context too.
    selected = ['--context', 'selected'] if ambient_unusable else []
    return call(['kubectl', '--kubeconfig', str(cfg), *selected, *args], label, content)


def scout(*args, label, context='selected', expected=0):
    return call(['./cub-scout', *args, '--kube-context', context], label, env=scout_env, expected=expected)


def settings(*args, label, context='selected'):
    report = json.loads(scout('gitops', 'settings', '--format', 'json', *args, label=label, context=context))
    deployers = {(d['kind'], d['namespace'], d['name']): d for d in report['deployers']}
    return report, deployers


def setting(deployer, name):
    found = [s for s in deployer['settings'] if s['name'] == name]
    assert len(found) == 1, (deployer['name'], name, found)
    return found[0]


def names(report):
    return sorted(d['kind'] + '/' + d['namespace'] + '/' + d['name'] for d in report['deployers'])


def application(app, namespace, project, sync_policy=None, ignore=None):
    spec = {'project': project, 'source': {'repoURL': GUESTBOOK, 'path': 'guestbook', 'targetRevision': 'HEAD'},
            'destination': {'server': 'https://kubernetes.default.svc', 'namespace': app}}
    if sync_policy is not None:
        spec['syncPolicy'] = sync_policy
    if ignore:
        spec['ignoreDifferences'] = ignore
    return {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'Application', 'metadata': {'name': app, 'namespace': namespace},
            'spec': spec}


def helm_release(release, extra):
    return {'apiVersion': 'helm.toolkit.fluxcd.io/v2', 'kind': 'HelmRelease', 'metadata': {'name': release, 'namespace': 'team-h'},
            'spec': {'interval': '10m', 'chart': {'spec': {'chart': 'podinfo', 'version': '6.7.1', 'sourceRef': {
                'kind': 'HelmRepository', 'name': 'podinfo'}}}, **extra}}


def restricted_context(config, identity, rules):
    """A context whose service account may list only what rules allow."""
    kubectl('-n', 'default', 'create', 'serviceaccount', identity, label='create-sa-' + identity)
    kubectl('apply', '-f', '-', label='rbac-' + identity, content=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'ClusterRole', 'metadata': {'name': identity}, 'rules': rules},
        {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'ClusterRoleBinding', 'metadata': {'name': identity},
         'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'ClusterRole', 'name': identity},
         'subjects': [{'kind': 'ServiceAccount', 'name': identity, 'namespace': 'default'}]}]}))
    token = kubectl('-n', 'default', 'create', 'token', identity, '--duration', '1h', label='private-token-' + identity).strip()
    config['users'].append({'name': identity, 'user': {'token': token}})
    config['contexts'].append({'name': identity, 'context': {'cluster': config['contexts'][0]['context']['cluster'], 'user': identity}})


try:
    subprocess.run(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], cwd=source, env=host_env,
                   check=True, capture_output=True, timeout=300)
    binary_hash = sha(root / 'cub-scout')
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg),
          '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    call([real_flux, 'install', '--kubeconfig', str(cfg), '--components=source-controller,kustomize-controller,helm-controller',
          '--timeout', '5m'], 'install-flux')
    manifest = root / 'argocd.yaml'
    call(['curl', '--fail', '--silent', '--show-error', '--location', '--retry', '3', '--proto', '=https',
          '--proto-redir', '=https', ARGOCD_MANIFEST, '-o', str(manifest)], 'fetch-argocd-manifest')
    assert sha(manifest) == ARGOCD_MANIFEST_SHA256, 'Argo CD manifest checksum mismatch'
    for namespace in ['argocd', 'team-a', 'team-b', 'team-h']:
        kubectl('create', 'namespace', namespace, label='create-namespace-' + namespace)
    kubectl('apply', '--server-side', '--force-conflicts', '-n', 'argocd', '-f', str(manifest), label='install-argocd')
    kubectl('wait', '--for=condition=Established', 'crd/applications.argoproj.io', '--timeout=120s', label='wait-crd')
    kubectl('-n', 'argocd', 'rollout', 'status', 'deployment/argocd-repo-server', '--timeout=300s', label='wait-repo-server')
    kubectl('-n', 'argocd', 'rollout', 'status', 'statefulset/argocd-application-controller', '--timeout=300s',
            label='wait-application-controller')

    kubectl('apply', '-f', '-', label='create-deployers', content=json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [
        {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'AppProject', 'metadata': {'name': 'payments', 'namespace': 'argocd'},
         'spec': {'sourceRepos': ['*'], 'destinations': [{'namespace': '*', 'server': '*'}]}},
        application('guestbook-auto', 'argocd', 'default', {'automated': {'prune': True, 'selfHeal': True},
                                                            'syncOptions': ['CreateNamespace=true', 'ServerSideApply=true']}),
        application('guestbook-manual', 'argocd', 'payments'),
        application('guestbook-partial', 'argocd', 'payments',
                    {'automated': {}, 'syncOptions': ['CreateNamespace=true', 'Validate=false',
                                                      'PrunePropagationPolicy=foreground', 'RespectIgnoreDifferences=true']},
                    [{'group': 'apps', 'kind': 'Deployment', 'jsonPointers': ['/spec/replicas']}]),
        # An Application outside the Argo CD namespace: there is no argocd-cm beside it.
        application('outside', 'team-b', 'default', {'automated': {'selfHeal': False}}),
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'GitRepository', 'metadata': {'name': 'podinfo', 'namespace': 'flux-system'},
         'spec': {'interval': '10m', 'url': PODINFO, 'ref': {'tag': '6.7.1'}}},
        {'apiVersion': 'kustomize.toolkit.fluxcd.io/v1', 'kind': 'Kustomization', 'metadata': {'name': 'podinfo', 'namespace': 'flux-system'},
         'spec': {'interval': '10m', 'path': './kustomize', 'prune': True, 'targetNamespace': 'team-a',
                  'sourceRef': {'kind': 'GitRepository', 'name': 'podinfo'}}},
        {'apiVersion': 'kustomize.toolkit.fluxcd.io/v1', 'kind': 'Kustomization', 'metadata': {'name': 'podinfo-held', 'namespace': 'flux-system'},
         'spec': {'interval': '10m', 'path': './kustomize', 'prune': False, 'suspend': True, 'force': True, 'wait': True,
                  'targetNamespace': 'team-b', 'sourceRef': {'kind': 'GitRepository', 'name': 'podinfo'}}},
        {'apiVersion': 'source.toolkit.fluxcd.io/v1', 'kind': 'HelmRepository', 'metadata': {'name': 'podinfo', 'namespace': 'team-h'},
         'spec': {'interval': '10m', 'url': 'https://stefanprodan.github.io/podinfo'}},
        helm_release('plain', {}),
        helm_release('tuned', {
            'driftDetection': {'mode': 'enabled', 'ignore': [{'paths': ['/spec/replicas'], 'target': {'kind': 'Deployment'}}]},
            'install': {'createNamespace': True, 'remediation': {'retries': 3}},
            'upgrade': {'force': True, 'cleanupOnFail': True, 'remediation': {'retries': 2, 'strategy': 'rollback'}}}),
    ]}))
    # The controllers are real and have acted on these objects before they are read.
    kubectl('-n', 'argocd', 'wait', '--for=jsonpath={.status.sync.status}=Synced', 'application/guestbook-auto', '--timeout=300s',
            label='wait-application')
    kubectl('-n', 'flux-system', 'wait', 'kustomization/podinfo', '--for=condition=Ready', '--timeout=240s', label='wait-kustomization')
    kubectl('-n', 'team-h', 'wait', 'helmrelease/tuned', '--for=jsonpath={.status.observedGeneration}=1', '--timeout=240s',
            label='wait-helmrelease')

    config = json.loads(kubectl('config', 'view', '--raw', '-o', 'json', label='private-config'))
    config['contexts'][0]['name'] = 'selected'
    config['current-context'] = 'unusable-ambient'
    restricted_context(config, 'flux-only', [
        {'apiGroups': ['kustomize.toolkit.fluxcd.io', 'helm.toolkit.fluxcd.io'], 'resources': ['kustomizations', 'helmreleases'],
         'verbs': ['list']}])
    restricted_context(config, 'apps-only', [
        {'apiGroups': ['argoproj.io'], 'resources': ['applications'], 'verbs': ['list']}])
    cfg.write_text(json.dumps(config))
    cfg.chmod(0o600)
    ambient_unusable = True
    captured = sha(cfg)
    for resource in ['applications.argoproj.io', 'kustomizations.kustomize.toolkit.fluxcd.io', 'helmreleases.helm.toolkit.fluxcd.io']:
        listed = json.loads(kubectl('get', resource, '-A', '-o', 'json', label='private-record-' + resource))
        (root / 'recorded' / (resource.split('.')[0] + '.json')).write_text(json.dumps(listed, indent=2) + '\n')

    # Control: with no flag the ambient selection is unusable, so the read must fail.
    call(['./cub-scout', 'gitops', 'settings'], 'control-ambient-unusable', expected=1, env=scout_env)
    for selection in ['', 'missing']:
        scout('gitops', 'settings', label='refuses-' + (selection or 'blank'), context=selection, expected=1)

    # Before the Argo CD URL is configured, no link may be shown.
    report, deployers = settings(label='before-url')
    observations['linkSourcesBeforeURL'] = report['linkSources']
    assert all(link['status'] != 'found' for link in report['linkSources']), report['linkSources']
    assert all('url' not in d for d in deployers.values()), 'a link was shown with no configured Argo CD URL'
    kubectl('-n', 'argocd', 'patch', 'configmap', 'argocd-cm', '--type', 'merge', '-p', json.dumps({'data': {'url': ARGOCD_URL + '/'}}),
            label='configure-argocd-url')

    report, deployers = settings(label='all')
    observations['reads'], observations['counts'] = report['reads'], report['counts']
    observations['linkSources'] = report['linkSources']
    observations['declared'] = {'/'.join(key): {s['name']: s['value'] for s in d['settings']} for key, d in deployers.items()}
    assert report['complete'] and [(r['kind'], r['status'], r['count']) for r in report['reads']] == [
        ('Application', 'read', 4), ('Kustomization', 'read', 2), ('HelmRelease', 'read', 2)], report['reads']

    auto = deployers[('Application', 'argocd', 'guestbook-auto')]
    assert (auto['group'], auto['url']) == ('default', ARGOCD_URL + '/applications/argocd/guestbook-auto'), auto
    assert [setting(auto, n)['value'] for n in ['auto-sync', 'self-heal', 'prune']] == ['on', 'on', 'on'], auto
    assert setting(auto, 'ServerSideApply')['value'] == 'true' and setting(auto, 'CreateNamespace')['value'] == 'true'

    manual = deployers[('Application', 'argocd', 'guestbook-manual')]
    sync = setting(manual, 'auto-sync')
    assert (manual['group'], sync['value'], sync['default'], sync['effective']) == ('payments', 'unset', 'off', 'off'), manual
    assert [setting(manual, n)['value'] for n in ['self-heal', 'prune']] == ['n/a', 'n/a'], manual

    partial = deployers[('Application', 'argocd', 'guestbook-partial')]
    heal = setting(partial, 'self-heal')
    assert setting(partial, 'auto-sync')['value'] == 'on' and (heal['value'], heal['default']) == ('unset', 'off'), partial
    assert setting(partial, 'Validate')['value'] == 'false' and setting(partial, 'PrunePropagationPolicy')['value'] == 'foreground'
    ignore = setting(partial, 'ignoreDifferences')
    assert ignore['detail'] == '1 rule, also respected on sync' and len(partial['ignoreRules']) == 1, partial

    outside = deployers[('Application', 'team-b', 'outside')]
    assert 'url' not in outside and setting(outside, 'self-heal')['value'] == 'off', outside
    assert {l['namespace']: l['status'] for l in report['linkSources']} == {'argocd': 'found', 'team-b': 'not_found'}, report['linkSources']

    held = deployers[('Kustomization', 'flux-system', 'podinfo-held')]
    assert [setting(held, n)['value'] for n in ['suspend', 'prune', 'force', 'wait']] == ['on', 'off', 'on', 'on'], held
    podinfo = deployers[('Kustomization', 'flux-system', 'podinfo')]
    assert setting(podinfo, 'prune')['value'] == 'on' and setting(podinfo, 'suspend')['effective'] == 'off', podinfo

    tuned = deployers[('HelmRelease', 'team-h', 'tuned')]
    assert setting(tuned, 'driftDetection.mode')['value'] == 'enabled' and len(tuned['ignoreRules']) == 1, tuned
    assert [setting(tuned, n)['value'] for n in ['install.createNamespace', 'install.remediation.retries', 'upgrade.force',
                                                 'upgrade.remediation.strategy']] == ['true', '3', 'true', 'rollback'], tuned
    drift = setting(deployers[('HelmRelease', 'team-h', 'plain')], 'driftDetection.mode')
    assert (drift['value'], drift['default'], drift['effective']) == ('unset', 'disabled', 'disabled'), drift

    payments = [g for g in report['groups'] if g['kind'] == 'Application' and g['group'] == 'payments']
    assert len(payments) == 1 and payments[0]['deployers'] == 2, report['groups']
    by_value = {v['value']: v for v in [s for s in payments[0]['settings'] if s['name'] == 'auto-sync'][0]['values']}
    assert [d['name'] for d in by_value['on']['deployers']] == ['guestbook-partial'] and by_value['off']['unset'] == 1, by_value

    # Filters.
    assert names(settings('--setting', 'self-heal=off', label='filter-self-heal-off')[0]) == [
        'Application/argocd/guestbook-partial', 'Application/team-b/outside']
    assert names(settings('--setting', 'prune=off', label='filter-prune-off')[0]) == [
        'Application/argocd/guestbook-partial', 'Application/team-b/outside', 'Kustomization/flux-system/podinfo-held']
    assert names(settings('--setting', 'Validate=false', '--setting', 'ignoreDifferences', label='filter-two')[0]) == [
        'Application/argocd/guestbook-partial']
    by_project = settings('--project', 'payments', label='filter-project')[0]
    assert names(by_project) == ['Application/argocd/guestbook-manual', 'Application/argocd/guestbook-partial']
    assert any('4 Flux object(s) have no project' in note for note in by_project['notes']), by_project.get('notes')
    scoped = settings('--namespace', 'team-h', label='namespace-team-h')[0]
    assert names(scoped) == ['HelmRelease/team-h/plain', 'HelmRelease/team-h/tuned'], names(scoped)

    # Every rendering comes from the same model.
    ascii_out = scout('gitops', 'settings', label='ascii')
    for expected in ['Argo CD Application, project payments (2)', 'Flux Kustomization, namespace flux-system (2)',
                     'Flux HelmRelease, namespace team-h (2)', 'argocd: ' + ARGOCD_URL + '/applications/argocd/<name>',
                     'team-b: no argocd-cm in this namespace; no links', 'argocd/guestbook-manual (unset)']:
        assert expected in ascii_out, ('ascii', expected)
    assert 'NOT READ' not in ascii_out and 'INCOMPLETE' not in ascii_out
    by_setting = scout('gitops', 'settings', '--group-by', 'setting', label='ascii-by-setting')
    assert 'Argo CD Application (4)' in by_setting and 'Validate=false  1  argocd/guestbook-partial' in by_setting, by_setting
    by_deployer = scout('gitops', 'settings', '--group-by', 'deployer', label='ascii-by-deployer')
    assert ARGOCD_URL + '/applications/argocd/guestbook-auto' in by_deployer
    markdown = scout('gitops', 'settings', '--format', 'md', label='markdown')
    assert '[argocd/guestbook-auto](' + ARGOCD_URL + '/applications/argocd/guestbook-auto)' in markdown
    deep_dive = scout('map', 'deep-dive', label='map-deep-dive')
    assert 'DELIVERY SETTINGS' in deep_dive and 'Argo CD Application, project payments (2)' in deep_dive

    # An identity that may not list Applications: they are reported as not read, not as absent.
    flux_only, _ = settings(label='flux-only', context='flux-only')
    observations['fluxOnlyReads'] = flux_only['reads']
    assert not flux_only['complete'] and [(r['kind'], r['status'], r.get('reason')) for r in flux_only['reads']] == [
        ('Application', 'not_read', 'forbidden'), ('Kustomization', 'read', None), ('HelmRelease', 'read', None)], flux_only['reads']
    assert len(flux_only['deployers']) == 4
    flux_only_ascii = scout('gitops', 'settings', label='flux-only-ascii', context='flux-only')
    assert 'INCOMPLETE' in flux_only_ascii and 'NOT READ (forbidden); Applications are not known to be absent' in flux_only_ascii

    # An identity that may list Applications but not read argocd-cm: no link is invented.
    apps_only, apps_only_deployers = settings(label='apps-only', context='apps-only')
    observations['appsOnlyReads'], observations['appsOnlyLinkSources'] = apps_only['reads'], apps_only['linkSources']
    assert [(r['kind'], r['status']) for r in apps_only['reads']] == [
        ('Application', 'read'), ('Kustomization', 'not_read'), ('HelmRelease', 'not_read')], apps_only['reads']
    assert all(l['status'] == 'not_read' and l['reason'] == 'forbidden' for l in apps_only['linkSources']), apps_only['linkSources']
    assert len(apps_only_deployers) == 4 and all('url' not in d for d in apps_only_deployers.values())
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
        'schema': 'delivery-settings-live-proof.v1',
        'sourceCommit': head,
        'sourceCommitUnchanged': head == final,
        'sourceWorktreeCleanBeforeAndAfter': clean,
        'binarySHA256': locals().get('binary_hash'),
        'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),
        'kubernetesNodeImage': 'kindest/node:v1.35.0',
        'argocdManifest': ARGOCD_MANIFEST,
        'argocdManifestSHA256': ARGOCD_MANIFEST_SHA256,
        'fluxCLI': subprocess.run([real_flux, '--version'], capture_output=True, text=True).stdout.strip(),
        'fluxComponents': ['source-controller', 'kustomize-controller', 'helm-controller'],
        'ownedClusterRemoved': removed,
        'sharedKubeconfigUnchanged': sha(shared) == shared_before,
        'observations': observations,
        'passed': not failures,
        'failures': failures,
        'steps': steps,
        'limits': 'One owned kind cluster with real Argo CD v3.5.3 and Flux. Shows that gitops settings reports what each '
                  'Application, Kustomization and HelmRelease spec declares, that an absent field is unset and not off, '
                  'that a kind the caller may not list is not read and not absent, and that a link is shown only when '
                  'argocd-cm in the Application\'s namespace declares a url. Only guestbook-auto and the podinfo '
                  'Kustomization are waited on to sync; the other objects exist to be read. The Application in team-b is '
                  'not managed by this Argo CD, which is not configured for Applications in any namespace. It does not '
                  'show that a controller acts on any setting, does not cover ApplicationSet-generated Applications, '
                  'AppProject sync windows, per-resource sync-option annotations, the --tui viewport, or older Flux '
                  'API versions, and makes no claim about agent cost.',
    }
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
