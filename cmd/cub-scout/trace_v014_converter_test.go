// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
)

func TestConvertSecretEvidence_NilInput(t *testing.T) {
	result := convertSecretEvidence(nil)
	if result != nil {
		t.Fatalf("convertSecretEvidence(nil) = %v, want nil", result)
	}
}

func TestConvertSecretEvidence_EmptySecrets(t *testing.T) {
	evidence := &agent.SecretEvidenceResult{
		Secrets: []agent.SecretEvidence{},
		Summary: agent.SecretEvidenceSummary{Total: 0},
	}
	result := convertSecretEvidence(evidence)
	if result == nil {
		t.Fatal("convertSecretEvidence() returned nil for empty secrets")
	}
	if len(result.Secrets) != 0 {
		t.Fatalf("len(result.Secrets) = %d, want 0", len(result.Secrets))
	}
}

func TestConvertSecretEvidence_FullConversion(t *testing.T) {
	evidence := &agent.SecretEvidenceResult{
		Resource: agent.ResourceRef{Kind: "Deployment", Namespace: "default", Name: "app"},
		Secrets: []agent.SecretEvidence{
			{
				Name:         "db-creds",
				Namespace:    "default",
				RefType:      agent.SecretRefTypeEnvFrom,
				RefPath:      "containers[app].envFrom[0].secretRef",
				Status:       agent.SecretStatusPresent,
				SecretType:   "Opaque",
				Optional:     false,
			},
			{
				Name:         "tls-cert",
				Namespace:    "default",
				RefType:      agent.SecretRefTypeVolume,
				RefPath:      "volumes[certs].secret",
				Status:       agent.SecretStatusMissing,
				StatusReason: "secret not found in namespace",
				Optional:     true,
			},
		},
		Summary: agent.SecretEvidenceSummary{
			Total:   2,
			Present: 1,
			Missing: 1,
		},
	}

	result := convertSecretEvidence(evidence)
	if result == nil {
		t.Fatal("convertSecretEvidence() returned nil")
	}

	// Check secrets count
	if len(result.Secrets) != 2 {
		t.Fatalf("len(result.Secrets) = %d, want 2", len(result.Secrets))
	}

	// Check first secret
	s0 := result.Secrets[0]
	if s0.Name != "db-creds" {
		t.Errorf("Secrets[0].Name = %q, want %q", s0.Name, "db-creds")
	}
	if s0.RefType != "envFrom.secretRef" {
		t.Errorf("Secrets[0].RefType = %q, want %q", s0.RefType, "envFrom.secretRef")
	}
	if s0.Status != "present" {
		t.Errorf("Secrets[0].Status = %q, want %q", s0.Status, "present")
	}
	if s0.Optional != false {
		t.Errorf("Secrets[0].Optional = %v, want false", s0.Optional)
	}

	// Check second secret
	s1 := result.Secrets[1]
	if s1.Status != "missing" {
		t.Errorf("Secrets[1].Status = %q, want %q", s1.Status, "missing")
	}
	if s1.StatusReason != "secret not found in namespace" {
		t.Errorf("Secrets[1].StatusReason = %q, want %q", s1.StatusReason, "secret not found in namespace")
	}
	if s1.Optional != true {
		t.Errorf("Secrets[1].Optional = %v, want true", s1.Optional)
	}

	// Check summary
	if result.Summary.Total != 2 {
		t.Errorf("Summary.Total = %d, want 2", result.Summary.Total)
	}
	if result.Summary.Present != 1 {
		t.Errorf("Summary.Present = %d, want 1", result.Summary.Present)
	}
	if result.Summary.Missing != 1 {
		t.Errorf("Summary.Missing = %d, want 1", result.Summary.Missing)
	}
}

func TestConvertTraceToV014_IncludesSecrets(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		Chain: []agent.ChainLink{
			{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		},
		Tool: "flux",
		Secrets: &agent.SecretEvidenceResult{
			Secrets: []agent.SecretEvidence{
				{
					Name:       "api-key",
					Namespace:  "default",
					RefType:    agent.SecretRefTypeEnvValueFrom,
					RefPath:    "containers[app].env[0:API_KEY].valueFrom.secretKeyRef",
					Status:     agent.SecretStatusPresent,
					SecretType: "Opaque",
				},
			},
			Summary: agent.SecretEvidenceSummary{
				Total:   1,
				Present: 1,
			},
		},
	}

	output := convertTraceToV014(result, "Deployment", "web-app", "default", nil)

	// Secrets should be populated
	if output.Secrets == nil {
		t.Fatal("convertTraceToV014() did not include Secrets in output")
	}
	if len(output.Secrets.Secrets) != 1 {
		t.Fatalf("len(output.Secrets.Secrets) = %d, want 1", len(output.Secrets.Secrets))
	}
	if output.Secrets.Secrets[0].Name != "api-key" {
		t.Errorf("Secrets[0].Name = %q, want %q", output.Secrets.Secrets[0].Name, "api-key")
	}
}

func TestConvertTraceToV014_OmitsSecretsWhenNil(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		Chain: []agent.ChainLink{
			{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		},
		Tool:    "flux",
		Secrets: nil, // No secrets
	}

	output := convertTraceToV014(result, "Deployment", "web-app", "default", nil)

	// Secrets should be nil (omitempty)
	if output.Secrets != nil {
		t.Fatalf("convertTraceToV014() included Secrets when result.Secrets was nil")
	}
}

func TestConvertTraceToV014_OmitsSecretsWhenEmpty(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		Chain: []agent.ChainLink{
			{Kind: "Deployment", Namespace: "default", Name: "web-app"},
		},
		Tool: "flux",
		Secrets: &agent.SecretEvidenceResult{
			Secrets: []agent.SecretEvidence{}, // Empty
			Summary: agent.SecretEvidenceSummary{Total: 0},
		},
	}

	output := convertTraceToV014(result, "Deployment", "web-app", "default", nil)

	// Secrets should be nil when empty (omitempty behavior)
	if output.Secrets != nil {
		t.Fatalf("convertTraceToV014() included Secrets when result.Secrets was empty")
	}
}

func TestConvertTraceToV014_SecretsJSON(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Namespace: "default", Name: "app"},
		Chain:  []agent.ChainLink{{Kind: "Deployment", Namespace: "default", Name: "app"}},
		Tool:   "flux",
		Secrets: &agent.SecretEvidenceResult{
			Secrets: []agent.SecretEvidence{
				{
					Name:       "db-password",
					Namespace:  "default",
					RefType:    agent.SecretRefTypeEnvFrom,
					RefPath:    "containers[0].envFrom[0].secretRef",
					Status:     agent.SecretStatusPresent,
					SecretType: "Opaque",
				},
			},
			Summary: agent.SecretEvidenceSummary{Total: 1, Present: 1},
		},
	}

	output := convertTraceToV014(result, "Deployment", "app", "default", nil)

	// Marshal to JSON and verify structure
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}

	// Unmarshal back to verify structure
	var parsed mapsvc.TraceOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if parsed.Secrets == nil {
		t.Fatal("parsed.Secrets is nil after JSON round-trip")
	}
	if len(parsed.Secrets.Secrets) != 1 {
		t.Fatalf("len(parsed.Secrets.Secrets) = %d, want 1", len(parsed.Secrets.Secrets))
	}
	if parsed.Secrets.Secrets[0].Name != "db-password" {
		t.Errorf("Secrets[0].Name = %q, want %q", parsed.Secrets.Secrets[0].Name, "db-password")
	}
	if parsed.Secrets.Secrets[0].Status != "present" {
		t.Errorf("Secrets[0].Status = %q, want %q", parsed.Secrets.Secrets[0].Status, "present")
	}
}
