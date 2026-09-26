// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every eval case reads its own copy of the recorded cluster export (add_dirs
// must be inside the case directory). The copies must match the recording, so
// both eval arms and every case see the same evidence (#603).
func TestEvalCaseClusterCopiesMatchRecording(t *testing.T) {
	root := filepath.Join("..", "..", "evals")
	recorded, err := filepath.Glob(filepath.Join(root, "fixtures", "cluster", "*.yaml"))
	if err != nil || len(recorded) == 0 {
		t.Fatalf("no recorded export under evals/fixtures/cluster: %v", err)
	}
	cases, _ := filepath.Glob(filepath.Join(root, "*", "case.yaml"))
	if len(cases) == 0 {
		t.Fatal("no eval cases found")
	}
	for _, caseYAML := range cases {
		data, err := os.ReadFile(caseYAML)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "add_dirs: [cluster]") {
			continue
		}
		dir := filepath.Dir(caseYAML)
		for _, src := range recorded {
			want, _ := os.ReadFile(src)
			got, err := os.ReadFile(filepath.Join(dir, "cluster", filepath.Base(src)))
			if err != nil || string(got) != string(want) {
				t.Errorf("%s/cluster/%s differs from the recording; re-run evals/scripts/record.py", dir, filepath.Base(src))
			}
		}
	}
}
