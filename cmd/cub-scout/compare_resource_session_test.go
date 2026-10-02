// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// These recorded TLS API responses validate the internal LIVE reader used by
// the live-delivery/comparison examples; they require no Kubernetes cluster.
type compareSessionFixture struct {
	server                       *httptest.Server
	mu                           sync.Mutex
	reads                        map[string]int
	marker                       string
	workloadStatus, sourceStatus int
	labels                       map[string]string
	multiSource                  bool
	ambiguous                    bool
}

func newCompareSessionFixture(t *testing.T, marker string) *compareSessionFixture {
	t.Helper()
	f := &compareSessionFixture{marker: marker, reads: map[string]int{}, workloadStatus: 200, sourceStatus: 200, labels: map[string]string{"argocd.argoproj.io/instance": "app"}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reads[r.URL.Path]++
		f.mu.Unlock()
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer "+marker+"-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		var status int
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/team/deployments/app":
			status = f.workloadStatus
			if status == 200 {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "app", "namespace": "team", "resourceVersion": marker, "labels": f.labels}, "spec": map[string]interface{}{"replicas": 2, "template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": marker + ":v1"}}}}}})
				return
			}
		case "/apis/argoproj.io/v1alpha1/applications", "/apis/argoproj.io/v1alpha1/namespaces/argocd/applications/app", "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/app":
			status = f.sourceStatus
			if status == 200 {
				namespace := "argocd"
				if r.URL.Path == "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/app" {
					namespace = "delivery"
				}
				source := map[string]interface{}{"repoURL": "https://" + marker + ".invalid/repo", "targetRevision": marker + "-revision", "path": "apps/" + marker}
				spec := map[string]interface{}{"source": source}
				if f.multiSource {
					spec = map[string]interface{}{"sources": []interface{}{source, map[string]interface{}{"repoURL": "https://other.invalid/repo"}}}
				}
				app := map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application", "metadata": map[string]interface{}{"name": "app", "namespace": namespace, "annotations": map[string]string{"confighub.com/UnitSlug": marker + "-unit", "confighub.com/SpaceID": marker + "-space"}}, "spec": spec}
				if r.URL.Path == "/apis/argoproj.io/v1alpha1/applications" {
					assert.Equal(t, "metadata.name=app", r.URL.Query().Get("fieldSelector"))
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "ApplicationList", "items": compareSessionApplications(f, app, spec)})
				} else {
					_ = json.NewEncoder(w).Encode(app)
				}
				return
			}
		case "/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/source":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository", "metadata": map[string]interface{}{"name": "source", "namespace": "flux-system"}, "spec": map[string]interface{}{"url": "https://" + marker + ".invalid/repo"}})
			return
		default:
			t.Errorf("unexpected selected API read: %s", r.URL.Path)
			status = 404
		}
		writeTraceStatus(w, status, http.StatusText(status))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *compareSessionFixture) config() *rest.Config {
	return &rest.Config{Host: f.server.URL, BearerToken: f.marker + "-token", TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.Certificate().Raw})}}
}

func (f *compareSessionFixture) counts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for key, count := range f.reads {
		out[key] = count
	}
	return out
}

func capturedCompareTestSession(t *testing.T, selected, ambient *compareSessionFixture) *traceSession {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-config")
	write := func(f *compareSessionFixture) {
		cfg := api.NewConfig()
		cfg.CurrentContext = "selected"
		cfg.Contexts["selected"] = &api.Context{Cluster: "selected", AuthInfo: "selected"}
		cfg.Clusters["selected"] = &api.Cluster{Server: f.server.URL, CertificateAuthorityData: f.config().CAData}
		cfg.AuthInfos["selected"] = &api.AuthInfo{Token: f.marker + "-token"}
		require.NoError(t, clientcmd.WriteToFile(*cfg, path))
	}
	write(selected)
	t.Setenv("KUBECONFIG", path)
	cfg, label, err := resolveClusterConfig("selected", true, &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}, nil)
	require.NoError(t, err)
	session, err := newTraceSession(cfg, label)
	require.NoError(t, err)
	cfg.Host, cfg.BearerToken = ambient.server.URL, ambient.marker+"-token"
	write(ambient)
	retargeted, err := os.ReadFile(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		current, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, retargeted, current, "captured reader must not mutate the retargeted config")
		require.Equal(t, path, os.Getenv("KUBECONFIG"))
		require.Empty(t, ambient.counts(), "no request may reach the ambient endpoint")
	})
	return session
}

func TestCompareLiveSnapshotCapturedSession(t *testing.T) {
	selected := newCompareSessionFixture(t, "selected")
	ambient := newCompareSessionFixture(t, "ambient")
	session := capturedCompareTestSession(t, selected, ambient)
	summary, err := loadCompareLiveSnapshotWithTraceSession(context.Background(), session, "Deployment", "app", "team")
	require.NoError(t, err)
	require.Equal(t, "selected", summary.ResourceVersion)
	require.Equal(t, []string{"selected:v1"}, summary.Images)
	require.Equal(t, "selected-unit", summary.UnitSlug)
	require.Equal(t, "selected-space", summary.SpaceID)
	require.Equal(t, &agent.GitSourceAnchor{RepoURL: "https://selected.invalid/repo", Revision: "selected-revision", Path: "apps/selected"}, summary.GitSource)
	require.Equal(t, map[string]int{"/apis/apps/v1/namespaces/team/deployments/app": 1, "/apis/argoproj.io/v1alpha1/applications": 1, "/apis/argoproj.io/v1alpha1/namespaces/argocd/applications/app": 1}, selected.counts())
}

func TestCompareLiveSnapshotCapturedMissingMetadata(t *testing.T) {
	selected := newCompareSessionFixture(t, "selected")
	selected.labels = nil
	ambient := newCompareSessionFixture(t, "ambient")
	session := capturedCompareTestSession(t, selected, ambient)
	summary, err := loadCompareLiveSnapshotWithTraceSessionAndFlux(context.Background(), session, "Deployment", "app", "team", func(*traceSession) (agent.Tracer, func() error, error) {
		t.Fatal("missing source metadata must not launch a speculative tracer")
		return nil, nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, "selected", summary.ResourceVersion)
	require.Nil(t, summary.GitSource, "nil is unavailable provenance, not an unmanaged verdict")
	require.Empty(t, summary.UnitSlug)
	require.Equal(t, map[string]int{"/apis/apps/v1/namespaces/team/deployments/app": 1}, selected.counts())
}

func TestCompareLiveSnapshotCapturedFailures(t *testing.T) {
	for _, tc := range []struct {
		name, kind, wantError            string
		workloadStatus, sourceStatus     int
		cancel, cancelSource, nilSession bool
		wantLive                         bool
		wantReads                        int
	}{
		{name: "missing session", kind: "Deployment", wantError: "session is unavailable", nilSession: true},
		{name: "unsupported", kind: "Unsupported", wantError: "unsupported resource kind"},
		{name: "missing workload", kind: "Deployment", workloadStatus: 404, wantError: "Not Found", wantReads: 1},
		{name: "denied workload", kind: "Deployment", workloadStatus: 403, wantError: "Forbidden", wantReads: 1},
		{name: "denied source and link", kind: "Deployment", sourceStatus: 403, wantError: "Git source enrichment unavailable", wantLive: true, wantReads: 3},
		{name: "missing source", kind: "Deployment", sourceStatus: 404, wantError: "Not Found", wantLive: true, wantReads: 3},
		{name: "canceled", kind: "Deployment", cancel: true, wantError: "context canceled"},
		{name: "canceled source", kind: "Deployment", cancelSource: true, wantError: "context canceled", wantLive: true, wantReads: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := newCompareSessionFixture(t, "selected")
			ambient := newCompareSessionFixture(t, "ambient")
			if tc.workloadStatus != 0 {
				selected.workloadStatus = tc.workloadStatus
			}
			if tc.sourceStatus != 0 {
				selected.sourceStatus = tc.sourceStatus
			}
			session := capturedCompareTestSession(t, selected, ambient)
			if tc.nilSession {
				session = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelSource {
				session.config.WrapTransport = func(next http.RoundTripper) http.RoundTripper {
					return rolloutTransportFunc(func(r *http.Request) (*http.Response, error) {
						if r.URL.Path == "/apis/argoproj.io/v1alpha1/applications" {
							cancel()
							return nil, ctx.Err()
						}
						return next.RoundTrip(r)
					})
				}
			}
			if tc.cancel {
				cancel()
			}
			summary, err := loadCompareLiveSnapshotWithTraceSession(ctx, session, tc.kind, "app", "team")
			require.ErrorContains(t, err, tc.wantError)
			require.Equal(t, tc.wantLive, summary.Source == "cluster")
			require.Nil(t, summary.GitSource)
			require.Empty(t, summary.UnitSlug)
			if tc.wantLive {
				require.Equal(t, "selected", summary.ResourceVersion)
				require.ErrorContains(t, err, "ConfigHub link enrichment unavailable")
			}
			reads := 0
			for _, count := range selected.counts() {
				reads += count
			}
			require.Equal(t, tc.wantReads, reads)
		})
	}
}

func TestCompareLiveSnapshotDirectApplicationAndMultiSource(t *testing.T) {
	selected := newCompareSessionFixture(t, "selected")
	selected.multiSource = true
	ambient := newCompareSessionFixture(t, "ambient")
	session := capturedCompareTestSession(t, selected, ambient)
	summary, err := loadCompareLiveSnapshotWithTraceSession(context.Background(), session, "Application", "app", "delivery")
	require.ErrorContains(t, err, "first of multiple declared sources")
	require.Equal(t, "delivery", summary.Namespace)
	require.Equal(t, "https://selected.invalid/repo", summary.GitSource.RepoURL)
	require.Equal(t, map[string]int{"/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/app": 2}, selected.counts())
}

type compareBoundFluxFixture struct {
	session *traceSession
	t       *testing.T
	owner   agent.Ownership
	direct  bool
}

func (f *compareBoundFluxFixture) Available() bool  { return true }
func (f *compareBoundFluxFixture) ToolName() string { return "flux" }
func (f *compareBoundFluxFixture) TraceByOwnership(ctx context.Context, owner agent.Ownership) (*agent.TraceResult, error) {
	f.owner = owner
	return f.source(ctx)
}
func (f *compareBoundFluxFixture) Trace(ctx context.Context, kind, name, namespace string) (*agent.TraceResult, error) {
	f.direct = true
	require.Equal(f.t, "Deployment/app/team", kind+"/"+name+"/"+namespace)
	return f.source(ctx)
}
func (f *compareBoundFluxFixture) source(ctx context.Context) (*agent.TraceResult, error) {
	dyn, err := f.session.dynamicClient()
	if err != nil {
		return nil, err
	}
	obj, err := dyn.Resource(kindToGVR("GitRepository")).Namespace("flux-system").Get(ctx, "source", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	url, _, _ := unstructured.NestedString(obj.Object, "spec", "url")
	return &agent.TraceResult{Chain: []agent.ChainLink{{Kind: "GitRepository", URL: url}, {Kind: "HelmRelease"}}}, nil
}

// Use the established factory seam instead of requiring a real Flux process.
func TestCompareLiveSnapshotCapturedFluxFactory(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		configHub, setupFailure, cleanupFailure bool
	}{
		{name: "Flux owner"}, {name: "ConfigHub fallback", configHub: true}, {name: "setup failure", setupFailure: true}, {name: "cleanup failure", cleanupFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := newCompareSessionFixture(t, "selected")
			selected.labels = map[string]string{"helm.toolkit.fluxcd.io/name": "release", "helm.toolkit.fluxcd.io/namespace": "flux-system"}
			if tc.configHub {
				selected.labels = map[string]string{"confighub.com/UnitSlug": "actual-unit", "confighub.com/SpaceName": "actual-space"}
			}
			ambient := newCompareSessionFixture(t, "ambient")
			session := capturedCompareTestSession(t, selected, ambient)
			tracer := &compareBoundFluxFixture{session: session, t: t}
			cleaned := false
			factory := func(got *traceSession) (agent.Tracer, func() error, error) {
				require.Same(t, session, got)
				// The production adapter's private child config captures exactly this
				// session and is deleted on all exits, including setup failure.
				child, err := got.createChildKubeconfig()
				require.NoError(t, err)
				loaded, err := clientcmd.LoadFromFile(child.Path)
				require.NoError(t, err)
				require.Equal(t, selected.server.URL, loaded.Clusters[child.Context].Server)
				require.Equal(t, "selected-token", loaded.AuthInfos[child.Context].Token)
				cleanup := func() error {
					cleaned = true
					require.NoError(t, child.Cleanup())
					_, err := os.Stat(child.Path)
					require.True(t, os.IsNotExist(err))
					if tc.cleanupFailure {
						return fmt.Errorf("private-credential-path")
					}
					return nil
				}
				if tc.setupFailure {
					return nil, cleanup, fmt.Errorf("fixture setup failed")
				}
				return tracer, cleanup, nil
			}
			summary, err := loadCompareLiveSnapshotWithTraceSessionAndFlux(context.Background(), session, "Deployment", "app", "team", factory)
			require.True(t, cleaned)
			if tc.setupFailure || tc.cleanupFailure {
				require.Error(t, err)
				require.Nil(t, summary.GitSource)
				require.Equal(t, "selected", summary.ResourceVersion)
				if tc.cleanupFailure {
					require.ErrorContains(t, err, "unable to remove temporary Trace credentials")
					require.NotContains(t, err.Error(), "private-credential-path")
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "https://selected.invalid/repo", summary.GitSource.RepoURL)
				require.Equal(t, agent.GitSourceTypeHelm, summary.GitSource.SourceType)
				require.Equal(t, agent.GitSourceTemplatedNotResolved, summary.GitSource.Resolution)
				if tc.configHub {
					require.True(t, tracer.direct)
					require.Equal(t, "actual-unit", summary.UnitSlug)
				} else {
					require.Equal(t, "release", tracer.owner.Name)
					require.Equal(t, "flux-system", tracer.owner.Namespace)
				}
			}
		})
	}
}

func compareSessionApplications(f *compareSessionFixture, app, spec map[string]interface{}) []interface{} {
	if !f.ambiguous {
		return []interface{}{app}
	}
	return []interface{}{app, map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application", "metadata": map[string]interface{}{"name": "app", "namespace": "other"}, "spec": spec}}
}

func TestCompareLiveSnapshotCapturedAmbiguousApplication(t *testing.T) {
	selected := newCompareSessionFixture(t, "selected")
	selected.ambiguous = true
	ambient := newCompareSessionFixture(t, "ambient")
	session := capturedCompareTestSession(t, selected, ambient)
	summary, err := loadCompareLiveSnapshotWithTraceSession(context.Background(), session, "Deployment", "app", "team")
	require.ErrorContains(t, err, "ambiguous across 2 namespaces")
	require.Nil(t, summary.GitSource)
	require.Equal(t, "selected", summary.ResourceVersion)
	require.Equal(t, "selected-unit", summary.UnitSlug, "existing link lookup remains separate metadata enrichment")
}

func TestCompareGitSourceUnavailableInput(t *testing.T) {
	anchor, err := collectCompareGitSourceWithTraceSession(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Nil(t, anchor)
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Deployment", "metadata": map[string]interface{}{"name": "app"}}}
	anchor, err = collectCompareGitSourceWithTraceSession(context.Background(), nil, obj, nil)
	require.ErrorContains(t, err, "trace session is unavailable")
	require.Nil(t, anchor)
	result := &agent.TraceResult{Error: "source read denied"}
	require.ErrorContains(t, compareGitSourceObservationError(result, nil), "source read denied")
	result = &agent.TraceResult{MultiSource: true, Error: "partial read denied", Chain: []agent.ChainLink{{Kind: "Source", URL: "https://first.invalid"}}}
	err = compareGitSourceObservationError(result, nil)
	require.ErrorContains(t, err, "partial read denied")
	require.ErrorContains(t, err, "first of multiple declared sources")
}
