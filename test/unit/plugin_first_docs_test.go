// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPluginFirstDocs_ShowPluginFormFirstAndKeepStandalone enforces #386:
// cub scout is the documented primary invocation form in 3.0, while the
// standalone cub-scout binary stays documented alongside it.
func TestPluginFirstDocs_ShowPluginFormFirstAndKeepStandalone(t *testing.T) {
	files := []string{
		"README.md",
		"CLI-GUIDE.md",
		filepath.Join("docs", "getting-started", "start-here.md"),
		filepath.Join("docs", "getting-started", "install.md"),
	}
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join("..", "..", rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(raw)
		if !strings.Contains(text, "cub scout ") {
			t.Errorf("%s must document the primary plugin form (`cub scout ...`)", rel)
		}
		if !strings.Contains(text, "cub-scout ") {
			t.Errorf("%s must keep documenting the standalone binary (`cub-scout ...`)", rel)
		}
	}

	readmeBytes, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeBytes)
	invokeIdx := strings.Index(readme, "## How to invoke")
	if invokeIdx < 0 {
		t.Fatal("README.md missing ## How to invoke section")
	}
	invokeSection := readme[invokeIdx:]
	pluginPos := strings.Index(invokeSection, "cub scout")
	standalonePos := strings.Index(invokeSection, "cub-scout")
	if pluginPos < 0 || standalonePos < 0 || pluginPos > standalonePos {
		t.Fatalf("README.md ## How to invoke must show `cub scout` before `cub-scout` (pluginPos=%d, standalonePos=%d)", pluginPos, standalonePos)
	}
}
