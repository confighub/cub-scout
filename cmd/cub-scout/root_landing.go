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
	fmt.Fprintln(w, "cub-scout - explore and map GitOps in your clusters")
	fmt.Fprintln(w)

	if firstRun {
		fmt.Fprintln(w, "WELCOME TO CUB-SCOUT")
		fmt.Fprintln(w, "Start with three commands to get an aha in under a minute:")
		fmt.Fprintln(w, "  cub-scout quickstart --yes  Guided first-run walkthrough")
		fmt.Fprintln(w, "  cub-scout doctor            One-command cluster summary")
		fmt.Fprintln(w, "  cub-scout map               Interactive TUI (press ? for help)")
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Quick start:")
	fmt.Fprintln(w, "  cub-scout quickstart       Guided first-run tour")
	fmt.Fprintln(w, "  cub-scout doctor           Cluster health summary")
	fmt.Fprintln(w, "  cub-scout map              Interactive TUI (press ? for help)")
	fmt.Fprintln(w, "  cub-scout explain deploy/x -n <namespace>  Explain one resource")
	fmt.Fprintln(w, "  cub-scout tree ownership   See resources by GitOps owner")
	fmt.Fprintln(w, "  cub-scout trace deploy/x   Trace a resource to Git")
	fmt.Fprintln(w, "  cub-scout map list --json  JSON output for automation")
	fmt.Fprintln(w, "  cub-scout import --dry-run Preview ConfigHub import (connected)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'cub-scout --help' for all commands")
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
