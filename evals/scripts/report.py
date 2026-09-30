#!/usr/bin/env python3
"""Summarise `claude plugin eval --json` results: correctness, cost and speed.

Usage: evals/scripts/report.py RESULT.json [RESULT.json ...] [--by-tag] [--require-complete]

Prints a Markdown table per case and arm (with cub-scout / without), then the
totals: mean score, cost per run, cost per correct answer, turns and seconds.
Several result files are combined, so separate runs of different cases can be
reported together. Cost per run is everything the run spent: the agent, any
judge graders and the agent mocks.

Cost per correct answer is total cost divided by total score, so a run that
scores 0.5 counts as half a correct answer. It is the fairest single cost
measure when one arm answers more often: an arm that is cheap per run but
rarely right is expensive per correct answer. It is "n/a" when an arm never
scored.

Runs that ended in an error (a timeout, the turn cap) are counted, and listed
under the table, because their cost was really spent.
"""
import json
import os
import re
import sys
from collections import defaultdict

ARMS = ("with", "without")


def run_cost(run):
    mocks = ((run.get("mocks") or {}).get("calls") or {}).get("costUsd") or 0
    return (run.get("costUsd") or 0) + (run.get("judgeCostUsd") or 0) + mocks


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
            entry = cases.setdefault(case["name"], {"tags": case_tags(case), "runs": defaultdict(list)})
            for arm in ARMS:
                for run in (case.get("arms") or {}).get(arm) or []:
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
    cost = sum(run_cost(r) for r in runs)
    return {
        "runs": n,
        "score": score / n,
        "cost": cost / n,
        "per_correct": cost / score if score > 0 else None,
        "turns": sum(r.get("turns") or 0 for r in runs) / n,
        "seconds": sum(r.get("durationSeconds") or 0 for r in runs) / n,
        "errors": sum(1 for r in runs if r.get("error")),
    }


def fmt(s):
    if s is None:
        return "| – | – | – | – | – | – |"
    per = "n/a" if s["per_correct"] is None else "$%.2f" % s["per_correct"]
    return "| %d | %.2f | $%.2f | %s | %.1f | %.0f |" % (s["runs"], s["score"], s["cost"], per, s["turns"], s["seconds"])


HEADER = "| {label} | Arm | Runs | Score | $/run | $/correct | Turns | Seconds |\n|---|---|---|---|---|---|---|---|"


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
