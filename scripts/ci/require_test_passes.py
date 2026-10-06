#!/usr/bin/env python3
"""Reject skipped/missing/failed required tests in a complete Go JSON stream."""
import json
import sys


def require_passes(lines, required):
    if not required or len(set(required)) != len(required):
        raise ValueError("required test names must be nonempty and unique")
    outcomes = {}
    packages = {}
    for line in lines:
        event = json.loads(line)
        if not isinstance(event, dict) or not isinstance(event.get("Package"), str):
            raise ValueError("invalid Go test event")
        action = event.get("Action")
        if action in ("pass", "fail", "skip"):
            test = event.get("Test")
            if action == "fail":
                raise ValueError("test stream contains failure")
            if test is None:
                package = event["Package"]
                if package in packages:
                    raise ValueError("duplicate package outcome")
                packages[package] = action
            elif test in required:
                if test in outcomes:
                    raise ValueError("duplicate required test outcome")
                outcomes[test] = (action, event["Package"])
    for test in required:
        outcome, package = outcomes.get(test, (None, None))
        if outcome != "pass" or packages.get(package) != "pass":
            raise ValueError("required test or containing package did not pass: " + test)


def main():
    try:
        with open(sys.argv[1], encoding="utf-8") as stream:
            require_passes(stream, sys.argv[2:])
    except (OSError, ValueError, IndexError):
        print("Required Go test acceptance failed (missing, skipped, failed or invalid stream)", file=sys.stderr)
        return 1
    print("All required Go tests and their packages passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
