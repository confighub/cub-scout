#!/usr/bin/env python3
"""Offline tests for recorded scale staging and preflight validation."""
import hashlib
import os
import signal
import time
import copy
import importlib.util
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("recorded_scale_prepare", ROOT / "prepare.py")
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)
spec = importlib.util.spec_from_file_location("recorded_scale_preflight", ROOT / "preflight.py")
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)
spec = importlib.util.spec_from_file_location("recorded_scale_regrade", ROOT.parent / "scripts/regrade.py")
regrade = importlib.util.module_from_spec(spec)
spec.loader.exec_module(regrade)


def expected_report():
    names = preflight.NO_MARKER
    resources = [{"namespace": item.split("/", 1)[0], "name": item.split("/", 1)[1], "owner": "Native"}
                 for item in names]
    for owner, count in preflight.EXPECTED_COUNTS.items():
        if owner != "Native":
            resources.extend({"namespace": "team-fixture", "name": f"{owner.lower()}-{i}", "owner": owner}
                             for i in range(count))
    for resource in resources:
        resource.update(apiVersion="apps/v1", kind="Deployment")
    return {
        "schema": "map-list-recorded.v1",
        "provenance": {"objectCount": 302, "sha256": preflight.DEPLOYMENT_SHA256,
                       "captureTime": "unknown", "captureCompleteness": "unknown"},
        "scope": {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"},
        "selectedCount": 300, "excludedFromScopeCount": 2,
        "ownerCounts": copy.deepcopy(preflight.EXPECTED_COUNTS),
        "resources": resources,
    }


def expected_views_reports():
    full = expected_report()
    full["provenance"].update(kind="kubernetes-object-recording", bytes=1031673, documents=1)
    for resource in full["resources"]:
        if resource["owner"] == "Native":
            resource["ownershipDetection"] = {"status": "no_known_marker", "source": "builtin"}
    native = copy.deepcopy(full)
    native["scope"]["owner"] = "Native"
    native["resources"] = [r for r in native["resources"] if r["owner"] == "Native"]
    native["selectedCount"] = 12
    native["excludedFromScopeCount"] = 290
    native["ownerCounts"] = {"Native": 12}

    def summarize(report):
        summary = copy.deepcopy(report)
        summary["schema"] = "map-list-recorded-summary.v1"
        summary["view"] = "summary"
        summary["perObjectEvidenceGuide"] = (
            "Per-object detector evidence is omitted in this summary; request the full recorded inventory without summary.")
        del summary["resources"]
        return summary

    return {"full": full, "summary": summarize(full), "native": native,
            "native-summary": summarize(native)}


def views_map_schema():
    return {"type": "object", "properties": {
        **{key: {"type": "string"} for key in ("api_version", "kind", "namespace")},
        "namespace_prefix": {"type": "string", "minLength": 1},
        "owner": {"type": "string", "enum": list(preflight.VIEW_OWNERS)},
        "summary": {"type": "boolean"}},
        "required": ["api_version", "kind"], "additionalProperties": False}


def fake_views_cli(path: Path, reports: dict) -> Path:
    payload = repr(json.dumps(reports))
    source = ("#!" + sys.executable + "\nimport json,sys\n"
              "reports=json.loads(" + payload + ")\nargs=sys.argv[1:]\n"
              "owner='Native' if '--owner' in args else None\n"
              "summary='--summary' in args\n"
              "key=('native-summary' if summary else 'native') if owner else ('summary' if summary else 'full')\n"
              "print(json.dumps(reports[key],separators=(',',':')))\n")
    path.write_text(source)
    path.chmod(0o700)
    return path


def fake_views_mcp(path: Path, reports: dict, schema: dict | None = None) -> Path:
    payload = repr(json.dumps(reports))
    schema_literal = repr(schema or views_map_schema())
    source = ("#!" + sys.executable + "\nimport json,sys\n"
              "reports=json.loads(" + payload + ")\nmap_schema=" + schema_literal + "\n"
              "for line in sys.stdin:\n"
              " m=json.loads(line); i=m.get('id'); method=m.get('method'); p=m.get('params',{})\n"
              " if i is None: continue\n"
              " if method == 'initialize': result={'protocolVersion':'2024-11-05'}\n"
              " elif method == 'tools/list': result={'tools':[{'name':'explain'},{'name':'map','inputSchema':map_schema}]}\n"
              " elif method == 'tools/call' and p.get('name') not in ('map','explain'): result={'isError':True,'content':[]}\n"
              " elif method == 'tools/call' and p.get('name') == 'map' and 'kube_context' in p.get('arguments',{}): result={'isError':True,'content':[]}\n"
              " elif method == 'tools/call' and p.get('name') == 'map':\n"
              "  a=p.get('arguments',{}); owner='Native' if a.get('owner') == 'Native' else None; summary=a.get('summary') is True\n"
              "  key=('native-summary' if summary else 'native') if owner else ('summary' if summary else 'full')\n"
              "  result={'content':[{'type':'text','text':json.dumps(reports[key],separators=(',',':'))}]}\n"
              " else: result={}\n"
              " print(json.dumps({'jsonrpc':'2.0','id':i,'result':result}),flush=True)\n")
    path.write_text(source)
    path.chmod(0o700)
    return path


def grader_pattern(path: Path):
    text = path.read_text()
    pattern = re.search(r"^pattern: '(.*)'$", text, re.M)
    flags = re.search(r"^flags: ([A-Za-z]*)$", text, re.M)
    return pattern.group(1), flags.group(1) if flags else ""


def javascript_regex_matches(node: str, pattern: str, flags: str, candidates: list[str]):
    payload = json.dumps({"pattern": pattern, "flags": flags, "candidates": candidates})
    program = ("const p=JSON.parse(process.argv[1]); const r=new RegExp(p.pattern,p.flags); "
               "process.stdout.write(JSON.stringify(p.candidates.map(s=>r.test(s))));")
    result = subprocess.run([node, "-e", program, payload], text=True, capture_output=True,
                            timeout=5, check=True)
    return json.loads(result.stdout)


def python_regrader_matches(path: Path, candidates: list[str]):
    grader = regrade.parse_grader(path, path.read_bytes())
    return [bool(grader["compiled"].search(candidate)) for candidate in candidates]


class RecordedScalePacket(unittest.TestCase):
    def test_strict_answer_contract_is_opt_in_and_hashes_generated_files(self):
        node = shutil.which("node")
        if node is None:
            self.skipTest("node is needed to exercise the eval harness JavaScript regex dialect")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            legacy_plugin = root / "legacy-plugin"
            strict_plugin = root / "strict-plugin"
            legacy_plugin.mkdir()
            strict_plugin.mkdir()
            source_bytes = {}
            for case, (grader, _) in prepare.STRICT_GRADERS.items():
                case_dir = prepare.REPO / "evals/scale" / case
                source_bytes[case] = ((case_dir / "prompt.md").read_bytes(),
                                      (case_dir / "graders" / grader).read_bytes())
                prepare.stage_case(legacy_plugin, case)
                prepare.stage_case(strict_plugin, case, prepare.ANSWER_CONTRACT_STRICT)
                legacy_case = legacy_plugin / "evals/recorded-scale" / case
                strict_case = strict_plugin / "evals/recorded-scale" / case
                self.assertEqual((legacy_case / "prompt.md").read_bytes(), source_bytes[case][0])
                self.assertEqual((legacy_case / "graders" / grader).read_bytes(), source_bytes[case][1])
                self.assertIn(prepare.STRICT_PROMPT_SUFFIX,
                              (strict_case / "prompt.md").read_text())
                self.assertEqual((prepare.REPO / "evals/scale" / case / "prompt.md").read_bytes(),
                                 source_bytes[case][0])
                self.assertEqual((prepare.REPO / "evals/scale" / case / "graders" / grader).read_bytes(),
                                 source_bytes[case][1])
                for relative in ("case.yaml", "fixtures", "mocks", "scaffold.sh"):
                    legacy_path, strict_path = legacy_case / relative, strict_case / relative
                    if relative == "fixtures" or relative == "mocks":
                        legacy_files = {p.relative_to(legacy_path): p.read_bytes()
                                        for p in legacy_path.rglob("*") if p.is_file()}
                        strict_files = {p.relative_to(strict_path): p.read_bytes()
                                        for p in strict_path.rglob("*") if p.is_file()}
                        self.assertEqual(legacy_files, strict_files, f"strict staging changed {case}/{relative}")
                    else:
                        self.assertEqual(legacy_path.read_bytes(), strict_path.read_bytes())
                strict_grader = strict_case / "graders" / grader
                self.assertNotEqual(strict_grader.read_bytes(), source_bytes[case][1])
                legacy_files = {p.relative_to(legacy_case): p.read_bytes()
                                for p in legacy_case.rglob("*") if p.is_file()}
                strict_files = {p.relative_to(strict_case): p.read_bytes()
                                for p in strict_case.rglob("*") if p.is_file()}
                self.assertEqual(set(legacy_files), set(strict_files))
                changed_paths = {path for path in legacy_files if legacy_files[path] != strict_files[path]}
                self.assertEqual(changed_paths, {Path("prompt.md"), Path("graders") / grader})

            # A historically accepted answer can contain a contradictory line:
            # the legacy multiline grader finds the correct line anywhere, while
            # the generated strict grader requires the whole final message.
            contradictory_counts = (
                "COUNTS: flux=120 argocd=90 helm=45 confighub=33 unmanaged=12\n"
                "Correction: unmanaged=999."
            )
            contradictory_unmanaged = (
                "UNMANAGED: " + ", ".join(prepare.UNMANAGED_NAMES) +
                "\nActually, one additional workload is unmanaged."
            )
            for case, grader, contradictory in (
                ("scale-ownership-counts", "counts-line.md", contradictory_counts),
                ("scale-unmanaged", "unmanaged-line.md", contradictory_unmanaged),
            ):
                legacy_path = prepare.REPO / "evals/scale" / case / "graders" / grader
                strict_path = strict_plugin / "evals/recorded-scale" / case / "graders" / grader
                legacy_pattern, legacy_flags = grader_pattern(legacy_path)
                strict_pattern, strict_flags = grader_pattern(strict_path)
                self.assertEqual(strict_flags, "s", "strict grader must not enable multiline or case-insensitive matching")
                accepted = javascript_regex_matches(node, legacy_pattern, legacy_flags, [contradictory])[0]
                rejected = javascript_regex_matches(node, strict_pattern, strict_flags, [contradictory])[0]
                self.assertTrue(accepted, f"legacy grader no longer demonstrates multiline acceptance: {case}")
                self.assertFalse(rejected, f"strict grader accepted contradictory prose: {case}")
                self.assertEqual(python_regrader_matches(strict_path, [contradictory]), [rejected])

            counts_case = strict_plugin / "evals/recorded-scale/scale-ownership-counts"
            counts_pattern, counts_flags = grader_pattern(counts_case / "graders/counts-line.md")
            counts_line = "COUNTS: flux=120 argocd=90 helm=45 confighub=33 unmanaged=12"
            counts_candidates = [
                " \n\t" + counts_line + " \t\n", counts_line + "\nContradiction",
                "Explanation\n" + counts_line, "```\n" + counts_line + "\n```",
                "COUNTS: flux=120 argocd=90 helm=45 confighub=33 unmanaged=13",
                "COUNTS: flux=120 argocd=90 helm=45 confighub=33",
                counts_line + " extra", "COUNTS: flux=120 argocd=90 helm=45 confighub=33 unmanaged=12 other=0",
            ]
            counts_matches = javascript_regex_matches(node, counts_pattern, counts_flags, counts_candidates)
            self.assertEqual(counts_matches, [True, False, False, False, False, False, False, False])
            self.assertEqual(python_regrader_matches(counts_case / "graders/counts-line.md", counts_candidates),
                             counts_matches)

            unmanaged_case = strict_plugin / "evals/recorded-scale/scale-unmanaged"
            unmanaged_pattern, unmanaged_flags = grader_pattern(unmanaged_case / "graders/unmanaged-line.md")
            exact_unmanaged = "UNMANAGED: " + ", ".join(prepare.UNMANAGED_NAMES)
            valid_unmanaged = "\n  " + "UNMANAGED: " + ", ".join(reversed(prepare.UNMANAGED_NAMES)) + "\t\n"
            missing = "UNMANAGED: " + ", ".join(prepare.UNMANAGED_NAMES[:-1])
            duplicate = "UNMANAGED: " + ", ".join((prepare.UNMANAGED_NAMES[0],) + prepare.UNMANAGED_NAMES[:-1])
            extra = "UNMANAGED: " + ", ".join(prepare.UNMANAGED_NAMES + ("team-99/extra",))
            wrong_namespace = "UNMANAGED: " + ", ".join(
                ("team-02/authentic",) + prepare.UNMANAGED_NAMES[1:])
            unmanaged_candidates = [valid_unmanaged, exact_unmanaged + "\nMore prose", "Details\n" + exact_unmanaged,
                                    "```\n" + exact_unmanaged + "\n```", missing, duplicate, extra,
                                    wrong_namespace, exact_unmanaged + "suffix"]
            unmanaged_matches = javascript_regex_matches(node, unmanaged_pattern, unmanaged_flags,
                                                           unmanaged_candidates)
            self.assertEqual(unmanaged_matches, [True, False, False, False, False, False, False, False, False])
            self.assertEqual(python_regrader_matches(unmanaged_case / "graders/unmanaged-line.md",
                                                      unmanaged_candidates), unmanaged_matches)

    def test_staged_pair_files_are_byte_identical_and_match_pinned_sources(self):
        with tempfile.TemporaryDirectory() as tmp:
            plugin = Path(tmp) / "plugin"
            plugin.mkdir()
            facts = prepare.stage_case(plugin, "scale-ownership-counts")
            self.assertEqual(set(facts), {Path(p).name for p in prepare.FILES})
            for name, metadata in facts.items():
                raw = (plugin / "evals/recorded-scale/scale-ownership-counts/fixtures" / name).read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), metadata["sha256"])
            # The generated shell scaffold is exercised twice and compares every byte.
            self.assertEqual((plugin / "evals/recorded-scale/scale-ownership-counts/prompt.md").read_bytes(),
                             (prepare.REPO / "evals/scale/scale-ownership-counts/prompt.md").read_bytes())

    def test_scope_counts_and_all_twelve_no_marker_identities(self):
        preflight.validate_report(expected_report())
        self.assertTrue(preflight.exact_map_schema({
            "type": "object", "properties": {key: {"type": "string"} for key in
                ("api_version", "kind", "namespace", "namespace_prefix")},
            "required": ["api_version", "kind"], "additionalProperties": False}))
        invalid_schema = {"type": "object", "properties": {key: {"type": "string"} for key in
                          ("api_version", "kind", "namespace", "namespace_prefix", "context")},
                          "required": ["api_version", "kind"], "additionalProperties": False}
        self.assertFalse(preflight.exact_map_schema(invalid_schema))
        views_schema = {"type": "object", "properties": {
            **{key: {"type": "string"} for key in ("api_version", "kind", "namespace")},
            "namespace_prefix": {"type": "string", "minLength": 1},
            "owner": {"type": "string", "enum": list(preflight.VIEW_OWNERS)},
            "summary": {"type": "boolean"}},
            "required": ["api_version", "kind"], "additionalProperties": False}
        self.assertTrue(preflight.exact_map_schema(views_schema, preflight.MAP_CONTRACT_VIEWS))
        for mutation in ("extra-property", "summary-string", "owner-enum", "owner-required",
                         "allows-extra", "extra-keyword", "noncanonical-enum", "kind-enum",
                         "wrong-prefix-minimum", "missing-prefix-minimum", "summary-enum"):
            schema = copy.deepcopy(views_schema)
            if mutation == "extra-property":
                schema["properties"]["context"] = {"type": "string"}
            elif mutation == "summary-string":
                schema["properties"]["summary"]["type"] = "string"
            elif mutation == "owner-enum":
                schema["properties"]["owner"]["enum"].remove("Kubernetes")
            elif mutation == "owner-required":
                schema["required"].append("owner")
            elif mutation == "allows-extra":
                schema["additionalProperties"] = True
            elif mutation == "extra-keyword":
                schema["not"] = {"required": ["summary"]}
            elif mutation == "kind-enum":
                schema["properties"]["kind"]["enum"] = ["Deployment"]
            elif mutation == "wrong-prefix-minimum":
                schema["properties"]["namespace_prefix"]["minLength"] = 0
            elif mutation == "missing-prefix-minimum":
                del schema["properties"]["namespace_prefix"]["minLength"]
            elif mutation == "summary-enum":
                schema["properties"]["summary"]["enum"] = [True, False]
            else:
                schema["properties"]["owner"]["enum"][0] = "Unknown"
            with self.subTest(schema_mutation=mutation):
                self.assertFalse(preflight.exact_map_schema(schema, preflight.MAP_CONTRACT_VIEWS))
        for mutation in ("counts", "native-names", "scope", "row-owner", "duplicate", "capture-time"):
            report = expected_report()
            if mutation == "counts":
                report["ownerCounts"]["Native"] = 11
            elif mutation == "native-names":
                report["resources"].pop()
            elif mutation == "row-owner":
                report["resources"][-1]["owner"] = "ArgoCD"
            elif mutation == "duplicate":
                report["resources"][-1] = report["resources"][-2].copy()
            elif mutation == "capture-time":
                report["provenance"]["captureTime"] = "now"
            else:
                report["scope"]["namespacePrefix"] = "team"
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                preflight.validate_report(report)

    def test_views_reports_require_exact_full_summary_and_native_contract(self):
        reports = expected_views_reports()
        preflight.validate_views_reports(reports)
        for mutation in ("summary-schema", "summary-view", "summary-rows", "summary-guide",
                         "summary-counts", "native-owner", "native-name", "native-excluded",
                         "native-detector-detail", "provenance", "missing-variant"):
            changed = copy.deepcopy(reports)
            if mutation == "summary-schema":
                changed["summary"]["schema"] = "map-list-recorded.v1"
            elif mutation == "summary-view":
                changed["summary"]["view"] = "full"
            elif mutation == "summary-rows":
                changed["summary"]["resources"] = []
            elif mutation == "summary-guide":
                changed["summary"]["perObjectEvidenceGuide"] = ""
            elif mutation == "summary-counts":
                changed["summary"]["ownerCounts"]["Native"] = 11
            elif mutation == "native-owner":
                changed["native"]["resources"][0]["owner"] = "Flux"
            elif mutation == "native-name":
                changed["native"]["resources"][0]["name"] = "wrong"
            elif mutation == "native-excluded":
                changed["native"]["excludedFromScopeCount"] = 289
            elif mutation == "native-detector-detail":
                changed["native"]["resources"][0]["ownershipDetection"]["source"] = "different-detector"
            elif mutation == "provenance":
                changed["native-summary"]["provenance"]["sha256"] = "0" * 64
            else:
                del changed["native-summary"]
            with self.subTest(views_mutation=mutation), self.assertRaises(ValueError):
                preflight.validate_views_reports(changed)

    def test_views_contract_cli_reports_are_checked_for_all_four_views(self):
        reports = expected_views_reports()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = fake_views_cli(root / "fake-cub-scout", reports)
            kubeconfig = root / "kubeconfig-empty"
            recording = root / "recording.yaml"
            cli_reports = preflight.call_map_cli(binary, kubeconfig, recording,
                                                  preflight.MAP_CONTRACT_VIEWS)
            self.assertEqual(cli_reports, reports)
            self.assertEqual(set(cli_reports), set(preflight.VIEWS))

            changed = copy.deepcopy(reports)
            changed["native-summary"]["excludedFromScopeCount"] = 289
            bad_binary = fake_views_cli(root / "bad-cub-scout", changed)
            with self.assertRaisesRegex(ValueError, "selection counts mismatch"):
                preflight.call_map_cli(bad_binary, kubeconfig, recording,
                                       preflight.MAP_CONTRACT_VIEWS)

    def test_views_contract_mcp_checks_schema_parity_and_startup_snapshot(self):
        reports = expected_views_reports()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            kubeconfig = root / "kubeconfig-empty"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording.yaml"
            recording.write_bytes(b"immutable bytes")
            server = fake_views_mcp(root / "fake-views-mcp", reports)
            result = preflight.call_map_stdio(server, kubeconfig, recording,
                                               contract=preflight.MAP_CONTRACT_VIEWS)
            self.assertEqual(result["reports"], reports)
            self.assertEqual(recording.read_bytes(), b"immutable bytes")

            basic_schema = {"type": "object", "properties": {
                key: {"type": "string"} for key in
                ("api_version", "kind", "namespace", "namespace_prefix")},
                "required": ["api_version", "kind"], "additionalProperties": False}
            old_server = fake_views_mcp(root / "old-views-mcp", reports, basic_schema)
            with self.assertRaisesRegex(ValueError, "schema differs"):
                preflight.call_map_stdio(old_server, kubeconfig, recording,
                                         contract=preflight.MAP_CONTRACT_VIEWS)
            self.assertEqual(recording.read_bytes(), b"immutable bytes")

    def test_prepared_file_tampering_and_binary_hash_mismatch_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            plugin = root / "plugin"
            plugin.mkdir()
            marker = plugin / "marker"
            marker.write_text("pinned")
            binary = root / "binary"
            binary.write_text("binary")
            facts = {"binarySha256": preflight.sha256(binary),
                     "binary": str(binary),
                     "recordingManifestSha256": "",
                     "generatedPluginFiles": {"marker": preflight.sha256(marker)}}
            (root / "source-recording-manifest.json").write_text("manifest")
            facts["recordingManifestSha256"] = preflight.sha256(root / "source-recording-manifest.json")
            (root / "kubeconfig-empty").write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            (root / "prepared.json").write_text(json.dumps(facts))
            preflight.verify_prepared(root, binary, facts["binarySha256"])
            marker.write_text("changed")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                preflight.verify_prepared(root, binary, facts["binarySha256"])
            marker.write_text("pinned")
            with self.assertRaisesRegex(ValueError, "pinned hash"):
                preflight.verify_prepared(root, binary, "0" * 64)
            other = root / "other-binary"
            other.write_bytes(binary.read_bytes())
            with self.assertRaisesRegex(ValueError, "binary path differs"):
                preflight.verify_prepared(root, other, facts["binarySha256"])
            kubeconfig = root / "kubeconfig-empty"
            kubeconfig.write_text("apiVersion: v1\nkind: Config\nclusters: [{server: https://unexpected}]\n")
            with self.assertRaisesRegex(ValueError, "kubeconfig"):
                preflight.verify_prepared(root, binary, facts["binarySha256"])

    def test_configuration_keeps_full_plugin_and_limits_recorded_server_tooling(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "binary"
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "${HOME-unset}" "$KUBECONFIG" "$@"\n')
            binary.chmod(0o700)
            plugin = root / "plugin"
            plugin.mkdir()
            (plugin / ".claude-plugin").mkdir()
            (plugin / ".claude-plugin/plugin.json").write_text(json.dumps({
                "name": "cub-scout", "skills": ["retained"], "mcpServers": {"other": {"command": "x"}}}))
            record = root / "recording.yaml"
            record.write_bytes(b"recording")
            kubeconfig = root / "empty-kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            prepare.generate_wrapper(plugin, binary, prepare.sha256(binary), record, prepare.sha256(record), kubeconfig)
            manifest = json.loads((plugin / ".claude-plugin/plugin.json").read_text())
            self.assertEqual(manifest["name"], "cub-scout")
            self.assertEqual(manifest["skills"], ["retained"])
            self.assertEqual(list(manifest["mcpServers"]), ["cub-scout"])
            self.assertEqual(manifest["mcpServers"]["cub-scout"]["args"], [])
            wrapper = (plugin / "bin/recorded-cub-scout").read_text()
            self.assertIn("env -i", wrapper)
            self.assertIn(str(kubeconfig), wrapper)
            self.assertIn(prepare.sha256(record), wrapper)
            self.assertNotIn("HOME=", wrapper)
            self.assertIn("mcp serve --recording", wrapper)
            result = subprocess.run([str(plugin / "bin/recorded-cub-scout")], text=True,
                                    capture_output=True, timeout=5,
                                    env={"PATH": "/usr/bin:/bin", "HOME": "must-not-be-forwarded",
                                         "KUBECONFIG": "must-not-be-forwarded"})
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), ["unset", str(kubeconfig), "mcp", "serve",
                                                          "--recording", str(record)])
            record.write_bytes(b"changed after preparation")
            rejected = subprocess.run([str(plugin / "bin/recorded-cub-scout")], text=True,
                                      capture_output=True, timeout=5,
                                      env={"PATH": "/usr/bin:/bin", "KUBECONFIG": str(kubeconfig)})
            self.assertNotEqual(rejected.returncode, 0)
            self.assertEqual(rejected.stdout, "")

    def test_full_prepare_is_offline_and_keeps_manifest_outside_case_inputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "cub-scout"
            binary.write_text("#!/bin/sh\nexit 0\n")
            binary.chmod(0o700)
            out = root / "prepared"
            prepare.prepare(binary, prepare.sha256(binary), out)
            facts = json.loads((out / "prepared.json").read_text())
            self.assertFalse(facts["modelRun"])
            self.assertEqual(facts["mapInputContract"], prepare.MAP_CONTRACT_BASIC)
            self.assertEqual(facts["answerContract"], prepare.ANSWER_CONTRACT_LEGACY)
            self.assertEqual(facts["preflight"], "not run by preparation; invoke separately with explicit --binary")
            self.assertTrue(facts["allSevenFilesByteEqualAcrossArms"])
            self.assertEqual((out / "source-recording-manifest.json").read_bytes(), prepare.MANIFEST.read_bytes())
            a = out / "plugin/evals/recorded-scale/scale-ownership-counts/fixtures"
            b = out / "plugin/evals/recorded-scale/scale-unmanaged/fixtures"
            self.assertEqual({p.name: p.read_bytes() for p in a.iterdir()},
                             {p.name: p.read_bytes() for p in b.iterdir()})
            self.assertFalse((a / "recording-2026-09-30.json").exists())
            for case in prepare.CASES:
                graders = out / "plugin/evals/recorded-scale" / case / "graders"
                self.assertTrue(any(graders.glob("*.md")), "correctness grader retained")
                self.assertFalse(list(graders.glob("used-cub-scout-*.md")))
                answer_hashes = facts["answerContractFiles"][case]
                case_dir = out / "plugin/evals/recorded-scale" / case
                self.assertEqual(answer_hashes["promptSha256"], prepare.sha256(case_dir / "prompt.md"))
                self.assertEqual(answer_hashes["graderSha256"],
                                 prepare.sha256(out / "plugin" / answer_hashes["graderPath"]))
            preflight.verify_prepared(out, binary, prepare.sha256(binary))

            views_out = root / "prepared-views"
            cli = subprocess.run([sys.executable, str(prepare.PACKET / "prepare.py"),
                                  "--binary", str(binary), "--binary-sha256", prepare.sha256(binary),
                                  "--map-contract", prepare.MAP_CONTRACT_VIEWS,
                                  "--answer-contract", prepare.ANSWER_CONTRACT_STRICT,
                                  "--out", str(views_out)], capture_output=True, text=True,
                                 timeout=30, env={**os.environ, "PYTHONDONTWRITEBYTECODE": "1"})
            self.assertEqual(cli.returncode, 0, cli.stderr)
            self.assertIn("no model run", cli.stdout)
            views_facts = json.loads((views_out / "prepared.json").read_text())
            self.assertEqual(views_facts["mapInputContract"], prepare.MAP_CONTRACT_VIEWS)
            self.assertEqual(views_facts["answerContract"], prepare.ANSWER_CONTRACT_STRICT)
            for case, answer_hashes in views_facts["answerContractFiles"].items():
                case_dir = views_out / "plugin/evals/recorded-scale" / case
                self.assertEqual(answer_hashes["promptSha256"], prepare.sha256(case_dir / "prompt.md"))
                self.assertEqual(answer_hashes["graderSha256"],
                                 prepare.sha256(views_out / "plugin" / answer_hashes["graderPath"]))
            preflight.verify_prepared(views_out, binary, prepare.sha256(binary),
                                      prepare.MAP_CONTRACT_VIEWS)
            with self.assertRaisesRegex(ValueError, "differs from prepared.json"):
                preflight.verify_prepared(views_out, binary, prepare.sha256(binary),
                                          prepare.MAP_CONTRACT_BASIC)

    def test_mcp_rejects_unsupported_tool_and_live_args_and_holds_startup_snapshot(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            report = expected_report()
            fake = root / "fake-mcp"
            # This tiny stdio fixture parses the immutable report before serving requests.
            fake.write_text("#!" + sys.executable + "\n" + """
import json,sys
report = json.loads(%r)
for line in sys.stdin:
 m=json.loads(line); i=m.get('id'); method=m.get('method'); p=m.get('params',{})
 if i is None: continue
 if method == 'initialize': result={'protocolVersion':'2024-11-05'}
 elif method == 'tools/list': result={'tools':[{'name':'explain'},{'name':'map','inputSchema':{'type':'object','properties':{'api_version':{'type':'string'},'kind':{'type':'string'},'namespace':{'type':'string'},'namespace_prefix':{'type':'string'}},'required':['api_version','kind'],'additionalProperties':False}}]}
 elif method == 'tools/call' and p.get('name') not in ('map','explain'): result={'isError':True,'content':[]}
 elif method == 'tools/call' and p.get('name') == 'map' and 'kube_context' in p.get('arguments',{}): result={'isError':True,'content':[]}
 elif method == 'tools/call' and p.get('name') == 'map': result={'content':[{'type':'text','text':json.dumps(report)}]}
 else: result={}
 print(json.dumps({'jsonrpc':'2.0','id':i,'result':result}),flush=True)
""" % json.dumps(report))
            fake.chmod(0o700)
            wrapper = fake
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original bytes")
            result = preflight.call_map_stdio(wrapper, kubeconfig, recording)
            self.assertEqual(result["tools"], ["explain", "map"])
            self.assertTrue(result["unsupportedToolRejected"] and result["unsupportedLiveArgumentRejected"])
            self.assertEqual(recording.read_bytes(), b"original bytes")

    def test_timeout_drains_stderr_bounds_partial_line_and_kills_owned_descendant(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            script = ("#!" + sys.executable + "\nimport signal,subprocess,sys,time\n"
                      "signal.signal(signal.SIGTERM, signal.SIG_IGN)\n"
                      "subprocess.Popen([sys.executable,'-c'," + repr(child) + "])\n"
                      "sys.stderr.write('e'*200000); sys.stderr.flush()\n"
                      "sys.stdout.write('{\\\"jsonrpc\\\":'); sys.stdout.flush()\n"
                      "time.sleep(30)\n")
            server = root / "fake-server"
            server.write_text(script)
            server.chmod(0o700)
            wrapper = server
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original")
            started = __import__("time").monotonic()
            original_popen = preflight.subprocess.Popen

            def start_after_heartbeat(*args, **kwargs):
                process = original_popen(*args, **kwargs)
                ready_by = __import__("time").monotonic() + 2
                while not heartbeat.exists() and __import__("time").monotonic() < ready_by:
                    __import__("time").sleep(0.01)
                if not heartbeat.exists():
                    try:
                        preflight.stop_owned_group(process.pid, grace=0.05, process=process)
                        process.wait(timeout=1)
                    finally:
                        for pipe in (process.stdin, process.stdout, process.stderr):
                            if pipe is not None:
                                pipe.close()
                    raise AssertionError("owned descendant did not reach its heartbeat readiness signal")
                return process

            # Synchronize only the test fixture's child startup. This keeps the
            # production 0.3s timeout and drain/kill assertions unchanged.
            with mock.patch.object(preflight.subprocess, "Popen", side_effect=start_after_heartbeat):
                with self.assertRaises(TimeoutError):
                    preflight.call_map_stdio(wrapper, kubeconfig, recording, timeout=0.3)
            self.assertLess(__import__("time").monotonic() - started, 3)
            self.assertEqual(recording.read_bytes(), b"original")
            self.assertTrue(heartbeat.exists())
            value = heartbeat.read_bytes()
            __import__("time").sleep(0.12)
            self.assertEqual(heartbeat.read_bytes(), value)

    def test_run_bounded_timeout_cleans_inherited_pipe_descendants(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            parent = ("import signal,subprocess,sys,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); "
                      "subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); "
                      "sys.stderr.write('x'*200000); sys.stderr.flush(); time.sleep(30)")
            started = __import__("time").monotonic()
            with self.assertRaisesRegex(TimeoutError, "timed out"):
                preflight.run_bounded([sys.executable, "-c", parent], {"PATH": "/usr/bin:/bin"}, timeout=0.3)
            self.assertLess(__import__("time").monotonic() - started, 3)
            self.assertTrue(heartbeat.exists())
            value = heartbeat.read_bytes()
            __import__("time").sleep(0.12)
            self.assertEqual(heartbeat.read_bytes(), value)

    def test_exited_leader_does_not_leave_term_ignoring_child(self):
        with tempfile.TemporaryDirectory() as tmp:
            heartbeat = Path(tmp) / "heartbeat"
            child = ("import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); "
                     "p=pathlib.Path(" + repr(str(heartbeat)) + ");\n"
                     "while True: p.write_text(str(time.monotonic())); time.sleep(.02)")
            parent = ("import subprocess,sys,time,pathlib; "
                      "subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); "
                      "p=pathlib.Path(" + repr(str(heartbeat)) + ");\n"
                      "while not p.exists(): time.sleep(.01)\n")
            with self.assertRaises(TimeoutError):
                preflight.run_bounded([sys.executable, "-c", parent], {"PATH": "/usr/bin:/bin"}, timeout=.4)
            value = heartbeat.read_bytes()
            time.sleep(.12)
            self.assertEqual(heartbeat.read_bytes(), value, "owned descendant survived its leader")

    def test_cancellation_restores_private_recording_and_cleans_owned_group(self):
        import signal
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            heartbeat = root / "heartbeat"
            child = "import signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); p=pathlib.Path(" + repr(str(heartbeat)) + ");\nwhile True: p.write_text(str(time.monotonic())); time.sleep(.02)"
            server = root / "fake-server"
            server.write_text("#!" + sys.executable + "\nimport signal,subprocess,sys,time\n"
                              "signal.signal(signal.SIGTERM,signal.SIG_IGN)\n"
                              "subprocess.Popen([sys.executable,'-c'," + repr(child) + "])\n"
                              "sys.stdout.write('{\\\"jsonrpc\\\":'); sys.stdout.flush()\n"
                              "time.sleep(30)\n")
            server.chmod(0o700)
            wrapper = server
            kubeconfig = root / "kubeconfig"
            kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
            recording = root / "recording"
            recording.write_bytes(b"original")
            module = str(ROOT / "preflight.py")
            driver = ("import importlib.util,sys,pathlib; spec=importlib.util.spec_from_file_location('pf'," + repr(module) + "); "
                      "m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m); "
                      "m.call_map_stdio(pathlib.Path(" + repr(str(wrapper)) + "),pathlib.Path(" + repr(str(kubeconfig)) + "),pathlib.Path(" + repr(str(recording)) + "),timeout=20)")
            process = subprocess.Popen([sys.executable, "-c", driver], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                       env={"PATH": "/usr/bin:/bin"}, start_new_session=True)
            try:
                deadline = __import__("time").monotonic() + 3
                while not heartbeat.exists() and __import__("time").monotonic() < deadline:
                    __import__("time").sleep(0.02)
                self.assertTrue(heartbeat.exists())
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=3)
                self.assertNotEqual(process.returncode, 0)
                self.assertEqual(recording.read_bytes(), b"original")
                value = heartbeat.read_bytes()
                __import__("time").sleep(0.12)
                self.assertEqual(heartbeat.read_bytes(), value)
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=3)


if __name__ == "__main__":
    unittest.main()
