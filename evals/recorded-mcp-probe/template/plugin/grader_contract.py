#!/usr/bin/env python3
"""Run positive and negative response checks in Python and JS regex engines."""
import json
import os
from pathlib import Path
import re
import subprocess

root = Path(__file__).resolve().parent
grader = (root / "evals/recorded-explain-mcp/graders/answer.md").read_text()
pattern = grader.split("pattern: '", 1)[1].split("'", 1)[0]
field_path = '.spec.template.spec.containers[name="checkout"].image'
base = {"resourceOwner": "Flux", "fieldPath": field_path,
        "fieldManagers": ["kubectl-set"],
        "recordedInputSha256": "305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8",
        "resourceRead": None}

def compact(obj):
    return json.dumps(obj, separators=(",", ":"))

cases = [
    ("pretty", json.dumps(base, indent=2), True),
    ("compact", compact(base), True),
    ("outer-whitespace", " \n" + json.dumps(base, indent=2) + "\n ", True),
    ("wrong-resource-owner", compact({**base, "resourceOwner": "kubectl-set"}), False),
    ("wrong-field-manager", compact({**base, "fieldManagers": ["helm"]}), False),
    ("wrong-path", compact({**base, "fieldPath": field_path.replace("].image", "]ximage")}), False),
    ("wrong-hash", compact({**base, "recordedInputSha256": "0" * 64}), False),
    ("live-read-claim", compact({**base, "resourceRead": {"live": True}}), False),
    ("extra-key", compact({**base, "extra": True}), False),
    ("duplicate-key", compact(base)[:-1] + ',"resourceOwner":"Flux"}', False),
    ("reordered", compact(dict(reversed(list(base.items())))), True),
    ("unmatched-fence", "```json\n" + compact(base), False),
    ("prose", "Answer: " + compact(base), False),
    ("fenced", "```json\n" + json.dumps(base, indent=2) + "\n```", True),
]
for name, text, want in cases:
    assert bool(re.search(pattern, text, re.S)) == want, f"Python regex case failed: {name}"

js = r'''const x=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=new RegExp(x.pattern,"s");for(const [n,v,w] of x.cases){if(r.test(v)!==w)throw Error(n)};console.log("JS regex cases passed: "+x.cases.length);'''
payload = json.dumps({"pattern": pattern, "cases": cases})
node_bin = os.environ.get("NODE_BIN")
if not node_bin or not Path(node_bin).is_file():
    raise RuntimeError("prepare.py must provide the resolved NODE_BIN path")
subprocess.run([node_bin, "-e", js], input=payload, text=True, check=True, timeout=10,
               env={"PATH": "/usr/bin:/bin"})
print(f"Python regex cases passed: {len(cases)}")
