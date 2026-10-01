package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

type traceSessionCountingPersister struct{ calls int }

func (p *traceSessionCountingPersister) Persist(map[string]string) error {
	p.calls++
	return nil
}

func TestTraceSessionUsesCapturedConfigAfterCallerRetarget(t *testing.T) {
	var mu sync.Mutex
	var got []string
	server := func(marker string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			got = append(got, marker+" "+r.URL.Path)
			mu.Unlock()
			if r.URL.Path != "/apis/apps/v1/namespaces/team-a/deployments/api" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{
					"name": "api", "namespace": "team-a", "uid": marker,
					"labels": map[string]interface{}{"confighub.com/UnitSlug": marker},
				},
			})
		}))
	}
	a := server("alpha")
	defer a.Close()
	b := server("beta")
	defer b.Close()

	config := &rest.Config{Host: a.URL}
	session, err := newTraceSession(config, "alpha-context")
	if err != nil {
		t.Fatal(err)
	}
	config.Host = b.URL
	owner, err := detectResourceOwnershipWithTraceSession(context.Background(), session, "Deployment", "api", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "alpha" {
		t.Fatalf("ownership name = %q, want alpha from captured endpoint", owner.Name)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "alpha /apis/apps/v1/namespaces/team-a/deployments/api" {
		t.Fatalf("requests = %v, want exactly the captured alpha endpoint", got)
	}
}

func TestTraceSessionRejectsNilAndInvalidConfigWithoutFallback(t *testing.T) {
	if _, err := detectResourceOwnershipWithTraceSession(context.Background(), nil, "Deployment", "api", "team-a"); err == nil {
		t.Fatal("nil trace session unexpectedly fell back to ambient config")
	}
	if _, err := newTraceSession(nil, "context"); err == nil {
		t.Fatal("nil config unexpectedly produced a trace session")
	}
	if _, err := newTraceSession(&rest.Config{}, "context"); err == nil {
		t.Fatal("invalid config unexpectedly produced a trace session")
	}
}

func TestTraceSessionDropsAuthConfigPersister(t *testing.T) {
	persister := &traceSessionCountingPersister{}
	config := &rest.Config{
		Host:                "https://127.0.0.1",
		AuthProvider:        &api.AuthProviderConfig{Name: "test", Config: map[string]string{"token": "captured"}},
		AuthConfigPersister: persister,
	}
	session, err := newTraceSession(config, "context")
	if err != nil {
		t.Fatal(err)
	}
	if session.config.AuthConfigPersister != nil {
		t.Fatal("trace session retained kubeconfig auth persistence")
	}
	if got := session.config.AuthProvider.Config["token"]; got != "captured" {
		t.Fatalf("copied auth config token = %q, want captured", got)
	}
	if persister.calls != 0 {
		t.Fatalf("original persister called %d times", persister.calls)
	}
}

func TestTraceSessionCopiesMutableTLSAuthAndImpersonationConfig(t *testing.T) {
	config := &rest.Config{
		Host: "https://127.0.0.1",
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte("ca-before"), CertData: []byte("cert-before"), KeyData: []byte("key-before"),
		},
		Impersonate: rest.ImpersonationConfig{
			Groups: []string{"group-before"},
			Extra:  map[string][]string{"scope": {"one"}},
		},
		ExecProvider: &api.ExecConfig{Command: "auth", Args: []string{"arg-before"}, Env: []api.ExecEnvVar{{Name: "MODE", Value: "before"}}},
		AuthProvider: &api.AuthProviderConfig{Name: "test", Config: map[string]string{"token": "before"}},
	}
	session, err := newTraceSession(config, "alpha")
	require.NoError(t, err)

	config.Host = "https://127.0.0.2"
	config.TLSClientConfig.CAData[0] = 'X'
	config.TLSClientConfig.CertData[0] = 'X'
	config.TLSClientConfig.KeyData[0] = 'X'
	config.Impersonate.Groups[0] = "group-after"
	config.Impersonate.Extra["scope"][0] = "two"
	config.ExecProvider.Args[0] = "arg-after"
	config.ExecProvider.Env[0].Value = "after"
	config.AuthProvider.Config["token"] = "after"

	got, err := session.restConfig()
	require.NoError(t, err)
	require.Equal(t, "https://127.0.0.1", got.Host)
	require.Equal(t, []byte("ca-before"), got.TLSClientConfig.CAData)
	require.Equal(t, []byte("cert-before"), got.TLSClientConfig.CertData)
	require.Equal(t, []byte("key-before"), got.TLSClientConfig.KeyData)
	require.Equal(t, []string{"group-before"}, got.Impersonate.Groups)
	require.Equal(t, []string{"one"}, got.Impersonate.Extra["scope"])
	require.Equal(t, "arg-before", got.ExecProvider.Args[0])
	require.Equal(t, "before", got.ExecProvider.Env[0].Value)
	require.Equal(t, "before", got.AuthProvider.Config["token"])
}

type traceSessionFixture struct {
	server   *httptest.Server
	marker   string
	mu       sync.Mutex
	requests map[string]int
}

func newTraceSessionFixture(t *testing.T, marker string) *traceSessionFixture {
	t.Helper()
	f := &traceSessionFixture{marker: marker, requests: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests[r.Method+" "+r.URL.Path]++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "v1", "kind": "APIGroupList", "groups": []interface{}{
				map[string]interface{}{"name": "argoproj.io", "preferredVersion": map[string]interface{}{"groupVersion": "argoproj.io/v1alpha1", "version": "v1alpha1"}, "versions": []interface{}{map[string]interface{}{"groupVersion": "argoproj.io/v1alpha1", "version": "v1alpha1"}}},
				map[string]interface{}{"name": "source.toolkit.fluxcd.io", "preferredVersion": map[string]interface{}{"groupVersion": "source.toolkit.fluxcd.io/v1", "version": "v1"}, "versions": []interface{}{map[string]interface{}{"groupVersion": "source.toolkit.fluxcd.io/v1", "version": "v1"}}},
				map[string]interface{}{"name": "aws.upbound.io", "preferredVersion": map[string]interface{}{"groupVersion": "aws.upbound.io/v1", "version": "v1"}, "versions": []interface{}{map[string]interface{}{"groupVersion": "aws.upbound.io/v1", "version": "v1"}}},
			}})
		case "/apis/argoproj.io/v1alpha1", "/apis/source.toolkit.fluxcd.io/v1", "/apis/aws.upbound.io/v1":
			groupVersion := strings.TrimPrefix(r.URL.Path, "/apis/")
			resources := []interface{}{}
			switch groupVersion {
			case "argoproj.io/v1alpha1":
				resources = append(resources, apiResource("applications", "Application", true))
			case "source.toolkit.fluxcd.io/v1":
				resources = append(resources, apiResource("gitrepositories", "GitRepository", true))
			case "aws.upbound.io/v1":
				resources = append(resources, apiResource("providerconfigs", "ProviderConfig", false))
			}
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "v1", "kind": "APIResourceList", "groupVersion": groupVersion, "resources": resources})
		case "/apis/apps/v1/namespaces/team-a/deployments/api":
			writeTraceFixtureJSON(w, f.deployment())
		case "/api/v1/namespaces/team-a/secrets/creds":
			writeTraceFixtureJSON(w, f.secret())
		case "/api/v1/namespaces/team-a/events":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "v1", "kind": "EventList", "metadata": map[string]interface{}{}, "items": []interface{}{
				map[string]interface{}{"apiVersion": "v1", "kind": "Event", "metadata": map[string]interface{}{"name": "event-" + f.marker, "namespace": "team-a"}, "type": "Warning", "reason": "Fixture" + f.marker, "message": "event from " + f.marker, "count": int64(1), "involvedObject": map[string]interface{}{"kind": "Deployment", "name": "api", "namespace": "team-a"}, "lastTimestamp": "2025-01-02T03:04:05Z"},
			}})
		case "/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/source":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository", "metadata": map[string]interface{}{"name": "source", "namespace": "flux-system"}, "status": map[string]interface{}{"artifact": map[string]interface{}{"url": "https://example.invalid/" + f.marker, "revision": f.marker + "@sha1:abc", "digest": "sha256:abc", "lastUpdateTime": "2025-01-02T03:04:05Z"}}})
		case "/apis/aws.upbound.io/v1/providerconfigs/default":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "aws.upbound.io/v1", "kind": "ProviderConfig", "metadata": map[string]interface{}{"name": "default", "uid": "provider-" + f.marker}, "spec": map[string]interface{}{"credentials": map[string]interface{}{"source": "Secret", "secretRef": map[string]interface{}{"name": "creds", "namespace": "team-a"}}}})
		case "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api":
			source := map[string]interface{}{"repoURL": "oci://example.invalid/config", "targetRevision": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "path": "."}
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application", "metadata": map[string]interface{}{"name": "api", "namespace": "delivery", "uid": "application-" + f.marker, "generation": int64(1)}, "spec": map[string]interface{}{"source": source}, "status": map[string]interface{}{"sync": map[string]interface{}{"revision": source["targetRevision"], "comparedTo": map[string]interface{}{"source": source}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func apiResource(name, kind string, namespaced bool) map[string]interface{} {
	return map[string]interface{}{"name": name, "singularName": "", "namespaced": namespaced, "kind": kind, "verbs": []string{"get"}}
}

func writeTraceFixtureJSON(w http.ResponseWriter, value interface{}) {
	_ = json.NewEncoder(w).Encode(value)
}

func (f *traceSessionFixture) deployment() map[string]interface{} {
	transitionTime := "2025-01-02T03:04:05Z"
	if f.marker == "beta" {
		transitionTime = "2024-01-02T03:04:05Z"
	}
	return map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "uid": "deployment-" + f.marker, "labels": map[string]interface{}{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": "release-" + f.marker, "confighub.com/UnitSlug": "unit-" + f.marker}, "annotations": map[string]interface{}{"confighub.com/UnitID": "id-" + f.marker}},
		"spec":     map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": "example.invalid/app", "envFrom": []interface{}{map[string]interface{}{"secretRef": map[string]interface{}{"name": "creds"}}}}}}}},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Available", "status": "True", "lastTransitionTime": transitionTime}}},
	}
}

func (f *traceSessionFixture) secret() map[string]interface{} {
	return map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{"name": "creds", "namespace": "team-a", "labels": map[string]interface{}{"confighub.com/UnitSlug": "secret-" + f.marker}}, "type": "Opaque"}
}

func (f *traceSessionFixture) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[method+" "+path]
}

func writeTraceKubeconfig(t *testing.T, path, current, alphaServer, betaServer string) []byte {
	t.Helper()
	cfg := api.NewConfig()
	cfg.CurrentContext = current
	cfg.Clusters["alpha"] = &api.Cluster{Server: alphaServer}
	cfg.Clusters["beta"] = &api.Cluster{Server: betaServer}
	cfg.AuthInfos["alpha"] = &api.AuthInfo{Token: "alpha-token"}
	cfg.AuthInfos["beta"] = &api.AuthInfo{Token: "beta-token"}
	cfg.Contexts["alpha-context"] = &api.Context{Cluster: "alpha", AuthInfo: "alpha"}
	cfg.Contexts["beta-context"] = &api.Context{Cluster: "beta", AuthInfo: "beta"}
	cfg.CurrentContext = current
	data, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	return data
}

func TestTraceSessionNestedReadersStayOnCapturedContext(t *testing.T) {
	alpha := newTraceSessionFixture(t, "alpha")
	beta := newTraceSessionFixture(t, "beta")
	kubeconfig := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, kubeconfig, "alpha-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", kubeconfig)

	config, contextLabel, err := resolveClusterConfig("alpha-context", true, &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, nil)
	require.NoError(t, err)
	session, err := newTraceSession(config, contextLabel)
	require.NoError(t, err)

	// Retarget both the caller-owned config and the actual ambient kubeconfig.
	// A previously captured session must keep reaching alpha.
	config.Host = beta.server.URL
	writeTraceKubeconfig(t, kubeconfig, "beta-context", alpha.server.URL, beta.server.URL)
	ambientAfterRetarget, err := os.ReadFile(kubeconfig)
	require.NoError(t, err)
	require.Equal(t, "alpha-context", session.contextLabel())
	assertAlphaRead := func(path string, before int) {
		t.Helper()
		require.Greater(t, alpha.count(http.MethodGet, path), before, "alpha should serve %s", path)
		require.Zero(t, beta.count(http.MethodGet, path), "beta must not serve %s", path)
	}

	ctx := context.Background()
	deploymentPath := "/apis/apps/v1/namespaces/team-a/deployments/api"
	beforeDeployment := alpha.count(http.MethodGet, deploymentPath)
	owner, err := detectResourceOwnershipWithTraceSession(ctx, session, "Deployment", "api", "team-a")
	require.NoError(t, err)
	require.Equal(t, agent.OwnerHelm, owner.Type)
	require.Equal(t, "release-alpha", owner.Name)
	assertAlphaRead(deploymentPath, beforeDeployment)

	result := &agent.TraceResult{Chain: []agent.ChainLink{{Kind: "Deployment", Name: "api", Namespace: "team-a"}}}
	beforeDeployment = alpha.count(http.MethodGet, deploymentPath)
	enrichTraceWithTimingSession(ctx, session, result)
	require.NotNil(t, result.Chain[0].LastTransitionTime)
	require.Equal(t, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC), *result.Chain[0].LastTransitionTime)
	assertAlphaRead(deploymentPath, beforeDeployment)

	secretPath := "/api/v1/namespaces/team-a/secrets/creds"
	beforeDeployment = alpha.count(http.MethodGet, deploymentPath)
	beforeSecret := alpha.count(http.MethodGet, secretPath)
	crossRefs, err := detectCrossOwnerReferencesWithTraceSession(ctx, session, "Deployment", "api", "team-a", owner)
	require.NoError(t, err)
	require.Len(t, crossRefs, 1)
	require.Equal(t, "creds", crossRefs[0].Ref.Name)
	require.Equal(t, "secret-alpha", crossRefs[0].Owner.Name)
	assertAlphaRead(deploymentPath, beforeDeployment)
	assertAlphaRead(secretPath, beforeSecret)

	beforeDeployment = alpha.count(http.MethodGet, deploymentPath)
	beforeSecret = alpha.count(http.MethodGet, secretPath)
	secretEvidence := collectSecretEvidenceWithTraceSession(ctx, session, "Deployment", "api", "team-a")
	require.NotNil(t, secretEvidence)
	require.Equal(t, 1, secretEvidence.Summary.Present)
	require.Equal(t, "creds", secretEvidence.Secrets[0].Name)
	assertAlphaRead(deploymentPath, beforeDeployment)
	assertAlphaRead(secretPath, beforeSecret)

	eventsPath := "/api/v1/namespaces/team-a/events"
	beforeEvents := alpha.count(http.MethodGet, eventsPath)
	events, err := fetchResourceEventsWithTraceSession(ctx, session, "team-a", "Deployment", "api")
	require.NoError(t, err)
	require.Equal(t, "Fixturealpha", events.Events[0].Reason)
	assertAlphaRead(eventsPath, beforeEvents)

	artifactPath := "/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/source"
	beforeArtifact := alpha.count(http.MethodGet, artifactPath)
	artifactTrace := &agent.TraceResult{Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "source", Namespace: "flux-system"}}}
	artifacts := collectTraceArtifactsWithTraceSession(ctx, session, artifactTrace)
	require.Equal(t, "https://example.invalid/alpha", artifacts[traceArtifactKey("GitRepository", "flux-system", "source")].URL)
	assertAlphaRead(artifactPath, beforeArtifact)

	beforeDeployment = alpha.count(http.MethodGet, deploymentPath)
	configHubTrace := &agent.TraceResult{}
	client := enrichTraceConfigHubFromLiveWithTraceSession(ctx, session, configHubTrace, "Deployment", "api", "team-a")
	require.NotNil(t, client)
	require.Equal(t, "unit-alpha", configHubTrace.ConfigHub.UnitSlug)
	require.Equal(t, "id-alpha", configHubTrace.ConfigHub.UnitID)
	assertAlphaRead(deploymentPath, beforeDeployment)

	beforeDiscovery := alpha.count(http.MethodGet, "/apis")
	beforeProviderDiscovery := alpha.count(http.MethodGet, "/apis/aws.upbound.io/v1")
	beforeProvider := alpha.count(http.MethodGet, "/apis/aws.upbound.io/v1/providerconfigs/default")
	providerOwner, err := detectResourceOwnershipWithTraceSession(ctx, session, "ProviderConfig", "default", "")
	require.NoError(t, err)
	require.Equal(t, agent.OwnerCrossplane, providerOwner.Type)
	require.Equal(t, "default", providerOwner.Name)
	assertAlphaRead("/apis", beforeDiscovery)
	assertAlphaRead("/apis/aws.upbound.io/v1", beforeProviderDiscovery)
	assertAlphaRead("/apis/aws.upbound.io/v1/providerconfigs/default", beforeProvider)
	dynClient, err := session.dynamicClient()
	require.NoError(t, err)
	provider, err := fetchProviderConfigResourceWithTraceSession(ctx, session, dynClient, "default", "")
	require.NoError(t, err)
	require.Equal(t, "provider-alpha", string(provider.GetUID()))
	assertAlphaRead("/apis/aws.upbound.io/v1/providerconfigs/default", beforeProvider)

	beforeDeployment = alpha.count(http.MethodGet, deploymentPath)
	reverse, err := agent.NewReverseTracer(dynClient).Trace(ctx, "Deployment", "api", "team-a")
	require.NoError(t, err)
	require.Equal(t, "helm", reverse.Owner)
	require.Equal(t, "deployment-alpha", string(reverse.Objects[0].GetUID()))
	assertAlphaRead(deploymentPath, beforeDeployment)

	beforeAppDiscovery := alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1")
	appPath := "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api"
	beforeApp := alpha.count(http.MethodGet, appPath)
	ociTrace := &agent.TraceResult{Tool: "argocd", Chain: []agent.ChainLink{
		{Kind: "ConfigHub OCI", OCISource: &agent.OCISourceInfo{Raw: "oci://example.invalid/config", IsConfigHub: true}},
		{Kind: "Application", Name: "api", Namespace: "delivery"},
	}}
	correlation := agent.TraceDeliveryCorrelation{OCIIdentityStatus: "exact", OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	confirmTraceOCISourceWithTraceSession(ctx, session, ociTrace, &correlation)
	require.True(t, correlation.OCISourceVerified)
	require.Equal(t, "alpha-context", correlation.OCISourceRead.Context)
	require.Equal(t, "application-alpha", correlation.OCISourceRead.UID)
	assertAlphaRead("/apis/argoproj.io/v1alpha1", beforeAppDiscovery)
	assertAlphaRead(appPath, beforeApp)

	for _, path := range []string{
		"/apis/apps/v1/namespaces/team-a/deployments/api",
		"/api/v1/namespaces/team-a/secrets/creds",
		"/api/v1/namespaces/team-a/events",
		"/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/source",
		"/apis",
		"/apis/aws.upbound.io/v1",
		"/apis/aws.upbound.io/v1/providerconfigs/default",
		"/apis/argoproj.io/v1alpha1",
		"/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api",
	} {
		require.Greater(t, alpha.count(http.MethodGet, path), 0, "alpha should serve %s", path)
		require.Zero(t, beta.count(http.MethodGet, path), "beta must not serve %s", path)
	}
	afterReaders, err := os.ReadFile(kubeconfig)
	require.NoError(t, err)
	require.Equal(t, ambientAfterRetarget, afterReaders, "readers must not rewrite the kubeconfig")
}

func TestTraceConfigHubEnrichmentWithMissingSessionDoesNotUseAmbientConfig(t *testing.T) {
	alpha := newTraceSessionFixture(t, "alpha")
	beta := newTraceSessionFixture(t, "beta")
	kubeconfig := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, kubeconfig, "beta-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", kubeconfig)
	result := &agent.TraceResult{}

	client := enrichTraceConfigHubFromLiveWithTraceSession(context.Background(), nil, result, "Deployment", "api", "team-a")
	require.Nil(t, client)
	require.Nil(t, result.ConfigHub)
	require.Zero(t, beta.count(http.MethodGet, "/apis/apps/v1/namespaces/team-a/deployments/api"), "missing session must not fall back to ambient beta config")
}

func TestConcurrentTraceSessionsKeepEndpointsIndependent(t *testing.T) {
	servers := make([]*httptest.Server, 2)
	for i, marker := range []string{"one", "two"} {
		marker := marker
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{
					"name": "api", "namespace": "team-a", "uid": marker,
					"labels": map[string]interface{}{"confighub.com/UnitSlug": marker},
				},
			})
		}))
	}
	defer servers[0].Close()
	defer servers[1].Close()

	results := make([]string, 2)
	var wg sync.WaitGroup
	for i, server := range servers {
		i, server := i, server
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := newTraceSession(&rest.Config{Host: server.URL}, "context")
			if err != nil {
				t.Errorf("new session: %v", err)
				return
			}
			owner, err := detectResourceOwnershipWithTraceSession(context.Background(), session, "Deployment", "api", "team-a")
			if err != nil {
				t.Errorf("detect ownership: %v", err)
				return
			}
			results[i] = owner.Name
		}()
	}
	wg.Wait()
	if results[0] != "one" || results[1] != "two" {
		t.Fatalf("ownership results = %v, want independent one/two endpoints", results)
	}
}
