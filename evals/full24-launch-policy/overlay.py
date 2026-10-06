#!/usr/bin/env python3
"""Prepare a source-bound runtime metadata/plugin overlay, never execute it."""
import json
from pathlib import Path

import policy
import recorded_server
import dispatch_guard


def encoded(value):
    return (json.dumps(value, indent=2, sort_keys=True) + '\n').encode()


def render(launch, model_stage):
    if launch['status'] != 'candidate_not_executed':
        raise ValueError('unreviewed recorded binding cannot produce an overlay')
    files = {'mcp.json': encoded(launch['mcpConfig']), 'launch-policy.json': encoded(launch)}
    guard = (policy.HERE / 'dispatch_guard.py').read_bytes()
    settings = encoded(policy.dispatch_settings(launch['selection']))
    admission = launch['runtimeAssetAdmission']['dispatchGuard']
    if policy.sha(guard) != admission['sourceSha256'] or policy.sha(settings) != admission['settingsSha256']:
        raise ValueError('dispatch guard/settings source differs')
    files.update({'dispatch_guard.py': guard, 'guard-settings.json': settings,
                  'dispatch-binding.json': encoded(dispatch_guard.binding_for(launch))})
    if launch['selection']['arm'] == 'with':
        manifest_raw = (model_stage / '.claude-plugin/plugin.json').read_bytes()
        if policy.sha(manifest_raw) != launch['stageFiles']['.claude-plugin/plugin.json']:
            raise ValueError('staged plugin manifest differs')
        manifest = json.loads(manifest_raw, object_pairs_hook=recorded_server.unique)
        if not isinstance(manifest, dict):
            raise ValueError('malformed plugin manifest')
        # Prevent a second ambient server from the plugin. The strict config
        # contains the sole fixed recorded server; source stages remain inert.
        manifest.pop('mcpServers', None)
        files['plugin/.claude-plugin/plugin.json'] = encoded(manifest)
        for name, pin in launch['stageFiles'].items():
            if name.startswith('skills/'):
                raw = (model_stage / name).read_bytes()
                if policy.sha(raw) != pin:
                    raise ValueError('staged skill differs')
                files['plugin/' + name] = raw
        files['recorded-binding.json'] = encoded(policy.exec_binding(launch))
        wrapper = (policy.HERE / 'recorded_server.py').read_bytes()
        if policy.sha(wrapper) != launch['runtimeAssetAdmission']['wrapper']['sourceSha256']:
            raise ValueError('wrapper source differs')
        files['recorded_server.py'] = wrapper
    return files


def build(source, stage, case, arm, output, control=None):
    launch = policy.build(stage, source, case, arm, control)
    output = policy.stager._no_symlink_components(output)
    for input_root in (source.resolve(), stage.resolve()):
        if output == input_root or input_root in output.parents or output in input_root.parents:
            raise ValueError('overlay may not overlap source preparation or selected stage')
    if output.exists():
        raise ValueError('overlay output must be new')
    files = render(launch, stage / 'model-stage')
    output.mkdir(mode=0o700, parents=True)
    for name, raw in sorted(files.items()):
        policy.stager._write_new(output / name, raw, 0o444)
    receipt = {'schema': 'full24-runtime-overlay.v1', 'selection': launch['selection'],
               'sourcePreparationReportSha256': launch['sourcePreparationReportSha256'],
               'files': {name: policy.sha(raw) for name, raw in sorted(files.items())},
               'runtimeAssetAdmission': launch['runtimeAssetAdmission'],
               'claims': {'executed': False, 'pluginLoaded': False, 'mcpUsable': False,
                          'assetsAdmitted': False, 'paidRunAdmitted': False}}
    policy.stager._write_new(output / 'overlay-receipt.json', encoded(receipt), 0o444)
    for path in sorted(output.rglob('*'), reverse=True):
        if path.is_dir():
            path.chmod(0o555)
    output.chmod(0o555)
    policy.verify(launch, stage, source, case, arm, control)
    return receipt


def verify(source, stage, case, arm, output, control=None):
    launch = policy.build(stage, source, case, arm, control)
    files = render(launch, stage / 'model-stage')
    root = policy.stager._no_symlink_components(output)
    actual = policy.stager._hash_tree(root)
    receipt_raw = (root / 'overlay-receipt.json').read_bytes()
    receipt = json.loads(receipt_raw, object_pairs_hook=recorded_server.unique)
    if not isinstance(receipt, dict):
        raise ValueError('overlay receipt must be an object')
    if any(path.stat().st_mode & 0o222 for path in (root, *root.rglob('*'))):
        raise ValueError('overlay is not frozen read-only')
    expected = {name: policy.sha(raw) for name, raw in files.items()}
    expected['overlay-receipt.json'] = policy.sha(receipt_raw)
    if actual != expected or receipt.get('files') != {name: policy.sha(raw) for name, raw in sorted(files.items())}:
        raise ValueError('overlay file set/hash differs from source-bound reconstruction')
    expected_receipt = {'schema': 'full24-runtime-overlay.v1', 'selection': launch['selection'],
                        'sourcePreparationReportSha256': launch['sourcePreparationReportSha256'],
                        'files': {name: policy.sha(raw) for name, raw in sorted(files.items())},
                        'runtimeAssetAdmission': launch['runtimeAssetAdmission'],
                        'claims': {key: False for key in ('executed', 'pluginLoaded', 'mcpUsable', 'assetsAdmitted', 'paidRunAdmitted')}}
    if receipt != expected_receipt:
        raise ValueError('overlay receipt differs from exact reconstruction')
    return receipt
