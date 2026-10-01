#!/usr/bin/env python3
"""Conservative offline admission audit for recorded Claude eval traces.

This never launches an eval or judge. It preserves producer cost as an
unreconciled estimate and does not change correctness grading or reports.
Trace roots must be explicitly supplied ``out`` folders. Home and sealed paths
are deliberately refused; callers can select a permitted out folder without
the audit discovering paths from result metadata.
"""
import argparse
import hashlib
import json
import math
import os
import stat
import sys
from pathlib import Path


MAX_RESULT_BYTES = 10 * 1024 * 1024
MAX_TRACE_BYTES = 20 * 1024 * 1024
MAX_TRACE_RECORDS = 100_000
MAX_TRACE_LINE_BYTES = 2 * 1024 * 1024
MAX_CASES = 200
MAX_RUNS = 1000
ARM_NAMES = {"with", "without"}
REPO_ROOT = Path(__file__).resolve().parents[2]
USER_HOME = Path.home().resolve()


def sha256(raw):
    return hashlib.sha256(raw).hexdigest()


def inside(path, root):
    try:
        path.relative_to(root)
        return True
    except ValueError:
        return False


def safe_location(path):
    """Allow repository fixtures and explicit temp out roots, refuse home/sealed paths."""
    parts = path.parts
    if "sealed" in {part.lower() for part in parts} or ".claude" in parts:
        return False
    if inside(path, REPO_ROOT):
        relative_parts = path.relative_to(REPO_ROOT).parts
        if any(part.lower() == "home" for part in relative_parts):
            return False
        return True
    if "home" in {part.lower() for part in parts} or inside(path, USER_HOME):
        return False
    return True


def bounded_read(path, limit):
    flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
    try:
        descriptor = os.open(path, flags)
    except OSError as exc:
        raise ValueError("cannot safely open input: " + str(exc)) from exc
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode):
            raise ValueError("input must be a regular non-symlink file")
        with os.fdopen(descriptor, "rb") as stream:
            descriptor = -1
            raw = stream.read(limit + 1)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    if len(raw) > limit:
        raise ValueError("input exceeds the configured byte limit")
    return raw


def finite_number(value):
    try:
        return math.isfinite(value)
    except (TypeError, OverflowError):
        return False


def strict_json_loads(raw):
    def reject_constant(value):
        raise ValueError("non-finite JSON number: " + value)

    def parse_float(value):
        number = float(value)
        if not finite_number(number):
            raise ValueError("non-finite JSON number")
        return number

    def unique_object(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("duplicate JSON object key: " + repr(key))
            result[key] = value
        return result

    return json.loads(raw, parse_constant=reject_constant, parse_float=parse_float,
                      object_pairs_hook=unique_object)


def load_json_file(path, limit):
    raw = bounded_read(path, limit)
    try:
        return raw, strict_json_loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ValueError("input is not valid UTF-8 JSON") from exc


def parse_trace(path):
    """Parse only bounded JSONL data; never execute or interpret trace text."""
    raw = bounded_read(path, MAX_TRACE_BYTES)
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise ValueError("trace is not UTF-8") from exc
    records = []
    for line in text.splitlines():
        if not line.strip():
            continue
        if len(line.encode("utf-8")) > MAX_TRACE_LINE_BYTES:
            raise ValueError("trace contains an oversized record")
        if len(records) >= MAX_TRACE_RECORDS:
            raise ValueError("trace exceeds the record-count limit")
        try:
            item = strict_json_loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError("trace contains malformed JSON") from exc
        if not isinstance(item, dict) or not isinstance(item.get("type"), str):
            raise ValueError("trace record is missing a string type")
        records.append(item)
    if not records:
        raise ValueError("trace is empty")
    return raw, records


def content_blocks(record):
    message = record.get("message")
    if isinstance(message, dict):
        content = message.get("content")
        if isinstance(content, list):
            return content
    content = record.get("content")
    return content if isinstance(content, list) else []


def analyze_trace(records):
    init_records = [r for r in records if r.get("type") == "system" and r.get("subtype") == "init"]
    terminal_records = [r for r in records if r.get("type") == "result"]
    reasons = []
    if len(init_records) != 1:
        reasons.append("trace must contain exactly one system init record")
    if len(terminal_records) != 1:
        reasons.append("trace must contain exactly one terminal result record")

    init = init_records[0] if len(init_records) == 1 else {}
    tools = init.get("tools")
    model = init.get("model")
    if not isinstance(tools, list) or any(not isinstance(t, str) or not t for t in tools):
        tools = None
        reasons.append("init ordinary/MCP inventory is missing or malformed")
    elif len(set(tools)) != len(tools):
        tools = None
        reasons.append("init inventory contains duplicate tool names")
    if not isinstance(model, str) or not model:
        model = None
        reasons.append("init model is missing or malformed")

    visible = []
    assistant_models = set()
    result_ids = set()
    task_ids = {}
    progress_mcp = set()
    progress_mcp_events = []
    progress_mentions_tasks = False
    progress_lifecycle = []
    progress_records = 0
    task_starts = {}
    task_start_tool_ids = {}
    task_progress_events = []
    completed_task_ids = set()
    for record in records:
        if record.get("type") == "assistant":
            message = record.get("message")
            if not isinstance(message, dict) or not isinstance(message.get("content"), list):
                reasons.append("assistant message/content is missing or malformed")
            model_values = [record.get("model")]
            if isinstance(message, dict):
                model_values.append(message.get("model"))
            for value in model_values:
                if value is None:
                    continue
                if isinstance(value, str) and value:
                    assistant_models.add(value)
                else:
                    reasons.append("assistant model metadata is malformed")
        if record.get("type") in {"assistant", "user"}:
            for block in content_blocks(record):
                if not isinstance(block, dict):
                    continue
                if block.get("type") == "tool_use":
                    name = block.get("name")
                    if not isinstance(name, str) or not name:
                        reasons.append("visible tool-use block has malformed name")
                        continue
                    visible.append(name)
                    if name in {"Task", "Agent"}:
                        use_id = block.get("id")
                        task_ids[str(use_id) if use_id is not None else f"anonymous-{len(task_ids)}"] = name
                elif block.get("type") == "tool_result":
                    use_id = block.get("tool_use_id")
                    if isinstance(use_id, str):
                        result_ids.add(use_id)
        subtype = record.get("subtype")
        is_task_progress = record.get("type") == "system" and subtype == "task_progress"
        if record.get("type") == "system" and subtype == "task_started":
            task_id = record.get("task_id")
            depth = record.get("spawn_depth")
            tool_use_id = record.get("tool_use_id")
            if not isinstance(task_id, str) or not task_id:
                reasons.append("task_started record has malformed task_id")
            key = task_id if isinstance(task_id, str) else f"unknown-{len(task_starts)}"
            if key in task_starts:
                reasons.append("trace contains a duplicate task_started task_id")
            task_starts[key] = depth if type(depth) is int and 0 <= depth <= 100 else None
            if type(depth) is not int or not 0 <= depth <= 100:
                reasons.append("task_started record has malformed spawn_depth")
            if isinstance(tool_use_id, str) and tool_use_id:
                task_start_tool_ids[tool_use_id] = key
            elif tool_use_id is not None:
                reasons.append("task_started record has malformed tool_use_id")
        if record.get("type") == "system" and subtype in {"task_notification", "task_completed"}:
            task_id, status = record.get("task_id"), record.get("status")
            if isinstance(task_id, str) and isinstance(status, str) and status.lower() in {"completed", "complete", "finished", "success"}:
                completed_task_ids.add(task_id)
                progress_lifecycle.append(status.lower())
        if record.get("type") == "progress" or is_task_progress:
            progress_records += 1
            if is_task_progress:
                progress_mentions_tasks = True
                last_tool = record.get("last_tool_name")
                progress_task_id = record.get("task_id")
                progress_depth = record.get("spawn_depth")
                task_progress_events.append({"taskId": progress_task_id,
                                             "spawnDepthReported": progress_depth,
                                             "lastToolName": last_tool})
                if isinstance(last_tool, str) and last_tool.startswith("mcp__"):
                    progress_mcp.add(last_tool)
                    progress_mcp_events.append({"taskId": record.get("task_id"),
                                                "spawnDepthReported": record.get("spawn_depth"),
                                                "lastToolName": last_tool})
                if not isinstance(progress_task_id, str) or not progress_task_id:
                    reasons.append("task_progress record has malformed task_id")
                if not isinstance(last_tool, str) or not last_tool:
                    reasons.append("task_progress record has malformed last_tool_name")
            # Only exact structured event fields count; arbitrary trace prose
            # is never normalized into a tool identifier.
    outstanding = [name for use_id, name in task_ids.items() if use_id.startswith("anonymous-") or use_id not in result_ids]
    if outstanding:
        reasons.append("one or more visible Task/Agent calls lack a matching tool result")
    unresolved_started = sorted(task_id for task_id in task_starts if task_id not in completed_task_ids)
    if unresolved_started:
        reasons.append("task_started records lack explicit completed task notifications")
    for event in task_progress_events:
        task_id, reported_depth = event["taskId"], event["spawnDepthReported"]
        if not isinstance(task_id, str) or task_id not in task_starts:
            reasons.append("task_progress has no matching task_started record for its exact task_id")
            event["spawnDepthResolved"] = None
            continue
        started_depth = task_starts[task_id]
        if reported_depth is None:
            event["spawnDepthResolved"] = started_depth
            if started_depth is None:
                reasons.append("task_progress depth is unknown and task_started has no valid depth")
        elif type(reported_depth) is not int or not 0 <= reported_depth <= 100:
            event["spawnDepthResolved"] = started_depth
            reasons.append("task_progress spawn_depth is malformed")
        elif started_depth is None:
            event["spawnDepthResolved"] = None
            reasons.append("task_progress has a depth but matching task_started depth is unknown")
        elif reported_depth != started_depth:
            event["spawnDepthResolved"] = None
            reasons.append("task_progress spawn_depth conflicts with matching task_started depth")
        else:
            event["spawnDepthResolved"] = started_depth
    if task_progress_events:
        progress_ids = {event["taskId"] for event in task_progress_events if isinstance(event["taskId"], str)}
        unmatched_completions = completed_task_ids - progress_ids - set(task_starts)
        if unmatched_completions:
            reasons.append("task completion notification has no matching task_started/task_progress task_id")
        for task_id in progress_ids:
            if task_id not in completed_task_ids:
                reasons.append(f"task_progress task_id {task_id} lacks its own completion notification")
    if task_starts:
        reasons.append("descendant inventory/cost coverage is unavailable for spawned tasks")
    unmatched_task_tool_ids = [use_id for use_id in task_ids
                               if use_id.startswith("anonymous-") or use_id not in task_start_tool_ids]
    for use_id in task_ids:
        if use_id.startswith("anonymous-") or use_id not in task_start_tool_ids:
            reasons.append("visible Task/Agent tool-use lacks matching task_started tool_use_id")
    has_task_evidence = bool(task_starts or task_progress_events or task_ids)
    unknown_depth = (any(depth is None for depth in task_starts.values()) or
                     any(event.get("spawnDepthResolved") is None for event in task_progress_events) or
                     bool(unmatched_task_tool_ids))
    known_depths = [depth for depth in task_starts.values() if type(depth) is int]
    known_depths.extend(event["spawnDepthResolved"] for event in task_progress_events
                        if type(event.get("spawnDepthResolved")) is int)
    task_depth = None if has_task_evidence and unknown_depth else max(known_depths, default=0)
    if progress_mcp:
        reasons.append("nested progress mentions MCP without auditable call bodies")
    if tools is not None:
        unadvertised = sorted(set(visible) - set(tools))
        if unadvertised:
            reasons.append("visible tool-use names are absent from init inventory: " + ", ".join(unadvertised))
    if len(set(visible)) != len(visible):
        # Repeated calls are valid; this is evidence output, not ambiguity.
        pass

    terminal = terminal_records[0] if len(terminal_records) == 1 else {}
    terminal_ok = (records[-1].get("type") == "result" and
                   isinstance(terminal.get("result"), str) and
                   type(terminal.get("is_error")) is bool and terminal.get("is_error") is False and
                   isinstance(terminal.get("subtype"), str) and terminal.get("subtype") == "success") if terminal_records else False
    if not terminal_ok:
        reasons.append("terminal result is missing, malformed, or unsuccessful")

    ordinary = sorted(t for t in tools if not t.startswith("mcp__")) if tools is not None else None
    mcp = sorted(t for t in tools if t.startswith("mcp__")) if tools is not None else None
    return {
        "terminalResultPresent": len(terminal_records) == 1,
        "terminalResultSuccessful": terminal_ok,
        "initRecordCount": len(init_records),
        "terminalRecordCount": len(terminal_records),
        "model": model,
        "assistantModels": sorted(assistant_models),
        "inventory": {"ordinary": ordinary, "mcp": mcp},
        "visibleToolUseBlocks": visible,
        "visibleMcpCalls": [name for name in visible if name.startswith("mcp__")],
        "nestedProgressEvidence": {"mcpMentions": sorted(progress_mcp),
                                    "mcpProgressEvents": progress_mcp_events,
                                    "mentionsNestedTasks": progress_mentions_tasks,
                                    "recordCount": progress_records,
                                    "taskLifecycleStatuses": progress_lifecycle},
        "taskDepthMaxObserved": task_depth,
        "unresolvedTaskCalls": len(outstanding),
        "taskStarted": [{"taskId": task_id, "spawnDepth": depth} for task_id, depth in task_starts.items()],
        "unresolvedStartedTaskIds": unresolved_started,
        "taskProgressEvents": task_progress_events,
        "reasons": reasons,
    }


def trace_for(run, source_parent, roots):
    trace_path = run.get("tracePath")
    if not isinstance(trace_path, str) or not trace_path:
        raise ValueError("tracePath is missing or malformed")
    candidate = Path(trace_path)
    if not candidate.is_absolute():
        candidate = source_parent / candidate
    if candidate.is_symlink():
        raise ValueError("trace path is a symlink")
    # Resolve to catch traversal/symlink escapes, then also refuse any path
    # naming a sealed/home location, even if that path happens to be allowed.
    resolved = candidate.resolve(strict=True)
    if not safe_location(resolved):
        raise ValueError("trace path resolves into a sealed or home directory")
    if not any(inside(resolved, root) for root in roots):
        raise ValueError("trace path is outside explicitly allowed trace roots")
    raw, records = parse_trace(resolved)
    return resolved, raw, analyze_trace(records)


def audit(source_path, trace_root_paths, expected_arms, expected_runs):
    if source_path.is_symlink():
        raise ValueError("result path may not be a symlink")
    source = source_path.resolve(strict=True)
    if not safe_location(source):
        raise ValueError("result path resolves into a sealed or home directory")
    source_parent = source.parent.resolve(strict=True)
    roots = []
    for supplied in trace_root_paths:
        if supplied.is_symlink():
            raise ValueError("trace root may not be a symlink")
        root = supplied.resolve(strict=True)
        if not root.is_dir() or not safe_location(root):
            raise ValueError("trace roots must be existing, non-sealed directories")
        if root.name != "out":
            raise ValueError("each explicitly allowed trace root must be a copied out directory")
        roots.append(root)
    if not roots:
        raise ValueError("at least one explicit --trace-root is required")

    source_bytes, result = load_json_file(source, MAX_RESULT_BYTES)
    if not isinstance(result, dict) or not isinstance(result.get("cases"), list):
        raise ValueError("result must be an object with a cases array")
    if result.get("partial") is not False:
        partial_reason = "producer partial flag is true" if result.get("partial") is True else "producer partial flag is missing or malformed"
        source_incomplete = True
    else:
        partial_reason = None
        source_incomplete = False
    cases = result["cases"]
    if not cases or len(cases) > MAX_CASES:
        raise ValueError("cases array is empty or exceeds the configured limit")
    if expected_runs < 1 or expected_runs > MAX_RUNS:
        raise ValueError("explicit expected run count is outside configured bounds")
    if not expected_arms or len(set(expected_arms)) != len(expected_arms) or set(expected_arms) - ARM_NAMES:
        raise ValueError("expected arms must be an explicit unique subset of with,without")

    cases_out = []
    source_cost = result.get("costUsd")
    if isinstance(source_cost, bool) or not isinstance(source_cost, (int, float)) or not finite_number(source_cost) or source_cost < 0:
        source_cost = None
    for index, case in enumerate(cases):
        reasons = []
        if source_incomplete:
            reasons.append(partial_reason)
        if not isinstance(case, dict) or not isinstance(case.get("name"), str):
            cases_out.append({"caseIndex": index, "status": "NOT_ADMITTED", "reasons": ["case metadata is malformed"], "arms": {}})
            continue
        max_turns = case.get("maxTurns")
        if type(max_turns) is not int or max_turns < 1:
            reasons.append("declared maxTurns is missing or malformed")
            max_turns = None
        arms = case.get("arms")
        if not isinstance(arms, dict):
            arms = {}
            reasons.append("arm metadata is missing or malformed")
        expected_arm_set = set(expected_arms)
        if set(arms) != expected_arm_set:
            reasons.append("observed arms do not exactly match caller-supplied expected arms")
        arm_out = {}
        ordinary_inventories = []
        for arm in expected_arms:
            runs = arms.get(arm)
            if not isinstance(runs, list):
                runs = []
                reasons.append(f"{arm}: run list is missing or malformed")
            if len(runs) != expected_runs:
                reasons.append(f"{arm}: expected {expected_runs} run(s) from caller, observed {len(runs)}")
            if len(runs) > MAX_RUNS:
                reasons.append(f"{arm}: observed run list exceeds {MAX_RUNS} audit bound")
            runs = runs[:MAX_RUNS]
            audited_runs = []
            for run_index, run in enumerate(runs):
                run_reasons = []
                if not isinstance(run, dict):
                    audited_runs.append({"runIndex": run_index, "admitted": False, "reasons": ["run metadata is malformed"]})
                    reasons.append(f"{arm} run {run_index}: run metadata is malformed")
                    continue
                turns = run.get("turns")
                if type(turns) is not int or turns < 0:
                    run_reasons.append("reported turns are missing or malformed")
                elif max_turns is not None and turns > max_turns:
                    run_reasons.append(f"reported turns {turns} exceed declared maxTurns {max_turns}")
                if "error" in run and run["error"] is not None:
                    error_value = run["error"]
                    if not isinstance(error_value, str) or not error_value.strip():
                        run_reasons.append("producer run.error field is malformed")
                    else:
                        run_reasons.append("producer reports an error, timeout, or interruption")
                run_cost = run.get("costUsd")
                if (isinstance(run_cost, bool) or not isinstance(run_cost, (int, float)) or
                        not finite_number(run_cost) or run_cost < 0):
                    run_reasons.append("reported run costUsd is missing or malformed")
                timeout = case.get("timeoutSeconds")
                duration = run.get("durationSeconds")
                if (isinstance(timeout, bool) or not isinstance(timeout, (int, float)) or not finite_number(timeout) or timeout <= 0):
                    run_reasons.append("declared timeoutSeconds is missing or malformed")
                if (isinstance(duration, bool) or not isinstance(duration, (int, float)) or not finite_number(duration) or duration < 0):
                    run_reasons.append("reported durationSeconds is missing or malformed")
                elif isinstance(timeout, (int, float)) and not isinstance(timeout, bool) and finite_number(timeout) and duration >= timeout:
                    run_reasons.append("reported duration reached or exceeded case timeout")
                try:
                    path, trace_raw, trace = trace_for(run, source_parent, roots)
                    run_record = {"tracePath": str(path), "traceSha256": sha256(trace_raw), **trace}
                    run_reasons.extend(trace["reasons"])
                    ordinary_inventories.append(trace["inventory"]["ordinary"])
                except (OSError, ValueError) as exc:
                    run_record = {"tracePath": run.get("tracePath") if isinstance(run.get("tracePath"), str) else None,
                                  "traceSha256": None, "reasons": [str(exc)]}
                    run_reasons.append(str(exc))
                    ordinary_inventories.append(None)
                run_record.update({"reportedTurns": turns, "declaredMaxTurns": max_turns,
                                   "reportedDurationSeconds": duration,
                                   "declaredTimeoutSeconds": timeout,
                                   "reportedError": run.get("error"),
                                   "reportedCostUsd": run.get("costUsd"),
                                   "admitted": not run_reasons, "reasons": run_reasons})
                audited_runs.append(run_record)
                reasons.extend(f"{arm} run {run_index}: {reason}" for reason in run_reasons)
            arm_out[arm] = audited_runs
        if len(ordinary_inventories) == len(expected_arms) * expected_runs and all(isinstance(i, list) for i in ordinary_inventories):
            baseline = ordinary_inventories[0]
            if any(i != baseline for i in ordinary_inventories[1:]):
                reasons.append("top-level ordinary tool inventories differ across expected runs/arms")
            inventory_comparison = {"comparable": True, "equal": all(i == baseline for i in ordinary_inventories[1:])}
        else:
            inventory_comparison = {"comparable": False, "equal": None, "reason": "one or more inventories are missing or ambiguous"}
            reasons.append("top-level ordinary tool inventories cannot be compared unambiguously")
        init_models = [run.get("model") for runs in arm_out.values() for run in runs
                       if isinstance(run.get("model"), str)]
        assistant_models = [model for runs in arm_out.values() for run in runs
                            for model in run.get("assistantModels", []) if isinstance(model, str)]
        models = init_models + assistant_models
        declared_model = (result.get("suite") or {}).get("modelOverride") if isinstance(result.get("suite"), dict) else None
        models_equal = (len(init_models) == len(expected_arms) * expected_runs and
                        len(set(models)) == 1)
        if not models_equal:
            reasons.append("init/assistant model is missing or differs across expected runs/arms")
        if isinstance(declared_model, str) and declared_model and models_equal and models[0] != declared_model:
            reasons.append("observed init model differs from producer-declared modelOverride")
        model_comparison = {"comparable": models_equal, "equal": models_equal,
                            "observedInit": sorted(set(init_models)),
                            "observedAssistant": sorted(set(assistant_models)),
                            "declaredModelOverride": declared_model}
        if source_cost is None:
            reasons.append("producer aggregate costUsd is missing or malformed")
        cases_out.append({"caseIndex": index, "name": case["name"], "status": "ADMITTED" if not reasons else "NOT_ADMITTED",
                          "expectedArms": list(expected_arms), "expectedRunsPerArm": expected_runs,
                          "declaredRunsPerCaseIgnored": case.get("runsPerCase"),
                          "declaredMaxTurns": max_turns, "ordinaryInventoryComparison": inventory_comparison,
                          "modelComparison": model_comparison,
                          "arms": arm_out, "reasons": reasons})
    return {"format": "cub-scout-trace-admission-v1", "admissionScope": "trace-completeness-only",
            "source": str(source), "sourceSha256": sha256(source_bytes),
            "expectedArms": list(expected_arms), "expectedRunsPerArm": expected_runs,
            "producerCostUsdObservedUnreconciled": source_cost,
            "status": "ADMITTED" if all(c["status"] == "ADMITTED" for c in cases_out) else "NOT_ADMITTED",
            "cases": cases_out,
        "limitations": ["Trace evidence is bounded to explicitly allowed out directories.",
                            "Producer costs are retained as reported and are not independently reconciled.",
                            "Top-level inventory parity does not prove nested descendant parity.",
                            "Init inventory does not prove actual runtime grants; compare grants separately."]}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result", type=Path)
    parser.add_argument("--trace-root", action="append", type=Path, required=True,
                        help="explicit out directory allowed for trace reads (repeatable)")
    parser.add_argument("--expected-arms", required=True,
                        help="caller-supplied comma-separated arm plan, e.g. with,without")
    parser.add_argument("--expected-runs", required=True, type=int,
                        help="caller-supplied run count per arm; result metadata is not authoritative")
    parser.add_argument("--out", required=True, type=Path, help="new output JSON path")
    args = parser.parse_args(argv)
    try:
        output = args.out.resolve()
        source = args.result
        if output == source or output.exists():
            raise ValueError("--out must be a new path different from the source result")
        arms = [item.strip() for item in args.expected_arms.split(",") if item.strip()]
        report = audit(source, args.trace_root, arms, args.expected_runs)
        args.out.parent.mkdir(parents=True, exist_ok=True)
        with args.out.open("x", encoding="utf-8") as stream:
            stream.write(json.dumps(report, indent=2, sort_keys=True) + "\n")
        print(f"{report['status']}: {args.out}")
        return 0 if report["status"] == "ADMITTED" else 2
    except (OSError, ValueError) as exc:
        print("admission audit failed: " + str(exc), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
