package main

import (
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
)

func TestIsTraceSourceKind_ConfigHubOCI(t *testing.T) {
	if !isTraceSourceKind("ConfigHub OCI") {
		t.Fatalf("ConfigHub OCI should be treated as a source kind")
	}
}

func TestKindToGVR_ConfigHubOCI(t *testing.T) {
	gvr := kindToGVR("ConfigHub OCI")
	if gvr.Group != "source.toolkit.fluxcd.io" || gvr.Version != "v1beta2" || gvr.Resource != "ocirepositories" {
		t.Fatalf("kindToGVR(ConfigHub OCI) = %#v, want source.toolkit.fluxcd.io/v1beta2 ocirepositories", gvr)
	}
}

func TestArtifactForLink_ConfigHubOCI_FallbackToOCIRepositoryKey(t *testing.T) {
	link := agent.ChainLink{
		Kind:      "ConfigHub OCI",
		Namespace: "flux-system",
		Name:      "confighub-prod",
	}
	artifacts := map[string]mapsvc.TraceArtifactRef{
		traceArtifactKey("OCIRepository", "flux-system", "confighub-prod"): {
			URL:            "oci://oci.api.confighub.com/target/prod/us-west",
			Revision:       "latest@sha1:abc123",
			Digest:         "sha256:deadbeef",
			LastUpdateTime: "2026-03-01T12:00:00Z",
			SourceKind:     "OCIRepository",
		},
	}

	got := artifactForLink(link, artifacts)
	if got.URL != "oci://oci.api.confighub.com/target/prod/us-west" {
		t.Fatalf("artifactForLink() URL = %q, want fallback OCIRepository artifact", got.URL)
	}
	if got.SourceKind == "" {
		t.Fatalf("artifactForLink() SourceKind should not be empty")
	}
}
