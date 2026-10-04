"""Pure static-image controls; no Docker, Linux executable or network."""
import copy
import gzip
import io
import json
import tempfile
import tarfile
from pathlib import Path
import unittest
from unittest.mock import patch

import python_image as image


def tar(rows):
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode='w') as archive:
        for name, value in rows:
            member = tarfile.TarInfo(name)
            member.size = len(value)
            member.mode = 0o755
            archive.addfile(member, io.BytesIO(value))
    return out.getvalue()


def fixture(*, config_diff=None, paths=None, repeated_platform=False):
    elf = b'\x7fELF\x02\x01' + bytes(12) + b'\xb7\x00' + bytes(12)
    rows = paths or [('usr/local/bin/python3.11', elf),
                     ('usr/local/lib/python3.11/json.py', b'fixture standard library')]
    plain = tar(rows); packed = gzip.compress(plain, mtime=0)
    config = json.dumps({'os': 'linux', 'architecture': 'arm64', 'rootfs': {
        'diff_ids': [config_diff or 'sha256:' + image.sha(plain)]}}).encode()
    descriptor = {'digest': 'sha256:' + image.sha(packed), 'size': len(packed)}
    manifest = json.dumps({'config': {'digest': 'sha256:' + image.sha(config)},
                           'layers': [descriptor]}).encode()
    platform = {'architecture': 'arm64', 'os': 'linux', 'variant': 'v8'}
    ref = {'digest': 'sha256:' + image.sha(manifest), 'platform': platform}
    index = json.dumps({'manifests': [ref, ref] if repeated_platform else [ref]}).encode()
    archive = tar([('blobs/sha256/' + image.sha(value), value)
                   for value in (index, manifest, config, packed)])
    inventory = {name: {'type': 'file', 'mode': '0o755', 'layer': descriptor['digest'],
                       'bytes': len(value), 'sha256': image.sha(value)} for name, value in rows}
    encoded = json.dumps(dict(sorted(inventory.items())), sort_keys=True, indent=2).encode() + b'\n'
    expected = {'imageIndexDigest': 'sha256:' + image.sha(index),
                'platformManifestDigest': ref['digest'], 'configDigest': 'sha256:' + image.sha(config),
                'platform': platform, 'archive': {'bytes': len(archive), 'sha256': image.sha(archive)},
                'layers': [{'digest': descriptor['digest'], 'diffId': 'sha256:' + image.sha(plain),
                            'packedBytes': len(packed), 'plainBytes': len(plain)}],
                'effectiveInventory': {'entries': len(rows), 'bytes': len(encoded), 'sha256': image.sha(encoded)},
                'pythonAssets': {'usr/local/bin/python3.11': inventory.get('usr/local/bin/python3.11')},
                'pythonInterpreterELF': {'usr/local/bin/python3.11': {'format': 'ELF64 little endian',
                     'machine': 'AArch64', 'bytes': len(elf), 'sha256': image.sha(elf)}},
                'pythonLibraryFileCount': 1, 'pythonLibraryBytes': len(b'fixture standard library')}
    return archive, expected


class PythonImageTests(unittest.TestCase):
    def test_exact_static_candidate_never_admits_runtime(self):
        raw, expected = fixture()
        result = image.audit_bytes(raw, expected)
        self.assertEqual(result['pythonInterpreterELF'], expected['pythonInterpreterELF'])
        self.assertFalse(result['runtimeAdmission'])
        self.assertFalse(result['targetExecuted'])
        self.assertEqual(result['status'], 'static_identity_verified')

    def test_archive_and_each_receipt_identity_are_binding(self):
        raw, expected = fixture()
        with self.assertRaisesRegex(image.AssetError, 'archive pin'):
            image.audit_bytes(raw[:-1], expected)
        for key in ('imageIndexDigest', 'platformManifestDigest', 'configDigest'):
            bad = copy.deepcopy(expected); bad[key] = 'sha256:' + '0' * 64
            with self.subTest(key=key), self.assertRaises(image.AssetError):
                image.audit_bytes(raw, bad)
        for key in ('layers', 'effectiveInventory', 'pythonAssets', 'pythonInterpreterELF',
                    'pythonLibraryFileCount', 'pythonLibraryBytes'):
            bad = copy.deepcopy(expected); bad[key] = None
            with self.subTest(key=key), self.assertRaisesRegex(image.AssetError, 'inventory differs'):
                image.audit_bytes(raw, bad)

    def test_scope_diff_and_invalid_elf_refuse(self):
        raw, expected = fixture(repeated_platform=True)
        with self.assertRaisesRegex(image.AssetError, 'ambiguous platform'):
            image.audit_bytes(raw, expected)
        raw, expected = fixture(config_diff='sha256:' + '0' * 64)
        with self.assertRaisesRegex(image.AssetError, 'diff ID'):
            image.audit_bytes(raw, expected)
        raw, expected = fixture(paths=[('usr/local/bin/python3.11', b'not an ELF')])
        with self.assertRaisesRegex(image.AssetError, 'Python ELF'):
            image.audit_bytes(raw, expected)

    def test_unsafe_duplicate_paths_and_json_refuse(self):
        for path in ('../escape', '/absolute', 'a/../escape'):
            raw, expected = fixture(paths=[(path, b'x')])
            with self.subTest(path=path), self.assertRaisesRegex(image.AssetError, 'unsafe archive path'):
                image.audit_bytes(raw, expected)
        raw, expected = fixture(paths=[('a', b'x'), ('./a', b'y')])
        with self.assertRaisesRegex(image.AssetError, 'duplicate archive path'):
            image.audit_bytes(raw, expected)
        for raw in (b'{"a":1,"a":2}', b'{"a":NaN}'):
            with self.assertRaises(image.AssetError): image.parsed(raw)

    def test_compression_and_member_limits_refuse(self):
        raw, expected = fixture()
        with patch.object(image, 'MAX_LAYER', 512), self.assertRaisesRegex(image.AssetError, 'expanded layer'):
            image.audit_bytes(raw, expected)
        with patch.object(image, 'MAX_MEMBERS', 1), self.assertRaisesRegex(image.AssetError, 'member bound'):
            image.audit_bytes(raw, expected)
        with patch.object(image, 'MAX_ARCHIVE', 10), self.assertRaisesRegex(image.AssetError, 'archive exceeds'):
            image.audit_bytes(raw, expected)

    def test_blob_duplicate_outer_and_metadata_depth_refuse(self):
        raw, expected = fixture()
        with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
            rows = [(m.name, archive.extractfile(m).read()) for m in archive if m.isfile()]
        corrupt = tar([(name, value + b'x' if name.endswith(expected['imageIndexDigest'][7:]) else value)
                       for name, value in rows])
        bad = copy.deepcopy(expected); bad['archive'] = {'bytes': len(corrupt), 'sha256': image.sha(corrupt)}
        with self.assertRaisesRegex(image.AssetError, 'blob pin differs'):
            image.audit_bytes(corrupt, bad)
        duplicate = tar(rows + [rows[0]])
        bad['archive'] = {'bytes': len(duplicate), 'sha256': image.sha(duplicate)}
        with self.assertRaisesRegex(image.AssetError, 'duplicate archive path'):
            image.audit_bytes(duplicate, bad)
        with self.assertRaisesRegex(image.AssetError, 'nesting bound'):
            image.parsed(b'[' * 34 + b'0' + b']' * 34)
        with patch.object(image, 'MAX_TOTAL', 512), self.assertRaisesRegex(image.AssetError, 'expanded layer'):
            image.audit_bytes(raw, expected)

    def test_whiteouts_remove_descendants_and_opaque_directory_preserves_new_rows(self):
        inventory = {'d': {'type': 'directory'}, 'd/old': {'type': 'file'},
                     'd/sub/old': {'type': 'file'}, 'keep': {'type': 'file'}}
        image.apply_layer(tar([('d/.wh..wh..opq', b''), ('d/new', b'new')]), 'layer', inventory, {})
        self.assertEqual(set(inventory), {'d', 'd/new', 'keep'})
        image.apply_layer(tar([('.wh.d', b'')]), 'layer2', inventory, {})
        self.assertEqual(set(inventory), {'keep'})
        image.apply_layer(tar([('.wh..wh..opq', b''), ('new', b'x')]), 'layer3', inventory, {})
        self.assertEqual(set(inventory), {'new'})

    def test_regular_file_and_reviewed_receipt_required(self):
        raw, _ = fixture()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); path = root / 'image.tar'; path.write_bytes(raw)
            link = root / 'link'; link.symlink_to(path)
            with self.assertRaises(OSError): image.regular_bytes(link, image.MAX_ARCHIVE)
            with self.assertRaises(image.AssetError): image.regular_bytes(root, image.MAX_ARCHIVE)
            receipt = root / 'receipt.json'; receipt.write_text('{}')
            with patch.object(image, 'RECEIPT', receipt), self.assertRaisesRegex(image.AssetError, 'reviewed receipt changed'):
                image.verify_archive(path)


if __name__ == '__main__':
    unittest.main()
