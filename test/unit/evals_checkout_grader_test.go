// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
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

definition = regrade.parse_grader(pathlib.Path("evals/changed-by-checkout/graders/changed-by-line.md"))
assert definition["kind"] == "regex"
pattern = definition["compiled"]
good = "CHANGED_BY: kubectl-set manager supports kubectl set image, not a human identity | MANAGER: metadata.managedFields kubectl-set | FIELD_PATH: metadata.managedFields fieldsV1 spec.template.spec.containers[name=checkout].image | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided"
assert pattern.search(good), "regrader rejected the exact source-bounded answer"

# These are the unsupported claims recorded in the first fair smoke. The
# grader anchors the entire last message, so no arbitrary surrounding prose
# can be scored as a verified answer in this case.
smoke_with = "Independent read: cub-scout explain says this live cluster still matches the snapshot.\\n" + good
smoke_without = "Flux's Git source still declares pause:3.9.\\n" + good
for answer in (smoke_with, smoke_without, good + "\\nAdditional explanation."):
    assert not pattern.search(answer), "accepted answer outside the narrow contract: " + answer

# This complete fixture contains the field evidence. A manager-only or UNKNOWN
# response is therefore not a verified success for this case.
wrong = [
    good.replace("FIELD_PATH: metadata.managedFields fieldsV1 spec.template.spec.containers[name=checkout].image", "FIELD_PATH: UNKNOWN"),
    good.replace("CHANGED_BY: kubectl-set manager supports kubectl set image, not a human identity", "CHANGED_BY: UNKNOWN"),
]
for answer in wrong:
    assert not pattern.search(answer), "accepted an unsupported/unknown attribution: " + answer
`)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("changed-by-checkout grader contract: %v\n%s", err, out)
	}
}
