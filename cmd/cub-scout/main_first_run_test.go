package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAndMarkFirstRun_CreatesMarkerOnce(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "first-run.seen")
	t.Setenv("CUB_SCOUT_FIRST_RUN_FILE", markerPath)
	t.Setenv("CUB_SCOUT_TEST_FIRST_RUN", "")

	first, err := detectAndMarkFirstRun()
	if err != nil {
		t.Fatalf("detectAndMarkFirstRun first call failed: %v", err)
	}
	if !first {
		t.Fatalf("first call = %v, want true", first)
	}

	second, err := detectAndMarkFirstRun()
	if err != nil {
		t.Fatalf("detectAndMarkFirstRun second call failed: %v", err)
	}
	if second {
		t.Fatalf("second call = %v, want false", second)
	}
}

func TestDetectAndMarkFirstRun_RespectsForcedTestOverride(t *testing.T) {
	t.Setenv("CUB_SCOUT_TEST_FIRST_RUN", "true")
	first, err := detectAndMarkFirstRun()
	if err != nil {
		t.Fatalf("detectAndMarkFirstRun failed: %v", err)
	}
	if !first {
		t.Fatal("expected forced first-run override to return true")
	}
}

func TestRenderRootLanding_FirstRunIncludesWelcome(t *testing.T) {
	var b bytes.Buffer
	renderRootLanding(&b, true)
	out := b.String()

	required := []string{
		"WELCOME TO CUB-SCOUT",
		"cub-scout quickstart --yes",
		"cub-scout doctor",
		"cub-scout map",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in root first-run output:\n%s", s, out)
		}
	}
}

func TestRenderRootLanding_RepeatRunOmitsWelcome(t *testing.T) {
	var b bytes.Buffer
	renderRootLanding(&b, false)
	out := b.String()

	if strings.Contains(out, "WELCOME TO CUB-SCOUT") {
		t.Fatalf("did not expect welcome banner in repeat output:\n%s", out)
	}
	if !strings.Contains(out, "Quick start:") {
		t.Fatalf("expected quick start block in repeat output:\n%s", out)
	}
}
