// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func watchIdentityState(cluster *agent.ClusterIdentityEvidence, uid string) watchState {
	obj := mapInstanceObject(uid)
	obj.SetName("api")
	entry := mapInstanceEntry(obj, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, cluster)
	return watchState{clusterIdentity: cluster, entriesByID: map[string]MapEntry{entry.ID: entry}, findings: map[string]watchFinding{}}
}

func TestWatchIdentityRecreationAndLastObservedDeletion(t *testing.T) {
	before := watchIdentityState(mapInstanceCluster("cluster-a"), "uid-before")
	after := watchIdentityState(mapInstanceCluster("cluster-a"), "uid-after")
	events := buildWatchEvents(before, after, nil, "", time.Now)
	require.Len(t, events, 3)
	for _, event := range events {
		switch event.Type {
		case "resource.deleted":
			require.Equal(t, "uid-before", event.ResourceIdentity.Observed.UID)
			require.Same(t, before.clusterIdentity, event.Cluster)
		case "resource.discovered":
			require.Equal(t, "uid-after", event.ResourceIdentity.Observed.UID)
		case "cluster.observed":
			require.Nil(t, event.ResourceIdentity)
			require.Equal(t, "identity-reader", event.ClusterCostScope)
		default:
			t.Fatalf("unexpected event %s", event.Type)
		}
	}
	// Denied identity and replacement clusters cannot authorize an old deletion.
	for _, cluster := range []*agent.ClusterIdentityEvidence{unavailableMapClusterIdentity("selected", "forbidden"), mapInstanceCluster("cluster-b")} {
		empty := watchState{clusterIdentity: cluster}
		events = buildWatchEvents(before, empty, nil, "", time.Now)
		require.Len(t, events, 1)
		require.Equal(t, "cluster.observed", events[0].Type)
	}
}

func TestWatchIdentityMissingAndAmbiguousObjects(t *testing.T) {
	state := watchIdentityState(mapInstanceCluster("cluster-a"), "")
	events := buildWatchEvents(watchState{}, state, nil, "", time.Now)
	require.Equal(t, "object_identity_unavailable", events[1].ResourceIdentity.Omission)
	state = watchIdentityState(mapInstanceCluster("cluster-a"), "uid")
	for _, e := range state.entriesByID {
		state.entriesByID["other-group"] = e
		break
	}
	state.findings = map[string]watchFinding{"x": {Kind: "Deployment", Name: "api", Namespace: "team-a", Category: "OTHER"}}
	events = buildWatchEvents(watchState{}, state, nil, "", time.Now)
	for _, e := range events {
		if e.Type == "scan.finding" {
			require.Equal(t, "object_identity_ambiguous", e.ResourceIdentity.Omission)
			require.Nil(t, e.ResourceIdentity.Observed)
		}
	}
}

func TestWatchIdentityDefaultWireUnchanged(t *testing.T) {
	state := watchIdentityState(mapInstanceCluster("cluster-a"), "uid")
	state.clusterIdentity = nil
	events := buildWatchEvents(watchState{}, state, nil, "", time.Now)
	require.Len(t, events, 1)
	raw, err := json.Marshal(events[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "clusterCostScope")
	require.NotContains(t, string(raw), "resourceIdentity")
	require.NotContains(t, string(raw), `"cluster":{`)
}

func TestWatchAndBotIdentityUsesSelectedConfigOneRead(t *testing.T) {
	for _, name := range []string{"watch", "bot"} {
		for _, deny := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/denied=%t", name, deny), func(t *testing.T) {
				defer overrideWatchDeps(t)()
				prior := watchReceiptBatchCap
				defer func() { watchReceiptBatchCap = prior }()
				identityReads := 0
				selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/api/v1/namespaces/kube-system":
						identityReads++
						if deny {
							w.WriteHeader(403)
							fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","reason":"Forbidden","code":403}`)
							return
						}
						fmt.Fprint(w, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"cluster-a"}}`)
					case "/apis/apps/v1/namespaces/team-a/deployments":
						fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}`)
					default:
						t.Errorf("unexpected read %s", r.URL.Path)
						w.WriteHeader(404)
					}
				}))
				defer selected.Close()
				ambient := newCountedKubeServer(t)
				cfg, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})
				t.Setenv("KUBECONFIG", cfg)
				watchCollectState = func(ctx context.Context, client dynamic.Interface, namespace string) (watchState, error) {
					cluster := watchClusterIdentityFromContext(ctx)
					require.NotNil(t, cluster)
					_, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(namespace).List(ctx, metav1.ListOptions{})
					require.NoError(t, err)
					return watchIdentityState(cluster, "uid"), nil
				}
				cmd := watchContextCommand(t, name, "selected")
				cmd.Flags().Bool("cluster-identity", false, "")
				require.NoError(t, cmd.Flags().Set("cluster-identity", "true"))
				output := filepath.Join(t.TempDir(), "events.jsonl")
				require.NoError(t, runWatchWithOptions(cmd, watchOptions{OutputFile: output, Namespace: "team-a", Once: true, Interval: time.Second, MaxQueuedEvents: 10}))
				require.Equal(t, 1, identityReads)
				require.Zero(t, ambient.requests.Load())
				raw, err := os.ReadFile(output)
				require.NoError(t, err)
				require.Contains(t, string(raw), "cluster.observed")
				if deny {
					require.Contains(t, string(raw), "forbidden")
					require.Contains(t, string(raw), "cluster_identity_unverified")
					require.NotContains(t, string(raw), "mergeKey")
				} else {
					require.Contains(t, string(raw), "mergeKey")
				}
				after, err := os.ReadFile(cfg)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}

func TestWatchIdentityRefusesInformerBeforeSink(t *testing.T) {
	priorCap := watchReceiptBatchCap
	defer func() { watchReceiptBatchCap = priorCap }()
	cmd := watchContextCommand(t, "watch", "missing")
	cmd.Flags().Bool("cluster-identity", true, "")
	output := filepath.Join(t.TempDir(), "must-not-exist")
	err := runWatchWithOptions(cmd, watchOptions{OutputFile: output, Once: true, WatchBacked: true, Interval: time.Second, MaxQueuedEvents: 10})
	require.ErrorContains(t, err, "original object age")
	_, err = os.Stat(output)
	require.True(t, os.IsNotExist(err))
	for _, command := range []*cobra.Command{watchCmd, botCmd} {
		require.NotNil(t, command.Flags().Lookup("cluster-identity"))
	}
	emit, err := parseWatchEmitReceiptOn("cluster.observed")
	require.NoError(t, err)
	require.Equal(t, []string{"cluster.observed"}, unsupportedEmitReceiptTypes(emit))
}
