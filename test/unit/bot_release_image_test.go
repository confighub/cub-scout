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
