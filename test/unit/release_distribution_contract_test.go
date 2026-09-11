package unit

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestContainerUserSupportsRunAsNonRoot(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	user := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "USER") {
			user = fields[1]
		}
	}
	uid, err := strconv.ParseUint(strings.SplitN(user, ":", 2)[0], 10, 32)
	if err != nil || uid == 0 {
		t.Fatalf("container USER %q must have a numeric nonzero UID for the bot's runAsNonRoot policy", user)
	}
}

type goreleaserConfig struct {
	Dist   string `yaml:"dist"`
	Builds []struct {
		ID     string   `yaml:"id"`
		Goos   []string `yaml:"goos"`
		Goarch []string `yaml:"goarch"`
	} `yaml:"builds"`
	Archives []struct {
		ID                        string `yaml:"id"`
		AllowDifferentBinaryCount bool   `yaml:"allow_different_binary_count"`
		Formats                   []string
		FormatOverrides           []struct {
			Goos    string   `yaml:"goos"`
			Format  string   `yaml:"format"`
			Formats []string `yaml:"formats"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
	HomebrewCasks []struct {
		Name string `yaml:"name"`
	} `yaml:"homebrew_casks"`
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

	if cfg.Dist != ".goreleaser-dist" {
		t.Fatalf("goreleaser dist dir = %q, want %q to avoid tracked dist/ collisions", cfg.Dist, ".goreleaser-dist")
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
	hasMixedBinaryArchiveAllowance := false
	for _, arc := range cfg.Archives {
		if arc.ID == "default" && arc.AllowDifferentBinaryCount {
			hasMixedBinaryArchiveAllowance = true
		}
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
	if !hasMixedBinaryArchiveAllowance {
		t.Fatal("expected default archive to allow different binary counts across platforms")
	}

	if len(cfg.HomebrewCasks) == 0 {
		t.Fatal("expected at least one homebrew_casks entry in goreleaser (post-#389 / #413 migration)")
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

func TestInstallDocs_VersionedArchiveNames(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "getting-started", "install.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	if strings.Contains(doc, "/releases/latest/download/cub-scout-") {
		t.Fatal("install guide uses obsolete unversioned archive names")
	}
	links := regexp.MustCompile(`https://github\.com/confighub/cub-scout/releases/download/[^\s)]+`).FindAllString(doc, -1)
	seen := map[string]bool{}
	version := ""
	for _, link := range links {
		u, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		name, tag := path.Base(u.Path), path.Base(path.Dir(u.Path))
		if name == "checksums.txt" {
			continue
		}
		if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(tag) {
			t.Fatalf("archive link has no release tag: %s", link)
		}
		if version != "" && version != tag {
			t.Fatalf("install guide mixes release versions %s and %s", version, tag)
		}
		version = tag
		prefix := "cub-scout_" + strings.TrimPrefix(tag, "v") + "_"
		if !strings.HasPrefix(name, prefix) {
			t.Fatalf("archive %s does not match its release tag %s", name, tag)
		}
		seen[strings.TrimPrefix(name, prefix)] = true
	}
	for _, platform := range []string{"darwin_amd64.tar.gz", "darwin_arm64.tar.gz", "linux_amd64.tar.gz", "linux_arm64.tar.gz", "windows_amd64.zip", "windows_arm64.zip"} {
		if !seen[platform] {
			t.Errorf("install guide missing verified archive platform %s", platform)
		}
	}
}

func TestIntroDocs_FiveModesAndEvidenceBoundaries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	start, end := strings.Index(doc, "## Five ways to run cub-scout"), strings.Index(doc, "## User questions")
	if start < 0 || end <= start {
		t.Fatal("README must introduce five run modes before the detailed User questions")
	}
	for _, mode := range []string{"Standalone client", "ConfigHub plugin", "MCP server", "Watch stream", "In-cluster bot"} {
		if !strings.Contains(doc[start:end], "| "+mode+" |") {
			t.Errorf("README missing run mode %q", mode)
		}
	}
	for _, heading := range []string{"## Why this exists", "## What you get"} {
		if index := strings.Index(doc, heading); index < 0 || index > start {
			t.Errorf("README must explain %q before its run modes", heading)
		}
	}
	questions, _, _ := strings.Cut(doc[end:], "\nThe main path starts")
	for _, evidence := range []string{"--bounded", "--kube-context", "configHubOrigin", "STALE", "compare object-set", "receipt verify", "watch", "bot", "release-event or sync bot"} {
		if !strings.Contains(questions, evidence) {
			t.Errorf("README questions lost evidence surface %q", evidence)
		}
	}
	for _, file := range []string{"README.md", "CLI-GUIDE.md", "docs/README.md", "docs/getting-started/start-here.md", "docs/getting-started/install.md"} {
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`(?i)unreleased[^\n]{0,24}v2\.10`).Match(data) {
			t.Errorf("%s still describes v2.10 as unreleased", file)
		}
	}
}
