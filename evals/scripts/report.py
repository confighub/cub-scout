#!/usr/bin/env python3
"""Summarise `claude plugin eval --json` results: correctness, cost and speed.

Usage: evals/scripts/report.py RESULT.json [RESULT.json ...] [--by-tag] [--require-complete]

Prints the legacy harness score and cost tables as diagnostics, plus a binary
verified-answer table based on the grader definitions embedded in each result.
The legacy $/score figure is not a correctness rate.
Several result files are combined, so separate runs of different cases can be
reported together. `costUsd` is inclusive of agent, judge, and mock costs.
Optional judge/mock breakdowns are validated against it, never added again.

The legacy $/score figure is total cost divided by total harness score. It is
retained for continuity and diagnostic comparison; binary verified answers
are reported separately.

Runs that ended in an error (a timeout, the turn cap) are counted, and listed
under the table, because their cost was really spent.
"""
import json
import math
import os
import re
import sys
from collections import defaultdict

ARMS = ("with", "without")


def cost_value(value):
    finite = not isinstance(value, float) or math.isfinite(value)
    return (isinstance(value, (int, float)) and not isinstance(value, bool) and
            finite and value >= 0)


def run_cost(run):
    """Return inclusive costUsd; validate optional breakdowns without double-counting."""
    if not isinstance(run, dict) or "costUsd" not in run or not cost_value(run["costUsd"]):
        return None
    total = run["costUsd"]
    breakdown = 0
    if "judgeCostUsd" in run:
        if not cost_value(run["judgeCostUsd"]):
            return None
        breakdown += run["judgeCostUsd"]
    if "mocks" in run:
        mocks = run["mocks"]
        if not isinstance(mocks, dict) or not isinstance(mocks.get("calls"), dict):
            return None
        mock_cost = mocks["calls"].get("costUsd")
        if not cost_value(mock_cost):
            return None
        breakdown += mock_cost
    if breakdown > total:
        return None
    return total


def case_tags(case):
    """Tags from the case's prompt.md frontmatter (the result JSON holds only
    the prompt body). `dir` is relative to the plugin root, so run from there."""
    try:
        text = open(os.path.join(case.get("dir") or "", "prompt.md")).read()
    except OSError:
        return []
    m = re.search(r"^tags:\s*\[(.*?)\]\s*$", text, re.M)
    return [t.strip() for t in m.group(1).split(",") if t.strip()] if m else []


def load(paths):
    cases = {}
    inputs = []
    for path in paths:
        result = json.load(open(path))
        file_cases = result.get("cases") or []
        ablation = (result.get("suite") or {}).get("ablation")
        expected_arms = ARMS if ablation == "with-without" else ("with",) if ablation == "none" else None
        inputs.append({
            "path": path,
            "partial": result.get("partial"),
            "reason": result.get("partialReason"),
            "cases": file_cases,
            "expected_arms": expected_arms,
        })
        for case in file_cases:
            definitions = grader_definitions(case)
            entry = cases.setdefault(case["name"], {"tags": case_tags(case), "runs": defaultdict(list)})
            for arm in ARMS:
                for run in (case.get("arms") or {}).get(arm) or []:
                    run["_grader_definitions"] = definitions
                    entry["runs"][arm].append(run)
    return cases, inputs


def completeness(inputs):
    diagnostics = []
    incomplete = False
    unknown = False
    for source in inputs:
        path = source["path"]
        if source["partial"] is True:
            incomplete = True
            detail = "partial=true"
            if source["reason"]:
                detail += " reason=" + str(source["reason"])
            diagnostics.append(("INPUT", path, detail))
        elif source["partial"] is False:
            diagnostics.append(("INPUT", path, "partial=false"))
        else:
            unknown = True
            diagnostics.append(("UNKNOWN", path, "partial flag missing"))

        expected_arms = source["expected_arms"]
        if expected_arms is None:
            unknown = True
            diagnostics.append(("UNKNOWN", path, "suite ablation does not identify expected arms"))
            continue
        if not source["cases"]:
            unknown = True
            diagnostics.append(("UNKNOWN", path, "no case run metadata"))
            continue
        for case in source["cases"]:
            planned = case.get("runsPerCase")
            if type(planned) is not int or planned <= 0:
                unknown = True
                diagnostics.append(("UNKNOWN", path, "%s: planned run count missing" % case.get("name", "<unnamed>")))
                continue
            arms = case.get("arms") or {}
            for arm in expected_arms:
                observed = len(arms.get(arm) or [])
                diagnostics.append(("RUNS", path, "%s (%s): %d planned, %d observed" %
                                    (case.get("name", "<unnamed>"), arm, planned, observed)))
                if observed < planned:
                    incomplete = True
                    diagnostics.append(("INCOMPLETE", path, "%s (%s): expected %d, observed %d" %
                                        (case.get("name", "<unnamed>"), arm, planned, observed)))
    status = "INCOMPLETE" if incomplete else "UNKNOWN" if unknown else "COMPLETE"
    return status, diagnostics


def summarise(runs):
    n = len(runs)
    if n == 0:
        return None
    score = sum(r.get("score") or 0 for r in runs)
    run_costs = [run_cost(r) for r in runs]
    costs_known = all(cost is not None for cost in run_costs)
    cost = sum(run_costs) / n if costs_known else None
    total_cost = sum(run_costs) if costs_known else None
    return {
        "runs": n,
        "score": score / n,
        "cost": cost,
        "per_score": total_cost / score if costs_known and score > 0 else None,
        "turns": sum(r.get("turns") or 0 for r in runs) / n,
        "seconds": sum(r.get("durationSeconds") or 0 for r in runs) / n,
        "errors": sum(1 for r in runs if r.get("error")),
    }


def grader_definitions(case):
    """Return correctness grader definitions embedded in the result, if present."""
    for key in ("graderDefinitions", "graders"):
        definitions = case.get(key)
        if isinstance(definitions, list) and definitions and all(isinstance(g, dict) and "type" in g for g in definitions):
            return definitions
    return None


def verified_answer(run, definitions):
    """Return True/False for a binary answer check, or None when unprovable."""
    if run.get("error"):
        return False
    if definitions is None or not isinstance(run.get("graders"), list):
        return None
    if run.get("skippedPaidGraders") is True:
        return None
    required = {}
    for definition in definitions:
        if not isinstance(definition, dict) or not isinstance(definition.get("type"), str):
            return None
        if definition["type"] == "tool_used":
            continue
        weight = definition.get("weight", 1)
        if (isinstance(weight, bool) or not isinstance(weight, (int, float)) or
                isinstance(weight, float) and not math.isfinite(weight)):
            return None
        if weight > 0:
            name = definition.get("name")
            if not isinstance(name, str) or not name or name in required:
                return None
            required[name] = True
    if not required:
        return None
    by_name = defaultdict(list)
    for result in run["graders"]:
        if isinstance(result, dict) and isinstance(result.get("name"), str):
            by_name[result["name"]].append(result)
    verified = True
    for name in required:
        matches = by_name.get(name) or []
        if len(matches) != 1 or type(matches[0].get("passed")) is not bool:
            return None
        verified = verified and matches[0]["passed"]
    return verified


def verified_summary(runs):
    statuses = [verified_answer(run, run.get("_grader_definitions")) for run in runs]
    known = [status for status in statuses if status is not None]
    verified = sum(status is True for status in known)
    run_costs = [run_cost(run) for run in runs]
    unknown_cost = sum(cost is None for cost in run_costs)
    cost = sum(cost for cost in run_costs if cost is not None) if unknown_cost == 0 else None
    unknown = len(statuses) - len(known)
    per_verified = None if unknown or unknown_cost or not verified else cost / verified
    return {"runs": len(runs), "verified": verified, "known": len(known),
            "unknown": unknown, "unknown_cost": unknown_cost, "cost": cost,
            "per_verified": per_verified}


def verified_table(rows):
    out = ["| Case/group | Arm | Runs | Verified | Unknown answer | Unknown cost | $/run | $/verified |",
           "|---|---|---:|---:|---:|---:|---:|---:|"]
    for label, by_arm in rows:
        for arm in ARMS:
            s = by_arm.get(arm)
            if s is None:
                out.append("| %s | %s | 0 | 0 | 0 | 0 | n/a | n/a |" % (label if arm == "with" else "", arm))
                continue
            per = "UNKNOWN" if s["unknown"] or s["unknown_cost"] else "n/a" if s["per_verified"] is None else "$%.2f" % s["per_verified"]
            cost_per_run = "UNKNOWN" if s["unknown_cost"] else "n/a" if not s["runs"] else "$%.2f" % (s["cost"] / s["runs"])
            out.append("| %s | %s | %d | %d/%d | %d | %d | %s | %s |" %
                       (label if arm == "with" else "", arm, s["runs"], s["verified"],
                        s["known"], s["unknown"], s["unknown_cost"], cost_per_run, per))
    return "\n".join(out)


def fmt(s):
    if s is None:
        return "| – | – | – | – | – | – |"
    per = "n/a" if s["per_score"] is None else "$%.2f" % s["per_score"]
    cost = "UNKNOWN" if s["cost"] is None else "$%.2f" % s["cost"]
    return "| %d | %.2f | %s | %s | %.1f | %.0f |" % (s["runs"], s["score"], cost, per, s["turns"], s["seconds"])


HEADER = "| {label} | Arm | Runs | Score | $/run | $/score | Turns | Seconds |\n|---|---|---|---|---|---|---|---|"


def table(title, rows):
    out = [HEADER.format(label=title)]
    for label, by_arm in rows:
        for arm in ARMS:
            name = label if arm == "with" else ""
            out.append("| %s | %s %s" % (name, arm, fmt(by_arm.get(arm))))
    return "\n".join(out)


def main(argv):
    by_tag = "--by-tag" in argv
    require_complete = "--require-complete" in argv
    paths = [a for a in argv if a not in ("--by-tag", "--require-complete")]
    if not paths:
        raise SystemExit(__doc__)
    cases, inputs = load(paths)
    status, diagnostics = completeness(inputs)
    print("Completeness: " + status)
    for level, path, detail in diagnostics:
        print("- %s: %s — %s" % (level, path, detail))
    print()

    rows = [(name, {arm: summarise(c["runs"][arm]) for arm in ARMS}) for name, c in sorted(cases.items())]
    print(table("Case", rows))
    print()

    groups = [("All cases", list(cases.values()))]
    if by_tag:
        tags = sorted({t for c in cases.values() for t in c["tags"]})
        groups += [("tag: " + t, [c for c in cases.values() if t in c["tags"]]) for t in tags]
    totals = [(label, {arm: summarise([r for c in members for r in c["runs"][arm]]) for arm in ARMS})
              for label, members in groups]
    print(table("Group", totals))

    verified_rows = [(name, {arm: verified_summary(c["runs"][arm]) if c["runs"][arm] else None
                             for arm in ARMS}) for name, c in sorted(cases.items())]
    verified_rows += [(label, {arm: verified_summary([r for c in members for r in c["runs"][arm]])
                                if any(c["runs"][arm] for c in members) else None
                                for arm in ARMS}) for label, members in groups]
    print("\nBinary verified answers (from each result's embedded grader definitions; positive-weight non-tool_used checks only):")
    print(verified_table(verified_rows))

    errored = [(name, arm, r.get("error")) for name, c in sorted(cases.items())
               for arm in ARMS for r in c["runs"][arm] if r.get("error")]
    if errored:
        print()
        print("Runs that ended in an error (counted above):")
        for name, arm, err in errored:
            print("- %s (%s): %s" % (name, arm, err))

    if require_complete and status != "COMPLETE":
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
