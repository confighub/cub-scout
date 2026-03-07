package golden

import (
	"strings"
	"testing"
)

func TestNormalize_TempPathsWithPrivatePrefix(t *testing.T) {
	input := "A: /private/tmp/work/file.yaml\nB: /tmp/work/file.yaml\nC: /var/folders/zz/file.yaml\n"
	out := Normalize(input)

	if strings.Count(out, "<TEMP_PATH>") != 3 {
		t.Fatalf("expected 3 normalized temp paths, got %q", out)
	}
	if strings.Contains(out, "/private/tmp") || strings.Contains(out, "/tmp/") || strings.Contains(out, "/var/folders/") {
		t.Fatalf("expected temp paths to be normalized, got %q", out)
	}
}
