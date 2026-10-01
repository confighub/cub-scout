"""Pure source tests for the combined runner; these never call a binary or socket."""
import base64
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
    return module


runner = load("combined_runtime_runner_tests", HERE / "run_pair.py")
payload = load("combined_runtime_payload_tests", HERE / "payload.py")
direct = load("combined_runtime_direct_source", HERE.parent / "direct-cli-controls/probe.py")


class RunnerTests(unittest.TestCase):
    def test_host_staging_accepts_precreated_empty_stage_and_keeps_arm_delta(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as temp:
            root = Path(temp); assets = root / "assets"; assets.mkdir()
            for name in runner.ASSET_NAMES: (assets / name).write_bytes(("fake-" + name).encode())
            def pinned(path, name):
                return {"sha256": runner.file_sha(path), "version": "synthetic-test-asset"}
            stages = {}
            with mock.patch.object(runner.linux, "validate_asset", side_effect=pinned):
                for arm in ("baseline", "treatment"):
                    stage = root / (arm + "-tools"); stage.mkdir()
                    stages[arm] = runner.stage_common(stage, assets, include_scout=(arm == "treatment"))
                    self.assertTrue((stage / "evals/recorded-scale/preflight.py").is_file())
                    self.assertTrue((stage / "evals/direct-cli-controls/probe.py").is_file())
                    self.assertTrue((stage / "payload.py").is_file())
                    self.assertEqual((stage / "plugin").is_dir(), arm == "treatment")
                self.assertNotIn("bin/cub-scout", stages["baseline"])
                self.assertIn("bin/cub-scout", stages["treatment"])
                self.assertEqual({k: v for k, v in stages["baseline"].items() if k != "_stageFiles"},
                                 {k: v for k, v in stages["treatment"].items()
                                  if k not in ("_stageFiles", "bin/cub-scout")})
                for stage in (root / "baseline-tools", root / "treatment-tools"):
                    for item in sorted(stage.rglob("*"), reverse=True):
                        if item.is_dir(): item.chmod(0o700)
                        else: item.chmod(0o600)
                    stage.chmod(0o700)

    def test_cleanup_identity_uses_shared_owned_label_and_default_docker_is_linux(self):
        self.assertEqual(runner.OWNER_LABEL, runner.linux.OWNER_LABEL)
        self.assertEqual(runner.DOCKER_DEFAULT, Path("/usr/local/bin/docker"))

    def test_inspect_rejects_unowned_or_escaped_container_settings(self):
        with tempfile.TemporaryDirectory(dir="/tmp") as temp:
            root = Path(temp); stage = root / "tools"; stage.mkdir(); plugin = root / "plugin"; plugin.mkdir()
            name, owner = "scout-combined-0123456789abcdef", "a" * 32
            value = {"Id": "b" * 64, "Name": "/" + name, "Image": runner.IMAGE_ID,
                "Config": {"Image": runner.IMAGE_ID, "User": "65534:65534",
                    "Labels": {runner.OWNER_LABEL: owner}},
                "HostConfig": {"ReadonlyRootfs": True, "NetworkMode": "none", "CapDrop": ["ALL"],
                    "CapAdd": [], "PidMode": "private", "IpcMode": "private",
                    "SecurityOpt": ["no-new-privileges"], "PidsLimit": 64, "Memory": 1073741824,
                    "NanoCpus": 1000000000, "Privileged": False,
                    "Tmpfs": {"/tmp": "rw,nosuid,nodev,size=67108864,uid=65534,gid=65534"}},
                "Mounts": [{"Type": "bind", "Source": str(stage), "Destination": "/tools", "RW": False},
                           {"Type": "bind", "Source": str(plugin), "Destination": "/tools/plugin", "RW": False}],
                "State": {"Status": "exited", "ExitCode": 0}}
            self.assertEqual(runner.inspect_runtime(json.dumps(value).encode(), name, owner, stage, plugin, finished=True), "b" * 64)
            for mutate in (lambda x: x["HostConfig"].update(CapAdd=["NET_ADMIN"]),
                           lambda x: x["Mounts"].append({"Type": "volume", "Destination": "/etc"}),
                           lambda x: x["Config"]["Labels"].update({runner.OWNER_LABEL: "c" * 32})):
                changed = copy.deepcopy(value); mutate(changed)
                with self.assertRaises((ValueError, RuntimeError)):
                    runner.inspect_runtime(json.dumps(changed).encode(), name, owner, stage, plugin, finished=True)

    def test_bash_transport_only_allows_one_final_newline_removed(self):
        expected = '{"captured":true}\n'
        self.assertTrue(payload.exact_bash_body(expected, expected))
        self.assertTrue(payload.exact_bash_body(expected[:-1], expected))
        for actual in ('prefix' + expected, expected + 'suffix', '{ "captured":true}', '{"captured":false}'):
            self.assertFalse(payload.exact_bash_body(actual, expected))

    def test_tool_schedule_only_uses_expected_safe_tools(self):
        names = [{"name": "mcp__cub-scout__map"}, {"name": "mcp__cub-scout__explain"}]
        baseline = payload.tool_plan("baseline", names)
        treatment = payload.tool_plan("treatment", names)
        self.assertEqual([n for n, _ in baseline], ["Read", "Bash", "Task", "Agent"])
        self.assertEqual([n for n, _ in treatment], ["Read", "Bash", "Task", "Agent", "mcp__cub-scout__map"])
        with self.assertRaises(RuntimeError): payload.tool_plan("treatment", [{"name": "other"}])
        with self.assertRaises(RuntimeError): payload.tool_plan("treatment", names + [{"name": "mcp__other__map"}])

    def test_synthetic_responses_are_parseable_json_and_sse(self):
        for stream in (False, True):
            kind, body = payload.provider_response(stream, "baseline", 1, [])
            record = {"response_content_type": kind, "response_body_base64": base64.b64encode(body).decode()}
            uses = direct._response_tool_uses(record)
            self.assertEqual(uses, [{"type": "tool_use", "id": "toolu_combined_baseline_1", "name": "Read",
                                     "input": {"file_path": "/tools/evidence/events.yaml"}}])
            kind, body = payload.provider_response(stream, "baseline", 5, [])
            parsed = direct.strict_json(body) if not stream else None
            self.assertTrue((parsed is not None and parsed["content"][0]["text"] == "offline-probe-terminal")
                            or (stream and b"offline-probe-terminal" in body))

    def test_arm_container_args_are_offline_and_treatment_scoped(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp); stage = root / "tools"; stage.mkdir(); plugin = root / "plugin"; plugin.mkdir()
            baseline = runner.custom_create_args("scout-combined-0123456789abcdef", "a" * 32, stage, plugin=None)
            treatment = runner.custom_create_args("scout-combined-fedcba9876543210", "b" * 32, stage, plugin=plugin)
            for argv in (baseline, treatment):
                joined = " ".join(argv)
                for required in ("--network=none", "--read-only", "65534:65534", "--cap-drop=ALL",
                                 "--pids-limit=64", "--memory=1g", "--cpus=1", "size=64m"):
                    self.assertIn(required, joined)
            self.assertNotIn("dst=/tools/plugin", " ".join(baseline))
            self.assertIn("dst=/tools/plugin,readonly", " ".join(treatment))
            self.assertNotIn("/tools/cub-scout", " ".join(baseline))

    def test_staged_treatment_has_35_skill_sources_and_neutral_mcp_metadata(self):
        with tempfile.TemporaryDirectory() as temp:
            staged = Path(temp) / "plugin"
            facts = runner.stage_treatment_plugin(staged, Path(temp))
            self.assertEqual(facts["skillCount"], 35)
            self.assertEqual(facts["skillTreeSha256"], runner.contract.SKILL_TREE_SHA256)
            plugin_json = json.loads((staged / ".claude-plugin/plugin.json").read_text())
            self.assertEqual(plugin_json["mcpServers"], {})
            wrapper = (staged / "bin/recorded-cub-scout").read_text()
            self.assertIn("env -i", wrapper)
            self.assertIn("KUBECONFIG=/tmp/empty-kubeconfig", wrapper)
            self.assertIn("mcp serve --recording /tools/evidence/deployments.yaml", wrapper)

    def test_payload_environment_contains_only_synthetic_provider_credentials(self):
        text = (HERE / "payload.py").read_text()
        self.assertIn('"ANTHROPIC_API_KEY": FAKE_KEY', text)
        self.assertIn('"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"', text)
        self.assertIn('"CLAUDE_CODE_DISABLE_FAST_MODE": "1"', text)
        self.assertIn('"CLAUDE_CONFIG_DIR": str(config_dir)', text)
        self.assertNotIn("ANTHROPIC_AUTH_TOKEN", text)
        self.assertNotIn("os.environ", text)


if __name__ == "__main__":
    unittest.main()
