// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

type releaseFailWriter struct{ err error }

func (w releaseFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestReleaseCheckOutputFailure(t *testing.T) {
	for _, format := range []string{"ascii", "json", "md"} {
		t.Run(format, func(t *testing.T) {
			f := newReleaseFixture(t, "argo")
			args, err := releaseCheckMCPTool().BuildArgs(map[string]interface{}{
				"bundle": f.options.Bundle, "oci_layout": f.options.Layout,
				"controller": f.options.Controller, "api_version": f.options.APIVersion,
				"controller_namespace": "delivery", "context": "cluster-a",
			})
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "report.json")
			writeErr := errors.New("output unavailable")
			cmd := newReleaseCheckCommand()
			cmd.SetOut(releaseFailWriter{writeErr})
			cmd.SetArgs(append(args[2:], "--format", format, "--out", path))
			require.ErrorIs(t, cmd.Execute(), writeErr)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var report agent.ReleaseCheckReport
			require.NoError(t, json.Unmarshal(data, &report))
			require.Equal(t, agent.VerdictPASS, report.Verdict, "saved evidence survives stdout failure")
		})
	}
}

// Reuse the binary built by TestReleaseCheckCLIAndMCP. No PTY, open stdin,
// ConfigHub credentials or interactive UI is available to these processes.
func testReleaseCheckHeadless(t *testing.T, ctx context.Context, binary string, yaml []byte) {
	t.Helper()
	for _, backend := range []string{"argo", "flux"} {
		for _, plugin := range []string{"", "1"} {
			for _, scenario := range []string{"pass", "missing-status", "ungated-inconclusive", "denied", "capped", "configuration-diff", "invalid-input", "output-error"} {
				t.Run("headless/"+backend+"/plugin="+plugin+"/"+scenario, func(t *testing.T) {
					f := newReleaseFixture(t, backend, yaml)
					addReleaseImagePods(f, "sha256:"+strings.Repeat("a", 64))
					f.mu.Lock()
					wantCode, wantReads := 0, 11
					wantVerdict := agent.VerdictPASS
					extra := []string{}
					switch scenario {
					case "missing-status":
						unstructured.RemoveNestedField(f.pods[1].Object, "status", "containerStatuses")
						wantCode, wantVerdict = 2, agent.VerdictINCONCLUSIVE
					case "ungated-inconclusive":
						unstructured.RemoveNestedField(f.pods[1].Object, "status", "containerStatuses")
						wantVerdict = agent.VerdictINCONCLUSIVE
					case "denied":
						f.podListErr = 403
						wantCode, wantReads, wantVerdict = 2, 7, agent.VerdictINCONCLUSIVE
					case "capped":
						extra = []string{"--max-pods", "1"}
						wantCode, wantReads, wantVerdict = 2, 7, agent.VerdictINCONCLUSIVE
					case "configuration-diff":
						require.NoError(t, unstructured.SetNestedField(f.objects["Deployment/api"].Object, int64(1), "spec", "replicas"))
						wantCode, wantVerdict = 2, agent.VerdictBLOCK
					case "invalid-input":
						extra = []string{"--bundle", "oci://example.invalid/config:latest"}
						wantCode = 1
					case "output-error":
						wantCode = 1
					}
					f.mu.Unlock()
					if backend == "flux" {
						wantReads += 4
					}
					if scenario == "invalid-input" {
						wantReads = 0
					}
					args, err := releaseCheckMCPTool().BuildArgs(map[string]interface{}{
						"bundle": f.options.Bundle, "oci_layout": f.options.Layout,
						"controller": f.options.Controller, "api_version": f.options.APIVersion,
						"controller_namespace": "delivery", "context": "cluster-a", "check_running_image": true,
					})
					require.NoError(t, err)
					path := filepath.Join(t.TempDir(), "report.json")
					if scenario == "output-error" {
						path = t.TempDir()
					}
					args = append(args, "--out", path)
					if scenario != "ungated-inconclusive" {
						args = append(args, "--fail-on", "any-non-pass")
					}
					args = append(args, extra...)
					cmd := exec.CommandContext(ctx, binary, args...)
					cmd.Stdin = strings.NewReader("")
					cmd.Env = append(os.Environ(), "CUB_PLUGIN="+plugin, "TERM=dumb", "NO_COLOR=1")
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err = cmd.Run()
					if wantCode == 0 {
						require.NoError(t, err, "%s", stderr.String())
						require.Empty(t, stderr.String())
					} else {
						var exitErr *exec.ExitError
						require.ErrorAs(t, err, &exitErr, "%s", stderr.String())
						require.Equal(t, wantCode, exitErr.ExitCode(), "%s", stderr.String())
						require.NotEmpty(t, stderr.String())
					}
					f.mu.Lock()
					reads := f.requests
					f.mu.Unlock()
					require.Equal(t, wantReads, reads, "bounded requests, no hidden retry or broad fallback")
					if scenario == "output-error" {
						require.Empty(t, stdout.String())
						return
					}
					if scenario == "invalid-input" {
						require.Empty(t, stdout.String())
						_, err := os.Stat(path)
						require.True(t, os.IsNotExist(err))
						return
					}
					var report agent.ReleaseCheckReport
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &report), "stdout must contain JSON only")
					require.Equal(t, wantVerdict, report.Verdict)
					require.Equal(t, reads, report.RequestCounts.Discovery+report.RequestCounts.Object)
					saved, err := os.ReadFile(path)
					require.NoError(t, err)
					require.JSONEq(t, stdout.String(), string(saved))
				})
			}
		}
	}
}

func TestReleaseCheckImageReadScaling(t *testing.T) {
	data, err := os.ReadFile("../../examples/oci-release-check/image-deployment.yaml")
	require.NoError(t, err)
	for _, backend := range []string{"argo", "flux"} {
		for _, count := range []int{2, 50, 200} {
			t.Run(fmt.Sprintf("%s/%d-pods", backend, count), func(t *testing.T) {
				var desired map[string]interface{}
				require.NoError(t, yaml.Unmarshal(data, &desired))
				require.NoError(t, unstructured.SetNestedField(desired, int64(count), "spec", "replicas"))
				input, err := json.Marshal(desired)
				require.NoError(t, err)
				f := newReleaseFixture(t, backend, input)
				addReleaseImagePods(f, "sha256:"+strings.Repeat("a", 64))
				f.mu.Lock()
				seed := f.pods[0]
				f.pods = nil
				for i := 0; i < count; i++ {
					p := seed.DeepCopy()
					p.SetName(fmt.Sprintf("api-%d", i))
					p.SetUID(types.UID(fmt.Sprintf("pod-%d", i)))
					f.pods = append(f.pods, p)
				}
				for _, field := range []string{"replicas", "updatedReplicas", "readyReplicas", "availableReplicas"} {
					require.NoError(t, unstructured.SetNestedField(f.objects["Deployment/api"].Object, int64(count), "status", field))
				}
				f.options.CheckRunningImage, f.options.MaxPods = true, count
				f.mu.Unlock()
				r, err := observeReleaseCheck(context.Background(), f.options)
				require.NoError(t, err)
				require.Equal(t, agent.VerdictPASS, r.Verdict, "%+v", r.Stages)
				require.Equal(t, count, r.RunningImage.Workloads[0].Deployment.OwnedPods)
				want := 11
				if backend == "flux" {
					want = 15
				}
				require.Equal(t, want, r.RequestCounts.Discovery+r.RequestCounts.Object)
				f.mu.Lock()
				defer f.mu.Unlock()
				require.Equal(t, want, f.requests, "Pod cardinality must not multiply owner reads")
				require.Equal(t, 1, f.gets["ReplicaSet/api-current"])
			})
		}
	}
}
