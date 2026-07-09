// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestKubectlPlugin_BuildTargetProducesExecutablePlugin(t *testing.T) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not installed")
	}

	repoRoot := mustFindRepoRoot(t)
	pluginPath := filepath.Join(repoRoot, "kubectl-cub_scout")
	_ = os.Remove(pluginPath)
	t.Cleanup(func() {
		_ = os.Remove(pluginPath)
	})

	if makeOut, err := runInRepo(repoRoot, "make", "build-kubectl-plugin"); err != nil {
		t.Fatalf("make build-kubectl-plugin failed: %v\n%s", err, makeOut)
	}

	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("plugin binary missing at %s: %v", pluginPath, err)
	}

	kubectlOut, err := runWithPath(repoRoot, "kubectl", "cub-scout", "version")
	if err != nil {
		t.Fatalf("kubectl cub-scout version failed: %v\n%s", err, kubectlOut)
	}
	if !strings.Contains(kubectlOut, "cub-scout version") {
		t.Fatalf("unexpected plugin version output:\n%s", kubectlOut)
	}
}

func TestKubectlPlugin_MapListJSONParity(t *testing.T) {
	skipIfNoCluster(t)
	repoRoot := mustFindRepoRoot(t)

	if makeOut, err := runInRepo(repoRoot, "make", "build-kubectl-plugin"); err != nil {
		t.Fatalf("make build-kubectl-plugin failed: %v\n%s", err, makeOut)
	}

	namespace := createKubectlPluginParityFixture(t)

	directOut, err := runCmd(getCubAgentPath(), "map", "list", "--namespace", namespace, "--json")
	if err != nil {
		t.Fatalf("cub-scout map list --json failed: %v\n%s", err, directOut)
	}

	pluginOut, err := runWithPath(repoRoot, "kubectl", "cub-scout", "map", "list", "--namespace", namespace, "--json")
	if err != nil {
		t.Fatalf("kubectl cub-scout map list --json failed: %v\n%s", err, pluginOut)
	}

	directKeys, err := extractMapListIdentityKeys(directOut)
	if err != nil {
		t.Fatalf("parse direct map list json: %v\n%s", err, directOut)
	}
	pluginKeys, err := extractMapListIdentityKeys(pluginOut)
	if err != nil {
		t.Fatalf("parse plugin map list json: %v\n%s", err, pluginOut)
	}

	if len(directKeys) != len(pluginKeys) {
		t.Fatalf("map list entry count mismatch: direct=%d plugin=%d", len(directKeys), len(pluginKeys))
	}
	if len(directKeys) == 0 {
		t.Fatalf("map list returned no entries for fixture namespace %q", namespace)
	}

	for i := range directKeys {
		if directKeys[i] != pluginKeys[i] {
			t.Fatalf("map list identity mismatch at index %d: direct=%q plugin=%q", i, directKeys[i], pluginKeys[i])
		}
	}
}

func TestKubectlPlugin_KrewManifestContract(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	manifestPath := filepath.Join(repoRoot, "dist", "krew", "cub-scout.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read krew manifest: %v", err)
	}

	var manifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Platforms []struct {
				Bin string `yaml:"bin"`
				URI string `yaml:"uri"`
			} `yaml:"platforms"`
		} `yaml:"spec"`
	}

	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse krew manifest: %v", err)
	}

	if manifest.APIVersion != "krew.googlecontainertools.github.com/v1alpha2" {
		t.Fatalf("apiVersion = %q, want krew.googlecontainertools.github.com/v1alpha2", manifest.APIVersion)
	}
	if manifest.Kind != "Plugin" {
		t.Fatalf("kind = %q, want Plugin", manifest.Kind)
	}
	if manifest.Metadata.Name != "cub-scout" {
		t.Fatalf("metadata.name = %q, want cub-scout", manifest.Metadata.Name)
	}
	if len(manifest.Spec.Platforms) == 0 {
		t.Fatal("manifest has no platforms")
	}

	for i, p := range manifest.Spec.Platforms {
		if p.Bin != "kubectl-cub_scout" {
			t.Fatalf("platform[%d].bin = %q, want kubectl-cub_scout", i, p.Bin)
		}
		if strings.TrimSpace(p.URI) == "" {
			t.Fatalf("platform[%d].uri is empty", i)
		}
	}
}

func createKubectlPluginParityFixture(t *testing.T) string {
	t.Helper()

	namespace := fmt.Sprintf("cub-scout-plugin-%d", time.Now().UnixNano())
	if out, err := runCmd("kubectl", "create", "namespace", namespace); err != nil {
		t.Fatalf("create fixture namespace %s: %v\n%s", namespace, err, out)
	}
	t.Cleanup(func() {
		_, _ = runCmd("kubectl", "delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false")
	})

	if out, err := runCmd("kubectl", "-n", namespace, "create", "deployment", "parity-api", "--image=nginx:1.25", "--replicas=0"); err != nil {
		t.Fatalf("create fixture deployment in %s: %v\n%s", namespace, err, out)
	}
	if out, err := runCmd("kubectl", "-n", namespace, "create", "configmap", "parity-config", "--from-literal=mode=plugin-parity"); err != nil {
		t.Fatalf("create fixture configmap in %s: %v\n%s", namespace, err, out)
	}

	return namespace
}

func extractMapListIdentityKeys(raw string) ([]string, error) {
	var entries []struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Owner     string `json:"owner"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, fmt.Sprintf("%s/%s/%s/%s", e.Namespace, e.Kind, e.Name, e.Owner))
	}
	sort.Strings(keys)
	return keys, nil
}

func runInRepo(repoRoot, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runWithPath(pathPrefix, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "PATH="+pathPrefix+":"+os.Getenv("PATH"), "NO_COLOR=1")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	output, err := cmd.CombinedOutput()
	return string(output), err
}
