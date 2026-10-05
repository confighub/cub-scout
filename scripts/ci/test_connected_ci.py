#!/usr/bin/env python3
"""Offline guards for fail-closed provisioning and real Go acceptance outcomes."""
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

from require_test_passes import require_passes

ROOT = Path(__file__).resolve().parents[2]
SETUP = ROOT / "scripts/ci/setup-connected-server.sh"


class RequiredOutcomes(unittest.TestCase):
    def test_ci_map_namespace_argv_is_accepted_by_actual_cli(self):
        with tempfile.TemporaryDirectory() as directory:
            fixture = Path(directory) / 'entries.json'
            fixture.write_text(json.dumps([{
                'id': 'test/boutique/apps/Deployment/frontend', 'clusterName': 'test',
                'namespace': 'boutique', 'kind': 'Deployment', 'name': 'frontend',
                'apiVersion': 'apps/v1', 'owner': 'Flux', 'status': 'Ready',
            }]))
            workflow = (ROOT / '.github/workflows/ci.yaml').read_text()
            command = next(line.strip().split(' > ', 1)[0] for line in workflow.splitlines()
                           if './cub-scout map list' in line and 'flux-ownership.json' in line)
            argv = shlex.split(command)
            argv[0] = str(ROOT / 'cub-scout')
            result = subprocess.run(argv, capture_output=True, text=True,
                                    env={**os.environ, 'CUB_SCOUT_TEST_MAP_ENTRIES_JSON': str(fixture)})
            self.assertEqual(result.returncode, 0, result.stderr)
            entries = json.loads(result.stdout)
            self.assertEqual([(entry['namespace'], entry['name'], entry['owner']) for entry in entries],
                             [('boutique', 'frontend', 'Flux')])

    def stream(self, action="pass", package_action="pass"):
        return [json.dumps(event) for event in (
            {"Package": "example", "Test": "TestImport", "Action": action},
            {"Package": "example", "Action": package_action},
        )]

    def test_completed_pass_is_accepted(self):
        require_passes(self.stream(), ["TestImport"])

    def test_skip_failure_missing_and_truncation_are_rejected(self):
        for stream in (self.stream("skip"), self.stream("fail"),
                       self.stream(package_action="fail"), [], self.stream()[:1],
                       self.stream()[1:], ["not JSON"], ["[]"], self.stream() * 2):
            with self.subTest(stream=stream), self.assertRaises(ValueError):
                require_passes(stream, ["TestImport"])

    def test_another_package_cannot_supply_completion(self):
        stream = self.stream()
        stream[1] = json.dumps({"Package": "other", "Action": "pass"})
        with self.assertRaises(ValueError):
            require_passes(stream, ["TestImport"])


class Provisioning(unittest.TestCase):
    def test_missing_scanner_credential_fails_before_installation(self):
        with tempfile.TemporaryDirectory() as directory:
            environment = dict(os.environ)
            environment.pop('CUB_SCAN_RELEASE_TOKEN', None)
            environment['RUNNER_TEMP'] = directory
            result = subprocess.run(['bash', str(ROOT / 'scripts/ci/setup-scan-provider.sh')],
                                    capture_output=True, text=True, env=environment)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('read-only credential', result.stderr)
            self.assertFalse((Path(directory) / 'scout-scan-v073').exists())

    def run_setup(self, existing_cluster=False, existing_root=False):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            tools = base / "bin"
            tools.mkdir()
            scripts = {
                "uname": '#!/bin/sh\n[ "$1" = -s ] && echo Linux || echo x86_64\n',
                "kubectl": '#!/bin/sh\necho fixture-kubeconfig\n',
                "kind": '#!/bin/sh\necho ' + ("scout-ci-connected-server" if existing_cluster else "workload") + '\n',
                "curl": '#!/bin/sh\nwhile [ "$1" != -o ]; do shift; done\nprintf tampered > "$2"\n',
                "sha256sum": '#!/usr/bin/env python3\nimport hashlib,sys\nfrom pathlib import Path\ndigest,path=sys.stdin.read().strip().split("  ",1)\nsys.exit(0 if hashlib.sha256(Path(path).read_bytes()).hexdigest()==digest else 1)\n',
            }
            for name, content in scripts.items():
                path = tools / name
                path.write_text(content)
                path.chmod(0o700)
            private = base / "private"
            if existing_root:
                private.mkdir()
            result = subprocess.run(["bash", str(SETUP), str(private)], capture_output=True, text=True,
                                    env={**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"],
                                         "GITHUB_ENV": str(base / "env"), "GITHUB_PATH": str(base / "path")})
            return result, (private / "owns-server-cluster").exists(), (base / "env").exists()

    def test_tampered_download_never_authenticates_or_owns_cluster(self):
        result, owns, exported = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(owns)
        self.assertFalse(exported)

    def test_existing_cluster_is_not_reused_or_cleaned(self):
        result, owns, exported = self.run_setup(existing_cluster=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Refusing to reuse", result.stderr)
        self.assertFalse(owns)
        self.assertFalse(exported)

    def test_existing_private_directory_is_rejected(self):
        result, owns, exported = self.run_setup(existing_root=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("new absolute", result.stderr)
        self.assertFalse(owns)
        self.assertFalse(exported)


if __name__ == "__main__":
    unittest.main()
