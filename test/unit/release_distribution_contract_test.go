package unit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type goreleaserConfig struct {
	Builds []struct {
		ID     string   `yaml:"id"`
		Goos   []string `yaml:"goos"`
		Goarch []string `yaml:"goarch"`
	} `yaml:"builds"`
	Archives []struct {
		ID              string `yaml:"id"`
		Formats         []string
		FormatOverrides []struct {
			Goos    string   `yaml:"goos"`
			Format  string   `yaml:"format"`
			Formats []string `yaml:"formats"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
	Brews []struct {
		Name string `yaml:"name"`
	} `yaml:"brews"`
	Dockers []struct {
		ImageTemplates []string `yaml:"image_templates"`
	} `yaml:"dockers"`
}

func TestGoReleaser_DistributionTargets(t *testing.T) {
	cfgPath := filepath.Join("..", "..", ".goreleaser.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read goreleaser config: %v", err)
	}

	var cfg goreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse goreleaser config: %v", err)
	}

	requiredBuilds := map[string]bool{
		"cub-scout":         false,
		"kubectl-cub-scout": false,
	}
	for _, b := range cfg.Builds {
		if _, ok := requiredBuilds[b.ID]; !ok {
			continue
		}
		requiredBuilds[b.ID] = true
		for _, requiredOS := range []string{"linux", "darwin", "windows"} {
			if !slices.Contains(b.Goos, requiredOS) {
				t.Fatalf("build %q missing goos %q", b.ID, requiredOS)
			}
		}
		for _, requiredArch := range []string{"amd64", "arm64"} {
			if !slices.Contains(b.Goarch, requiredArch) {
				t.Fatalf("build %q missing goarch %q", b.ID, requiredArch)
			}
		}
	}
	for id, found := range requiredBuilds {
		if !found {
			t.Fatalf("missing required build id %q", id)
		}
	}

	if len(cfg.Archives) == 0 {
		t.Fatal("goreleaser archives config missing")
	}

	hasWindowsZip := false
	for _, arc := range cfg.Archives {
		for _, override := range arc.FormatOverrides {
			formats := override.Formats
			if override.Format != "" {
				formats = append(formats, override.Format)
			}
			if override.Goos == "windows" && slices.Contains(formats, "zip") {
				hasWindowsZip = true
			}
		}
	}
	if !hasWindowsZip {
		t.Fatal("expected windows archive zip format override")
	}

	if len(cfg.Brews) == 0 {
		t.Fatal("expected at least one Homebrew config in goreleaser")
	}
	if len(cfg.Dockers) == 0 {
		t.Fatal("expected at least one Docker image config in goreleaser")
	}
}

func TestInstallDocs_IncludeDistributionChannels(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	installPath := filepath.Join("..", "..", "docs", "getting-started", "install.md")

	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	installBytes, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("read install doc: %v", err)
	}

	readme := string(readmeBytes)
	installDoc := string(installBytes)

	requiredSnippets := []string{
		"brew install confighub/tap/cub-scout",
		"go install github.com/confighub/cub-scout/cmd/cub-scout@latest",
		"github.com/confighub/cub-scout/releases",
		"docker run ghcr.io/confighub/cub-scout",
		"kubectl krew install cub-scout",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(readme, snippet) {
			t.Fatalf("README missing distribution channel snippet %q", snippet)
		}
		if !strings.Contains(installDoc, snippet) {
			t.Fatalf("install doc missing distribution channel snippet %q", snippet)
		}
	}
}
