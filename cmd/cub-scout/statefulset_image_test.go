// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func addStatefulReleaseEvidence(t *testing.T, f *releaseFixture) {
	t.Helper()
	l := f.objects["StatefulSet/api"]
	require.NoError(t, unstructured.SetNestedMap(l.Object, map[string]interface{}{"observedGeneration": int64(2), "replicas": int64(2), "currentReplicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": int64(2), "currentRevision": "api-current", "updateRevision": "api-current"}, "status"))
	controller := true
	owner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: l.GetName(), UID: l.GetUID(), Controller: &controller}
	template, _, _ := unstructured.NestedMap(l.Object, "spec", "template")
	template["$patch"] = "replace"
	revision := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "ControllerRevision", "metadata": map[string]interface{}{"name": "api-current", "namespace": "delivery", "uid": "revision-uid", "resourceVersion": "1"}, "data": map[string]interface{}{"spec": map[string]interface{}{"template": template}}}}
	revision.SetOwnerReferences([]metav1.OwnerReference{owner})
	f.objects["ControllerRevision/api-current"] = revision
	for _, name := range []string{"api-0", "api-1"} {
		p := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Pod", "spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "api", "image": "example.invalid/api@sha256:" + strings.Repeat("a", 64)}}}, "status": map[string]interface{}{"phase": "Running", "conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}}, "containerStatuses": []interface{}{map[string]interface{}{"name": "api", "ready": true, "imageID": "sha256:" + strings.Repeat("a", 64), "state": map[string]interface{}{"running": map[string]interface{}{}}}}}}}
		p.SetName(name)
		p.SetNamespace("delivery")
		p.SetUID(types.UID(name + "-uid"))
		p.SetLabels(map[string]string{"app": "api", "controller-revision-hash": "api-current"})
		p.SetOwnerReferences([]metav1.OwnerReference{owner})
		f.pods = append(f.pods, p)
	}
}

func TestStatefulSetReleaseImageReadBudgetsAndFailures(t *testing.T) {
	data, err := os.ReadFile("../../examples/oci-release-check/image-statefulset.yaml")
	require.NoError(t, err)
	for _, backend := range []string{"argo", "flux"} {
		for _, scenario := range []string{"match", "denied", "capped", "revision-denied", "recreated", "old-pod", "missing-status", "stale-generation", "changed", "default"} {
			t.Run(backend+"/"+scenario, func(t *testing.T) {
				f := newReleaseFixture(t, backend, data)
				addStatefulReleaseEvidence(t, f)
				o := f.options
				o.CheckRunningImage = scenario != "default"
				o.MaxPods = 50
				switch scenario {
				case "denied":
					f.podListErr = 403
				case "capped":
					o.MaxPods = 1
				case "revision-denied":
					f.change = func(obj *unstructured.Unstructured, _ int) int {
						if obj.GetKind() == "ControllerRevision" {
							return 403
						}
						return 0
					}
				case "recreated":
					refs := f.pods[0].GetOwnerReferences()
					refs[0].UID = "old-statefulset"
					f.pods[0].SetOwnerReferences(refs)
				case "old-pod":
					f.pods[0].SetLabels(map[string]string{"app": "api", "controller-revision-hash": "old"})
				case "missing-status":
					unstructured.RemoveNestedField(f.pods[1].Object, "status", "containerStatuses")
				case "stale-generation":
					_ = unstructured.SetNestedField(f.objects["StatefulSet/api"].Object, int64(1), "status", "observedGeneration")
				case "changed":
					f.change = func(obj *unstructured.Unstructured, n int) int {
						if obj.GetKind() == "StatefulSet" && n == 2 {
							obj.SetResourceVersion("2")
						}
						return 0
					}
				}
				r, err := observeReleaseCheck(context.Background(), o)
				require.NoError(t, err)
				require.Equal(t, f.requests, r.RequestCounts.Discovery+r.RequestCounts.Object)
				if scenario == "default" {
					require.Nil(t, r.RunningImage)
					require.Zero(t, f.gets["ControllerRevision/api-current"])
					return
				}
				require.NotNil(t, r.RunningImage)
				if scenario == "match" {
					require.Equal(t, agent.VerdictPASS, r.Verdict)
					require.Equal(t, "match", r.RunningImage.Verdict)
					c := r.RunningImage.Workloads[0].StatefulSet
					require.NotNil(t, c)
					require.True(t, c.Complete)
					require.Equal(t, 2, c.OwnedPods)
					require.Equal(t, 1, f.gets["ControllerRevision/api-current"])
					require.Equal(t, 2, f.gets["StatefulSet/api"])
					for _, format := range []string{"ascii", "md"} {
						require.Contains(t, renderReleaseCheck(r, format), "StatefulSet uid=")
						require.Contains(t, renderReleaseCheck(r, format), "owned pods=2/2")
					}
					model := newReleaseCheckModel(context.Background(), o)
					model.Update(releaseCheckMessage{report: r})
					require.Contains(t, model.content, "StatefulSet uid=")
					args, err := releaseCheckMCPTool().BuildArgs(map[string]interface{}{"bundle": o.Bundle, "oci_layout": o.Layout, "controller": o.Controller, "api_version": o.APIVersion, "controller_namespace": "delivery", "context": "cluster-a", "check_running_image": true})
					require.NoError(t, err)
					cmd := newReleaseCheckCommand()
					var out bytes.Buffer
					cmd.SetOut(&out)
					cmd.SetArgs(args[2:])
					require.NoError(t, cmd.Execute())
					var shared agent.ReleaseCheckReport
					require.NoError(t, json.Unmarshal(out.Bytes(), &shared))
					require.True(t, shared.RunningImage.Workloads[0].StatefulSet.Complete)
				} else {
					require.Equal(t, "unknown", r.RunningImage.Verdict)
					require.NotEqual(t, agent.VerdictPASS, r.Verdict)
				}
				if scenario == "denied" || scenario == "capped" {
					require.Zero(t, f.gets["ControllerRevision/api-current"])
					require.Equal(t, 1, f.gets["StatefulSet/api"])
				}
			})
		}
	}
}

func testStatefulSetCLIAndMCP(t *testing.T, ctx context.Context, binary string) {
	t.Helper()
	data, err := os.ReadFile("../../examples/oci-release-check/image-statefulset.yaml")
	require.NoError(t, err)
	f := newReleaseFixture(t, "argo", data)
	addStatefulReleaseEvidence(t, f)
	a := map[string]interface{}{"bundle": f.options.Bundle, "oci_layout": f.options.Layout, "controller": f.options.Controller, "controller_namespace": "delivery", "api_version": f.options.APIVersion, "context": "cluster-a", "check_running_image": true}
	args, err := releaseCheckMCPTool().BuildArgs(a)
	require.NoError(t, err)
	for _, format := range []string{"json", "ascii", "md"} {
		cmd := exec.CommandContext(ctx, binary, append(args, "--format", format)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		if format == "json" {
			var r agent.ReleaseCheckReport
			require.NoError(t, json.Unmarshal(out, &r))
			require.Equal(t, agent.VerdictPASS, r.Verdict)
			require.True(t, r.RunningImage.Workloads[0].StatefulSet.Complete)
		} else {
			require.Contains(t, string(out), "StatefulSet uid=")
		}
	}
	if host, err := exec.LookPath("cub"); err == nil {
		dir := t.TempDir()
		p := filepath.Join(dir, "plugins", "scout")
		require.NoError(t, os.MkdirAll(p, 0700))
		require.NoError(t, os.Link(binary, filepath.Join(p, "main")))
		cmd := exec.CommandContext(ctx, host, append([]string{"scout"}, args...)...)
		cmd.Env = append(os.Environ(), "CUB_CONFIG="+dir, "CUB_CONTEXT=", "CUB_SPACE=", "CUB_TOKEN=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		var r agent.ReleaseCheckReport
		require.NoError(t, json.Unmarshal(out, &r))
		require.True(t, r.RunningImage.Workloads[0].StatefulSet.Complete)
	}
	process := exec.CommandContext(ctx, binary, "mcp", "serve")
	stdin, err := process.StdinPipe()
	require.NoError(t, err)
	stdout, err := process.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, process.Start())
	defer func() { _ = stdin.Close(); _ = process.Process.Kill(); _ = process.Wait() }()
	payload, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]interface{}{"name": "release_check", "arguments": a}})
	require.NoError(t, err)
	_, err = fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
	require.NoError(t, err)
	frame, err := readMCPFrame(bufio.NewReader(stdout))
	require.NoError(t, err)
	var response struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(frame, &response))
	require.False(t, response.Result.IsError)
	require.Len(t, response.Result.Content, 1)
	var r agent.ReleaseCheckReport
	require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &r))
	require.Equal(t, agent.VerdictPASS, r.Verdict)
	require.True(t, r.RunningImage.Workloads[0].StatefulSet.Complete)
}
