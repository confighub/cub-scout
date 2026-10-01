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
MAP_CONTRACT_BASIC = "recorded-map-basic.v1"
MAP_CONTRACT_VIEWS = "recorded-map-views.v1"
MAP_CONTRACTS = (MAP_CONTRACT_BASIC, MAP_CONTRACT_VIEWS)
ANSWER_CONTRACT_LEGACY = "recorded-scale-answer-legacy.v1"
ANSWER_CONTRACT_STRICT = "recorded-scale-answer-line.v1"
ANSWER_CONTRACTS = (ANSWER_CONTRACT_LEGACY, ANSWER_CONTRACT_STRICT)
STRICT_PROMPT_SUFFIX = (
    "For this strict-answer variant, the entire final response must contain only the requested answer line. "
    "Do not include an explanation, heading, Markdown fence, or any other text. Surrounding whitespace is allowed."
)
UNMANAGED_NAMES = (
    "team-02/auth", "team-03/auth", "team-05/cron", "team-05/auth", "team-05/notify",
    "team-11/api", "team-11/notify", "team-12/web", "team-18/web", "team-19/cron",
    "team-25/search", "team-30/cache",
)


def strict_unmanaged_pattern() -> str:
    alternatives = "(?:" + "|".join(re.escape(name) for name in UNMANAGED_NAMES) + ")"
    present = "".join(
        "(?=[^\\r\\n]*\\b" + re.escape(name) + r"(?:[ \t]*,|\s*$))"
        for name in UNMANAGED_NAMES
    )
    return (r"^\s*UNMANAGED:[ \t]*" + present + alternatives +
            r"(?:[ \t]*,[ \t]*" + alternatives + r"){11}[ \t]*\s*$")


STRICT_GRADERS = {
    "scale-ownership-counts": ("counts-line.md", r"^\s*COUNTS:[ \t]*flux=120[ \t]+argocd=90[ \t]+helm=45[ \t]+confighub=33[ \t]+unmanaged=12[ \t]*\s*$"),
    "scale-unmanaged": ("unmanaged-line.md", strict_unmanaged_pattern()),
}


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
    # Content hashes bind the source even in a shallow checkout; no network fetch.


def write_scaffold(case_dir: Path) -> None:
    lines = ["#!/bin/sh", "set -eu", "mkdir -p cluster"]
    for relative in sorted(FILES):
        name = Path(relative).name
        lines.append(f"cp \"$(dirname \"$0\")/fixtures/{name}\" \"cluster/{name}\"")
    script = case_dir / "scaffold.sh"
    script.write_text("\n".join(lines) + "\n")
    script.chmod(0o755)


def stage_case(plugin: Path, name: str,
               answer_contract: str = ANSWER_CONTRACT_LEGACY) -> dict:
    if answer_contract not in ANSWER_CONTRACTS:
        raise ValueError(f"unsupported answer contract: {answer_contract}")
    source_case = REPO / "evals/scale" / name
    target_case = plugin / "evals/recorded-scale" / name
    shutil.copytree(source_case, target_case, ignore=shutil.ignore_patterns("scaffold.sh"))
    if answer_contract == ANSWER_CONTRACT_STRICT:
        prompt_path = target_case / "prompt.md"
        prompt_path.write_text(prompt_path.read_text().rstrip() + "\n\n" + STRICT_PROMPT_SUFFIX + "\n")
        grader_name, pattern = STRICT_GRADERS[name]
        grader_path = target_case / "graders" / grader_name
        # The explicit JS dotAll flag is inert here (the patterns use no dot
        # operator); omitting multiline/case-insensitive flags keeps anchors exact.
        grader_path.write_text(
            "---\ntype: regex\npattern: '" + pattern + "'\nflags: s\ntarget: last_message\n---\n"
        )
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


def answer_contract_facts(plugin: Path) -> dict:
    facts = {}
    for case, (grader_name, _) in STRICT_GRADERS.items():
        case_dir = plugin / "evals/recorded-scale" / case
        prompt = case_dir / "prompt.md"
        grader = case_dir / "graders" / grader_name
        facts[case] = {
            "promptSha256": sha256(prompt),
            "graderPath": str(grader.relative_to(plugin)),
            "graderSha256": sha256(grader),
        }
    return facts


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


def prepare(binary: Path, expected_hash: str, out: Path,
            map_contract: str = MAP_CONTRACT_BASIC,
            answer_contract: str = ANSWER_CONTRACT_LEGACY) -> None:
    if map_contract not in MAP_CONTRACTS:
        raise ValueError(f"unsupported map input contract: {map_contract}")
    if answer_contract not in ANSWER_CONTRACTS:
        raise ValueError(f"unsupported answer contract: {answer_contract}")
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
    case_facts = {name: stage_case(plugin, name, answer_contract) for name in CASES}
    generated = {str(p.relative_to(plugin)): sha256(p) for p in sorted(plugin.rglob("*")) if p.is_file()}
    facts = {
        "schema": "recorded-scale-preparation.v1", "sourceCommit": SOURCE_COMMIT,
        "sourceVerification": "pinned manifest and all fixture content hashes; ancestry not asserted",
        "preparationCommit": subprocess.run(["git", "rev-parse", "HEAD"], cwd=REPO,
                                              text=True, capture_output=True, check=True, timeout=5).stdout.strip(),
        "binary": str(binary), "binarySha256": expected_hash,
        "recordingManifestSha256": MANIFEST_SHA256, "deploymentRecordingSha256": DEPLOYMENT_SHA256,
        "caseFixtureFacts": case_facts, "allSevenFilesByteEqualAcrossArms": True,
        "mcpToolsExpected": ["explain", "map"], "mcpScope": {"api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-"},
        "mapInputContract": map_contract,
        "answerContract": answer_contract,
        "answerContractFiles": answer_contract_facts(plugin),
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
    parser.add_argument("--map-contract", choices=MAP_CONTRACTS, default=MAP_CONTRACT_BASIC,
                        help="Reviewed recorded map input contract to require during preflight")
    parser.add_argument("--answer-contract", choices=ANSWER_CONTRACTS, default=ANSWER_CONTRACT_LEGACY,
                        help="Answer-line contract to stage; strict mode changes generated prompt/grader copies only")
    args = parser.parse_args()
    try:
        prepare(args.binary, args.binary_sha256, args.out, args.map_contract, args.answer_contract)
    except Exception as exc:
        parser.error(str(exc))
    print(f"Prepared recorded-scale packet at {args.out.resolve()}; no model run or preflight was launched.")


if __name__ == "__main__":
    main()
