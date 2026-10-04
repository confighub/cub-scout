"""Offline OCI Python asset verifier. Never extracts, executes or admits runtime."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import posixpath
import re
import stat
import struct
import tarfile

RECEIPT = Path(__file__).with_name('runtime-python-candidate.json')
RECEIPT_SHA256 = 'b616cf307c8bcb8103a33ec829e1d7f7bbbee40d7fe770000d2770fbf18cce46'
MAX_ARCHIVE = 64 * 1024 * 1024
MAX_LAYER = 128 * 1024 * 1024
MAX_TOTAL = 256 * 1024 * 1024
MAX_MEMBERS = 20000
FIELDS = ('imageIndexDigest', 'platformManifestDigest', 'configDigest', 'platform',
          'archive', 'layers', 'effectiveInventory', 'pythonAssets',
          'pythonInterpreterELF', 'pythonLibraryFileCount', 'pythonLibraryBytes')


class AssetError(ValueError):
    pass


def sha(data):
    return hashlib.sha256(data).hexdigest()


def require(ok, message):
    if not ok:
        raise AssetError(message)


def regular_bytes(path, limit):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        require(stat.S_ISREG(before.st_mode) and before.st_size <= limit,
                'asset must be a bounded regular file')
    except BaseException:
        os.close(fd)
        raise
    with os.fdopen(fd, 'rb') as stream:
        raw = stream.read(limit + 1)
        after = os.fstat(stream.fileno())
        require(len(raw) == before.st_size and len(raw) <= limit and
                (before.st_size, before.st_mtime_ns, before.st_ctime_ns) ==
                (after.st_size, after.st_mtime_ns, after.st_ctime_ns), 'asset changed while reading')
        return raw


def unique(pairs):
    out = {}
    for key, value in pairs:
        require(key not in out, 'duplicate JSON key')
        out[key] = value
    return out


def parsed(raw):
    require(len(raw) <= 1024 * 1024, 'metadata exceeds bound')
    value = json.loads(raw, object_pairs_hook=unique,
                       parse_constant=lambda _: require(False, 'invalid JSON constant'))
    def depth(item, level=0):
        require(level <= 32, 'metadata exceeds nesting bound')
        if isinstance(item, dict):
            for child in item.values(): depth(child, level + 1)
        elif isinstance(item, list):
            for child in item: depth(child, level + 1)
    depth(value)
    return value


def member_path(name):
    require(not name.startswith('/') and '..' not in name.split('/'), 'unsafe archive path')
    normalized = posixpath.normpath(name)
    return '' if normalized == '.' else normalized


def members(archive):
    out = {}
    for member in archive:
        require(len(out) < MAX_MEMBERS, 'archive member bound exceeded')
        name = member_path(member.name)
        require(name not in out, 'duplicate archive path')
        out[name] = member
    return out


def apply_layer(raw, digest, inventory, elf):
    with tarfile.open(fileobj=io.BytesIO(raw)) as layer:
        rows = members(layer)
        for name in rows:
            base, parent = posixpath.basename(name), posixpath.dirname(name)
            if base == '.wh..wh..opq':
                targets = [p for p in inventory if (not parent or p.startswith(parent + '/')) and p != parent]
            elif base.startswith('.wh.'):
                target = posixpath.join(parent, base[4:])
                targets = [p for p in inventory if p == target or p.startswith(target + '/')]
            else:
                continue
            for target in targets:
                inventory.pop(target, None); elf.pop(target, None)
        for name, member in rows.items():
            if posixpath.basename(name).startswith('.wh.'):
                require(member.isfile() and member.size == 0, 'unsupported whiteout encoding')
                continue
            row = {'type': 'other', 'mode': oct(member.mode), 'layer': digest}
            elf.pop(name, None)
            if member.isfile():
                require(member.size <= MAX_LAYER, 'file exceeds bound')
                data = layer.extractfile(member).read()
                require(len(data) == member.size, 'truncated archive file')
                row.update(type='file', bytes=len(data), sha256=sha(data))
                if name.startswith('usr/local/bin/python3.') and name.removeprefix('usr/local/bin/python3.').isdigit():
                    require(len(data) >= 20 and data[:6] == b'\x7fELF\x02\x01' and
                            struct.unpack_from('<H', data, 18)[0] == 183, 'unsupported Python ELF')
                    elf[name] = {'format': 'ELF64 little endian', 'machine': 'AArch64',
                                 'bytes': len(data), 'sha256': sha(data)}
            elif member.issym(): row.update(type='symlink', target=member.linkname)
            elif member.islnk(): row.update(type='hardlink', target=member.linkname)
            elif member.isdir(): row.update(type='directory')
            else: raise AssetError('unsupported archive entry type')
            inventory[name] = row
        require(len(inventory) <= MAX_MEMBERS, 'inventory exceeds bound')


def audit_bytes(raw, expected):
    """Pure comparison boundary; private expected values support synthetic tests."""
    require(len(raw) <= MAX_ARCHIVE, 'archive exceeds bound')
    require(expected['archive'] == {'bytes': len(raw), 'sha256': sha(raw)}, 'archive pin differs')
    inventory, elf, layers, total = {}, {}, [], 0
    with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
        files = members(archive)
        def blob(digest):
            require(isinstance(digest, str) and re.fullmatch('sha256:[0-9a-f]{64}', digest), 'invalid blob digest')
            member = files.get('blobs/sha256/' + digest[7:])
            require(member is not None and member.isfile() and member.size <= MAX_ARCHIVE, 'missing or invalid blob')
            body = archive.extractfile(member).read()
            require(len(body) == member.size and sha(body) == digest[7:], 'blob pin differs')
            return body
        index = parsed(blob(expected['imageIndexDigest']))
        matches = [m for m in index['manifests'] if m.get('platform') == expected['platform']]
        require(expected['platform'] == {'architecture': 'arm64', 'os': 'linux', 'variant': 'v8'} and len(matches) == 1,
                'unsupported or ambiguous platform')
        manifest_digest = matches[0]['digest']
        require(manifest_digest == expected['platformManifestDigest'], 'platform manifest differs')
        manifest = parsed(blob(manifest_digest))
        config_digest = manifest['config']['digest']
        require(config_digest == expected['configDigest'], 'configuration differs')
        config = parsed(blob(config_digest))
        require(config['os'] == 'linux' and config['architecture'] == 'arm64', 'configuration platform differs')
        diffs = config['rootfs']['diff_ids']
        require(0 < len(manifest['layers']) == len(diffs) <= 8, 'layer count differs')
        for descriptor, diff in zip(manifest['layers'], diffs):
            packed = blob(descriptor['digest'])
            require(len(packed) == descriptor['size'], 'layer descriptor size differs')
            if packed.startswith(b'\x1f\x8b'):
                with gzip.GzipFile(fileobj=io.BytesIO(packed)) as zipped:
                    plain = zipped.read(MAX_LAYER + 1)
            else: plain = packed
            total += len(plain)
            require(len(plain) <= MAX_LAYER and total <= MAX_TOTAL, 'expanded layer exceeds bound')
            require('sha256:' + sha(plain) == diff, 'layer diff ID differs')
            layers.append({'digest': descriptor['digest'], 'diffId': diff,
                           'packedBytes': len(packed), 'plainBytes': len(plain)})
            apply_layer(plain, descriptor['digest'], inventory, elf)
    ordered = dict(sorted(inventory.items()))
    encoded = json.dumps(ordered, sort_keys=True, indent=2).encode() + b'\n'
    assets = {p: r for p, r in ordered.items() if p.startswith('usr/local/bin/python') or p.startswith('usr/local/lib/libpython')}
    libraries = [r for p, r in ordered.items() if p.startswith('usr/local/lib/python') and r['type'] == 'file']
    require(assets and libraries and elf, 'Python assets unavailable')
    observed = {'imageIndexDigest': expected['imageIndexDigest'], 'platformManifestDigest': manifest_digest,
                'configDigest': config_digest, 'platform': expected['platform'], 'archive': expected['archive'],
                'layers': layers, 'effectiveInventory': {'entries': len(ordered), 'sha256': sha(encoded), 'bytes': len(encoded)},
                'pythonAssets': assets, 'pythonInterpreterELF': elf,
                'pythonLibraryFileCount': len(libraries), 'pythonLibraryBytes': sum(r['bytes'] for r in libraries)}
    require(all(observed[k] == expected[k] for k in FIELDS), 'static asset inventory differs')
    return {'schema': 'runtime.python-image.verification.v1', 'status': 'static_identity_verified',
            **observed, 'runtimeAdmission': False, 'targetExecuted': False}


def verify_archive(path):
    receipt = regular_bytes(RECEIPT, 1024 * 1024)
    require(sha(receipt) == RECEIPT_SHA256, 'reviewed receipt changed')
    return audit_bytes(regular_bytes(path, MAX_ARCHIVE), parsed(receipt))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archive', type=Path)
    args = parser.parse_args()
    try:
        print(json.dumps(verify_archive(args.archive), sort_keys=True, indent=2))
    except (ValueError, OSError, KeyError, TypeError, tarfile.TarError, EOFError, RecursionError):
        parser.exit(2, 'static Python image verification refused\n')


if __name__ == '__main__':
    main()
