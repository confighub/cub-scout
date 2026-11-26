// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/stretchr/testify/require"
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
