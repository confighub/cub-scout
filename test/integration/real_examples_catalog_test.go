//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type realExamplesCatalog struct {
	Version   int                      `yaml:"version"`
	Source    map[string]string        `yaml:"source_docs"`
	Principle []string                 `yaml:"principles"`
	Examples  []realExamplesCatalogRow `yaml:"examples"`
}

type realExamplesCatalogRow struct {
	ID            string   `yaml:"id"`
	Repo          string   `yaml:"repo"`
	Path          string   `yaml:"path"`
	SkeletonID    string   `yaml:"skeleton_id"`
	Required      bool     `yaml:"required"`
	ValueProps    []string `yaml:"value_props"`
	UseCases      []string `yaml:"use_cases"`
	DemoScenarios []string `yaml:"demo_scenarios"`
	RequiredPaths []string `yaml:"required_paths"`
}

func TestRealExamplesCatalog(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	catalogPath := filepath.Join(repoRoot, "test", "examples", "real-examples-catalog.yaml")

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog %s: %v", catalogPath, err)
	}

	var catalog realExamplesCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse catalog %s: %v", catalogPath, err)
	}

	if catalog.Version != 1 {
		t.Fatalf("catalog version = %d, want 1", catalog.Version)
	}
	if len(catalog.Examples) == 0 {
		t.Fatal("catalog has no examples")
	}

	repoRoots := map[string]string{
		"cub-scout":         repoRoot,
		"examples":          filepath.Join(repoRoot, "..", "examples"),
		"examples-internal": filepath.Join(repoRoot, "..", "examples-internal"),
		"confighub/public":  filepath.Join(repoRoot, "..", "confighub", "public"),
	}

	requiredSkeletons := map[string]bool{
		"argo-aoa-mono":    false,
		"argo-appset-mono": false,
		"flux-tenant-mono": false,
		"helm-umbrella":    false,
	}
	requiredScenarios := map[string]bool{
		"Monday Panic":   false,
		"2AM kubectl":    false,
		"Security Patch": false,
	}

	seenIDs := map[string]bool{}
	requiredPresent := 0

	for _, row := range catalog.Examples {
		if row.ID == "" {
			t.Error("catalog row has empty id")
			continue
		}
		if seenIDs[row.ID] {
			t.Errorf("duplicate example id: %s", row.ID)
			continue
		}
		seenIDs[row.ID] = true

		if row.Repo == "" || row.Path == "" {
			t.Errorf("%s missing repo/path", row.ID)
			continue
		}
		if row.SkeletonID == "" {
			t.Errorf("%s missing skeleton_id", row.ID)
		}
		if len(row.ValueProps) == 0 {
			t.Errorf("%s missing value_props", row.ID)
		}
		if len(row.UseCases) == 0 {
			t.Errorf("%s missing use_cases", row.ID)
		}
		if len(row.DemoScenarios) == 0 {
			t.Errorf("%s missing demo_scenarios", row.ID)
		}
		if len(row.RequiredPaths) == 0 {
			t.Errorf("%s missing required_paths", row.ID)
		}

		root, ok := repoRoots[row.Repo]
		if !ok {
			t.Errorf("%s uses unknown repo key: %s", row.ID, row.Repo)
			continue
		}

		exampleDir := filepath.Join(root, row.Path)
		if _, err := os.Stat(exampleDir); err != nil {
			if row.Required {
				t.Errorf("required example missing: %s (%v)", exampleDir, err)
			} else {
				t.Logf("optional example unavailable: %s", exampleDir)
			}
			continue
		}

		if row.Required {
			requiredPresent++
		}

		for _, requiredRel := range row.RequiredPaths {
			requiredAbs := filepath.Join(exampleDir, requiredRel)
			if _, err := os.Stat(requiredAbs); err != nil {
				t.Errorf("%s missing required path: %s", row.ID, requiredAbs)
			}
		}

		if _, ok := requiredSkeletons[row.SkeletonID]; ok {
			requiredSkeletons[row.SkeletonID] = true
		}
		for _, scenario := range row.DemoScenarios {
			if _, ok := requiredScenarios[scenario]; ok {
				requiredScenarios[scenario] = true
			}
		}
	}

	if requiredPresent < 5 {
		t.Errorf("required examples present = %d, want at least 5", requiredPresent)
	}

	for skeleton, covered := range requiredSkeletons {
		if !covered {
			t.Errorf("required skeleton not covered by existing examples: %s", skeleton)
		}
	}
	for scenario, covered := range requiredScenarios {
		if !covered {
			t.Errorf("required demo scenario not represented in catalog: %s", scenario)
		}
	}
}

func mustFindRepoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := cwd
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "test", "examples", "real-examples-catalog.yaml")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not locate repo root from cwd=%s", cwd)
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
