// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

const fluxMultiHopBlastFixture = `# observedAt: 2026-11-26T00:00:00Z
# sourceContext: prod-east
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: infra
  namespace: flux-system
spec:
  url: https://github.com/example/infra
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: infra
    namespace: flux-system
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
data:
  LOG_LEVEL: info
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:v1
          envFrom:
            - configMapRef:
                name: app-cfg
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: api-pdb
  namespace: prod
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: api
`

const argoMultiHopBlastFixture = `# observedAt: 2026-11-26T01:00:00Z
# sourceContext: prod-west
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: checkout
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/example/checkout
---
apiVersion: v1
kind: Secret
metadata:
  name: db-creds
  namespace: prod
  labels:
    argocd.argoproj.io/instance: checkout
type: Opaque
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: prod
  annotations:
    argocd.argoproj.io/tracking-id: "checkout:apps/Deployment:prod/checkout-api"
spec:
  template:
    metadata:
      labels:
        app: checkout-api
    spec:
      volumes:
        - name: creds
          secret:
            secretName: db-creds
      containers:
        - name: api
          image: ghcr.io/example/checkout:v2
          env:
            - name: DB_USER
              valueFrom:
                secretKeyRef:
                  name: db-creds
                  key: user
---
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: checkout-api-rs
  namespace: prod
  ownerReferences:
    - apiVersion: apps/v1
      kind: Deployment
      name: checkout-api
      uid: dep-1
---
apiVersion: v1
kind: Pod
metadata:
  name: checkout-api-rs-pod1
  namespace: prod
  ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: checkout-api-rs
      uid: rs-1
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: checkout-pdb
  namespace: prod
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: checkout-api
`

func TestGraphBlastRadiusFluxMultiHopAndByteCeiling(t *testing.T) {
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(fluxMultiHopBlastFixture))
	require.NoError(t, err)

	// Query from middle of chain (ConfigMap) -> verifies both ancestors and descendants.
	cmOut := computeBlastRadius(snapshot, "ConfigMap", "app-cfg", "prod")
	require.Equal(t, "ConfigMap/prod/app-cfg", cmOut.Target)
	require.Equal(t, "2026-11-26T00:00:00Z", cmOut.ObservedAt)
	require.False(t, cmOut.Truncated)
	require.Equal(t, []blastRadiusNode{
		{Ref: "Kustomization/flux-system/apps", Via: "flux-kustomization", Hops: 1},
		{Ref: "GitRepository/flux-system/infra", Via: "source-ref", Hops: 2},
	}, cmOut.Ancestors)
	require.Equal(t, []blastRadiusNode{
		{Ref: "Deployment/prod/api", Via: "configmap-ref", Hops: 1},
		{Ref: "PodDisruptionBudget/prod/api-pdb", Via: "pdb-selector", Hops: 2},
	}, cmOut.Descendants)
	require.Equal(t, []string{"Deployment/prod/api"}, cmOut.AffectedWorkloads)
	requireConformingAnswer(t, cmOut.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, cmOut.Answer.State)
	require.Equal(t, "offline-recording", cmOut.Answer.Cost.Coverage)

	// Compact JSON byte ceiling (<= 1,200 bytes for a 4-hop chain).
	raw, err := json.Marshal(cmOut)
	require.NoError(t, err)
	t.Logf("4-hop blast-radius compact JSON size: %d bytes (~%d tokens)", len(raw), (len(raw)+3)/4)
	require.LessOrEqual(t, len(raw), 1200, "compact JSON for 4-hop chain must stay <= 1200 bytes; got %d: %s", len(raw), string(raw))

	// Query from root of 4-hop chain (GitRepository -> Kustomization -> ConfigMap/Deployment -> PDB).
	gitOut := computeBlastRadius(snapshot, "GitRepository", "infra", "flux-system")
	require.Empty(t, gitOut.Ancestors)
	require.Equal(t, []string{"Deployment/prod/api"}, gitOut.AffectedWorkloads)
	require.Len(t, gitOut.Descendants, 4)
	gitRaw, err := json.Marshal(gitOut)
	require.NoError(t, err)
	t.Logf("root-to-leaf 4-hop blast-radius compact JSON size: %d bytes", len(gitRaw))
	require.LessOrEqual(t, len(gitRaw), 1200, "root-to-leaf compact JSON must stay <= 1200 bytes; got %d", len(gitRaw))
}

func TestGraphBlastRadiusArgoSecretOwnerRefAndPDB(t *testing.T) {
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(argoMultiHopBlastFixture))
	require.NoError(t, err)

	secOut := computeBlastRadius(snapshot, "Secret", "db-creds", "prod")
	require.Equal(t, "Secret/prod/db-creds", secOut.Target)
	require.Equal(t, "2026-11-26T01:00:00Z", secOut.ObservedAt)
	require.False(t, secOut.Truncated)
	require.Equal(t, []blastRadiusNode{
		{Ref: "Application/argocd/checkout", Via: "argocd-instance", Hops: 1},
	}, secOut.Ancestors)
	require.Equal(t, []blastRadiusNode{
		{Ref: "Deployment/prod/checkout-api", Via: "secret-ref", Hops: 1},
		{Ref: "PodDisruptionBudget/prod/checkout-pdb", Via: "pdb-selector", Hops: 2},
		{Ref: "ReplicaSet/prod/checkout-api-rs", Via: "owner-ref", Hops: 2},
		{Ref: "Pod/prod/checkout-api-rs-pod1", Via: "owner-ref", Hops: 3},
	}, secOut.Descendants)
	require.Equal(t, []string{"Deployment/prod/checkout-api"}, secOut.AffectedWorkloads)
	requireConformingAnswer(t, secOut.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, secOut.Answer.State)

	// Query from Application -> reaches tracking-id Deployment and instance-labelled Secret.
	appOut := computeBlastRadius(snapshot, "Application", "checkout", "argocd")
	require.Empty(t, appOut.Ancestors)
	require.Equal(t, []string{"Deployment/prod/checkout-api"}, appOut.AffectedWorkloads)
	require.Len(t, appOut.Descendants, 5)
}

func TestGraphBlastRadiusCycleSafety(t *testing.T) {
	cyclicYAML := `# observedAt: 2026-11-26T02:00:00Z
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: ks-a
  namespace: flux-system
spec:
  dependsOn:
    - name: ks-b
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: ks-b
  namespace: flux-system
spec:
  dependsOn:
    - name: ks-a
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: prod
  ownerReferences:
    - apiVersion: v1
      kind: ConfigMap
      name: cm-b
      uid: b
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  namespace: prod
  ownerReferences:
    - apiVersion: v1
      kind: ConfigMap
      name: cm-a
      uid: a
`
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(cyclicYAML))
	require.NoError(t, err)

	ksOut := computeBlastRadius(snapshot, "Kustomization", "ks-a", "flux-system")
	require.False(t, ksOut.Truncated)
	require.Equal(t, []blastRadiusNode{{Ref: "Kustomization/flux-system/ks-b", Via: "depends-on", Hops: 1}}, ksOut.Ancestors)
	require.Equal(t, []blastRadiusNode{{Ref: "Kustomization/flux-system/ks-b", Via: "depends-on", Hops: 1}}, ksOut.Descendants)

	cmOut := computeBlastRadius(snapshot, "ConfigMap", "cm-a", "prod")
	require.False(t, cmOut.Truncated)
	require.Equal(t, []blastRadiusNode{{Ref: "ConfigMap/prod/cm-b", Via: "owner-ref", Hops: 1}}, cmOut.Ancestors)
	require.Equal(t, []blastRadiusNode{{Ref: "ConfigMap/prod/cm-b", Via: "owner-ref", Hops: 1}}, cmOut.Descendants)
}

func TestGraphBlastRadiusTruncationCap(t *testing.T) {
	var docs []string
	docs = append(docs, `# observedAt: 2026-11-26T03:00:00Z
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: fleet
  namespace: flux-system
spec:
  path: ./apps`)
	for i := 0; i < 60; i++ {
		docs = append(docs, fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: svc-%02d
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: fleet
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  replicas: 1`, i))
	}
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(strings.Join(docs, "\n---\n")))
	require.NoError(t, err)

	out := computeBlastRadius(snapshot, "Kustomization", "fleet", "flux-system")
	require.True(t, out.Truncated)
	require.Equal(t, blastRadiusMaxNodes, len(out.Ancestors)+len(out.Descendants))
	require.Len(t, out.AffectedWorkloads, blastRadiusMaxNodes)
	requireConformingAnswer(t, out.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, out.Answer.State)

	hasTruncatedUnknown := false
	for _, u := range out.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownNotObservable && strings.Contains(u.Reason, "truncated") {
			hasTruncatedUnknown = true
		}
	}
	require.True(t, hasTruncatedUnknown, "expected truncation unknown in answer.unknowns")
}

func TestGraphBlastRadiusNotObservable(t *testing.T) {
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(fluxMultiHopBlastFixture))
	require.NoError(t, err)

	out := computeBlastRadius(snapshot, "Deployment", "does-not-exist", "prod")
	require.Equal(t, "Deployment/prod/does-not-exist", out.Target)
	require.Empty(t, out.Ancestors)
	require.Empty(t, out.Descendants)
	require.Empty(t, out.AffectedWorkloads)
	require.False(t, out.Truncated)
	requireConformingAnswer(t, out.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateNotObservable, out.Answer.State)
	require.Equal(t, explainReasonObjectNotRead, out.Answer.Reason)
}

func TestGraphBlastRadiusCLIFromRecordingFile(t *testing.T) {
	dir := t.TempDir()
	recPath := filepath.Join(dir, "snapshot.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(fluxMultiHopBlastFixture), 0644))

	prevNS, prevRec, prevFmt := graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusFormat
	t.Cleanup(func() {
		graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusFormat = prevNS, prevRec, prevFmt
		graphBlastRadiusCmd.SetOut(nil)
	})

	// JSON output via CLI handler
	graphBlastRadiusNamespace = "prod"
	graphBlastRadiusRecording = recPath
	graphBlastRadiusFormat = "json"
	var jsonBuf bytes.Buffer
	graphBlastRadiusCmd.SetOut(&jsonBuf)
	require.NoError(t, runGraphBlastRadius(graphBlastRadiusCmd, []string{"ConfigMap/app-cfg"}))

	var decoded blastRadiusOutput
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &decoded))
	require.Equal(t, "ConfigMap/prod/app-cfg", decoded.Target)
	require.Equal(t, explainStateExplained, decoded.Answer.State)
	require.LessOrEqual(t, len(bytes.TrimSpace(jsonBuf.Bytes())), 1200)

	// ASCII output via CLI handler
	graphBlastRadiusFormat = "ascii"
	var asciiBuf bytes.Buffer
	graphBlastRadiusCmd.SetOut(&asciiBuf)
	require.NoError(t, runGraphBlastRadius(graphBlastRadiusCmd, []string{"ConfigMap/app-cfg"}))
	require.Contains(t, asciiBuf.String(), "Blast Radius: ConfigMap/prod/app-cfg")
	require.Contains(t, asciiBuf.String(), "Deployment/prod/api")
	require.Contains(t, asciiBuf.String(), "PodDisruptionBudget/prod/api-pdb")
}

func TestGraphBlastRadiusLiveClusterAndSanitization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "live blast-radius must be strictly read-only")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/prod/configmaps":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"ConfigMapList","items":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-cfg","namespace":"prod","labels":{"app":"api"}},"data":{"SECRET_LIKE":"should-be-stripped"}}]}`))
		case "/api/v1/namespaces/prod/secrets":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"SecretList","items":[{"apiVersion":"v1","kind":"Secret","metadata":{"name":"db-creds","namespace":"prod"},"type":"Opaque","data":{"password":"c2VjcmV0"}}]}`))
		case "/apis/apps/v1/namespaces/prod/deployments":
			_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"prod"},"spec":{"template":{"metadata":{"labels":{"app":"api"}},"spec":{"containers":[{"name":"api","image":"ghcr.io/example/api:v1","envFrom":[{"configMapRef":{"name":"app-cfg"}},{"secretRef":{"name":"db-creds"}}]}]}}}}]}`))
		case "/apis/apps/v1/namespaces/prod/replicasets":
			_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"ReplicaSetList","items":[{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"api-rs","namespace":"prod","ownerReferences":[{"apiVersion":"apps/v1","kind":"Deployment","name":"api","uid":"dep-1"}]}}]}`))
		case "/api/v1/namespaces/prod/pods":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"PodList","items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"api-pod-1","namespace":"prod","ownerReferences":[{"apiVersion":"apps/v1","kind":"ReplicaSet","name":"api-rs","uid":"rs-1"}]},"spec":{"containers":[{"name":"api","image":"ghcr.io/example/api:v1","env":[{"name":"TOKEN","value":"sensitive"}]}]}}]}`))
		case "/apis/policy/v1/namespaces/prod/poddisruptionbudgets":
			_, _ = w.Write([]byte(`{"apiVersion":"policy/v1","kind":"PodDisruptionBudgetList","items":[{"apiVersion":"policy/v1","kind":"PodDisruptionBudget","metadata":{"name":"api-pdb","namespace":"prod"},"spec":{"minAvailable":1,"selector":{"matchLabels":{"app":"api"}}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prevNS, prevRec, prevCtx, prevFmt := graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat
	prevSessionFn := newBlastRadiusSessionForSelection
	prevNow := blastRadiusNow
	t.Cleanup(func() {
		graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat = prevNS, prevRec, prevCtx, prevFmt
		newBlastRadiusSessionForSelection = prevSessionFn
		blastRadiusNow = prevNow
		graphBlastRadiusCmd.SetOut(nil)
		if f := graphBlastRadiusCmd.Flags().Lookup("kube-context"); f != nil {
			f.Changed = false
		}
	})

	fixedTime := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	blastRadiusNow = func() time.Time { return fixedTime }
	var seenSelection clusterContextSelection
	newBlastRadiusSessionForSelection = func(sel clusterContextSelection) (*traceSession, error) {
		seenSelection = sel
		return newTraceSession(&rest.Config{Host: srv.URL}, sel.name)
	}

	// Verify sanitization directly on collected live objects.
	sess, err := newTraceSession(&rest.Config{Host: srv.URL}, "live-prod")
	require.NoError(t, err)
	dynClient, err := sess.dynamicClient()
	require.NoError(t, err)
	collected, truncated, err := collectLiveBlastRadiusObjects(context.Background(), dynClient, "prod", defaultSnapshotMaxObjects)
	require.NoError(t, err)
	require.False(t, truncated)
	for _, obj := range collected {
		if obj.GetKind() == "Secret" {
			_, hasData := obj.Object["data"]
			_, hasStringData := obj.Object["stringData"]
			require.False(t, hasData, "live blast-radius must strip Secret.data")
			require.False(t, hasStringData, "live blast-radius must strip Secret.stringData")
		}
		if obj.GetKind() == "ConfigMap" {
			_, hasData := obj.Object["data"]
			require.False(t, hasData, "live blast-radius must strip ConfigMap.data")
		}
	}

	// Run CLI in live mode with explicit --kube-context.
	graphBlastRadiusNamespace = "prod"
	graphBlastRadiusRecording = ""
	graphBlastRadiusFormat = "json"
	require.NoError(t, graphBlastRadiusCmd.Flags().Set("kube-context", "live-prod"))

	var outBuf bytes.Buffer
	graphBlastRadiusCmd.SetOut(&outBuf)
	require.NoError(t, runGraphBlastRadius(graphBlastRadiusCmd, []string{"ConfigMap/app-cfg"}))
	require.True(t, seenSelection.explicit)
	require.Equal(t, "live-prod", seenSelection.name)

	var decoded blastRadiusOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &decoded))
	require.Equal(t, "ConfigMap/prod/app-cfg", decoded.Target)
	require.Equal(t, "2026-11-27T08:00:00Z", decoded.ObservedAt)
	require.Equal(t, []blastRadiusNode{
		{Ref: "Deployment/prod/api", Via: "configmap-ref", Hops: 1},
		{Ref: "PodDisruptionBudget/prod/api-pdb", Via: "pdb-selector", Hops: 2},
		{Ref: "ReplicaSet/prod/api-rs", Via: "owner-ref", Hops: 2},
		{Ref: "Pod/prod/api-pod-1", Via: "owner-ref", Hops: 3},
	}, decoded.Descendants)
	require.Equal(t, []string{"Deployment/prod/api"}, decoded.AffectedWorkloads)
	require.NotNil(t, decoded.Answer)
	requireConformingAnswer(t, decoded.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, decoded.Answer.State)
	require.Equal(t, "live-prod", decoded.Answer.Cluster.Context)
	require.Equal(t, blastRadiusCostCoverage, decoded.Answer.Cost.Coverage)
	require.Greater(t, decoded.Answer.Cost.RequestsMade, int64(0))
	require.Empty(t, decoded.Answer.Unknowns)

	// Missing target in live namespace -> not-observable.
	outBuf.Reset()
	require.NoError(t, runGraphBlastRadius(graphBlastRadiusCmd, []string{"Deployment/missing"}))
	var missingOut blastRadiusOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &missingOut))
	require.Equal(t, explainStateNotObservable, missingOut.Answer.State)
	require.Equal(t, explainReasonObjectNotRead, missingOut.Answer.Reason)
	require.Equal(t, blastRadiusCostCoverage, missingOut.Answer.Cost.Coverage)
}

func TestGraphBlastRadiusKubeContextValidationAndMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	recPath := filepath.Join(dir, "snapshot.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(fluxMultiHopBlastFixture), 0o600))

	kubeconfigPath := filepath.Join(dir, "kubeconfig")
	kubeconfigYAML := `apiVersion: v1
kind: Config
current-context: valid-ctx
contexts:
- name: valid-ctx
  context:
    cluster: valid-cluster
    user: valid-user
clusters:
- name: valid-cluster
  cluster:
    server: https://127.0.0.1:6443
users:
- name: valid-user
  user:
    token: dummy
`
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfigYAML), 0o600))
	t.Setenv("KUBECONFIG", kubeconfigPath)

	prevNS, prevRec, prevCtx, prevFmt := graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat
	t.Cleanup(func() {
		graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat = prevNS, prevRec, prevCtx, prevFmt
		graphBlastRadiusCmd.SetOut(nil)
		if f := graphBlastRadiusCmd.Flags().Lookup("kube-context"); f != nil {
			f.Changed = false
		}
	})

	// 1. Mutual exclusion: --recording + --kube-context must be refused.
	graphBlastRadiusNamespace = "prod"
	graphBlastRadiusRecording = recPath
	graphBlastRadiusFormat = "json"
	require.NoError(t, graphBlastRadiusCmd.Flags().Set("kube-context", "valid-ctx"))

	err := runGraphBlastRadius(graphBlastRadiusCmd, []string{"ConfigMap/app-cfg"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--recording cannot be combined with --kube-context")

	// 2. Invalid --kube-context in live mode must be refused without ambient fallback.
	graphBlastRadiusRecording = ""
	require.NoError(t, graphBlastRadiusCmd.Flags().Set("kube-context", "nonexistent-ctx"))

	err = runGraphBlastRadius(graphBlastRadiusCmd, []string{"ConfigMap/app-cfg"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent-ctx")
}

func TestGraphBlastRadius_MCPToolRegistrationAndRecording(t *testing.T) {
	dir := t.TempDir()
	recPath := filepath.Join(dir, "flux-blast.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(fluxMultiHopBlastFixture), 0o600))

	prevNS, prevRec, prevCtx, prevFmt := graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat
	t.Cleanup(func() {
		graphBlastRadiusNamespace, graphBlastRadiusRecording, graphBlastRadiusKubeContext, graphBlastRadiusFormat = prevNS, prevRec, prevCtx, prevFmt
		graphBlastRadiusCmd.SetOut(nil)
		if f := graphBlastRadiusCmd.Flags().Lookup("kube-context"); f != nil {
			f.Changed = false
		}
	})

	var capturedArgs []string
	runner := func(_ context.Context, args []string) (string, error) {
		capturedArgs = append([]string(nil), args...)
		// Execute the real in-memory graph blast-radius logic for --recording calls.
		graphBlastRadiusNamespace = ""
		graphBlastRadiusRecording = ""
		graphBlastRadiusKubeContext = ""
		graphBlastRadiusFormat = "json"
		if f := graphBlastRadiusCmd.Flags().Lookup("kube-context"); f != nil {
			f.Changed = false
		}
		var target string
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "-n":
				i++
				graphBlastRadiusNamespace = args[i]
			case "--recording":
				i++
				graphBlastRadiusRecording = args[i]
			case "--format":
				i++
				graphBlastRadiusFormat = args[i]
			default:
				if target == "" {
					target = args[i]
				}
			}
		}
		var buf bytes.Buffer
		graphBlastRadiusCmd.SetOut(&buf)
		if err := runGraphBlastRadius(graphBlastRadiusCmd, []string{target}); err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	gateway := newMCPGateway(runner)
	tool, ok := gateway.tools["graph_blast_radius"]
	require.True(t, ok, "graph_blast_radius must be registered as a standalone MCP tool")
	require.NotNil(t, tool.Descriptor.Annotations)
	require.True(t, tool.Descriptor.Annotations.ReadOnlyHint)

	// 1. Recording execution through MCP gateway returns structuredContent and exact blast-radius output.
	reply := gateway.runNamedTool(context.Background(), "graph_blast_radius", tool, map[string]interface{}{
		"target":    "ConfigMap/app-cfg",
		"namespace": "prod",
		"recording": recPath,
	})
	require.Equal(t, false, reply["isError"], "reply: %v", reply)
	require.Equal(t, []string{"graph", "blast-radius", "ConfigMap/app-cfg", "-n", "prod", "--recording", recPath, "--format", "json"}, capturedArgs)
	require.NotNil(t, reply["structuredContent"])

	content, ok := reply["content"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, content, 1)
	var decoded blastRadiusOutput
	require.NoError(t, json.Unmarshal([]byte(content[0]["text"]), &decoded))
	require.Equal(t, "ConfigMap/prod/app-cfg", decoded.Target)
	require.Equal(t, []string{"Deployment/prod/api"}, decoded.AffectedWorkloads)
	require.False(t, decoded.Truncated)
	requireConformingAnswer(t, decoded.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, decoded.Answer.State)

	// 2. Live kube_context BuildArgs.
	liveArgs, err := tool.BuildArgs(map[string]interface{}{
		"target":       "ConfigMap/app-cfg",
		"namespace":    "prod",
		"kube_context": "prod-east",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"graph", "blast-radius", "ConfigMap/app-cfg", "-n", "prod", "--kube-context", "prod-east", "--format", "json"}, liveArgs)

	// 3. Reject mutual exclusion and malformed target.
	_, err = tool.BuildArgs(map[string]interface{}{
		"target":       "ConfigMap/app-cfg",
		"kube_context": "prod-east",
		"recording":    recPath,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "recording cannot be combined with kube_context")

	_, err = tool.BuildArgs(map[string]interface{}{"target": "no-slash"})
	require.Error(t, err)
}
