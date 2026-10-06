#!/usr/bin/env python3
"""Offline selected-grader adapter over one complete terminal-message trace.

Never launches Claude or the official evaluator. Reported spend is retained,
not reconciled billing. Source, oracle and stage must remain host-side.
"""
import argparse
import json
import math
from pathlib import Path
import subprocess
import yaml

import policy
import recorded_server

MAX_TRACE = 32 * 1024 * 1024
MAX_LINE = 512 * 1024
MAX_ANSWER = 128 * 1024
NODE_SCRIPT = "const fs=require('fs');const x=JSON.parse(fs.readFileSync(0,'utf8'));process.stdout.write(JSON.stringify({matched:new RegExp(x.pattern,x.flags).test(x.answer)}));"


def strict_json(raw):
    def finite_float(value):
        number = float(value)
        if not math.isfinite(number):
            raise ValueError('nonfinite JSON number')
        return number
    return json.loads(raw, object_pairs_hook=recorded_server.unique,
                      parse_float=finite_float,
                      parse_constant=lambda value: (_ for _ in ()).throw(ValueError('nonfinite JSON number')))


def terminal(raw):
    """Reject ambiguous/partial streams; intermediate assistant text never grades."""
    if not isinstance(raw, bytes) or len(raw) > MAX_TRACE:
        raise ValueError('trace exceeds bound')
    lines = raw.decode('utf-8', 'strict').splitlines()
    records = []
    for line in lines:
        if not line.strip():
            continue
        if len(line.encode()) > MAX_LINE:
            raise ValueError('trace record exceeds bound')
        item = strict_json(line)
        if not isinstance(item, dict) or not isinstance(item.get('type'), str):
            raise ValueError('trace record lacks a type')
        records.append(item)
    results = [i for i, item in enumerate(records) if item['type'] == 'result']
    if results != [len(records) - 1] or not records:
        raise ValueError('exactly one final terminal result is required')
    result = records[-1]
    if type(result.get('is_error')) is not bool or not isinstance(result.get('subtype'), str):
        raise ValueError('terminal completion metadata missing or malformed')
    if type(result.get('num_turns')) is not int or result['num_turns'] < 0:
        raise ValueError('terminal turn count missing or malformed')
    if not result['is_error'] and result['subtype'] == 'success':
        if result['num_turns'] < 1:
            raise ValueError('successful terminal has no reported turn')
        if not isinstance(result.get('result'), str) or len(result['result'].encode()) > MAX_ANSWER:
            raise ValueError('terminal answer missing, malformed or exceeds bound')
    return result


def definition(raw):
    text = raw.decode('utf-8', 'strict')
    if text.startswith('---\n'):
        if '\n---' not in text[4:]:
            raise ValueError('grader frontmatter incomplete')
        text = text[4:].split('\n---', 1)[0]
    value = policy.stager.prepare._yaml_load(text)
    if (not isinstance(value, dict) or value.get('type') != 'regex'
        or set(value) - {'type', 'pattern', 'flags', 'target', 'weight'}
        or value.get('target', 'last_message') != 'last_message'
        or type(value.get('weight', 1)) not in (int, float)
        or value.get('weight', 1) != 1 or not isinstance(value.get('pattern'), str)):
        raise ValueError('unsupported selected answer grader')
    flags = value.get('flags', '')
    if not isinstance(flags, str) or len(set(flags)) != len(flags) or any(c not in 'ims' for c in flags):
        raise ValueError('unsupported grader flags')
    return {'pattern': value['pattern'], 'flags': flags}


def node_grade(node, selected, answer):
    """Pinned-source patterns only; bounded local JS compatibility check."""
    executable = Path(node).resolve(strict=True)
    if not executable.is_file():
        raise ValueError('Node executable missing')
    packet = json.dumps({**selected, 'answer': answer}).encode()
    if len(packet) > 256 * 1024:
        raise ValueError('grader input exceeds bound')
    child = subprocess.run([str(executable), '-e', NODE_SCRIPT], input=packet,
                           capture_output=True, timeout=5, env={'PATH': '/usr/bin:/bin'})
    if child.returncode != 0 or child.stderr or len(child.stdout) > 1024:
        raise ValueError('local JS grader failed or exceeded output bound')
    result = strict_json(child.stdout)
    if not isinstance(result, dict) or set(result) != {'matched'} or type(result['matched']) is not bool:
        raise ValueError('malformed local JS grader result')
    return result['matched']


def evaluate_bytes(raw, grader_raw, node, *, max_turns, grade=node_grade, control=False):
    output = {'traceSha256': policy.sha(raw), 'graderSha256': policy.sha(grader_raw),
              'outcome': 'unknown', 'reportedTerminalMetadata': None,
              'claims': {'officialEvaluatorExecuted': False, 'costReconciled': False,
                         'descendantAccountingComplete': False, 'paidRunAdmitted': False}}
    try:
        result = terminal(raw)
        output['reportedTerminalMetadata'] = {k: v for k, v in result.items() if k != 'result'}
        cost = result.get('total_cost_usd')
        output['reportedCostStatus'] = ('observed_unreconciled' if type(cost) in (int, float) and cost >= 0
                                        else 'unavailable' if cost is None else 'invalid')
        if result['is_error'] or result['subtype'] != 'success':
            output.update(outcome='fail', reason='terminal error, interruption or budget exit; no grader executed')
            return output
        if type(max_turns) is not int or max_turns <= 0:
            raise ValueError('invalid source-bound turn budget')
        if result['num_turns'] > max_turns:
            output.update(outcome='fail', reason='terminal turn count exceeds the source-bound launch budget')
            return output
        answer = result['result']
        output['lastMessageSha256'] = policy.sha(answer.encode())
        output['lastMessageBytes'] = len(answer.encode())
        if control:
            expected = strict_json(grader_raw)
            actual = strict_json(answer)
            if not isinstance(expected, dict) or any(type(v) is not str for v in expected.values()):
                raise ValueError('unsupported authored control acceptance shape')
            matched = (isinstance(actual, dict) and list(actual) == list(expected)
                       and all(type(actual[k]) is str and actual[k] == v for k, v in expected.items()))
            output.update(outcome='pass' if matched else 'fail',
                          reason='unweighted authored-control exact ordered JSON acceptance; not the original case grader')
            return output
        selected = definition(grader_raw)
        matched = grade(node, selected, answer)
        if type(matched) is not bool:
            raise ValueError('grader result is not a boolean')
        output['outcome'] = 'pass' if matched else 'fail'
        output['reason'] = 'exact selected last_message regex evaluated by local Node; not official evaluator execution'
    except (ValueError, OSError, UnicodeError, yaml.YAMLError, subprocess.TimeoutExpired) as exc:
        output['reason'] = type(exc).__name__ + ': ' + str(exc)[:250]
    return output


def evaluate(source, stage, case, arm, trace, node, attempt_binding, control=None):
    launch = policy.build(stage, source, case, arm, control)
    report = policy.stager.prepare.validate(source)
    row = next(item for item in report['cases'] if item['id'] == case)
    selected = row['selectedAnswerGraders']
    if len(selected) != 1 or selected[0]['type'] != 'regex' or selected[0]['weight'] != 1:
        raise ValueError('exactly one selected mandatory answer grader required')
    name = selected[0]['name']
    if Path(name).name != name:
        raise ValueError('unsafe grader filename')
    grader = (source / 'authored-input-controls' / case / 'oracle/acceptance.json' if control
              else source / 'oracle' / case / 'selected-graders' / name)
    raw = recorded_server.read_regular(trace.absolute(), MAX_TRACE)
    binding_raw = recorded_server.read_regular(attempt_binding.absolute(), 8192)
    expected_binding = {'schema': 'full24-attempt-binding.v1', 'selection': launch['selection'],
                        'sourcePreparationReportSha256': launch['sourcePreparationReportSha256'],
                        'stageReceiptSha256': policy.sha((stage / 'receipt.json').read_bytes()),
                        'traceSha256': policy.sha(raw)}
    if strict_json(binding_raw) != expected_binding:
        raise ValueError('attempt trace/selection/source/stage binding differs')
    result = evaluate_bytes(raw, grader.read_bytes(), node, max_turns=launch['budget']['max_turns'], control=control is not None)
    result.update(schema='full24-terminal-evaluation.v1', selection=launch['selection'],
                  sourcePreparationReportSha256=launch['sourcePreparationReportSha256'],
                  selectedGrader=({'name': 'acceptance.json', 'type': 'ordered-json-control', 'weight': 0} if control
                                 else {'name': name, 'type': 'regex', 'weight': 1}),
                  attemptBindingSha256=policy.sha(binding_raw),
                  scope='offline source-bound selected answer audit; no runtime or experiment admission')
    policy.verify(launch, stage, source, case, arm, control)
    if recorded_server.read_regular(trace.absolute(), MAX_TRACE) != raw:
        raise ValueError('trace changed during evaluation')
    if recorded_server.read_regular(attempt_binding.absolute(), 8192) != binding_raw:
        raise ValueError('attempt binding changed during evaluation')
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('source-prep', 'stage', 'trace', 'node', 'attempt-binding'):
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--case', required=True)
    parser.add_argument('--arm', choices=policy.stager.ARMS, required=True)
    parser.add_argument('--control')
    args = parser.parse_args()
    result = evaluate(args.source_prep, args.stage, args.case, args.arm, args.trace, args.node, args.attempt_binding, args.control)
    print(json.dumps(result, indent=2, sort_keys=True))
