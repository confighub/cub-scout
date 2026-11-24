#!/usr/bin/env python3
"""Deterministic bytes-and-tokens benchmark over the 24 prepared eval cases.

Package C / Measurement rung 1 of docs/roadmap-3.0-execution.md:
- Zero model calls, zero credits, zero network, zero cluster access.
- Measures exact UTF-8 byte size of the raw recorded evidence materialized for
  each of the 24 frozen cases in evals/benchmark-v1.json.
- Compares against the declared cub-scout `answer` block byte ceiling for the
  primary tool answering each case (from cmd/cub-scout/answer_size_ceilings_test.go).
- Uses the deterministic token approximation ceil(bytes / 4), matching
  cmd/cub-scout/mcp_catalog_size_test.go ("four bytes is roughly one token").

This measures evidence size only. Smaller evidence does not by itself prove
that an agent answers correctly for less money.
"""

import argparse
import json
import math
import pathlib
import subprocess
import sys
import tempfile

# Declared `answer` block byte ceilings (compact JSON) from
# cmd/cub-scout/answer_size_ceilings_test.go.
ANSWER_BLOCK_CEILINGS = {
    "compare_source_truth": 400,
    "doctor": 500,
    "explain": 900,
    "gitops_settings": 1000,
    "gitops_status": 900,
    "map": 400,
    "release_check": 1600,
    "scan": 700,
    "trace": 900,
}

# Primary cub-scout tool for each of the 24 frozen cases in evals/benchmark-v1.json.
CASE_PRIMARY_TOOL = {
    "INV-01": "map",
    "INV-02": "map",
    "INV-03": "explain",
    "INV-04": "map",
    "ATR-01": "explain",
    "ATR-02": "explain",
    "ATR-03": "explain",
    "ATR-04": "trace",
    "DEL-01": "trace",
    "DEL-02": "compare_source_truth",
    "DEL-03": "trace",
    "DEL-04": "release_check",
    "HLT-01": "gitops_status",
    "HLT-02": "gitops_status",
    "HLT-03": "doctor",
    "HLT-04": "gitops_status",
    "PRE-01": "doctor",
    "PRE-02": "doctor",
    "PRE-03": "trace",
    "PRE-04": "map",
    "RUL-01": "explain",
    "RUL-02": "explain",
    "RUL-03": "map",
    "RUL-04": "release_check",
}

# Measured MCP catalog sizes from cmd/cub-scout/mcp_catalog_size_test.go.
MCP_CATALOG_BYTES = {
    "standalone_all": 15218,
    "standalone_core": 9390,
    "connected_all": 31224,
    "connected_core": 12337,
}


def tokens_from_bytes(n: int) -> int:
    return (n + 3) // 4


def measure_raw_evidence(case_dir: pathlib.Path) -> tuple[int, int, str]:
    """Return (total_bytes, file_count, source_kind) for one case's raw evidence."""
    scaffold = case_dir / "scaffold.sh"
    if scaffold.is_file():
        with tempfile.TemporaryDirectory(prefix="cub-scout-bytes-") as tmp:
            proc = subprocess.run(
                ["bash", str(scaffold.resolve())],
                cwd=tmp,
                capture_output=True,
                text=True,
                check=False,
            )
            if proc.returncode != 0:
                raise RuntimeError(
                    f"scaffold.sh failed for {case_dir}: {proc.stderr.strip()}"
                )
            root = pathlib.Path(tmp)
            files = [p for p in root.rglob("*") if p.is_file()]
            total = sum(p.stat().st_size for p in files)
            return total, len(files), "scaffold.sh"

    fixtures = case_dir / "fixtures"
    if fixtures.is_dir():
        files = [p for p in fixtures.rglob("*") if p.is_file()]
        total = sum(p.stat().st_size for p in files)
        return total, len(files), "fixtures/"

    raise RuntimeError(f"case {case_dir} has neither scaffold.sh nor fixtures/")


def measure_all(repo_root: pathlib.Path) -> dict:
    manifest_path = repo_root / "evals" / "benchmark-v1.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

    rows = []
    group_totals = {}
    for group in manifest["groups"]:
        gid = group["id"]
        group_totals[gid] = {
            "cases": 0,
            "raw_bytes": 0,
            "raw_tokens": 0,
            "answer_ceiling_bytes": 0,
            "answer_ceiling_tokens": 0,
        }
        for case in group["cases"]:
            cid = case["id"]
            case_dir = repo_root / case["existing_case"]
            raw_bytes, file_count, source_kind = measure_raw_evidence(case_dir)
            tool = CASE_PRIMARY_TOOL[cid]
            ans_bytes = ANSWER_BLOCK_CEILINGS[tool]
            raw_tok = tokens_from_bytes(raw_bytes)
            ans_tok = tokens_from_bytes(ans_bytes)
            ratio_pct = round((ans_bytes / raw_bytes) * 100.0, 2) if raw_bytes else 0.0
            rows.append(
                {
                    "id": cid,
                    "group": gid,
                    "question": case["question"],
                    "source": source_kind,
                    "files": file_count,
                    "raw_bytes": raw_bytes,
                    "raw_tokens": raw_tok,
                    "tool": tool,
                    "answer_ceiling_bytes": ans_bytes,
                    "answer_ceiling_tokens": ans_tok,
                    "answer_pct_of_raw": ratio_pct,
                }
            )
            gt = group_totals[gid]
            gt["cases"] += 1
            gt["raw_bytes"] += raw_bytes
            gt["raw_tokens"] += raw_tok
            gt["answer_ceiling_bytes"] += ans_bytes
            gt["answer_ceiling_tokens"] += ans_tok

    total_raw_bytes = sum(r["raw_bytes"] for r in rows)
    total_raw_tokens = sum(r["raw_tokens"] for r in rows)
    total_ans_bytes = sum(r["answer_ceiling_bytes"] for r in rows)
    total_ans_tokens = sum(r["answer_ceiling_tokens"] for r in rows)

    return {
        "schema": "cub-scout-bytes-tokens-benchmark.v1",
        "method": {
            "token_formula": "ceil(utf8_bytes / 4)",
            "model_calls": 0,
            "credits_spent": 0,
            "caveat": "Measures recorded evidence bytes and declared answer-block ceilings only; does not prove agent accuracy or dollar savings.",
        },
        "mcp_catalog_bytes": MCP_CATALOG_BYTES,
        "cases": rows,
        "groups": group_totals,
        "totals": {
            "cases": len(rows),
            "raw_bytes": total_raw_bytes,
            "raw_tokens": total_raw_tokens,
            "answer_ceiling_bytes": total_ans_bytes,
            "answer_ceiling_tokens": total_ans_tokens,
            "answer_pct_of_raw": round((total_ans_bytes / total_raw_bytes) * 100.0, 2)
            if total_raw_bytes
            else 0.0,
        },
    }


def render_markdown(report: dict) -> str:
    lines = [
        "| Case | Group | Raw files | Raw bytes | Raw ~tokens | Primary tool | Answer ceiling (bytes / ~tokens) | Answer % of raw |",
        "|---|---|---:|---:|---:|---|---:|---:|",
    ]
    for r in report["cases"]:
        lines.append(
            f"| {r['id']} | {r['group']} | {r['files']} | {r['raw_bytes']:,} | {r['raw_tokens']:,} "
            f"| `{r['tool']}` | {r['answer_ceiling_bytes']:,} / {r['answer_ceiling_tokens']:,} "
            f"| {r['answer_pct_of_raw']:.2f}% |"
        )
    tot = report["totals"]
    lines.append(
        f"| **Total (24 cases)** | **6 groups** | — | **{tot['raw_bytes']:,}** | **{tot['raw_tokens']:,}** "
        f"| — | **{tot['answer_ceiling_bytes']:,} / {tot['answer_ceiling_tokens']:,}** "
        f"| **{tot['answer_pct_of_raw']:.2f}%** |"
    )
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--format", choices=("md", "json"), default="md")
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=pathlib.Path(__file__).resolve().parents[2],
    )
    args = parser.parse_args()
    report = measure_all(args.repo_root)
    if args.format == "json":
        print(json.dumps(report, indent=2))
    else:
        print(render_markdown(report), end="")
    return 0


if __name__ == "__main__":
    sys.exit(main())
