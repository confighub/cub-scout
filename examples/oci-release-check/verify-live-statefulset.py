"""Owned StatefulSet image acceptance. Controller binding is an authored CR fixture."""
import datetime, hashlib, json, os, pathlib, pty, select, shutil, struct, subprocess, tempfile, time, uuid
import fcntl
source = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-statefulset-'))
root.chmod(0o700)
cfg = root / 'config'
name = 'scout-v214-sts-' + uuid.uuid4().hex[:8]
shared = pathlib.Path.home() / '.kube/config'
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None
before = sha(shared)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
assert not subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip(), 'clean source required'
for directory in ['home', 'cub-config']: (root / directory).mkdir()
env = {**os.environ, 'HOME': str(root / 'home'), 'KUBECONFIG': str(cfg), 'CUB_CONFIG': str(root / 'cub-config'), 'GOTOOLCHAIN': 'go1.26.9'}
host = os.environ.get('CUB_CLI') or shutil.which('cub')
assert host and pathlib.Path(host).is_file(), 'CUB_CLI or cub host required before live setup'
steps, failures = [], []
created, removed = False, False

def call(args, label, content=None, expected=0, cwd=root):
    r = subprocess.run(args, cwd=cwd, env=env, input=content, text=True, capture_output=True, timeout=180)
    (root / (label + '.stdout')).write_text(r.stdout)
    (root / (label + '.stderr')).write_text(r.stderr)
    if not label.startswith('private-'):
        steps.append({'name': label, 'exit': r.returncode, 'stdoutSHA256': hashlib.sha256(r.stdout.encode()).hexdigest(), 'stderrSHA256': hashlib.sha256(r.stderr.encode()).hexdigest()})
    assert r.returncode == expected, (label, r.returncode)
    return r.stdout

try:
    call(['go', 'build', '-o', str(root / 'cub-scout'), './cmd/cub-scout'], 'build', cwd=source)
    binary_hash = sha(root / 'cub-scout')
    platform = 'linux/' + call(['docker', 'image', 'inspect', 'kindest/node:v1.35.0', '--format', '{{.Architecture}}'], 'node-image-platform').strip()
    descriptor = json.loads(call(['oras', 'manifest', 'fetch', '--platform', platform, '--descriptor', 'registry.k8s.io/pause:3.10'], 'independent-registry-descriptor'))
    digest = descriptor['digest']
    manifest = call(['oras', 'manifest', 'fetch', 'registry.k8s.io/pause@' + digest], 'independent-registry-manifest')
    # oras emits the raw manifest bytes without pretty-printing.
    assert 'sha256:' + hashlib.sha256(manifest.encode()).hexdigest() == digest
    image = 'registry.k8s.io/pause@' + digest
    assert name not in call(['kind', 'get', 'clusters'], 'before-clusters').splitlines()
    created = True
    call(['kind', 'create', 'cluster', '--name', name, '--image', 'kindest/node:v1.35.0', '--kubeconfig', str(cfg), '--wait', '90s'], 'create-owned-cluster')
    cfg.chmod(0o600)
    crd = {'apiVersion': 'apiextensions.k8s.io/v1', 'kind': 'CustomResourceDefinition', 'metadata': {'name': 'applications.argoproj.io'}, 'spec': {'group': 'argoproj.io', 'names': {'kind': 'Application', 'plural': 'applications'}, 'scope': 'Namespaced', 'versions': [{'name': 'v1alpha1', 'served': True, 'storage': True, 'schema': {'openAPIV3Schema': {'type': 'object', 'x-kubernetes-preserve-unknown-fields': True}}, 'subresources': {'status': {}}}]}}
    workload = {'apiVersion': 'apps/v1', 'kind': 'StatefulSet', 'metadata': {'name': 'api', 'namespace': 'delivery'}, 'spec': {'replicas': 1, 'serviceName': 'api', 'selector': {'matchLabels': {'app': 'api'}}, 'template': {'metadata': {'labels': {'app': 'api'}}, 'spec': {'containers': [{'name': 'api', 'image': image}]}}}}
    call(['kubectl', '--kubeconfig', str(cfg), 'apply', '-f', '-'], 'bootstrap-owned-resources', json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': [crd, {'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': 'delivery'}}, workload]}))
    call(['kubectl', '--kubeconfig', str(cfg), 'wait', '--for=condition=Established', 'crd/applications.argoproj.io', '--timeout=30s'], 'fixture-crd-established')
    call(['kubectl', '--kubeconfig', str(cfg), '-n', 'delivery', 'rollout', 'status', 'statefulset/api', '--timeout=90s'], 'actual-statefulset-ready')
    desired = root / 'desired.json'; desired.write_text(json.dumps(workload))
    layout = root / 'layout'
    bundle = call(['go', 'run', './examples/oci-release-check/create-layout', str(desired), str(layout)], 'create-local-authored-layout', cwd=source).strip()
    bundle_digest = bundle.split('@')[1]
    src = {'repoURL': 'oci://example.invalid/config', 'targetRevision': bundle_digest, 'path': '.'}
    dst = {'server': 'https://kubernetes.default.svc', 'namespace': 'delivery'}
    application = {'apiVersion': 'argoproj.io/v1alpha1', 'kind': 'Application', 'metadata': {'name': 'fixture', 'namespace': 'delivery'}, 'spec': {'source': src, 'destination': dst}}
    call(['kubectl', '--kubeconfig', str(cfg), 'apply', '-f', '-'], 'create-authored-controller-fixture', json.dumps(application))
    status = {'sync': {'status': 'Synced', 'revision': bundle_digest, 'comparedTo': {'source': src, 'destination': dst}}, 'reconciledAt': datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z'), 'resources': [{'group': 'apps', 'kind': 'StatefulSet', 'namespace': 'delivery', 'name': 'api'}]}
    call(['kubectl', '--kubeconfig', str(cfg), '-n', 'delivery', 'patch', 'application', 'fixture', '--subresource=status', '--type=merge', '-p', json.dumps({'status': status})], 'set-authored-controller-status')
    config = json.loads(call(['kubectl', '--kubeconfig', str(cfg), 'config', 'view', '--raw', '-o', 'json'], 'private-config'))
    config['contexts'][0]['name'] = 'selected'; config['current-context'] = 'unusable-ambient'
    cfg.write_text(json.dumps(config)); cfg.chmod(0o600); captured = sha(cfg)
    base = ['release', 'check', '--bundle', bundle, '--oci-layout', str(layout), '--controller', 'Application/fixture', '--api-version', 'argoproj.io/v1alpha1', '--controller-namespace', 'delivery', '--kube-context', 'selected', '--check-running-image']
    for fmt in ['json', 'ascii', 'md']:
        out = call(['./cub-scout', *base, '--format', fmt], 'actual-cli-' + fmt)
        if fmt == 'json':
            report = json.loads(out); row = report['runningImage']['workloads'][0]
            assert report['verdict'] == 'PASS', (report['verdict'], row.get('reason'))
            assert row['statefulSet']['complete'] and row['statefulSet']['ownedPods'] == 1
            assert row['containers'][0]['runningDigests'] == [digest]
            assert row['pods'][0]['statefulSetUID'] == row['statefulSet']['uid']
            assert row['pods'][0]['controllerRevision'] == row['statefulSet']['currentRevision']
        else: assert 'StatefulSet uid=' in out and 'owned pods=1/1' in out
    arguments = {'bundle': bundle, 'oci_layout': str(layout), 'controller': 'Application/fixture', 'api_version': 'argoproj.io/v1alpha1', 'controller_namespace': 'delivery', 'context': 'selected', 'check_running_image': True}
    child = subprocess.Popen(['./cub-scout', 'mcp', 'serve'], cwd=root, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        request = json.dumps({'jsonrpc': '2.0', 'id': 1, 'method': 'tools/call', 'params': {'name': 'release_check', 'arguments': arguments}}).encode()
        child.stdin.write(('Content-Length: %d\r\n\r\n' % len(request)).encode() + request); child.stdin.flush()
        framed = bytearray(); deadline = time.monotonic() + 120
        while b'\r\n\r\n' not in framed:
            assert time.monotonic() < deadline, 'MCP header timeout'
            if select.select([child.stdout], [], [], min(1, max(0, deadline - time.monotonic())))[0]:
                block = os.read(child.stdout.fileno(), 65536); assert block, 'MCP closed before response'; framed.extend(block)
            assert len(framed) <= 4 << 20, 'MCP frame budget'
        header, body = bytes(framed).split(b'\r\n\r\n', 1)
        length = int(next(line.split(b':')[1] for line in header.split(b'\r\n') if line.lower().startswith(b'content-length:')))
        assert length <= 4 << 20
        while len(body) < length:
            assert time.monotonic() < deadline, 'MCP body timeout'
            if select.select([child.stdout], [], [], min(1, max(0, deadline - time.monotonic())))[0]:
                block = os.read(child.stdout.fileno(), min(65536, length - len(body))); assert block, 'MCP closed before body'; body += block
        response = json.loads(body[:length]); assert not response['result'].get('isError')
        mcp_report = json.loads(response['result']['content'][0]['text'])
        assert mcp_report['runningImage']['workloads'][0]['statefulSet']['complete']
        (root / 'actual-mcp.json').write_text(json.dumps(mcp_report))
        steps.append({'name': 'actual-mcp', 'passed': True, 'reportSHA256': sha(root / 'actual-mcp.json')})
    finally:
        child.stdin.close(); child.terminate()
        try: child.wait(timeout=5)
        except subprocess.TimeoutExpired: child.kill(); child.wait(timeout=5)
    plugin = root / 'cub-config/plugins/scout'; plugin.mkdir(parents=True)
    os.link(root / 'cub-scout', plugin / 'main')
    plugin_report = json.loads(call([host, 'scout', *base, '--format', 'json'], 'actual-cub-plugin'))
    assert plugin_report['runningImage']['workloads'][0]['statefulSet']['complete']
    master, slave = pty.openpty()
    fcntl.ioctl(slave, 0x80087467, struct.pack('HHHH', 80, 400, 0, 0))
    child = subprocess.Popen(['./cub-scout', *base, '--interactive'], cwd=root, env={**env, 'TERM': 'xterm-256color'}, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave); terminal = bytearray(); tui_passed = False; deadline = time.monotonic() + 45
    try:
        while time.monotonic() < deadline and child.poll() is None:
            if select.select([master], [], [], 0.2)[0]:
                try: terminal.extend(os.read(master, 65536))
                except OSError: break
            if b'StatefulSet uid=' in terminal and b'owned pods=1/1' in terminal:
                tui_passed = True; break
    finally:
        if child.poll() is None: os.write(master, b'\x03')
        try: child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            child.terminate()
            try: child.wait(timeout=5)
            except subprocess.TimeoutExpired: child.kill(); child.wait(timeout=5)
        os.close(master); (root / 'actual-tui.terminal').write_bytes(terminal)
    assert tui_passed, 'actual release TUI did not show StatefulSet coverage'
    steps.append({'name': 'actual-release-tui', 'passed': True, 'terminalSHA256': sha(root / 'actual-tui.terminal')})
    # This checks current execution; completed Jobs and index resolution are separate.
    assert sha(cfg) == captured
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
    proof = {'schema': 'v214-statefulset-image-live-proof.v1', 'sourceCommit': head, 'sourceCommitUnchanged': head == final, 'sourceWorktreeCleanBeforeAndAfter': clean, 'binarySHA256': locals().get('binary_hash'), 'finished': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'passed': not failures and removed and before == sha(shared) and clean and head == final, 'failures': failures, 'ownedClusterRemoved': removed, 'sharedConfigUnchanged': before == sha(shared), 'steps': steps, 'image': locals().get('image'), 'platform': locals().get('platform'), 'scope': 'Actual Kubernetes StatefulSet, ControllerRevision and running Pod image coverage in CLI formats, the actual cub plugin, stdio MCP and actual release TUI. Expected platform-manifest digest fetched independently from the registry and raw bytes verified before cluster observation. The Application is an authored CR/status fixture, not a running Argo reconciliation proof. No functional health, OCI index adapter, other workloads, connected/fleet, paid benchmark or full release acceptance. Private HOME/config and owned Kubernetes 1.35 cluster, clean isolated source/binary hash binding.'}
    (root / 'proof.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'proof': str(root / 'proof.json'), 'passed': proof['passed'], 'failures': failures}))
    assert proof['passed']
