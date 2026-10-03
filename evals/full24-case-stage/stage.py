#!/usr/bin/env python3
"""Stage one validated full24 case for a future, separately reviewed runtime.

This module copies source-prepared bytes only. It never starts a runtime,
client, model, provider, container, server, or grader.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat

REPO = Path(__file__).resolve().parents[2]
PREPARE_PATH = REPO / "evals/full24-pair-preflight/prepare.py"
SPEC = importlib.util.spec_from_file_location("full24_case_stage_prepare", PREPARE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("full24 source preparer is unavailable")
prepare = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(prepare)

SCHEMA = "full24-case-stage.v1"
ARMS = ("without", "with")
CONTROL_CASES = {"DEL-03", "DEL-04"}


class StageError(ValueError):
    """The source package or requested stage is unsafe or inconsistent."""


def _hash(data: bytes) -> str:
    return prepare.digest(data)


def _no_symlink_components(path: Path) -> Path:
    """Return an absolute path after rejecting symlinks in every component."""
    expanded = path.expanduser()
    if ".." in expanded.parts:
        raise StageError("parent traversal is not allowed in source/output paths")
    absolute = Path(os.path.abspath(os.fspath(expanded)))
    current = Path(absolute.anchor)
    for part in absolute.parts[1:]:
        current = current / part
        try:
            mode = current.lstat().st_mode
        except FileNotFoundError:
            continue
        if stat.S_ISLNK(mode):
            raise StageError(f"symlink path component is not allowed: {current}")
    return absolute


def _reject_tree_symlinks(root: Path) -> None:
    if root.is_symlink():
        raise StageError("source preparation root may not be a symlink")
    for current, directories, files in os.walk(root, followlinks=False):
        base = Path(current)
        for name in directories + files:
            mode = (base / name).lstat().st_mode
            if not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)):
                raise StageError("tree entries must be regular files or directories")


def _hash_tree(root: Path) -> dict[str, str]:
    return prepare._safe_tree_hashes(root)


def _write_new(path: Path, data: bytes, mode: int) -> None:
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    fd = os.open(path, flags, mode)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
    os.chmod(path, mode)


def _freeze_tree(root: Path) -> None:
    for path in sorted(root.rglob("*"), reverse=True):
        if path.is_symlink():
            raise StageError("symlink appeared while writing model stage")
        if path.is_file():
            os.chmod(path, 0o444)
        elif path.is_dir():
            os.chmod(path, 0o555)
    os.chmod(root, 0o555)


def _expected_files(report: dict, case_id: str, arm: str, control_id: str | None) -> tuple[dict[str, bytes], dict]:
    case = next((item for item in report["cases"] if item["id"] == case_id), None)
    if case is None:
        raise StageError(f"unknown frozen case: {case_id}")

    ordinary_root = Path("cases") / case_id
    ordinary_hashes = {
        Path(name).relative_to(ordinary_root).as_posix(): sha
        for name, sha in report["arms"]["without"]["files"].items()
        if Path(name).is_relative_to(ordinary_root)
    }
    # The prepared package independently binds the common baseline and treatment
    # case trees. Keep this check explicit before reading any selected bytes.
    for source_arm in ARMS:
        arm_root = Path("cases") / case_id
        expected = {
            Path(name).relative_to(arm_root).as_posix(): sha
            for name, sha in report["arms"]["without"]["files"].items()
            if Path(name).is_relative_to(arm_root)
        }
        source = Path(report["_source_root"]) / "arms" / source_arm / arm_root
        if _hash_tree(source) != expected:
            raise StageError(f"{case_id} source case tree differs from its validated binding")
    selected_root = Path(report["_source_root"]) / "arms" / arm / ordinary_root
    ordinary = {name: (selected_root / name).read_bytes() for name in ordinary_hashes}

    if control_id is not None:
        control = report.get("authoredInputControls", {}).get(case_id)
        if case_id not in CONTROL_CASES or not isinstance(control, dict) or control.get("id") != control_id:
            raise StageError("control is not the declared authored input control for this case")
        control_root = Path(report["_source_root"]) / "authored-input-controls" / case_id
        source_control = control_root / "arms" / arm
        expected_control = {"prompt.md": control["promptSha256"]}
        expected_control.update(control["inputFiles"])
        if _hash_tree(source_control) != expected_control:
            raise StageError("authored-control source tree differs from its validated binding")
        # A control replaces prompt/evidence. It inherits only the case's declared
        # ordinary-tool grant; original-case evidence is not copied alongside it.
        files = {name: data for name, data in ordinary.items() if name == "ordinary-tool-grant.json"}
        files.update({name: (source_control / name).read_bytes() for name in expected_control})
    else:
        files = ordinary

    return files, case


def _treatment_files(source_root: Path, report: dict, arm: str) -> dict[str, bytes]:
    if arm == "without":
        return {}
    result = {}
    for relative, expected_hash in sorted(report["arms"]["with"]["treatmentFiles"].items()):
        source = source_root / "arms" / "with" / relative
        if source.is_symlink() or not source.is_file():
            raise StageError(f"declared treatment file is missing or unsafe: {relative}")
        data = source.read_bytes()
        if _hash(data) != expected_hash:
            raise StageError(f"treatment file hash differs from validated source: {relative}")
        result[relative] = data
    return result


def _set_private_directory(path: Path) -> None:
    path.mkdir(mode=0o700)
    os.chmod(path, 0o700)


def _remove_owned_tree(root: Path) -> None:
    if not root.exists() or root.is_symlink():
        return
    for path in root.rglob("*"):
        if path.is_dir() and not path.is_symlink():
            os.chmod(path, 0o700)
        elif path.is_file() and not path.is_symlink():
            os.chmod(path, 0o600)
    os.chmod(root, 0o700)
    shutil.rmtree(root, ignore_errors=True)


def _receipt_for(source_root: Path, report: dict, report_bytes: bytes, case_id: str,
                arm: str, control: str | None, case: dict,
                staged_hashes: dict[str, str], files: dict[str, bytes]) -> dict:
    return {
        "schema": SCHEMA,
        "status": "not_admitted_not_run",
        "modelStageStatus": "future_mount_candidate_not_mounted",
        "sourcePreparation": {
            "path": str(source_root),
            "reportSha256": _hash(report_bytes),
            "schema": report["schema"],
            "status": report["status"],
        },
        "selection": {"case": case_id, "arm": arm, "authoredInputControl": control},
        "caseBinding": {"sourceCase": case["sourceCase"], "group": case["group"]},
        "answerContract": {"contract": case["answerContract"]},
        "prompt": {"path": "prompt.md", "sha256": staged_hashes["prompt.md"]},
        "ordinaryToolGrant": {
            "path": "ordinary-tool-grant.json",
            "sha256": staged_hashes["ordinary-tool-grant.json"],
            "enforcement": "not-verified",
        },
        "stagedFiles": staged_hashes,
        "sourcePins": {name: _hash(data) for name, data in sorted(files.items())},
        "treatmentMcp": ({"status": "pending_inert_marker_retained", "execution": "not-invoked"}
                          if arm == "with" else {"status": "not-present_in_without_arm"}),
        "claims": {
            "ordinaryToolGrantEnforced": False,
            "sandboxVerified": False,
            "mcpUsable": False,
            "graderExecuted": False,
            "processAccountingComplete": False,
            "qualityOrBillingMeasured": False,
        },
    }


def stage(source_prep: Path, out: Path, case_id: str, arm: str, control: str | None = None) -> dict:
    if case_id not in prepare.FROZEN_CASE_IDS:
        raise StageError(f"unknown frozen case: {case_id}")
    if arm not in ARMS:
        raise StageError(f"arm must be one of: {', '.join(ARMS)}")

    source_path = _no_symlink_components(source_prep)
    if not source_path.is_dir():
        raise StageError("source preparation root must be an existing directory")
    _reject_tree_symlinks(source_path)
    destination = _no_symlink_components(out)
    if destination.exists():
        raise StageError("output must be a fresh directory path")
    source_real = source_path.resolve(strict=True)
    destination_parent = destination.parent.resolve(strict=True)
    destination_real = destination_parent / destination.name
    if (destination_real == source_real or source_real in destination_real.parents or
            destination_real in source_real.parents):
        raise StageError("source preparation and output paths may not overlap")

    try:
        report = prepare.validate(source_real)
    except (prepare.PreparationError, OSError, KeyError, TypeError, ValueError) as exc:
        raise StageError(f"full24 source preparation validation failed: {exc}") from None
    report_bytes = (source_real / "preflight.json").read_bytes()
    return _stage_validated(source_real, destination, case_id, arm, control, report, report_bytes)


def _stage_validated(source_real: Path, destination: Path, case_id: str, arm: str,
                     control: str | None, report: dict, report_bytes: bytes) -> dict:
    """Copy from an already validated source report (also used by bounded tests)."""
    if case_id not in prepare.FROZEN_CASE_IDS:
        raise StageError(f"unknown frozen case: {case_id}")
    if arm not in ARMS:
        raise StageError(f"arm must be one of: {', '.join(ARMS)}")
    destination = _no_symlink_components(destination)
    if destination.exists():
        raise StageError("output must be a fresh directory path")
    destination_parent = destination.parent.resolve(strict=True)
    destination_real = destination_parent / destination.name
    source_real = source_real.resolve(strict=True)
    if (destination_real == source_real or source_real in destination_real.parents or
            destination_real in source_real.parents):
        raise StageError("source preparation and output paths may not overlap")
    report["_source_root"] = str(source_real)
    files, case = _expected_files(report, case_id, arm, control)
    treatment = _treatment_files(source_real, report, arm)
    for relative, data in treatment.items():
        if relative in files:
            raise StageError(f"treatment file collides with case input: {relative}")
        files[relative] = data

    _set_private_directory(destination)
    model_root = destination / "model-stage"
    _set_private_directory(model_root)
    try:
        for relative, data in sorted(files.items()):
            relative_path = Path(relative)
            if relative_path.is_absolute() or ".." in relative_path.parts or relative_path == Path("."):
                raise StageError(f"unsafe staged relative path: {relative}")
            _write_new(model_root / relative_path, data, 0o444)
        _freeze_tree(model_root)
        staged_hashes = _hash_tree(model_root)
        expected_hashes = {name: _hash(data) for name, data in files.items()}
        if staged_hashes != expected_hashes:
            raise StageError("staged file set/hash verification failed")

        receipt = _receipt_for(source_real, report, report_bytes, case_id, arm, control,
                               case, staged_hashes, files)
        receipt_path = destination / "receipt.json"
        _write_new(receipt_path, (json.dumps(receipt, sort_keys=True, indent=2) + "\n").encode(), 0o600)
        _verify_validated_stage(destination, receipt, source_real, report, report_bytes)
        return receipt
    except BaseException:
        _remove_owned_tree(destination)
        raise


def verify(out: Path, source_prep: Path | None = None, case_id: str | None = None,
           arm: str | None = None, control: str | None = None) -> dict:
    root = _no_symlink_components(out)
    if not root.is_dir() or root.is_symlink():
        raise StageError("stage output root is missing or unsafe")
    _reject_tree_symlinks(root)
    if {entry.name for entry in root.iterdir()} != {"receipt.json", "model-stage"}:
        raise StageError("stage output has unexpected host-side entries")
    receipt_path = root / "receipt.json"
    model_root = root / "model-stage"
    if receipt_path.is_symlink() or model_root.is_symlink() or not receipt_path.is_file() or not model_root.is_dir():
        raise StageError("stage receipt or model tree is missing/unsafe")
    try:
        receipt = json.loads(receipt_path.read_bytes())
    except (OSError, json.JSONDecodeError):
        raise StageError("stage receipt is missing or malformed") from None
    if not isinstance(receipt, dict):
        raise StageError("stage receipt must be a JSON object")
    for section in ("sourcePreparation", "selection", "claims", "caseBinding",
                    "answerContract", "prompt", "ordinaryToolGrant", "stagedFiles",
                    "sourcePins", "treatmentMcp"):
        if not isinstance(receipt.get(section), dict):
            raise StageError(f"stage receipt {section} must be an object")
    source_value = receipt["sourcePreparation"].get("path")
    if not isinstance(source_value, str) or not source_value:
        raise StageError("stage receipt source path must be a nonempty string")
    for value in receipt["claims"].values():
        if type(value) is not bool or value:
            raise StageError("stage receipt claims must remain explicitly false")
    if receipt.get("schema") != SCHEMA or receipt.get("status") != "not_admitted_not_run":
        raise StageError("stage receipt schema/status is not source-only")
    if receipt.get("claims", {}).get("ordinaryToolGrantEnforced") is not False or receipt.get("claims", {}).get("sandboxVerified") is not False:
        raise StageError("stage receipt overclaims runtime enforcement")
    selection = receipt.get("selection", {})
    source_root = Path(receipt.get("sourcePreparation", {}).get("path", ""))
    source_path = _no_symlink_components(source_root)
    if source_prep is not None and _no_symlink_components(source_prep) != source_path:
        raise StageError("requested source path differs from staged receipt")
    if case_id is not None and receipt.get("selection", {}).get("case") != case_id:
        raise StageError("requested case differs from staged receipt")
    if arm is not None and receipt.get("selection", {}).get("arm") != arm:
        raise StageError("requested arm differs from staged receipt")
    if control != receipt.get("selection", {}).get("authoredInputControl"):
        raise StageError("requested authored control differs from staged receipt")
    _reject_tree_symlinks(source_path)
    try:
        report = prepare.validate(source_path)
    except (prepare.PreparationError, OSError, KeyError, TypeError, ValueError) as exc:
        raise StageError(f"source preparation no longer validates: {exc}") from None
    raw_report = (source_path / "preflight.json").read_bytes()
    return _verify_validated_stage(root, receipt, source_path, report, raw_report)


def _verify_validated_stage(root: Path, receipt: dict, source_path: Path,
                            report: dict, raw_report: bytes) -> dict:
    if _hash(raw_report) != receipt["sourcePreparation"].get("reportSha256"):
        raise StageError("source preparation report hash changed")
    report["_source_root"] = str(source_path)
    selection = receipt.get("selection", {})
    if (not isinstance(selection, dict) or selection.get("case") not in prepare.FROZEN_CASE_IDS
            or selection.get("arm") not in ARMS):
        raise StageError("receipt case or arm is not a frozen selection")
    model_root = root / "model-stage"
    receipt_path = root / "receipt.json"
    files, case = _expected_files(report, selection.get("case"), selection.get("arm"), selection.get("authoredInputControl"))
    files.update(_treatment_files(source_path, report, selection["arm"]))
    expected_hashes = {name: _hash(data) for name, data in files.items()}
    actual_hashes = _hash_tree(model_root)
    if actual_hashes != expected_hashes or actual_hashes != receipt.get("stagedFiles"):
        raise StageError("model-stage exact tree/hash set differs from receipt/source")
    expected_dirs = {Path("model-stage")}
    for name in expected_hashes:
        parent = Path("model-stage") / Path(name).parent
        while parent != Path("model-stage"):
            expected_dirs.add(parent)
            parent = parent.parent
    actual_dirs = {p.relative_to(root) for p in root.rglob("*") if p.is_dir()}
    if actual_dirs != expected_dirs:
        raise StageError("model-stage contains missing or extra directories")
    expected_receipt = _receipt_for(source_path, report, raw_report,
                                    selection["case"], selection["arm"],
                                    selection.get("authoredInputControl"), case,
                                    actual_hashes, files)
    if receipt != expected_receipt:
        raise StageError("host receipt differs from the reconstructed source binding")
    for path in [model_root, *model_root.rglob("*")]:
        if path.is_dir():
            if stat.S_IMODE(path.stat().st_mode) & 0o222:
                raise StageError("model-stage directory is writable")
        elif stat.S_IMODE(path.stat().st_mode) & 0o222:
            raise StageError("model-stage file is writable")
    if stat.S_IMODE(root.stat().st_mode) != 0o700 or stat.S_IMODE(receipt_path.stat().st_mode) != 0o600:
        raise StageError("private output/host receipt permissions drifted")
    if selection["arm"] == "with":
        marker = json.loads((model_root / ".claude-plugin/plugin.json").read_bytes())
        if any(value.get("command") != "__RECORDED_MCP_BINDING_PENDING__" for value in marker.get("mcpServers", {}).values()):
            raise StageError("treatment MCP marker is not inert/pending")
        if receipt.get("treatmentMcp", {}).get("status") != "pending_inert_marker_retained":
            raise StageError("treatment MCP status is not explicitly pending")
    return receipt


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-prep", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--case", choices=prepare.FROZEN_CASE_IDS, required=True)
    parser.add_argument("--arm", choices=ARMS, required=True)
    parser.add_argument("--control", help="exact declared authored-input control id, when selected")
    parser.add_argument("--verify", action="store_true", help="verify output and require source/case/arm/control to match its receipt")
    args = parser.parse_args(argv)
    try:
        if args.verify:
            receipt = verify(args.out, args.source_prep, args.case, args.arm, args.control)
        else:
            receipt = stage(args.source_prep, args.out, args.case, args.arm, args.control)
    except (StageError, OSError, KeyError, TypeError, ValueError) as exc:
        parser.exit(2, f"full24 case stage failed: {exc}\n")
    print(json.dumps({"schema": receipt["schema"], "status": receipt["status"],
                      "case": receipt["selection"]["case"], "arm": receipt["selection"]["arm"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
