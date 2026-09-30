#!/usr/bin/env python3
"""Build an isolated, hash-pinned recorded-only MCP plugin and preflight it."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

REPO = Path(__file__).resolve().parents[2]
PACKET = Path(__file__).resolve().parent
TEMPLATE = PACKET / "template/plugin"
SOURCE_FIXTURE = REPO / "evals/recorded-explain-contract/fixtures/deployments.yaml"
ECONOMY_PROMPT = PACKET / "economy/prompt.md"
FIXTURE_SHA256 = "305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8"


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def shell_quote(value: str) -> str:
    return "'" + value.replace("'", "'\"'\"'") + "'"


def configure_purpose(plugin: Path, purpose: str) -> None:
    """Apply purpose-specific prompt/scoring without changing plumbing default."""
    if purpose == "plumbing":
        return
    if purpose != "economy":
        raise ValueError(f"unsupported purpose: {purpose}")
    case_dir = plugin / "evals/recorded-explain-mcp"
    shutil.copyfile(ECONOMY_PROMPT, case_dir / "prompt.md")
    tool_grader = case_dir / "graders/tool-called.md"
    if tool_grader.exists():
        tool_grader.unlink()
    manifest_path = plugin / ".claude-plugin/plugin.json"
    manifest = json.loads(manifest_path.read_text())
    manifest["description"] = "Recorded-only evidence for a Kubernetes resource question."
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path, help="local cub-scout binary to pin")
    parser.add_argument("--binary-sha256", required=True, help="expected lowercase SHA-256 of --binary")
    parser.add_argument("--purpose", choices=("plumbing", "economy"), default="plumbing",
                        help="diagnostic objective; default plumbing preserves the original prompt and grader")
    parser.add_argument("--out", required=True, type=Path, help="new output directory; existing paths are refused")
    args = parser.parse_args()

    binary = args.binary.expanduser().resolve(strict=True)
    output = args.out.expanduser().resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        parser.error("--binary must be an executable file")
    if not re.fullmatch(r"[0-9a-f]{64}", args.binary_sha256):
        parser.error("--binary-sha256 must be 64 lowercase hexadecimal characters")
    actual_binary_sha = sha256(binary)
    if actual_binary_sha != args.binary_sha256:
        parser.error(f"binary hash mismatch: got {actual_binary_sha}")
    if output == REPO or REPO in output.parents:
        parser.error("--out must be outside the repository")
    if output.exists():
        parser.error(f"refusing to overwrite existing output: {output}")
    actual_fixture_sha = sha256(SOURCE_FIXTURE)
    if actual_fixture_sha != FIXTURE_SHA256:
        parser.error(f"recorded fixture source hash mismatch: got {actual_fixture_sha}")

    output.mkdir(parents=True)
    plugin = output / "plugin"
    server_home = output / "isolated-home"
    empty_kubeconfig = output / "kubeconfig-empty"
    server_home.mkdir()
    empty_kubeconfig.write_bytes(b"")
    plugin.mkdir()

    recording_path = plugin / "evals/recorded-explain-mcp/fixtures/deployments.yaml"
    substitutions = {
        "@BINARY_PATH@": shell_quote(str(binary)),
        "@BINARY_SHA256@": args.binary_sha256,
        "@RECORDING_PATH@": shell_quote(str(recording_path)),
        "@SERVER_HOME@": shell_quote(str(server_home)),
        "@EMPTY_KUBECONFIG@": shell_quote(str(empty_kubeconfig)),
    }
    for source in TEMPLATE.rglob("*"):
        if source.is_dir():
            continue
        relative = source.relative_to(TEMPLATE)
        target_relative = Path(str(relative)[:-3]) if relative.name.endswith(".in") else relative
        target = plugin / target_relative
        target.parent.mkdir(parents=True, exist_ok=True)
        text = source.read_text()
        if source.name == "server-wrapper.sh.in":
            for token, value in substitutions.items():
                text = text.replace(token, value)
        elif source.name == "plugin.json.in":
            wrapper_json = json.dumps(str(plugin / "server-wrapper.sh"))[1:-1]
            text = text.replace("@WRAPPER_PATH@", wrapper_json)
        target.write_text(text)
        if source.name in {"server-wrapper.sh.in", "scaffold.sh"}:
            target.chmod(0o755)

    configure_purpose(plugin, args.purpose)

    case_fixtures = plugin / "evals/recorded-explain-mcp/fixtures"
    case_fixtures.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(SOURCE_FIXTURE, case_fixtures / "deployments.yaml")
    recording = {
        "kind": "complete recorded Kubernetes Deployments List",
        "source": "evals/recorded-explain-contract/fixtures/deployments.yaml",
        "sha256": FIXTURE_SHA256,
        "bytes": SOURCE_FIXTURE.stat().st_size,
        "documents": 1,
        "objectCount": 14,
        "captureTimeTrusted": False,
        "purpose": "immutable historical input for a recorded MCP diagnostic; not current cluster state",
    }
    (case_fixtures / "recording.json").write_text(json.dumps(recording, indent=2) + "\n")

    # Run the authored scaffold independently for both arms before making a
    # plugin available to any model runner. Both byte outputs must be equal.
    scaffold = plugin / "evals/recorded-explain-mcp/scaffold.sh"
    with tempfile.TemporaryDirectory(prefix="recorded-mcp-arms-") as scratch:
        arm_outputs = []
        for arm in ("with", "without"):
            arm_dir = Path(scratch) / arm
            arm_dir.mkdir()
            subprocess.run(["bash", str(scaffold)], cwd=arm_dir, check=True,
                           timeout=10, env={"PATH": "/usr/bin:/bin"})
            arm_outputs.append((arm_dir / "cluster/deployments.yaml").read_bytes())
        if arm_outputs[0] != arm_outputs[1] or hashlib.sha256(arm_outputs[0]).hexdigest() != FIXTURE_SHA256:
            raise RuntimeError("the two arm scaffolds did not produce identical pinned fixture bytes")

    minimal_env = {"PATH": "/usr/bin:/bin"}
    node_bin = shutil.which("node")
    if not node_bin:
        raise RuntimeError("node is required for the JavaScript regex contract checks")
    grader_env = {**minimal_env, "NODE_BIN": node_bin}
    subprocess.run([sys.executable, str(plugin / "grader_contract.py")], check=True,
                   timeout=20, env=grader_env)
    subprocess.run([sys.executable, str(plugin / "preflight.py")], check=True,
                   timeout=40, env=minimal_env)

    generated = {}
    for path in sorted(plugin.rglob("*")):
        if path.is_file():
            generated[str(path.relative_to(plugin))] = sha256(path)
    case_dir = plugin / "evals/recorded-explain-mcp"
    provenance = {
        "purpose": args.purpose,
        "binary": str(binary),
        "binarySha256": args.binary_sha256,
        "recordedFixtureSha256": FIXTURE_SHA256,
        "armFixtureBytesEqual": True,
        "promptSha256": sha256(case_dir / "prompt.md"),
        "caseSha256": sha256(case_dir / "case.yaml"),
        "answerGraderSha256": sha256(case_dir / "graders/answer.md"),
        "toolUseGrader": "included for plumbing diagnostic only" if args.purpose == "plumbing" else "omitted; inspect raw trace as diagnostic only",
        "preflight": "passed: initialize, tools/list, exact call, unsupported-tool rejection",
        "modelRun": False,
        "generatedPluginFiles": generated,
    }
    (output / "prepared.json").write_text(json.dumps(provenance, indent=2, sort_keys=True) + "\n")
    print(f"Prepared plugin: {plugin}")
    print(f"Binary SHA-256: {args.binary_sha256}")
    print(f"Recording SHA-256: {FIXTURE_SHA256}")
    print("No model/eval run was started.")


if __name__ == "__main__":
    main()
