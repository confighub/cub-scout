// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalRecorderExportsManagedFieldsFromNamedContext(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(tmp, "kubectl.log")
	fakeKubectl := filepath.Join(binDir, "kubectl")
	fake := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$KUBECTL_LOG"
if [ "$1" = config ]; then
	printf 'apiVersion: v1\nkind: Config\ncurrent-context: named-context\n'
	exit 0
fi
if [ "$1" = get ]; then
	[ -f "$KUBECONFIG" ] || { echo "missing minified KUBECONFIG" >&2; exit 41; }
	grep -q 'current-context: named-context' "$KUBECONFIG" || { echo "wrong KUBECONFIG" >&2; exit 42; }
	cat <<EOF
apiVersion: v1
kind: $2
metadata:
  managedFields:
  - manager: fake-recorder
    operation: Apply
EOF
	exit 0
fi
echo "unexpected kubectl invocation: $*" >&2
exit 43
`
	if err := os.WriteFile(fakeKubectl, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	fixtures := filepath.Join(tmp, "fixtures")
	caseDir := filepath.Join(tmp, "case")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte("context:\n  scaffold_script: scaffold.sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	call := `import sys
sys.path.insert(0, sys.argv[1])
import record
record.record("named-context", {"fixtures": sys.argv[2], "cases": sys.argv[3], "live": True}, "unused-cub-scout")
`
	cmd := exec.Command(python, "-c", call, filepath.Join(root, "evals", "scripts"), fixtures, filepath.Join(tmp, "*", "case.yaml"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"KUBECTL_LOG="+logPath,
		"PYTHONDONTWRITEBYTECODE=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("record.py failed: %v\n%s", err, out)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	kinds := []string{"namespaces", "deployments", "replicasets", "pods", "services", "configmaps", "events"}
	if len(lines) != 1+len(kinds) {
		t.Fatalf("unexpected kubectl calls: %s", log)
	}
	if !strings.Contains(lines[0], "config view --minify --flatten --context named-context") {
		t.Errorf("did not minify the named context: %q", lines[0])
	}
	if strings.Contains(string(log), "config use-context") {
		t.Errorf("recorder switched the global kubectl context: %s", log)
	}
	for i, kind := range kinds {
		line := lines[i+1]
		if !strings.Contains(line, "get "+kind+" -A -o yaml") || !strings.Contains(line, "--show-managed-fields") {
			t.Errorf("%s dump did not request managedFields: %q", kind, line)
		}
		data, err := os.ReadFile(filepath.Join(fixtures, "cluster", kind+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "manager: fake-recorder") {
			t.Errorf("%s dump lost managedFields content", kind)
		}
	}
	scaffold, err := os.ReadFile(filepath.Join(caseDir, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scaffold), "manager: fake-recorder") {
		t.Error("generated scaffold does not preserve managedFields content")
	}
}
