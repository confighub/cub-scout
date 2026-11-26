// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestBotBuildFromRelease(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	for _, tc := range []struct {
		name, arch, checksum string
		args                 []string
		wantBuild            bool
		image                string
		failDownload         bool
		failBuild            bool
	}{
		{name: "amd64", arch: "amd64", checksum: "valid", wantBuild: true},
		{name: "arm64", arch: "arm64", checksum: "valid", wantBuild: true},
		{name: "custom image", arch: "arm64", checksum: "valid", image: "localhost:5000/observer:v2.10.1", args: []string{"v2.10.1", "arm64", "localhost:5000/observer:v2.10.1"}, wantBuild: true},
		{name: "bad checksum", arch: "amd64", checksum: "wrong"},
		{name: "missing checksum", arch: "amd64", checksum: "missing"},
		{name: "duplicate checksum", arch: "amd64", checksum: "duplicate"},
		{name: "substring checksum", arch: "amd64", checksum: "substring"},
		{name: "malformed checksum", arch: "amd64", checksum: "malformed"},
		{name: "missing binary", arch: "amd64", checksum: "valid"},
		{name: "download failure", arch: "amd64", checksum: "valid", failDownload: true},
		{name: "build failure", arch: "amd64", checksum: "valid", wantBuild: true, failBuild: true},
		{name: "mutable version", arch: "amd64", checksum: "valid", args: []string{"latest", "amd64"}},
		{name: "invalid arch", arch: "amd64", checksum: "valid", args: []string{"v2.10.1", "armv7"}},
		{name: "extra argument", arch: "amd64", checksum: "valid", args: []string{"v2.10.1", "amd64", "local:v2", "extra"}},
		{name: "invalid image", arch: "amd64", checksum: "valid", args: []string{"v2.10.1", "amd64", "--push"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			assets := filepath.Join(dir, "assets")
			bin := filepath.Join(dir, "bin")
			for _, path := range []string{assets, bin} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			archive := fmt.Sprintf("cub-scout_2.10.1_linux_%s.tar.gz", tc.arch)
			path := filepath.Join(assets, archive)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			tw := tar.NewWriter(gz)
			payload := []byte("synthetic-not-an-executable\n")
			member := "cub-scout"
			if tc.name == "missing binary" {
				member = "not-cub-scout"
			}
			if err := tw.WriteHeader(&tar.Header{Name: member, Mode: 0755, Size: int64(len(payload))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(payload); err != nil {
				t.Fatal(err)
			}
			for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
				if err := close(); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			checksum := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), archive)
			switch tc.checksum {
			case "wrong":
				checksum = strings.Repeat("0", 64) + "  " + archive + "\n"
			case "missing":
				checksum = ""
			case "duplicate":
				checksum += checksum
			case "substring":
				checksum = strings.TrimSpace(checksum) + ".extra\n"
			case "malformed":
				checksum = "not-a-digest  " + archive + "\n"
			}
			if err := os.WriteFile(filepath.Join(assets, "checksums.txt"), []byte(checksum), 0600); err != nil {
				t.Fatal(err)
			}
			curl := `#!/usr/bin/env bash
set -euo pipefail
out=''
while [[ $# -gt 0 ]]; do
  if [[ $1 == --output ]]; then out=$2; shift 2; else url=$1; shift; fi
done
[[ $url == https://github.com/confighub/cub-scout/releases/download/v2.10.1/* ]] || exit 1
printf '%s\n' "$out" >> "$FIXTURE_CURL_CALL"
[[ $FIXTURE_FAIL_DOWNLOAD != true ]] || exit 22
cp "$FIXTURE_ASSETS/${url##*/}" "$out"
`
			docker := `#!/usr/bin/env bash
set -euo pipefail
[[ $# == 9 && $1 == build && $2 == --load && $3 == --platform && $4 == linux/$FIXTURE_ARCH && $5 == --file && $7 == --tag && $8 == "$FIXTURE_IMAGE" ]]
[[ -f $6 && -x $9/cub-scout ]]
[[ $(cat "$9/cub-scout") == synthetic-not-an-executable ]]
printf '%s\n' "$@" > "$FIXTURE_DOCKER_CALL"
[[ $FIXTURE_FAIL_BUILD != true ]] || exit 7
`
			for name, body := range map[string]string{"curl": curl, "docker": docker} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			callFile := filepath.Join(dir, "docker-call")
			curlFile := filepath.Join(dir, "curl-call")
			args := tc.args
			if args == nil {
				args = []string{"v2.10.1", tc.arch}
			}
			args = append([]string{"../../examples/bot/build-from-release.sh"}, args...)
			image := tc.image
			if image == "" {
				image = "cub-scout-bot:v2.10.1-" + tc.arch
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", args...)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FIXTURE_ASSETS="+assets, "FIXTURE_ARCH="+tc.arch, "FIXTURE_DOCKER_CALL="+callFile, "FIXTURE_CURL_CALL="+curlFile, "FIXTURE_IMAGE="+image, fmt.Sprintf("FIXTURE_FAIL_DOWNLOAD=%t", tc.failDownload), fmt.Sprintf("FIXTURE_FAIL_BUILD=%t", tc.failBuild))
			out, err := cmd.CombinedOutput()
			wantSuccess := tc.wantBuild && !tc.failBuild
			if wantSuccess && err != nil {
				t.Fatalf("build failed: %v\n%s", err, out)
			}
			if !wantSuccess && err == nil {
				t.Fatalf("expected rejection, got: %s", out)
			}
			if !wantSuccess && strings.Contains(string(out), "Built ") {
				t.Fatalf("failed build must not report success: %s", out)
			}
			call, statErr := os.ReadFile(callFile)
			if tc.wantBuild {
				if statErr != nil {
					t.Fatal(statErr)
				}
				lines := strings.Split(strings.TrimSpace(string(call)), "\n")
				stage := lines[len(lines)-1]
				if _, err := os.Stat(stage); !os.IsNotExist(err) {
					t.Fatalf("temporary build context not removed: %s", stage)
				}
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("rejected input must not call docker: %s", call)
			}
			curlCalls, curlErr := os.ReadFile(curlFile)
			if tc.args != nil && !tc.wantBuild {
				if !os.IsNotExist(curlErr) {
					t.Fatalf("invalid arguments must not download: %s", curlCalls)
				}
			} else {
				if curlErr != nil {
					t.Fatal(curlErr)
				}
				first := strings.Split(string(curlCalls), "\n")[0]
				if _, err := os.Stat(filepath.Dir(first)); !os.IsNotExist(err) {
					t.Fatalf("temporary download directory was not removed: %s", first)
				}
			}
		})
	}
}

func TestBotImageInstallDocs(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	var question string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| Can I prepare a bot image") {
			question = line
		}
	}
	for _, term := range []string{"v2.11.0", "arm64", "amd64", "checksum", "numeric-nonroot", "No ConfigHub auth", "image push", "cluster deployment", "does not repair public registry access", "examples/bot/"} {
		if !strings.Contains(question, term) {
			t.Errorf("bot installation user question must explain %q", term)
		}
	}
	data, err = os.ReadFile("../../examples/bot/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	found := false
	for {
		var doc struct {
			Kind string `yaml:"kind"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Image           string `yaml:"image"`
							ImagePullPolicy string `yaml:"imagePullPolicy"`
						} `yaml:"containers"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if doc.Kind != "Deployment" {
			continue
		}
		for _, container := range doc.Spec.Template.Spec.Containers {
			found = true
			if !regexp.MustCompile(`^ghcr\.io/confighub/cub-scout:v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(container.Image) {
				t.Fatalf("bot example must pin a stable image, got %q", container.Image)
			}
			if container.ImagePullPolicy != "IfNotPresent" {
				t.Fatal("local loaded image requires IfNotPresent")
			}
		}
	}
	if !found {
		t.Fatal("bot deployment image not found")
	}
}

func TestBotRBACReadOnlyManifest(t *testing.T) {
	data, err := os.ReadFile("../../examples/bot/rbac-readonly.yaml")
	if err != nil {
		t.Fatalf("read rbac-readonly.yaml: %v", err)
	}

	allowedVerbs := map[string]bool{
		"get":   true,
		"list":  true,
		"watch": true,
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var saName, saNamespace string
	var crName string
	var crRuleCount int
	var bindingFound bool

	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
			Rules []struct {
				APIGroups []string `yaml:"apiGroups"`
				Resources []string `yaml:"resources"`
				Verbs     []string `yaml:"verbs"`
			} `yaml:"rules"`
			RoleRef struct {
				Kind string `yaml:"kind"`
				Name string `yaml:"name"`
			} `yaml:"roleRef"`
			Subjects []struct {
				Kind      string `yaml:"kind"`
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"subjects"`
		}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode rbac-readonly.yaml: %v", err)
		}

		switch doc.Kind {
		case "ServiceAccount":
			saName = doc.Metadata.Name
			saNamespace = doc.Metadata.Namespace
		case "ClusterRole":
			crName = doc.Metadata.Name
			for i, rule := range doc.Rules {
				crRuleCount++
				if len(rule.Verbs) == 0 {
					t.Errorf("ClusterRole %q rule %d has no verbs", crName, i)
				}
				for _, verb := range rule.Verbs {
					if !allowedVerbs[verb] {
						t.Errorf("ClusterRole %q rule %d (apiGroups=%v resources=%v) contains disallowed verb %q; only get, list, watch are permitted",
							crName, i, rule.APIGroups, rule.Resources, verb)
					}
				}
			}
		case "ClusterRoleBinding":
			bindingFound = true
			if doc.RoleRef.Kind != "ClusterRole" || doc.RoleRef.Name != "cub-scout-bot" {
				t.Errorf("ClusterRoleBinding roleRef = %s/%s, want ClusterRole/cub-scout-bot", doc.RoleRef.Kind, doc.RoleRef.Name)
			}
			if len(doc.Subjects) != 1 || doc.Subjects[0].Kind != "ServiceAccount" || doc.Subjects[0].Name != "cub-scout-bot" {
				t.Errorf("ClusterRoleBinding subjects = %+v, want single ServiceAccount cub-scout-bot", doc.Subjects)
			}
		}
	}

	if saName != "cub-scout-bot" || saNamespace != "cub-scout" {
		t.Fatalf("expected ServiceAccount cub-scout/cub-scout-bot, got %q/%q", saNamespace, saName)
	}
	if crName != "cub-scout-bot" || crRuleCount == 0 {
		t.Fatalf("expected ClusterRole cub-scout-bot with rules, got name=%q rules=%d", crName, crRuleCount)
	}
	if !bindingFound {
		t.Fatal("expected ClusterRoleBinding in examples/bot/rbac-readonly.yaml")
	}
}

func TestBotDeploymentHardenedManifest(t *testing.T) {
	data, err := os.ReadFile("../../examples/bot/deployment.yaml")
	if err != nil {
		t.Fatalf("read deployment.yaml: %v", err)
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	found := false
	for {
		var doc struct {
			Kind string `yaml:"kind"`
			Spec struct {
				Template struct {
					Spec struct {
						ServiceAccountName string `yaml:"serviceAccountName"`
						SecurityContext    struct {
							RunAsNonRoot   *bool `yaml:"runAsNonRoot"`
							SeccompProfile struct {
								Type string `yaml:"type"`
							} `yaml:"seccompProfile"`
						} `yaml:"securityContext"`
						Containers []struct {
							Name            string   `yaml:"name"`
							Image           string   `yaml:"image"`
							ImagePullPolicy string   `yaml:"imagePullPolicy"`
							Args            []string `yaml:"args"`
							Env             []struct {
								Name string `yaml:"name"`
							} `yaml:"env"`
							SecurityContext struct {
								RunAsNonRoot             *bool `yaml:"runAsNonRoot"`
								ReadOnlyRootFilesystem   *bool `yaml:"readOnlyRootFilesystem"`
								AllowPrivilegeEscalation *bool `yaml:"allowPrivilegeEscalation"`
								Capabilities             struct {
									Drop []string `yaml:"drop"`
								} `yaml:"capabilities"`
							} `yaml:"securityContext"`
						} `yaml:"containers"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode deployment.yaml: %v", err)
		}
		if doc.Kind != "Deployment" {
			continue
		}
		found = true

		podSpec := doc.Spec.Template.Spec
		if podSpec.ServiceAccountName != "cub-scout-bot" {
			t.Errorf("serviceAccountName = %q, want cub-scout-bot", podSpec.ServiceAccountName)
		}
		if podSpec.SecurityContext.SeccompProfile.Type != "RuntimeDefault" {
			t.Errorf("pod seccompProfile.type = %q, want RuntimeDefault", podSpec.SecurityContext.SeccompProfile.Type)
		}
		if len(podSpec.Containers) == 0 {
			t.Fatal("Deployment has no containers")
		}
		c := podSpec.Containers[0]
		if c.Image != "ghcr.io/confighub/cub-scout:v3.0.0" {
			t.Errorf("container image = %q, want ghcr.io/confighub/cub-scout:v3.0.0", c.Image)
		}
		if !slices.Contains(c.Args, "bot") || !slices.Contains(c.Args, "--watch-backed") {
			t.Errorf("container args = %v, want [bot --watch-backed]", c.Args)
		}
		envNames := make([]string, 0, len(c.Env))
		for _, e := range c.Env {
			envNames = append(envNames, e.Name)
		}
		for _, requiredEnv := range []string{"CUB_SCOUT_BOT_WEBHOOK_URL", "CUB_SCOUT_BOT_INTERVAL", "CUB_SCOUT_BOT_NAMESPACE"} {
			if !slices.Contains(envNames, requiredEnv) {
				t.Errorf("deployment missing documented env var %q (have %v)", requiredEnv, envNames)
			}
		}
		if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
			t.Error("container securityContext.runAsNonRoot must be true")
		}
		if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
			t.Error("container securityContext.readOnlyRootFilesystem must be true")
		}
		if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
			t.Error("container securityContext.allowPrivilegeEscalation must be false")
		}
		if !slices.Contains(c.SecurityContext.Capabilities.Drop, "ALL") {
			t.Errorf("container capabilities.drop = %v, want ALL", c.SecurityContext.Capabilities.Drop)
		}
	}
	if !found {
		t.Fatal("Deployment not found in examples/bot/deployment.yaml")
	}
}

func TestBotMultiArchContainerReleaseConfig(t *testing.T) {
	goreleaserBytes, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg struct {
		Dockers []struct {
			Goos               string   `yaml:"goos"`
			Goarch             string   `yaml:"goarch"`
			Use                string   `yaml:"use"`
			ImageTemplates     []string `yaml:"image_templates"`
			BuildFlagTemplates []string `yaml:"build_flag_templates"`
		} `yaml:"dockers"`
		DockerManifests []struct {
			NameTemplate   string   `yaml:"name_template"`
			ImageTemplates []string `yaml:"image_templates"`
		} `yaml:"docker_manifests"`
	}
	if err := yaml.Unmarshal(goreleaserBytes, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}

	seenArch := map[string]bool{}
	for _, d := range cfg.Dockers {
		if d.Goos != "linux" {
			continue
		}
		seenArch[d.Goarch] = true
		if d.Use != "buildx" {
			t.Errorf("docker build for linux/%s must set use: buildx, got %q", d.Goarch, d.Use)
		}
		wantPlatform := "--platform=linux/" + d.Goarch
		if !slices.Contains(d.BuildFlagTemplates, wantPlatform) {
			t.Errorf("docker build for linux/%s missing %q in build_flag_templates: %v", d.Goarch, wantPlatform, d.BuildFlagTemplates)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if !seenArch[arch] {
			t.Errorf(".goreleaser.yaml missing linux/%s docker build", arch)
		}
	}

	manifestByName := map[string][]string{}
	for _, m := range cfg.DockerManifests {
		manifestByName[m.NameTemplate] = m.ImageTemplates
	}
	for _, tc := range []struct {
		name  string
		needs []string
	}{
		{
			name:  "ghcr.io/confighub/cub-scout:{{ .Tag }}",
			needs: []string{"ghcr.io/confighub/cub-scout:{{ .Tag }}-amd64", "ghcr.io/confighub/cub-scout:{{ .Tag }}-arm64"},
		},
		{
			name:  "ghcr.io/confighub/cub-scout:latest",
			needs: []string{"ghcr.io/confighub/cub-scout:latest-amd64", "ghcr.io/confighub/cub-scout:latest-arm64"},
		},
	} {
		images, ok := manifestByName[tc.name]
		if !ok {
			t.Errorf(".goreleaser.yaml missing docker_manifests entry %q", tc.name)
			continue
		}
		for _, want := range tc.needs {
			if !slices.Contains(images, want) {
				t.Errorf("docker_manifests %q missing %q (have %v)", tc.name, want, images)
			}
		}
	}

	dockerfileBytes, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(dockerfileBytes), `ENTRYPOINT ["/cub-scout"]`) {
		t.Error(`Dockerfile must set ENTRYPOINT ["/cub-scout"] so Kubernetes args ["bot", "--watch-backed"] invoke cub-scout bot cleanly`)
	}
}
