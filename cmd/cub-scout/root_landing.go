package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func detectAndMarkFirstRun() (bool, error) {
	if forced, ok := forcedFirstRunFromEnv(); ok {
		return forced, nil
	}

	markerPath, err := firstRunMarkerPath()
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(markerPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return false, err
	}
	content := []byte(time.Now().UTC().Format(time.RFC3339) + "\n")
	if err := os.WriteFile(markerPath, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func renderRootLanding(w io.Writer, firstRun bool) {
	fmt.Fprintln(w, "cub-scout - GitOps explorer for agents")
	fmt.Fprintln(w)

	// The example commands follow the invocation form the user chose, so a
	// `cub scout` plugin user is shown `cub scout ...` lines to copy. Both
	// forms are nine characters wide, so the columns stay aligned.
	bin := preferInvocationForm("cub-scout")

	if firstRun {
		fmt.Fprintln(w, "WELCOME TO CUB-SCOUT")
		fmt.Fprintln(w, "Start with three commands to get an aha in under a minute:")
		fmt.Fprintf(w, "  %s quickstart --yes  Guided first-run walkthrough\n", bin)
		fmt.Fprintf(w, "  %s doctor            One-command cluster summary\n", bin)
		fmt.Fprintf(w, "  %s map               Interactive TUI (press ? for help)\n", bin)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Quick start:")
	fmt.Fprintf(w, "  %s quickstart       Guided first-run tour\n", bin)
	fmt.Fprintf(w, "  %s doctor           Cluster health summary\n", bin)
	fmt.Fprintf(w, "  %s map              Interactive TUI (press ? for help)\n", bin)
	fmt.Fprintf(w, "  %s explain deploy/x -n <namespace>  Explain one resource\n", bin)
	fmt.Fprintf(w, "  %s tree ownership   See resources by GitOps owner\n", bin)
	fmt.Fprintf(w, "  %s trace deploy/x   Trace a resource to Git\n", bin)
	fmt.Fprintf(w, "  %s map list --json  JSON output for automation\n", bin)
	fmt.Fprintf(w, "  %s import --dry-run Preview ConfigHub import (connected)\n", bin)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run '%s --help' for all commands\n", bin)
}

func forcedFirstRunFromEnv() (bool, bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CUB_SCOUT_TEST_FIRST_RUN")))
	switch v {
	case "":
		return false, false
	case "1", "true", "yes", "first":
		return true, true
	case "0", "false", "no":
		return false, true
	default:
		return false, false
	}
}

func firstRunMarkerPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CUB_SCOUT_FIRST_RUN_FILE")); path != "" {
		return path, nil
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "cub-scout", "first-run.seen"), nil
}
