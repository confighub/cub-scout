// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/confighub/cub-scout/v3/pkg/agent"
)

const recordedFluxLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  generation: 2
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  replicas: 2
status:
  observedGeneration: 2
  replicas: 2
  updatedReplicas: 2
  readyReplicas: 2
  availableReplicas: 2
  conditions:
    - type: Available
      status: "True"
    - type: Progressing
      status: "True"
      reason: NewReplicaSetAvailable
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  sourceRef:
    kind: OCIRepository
    name: apps-oci
    namespace: flux-system
status:
  lastAppliedRevision: latest@sha256:0828abcdef
  conditions:
    - type: Ready
      status: "True"
      reason: ReconciliationSucceeded
      message: Applied revision latest@sha256:0828abcdef
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: apps-oci
  namespace: flux-system
status:
  artifact:
    revision: latest@sha256:0828abcdef
  conditions:
    - type: Ready
      status: "True"
      reason: Succeeded
`

const recordedArgoLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: guestbook-ui
  namespace: guestbook
  generation: 1
  labels:
    argocd.argoproj.io/instance: guestbook
spec:
  replicas: 1
status:
  observedGeneration: 1
  replicas: 1
  updatedReplicas: 1
  readyReplicas: 1
  availableReplicas: 1
  conditions:
    - type: Available
      status: "True"
    - type: Progressing
      status: "True"
      reason: NewReplicaSetAvailable
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: argocd
status:
  sync:
    status: Synced
    revision: a1b2c3d4e5f67890
  health:
    status: Healthy
`

func TestLandedOfflineRecording_FluxAndArgo(t *testing.T) {
	fluxSnap, err := loadRecordedObjectSnapshot(strings.NewReader(recordedFluxLandingYAML))
	require.NoError(t, err)
	fluxStore := recordedLandingStore{objects: fluxSnap.Objects}
	fluxTarget := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}

	snap := fetchLandingSnapshotFromStore(context.Background(), fluxStore, "", fluxTarget)
	evalOK := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindOCIDigest, Value: "sha256:0828abcdef"}, snap)
	require.Equal(t, agent.LandingStateLanded, evalOK.State)
	require.Equal(t, agent.LandingReasonDigestAndWorkloadConverged, evalOK.Reason)

	evalWrong := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindOCIDigest, Value: "sha256:deadbeef"}, snap)
	require.Equal(t, agent.LandingStateNotLanded, evalWrong.State)
	require.Equal(t, agent.LandingReasonControllerSettledOtherRevision, evalWrong.Reason)

	argoSnap, err := loadRecordedObjectSnapshot(strings.NewReader(recordedArgoLandingYAML))
	require.NoError(t, err)
	argoStore := recordedLandingStore{objects: argoSnap.Objects}
	argoTarget := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "guestbook", Name: "guestbook-ui"}

	aSnap := fetchLandingSnapshotFromStore(context.Background(), argoStore, "", argoTarget)
	aOK := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "a1b2c3d"}, aSnap)
	require.Equal(t, agent.LandingStateLanded, aOK.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, aOK.Reason)

	aWrong := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "0000000"}, aSnap)
	require.Equal(t, agent.LandingStateNotLanded, aWrong.State)
	require.Equal(t, agent.LandingReasonControllerSettledOtherRevision, aWrong.Reason)

	missingSnap := fetchLandingSnapshotFromStore(context.Background(), argoStore, "", agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "guestbook", Name: "missing"})
	missingEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "a1b2c3d"}, missingSnap)
	require.Equal(t, agent.LandingStateNotObservable, missingEval.State)
	require.Equal(t, agent.LandingReasonTargetNotFound, missingEval.Reason)
}
