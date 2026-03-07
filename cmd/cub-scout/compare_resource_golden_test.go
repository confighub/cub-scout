package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

var updateCompareGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func TestCompareResource_Golden_Connected(t *testing.T) {
	result := compareResourceResult{
		Resource:  "Deployment/checkout",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Dry: &compareSideSummary{
			Source:     "dry",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "checkout",
			Namespace:  "prod",
			Replicas:   int64Ptr(1),
			Images:     []string{"ghcr.io/acme/checkout:v1"},
		},
		Wet: &compareSideSummary{
			Source:     "wet",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "checkout",
			Namespace:  "prod",
			Replicas:   int64Ptr(2),
			Images:     []string{"ghcr.io/acme/checkout:v2"},
		},
		Live: compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "checkout",
			Namespace:  "prod",
			Replicas:   int64Ptr(3),
			Images:     []string{"ghcr.io/acme/checkout:v3"},
		},
		Mismatches: []compareFieldMismatch{
			{
				Field: "replicas",
				Dry:   "1",
				Wet:   "2",
				Live:  "3",
			},
			{
				Field: "images",
				Dry:   "ghcr.io/acme/checkout:v1",
				Wet:   "ghcr.io/acme/checkout:v2",
				Live:  "ghcr.io/acme/checkout:v3",
			},
		},
		Notes: []string{
			"Connected DRY/WET snapshots loaded for linked unit checkout.",
		},
	}

	assertCompareGolden(t, "connected_ascii.golden.txt", renderCompareResourceASCII(result))
	assertCompareGolden(t, "connected_md.golden.md", renderCompareResourceMarkdown(result))

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal compare result: %v", err)
	}
	assertCompareGolden(t, "connected_json.golden.json", string(jsonBytes)+"\n")
}

func TestCompareResource_Golden_Standalone(t *testing.T) {
	result := compareResourceResult{
		Resource:  "Deployment/checkout",
		Namespace: "prod",
		Mode:      "live-only",
		Connected: false,
		Live: compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "checkout",
			Namespace:  "prod",
			Replicas:   int64Ptr(3),
			Images:     []string{"ghcr.io/acme/checkout:v3"},
		},
		Notes: []string{
			"Connect to ConfigHub to unlock DRY/WET/LIVE expected-state comparison.",
		},
	}

	assertCompareGolden(t, "standalone_ascii.golden.txt", renderCompareResourceASCII(result))
}

func assertCompareGolden(t *testing.T, goldenFile string, actual string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "compare", goldenFile)
	if updateCompareGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if actual != string(expected) {
		t.Fatalf("golden mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s", goldenFile, string(expected), actual)
	}
}
