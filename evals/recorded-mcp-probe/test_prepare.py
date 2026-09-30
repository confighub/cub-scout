#!/usr/bin/env python3
"""No-model tests for preparation refusal and generated wrapper boundaries."""
import hashlib
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('prepare', ROOT / 'prepare.py')
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)

class PreparationGuards(unittest.TestCase):
    def test_hash_mismatch_and_existing_output_are_not_overwritten(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'binary'
            binary.write_text('#!/bin/sh\nexit 71\n')
            binary.chmod(0o700)
            out = root / 'output'
            args = [sys.executable, str(ROOT / 'prepare.py'), '--binary', str(binary), '--out', str(out), '--binary-sha256']
            mismatch = subprocess.run(args + ['0' * 64], text=True, capture_output=True, timeout=10)
            self.assertNotEqual(mismatch.returncode, 0)
            self.assertIn('binary hash mismatch', mismatch.stderr)
            self.assertFalse(out.exists())
            out.mkdir()
            sentinel = out / 'retained'
            sentinel.write_text('existing evidence')
            existing = subprocess.run(args + [prepare.sha256(binary)], text=True, capture_output=True, timeout=10)
            self.assertNotEqual(existing.returncode, 0)
            self.assertIn('refusing to overwrite', existing.stderr)
            self.assertEqual(sentinel.read_text(), 'existing evidence')

    def test_wrapper_quotes_paths_and_clears_inherited_environment(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            unusual = root / "space 'quote' $(touch PWNED)"
            unusual.mkdir()
            binary = unusual / 'binary'
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "${SCOUT_TEST_SECRET-unset}" "$HOME" "$KUBECONFIG" "$@"\n')
            binary.chmod(0o700)
            fixture = unusual / 'recording'
            fixture.write_bytes(prepare.SOURCE_FIXTURE.read_bytes())
            home = unusual / 'home'
            home.mkdir()
            kubeconfig = unusual / 'kubeconfig'
            kubeconfig.write_bytes(b'')
            wrapper = root / 'wrapper'
            text = (ROOT / 'template/plugin/server-wrapper.sh.in').read_text()
            values = {'BINARY_PATH': prepare.shell_quote(str(binary)),
                      'BINARY_SHA256': prepare.sha256(binary),
                      'RECORDING_PATH': prepare.shell_quote(str(fixture)),
                      'SERVER_HOME': prepare.shell_quote(str(home)),
                      'EMPTY_KUBECONFIG': prepare.shell_quote(str(kubeconfig))}
            for key, value in values.items():
                text = text.replace('@' + key + '@', value)
            wrapper.write_text(text)
            wrapper.chmod(0o700)
            env = {**os.environ, 'SCOUT_TEST_SECRET': 'must-not-reach-server'}
            result = subprocess.run([str(wrapper)], cwd=root, env=env, text=True, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), ['unset', str(home), str(kubeconfig), 'mcp', 'serve', '--recording', str(fixture)])
            self.assertFalse((root / 'PWNED').exists())
            fixture.write_bytes(b'changed')
            rejected = subprocess.run([str(wrapper)], cwd=root, env=env, text=True, capture_output=True, timeout=10)
            self.assertNotEqual(rejected.returncode, 0)
            self.assertIn('recording hash mismatch', rejected.stderr)
            self.assertEqual(rejected.stdout, '')

if __name__ == '__main__':
    unittest.main()
