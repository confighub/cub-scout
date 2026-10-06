#!/usr/bin/env python3
"""Prepared PreToolUse guard; no model/tool execution or runtime admission.

The host must source-verify and mount the policy/binding/script immutably.
Allowed calls continue through normal CLI permissions. Hook failure/timeout
semantics still require adversarial acceptance against the pinned Claude CLI.
"""
import hashlib
import json
import math
import os
from pathlib import Path
import re
import stat
import sys

MAX_EVENT = 128 * 1024
ORDINARY = frozenset(('Read', 'Glob', 'Grep', 'Skill'))
MCP = ['mcp__cub-scout__map', 'mcp__cub-scout__explain']


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def encoded(value):
    return (json.dumps(value, indent=2, sort_keys=True) + '\n').encode()


def unique(items):
    out = {}
    for key, value in items:
        if key in out:
            raise ValueError('duplicate JSON property')
        out[key] = value
    return out


def parse(raw, maximum):
    if not isinstance(raw, bytes) or len(raw) > maximum:
        raise ValueError('input exceeds bound')
    def finite(value):
        number = float(value)
        if not math.isfinite(number):
            raise ValueError('invalid JSON number')
        return number
    try:
        return json.loads(raw.decode('utf-8', 'strict'), object_pairs_hook=unique, parse_float=finite,
                          parse_constant=lambda value: (_ for _ in ()).throw(ValueError('invalid JSON number')))
    except RecursionError as exc:
        raise ValueError('JSON nesting exceeds parser bound') from exc


def read_regular(path, maximum):
    path = Path(path)
    if not path.is_absolute() or '..' in path.parts or any(p.is_symlink() for p in (path, *path.parents)):
        raise ValueError('unsafe fixed runtime path')
    if not stat.S_ISREG(path.lstat().st_mode):
        raise ValueError('runtime asset is not regular')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o222:
            raise ValueError('runtime asset is not regular and read-only')
        raw = stream.read(maximum + 1)
    if len(raw) > maximum:
        raise ValueError('runtime asset exceeds bound')
    return raw


def binding_for(launch):
    # Only a source-verified host launch policy may be passed here. The runtime
    # digest is an integrity link, not an independent source attestation.
    return {'schema': 'full24-dispatch-binding.v1', 'selection': launch['selection'],
            'launchPolicySha256': digest(encoded(launch)),
            'ordinaryToolGrant': launch['ordinaryToolGrant'],
            'recordedTools': MCP if launch['selection']['arm'] == 'with' else []}


def decide(binding_raw, policy_raw, event_raw, case, arm, selection_sha):
    binding = parse(binding_raw, 8192)
    launch = parse(policy_raw, 512 * 1024)
    if not isinstance(launch, dict) or not isinstance(binding, dict):
        raise ValueError('invalid runtime document')
    selection = launch.get('selection')
    if (not isinstance(case, str) or not re.fullmatch('(ATR|DEL|HLT|INV|PRE|RUL)-0[1-4]', case)
        or arm not in ('with', 'without') or not isinstance(selection, dict)
        or selection.get('case') != case or selection.get('arm') != arm
        or digest(encoded(selection)) != selection_sha):
        raise ValueError('exact selection differs')
    grant = launch.get('ordinaryToolGrant')
    if (launch.get('schema') != 'full24-launch-policy.v1'
        or launch.get('status') != 'candidate_not_executed'
        or not isinstance(grant, list) or not grant
        or any(not isinstance(name, str) or name not in ORDINARY for name in grant)
        or len(grant) != len(set(grant))):
        raise ValueError('unreviewed launch grant')
    if binding != binding_for(launch) or digest(policy_raw) != binding['launchPolicySha256']:
        raise ValueError('source-bound policy/binding differs')
    event = parse(event_raw, MAX_EVENT)
    if (not isinstance(event, dict) or event.get('hook_event_name') != 'PreToolUse'
        or event.get('cwd') != '/evidence' or not isinstance(event.get('tool_input'), dict)
        or not isinstance(event.get('tool_name'), str)
        or not isinstance(event.get('tool_use_id'), str) or not event['tool_use_id']):
        raise ValueError('incomplete PreToolUse event')
    return event['tool_name'] in grant + binding['recordedTools']


def denial():
    # Never echo requested inputs, file paths, commands or credentials.
    return {'hookSpecificOutput': {'hookEventName': 'PreToolUse',
            'permissionDecision': 'deny',
            'permissionDecisionReason': 'Selected-case dispatch guard refused the tool or runtime binding.'}}


def main():
    try:
        if len(sys.argv) != 4:
            raise ValueError('fixed selection arguments required')
        allowed = decide(read_regular('/runtime/dispatch-binding.json', 8192),
                         read_regular('/runtime/launch-policy.json', 512 * 1024),
                         sys.stdin.buffer.read(MAX_EVENT + 1), *sys.argv[1:])
        result = {} if allowed else denial()
    except (ValueError, OSError, UnicodeError, TypeError, KeyError):
        result = denial()
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    main()
