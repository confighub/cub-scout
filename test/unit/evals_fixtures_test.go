// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each eval run starts in an empty workspace; a case's scaffold.sh writes the
// recorded cluster export there with the files embedded. Every case must
// write exactly the recording, so both eval arms and every case see the same
// evidence (#603).
func TestEvalScaffoldsWriteTheRecordedExport(t *testing.T) {
	root := filepath.Join("..", "..", "evals")
	for _, sc := range []struct{ name, export, cases string }{
		{"main", filepath.Join(root, "fixtures", "cluster"), filepath.Join(root, "*", "case.yaml")},
		{"scale", filepath.Join(root, "fixtures", "scale", "cluster"), filepath.Join(root, "scale", "*", "case.yaml")},
	} {
		t.Run(sc.name, func(t *testing.T) { checkScaffolds(t, sc.export, sc.cases) })
	}
}

func checkScaffolds(t *testing.T, export, casesGlob string) {
	t.Helper()
	recorded, err := filepath.Glob(filepath.Join(export, "*.yaml"))
	if err != nil || len(recorded) == 0 {
		t.Fatalf("no recorded export under %s: %v", export, err)
	}
	cases, _ := filepath.Glob(casesGlob)
	if len(cases) == 0 {
		t.Fatalf("no eval cases match %s", casesGlob)
	}
	for _, caseYAML := range cases {
		data, err := os.ReadFile(caseYAML)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "scaffold_script: scaffold.sh") {
			// Live-only cases read the cluster through cub-scout, not an export.
			prompt, _ := os.ReadFile(filepath.Join(filepath.Dir(caseYAML), "prompt.md"))
			if strings.Contains(string(prompt), "live-only") {
				continue
			}
			t.Errorf("%s: every case reads the export through scaffold.sh", caseYAML)
			continue
		}
		script, err := os.ReadFile(filepath.Join(filepath.Dir(caseYAML), "scaffold.sh"))
		if err != nil {
			t.Errorf("%s: %v", caseYAML, err)
			continue
		}
		written := scaffoldFiles(string(script))
		for _, src := range recorded {
			want, _ := os.ReadFile(src)
			name := filepath.Base(src)
			if got, ok := written[name]; !ok || got != strings.TrimRight(string(want), "\n") {
				t.Errorf("%s/scaffold.sh does not write the recorded %s; run evals/scripts/record.py --scaffolds-only (with --scenario scale for evals/scale)", filepath.Dir(caseYAML), name)
			}
		}
	}
}

// scaffoldFiles returns the files a generated scaffold.sh writes, by name.
func scaffoldFiles(script string) map[string]string {
	const delim = "CUB_SCOUT_EVAL_EOF"
	files := map[string]string{}
	lines := strings.Split(script, "\n")
	for i := 0; i < len(lines); i++ {
		name, ok := strings.CutPrefix(lines[i], "cat > cluster/")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(name, " ")
		var body []string
		for i++; i < len(lines) && lines[i] != delim; i++ {
			body = append(body, lines[i])
		}
		files[name] = strings.Join(body, "\n")
	}
	return files
}
