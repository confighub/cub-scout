// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"
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
	return fakeCubUnitPipelineFailing(t, payload, "")
}

// fakeCubUnitPipelineFailing is the same fake cub, with one subcommand made to
// fail the way cub does: a message on stderr and a non-zero exit. `failOn` is
// the first two words of the call, for example "unit update".
func fakeCubUnitPipelineFailing(t *testing.T, payload, failOn string) (logPath, stdinPath string) {
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
	fail := ""
	if failOn != "" {
		fail = "if [ \"$1 $2\" = '" + failOn + "' ]; then echo 'Failed: nope' >&2; exit 1; fi\n"
	}
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + logPath + "\"\n" +
		"echo 'Flag --json has been deprecated, use -o json' >&2\n" +
		fail +
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

// What the flow does, as argv: read the unit's data, write the annotated data
// back. There is no third call: `cub unit apply` was removed, and reporting a
// success cub-scout did not achieve is the defect this fixes.
func TestAnnotationUpdateRunsTheConfigHubPipeline(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	stubCluster(t, stdinPath, -1) // the cluster is watched; what it says is another test

	result, err := testAnnotationUpdate("prod", "api")
	if err != nil {
		t.Fatalf("testAnnotationUpdate: %v", err)
	}
	if result.ResourceName != "api" {
		t.Fatalf("result = %+v, want the annotated resource named", result)
	}

	calls := fakeCubCalls(t, logPath)
	if len(calls) != 2 {
		t.Fatalf("cub calls = %v, want two: the data read and the write-back", calls)
	}
	wants := []string{
		"unit data api --space prod",
		"unit update api - --space prod --change-desc",
	}
	for i, want := range wants {
		if !strings.HasPrefix(calls[i], want) {
			t.Fatalf("call %d = %q, want it to start with %q", i, calls[i], want)
		}
	}

	// The unit carries the annotation, and the flow says what it then saw in
	// the cluster rather than assuming anything.
	for _, want := range []string{"confighub.com/test-update", "written to unit api", "deployment/api in prod"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want it to contain %q", result.Message, want)
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

// A write-back that fails is an error, with cub's own reason, not a success
// with a message about the target.
func TestAnnotationUpdateReportsAFailedWriteBack(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipelineFailing(t, testDeploymentYAML, "unit update")

	result, err := testAnnotationUpdate("prod", "api")
	if err == nil {
		t.Fatalf("err = nil, want the write-back failure; result = %+v", result)
	}
	for _, want := range []string{"api", "prod", "Failed: nope"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
	if result != nil && result.Success {
		t.Fatal("result.Success = true although the write-back failed")
	}
	if calls := fakeCubCalls(t, logPath); len(calls) != 2 {
		t.Fatalf("cub calls = %v, want the flow to stop after the failed write-back", calls)
	}
	_ = stdinPath
}

// The rollout flow waited up to two minutes for live data to appear. There is
// no live data to wait for, and a unit's config data is there as soon as the
// unit is.
func TestRolloutRestartDoesNotWaitForLiveData(t *testing.T) {
	logPath, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	stubCluster(t, stdinPath, -1)

	result, err := testRolloutRestart("prod", "api")
	if err != nil {
		t.Fatalf("testRolloutRestart: %v", err)
	}
	if result.ResourceName != "api" {
		t.Fatalf("result = %+v, want the restarted resource named", result)
	}
	if result.Success {
		t.Fatalf("result.Success = true although the stubbed cluster never had the annotation: %+v", result)
	}

	calls := fakeCubCalls(t, logPath)
	if len(calls) != 2 || calls[0] != "unit data api --space prod" {
		t.Fatalf("cub calls = %v, want the data read and the write-back", calls)
	}
	written, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read what was written to the unit: %v", err)
	}
	if !strings.Contains(string(written), "kubectl.kubernetes.io/restartedAt") {
		t.Fatalf("written data = %q, want the restart annotation", written)
	}
}

// The wait is gone, not merely fast: a failing read is returned as an error on
// the first attempt. A retry loop would swallow it and try again, which is what
// the old code did for two minutes.
func TestRolloutRestartDoesNotRetryAFailedRead(t *testing.T) {
	logPath, _ := fakeCubUnitPipelineFailing(t, testDeploymentYAML, "unit data")

	start := time.Now()
	_, err := testRolloutRestart("prod", "api")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("err = nil, want the failed read reported")
	}
	if !strings.Contains(err.Error(), "Failed: nope") {
		t.Fatalf("err = %q, want cub's own reason", err)
	}
	if calls := fakeCubCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("cub calls = %v, want exactly one attempt", calls)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("took %v, want no retry", elapsed)
	}
}

// fakeCluster answers the live-object read from what the flow actually wrote to
// the unit, so a flow that writes one value and looks for another fails.
//
// It models where an annotation really lives: a rollout annotation reaches the
// pod template and never the workload's own metadata, which is the defect this
// fixture exists to catch.
type fakeCluster struct {
	t *testing.T
	// stdinPath is what the fake cub captured from `unit update <slug> -`.
	stdinPath string
	// arrivesAfter is how many reads pass before the cluster has the change;
	// -1 means never.
	arrivesAfter int
	reads        int
	now          time.Time
}

// stubCluster installs the fake cluster, a clock the sleep advances, and a
// short timeout. Everything is restored at the end of the test.
func stubCluster(t *testing.T, stdinPath string, arrivesAfter int) *fakeCluster {
	t.Helper()
	cluster := &fakeCluster{t: t, stdinPath: stdinPath, arrivesAfter: arrivesAfter, now: time.Now()}

	prevRead, prevSleep, prevNow, prevTimeout := readLiveAnnotationsFn, observeSleepFn, observeNowFn, testUpdateTimeoutFlag
	t.Cleanup(func() {
		readLiveAnnotationsFn, observeSleepFn, observeNowFn, testUpdateTimeoutFlag = prevRead, prevSleep, prevNow, prevTimeout
	})

	// The clock only moves when the wait sleeps, so the loop makes exactly the
	// number of reads the timeout allows and the test takes no real time.
	observeNowFn = func() time.Time { return cluster.now }
	observeSleepFn = func(d time.Duration) { cluster.now = cluster.now.Add(d) }
	testUpdateTimeoutFlag = 9 * time.Second // three reads at the 3s interval
	readLiveAnnotationsFn = cluster.read
	return cluster
}

func (c *fakeCluster) read(_ context.Context, target annotatedTarget) (map[string]string, error) {
	c.reads++
	if c.arrivesAfter < 0 || c.reads <= c.arrivesAfter {
		return map[string]string{}, nil
	}
	// What the flow wrote, read back from where it wrote it.
	written := c.writtenAnnotations(target.Where)
	return written, nil
}

// writtenAnnotations parses the YAML the flow sent to `cub unit update -` and
// returns the annotations at the location asked for. A real API server can only
// report what was written where it was written.
func (c *fakeCluster) writtenAnnotations(where annotationLocation) map[string]string {
	c.t.Helper()
	raw, err := os.ReadFile(c.stdinPath)
	if err != nil {
		c.t.Fatalf("read what was written to the unit: %v", err)
	}
	var doc struct {
		Metadata struct {
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"metadata"`
		Spec struct {
			Template struct {
				Metadata struct {
					Annotations map[string]string `yaml:"annotations"`
				} `yaml:"metadata"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		c.t.Fatalf("parse what was written to the unit: %v", err)
	}
	if where == annotationOnPodTemplate {
		return doc.Spec.Template.Metadata.Annotations
	}
	return doc.Metadata.Annotations
}

// The flow's claim is now an observation: the annotation was seen on the live
// object, or it was not, and how long it waited.
func TestAnnotationUpdateSucceedsOnlyWhenTheClusterHasIt(t *testing.T) {
	_, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	cluster := stubCluster(t, stdinPath, 1) // the second read sees it

	result, err := testAnnotationUpdate("prod", "api")
	if err != nil {
		t.Fatalf("testAnnotationUpdate: %v", err)
	}
	if !result.Success {
		t.Fatalf("result.Success = false although the cluster had the annotation: %+v", result)
	}
	for _, want := range []string{"written to unit api", "observed on deployment/api in prod", "(metadata)"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want it to contain %q", result.Message, want)
		}
	}
	if cluster.reads != 2 {
		t.Fatalf("the cluster was read %d times, want it watched until the annotation arrived", cluster.reads)
	}
}

// The rollout annotation goes on the pod template. A Deployment's own metadata
// never gains it, so a watch that reads the wrong place reports failure for a
// rollout that worked — and blames the worker for it.
func TestRolloutRestartWatchesThePodTemplate(t *testing.T) {
	_, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	cluster := stubCluster(t, stdinPath, 1)

	result, err := testRolloutRestart("prod", "api")
	if err != nil {
		t.Fatalf("testRolloutRestart: %v", err)
	}
	if !result.Success {
		t.Fatalf("result.Success = false although the pod template carried the annotation: %+v", result)
	}
	for _, want := range []string{"Restart annotation written to unit api", "observed on deployment/api in prod", "(pod template)"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want it to contain %q", result.Message, want)
		}
	}
	if cluster.reads != 2 {
		t.Fatalf("the cluster was read %d times, want it watched until the annotation arrived", cluster.reads)
	}
}

// When the change never arrives, the flow says so — bounded, with what it was
// waiting for and what to check. It does not claim the pipeline worked.
func TestAnnotationUpdateReportsWhatItNeverSaw(t *testing.T) {
	_, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	cluster := stubCluster(t, stdinPath, -1) // never arrives

	result, err := testAnnotationUpdate("prod", "api")
	if err != nil {
		t.Fatalf("testAnnotationUpdate: %v", err)
	}
	if result.Success {
		t.Fatal("result.Success = true although the annotation never reached the cluster")
	}
	for _, want := range []string{"not observed on deployment/api in prod", "deployment's metadata does not", "worker is running"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want it to contain %q", result.Message, want)
		}
	}
	// A 9s timeout at the 3s interval reads at 0s, 3s, 6s and 9s: polled, and
	// bounded — the last read is the one that reaches the deadline.
	if cluster.reads != 4 {
		t.Fatalf("the cluster was read %d times, want 4: polled to the deadline and no further", cluster.reads)
	}
}

// A live object that cannot be read is a different answer from one that has not
// converged, and the message says which.
func TestAnnotationUpdateSeparatesAReadFailureFromNoConvergence(t *testing.T) {
	_, stdinPath := fakeCubUnitPipeline(t, testDeploymentYAML)
	stubCluster(t, stdinPath, -1)
	readLiveAnnotationsFn = func(context.Context, annotatedTarget) (map[string]string, error) {
		return nil, fmt.Errorf(`deployments.apps "api" not found`)
	}

	result, err := testAnnotationUpdate("prod", "api")
	if err != nil {
		t.Fatalf("testAnnotationUpdate: %v", err)
	}
	if result.Success {
		t.Fatal("result.Success = true although the live object could not be read")
	}
	if !strings.Contains(result.Message, "could not be read") || !strings.Contains(result.Message, `"api" not found`) {
		t.Fatalf("message = %q, want it to carry the read failure", result.Message)
	}
	if strings.Contains(result.Message, "worker is running") {
		t.Fatalf("message = %q, want it not to blame the worker for a read failure", result.Message)
	}
}

// A unit whose data names no namespace cannot be watched: a read with no
// namespace asks for a cluster-scoped object and gets "the server could not
// find the requested resource", which explains nothing. Say what is missing
// instead, and do not spend the timeout on it.
func TestWatchRefusesATargetItCannotLocate(t *testing.T) {
	reads := 0
	prev := readLiveAnnotationsFn
	t.Cleanup(func() { readLiveAnnotationsFn = prev })
	readLiveAnnotationsFn = func(context.Context, annotatedTarget) (map[string]string, error) {
		reads++
		return map[string]string{}, nil
	}

	for _, tt := range []struct {
		name   string
		target annotatedTarget
		want   string
	}{
		{name: "no namespace", target: annotatedTarget{Kind: "Deployment", Name: "api"}, want: "names no namespace"},
		{name: "no workload", target: annotatedTarget{Kind: "Deployment", Namespace: "prod"}, want: "names no workload"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := waitForAnnotationInCluster(context.Background(), tt.target, "k", "v", time.Minute, time.Second)
			if got.Seen {
				t.Fatal("Seen = true for a target that cannot be located")
			}
			if !strings.Contains(got.Reason, tt.want) {
				t.Fatalf("reason = %q, want it to contain %q", got.Reason, tt.want)
			}
		})
	}
	if reads != 0 {
		t.Fatalf("the cluster was read %d times for a target that cannot be located, want 0", reads)
	}
}

// A failure waiting cannot fix — no kubeconfig, an unmappable kind — is
// reported at once. Retrying it for the whole timeout is a two-minute hang for
// someone who has no cluster access, which `import argocd` does not otherwise
// need.
func TestWatchDoesNotRetryAPermanentFailure(t *testing.T) {
	reads := 0
	prevRead, prevSleep, prevNow := readLiveAnnotationsFn, observeSleepFn, observeNowFn
	t.Cleanup(func() { readLiveAnnotationsFn, observeSleepFn, observeNowFn = prevRead, prevSleep, prevNow })
	now := time.Now()
	observeNowFn = func() time.Time { return now }
	observeSleepFn = func(d time.Duration) { now = now.Add(d) }
	readLiveAnnotationsFn = func(context.Context, annotatedTarget) (map[string]string, error) {
		reads++
		return nil, permanentObservationError{fmt.Errorf("build kubernetes config: no configuration has been provided")}
	}

	target := annotatedTarget{Kind: "Deployment", Name: "api", Namespace: "prod"}
	got := waitForAnnotationInCluster(context.Background(), target, "k", "v", 2*time.Minute, 3*time.Second)
	if got.Seen {
		t.Fatal("Seen = true although every read failed")
	}
	if !strings.Contains(got.Reason, "no configuration has been provided") {
		t.Fatalf("reason = %q, want the configuration failure", got.Reason)
	}
	if reads != 1 {
		t.Fatalf("the cluster was read %d times for a failure waiting cannot fix, want 1", reads)
	}
}

// --test-timeout 0 means "write it and do not wait". It used to be read as
// "unset" and silently waited two minutes.
func TestZeroTimeoutDoesNotWait(t *testing.T) {
	reads := 0
	prev := readLiveAnnotationsFn
	t.Cleanup(func() { readLiveAnnotationsFn = prev })
	readLiveAnnotationsFn = func(context.Context, annotatedTarget) (map[string]string, error) {
		reads++
		return map[string]string{}, nil
	}

	target := annotatedTarget{Kind: "Deployment", Name: "api", Namespace: "prod"}
	got := waitForAnnotationInCluster(context.Background(), target, "k", "v", 0, time.Second)
	if got.Seen || !strings.Contains(got.Reason, "did not wait") {
		t.Fatalf("observation = %+v, want it to say it did not wait", got)
	}
	if reads != 0 {
		t.Fatalf("the cluster was read %d times with no time to wait, want 0", reads)
	}
}
