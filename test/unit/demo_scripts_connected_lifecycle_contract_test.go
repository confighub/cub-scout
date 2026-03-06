package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgoFluxDemoScripts_UseWorkerLifecycleHelper(t *testing.T) {
	scripts := []string{
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "demo.sh"),
	}
	for _, path := range scripts {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		script := string(content)
		if !strings.Contains(script, "demo-worker-lifecycle.sh") {
			t.Fatalf("%s must invoke demo-worker-lifecycle.sh for discovery worker lifecycle", path)
		}
		if !strings.Contains(script, "--pid-file") {
			t.Fatalf("%s must manage worker pid file for cleanup/keep behavior", path)
		}
	}
}

func TestArgoFluxDemoScripts_ExposeSeedHistoryFlag(t *testing.T) {
	scripts := []string{
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "demo.sh"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "demo.sh"),
	}
	for _, path := range scripts {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		script := string(content)
		required := []string{
			"--seed-history",
			"seed-connected-demo-history.sh",
			"--allow-synthetic",
		}
		for _, needle := range required {
			if !strings.Contains(script, needle) {
				t.Fatalf("%s missing %q integration", path, needle)
			}
		}
	}
}
