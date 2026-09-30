// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestChangedByCheckoutRequiresExactRecordedEvidenceContract(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	cmd := exec.Command("python3", "-c", `
import pathlib, sys
sys.path.insert(0, "evals/scripts")
import regrade

prompt = pathlib.Path("evals/changed-by-checkout/prompt.md").read_text().split("\n---\n", 1)[1]
assert "kubectl-set" not in prompt, "prompt leaks the winning manager"
assert "MANUAL_TOOL" in prompt and "HUMAN_ACTOR" in prompt, "prompt omits answer schema semantics"

deployment_export = pathlib.Path("evals/fixtures/cluster/deployments.yaml").read_text()
field_evidence = '''fieldsV1:
        f:spec:
          f:template:
            f:spec:
              f:containers:
                k:{"name":"checkout"}:
                  f:image: {}
      manager: kubectl-set'''
assert field_evidence in deployment_export, "fixture no longer ties kubectl-set to checkout's image field"

grader_path = pathlib.Path("evals/changed-by-checkout/graders/changed-by-line.md")
definition = regrade.parse_grader(grader_path, grader_path.read_bytes())
assert definition["kind"] == "regex"
pattern = definition["compiled"]
good = "CHANGED_BY: MANUAL_TOOL | MANAGER: kubectl-set | FIELD_PATH: spec.template.spec.containers[name=checkout].image | HUMAN_ACTOR: UNKNOWN | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided"
assert pattern.search(good), "regrader rejected the exact source-bounded answer"
tick = chr(96)
assert pattern.search(tick + good + tick), "regrader rejected a whole-line inline-code wrapper"

# These are the unsupported claims recorded in the first fair smoke. The
# grader anchors the entire last message, so no arbitrary surrounding prose
# can be scored as a verified answer in this case.
smoke_with = "Independent read: cub-scout explain says this live cluster still matches the snapshot.\n" + good
smoke_without = "Flux's Git source still declares pause:3.9.\n" + good
for answer in (smoke_with, smoke_without, good + "\nAdditional explanation."):
    assert not pattern.search(answer), "accepted answer outside the narrow contract: " + answer

# This complete fixture contains the field evidence. A manager-only or UNKNOWN
# response is therefore not a verified success for this case.
wrong = [
    good.replace("FIELD_PATH: spec.template.spec.containers[name=checkout].image", "FIELD_PATH: UNKNOWN"),
    good.replace("CHANGED_BY: MANUAL_TOOL", "CHANGED_BY: CONTROLLER"),
    good.replace("MANAGER: kubectl-set", "MANAGER: kustomize-controller"),
    good.replace("HUMAN_ACTOR: UNKNOWN", "HUMAN_ACTOR: IDENTIFIED"),
    good.replace("SCOPE: recorded evidence only; no live confirmation; no Git desired state provided", "SCOPE: live cluster confirmed; Git desired state available"),
    good.replace("CHANGED_BY: MANUAL_TOOL", "CHANGED_BY: UNKNOWN"),
    tick + good,
    good + tick,
    tick + tick + good + tick + tick,
    "Answer: " + tick + good + tick,
]
for answer in wrong:
    assert not pattern.search(answer), "accepted an unsupported/unknown attribution: " + answer
`)
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("changed-by-checkout grader contract: %v\n%s", err, out)
	}
}
