package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Every surface 3.0 removes carries a notice naming 3.0 before it goes
// (#595): a flag through MarkDeprecated, a command through Deprecated. A
// notice counts only once a release containing it is published; this test
// keeps it from being dropped before then.
func TestEverythingThreeZeroRemovesSaysSo(t *testing.T) {
	flags := []struct {
		cmd  *cobra.Command
		flag string
	}{
		{traceCmd, "json"},
		{treeCmd, "json"},
		{graphExportCmd, "json"},
		{gitopsStatusCmd, "json"},
		{appCreateCmd, "set-context"},
	}
	for _, f := range flags {
		flag := f.cmd.Flags().Lookup(f.flag)
		if flag == nil {
			t.Errorf("%s --%s: no such flag", f.cmd.CommandPath(), f.flag)
			continue
		}
		if !strings.Contains(flag.Deprecated, "removed in 3.0") {
			t.Errorf("%s --%s: deprecation notice %q does not say it is removed in 3.0", f.cmd.CommandPath(), f.flag, flag.Deprecated)
		}
	}

	// connect builds its command in a function; find its --go there.
	connect, _, err := rootCmd.Find([]string{"connect"})
	if err != nil {
		t.Fatal(err)
	}
	if goFlag := connect.Flags().Lookup("go"); goFlag == nil || !strings.Contains(goFlag.Deprecated, "removed in 3.0") {
		t.Errorf("connect --go must say it is removed in 3.0: %+v", goFlag)
	}

	if !strings.Contains(hierarchyCmd.Deprecated, "removed in 3.0") {
		t.Errorf("hierarchy: deprecation notice %q does not say it is removed in 3.0", hierarchyCmd.Deprecated)
	}

	// fleet outliers stays in help, saying so, and prints its notice on
	// stderr before anything else, even when it then fails.
	if !strings.Contains(fleetOutliersCmd.Short, "removed in 3.0") || fleetOutliersCmd.Hidden || fleetOutliersCmd.Deprecated != "" {
		t.Errorf("fleet outliers must stay in help and say it is removed in 3.0: %q", fleetOutliersCmd.Short)
	}
	t.Setenv("CUB_SPACE", "")
	var stderr bytes.Buffer
	fleetOutliersCmd.SetErr(&stderr)
	defer fleetOutliersCmd.SetErr(nil)
	_ = runFleetOutliers(fleetOutliersCmd, nil)
	if !strings.HasPrefix(stderr.String(), fleetOutliersDeprecation) {
		t.Errorf("fleet outliers stderr = %q, want the notice first", stderr.String())
	}
}

// The migration guide lists every removal the notices announce.
func TestMigrationGuideListsEveryRemoval(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "migration-3.0.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(raw)
	for _, want := range []string{
		"`trace --json`", "`tree --json`", "`graph export --json`", "`gitops status --json`",
		"`connect --go`", "`app create --set-context`", "`hierarchy`", "`fleet outliers`", "verdict words",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("docs/migration-3.0.md does not list %s", want)
		}
	}
}
