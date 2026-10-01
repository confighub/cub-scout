#!/usr/bin/env python3
"""Prepare the opt-in strict answer variants for five legacy eval cases.

Preparation is offline and source-only. It never edits the historical cases,
launches an evaluation, or changes the default legacy grading path.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile

REPO = Path(__file__).resolve().parents[2]
CONTRACT = "legacy-answer-line.v1"
SOURCE_COMMIT = "660e8303f82652e1553dfdb20f65741c9a426ee5"
MANIFEST_PATH = "evals/benchmark-v1.json"
SEMANTICS_PROJECTION_SHA256 = "b30f26be745cbd576f66a11890eff66e5082490381784182aeb1a8b24c366f74"
CASES = {
    "INV-03": {
        "directory": "owner-confighub",
        "answer_grader": "owner-line.md",
        "prompt_suffix": (
            "For this prospective strict variant, output exactly one plain-text line and no other text. "
            "Use this schema: OWNER: <owner> | EVIDENCE: <exact label key=value on namespace/name> | "
            "SCOPE: recorded evidence only. Base the answer on the saved export; do not infer ownership "
            "from a resource name or shape."
        ),
        "answer": "OWNER: ConfigHub | EVIDENCE: confighub.com/UnitSlug=inventory on inventory/inventory | SCOPE: recorded evidence only",
    },
    "ATR-01": {
        "directory": "changed-by-checkout",
        "answer_grader": "changed-by-line.md",
        "prompt_suffix": (
            "For this prospective strict variant, output exactly one plain-text line and no other text. "
            "Use this schema: CHANGED_BY: <MANUAL_TOOL|CONTROLLER|UNKNOWN> | MANAGER: <exact field manager|UNKNOWN> | "
            "FIELD_PATH: <exact field path|UNKNOWN> | HUMAN_ACTOR: <identified person|UNKNOWN> | "
            "COMMAND: <literal argv|UNKNOWN> | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided. "
            "A manager name does not establish a person or argv."
        ),
        "answer": "CHANGED_BY: MANUAL_TOOL | MANAGER: kubectl-set | FIELD_PATH: spec.template.spec.containers[name=checkout].image | HUMAN_ACTOR: UNKNOWN | COMMAND: UNKNOWN | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided",
        "strip_structured_block": True,
    },
    "ATR-02": {
        "directory": "changed-by-cart",
        "answer_grader": "changed-by-line.md",
        "prompt_suffix": (
            "For this prospective strict variant, output exactly one plain-text line and no other text. "
            "Use this schema: CHANGED_BY: <MANUAL_TOOL|CONTROLLER|UNKNOWN> | MANAGER: <exact field manager|UNKNOWN> | "
            "FIELD_PATH: <exact field path|UNKNOWN> | SUBRESOURCE: <recorded subresource|none|UNKNOWN> | "
            "HUMAN_ACTOR: <identified person|UNKNOWN> | COMMAND: <literal argv|UNKNOWN> | SCOPE: recorded evidence only. "
            "A manager name does not establish a person or argv."
        ),
        "answer": "CHANGED_BY: MANUAL_TOOL | MANAGER: kubectl | FIELD_PATH: spec.replicas | SUBRESOURCE: scale | HUMAN_ACTOR: UNKNOWN | COMMAND: UNKNOWN | SCOPE: recorded evidence only",
    },
    "ATR-03": {
        "directory": "changed-by-payments",
        "answer_grader": "changed-by-line.md",
        "prompt_suffix": (
            "For this prospective strict variant, output exactly one plain-text line and no other text. "
            "Use this schema: NON_STATUS_MANAGER: <exact managedFields manager|UNKNOWN> | "
            "MANUAL_CHANGE: <EVIDENCED|NOT_EVIDENCED|UNKNOWN> | HUMAN_ACTOR: <identified person|UNKNOWN> | "
            "COMMAND: <literal argv|UNKNOWN> | SCOPE: recorded evidence only. "
            "Use NOT_EVIDENCED only when no manual non-status manager is recorded; do not infer a person or command from a manager name."
        ),
        "answer": "NON_STATUS_MANAGER: helm | MANUAL_CHANGE: NOT_EVIDENCED | HUMAN_ACTOR: UNKNOWN | COMMAND: UNKNOWN | SCOPE: recorded evidence only",
    },
    "ATR-04": {
        "directory": "argo-label-vs-tracking-id",
        "answer_grader": "application-line.md",
        "prompt_suffix": (
            "For this prospective strict variant, output exactly one plain-text line and no other text. "
            "Use this schema: APPLICATION: <name|UNKNOWN> | TRACKING_MODE: <annotation|label|UNKNOWN> | "
            "TRACKING_ID_APPLICATION: <name|UNKNOWN> | INSTANCE_LABEL_APPLICATION: <name|UNKNOWN> | "
            "TRACKING_SOURCE: <TRACKING_ID|INSTANCE_LABEL|UNKNOWN> | SCOPE: recorded evidence only. "
            "Use the configured Argo tracking mode to select between the recorded identifiers; do not treat the fields as interchangeable."
        ),
        "answer": "APPLICATION: payments | TRACKING_MODE: annotation | TRACKING_ID_APPLICATION: payments | INSTANCE_LABEL_APPLICATION: storefront | TRACKING_SOURCE: TRACKING_ID | SCOPE: recorded evidence only",
    },
}

LEGACY_OUTPUT_TAILS = {
    "owner-confighub": " Finish with one line `OWNER: <Flux|ArgoCD|Helm|ConfigHub|none>`.",
    "changed-by-cart": " Finish with one line `CHANGED_BY: <Argo CD if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.",
    "changed-by-payments": " Finish with one line `CHANGED_BY: <Helm if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.",
    "argo-label-vs-tracking-id": " Finish with one line `APPLICATION: <name, or UNKNOWN>`.",
}

# Each source case is content-pinned, including its historical grader and exact
# scaffold bytes. A generated variant changes only prompt.md and the answer
# grader; the source inputs themselves remain untouched.
SOURCE_PINS = {
    "owner-confighub": {
        "case.yaml": "cea6ee996519875c36333c2ef6c2bae4fca9ff0389e0101479d9ad0b52d64bd5",
        "graders/owner-line.md": "a8253d5d1af4ec0bdb4185c3930ded04e040f3a6dc3fbe8aee460e988d76af36",
        "graders/skill-fired.md": "071db4c9e0ef9fe898b6a3848cc2837e9d452f439e03310128375eaf2a91d806",
        "graders/used-cub-scout-mcp.md": "867aac97f5ee2c24edf4ae3b7788e2c718c3eff7c943a7bf4c93e72002312727",
        "prompt.md": "99a568267d295fc80e1046dffc4348fbe32f03b16fd10797d6a155d878eee63b",
        "scaffold.sh": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    },
    "changed-by-checkout": {
        "case.yaml": "b4c6c030fed98ac8258ad1503ad846c436b28696bef5b31a33f7ef203db8266f",
        "graders/changed-by-line.md": "e13344a1ca00815f340775602e8553001b5e146334f112442a364931d4ae5a28",
        "graders/no-ingest-skill.md": "205aea339a0a66548f45298221508a7e0d4ba3ced11759c69474a3e4b8740974",
        "graders/skill-fired.md": "2ef2008c684edc973dfb7a0e2e9b827e7540bf627e35a922417f24ecede8f650",
        "graders/used-cub-scout-mcp.md": "867aac97f5ee2c24edf4ae3b7788e2c718c3eff7c943a7bf4c93e72002312727",
        "prompt.md": "7791cf167a8485b44dea1c6c5fadd8008650ea083f774a55fb281b9da2fdd66b",
        "scaffold.sh": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    },
    "changed-by-cart": {
        "case.yaml": "642ae3346527032b5f6bd1d29509aff9b1932ae360a2d9d11ac620c2816935fa",
        "graders/changed-by-line.md": "37c714aae3373e69b81f545a58789c0d5ade0e660ef380e2a44e1361a24134bd",
        "graders/no-ingest-skill.md": "205aea339a0a66548f45298221508a7e0d4ba3ced11759c69474a3e4b8740974",
        "graders/skill-fired.md": "2ef2008c684edc973dfb7a0e2e9b827e7540bf627e35a922417f24ecede8f650",
        "graders/used-cub-scout-mcp.md": "867aac97f5ee2c24edf4ae3b7788e2c718c3eff7c943a7bf4c93e72002312727",
        "prompt.md": "f1fc8d4575b3c2434a7d1a35c7661094b6ec9bf9f98db007a561f907f2bbcf75",
        "scaffold.sh": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    },
    "changed-by-payments": {
        "case.yaml": "09a4b64e235f77fe83337b0aa777238a8cf5a98d622c256a773cc0bfffa09ae2",
        "graders/changed-by-line.md": "1fe23cf8b3b2c131b123fdad737137b249ba0346c143a09202865eeae8a77cd6",
        "graders/no-ingest-skill.md": "205aea339a0a66548f45298221508a7e0d4ba3ced11759c69474a3e4b8740974",
        "graders/skill-fired.md": "2ef2008c684edc973dfb7a0e2e9b827e7540bf627e35a922417f24ecede8f650",
        "graders/used-cub-scout-mcp.md": "867aac97f5ee2c24edf4ae3b7788e2c718c3eff7c943a7bf4c93e72002312727",
        "prompt.md": "c5472cacb4d15f0da9d82f6e52baf947400c088525c3e92ea98498398bd938c8",
        "scaffold.sh": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    },
    "argo-label-vs-tracking-id": {
        "case.yaml": "5dfa5293e2cf9ae3f43776f56927ffbb8b9488d060acfb217d814dbaf244374c",
        "graders/application-line.md": "805d7990e7f1e56b293e96e094f3248b3230d52d8de9809442351e44d9988365",
        "graders/skill-fired.md": "071db4c9e0ef9fe898b6a3848cc2837e9d452f439e03310128375eaf2a91d806",
        "graders/used-cub-scout-mcp.md": "d1c77e189e4f17ada0090a99faf58cc2e4fab2df44a555a64a69da96b1eed80e",
        "prompt.md": "1b794575613c11219e82381a6e7c0169cff3a0f628f917cd085bd9d407b4efbe",
        "scaffold.sh": "c27a8e21114d46fb210dbda7b6a4bed1fd0e374c9cb4b69df5a2e9276a25bc65",
    },
}

CONTROL_PINS = {
    "owner-unlabelled": {
        "prompt.md": "9bdf184b0ee3e3c1cbcc072832d7265e101675c207e76c84c01a176f10f82d88",
        "graders/owner-line.md": "6d359353358f8ef8e1588cd591e716f1758e574e7242c680db294639344b399d",
        "graders/no-false-owner.md": "ca9d85ed21066b1712819c994303a27be94fb573632fa4b5d9dc6528cb23a65d",
    }
}


class PacketError(ValueError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_pattern(answer: str) -> str:
    escaped = re.sub(r"([\\.^$*+?{}\[\]|()])", r"\\\1", answer).replace(" ", "[ ]")
    # Permit at most one conventional final line terminator; no other character
    # (including contradiction prose) may exist anywhere in last_message.
    return "^(?:" + escaped + r")(?:\r?\n)?(?![\s\S])"


def grader_bytes(answer: str) -> bytes:
    pattern = canonical_pattern(answer)
    return ("---\ntype: regex\npattern: '" + pattern + "'\nflags: s\ntarget: last_message\n---\n").encode()


def strict_prompt(source: bytes, case_id: str, suffix: str) -> bytes:
    try:
        text = source.decode("utf-8", "strict")
    except UnicodeDecodeError:
        raise PacketError("source prompt is not UTF-8") from None
    match = re.fullmatch(r"(---\n.*?\n---\n)(.*)", text, re.DOTALL)
    if not match:
        raise PacketError(f"{case_id} source prompt frontmatter is malformed")
    frontmatter, body = match.groups()
    if CASES[case_id].get("strip_structured_block"):
        marker = "\n\nThis case measures a narrow answer contract, not freeform prose safety."
        index = body.find(marker)
        if index < 0:
            raise PacketError(f"{case_id} legacy structured answer block changed")
        body = body[:index].rstrip()
    else:
        tail = LEGACY_OUTPUT_TAILS.get(CASES[case_id]["directory"])
        body_without_trailing_ws = body.rstrip()
        if not tail or not body_without_trailing_ws.endswith(tail):
            raise PacketError(f"{case_id} legacy output instruction changed")
        body = body_without_trailing_ws[:-len(tail)].rstrip()
    return (frontmatter + body + "\n\n" + suffix + "\n").encode("utf-8")


def _file_hashes(root: Path) -> dict[str, str]:
    paths = sorted(root.rglob("*"))
    if any(path.is_symlink() for path in paths):
        raise PacketError("source or staged case tree contains a symlink")
    return {p.relative_to(root).as_posix(): sha256(p.read_bytes())
            for p in paths if p.is_file()}


def _semantics_projection(manifest_value: dict) -> dict:
    group_by_case = {case["id"]: group["id"]
                     for group in manifest_value["groups"] for case in group["cases"]}
    case_index = {case["id"]: case for group in manifest_value["groups"] for case in group["cases"]}
    selected = []
    for case_id in CASES:
        case = case_index.get(case_id)
        if case is None:
            raise PacketError(f"frozen case {case_id} is missing from benchmark manifest")
        selected.append({key: case.get(key) for key in
                         ("id", "existing_case", "question", "reference", "controls")}
                        | {"group": group_by_case[case_id]})
    return {
        "schema": "legacy-case-semantics.v1",
        "groupWeights": [{"id": group["id"], "weight": group["weight"]}
                         for group in manifest_value["groups"]],
        "cases": selected,
    }


def verify_source() -> dict:
    manifest = REPO / MANIFEST_PATH
    if not manifest.is_file():
        raise PacketError("frozen benchmark manifest is missing")
    manifest_bytes = manifest.read_bytes()
    try:
        manifest_value = json.loads(manifest_bytes)
        projection = _semantics_projection(manifest_value)
    except (json.JSONDecodeError, KeyError, TypeError):
        raise PacketError("frozen benchmark manifest is malformed") from None
    projection_bytes = json.dumps(projection, sort_keys=True, separators=(",", ":"),
                                  ensure_ascii=False).encode()
    semantics_hash = sha256(projection_bytes)
    if semantics_hash != SEMANTICS_PROJECTION_SHA256:
        raise PacketError("frozen selected case semantics or group weights changed")
    case_index = {case["id"]: case for group in manifest_value["groups"] for case in group["cases"]}
    if set(CASES) - set(case_index):
        raise PacketError("one or more frozen case references are missing")
    source_facts = {}
    for case_id, spec in CASES.items():
        directory = spec["directory"]
        entry = case_index[case_id]
        if entry.get("existing_case") != f"evals/{directory}":
            raise PacketError(f"{case_id} frozen case binding changed")
        expected = SOURCE_PINS[directory]
        root = REPO / "evals" / directory
        if root.is_symlink() or not root.is_dir():
            raise PacketError(f"{case_id} source case is missing or unsafe")
        actual = _file_hashes(root)
        if actual != expected:
            raise PacketError(f"{case_id} source file set or hash changed")
        if spec["answer_grader"] not in {Path(name).name for name in expected if name.startswith("graders/") }:
            raise PacketError(f"{case_id} answer grader binding is invalid")
        source_facts[case_id] = {
            "caseDirectory": f"evals/{directory}",
            "sourceFiles": actual,
            "frozenQuestion": entry["question"],
            "frozenReference": entry["reference"],
            "group": next(group["id"] for group in manifest_value["groups"] if entry in group["cases"]),
        }
    # The control remains a separate frozen source case, not added to this
    # generated strict five-case set.
    control_root = REPO / "evals/owner-unlabelled"
    if not control_root.is_dir():
        raise PacketError("owner-unlabelled negative control is unavailable")
    for relative, expected in CONTROL_PINS["owner-unlabelled"].items():
        path = control_root / relative
        if not path.is_file() or sha256(path.read_bytes()) != expected:
            raise PacketError("owner-unlabelled negative-control source changed")
    return {"manifestSha256": sha256(manifest_bytes),
            "semanticsProjectionSha256": semantics_hash, "cases": source_facts,
            "negativeControls": ["evals/owner-unlabelled", "ATR-03 changed-by-payments"]}


def _expected_tree(case_id: str, source_root: Path) -> dict[str, bytes]:
    spec = CASES[case_id]
    files = {relative: (source_root / relative).read_bytes()
             for relative in SOURCE_PINS[spec["directory"]]}
    files["prompt.md"] = strict_prompt(files["prompt.md"], case_id, spec["prompt_suffix"])
    files.pop("graders/" + spec["answer_grader"])
    files["graders/verified-answer.md"] = grader_bytes(spec["answer"])
    return files


def _write_tree(directory: Path, files: dict[str, bytes]) -> None:
    directory.mkdir(parents=True)
    for relative, payload in files.items():
        target = directory / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(payload)


def prepare(out: Path, contract: str | None = None) -> dict:
    if contract != CONTRACT:
        raise PacketError(f"strict variant requires explicit --contract {CONTRACT}; legacy remains the default")
    source_facts = verify_source()
    requested = out.expanduser().absolute()
    if requested.exists() or requested.is_symlink() or ".." in out.parts:
        raise PacketError("output must be a fresh path")
    try:
        parent = requested.parent.resolve(strict=True)
        home = Path.home().resolve(strict=True)
    except OSError:
        raise PacketError("output parent is unavailable") from None
    if requested == REPO or REPO in requested.parents or requested == home or home in requested.parents:
        raise PacketError("output must be outside the source checkout and home directory")
    if not any(parent == Path(root).resolve() or Path(root).resolve() in parent.parents
               for root in (tempfile.gettempdir(), "/tmp", "/var/tmp")):
        raise PacketError("output must be under a temporary directory")
    requested.mkdir(mode=0o700)
    os.chmod(requested, 0o700)
    staged = {}
    try:
        for arm in ("without", "with"):
            for case_id, spec in CASES.items():
                source_root = REPO / "evals" / spec["directory"]
                files = _expected_tree(case_id, source_root)
                target = requested / "arms" / arm / "evals/legacy-strict" / spec["directory"]
                target.parent.mkdir(parents=True, exist_ok=True)
                _write_tree(target, files)
                staged[f"{arm}/{case_id}"] = {name: sha256(payload) for name, payload in sorted(files.items())}
        facts = {
            "schema": "legacy-strict-answer-preparation.v1",
            "contract": CONTRACT,
            "sourceCommit": SOURCE_COMMIT,
            "manifestSha256": source_facts["manifestSha256"],
            "semanticsProjectionSha256": source_facts["semanticsProjectionSha256"],
            "caseIds": list(CASES),
            "cases": source_facts["cases"],
            "negativeControls": source_facts["negativeControls"],
            "arms": {arm: {case_id: staged[f"{arm}/{case_id}"] for case_id in CASES}
                     for arm in ("without", "with")},
            "armInputsByteEqual": True,
            "questionReferenceGroupWeightsChanged": False,
            "historicalSourcesChanged": False,
            "modelRun": False,
            "graderSchema": "type=regex, pattern, flags=s, target=last_message; no frontmatter weight invented",
            "correctnessWeightSemantics": "one non-tool regex grader; report.py uses positive default weight 1 when omitted; future embedded result must verify exact single passing result",
        }
        (requested / "prepared.json").write_text(json.dumps(facts, indent=2, sort_keys=True) + "\n")
        return facts
    except Exception:
        shutil.rmtree(requested, ignore_errors=True)
        raise


def verify_prepared(root: Path, contract: str | None = None) -> dict:
    if contract != CONTRACT:
        raise PacketError(f"verification requires explicit --contract {CONTRACT}")
    root = root.expanduser().resolve(strict=True)
    source_facts = verify_source()
    metadata_path = root / "prepared.json"
    if metadata_path.is_symlink() or not metadata_path.is_file():
        raise PacketError("prepared metadata is missing or unsafe")
    try:
        facts = json.loads(metadata_path.read_text())
    except (OSError, json.JSONDecodeError):
        raise PacketError("prepared metadata is malformed") from None
    if (facts.get("schema") != "legacy-strict-answer-preparation.v1" or facts.get("contract") != CONTRACT
            or facts.get("manifestSha256") != source_facts["manifestSha256"]
            or facts.get("semanticsProjectionSha256") != source_facts["semanticsProjectionSha256"]
            or facts.get("caseIds") != list(CASES) or facts.get("modelRun") is not False):
        raise PacketError("prepared packet identity or case set changed")
    expected_top = {"prepared.json"}
    arm_hashes = {}
    for arm in ("without", "with"):
        arm_hashes[arm] = {}
        for case_id, spec in CASES.items():
            relative_root = Path("arms") / arm / "evals/legacy-strict" / spec["directory"]
            staged_root = root / relative_root
            expected_files = _expected_tree(case_id, REPO / "evals" / spec["directory"])
            actual = _file_hashes(staged_root) if staged_root.is_dir() and not staged_root.is_symlink() else {}
            expected_hashes = {name: sha256(payload) for name, payload in sorted(expected_files.items())}
            if actual != expected_hashes:
                raise PacketError(f"staged {arm} files differ for {case_id}")
            if facts.get("arms", {}).get(arm, {}).get(case_id) != expected_hashes:
                raise PacketError(f"staged hash record differs for {arm} {case_id}")
            arm_hashes[arm][case_id] = actual
            expected_top.update(f"{relative_root}/{name}" for name in expected_files)
    actual_paths = {p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file() or p.is_symlink()}
    if actual_paths != expected_top:
        raise PacketError("prepared packet has missing, extra, or unsafe files")
    if arm_hashes["without"] != arm_hashes["with"] or facts.get("armInputsByteEqual") is not True:
        raise PacketError("raw case inputs differ between baseline and treatment staging")
    if (facts.get("sourceCommit") != SOURCE_COMMIT or facts.get("cases") != source_facts["cases"]
            or facts.get("negativeControls") != source_facts["negativeControls"]
            or facts.get("questionReferenceGroupWeightsChanged") is not False
            or facts.get("historicalSourcesChanged") is not False):
        raise PacketError("source provenance or preservation claims differ")
    return facts


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--contract", choices=(CONTRACT,), help="must be explicit; default evals remain legacy")
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--verify", action="store_true", help="verify an existing prepared directory")
    args = parser.parse_args(argv)
    try:
        facts = verify_prepared(args.out, args.contract) if args.verify else prepare(args.out, args.contract)
    except (OSError, PacketError, KeyError, TypeError, json.JSONDecodeError) as exc:
        parser.error(str(exc))
    print(json.dumps({"contract": facts["contract"], "caseIds": facts["caseIds"],
                      "modelRun": facts["modelRun"], "armInputsByteEqual": facts["armInputsByteEqual"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
