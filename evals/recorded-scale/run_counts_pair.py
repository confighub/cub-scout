#!/usr/bin/env python3
"""Run one explicitly authorized counts-economy diagnostic pair."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys

import prepare
import preflight

POLICY = prepare.COUNTS_ECONOMY_POLICY
CASE = "scale-ownership-counts"
MODEL = prepare.COUNTS_ECONOMY["model"]
PAIR_TIMEOUT = prepare.COUNTS_ECONOMY["pairTimeoutSeconds"]
CLEANUP_GRACE = prepare.COUNTS_ECONOMY["cleanupGraceSeconds"]
MAX_COST = prepare.COUNTS_ECONOMY["maxCostUsd"]
RUNNER_PATH = prepare.REPO / "evals/recorded-mcp-probe/run_pair.py"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_policy_packet(root: Path) -> dict:
    root = root.resolve(strict=True)
    prepare.verify_sources()
    prepare.verify_counts_source()
    facts = json.loads((root / "prepared.json").read_text())
    if facts.get("schema") != "recorded-scale-preparation.v1" or facts.get("modelRun") is not False:
        raise ValueError("requires an unrun recorded-scale preparation")
    policy = facts.get("policy")
    if not isinstance(policy, dict) or policy.get("schema") != POLICY:
        raise ValueError("requires explicit counts-economy.v1 preparation")
    if facts.get("mapInputContract") != prepare.MAP_CONTRACT_VIEWS:
        raise ValueError("counts-economy.v1 requires recorded map views")
    if facts.get("answerContract") != prepare.ANSWER_CONTRACT_STRICT:
        raise ValueError("counts-economy.v1 requires strict answer grading")
    expected_policy = {
        "schema": POLICY,
        "originalMaxTurns": 30,
        "originalTimeoutSeconds": 900,
        "maxTurns": 12,
        "timeoutSeconds": 180,
        "case": CASE,
        "model": MODEL,
        "runs": 1,
        "concurrency": 1,
        "pairTimeoutSeconds": PAIR_TIMEOUT,
        "cleanupGraceSeconds": CLEANUP_GRACE,
        "maxCostUsd": MAX_COST,
        "speed": "normal",
        "judge": "none",
        "retry": "none",
        "publish": False,
        "originalPromptSha256": prepare.COUNTS_PROMPT_SHA256,
        "diagnosticPromptSha256": policy.get("diagnosticPromptSha256"),
        "mockToolOverridesRemoved": True,
        "promptBodySha256": policy.get("promptBodySha256"),
    }
    if policy != expected_policy or not re.fullmatch(r"[0-9a-f]{64}", str(policy.get("promptBodySha256", ""))):
        raise ValueError("counts-economy.v1 policy fields were changed")
    if facts.get("sourceCommit") != prepare.SOURCE_COMMIT or facts.get("recordingManifestSha256") != prepare.MANIFEST_SHA256:
        raise ValueError("recorded source pins differ from reviewed constants")
    if facts.get("caseMode") != "file-tools-only comparison; narrower than Experiment A kubectl/Helm baseline":
        raise ValueError("unexpected comparison scope")
    if facts.get("ordinaryToolPermissions") != "copied unchanged from existing scale cases in both arms":
        raise ValueError("ordinary tool declaration differs from reviewed source")
    if facts.get("mcpToolsExpected") != ["explain", "map"]:
        raise ValueError("unexpected recorded MCP tool inventory")
    if facts.get("caseFixtureFacts", {}).get(CASE) is None:
        raise ValueError("counts case fixture pins are missing")
    if facts.get("caseFixtureFacts", {}).get(CASE) != {
            Path(source).name: {"sha256": digest, "bytes": (prepare.REPO / source).stat().st_size}
            for source, digest in prepare.FILES.items()}:
        raise ValueError("counts case fixture facts differ from reviewed source pins")
    manifest_path = root / "source-recording-manifest.json"
    if sha256(manifest_path) != prepare.MANIFEST_SHA256:
        raise ValueError("preserved recording manifest differs from reviewed source pin")

    binary = Path(facts["binary"]).expanduser().resolve(strict=True)
    if not binary.is_file() or not os.access(binary, os.X_OK) or sha256(binary) != facts.get("binarySha256"):
        raise ValueError("pinned cub-scout binary is missing, not executable, or changed")
    preflight.verify_prepared(root, binary, facts["binarySha256"], prepare.MAP_CONTRACT_VIEWS)
    for source, digest in prepare.FILES.items():
        staged = root / "plugin/evals/recorded-scale" / CASE / "fixtures" / Path(source).name
        if sha256(staged) != digest:
            raise ValueError(f"staged fixture differs from source pin: {staged.name}")

    case_dir = root / "plugin/evals/recorded-scale" / CASE
    if set(p.name for p in (root / "plugin/evals/recorded-scale").iterdir()) != {CASE, "scale-unmanaged"}:
        raise ValueError("packet case set changed")
    source_prompt = (prepare.REPO / "evals/scale" / CASE / "prompt.md").read_text()
    if sha256(prepare.REPO / "evals/scale" / CASE / "prompt.md") != prepare.COUNTS_PROMPT_SHA256:
        raise ValueError("current source counts prompt differs from reviewed source pin")
    if sha256(prepare.REPO / "evals/scale" / CASE / "case.yaml") != prepare.COUNTS_CASE_SHA256:
        raise ValueError("current source counts case differs from reviewed source pin")
    expected_prompt = source_prompt.rstrip() + "\n\n" + prepare.STRICT_PROMPT_SUFFIX + "\n"
    match = re.match(r"\A---\n(.*?)\n---\n(.*)\Z", expected_prompt, re.DOTALL)
    if not match:
        raise ValueError("source counts prompt frontmatter is malformed")
    frontmatter, body = match.groups()
    frontmatter = re.sub(r"(?m)^max_turns: 30$", "max_turns: 12", frontmatter)
    frontmatter = re.sub(r"(?m)^timeout_seconds: 900$", "timeout_seconds: 180", frontmatter)
    expected_prompt = f"---\n{frontmatter}\n---\n{body}"
    prompt_path = case_dir / "prompt.md"
    if prompt_path.read_text() != expected_prompt or sha256(prompt_path) != facts.get("answerContractFiles", {}).get(CASE, {}).get("promptSha256"):
        raise ValueError("diagnostic prompt or its recorded hash changed")
    if policy.get("diagnosticPromptSha256") != sha256(prompt_path):
        raise ValueError("diagnostic prompt hash differs from the reviewed policy")
    if (case_dir / "mocks/cub-scout").exists():
        raise ValueError("recorded MCP mock overrides must be absent so the real recorded server is used")
    if hashlib.sha256(body.encode()).hexdigest() != policy["promptBodySha256"]:
        raise ValueError("question body or strict-answer instruction changed")

    grader_path = case_dir / "graders/counts-line.md"
    grader_dir = case_dir / "graders"
    if {path.name for path in grader_dir.iterdir()} != {"counts-line.md"}:
        raise ValueError("counts case must contain exactly the strict answer grader")
    expected_grader = "---\ntype: regex\npattern: '" + prepare.STRICT_GRADERS[CASE][1] + "'\nflags: s\ntarget: last_message\n---\n"
    if grader_path.read_text() != expected_grader:
        raise ValueError("strict count answer grader changed")
    plugin_manifest = json.loads((root / "plugin/.claude-plugin/plugin.json").read_text())
    if (plugin_manifest.get("name") != "cub-scout" or
            list(plugin_manifest.get("mcpServers", {})) != ["cub-scout"] or
            set(plugin_manifest["mcpServers"]["cub-scout"]) != {"command", "args"} or
            plugin_manifest["mcpServers"]["cub-scout"].get("args") != [] or
            plugin_manifest["mcpServers"]["cub-scout"].get("command") != str(root / "plugin/bin/recorded-cub-scout")):
        raise ValueError("treatment plugin MCP server set changed")
    if facts.get("policy", {}).get("schema") != POLICY:
        raise ValueError("policy was removed")
    return facts


def build_command(root: Path) -> list[str]:
    """Return the one-case paid command without invoking it."""
    plugin = root / "plugin"
    return [
        "claude", "plugin", "eval", ".", "--eval-dir", "evals/recorded-scale",
        "--scaffold", "--case", CASE, "--runs", "1", "--concurrency", "1",
        "--ablation", "with-without", "--mocks", "record", "--allow-real-servers",
        "--allow-tools", "mcp__plugin_cub-scout_cub-scout__map",
        "mcp__plugin_cub-scout_cub-scout__explain",
        "--model", MODEL, "--max-cost-usd", str(MAX_COST), "--no-publish",
        "--trust-plugin", "--keep-temp", "--json", str(root / "result.json"),
    ]


def load_owned_runner():
    spec = importlib.util.spec_from_file_location("recorded_mcp_probe_runner", RUNNER_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError("could not load the reviewed owned-process helper")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def run(root: Path) -> int:
    facts = verify_policy_packet(root)
    existing = [name for name in ("launch.json", "result.json", "run.stdout", "run.stderr", "completion.json")
                if (root / name).exists()]
    if existing:
        raise ValueError("refusing existing run artifacts: " + ", ".join(existing))
    # --version is a read-only CLI query. No eval process is started before explicit opt-in.
    version = subprocess.run(["claude", "--version"], check=True, text=True,
                             capture_output=True, timeout=10).stdout.strip()
    command = build_command(root)
    runner = load_owned_runner()
    metadata = {
        "policy": POLICY,
        "case": CASE,
        "model": MODEL,
        "claudeCliVersion": version,
        "binarySha256": facts["binarySha256"],
        "sourceCommit": facts["sourceCommit"],
        "recordingManifestSha256": facts["recordingManifestSha256"],
        "pairTimeoutSeconds": PAIR_TIMEOUT,
        "costCeilingUsd": MAX_COST,
        "costCeilingSemantics": "prelaunch check; not a strict in-flight billing stop",
        "toolInventoryParity": "must be checked from both raw traces after execution",
    }
    return runner.run_owned(command, root, timeout=PAIR_TIMEOUT, grace=CLEANUP_GRACE,
                            launch_metadata=metadata)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("prepared_directory", type=Path)
    parser.add_argument("--execute", action="store_true",
                        help="explicitly start the paid, one-pair Claude evaluation")
    parser.add_argument("--print-command", action="store_true",
                        help="validate packet and print the exact command without launching it")
    args = parser.parse_args(argv)
    try:
        root = args.prepared_directory.expanduser().resolve(strict=True)
        verify_policy_packet(root)
        if args.print_command:
            print(json.dumps(build_command(root), indent=2))
            return 0
        if not args.execute:
            parser.error("paid run requires explicit --execute; use --print-command for a dry review")
        return run(root)
    except (OSError, ValueError, KeyError, subprocess.SubprocessError, RuntimeError) as exc:
        parser.error(str(exc))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
