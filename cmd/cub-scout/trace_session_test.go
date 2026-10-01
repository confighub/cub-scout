package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"k8s.io/client-go/rest"
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
