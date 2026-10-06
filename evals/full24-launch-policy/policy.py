#!/usr/bin/env python3
"""Source-bound CLI launch policy. Never starts a model, process or server."""
from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path

import dispatch_guard

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
spec = importlib.util.spec_from_file_location('launch_policy_stager', REPO / 'evals/full24-case-stage/stage.py')
stager = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(stager)
pin_spec = importlib.util.spec_from_file_location('launch_policy_pins', REPO / 'evals/linux-runtime-versions/prepare.py')
pins = importlib.util.module_from_spec(pin_spec)
assert pin_spec and pin_spec.loader
pin_spec.loader.exec_module(pins)

SCHEMA = 'full24-launch-policy.v1'
MODEL = 'claude-haiku-4-5-20251001'
ORDINARY = {'Read', 'Glob', 'Grep', 'Skill'}
RECORDING = 'cluster/deployments.yaml'
RECORDINGS = {cid: RECORDING for cid in ('ATR-01', 'ATR-02', 'ATR-03', 'ATR-04', 'INV-01', 'INV-02', 'INV-03')}
RECORDINGS.update({'HLT-02': 'cluster/deployment.json', 'PRE-02': 'cluster/after-pod.json',
                   'RUL-01': 'cluster/after-pod.json', 'RUL-04': 'cluster/statefulset.json'})
RECORDED_CASES = frozenset(RECORDINGS)
MCP_TOOLS = ['mcp__cub-scout__map', 'mcp__cub-scout__explain']
FALLBACK_BUDGET = {'max_turns': 20, 'timeout_seconds': 600}


def dispatch_settings(selection):
    selection_sha = sha(dispatch_guard.encoded(selection))
    command = '/runtime/python3 /runtime/dispatch_guard.py ' + selection['case'] + ' ' + selection['arm'] + ' ' + selection_sha
    return {'hooks': {'PreToolUse': [{'matcher': '.*', 'hooks': [
        {'type': 'command', 'command': command, 'timeout': 5}]}]}}


def sha(data):
    return stager.prepare.digest(data)


def _binding(receipt):
    case = receipt['selection']['case']
    if case not in RECORDED_CASES:
        return {'status': 'unavailable', 'reason': 'No reviewed recorded-object binding for this case. No live fallback or fabricated tool response.'}
    files = receipt['stagedFiles']
    recording = RECORDINGS[case]
    if recording not in files:
        raise ValueError('reviewed recording missing from selected case')
    return {'status': 'source_bound_candidate_not_probed', 'case': case,
            'path': '/evidence/' + recording, 'relativePath': recording,
            'sha256': files[recording], 'tools': MCP_TOOLS,
            'scope': 'Only this exact selected static object/export; other files, before/after frames and contexts remain distinct. No controller health, trusted capture-time or original-cluster completeness claim.'}


def _build(receipt, source_report, model_root):
    selection = receipt['selection']
    case = next(item for item in source_report['cases'] if item['id'] == selection['case'])
    grant = json.loads((model_root / 'ordinary-tool-grant.json').read_bytes())
    if (grant != case['ordinaryToolGrant'] or not grant or len(grant) != len(set(grant))
        or any(tool not in ORDINARY for tool in grant)):
        raise ValueError('ordinary tool grant changed or contains an unreviewed tool')
    prompt_bytes = (model_root / 'prompt.md').read_bytes()
    metadata, body = stager.prepare._frontmatter(prompt_bytes)
    if metadata.get('allowed_tools', grant) != grant:
        raise ValueError('prompt tool declaration differs from grant')
    source_budget = case['budgets']
    for key, value in source_budget.items():
        if metadata.get(key, value) != value:
            raise ValueError('prompt source budget changed')
    budget = {key: source_budget.get(key, value) for key, value in FALLBACK_BUDGET.items()}
    if any(type(v) is not int or v <= 0 for v in budget.values()):
        raise ValueError('invalid case budget')
    arm = selection['arm']
    binding = _binding(receipt) if arm == 'with' else {'status': 'absent_in_baseline'}
    ready = binding['status'] != 'unavailable'
    mcp = {'mcpServers': {}}
    if arm == 'with' and ready:
        mcp['mcpServers']['cub-scout'] = {
            'command': '/runtime/python3', 'args': ['/runtime/recorded_server.py', '/runtime/recorded-binding.json', selection['case']]}
    ordinary = ','.join(grant)
    allowed = ordinary + (',' + ','.join(MCP_TOOLS) if arm == 'with' and ready else '')
    argv = ['/runtime/claude', '--print', '--verbose', '--output-format', 'stream-json',
            '--model', MODEL, '--no-session-persistence', '--tools', ordinary,
            '--allowedTools', allowed, '--disallowedTools', 'Task,Agent',
            '--strict-mcp-config', '--mcp-config', '/runtime/mcp.json',
            '--settings', '/runtime/guard-settings.json',
            '--setting-sources', '', '--max-turns', str(budget['max_turns'])]
    if arm == 'with':
        argv += ['--plugin-dir', '/runtime/plugin']
    # This is intentionally not an executable host command. A later runner must
    # construct/inspect the isolated mounts, pinned assets, plugin and child
    # accounting before consuming this policy; none are proved by a JSON plan.
    return {
        'schema': SCHEMA, 'selection': selection,
        'status': 'candidate_not_executed' if ready else 'blocked_recorded_mcp_binding',
        'sourcePreparationReportSha256': receipt['sourcePreparation']['reportSha256'],
        'stageFiles': receipt['stagedFiles'], 'ordinaryToolGrant': grant,
        'budget': {**budget, 'source': source_budget, 'harnessDefaults': {k: v for k, v in FALLBACK_BUDGET.items() if k not in source_budget}},
        'prompt': {'path': '/evidence/prompt.md', 'sha256': sha(prompt_bytes),
                   'bodySha256': sha(body.encode()), 'bodyBytes': len(body.encode()),
                   'delivery': 'exact UTF-8 frontmatter body on stdin; no host oracle or invented question'},
        'recordedMcp': binding, 'mcpConfig': mcp if ready else None,
        'runtimePins': {name: {'bytes': pins.EXPECTED[name][0], 'sha256': pins.EXPECTED[name][1]}
                        for name in (('claude', 'cub-scout') if arm == 'with' and ready else ('claude',))},
        'runtimeAssetAdmission': {
            'status': 'blocked_unreviewed_python_and_overlay' if arm == 'with' else 'blocked_runtime_not_constructed',
            'wrapper': {'sourceSha256': sha((HERE / 'recorded_server.py').read_bytes()),
                        'runtimePath': '/runtime/recorded_server.py'} if arm == 'with' else None,
            'python': {'runtimePath': '/runtime/python3', 'trustedSha256': None},
            'dispatchGuard': {'sourceSha256': sha((HERE / 'dispatch_guard.py').read_bytes()),
                              'runtimePath': '/runtime/dispatch_guard.py',
                              'settingsSha256': sha(dispatch_guard.encoded(dispatch_settings(selection))),
                              'runtimeInvoked': False},
            'rule': 'Before execution, independently pin Python and its runtime dependencies, validate the wrapper against this source digest, and inspect all read-only runtime asset mounts. A policy is never runtime admission.'},
        'argv': argv if ready else None,
        'environment': {'PATH': '/runtime:/usr/bin:/bin', 'HOME': '/tmp/private-home',
                        'CLAUDE_CONFIG_DIR': '/tmp/private-claude', 'KUBECONFIG': '/runtime/empty-kubeconfig',
                        'LANG': 'C.UTF-8'},
        'cwd': '/evidence',
        'plugin': {'status': 'not_present' if arm == 'without' else 'runtime_overlay_required',
                   'rule': 'Copy only source-bound treatment metadata/skills; clear manifest mcpServers; configure the recorded server only through strict mcp.json. Never mutate the source model-stage.'},
        'requirements': ['Pinned reviewed Claude and Scout binaries plus Python, mounted as runtime assets only.',
                         'Only the selected model-stage at /evidence; no source preparation, sibling or host receipt mounts.',
                         'Case budget enforced by supervisor; invocation-owned descendant cleanup and raw trace accounting.',
                         'Provider/auth endpoint scope and attribution admitted separately; no shared settings or credentials.',
                         'Recorded MCP preflight and official evaluator/last-message integration.'],
        'claims': {key: False for key in ('cliExecuted', 'ordinaryToolGrantEnforced', 'mcpUsable',
                                         'skillsLoaded', 'officialGraderExecuted', 'processAccountingComplete',
                                         'paidRunAdmitted', 'qualityOrSavingsMeasured')},
    }


def build(stage_root, source, case_id, arm, control=None):
    receipt = stager.verify(stage_root, source, case_id, arm, control)
    report = stager.prepare.validate(source)
    return _build(receipt, report, stage_root / 'model-stage')


def verify(policy, stage_root, source, case_id, arm, control=None):
    if not isinstance(policy, dict) or policy != build(stage_root, source, case_id, arm, control):
        raise ValueError('launch policy differs from source-bound reconstruction')
    return policy


def authorize_tool(policy, tool):
    """Pre-dispatch allowlist for a future runtime; not proof of CLI enforcement."""
    if not isinstance(policy, dict) or policy.get('schema') != SCHEMA or policy.get('status') != 'candidate_not_executed':
        return False
    grant = policy.get('ordinaryToolGrant')
    if not isinstance(grant, list) or not grant or any(not isinstance(name, str) or name not in ORDINARY for name in grant):
        return False
    # A caller must source-verify the whole policy before dispatch; this function
    # cannot substitute for verification or override the actual CLI flag surface.
    if isinstance(tool, str) and tool in grant:
        return True
    binding = policy.get('recordedMcp')
    selection = policy.get('selection')
    return (isinstance(selection, dict) and selection.get('arm') == 'with' and isinstance(binding, dict)
            and binding.get('status') == 'source_bound_candidate_not_probed'
            and isinstance(tool, str) and tool in MCP_TOOLS)


def exec_binding(policy):
    """Only source-verified policies may reach the later runtime writer."""
    binding = policy.get('recordedMcp', {})
    if policy.get('selection', {}).get('arm') != 'with' or binding.get('status') != 'source_bound_candidate_not_probed':
        raise ValueError('no reviewed recorded binding')
    return {'schema': 'full24-recorded-mcp-exec.v1', 'case': policy['selection']['case'],
            'recording': binding['path'], 'recordingSha256': binding['sha256'],
            'binary': '/runtime/cub-scout', 'binarySha256': policy['runtimePins']['cub-scout']['sha256']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-prep', type=Path, required=True)
    parser.add_argument('--stage', type=Path, required=True)
    parser.add_argument('--case', required=True)
    parser.add_argument('--arm', choices=stager.ARMS, required=True)
    parser.add_argument('--control')
    args = parser.parse_args()
    try:
        result = build(args.stage, args.source_prep, args.case, args.arm, args.control)
    except (ValueError, OSError) as exc:
        parser.exit(2, str(exc) + '\n')
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0 if result['status'] == 'candidate_not_executed' else 3


if __name__ == '__main__':
    raise SystemExit(main())
