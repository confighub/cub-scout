#!/usr/bin/env python3
"""Summarise `claude plugin eval --json` results: correctness, cost and speed.

Usage: evals/scripts/report.py RESULT.json [RESULT.json ...] [--by-tag]

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
    for path in paths:
        for case in json.load(open(path))["cases"]:
            entry = cases.setdefault(case["name"], {"tags": case_tags(case), "runs": defaultdict(list)})
            for arm in ARMS:
                for run in (case.get("arms") or {}).get(arm) or []:
                    entry["runs"][arm].append(run)
    return cases


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
    paths = [a for a in argv if a != "--by-tag"]
    if not paths:
        raise SystemExit(__doc__)
    cases = load(paths)

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


if __name__ == "__main__":
    main(sys.argv[1:])
