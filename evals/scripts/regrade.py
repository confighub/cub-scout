#!/usr/bin/env python3
"""Regrade saved Claude eval transcripts with the current regex graders.

Usage: evals/scripts/regrade.py RESULT.json --out AUDIT.json

This is an offline audit. It does not edit the source result and never reruns
an agent or judge. Only one-line regex grader frontmatter is supported.
"""
import argparse
import hashlib
import json
import os
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TOOL_TYPE = "tool_used"


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def contained(path, parent):
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


class TraceError(ValueError):
    def __init__(self, message, metadata=None):
        super().__init__(message)
        self.metadata = metadata or {}


def parse_grader(path, raw):
    """Parse the repo's deliberately small grader frontmatter subset."""
    text = raw.decode("utf-8")
    if not text.startswith("---\n"):
        raise ValueError("missing YAML frontmatter")
    end = text.find("\n---\n", 4)
    if end < 0:
        raise ValueError("unterminated YAML frontmatter")
    fields = {}
    for line in text[4:end].splitlines():
        if not line or line.startswith("#"):
            continue
        match = re.fullmatch(r"([A-Za-z_][A-Za-z0-9_]*): (.*)", line)
        if not match or match.group(1) in fields:
            raise ValueError("unsupported frontmatter shape")
        fields[match.group(1)] = match.group(2)
    grader_type = fields.get("type")
    if grader_type == TOOL_TYPE:
        return {"kind": TOOL_TYPE, "sha256": sha256(raw)}
    if grader_type != "regex":
        raise ValueError("unsupported grader type: " + str(grader_type))
    allowed = {"type", "pattern", "flags", "target"}
    if set(fields) - allowed:
        raise ValueError("unsupported regex grader fields")
    pattern_value = fields.get("pattern", "")
    if len(pattern_value) < 2 or pattern_value[0] != "'" or pattern_value[-1] != "'":
        raise ValueError("regex pattern must be a single-quoted one-line value")
    pattern = pattern_value[1:-1].replace("''", "'")
    flags_text = fields.get("flags", "")
    flag_map = {"i": re.IGNORECASE, "m": re.MULTILINE, "s": re.DOTALL}
    flags = 0
    for char in flags_text:
        if char not in flag_map:
            raise ValueError("unsupported regex flag: " + char)
        flags |= flag_map[char]
    target = fields.get("target", "last_message")
    if target != "last_message":
        raise ValueError("unsupported regex target: " + target)
    try:
        compiled = re.compile(pattern, flags)
    except re.error as exc:
        raise ValueError("invalid regex: " + str(exc)) from exc
    return {"kind": "regex", "pattern": pattern, "compiled": compiled, "sha256": sha256(raw)}


def graders_for_case(case, hashes):
    directory = case.get("dir")
    if not isinstance(directory, str) or not directory:
        raise ValueError("case dir is missing or malformed")
    case_dir = (ROOT / directory).resolve()
    if not contained(case_dir, ROOT) or not case_dir.is_dir():
        raise ValueError("case dir is missing or outside repository root")
    graders_dir = (case_dir / "graders").resolve()
    if not contained(graders_dir, case_dir) or not graders_dir.is_dir():
        raise ValueError("grader directory is missing or escapes case dir")
    files = sorted(graders_dir.glob("*.md"))
    if not files:
        raise ValueError("no current grader files")
    safe_files = []
    for path in files:
        resolved = path.resolve()
        if not contained(resolved, graders_dir):
            raise ValueError("grader file escapes grader directory")
        safe_files.append(resolved)

    sources = []
    for path in safe_files:
        raw = path.read_bytes()
        hashes.append({"path": path.relative_to(ROOT).as_posix(), "sha256": sha256(raw)})
        sources.append((path, raw))
    parsed = []
    for path, raw in sources:
        definition = parse_grader(path, raw)
        if definition["kind"] != TOOL_TYPE:
            parsed.append((path.name, definition))
    if not parsed:
        raise ValueError("no required regex graders (tool_used graders are excluded)")
    return parsed, hashes


def terminal_result(trace_path, input_parent, trace_roots):
    if not isinstance(trace_path, str) or not trace_path:
        raise TraceError("tracePath is missing or malformed")
    candidate = Path(trace_path)
    if not candidate.is_absolute():
        candidate = input_parent / candidate
    resolved = candidate.resolve()
    if not any(contained(resolved, root) for root in trace_roots):
        raise TraceError("trace path is outside the allowed trace roots")
    metadata = {"tracePath": str(resolved)}
    try:
        raw = resolved.read_bytes()
    except OSError as exc:
        raise TraceError("cannot read trace: " + str(exc), metadata) from exc
    metadata["traceSha256"] = sha256(raw)
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise TraceError("trace is not UTF-8", metadata) from exc
    records = [line for line in text.splitlines() if line.strip()]
    if not records:
        raise TraceError("trace is empty", metadata)
    try:
        terminal = json.loads(records[-1])
    except json.JSONDecodeError as exc:
        raise TraceError("terminal trace record is malformed JSON", metadata) from exc
    if not isinstance(terminal, dict) or terminal.get("type") != "result":
        raise TraceError("trace has no terminal type=result record", metadata)
    if not isinstance(terminal.get("result"), str):
        raise TraceError("terminal result text is missing or malformed", metadata)
    return terminal, metadata


def spend_metadata(run, terminal):
    # Retain all run-side metadata so new/unknown agent, judge, and mock cost
    # fields survive future harness versions too. The trace path is represented
    # separately by the source result and is not spend metadata.
    return {
        "run": {key: value for key, value in run.items() if key != "tracePath"},
        "terminalResult": {key: value for key, value in terminal.items() if key != "result"},
    }


def regrade_run(run, graders, input_parent, trace_roots):
    if not isinstance(run, dict):
        return {"outcome": "unknown", "reason": "run metadata is malformed", "spend": {"run": {}}}
    try:
        terminal, trace_info = terminal_result(run.get("tracePath"), input_parent, trace_roots)
    except TraceError as exc:
        trace_info = exc.metadata
        if run.get("error"):
            return {"outcome": "fail", "reason": "run ended in an error or interruption",
                    "spend": spend_metadata(run, {}), **trace_info}
        return {"outcome": "unknown", "reason": str(exc), "spend": spend_metadata(run, {}), **trace_info}
    spend = spend_metadata(run, terminal)
    if "is_error" in terminal and type(terminal["is_error"]) is not bool:
        return {"outcome": "unknown", "reason": "terminal is_error metadata is malformed", "spend": spend, **trace_info}
    subtype = terminal.get("subtype")
    if subtype is not None and not isinstance(subtype, str):
        return {"outcome": "unknown", "reason": "terminal subtype metadata is malformed", "spend": spend, **trace_info}
    if run.get("error") or terminal.get("is_error") is True or (subtype and subtype != "success"):
        return {"outcome": "fail", "reason": "run ended in an error or interruption", "spend": spend, **trace_info}
    answer = terminal["result"]
    failures = []
    for name, definition in graders:
        if definition["kind"] == TOOL_TYPE:
            continue
        if definition["compiled"].search(answer) is None:
            failures.append(name)
    if failures:
        return {"outcome": "fail", "reason": "required regex grader(s) did not match: " + ", ".join(failures), "spend": spend, **trace_info}
    return {"outcome": "pass", "reason": "all current required regex graders matched terminal result text", "spend": spend, **trace_info}


def audit(source_path, explicit_trace_roots=None):
    input_path = source_path.resolve(strict=True)
    input_parent = input_path.parent.resolve(strict=True)
    explicit_trace_roots = explicit_trace_roots or []
    trace_roots = [input_parent] + explicit_trace_roots
    source_bytes = input_path.read_bytes()
    source = json.loads(source_bytes)
    if not isinstance(source, dict) or not isinstance(source.get("cases"), list):
        raise ValueError("input must be a result object with a cases array")
    cases_out = []
    for case_index, case in enumerate(source["cases"]):
        label = {"caseIndex": case_index, "name": case.get("name") if isinstance(case, dict) else None}
        if not isinstance(case, dict):
            cases_out.append({**label, "graderHashes": [], "arms": {}, "reason": "case metadata is malformed"})
            continue
        try:
            hashes = []
            graders, hashes = graders_for_case(case, hashes)
            load_error = None
        except (OSError, UnicodeError, ValueError) as exc:
            graders, load_error = [], str(exc)
        arms_out = {}
        arms = case.get("arms")
        if not isinstance(arms, dict):
            arms = {}
            load_error = load_error or "case arms metadata is missing or malformed"
        for arm, runs in arms.items():
            if not isinstance(runs, list):
                arms_out[str(arm)] = [{"runIndex": 0, "outcome": "unknown", "reason": "arm run list is malformed", "spend": {"run": {}}}]
                continue
            audited = []
            for run_index, run in enumerate(runs):
                if load_error:
                    outcome = {"outcome": "unknown", "reason": load_error,
                               "spend": spend_metadata(run, {}) if isinstance(run, dict) else {"run": {}}}
                else:
                    outcome = regrade_run(run, graders, input_parent, trace_roots)
                audited.append({"runIndex": run_index, **outcome})
            arms_out[str(arm)] = audited
        cases_out.append({**label, "dir": case.get("dir"), "graderHashes": hashes,
                          "arms": arms_out, **({"reason": load_error} if load_error else {})})
    return {"format": "cub-scout-eval-regrade-v1", "source": str(input_path),
            "sourceParent": str(input_parent), "sourceSha256": sha256(source_bytes),
            "sourceMetadata": {key: value for key, value in source.items() if key != "cases"},
            "traceRoots": {"default": str(input_parent),
                           "explicit": [str(path) for path in explicit_trace_roots],
                           "effective": [str(path) for path in trace_roots]},
            "cases": cases_out}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result", type=Path)
    parser.add_argument("--out", required=True, type=Path, help="new audit JSON path (must not already exist)")
    parser.add_argument("--trace-root", action="append", default=[], type=Path,
                        help="explicit directory allowed to contain saved traces (repeatable; defaults to input directory)")
    args = parser.parse_args(argv)
    source = args.result.resolve(strict=True)
    output = args.out.resolve()
    if source == output:
        parser.error("--out must differ from the input result")
    if args.trace_root:
        try:
            explicit_trace_roots = [path.resolve(strict=True) for path in args.trace_root]
        except OSError as exc:
            parser.error("trace root must exist: " + str(exc))
    else:
        explicit_trace_roots = []
    if any(not path.is_dir() for path in explicit_trace_roots):
        parser.error("each --trace-root must be a directory")
    payload = audit(source, explicit_trace_roots)
    encoded = (json.dumps(payload, indent=2, sort_keys=True) + "\n").encode("utf-8")
    try:
        fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o644)
    except FileExistsError:
        parser.error("output already exists; refusing to overwrite: " + str(output))
    with os.fdopen(fd, "wb") as stream:
        stream.write(encoded)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError) as exc:
        raise SystemExit("regrade: " + str(exc))
