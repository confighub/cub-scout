// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fleetVerifyJSONOutput struct {
	SchemaVersion    string   `json:"schemaVersion"`
	ExpectedRevision string   `json:"expectedRevision"`
	RequiredClusters []string `json:"requiredClusters"`
	Summary          struct {
		Total            int `json:"total"`
		ValidSignatures  int `json:"validSignatures"`
		InvalidReceipts  int `json:"invalidReceipts"`
		Landed           int `json:"landed"`
		Converging       int `json:"converging"`
		NotLanded        int `json:"notLanded"`
		NotObservable    int `json:"notObservable"`
		MissingClusters  int `json:"missingClusters"`
		StaleReceipts    int `json:"staleReceipts"`
		RevisionMismatch int `json:"revisionMismatch"`
	} `json:"summary"`
	Clusters []struct {
		Cluster       string `json:"cluster"`
		Valid         bool   `json:"valid"`
		State         string `json:"state"`
		Reason        string `json:"reason"`
		Revision      string `json:"revision"`
		RevisionMatch *bool  `json:"revisionMatch"`
		ObservedAt    string `json:"observedAt"`
	} `json:"clusters"`
	Answer json.RawMessage `json:"answer"`
}

type fleetVerifyAnswerEnvelope struct {
	State   string `json:"state"`
	Reason  string `json:"reason"`
	Summary string `json:"summary"`
}

func buildCubScoutForFleetExampleTest(t *testing.T, repoRoot string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "cub-scout")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cub-scout")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed:\n%s", string(out))
	return binPath
}

func TestFleetPromotionExample_VerifyScriptPassAndTamperFailClosed(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	binPath := buildCubScoutForFleetExampleTest(t, repoRoot)
	scriptPath := filepath.Join(repoRoot, "examples", "fleet-promotion-proof", "verify.sh")

	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CUB_SCOUT_BIN="+binPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "examples/fleet-promotion-proof/verify.sh failed:\n%s", string(out))
	require.Contains(t, string(out), "FLEET VERIFY: landed (fleet-landed)")
	require.Contains(t, string(out), "PASS: fleet promotion proof verified")
}

func TestFleetPromotionExample_FixturesDirectAndTamper(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	binPath := buildCubScoutForFleetExampleTest(t, repoRoot)
	exampleDir := filepath.Join(repoRoot, "examples", "fleet-promotion-proof")
	receiptsDir := filepath.Join(exampleDir, "receipts")
	pubKeyPath := filepath.Join(exampleDir, "test-pub.pem")

	// 1. Committed 4-cluster receipts verify cleanly -> exit 0, answer.state == landed.
	passCmd := exec.Command(
		binPath, "receipt", "verify-fleet",
		"--inputs", receiptsDir,
		"--pubkey", pubKeyPath,
		"--revision", "7c9e2a4f",
		"--require-clusters", "dev,staging,prod-east,prod-west",
		"--format", "json",
	)
	passCmd.Dir = repoRoot
	passOut, err := passCmd.CombinedOutput()
	require.NoError(t, err, "receipt verify-fleet failed on committed fixtures:\n%s", string(passOut))

	var passRes fleetVerifyJSONOutput
	require.NoError(t, json.Unmarshal(passOut, &passRes), "unmarshal verify-fleet json:\n%s", string(passOut))
	require.Equal(t, "fleet-receipt-verification/v1", passRes.SchemaVersion)
	require.Equal(t, "7c9e2a4f", passRes.ExpectedRevision)
	require.Equal(t, 4, passRes.Summary.Total)
	require.Equal(t, 4, passRes.Summary.ValidSignatures)
	require.Equal(t, 4, passRes.Summary.Landed)
	require.Equal(t, 0, passRes.Summary.InvalidReceipts)
	require.Len(t, passRes.Clusters, 4)

	var passAns fleetVerifyAnswerEnvelope
	require.NoError(t, json.Unmarshal(passRes.Answer, &passAns))
	require.Equal(t, "landed", passAns.State)
	require.Equal(t, "fleet-landed", passAns.Reason)

	// Compact answer envelope ceiling (<= 700 bytes).
	compactAns, err := json.Marshal(json.RawMessage(passRes.Answer))
	require.NoError(t, err)
	require.LessOrEqual(t, len(compactAns), 700, "answer envelope exceeds 700B ceiling: %s", string(compactAns))

	// 2. Tampering a copy of prod-west.json fails closed -> non-zero exit, answer.state == not-landed.
	tamperDir := filepath.Join(t.TempDir(), "receipts")
	require.NoError(t, os.MkdirAll(tamperDir, 0o755))
	for _, name := range []string{"dev.json", "staging.json", "prod-east.json", "prod-west.json"} {
		raw, rErr := os.ReadFile(filepath.Join(receiptsDir, name))
		require.NoError(t, rErr)
		if name == "prod-west.json" {
			tampered := strings.ReplaceAll(string(raw), "Deployment/api converged on the requested git-sha.", "Deployment/api forged summary.")
			require.NotEqual(t, string(raw), tampered, "expected tamper replacement to modify prod-west.json")
			raw = []byte(tampered)
		}
		require.NoError(t, os.WriteFile(filepath.Join(tamperDir, name), raw, 0o644))
	}

	tamperCmd := exec.Command(
		binPath, "receipt", "verify-fleet",
		"--inputs", tamperDir,
		"--pubkey", pubKeyPath,
		"--revision", "7c9e2a4f",
		"--require-clusters", "dev,staging,prod-east,prod-west",
		"--format", "json",
	)
	tamperCmd.Dir = repoRoot
	tamperOut, tamperErr := tamperCmd.Output()
	require.Error(t, tamperErr, "expected non-zero exit on tampered receipt copy")

	var tamperRes fleetVerifyJSONOutput
	require.NoError(t, json.Unmarshal(tamperOut, &tamperRes), "unmarshal tampered verify-fleet json:\n%s", string(tamperOut))
	require.Equal(t, 1, tamperRes.Summary.InvalidReceipts)
	require.Equal(t, 3, tamperRes.Summary.ValidSignatures)

	var tamperAns fleetVerifyAnswerEnvelope
	require.NoError(t, json.Unmarshal(tamperRes.Answer, &tamperAns))
	require.Equal(t, "not-landed", tamperAns.State)
	require.Equal(t, "fleet-signature-invalid", tamperAns.Reason)
}

func TestFleetPromotionExample_NoPrivateKeyAndReadOnlyContract(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	exampleDir := filepath.Join(repoRoot, "examples", "fleet-promotion-proof")

	// Never commit any private key material in the example directory.
	err = filepath.WalkDir(exampleDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		raw, rErr := os.ReadFile(path)
		require.NoError(t, rErr)
		require.NotContains(t, string(raw), "PRIVATE KEY", "private key material must never be committed: %s", path)
		return nil
	})
	require.NoError(t, err)

	// test-pub.pem must be a public key PEM.
	pubBytes, err := os.ReadFile(filepath.Join(exampleDir, "test-pub.pem"))
	require.NoError(t, err)
	require.Contains(t, string(pubBytes), "BEGIN PUBLIC KEY")

	// Verify docs and scripts stay strictly read-only and cover required elements.
	readmeBytes, err := os.ReadFile(filepath.Join(exampleDir, "README.md"))
	require.NoError(t, err)
	guideBytes, err := os.ReadFile(filepath.Join(repoRoot, "docs", "howto", "fleet-promotion-proof.md"))
	require.NoError(t, err)
	scriptBytes, err := os.ReadFile(filepath.Join(exampleDir, "verify.sh"))
	require.NoError(t, err)

	for _, req := range []string{"dev", "staging", "prod-east", "prod-west", "verify-fleet", "test-pub.pem", "signed-receipt/v1"} {
		require.Contains(t, string(readmeBytes), req)
	}
	for _, req := range []string{"dev", "staging", "prod-east", "prod-west", "G4", "ChangeWorkflow", "D6", "not-landed", "converging", "not-observable", "observedAt"} {
		require.Contains(t, string(guideBytes), req)
	}

	forbiddenWritePatterns := []string{
		"kubectl apply",
		"kubectl patch",
		"kubectl delete",
		"kubectl create",
		"kubectl replace",
		"kubectl edit",
	}
	for name, content := range map[string]string{
		"verify.sh":                string(scriptBytes),
		"README.md":                string(readmeBytes),
		"fleet-promotion-proof.md": string(guideBytes),
	} {
		lower := strings.ToLower(content)
		for _, forbidden := range forbiddenWritePatterns {
			require.NotContains(t, lower, forbidden, "%s must remain strictly read-only (found %q)", name, forbidden)
		}
	}
}
