#!/usr/bin/env python3
"""Prepare (but never launch) a hash-pinned recorded scale plugin packet."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

REPO = Path(__file__).resolve().parents[2]
PACKET = Path(__file__).resolve().parent
SOURCE = REPO / "evals/fixtures/scale"
MANIFEST = SOURCE / "recording-2026-09-30.json"
PLUGIN_SOURCE = REPO / ".claude-plugin"
SKILLS_SOURCE = REPO / "skills"
SOURCE_COMMIT = "7f5d38e8881d2f6b22e132b3da57ca60f483e703"
MANIFEST_SHA256 = "e139701bf9d894ca9dd16ddb302fe0e4f28f3122922030cd9cdda57e8eb3fa22"
DEPLOYMENT_SHA256 = "822716e41eaf59674cec8b52913b2613c76932b578c45d54285f71747dd228b6"
FILES = json.loads(MANIFEST.read_text())["files"]
CASES = ("scale-ownership-counts", "scale-unmanaged")


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def verify_sources() -> None:
    if sha256(MANIFEST) != MANIFEST_SHA256:
        raise ValueError("historical scale manifest hash mismatch")
    for relative, expected in FILES.items():
        path = REPO / relative
        if sha256(path) != expected:
            raise ValueError(f"scale source hash mismatch: {relative}")
    if FILES.get("evals/fixtures/scale/cluster/deployments.yaml") != DEPLOYMENT_SHA256:
        raise ValueError("recorded Deployment digest does not match pinned source")
    subprocess.run(["git", "merge-base", "--is-ancestor", SOURCE_COMMIT, "HEAD"], cwd=REPO,
                   text=True, capture_output=True, check=True, timeout=5)


def write_scaffold(case_dir: Path) -> None:
    lines = ["#!/bin/sh", "set -eu", "mkdir -p cluster"]
    for relative in sorted(FILES):
        name = Path(relative).name
        lines.append(f"cp \"$(dirname \"$0\")/fixtures/{name}\" \"cluster/{name}\"")
    script = case_dir / "scaffold.sh"
    script.write_text("\n".join(lines) + "\n")
    script.chmod(0o755)


def stage_case(plugin: Path, name: str) -> dict:
    source_case = REPO / "evals/scale" / name
    target_case = plugin / "evals/recorded-scale" / name
    shutil.copytree(source_case, target_case, ignore=shutil.ignore_patterns("scaffold.sh"))
    # Keep answer/correctness graders, but do not let prior tool-use checks
    # turn a selected tool into a correctness outcome.
    for grader in (target_case / "graders").glob("used-cub-scout-*.md"):
        grader.unlink()
    fixtures = target_case / "fixtures"
    fixtures.mkdir()
    for relative in sorted(FILES):
        shutil.copyfile(REPO / relative, fixtures / Path(relative).name)
    write_scaffold(target_case)
    output = {}
    with tempfile.TemporaryDirectory(prefix="recorded-scale-stage-") as temp:
        arm_outputs = []
        for arm in ("with", "without"):
            work = Path(temp) / arm
            work.mkdir()
            subprocess.run(["bash", str(target_case / "scaffold.sh")], cwd=work,
                           check=True, timeout=10, env={"PATH": "/usr/bin:/bin"})
            files = {p.name: p.read_bytes() for p in sorted((work / "cluster").iterdir())}
            if set(files) != {Path(p).name for p in FILES}:
                raise ValueError("staged arm does not contain exactly seven source files")
            arm_outputs.append(files)
        if arm_outputs[0] != arm_outputs[1]:
            raise ValueError("two arms do not have byte-identical raw exports")
        for filename, data in arm_outputs[0].items():
            expected = FILES[f"evals/fixtures/scale/cluster/{filename}"]
            if sha256_bytes(data) != expected:
                raise ValueError(f"scaffold output hash mismatch: {filename}")
            output[filename] = {"sha256": expected, "bytes": len(data)}
    return output


def generate_wrapper(plugin: Path, binary: Path, binary_hash: str,
                     recording: Path, recording_hash: str, kubeconfig: Path) -> None:
    wrapper = plugin / "bin/recorded-cub-scout"
    wrapper.parent.mkdir(parents=True, exist_ok=True)
    # Values are JSON-escaped, then safely single-quoted for the shell.
    def sq(s: str) -> str:
        return "'" + s.replace("'", "'\"'\"'") + "'"
    wrapper.write_text("#!/bin/sh\nset -eu\n" +
        f"test \"$(/usr/bin/shasum -a 256 {sq(str(binary))} | /usr/bin/awk '{{print $1}}')\" = {sq(binary_hash)}\n" +
        f"test \"$(/usr/bin/shasum -a 256 {sq(str(recording))} | /usr/bin/awk '{{print $1}}')\" = {sq(recording_hash)}\n" +
        f"exec /usr/bin/env -i PATH=/usr/bin:/bin KUBECONFIG={sq(str(kubeconfig))} {sq(str(binary))} mcp serve --recording {sq(str(recording))}\n")
    wrapper.chmod(0o755)
    manifest_path = plugin / ".claude-plugin/plugin.json"
    manifest = json.loads(manifest_path.read_text())
    manifest["mcpServers"] = {"cub-scout": {"command": str(wrapper), "args": []}}
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")


def prepare(binary: Path, expected_hash: str, out: Path) -> None:
    binary = binary.expanduser().resolve(strict=True)
    out = out.expanduser().resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise ValueError("--binary must be an executable file")
    if not re.fullmatch(r"[0-9a-f]{64}", expected_hash) or sha256(binary) != expected_hash:
        raise ValueError("binary SHA-256 mismatch or invalid expected digest")
    if out.exists():
        raise FileExistsError(f"refusing to overwrite existing output: {out}")
    if out == REPO or REPO in out.parents:
        raise ValueError("--out must be outside the repository")
    verify_sources()
    out.mkdir(parents=True)
    shutil.copyfile(MANIFEST, out / "source-recording-manifest.json")
    plugin = out / "plugin"
    plugin.mkdir()
    shutil.copytree(PLUGIN_SOURCE, plugin / ".claude-plugin")
    shutil.copytree(SKILLS_SOURCE, plugin / "skills")
    (plugin / "evals/recorded-scale").mkdir(parents=True)
    # MCP gets only the supported Deployment recording; both model arms get all seven files.
    mcp_recording = plugin / "recordings/deployments.yaml"
    mcp_recording.parent.mkdir()
    shutil.copyfile(SOURCE / "cluster/deployments.yaml", mcp_recording)
    empty_kubeconfig = out / "kubeconfig-empty"
    empty_kubeconfig.write_text('apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: ""\n')
    generate_wrapper(plugin, binary, expected_hash, mcp_recording, DEPLOYMENT_SHA256, empty_kubeconfig)
    case_facts = {name: stage_case(plugin, name) for name in CASES}
    generated = {str(p.relative_to(plugin)): sha256(p) for p in sorted(plugin.rglob("*")) if p.is_file()}
    facts = {
        "schema": "recorded-scale-preparation.v1", "sourceCommit": SOURCE_COMMIT,
        "preparationCommit": subprocess.run(["git", "rev-parse", "HEAD"], cwd=REPO,
                                              text=True, capture_output=True, check=True, timeout=5).stdout.strip(),
        "binary": str(binary), "binarySha256": expected_hash,
        "recordingManifestSha256": MANIFEST_SHA256, "deploymentRecordingSha256": DEPLOYMENT_SHA256,
        "caseFixtureFacts": case_facts, "allSevenFilesByteEqualAcrossArms": True,
        "mcpToolsExpected": ["explain", "map"], "mcpScope": {"api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-"},
        "ordinaryToolPermissions": "copied unchanged from existing scale cases in both arms",
        "caseMode": "file-tools-only comparison; narrower than Experiment A kubectl/Helm baseline",
        "toolUseGraders": "removed; any actual tool use is a trace observation, not a correctness score",
        "modelRun": False, "preflight": "not run by preparation; invoke separately with explicit --binary",
        "generatedPluginFiles": generated,
    }
    (out / "prepared.json").write_text(json.dumps(facts, indent=2, sort_keys=True) + "\n")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--binary-sha256", required=True)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    try:
        prepare(args.binary, args.binary_sha256, args.out)
    except Exception as exc:
        parser.error(str(exc))
    print(f"Prepared recorded-scale packet at {args.out.resolve()}; no model run or preflight was launched.")


if __name__ == "__main__":
    main()
