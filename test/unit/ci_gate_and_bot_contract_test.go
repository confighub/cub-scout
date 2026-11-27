// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

type compositeActionYAML struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Inputs      map[string]struct {
		Description string `json:"description"`
		Required    bool   `json:"required"`
		Default     string `json:"default"`
	} `json:"inputs"`
	Outputs map[string]struct {
		Description string `json:"description"`
		Value       string `json:"value"`
	} `json:"outputs"`
	Runs struct {
		Using string `json:"using"`
		Steps []struct {
			ID    string            `json:"id"`
			Name  string            `json:"name"`
			Shell string            `json:"shell"`
			Env   map[string]string `json:"env"`
			Run   string            `json:"run"`
		} `json:"steps"`
	} `json:"runs"`
}

func TestCIGateActionYAMLAndReadOnlyContract(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	gateDir := filepath.Join(repoRoot, "examples", "ci-gates", "verify-landed")
	actionPath := filepath.Join(gateDir, "action.yml")
	scriptPath := filepath.Join(gateDir, "verify-landed.sh")
	workflowPath := filepath.Join(gateDir, "workflow-example.yaml")
	readmePath := filepath.Join(gateDir, "README.md")

	// 1. Validate composite action YAML structure and required inputs/outputs.
	actionBytes, err := os.ReadFile(actionPath)
	require.NoError(t, err)

	var act compositeActionYAML
	require.NoError(t, yaml.Unmarshal(actionBytes, &act))
	require.NotEmpty(t, act.Name)
	require.Equal(t, "composite", act.Runs.Using)
	require.NotEmpty(t, act.Runs.Steps)

	for _, reqInput := range []string{
		"target",
		"namespace",
		"revision",
		"verify-pod-images",
		"image",
		"wait",
		"run-before-write",
		"field-path",
		"sign-key-path",
		"verify-pubkey-path",
		"recording",
	} {
		_, ok := act.Inputs[reqInput]
		require.True(t, ok, "action.yml missing input %q", reqInput)
	}
	require.True(t, act.Inputs["target"].Required, "target input must be required")

	for _, reqOutput := range []string{"state", "receipt-path", "signed-receipt-path"} {
		out, ok := act.Outputs[reqOutput]
		require.True(t, ok, "action.yml missing output %q", reqOutput)
		require.NotEmpty(t, out.Value, "action.yml output %q must map to step output", reqOutput)
	}

	// 2. Validate workflow-example.yaml parses cleanly and uses least-privilege permissions.
	wfBytes, err := os.ReadFile(workflowPath)
	require.NoError(t, err)
	var wf map[string]any
	require.NoError(t, yaml.Unmarshal(wfBytes, &wf))
	require.Contains(t, wf, "jobs")
	require.Contains(t, string(wfBytes), "contents: read")
	require.Contains(t, string(wfBytes), "./examples/ci-gates/verify-landed")
	require.Contains(t, string(wfBytes), "landed_state == 'landed'")

	// 3. Verify strictly read-only flags and zero cluster write verbs across gate files.
	scriptBytes, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	scriptText := string(scriptBytes)
	require.Contains(t, scriptText, "before-write")
	require.Contains(t, scriptText, "landed")
	require.Contains(t, scriptText, "receipt sign")
	require.Contains(t, scriptText, "receipt verify")
	require.Contains(t, scriptText, `if [[ "$STATE" != "landed" ]]; then`)

	forbiddenWritePatterns := []string{
		"kubectl apply",
		"kubectl patch",
		"kubectl delete",
		"kubectl create",
		"kubectl replace",
		"kubectl edit",
		"cub-scout remedy",
		"cub-scout import",
	}
	for name, content := range map[string]string{
		"action.yml":            string(actionBytes),
		"verify-landed.sh":      scriptText,
		"workflow-example.yaml": string(wfBytes),
	} {
		lower := strings.ToLower(content)
		for _, forbidden := range forbiddenWritePatterns {
			require.NotContains(t, lower, forbidden, "%s must remain strictly read-only (found %q)", name, forbidden)
		}
	}

	// 4. Verify README exists and documents key contract elements.
	readmeBytes, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	readmeText := string(readmeBytes)
	require.Contains(t, readmeText, "before-write")
	require.Contains(t, readmeText, "landed")
	require.Contains(t, readmeText, "receipt sign")
	require.Contains(t, readmeText, "receipt verify")
	require.Contains(t, readmeText, "signed-receipt-path")
}

func TestCIGateVerifyLandedOfflinePassAndFailClosed(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	binPath := filepath.Join(t.TempDir(), "cub-scout")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/cub-scout")
	buildCmd.Dir = repoRoot
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOut))

	testScript := filepath.Join(repoRoot, "examples", "ci-gates", "verify-landed", "test_verify_landed.sh")
	cmd := exec.Command("bash", testScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CUB_SCOUT_BIN="+binPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "test_verify_landed.sh failed:\n%s", string(out))
	require.Contains(t, string(out), "PASS: verify-landed offline CI gate tests passed")
}

func TestBotConsumerContractDocument(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	contractPath := filepath.Join(repoRoot, "examples", "bot", "consumer-contract.md")
	raw, err := os.ReadFile(contractPath)
	require.NoError(t, err)
	text := string(raw)

	for _, required := range []string{
		"answer.state",
		"answer.summary",
		"answer.basis[].quoted",
		"answer.unknowns[].quoted",
		"answer.next",
		"issues-flagged",
		"nothing-flagged",
		"incomplete",
		"not-observable",
		"700",
		"k8sbot",
		"Pilot",
	} {
		require.Contains(t, text, required, "examples/bot/consumer-contract.md missing %q", required)
	}
}
