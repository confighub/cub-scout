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

const recordedHelmLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: payments
  namespace: shop
  generation: 2
  labels:
    app.kubernetes.io/managed-by: Helm
  annotations:
    meta.helm.sh/release-name: payments
    meta.helm.sh/release-namespace: shop
spec:
  replicas: 1
status:
  observedGeneration: 2
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
apiVersion: v1
kind: Secret
metadata:
  name: sh.helm.release.v1.payments.v1
  namespace: shop
  labels:
    owner: helm
    name: payments
    version: "1"
    status: superseded
type: helm.sh/release.v1
---
apiVersion: v1
kind: Secret
metadata:
  name: sh.helm.release.v1.payments.v2
  namespace: shop
  labels:
    owner: helm
    name: payments
    version: "2"
    status: deployed
type: helm.sh/release.v1
`

func TestLandedOfflineRecording_Helm(t *testing.T) {
	helmSnap, err := loadRecordedObjectSnapshot(strings.NewReader(recordedHelmLandingYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: helmSnap.Objects}
	target := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "shop", Name: "payments"}

	snap := fetchLandingSnapshotFromStore(context.Background(), store, "", target)
	require.Equal(t, agent.OwnerHelm, snap.Owner)
	require.NotNil(t, snap.Deployer)
	require.Equal(t, "2", snap.Deployer.Revision)
	require.Equal(t, "Secret", snap.Deployer.Ref.Kind)
	require.Equal(t, "sh.helm.release.v1.payments.v2", snap.Deployer.Ref.Name)

	okEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, snap)
	require.Equal(t, agent.LandingStateLanded, okEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, okEval.Reason)

	wrongEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "3"}, snap)
	require.Equal(t, agent.LandingStateNotLanded, wrongEval.State)
	require.Equal(t, agent.LandingReasonControllerSettledOtherRevision, wrongEval.Reason)

	// Pending-upgrade release must report converging, never landed.
	pendingYAML := strings.ReplaceAll(recordedHelmLandingYAML, "status: deployed", "status: pending-upgrade")
	pendingSnapObj, err := loadRecordedObjectSnapshot(strings.NewReader(pendingYAML))
	require.NoError(t, err)
	pendingSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: pendingSnapObj.Objects}, "", target)
	pendingEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, pendingSnap)
	require.Equal(t, agent.LandingStateConverging, pendingEval.State)
	require.Equal(t, agent.LandingReasonControllerReconciling, pendingEval.Reason)

	// Failed release must report not-landed.
	failedYAML := strings.ReplaceAll(recordedHelmLandingYAML, "status: deployed", "status: failed")
	failedSnapObj, err := loadRecordedObjectSnapshot(strings.NewReader(failedYAML))
	require.NoError(t, err)
	failedSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: failedSnapObj.Objects}, "", target)
	failedEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, failedSnap)
	require.Equal(t, agent.LandingStateNotLanded, failedEval.State)
	require.Equal(t, agent.LandingReasonControllerReconcileFailed, failedEval.Reason)
}

const recordedFluxLandingWithPodsYAML = `apiVersion: apps/v1
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
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: ghcr.io/org/api:v1.2.3
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
---
apiVersion: v1
kind: Pod
metadata:
  name: api-pod-1
  namespace: prod
  labels:
    app: api
status:
  phase: Running
  containerStatuses:
    - name: api
      image: ghcr.io/org/api:v1.2.3
      imageID: docker-pullable://ghcr.io/org/api@sha256:1111222233334444
---
apiVersion: v1
kind: Pod
metadata:
  name: unrelated-pod
  namespace: prod
  labels:
    app: other
status:
  phase: Running
  containerStatuses:
    - name: other
      image: ghcr.io/org/other:v9.9.9
      imageID: docker-pullable://ghcr.io/org/other@sha256:9999999999999999
`

func TestLandedOfflineRecording_VerifyPodImages(t *testing.T) {
	recorded, err := loadRecordedObjectSnapshot(strings.NewReader(recordedFluxLandingWithPodsYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: recorded.Objects}
	target := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	anchor := agent.LandingAnchor{Kind: agent.AnchorKindOCIDigest, Value: "sha256:0828abcdef"}

	// Positive control: explicit expected pod digest matches running pod imageID.
	okSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), store, "", target, landingPodVerifyOptions{
		Verify:        true,
		ExpectedImage: "sha256:1111222233334444",
	})
	okEval := agent.EvaluateLanding(anchor, okSnap)
	require.Equal(t, agent.LandingStateLanded, okEval.State)
	require.Equal(t, agent.LandingReasonDigestAndWorkloadConverged, okEval.Reason)
	require.NotNil(t, okSnap.PodImages)
	require.Equal(t, "ready", okSnap.PodImages.Status)

	// Positive control: no --image supplied falls back to workload spec.template container image.
	specSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), store, "", target, landingPodVerifyOptions{
		Verify: true,
	})
	specEval := agent.EvaluateLanding(anchor, specSnap)
	require.Equal(t, agent.LandingStateLanded, specEval.State)

	// Negative control: bogus expected digest disagrees despite Ready controller and Ready Deployment.
	badSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), store, "", target, landingPodVerifyOptions{
		Verify:        true,
		ExpectedImage: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	badEval := agent.EvaluateLanding(anchor, badSnap)
	require.Equal(t, agent.LandingStateNotLanded, badEval.State)
	require.Equal(t, agent.LandingReasonWorkloadFailed, badEval.Reason)
	require.NotNil(t, badEval.BrokenLink)
	require.Equal(t, agent.LandingStageWorkload, badEval.BrokenLink.Stage)
	require.NotNil(t, badEval.BrokenLink.Ref)
	require.Equal(t, "Pod", badEval.BrokenLink.Ref.Kind)
	require.Equal(t, "api-pod-1", badEval.BrokenLink.Ref.Name)

	// Negative control: no running pods matching selector must never claim landed.
	noPodYAML := strings.ReplaceAll(recordedFluxLandingWithPodsYAML, "phase: Running", "phase: Pending")
	noPodRecorded, err := loadRecordedObjectSnapshot(strings.NewReader(noPodYAML))
	require.NoError(t, err)
	noPodSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), recordedLandingStore{objects: noPodRecorded.Objects}, "", target, landingPodVerifyOptions{
		Verify:        true,
		ExpectedImage: "sha256:1111222233334444",
	})
	noPodEval := agent.EvaluateLanding(anchor, noPodSnap)
	require.Equal(t, agent.LandingStateNotLanded, noPodEval.State)
	require.Equal(t, agent.LandingReasonWorkloadFailed, noPodEval.Reason)
	require.NotNil(t, noPodEval.BrokenLink)
	require.Equal(t, agent.LandingStageWorkload, noPodEval.BrokenLink.Stage)
}
