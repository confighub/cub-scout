// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// cubUnitHelpText is what `cub unit livedata <slug> --space <space>` prints on
// cub v0.5.2, recorded from a real run. The subcommand was removed in April
// 2026; cub falls back to the `unit` help and exits 0 (#571). Abbreviated in
// the middle, which does not change what it is: not config data.
const cubUnitHelpText = `The unit subcommands are used to manage units.

Units are explained at https://docs.confighub.com/background/entities/unit/.

Usage:
  cub unit [command]

Available Commands:
  approve          Approve a unit or multiple units
  data             Show the config data of a unit
  get              Get details about an unit
  list             List units
  update           Update a unit

Flags:
  -h, --help   help for unit

Use "cub unit [command] --help" for more information about a command.`

const testDeploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
      - name: api
        image: example/api:1.0
`

// fakeCubUnitPipeline puts a fake cub first on PATH for the unit-test-update
// flow. It records every argument vector, answers `unit data` with the given
// payload on stdout, prints a notice on stderr the way cub does, and saves
// whatever is written to stdin by `unit update`.
func fakeCubUnitPipeline(t *testing.T, payload string) (logPath, stdinPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	stdinPath = filepath.Join(dir, "stdin.yaml")
	dataPath := filepath.Join(dir, "data.yaml")
	if err := os.WriteFile(dataPath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + logPath + "\"\n" +
		"echo 'Flag --json has been deprecated, use -o json' >&2\n" +
		"case \"$1 $2\" in\n" +
		"  \"unit data\") cat \"" + dataPath + "\" ;;\n" +
		"  \"unit update\") cat > \"" + stdinPath + "\" ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath, stdinPath
}

// The config data of a unit is `cub unit data`. `cub unit livedata` was removed
// in April 2026 and exits 0 printing the `unit` help, which the old code took
// as config data.
func TestReadUnitConfigDataAsksCubForUnitData(t *testing.T) {
	logPath, _ := fakeCubUnitPipeline(t, testDeploymentYAML)

	data, err := readUnitConfigData("prod", "api")
	if err != nil {
		t.Fatalf("readUnitConfigData: %v", err)
	}

	calls := fakeCubCalls(t, logPath)
	if len(calls) != 1 || calls[0] != "unit data api --space prod" {
		t.Fatalf("cub calls = %v, want one `unit data api --space prod`", calls)
	}
	if !strings.Contains(data, "kind: Deployment") {
		t.Fatalf("data = %q, want the unit's config data", data)
	}
	// cub prints notices on stderr. They are not config data, and this flow
	// writes what it reads back to the unit.
	if strings.Contains(data, "deprecated") {
		t.Fatalf("data = %q, want cub's stderr kept out of it", data)
	}
}

func TestReadUnitConfigDataNamesTheUnitWhenThereIsNone(t *testing.T) {
	fakeCubUnitPipeline(t, "")

	_, err := readUnitConfigData("prod", "api")
	if err == nil {
		t.Fatal("err = nil, want a refusal for a unit with no config data")
	}
	for _, want := range []string{"api", "prod", "no config data"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
}

// A unit whose data is not a workload must not be written back, and the error
// must name the unit. The old flow reported "failed to modify YAML", which
// blamed the data for a command cub-scout should not have called.
func TestAnnotationUpdateRefusesDataThatIsNotAWorkload(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipeline(t, cubUnitHelpText)

	result, err := testAnnotationUpdate("prod", "api")
	if err == nil {
		t.Fatal("err = nil, want a refusal when the data holds no Deployment")
	}
	for _, want := range []string{"api", "prod", "no Deployment"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
	if result != nil && result.Success {
		t.Fatal("result.Success = true although nothing was written")
	}
	for _, call := range fakeCubCalls(t, logPath) {
		if strings.HasPrefix(call, "unit update") || strings.HasPrefix(call, "unit apply") {
			t.Fatalf("cub %s ran although the data held no workload", call)
		}
	}
	if _, err := os.Stat(stdinPath); err == nil {
		t.Fatal("something was written to a unit although the data held no workload")
	}
}

// The whole pipeline, as argv: read the unit's data, write the annotated data
// back, apply it and wait.
func TestAnnotationUpdateRunsTheConfigHubPipeline(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)

	result, err := testAnnotationUpdate("prod", "api")
	if err != nil {
		t.Fatalf("testAnnotationUpdate: %v", err)
	}
	if !result.Success || result.ResourceName != "api" {
		t.Fatalf("result = %+v, want success on resource api", result)
	}

	calls := fakeCubCalls(t, logPath)
	if len(calls) != 3 {
		t.Fatalf("cub calls = %v, want three", calls)
	}
	wants := []string{
		"unit data api --space prod",
		"unit update api - --space prod --change-desc",
		"unit apply api --space prod --wait",
	}
	for i, want := range wants {
		if !strings.HasPrefix(calls[i], want) {
			t.Fatalf("call %d = %q, want it to start with %q", i, calls[i], want)
		}
	}

	written, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read what was written to the unit: %v", err)
	}
	if !strings.Contains(string(written), "confighub.com/test-update") {
		t.Fatalf("written data = %q, want the annotation", written)
	}
	if strings.Contains(string(written), "deprecated") {
		t.Fatalf("written data = %q, want cub's stderr kept out of what is written back", written)
	}
}

// The rollout flow waited up to two minutes for live data to appear. There is
// no live data to wait for, and a unit's config data is there as soon as the
// unit is.
func TestRolloutRestartDoesNotWaitForLiveData(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)

	start := time.Now()
	result, err := testRolloutRestart("prod", "api")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("testRolloutRestart: %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("took %v, want no wait for live data", elapsed)
	}

	calls := fakeCubCalls(t, logPath)
	if len(calls) != 3 || calls[0] != "unit data api --space prod" {
		t.Fatalf("cub calls = %v, want unit data then update then apply", calls)
	}
	written, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read what was written to the unit: %v", err)
	}
	if !strings.Contains(string(written), "kubectl.kubernetes.io/restartedAt") {
		t.Fatalf("written data = %q, want the restart annotation", written)
	}
}
