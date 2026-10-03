#!/usr/bin/env python3
"""Validate a fixed Deployment-export binding before exec of recorded Scout.

No fallback, kubeconfig lookup, model or dynamic command interpretation.
The caller must supply the isolated runtime mounts and complete child accounting.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys

FIELDS = {'schema', 'case', 'recording', 'recordingSha256', 'binary', 'binarySha256'}
CASES = {'ATR-01', 'ATR-02', 'ATR-03', 'ATR-04', 'INV-01', 'INV-02', 'INV-03'}
LINUX_SCOUT_PIN = '5dac8765612592b60ec076f102c1810fbbce94b52236b3577ab3f65219e17fa6'


def unique(items):
    value = {}
    for key, item in items:
        if key in value:
            raise ValueError('duplicate binding property')
        value[key] = item
    return value


def read_regular(path, maximum):
    p = Path(path)
    if not p.is_absolute() or '..' in p.parts or any(x.is_symlink() for x in (p, *p.parents)):
        raise ValueError('unsafe binding path')
    if not stat.S_ISREG(p.lstat().st_mode):
        raise ValueError('binding path is not a regular file')
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > maximum:
            raise ValueError('binding file is not regular or exceeds bound')
        raw = stream.read(maximum + 1)
        if len(raw) > maximum:
            raise ValueError('binding file grew beyond bound')
    return raw


def validate(binding, expected_case, *, recording_path='/evidence/cluster/deployments.yaml', binary_path='/runtime/cub-scout', trusted_binary_sha256=LINUX_SCOUT_PIN):
    # Path overrides support offline unit/stdio tests only. CLI runtime uses the
    # fixed defaults and accepts no argv path override or arbitrary executable.
    if not isinstance(binding, dict) or set(binding) != FIELDS or binding.get('schema') != 'full24-recorded-mcp-exec.v1':
        raise ValueError('invalid binding schema')
    if (not isinstance(expected_case, str) or expected_case not in CASES or binding.get('case') != expected_case
        or binding.get('recording') != recording_path or binding.get('binary') != binary_path):
        raise ValueError('binding selection or fixed paths differ')
    for field in ('recordingSha256', 'binarySha256'):
        if not isinstance(binding[field], str) or not re.fullmatch('[0-9a-f]{64}', binding[field]):
            raise ValueError('invalid binding pin')
    if binding['binarySha256'] != trusted_binary_sha256:
        raise ValueError('executable pin is not the reviewed runtime pin')
    raw = read_regular(recording_path, 4 * 1024 * 1024)
    if hashlib.sha256(raw).hexdigest() != binding['recordingSha256']:
        raise ValueError('recording pin differs')
    binary = read_regular(binary_path, 128 * 1024 * 1024)
    if hashlib.sha256(binary).hexdigest() != binding['binarySha256'] or not os.access(binary_path, os.X_OK):
        raise ValueError('executable pin differs')
    return [binary_path, 'mcp', 'serve', '--recording', recording_path]


def main():
    if len(sys.argv) != 3 or sys.argv[1] != '/runtime/recorded-binding.json':
        raise ValueError('only the fixed runtime binding is accepted')
    binding = json.loads(read_regular(sys.argv[1], 8192), object_pairs_hook=unique)
    argv = validate(binding, sys.argv[2])
    os.execve(argv[0], argv, {'PATH': '/runtime:/usr/bin:/bin', 'HOME': '/tmp/private-mcp',
                            'KUBECONFIG': '/runtime/empty-kubeconfig', 'LANG': 'C.UTF-8'})


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError) as exc:
        print('recorded MCP binding refused: ' + str(exc), file=sys.stderr)
        raise SystemExit(2)
