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

const recordedJobAndCronJobYAML = `apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
status:
  succeeded: 1
  conditions:
    - type: Complete
      status: "True"
---
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate-failed
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
status:
  failed: 3
  conditions:
    - type: Failed
      status: "True"
      reason: BackoffLimitExceeded
      message: Job has reached the specified backoff limit
---
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate-running
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
status:
  active: 1
  failed: 1
---
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate-suspended
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  suspend: true
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly-report
  namespace: prod
  labels:
    argocd.argoproj.io/instance: reporting
status:
  lastScheduleTime: "2026-11-24T03:00:00Z"
  lastSuccessfulTime: "2026-11-24T03:02:15Z"
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: hourly-sync
  namespace: prod
  labels:
    argocd.argoproj.io/instance: reporting
status:
  lastScheduleTime: "2026-11-24T15:00:00Z"
  active:
    - apiVersion: batch/v1
      kind: Job
      name: hourly-sync-28876200
      namespace: prod
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: never-scheduled
  namespace: prod
  labels:
    argocd.argoproj.io/instance: reporting
status: {}
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: paused-report
  namespace: prod
  labels:
    argocd.argoproj.io/instance: reporting
spec:
  suspend: true
status:
  lastScheduleTime: "2026-11-20T03:00:00Z"
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
status:
  lastAppliedRevision: main@sha1:a1b2c3d4e5f6789012345678901234567890abcd
  conditions:
    - type: Ready
      status: "True"
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: reporting
  namespace: argocd
status:
  sync:
    status: Synced
    revision: a1b2c3d4e5f6789012345678901234567890abcd
  health:
    status: Healthy
`

func TestLandedOfflineRecording_JobAndCronJob(t *testing.T) {
	// Verify parseLandedTarget canonicalization and batch/v1 APIVersion.
	jobRef, err := parseLandedTarget([]string{"job/db-migrate"}, "prod")
	require.NoError(t, err)
	require.Equal(t, "batch/v1", jobRef.APIVersion)
	require.Equal(t, "Job", jobRef.Kind)

	cjRef, err := parseLandedTarget([]string{"cj/nightly-report"}, "prod")
	require.NoError(t, err)
	require.Equal(t, "batch/v1", cjRef.APIVersion)
	require.Equal(t, "CronJob", cjRef.Kind)

	snapObj, err := loadRecordedObjectSnapshot(strings.NewReader(recordedJobAndCronJobYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: snapObj.Objects}
	anchor := agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "a1b2c3d4e5f6789012345678901234567890abcd"}

	// Job succeeded -> landed.
	okSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", jobRef)
	okEval := agent.EvaluateLanding(anchor, okSnap)
	require.Equal(t, agent.LandingStateLanded, okEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, okEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(okEval, nil).Answer, landedStates, landedReasons)

	// Job failed with Failed condition -> not-landed (workload-failed).
	failRef, err := parseLandedTarget([]string{"jobs", "db-migrate-failed"}, "prod")
	require.NoError(t, err)
	failSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", failRef)
	failEval := agent.EvaluateLanding(anchor, failSnap)
	require.Equal(t, agent.LandingStateNotLanded, failEval.State)
	require.Equal(t, agent.LandingReasonWorkloadFailed, failEval.Reason)
	require.NotNil(t, failEval.BrokenLink)
	require.Equal(t, agent.LandingStageWorkload, failEval.BrokenLink.Stage)

	// Job active (even with prior failed retry, no terminal Failed condition) -> converging.
	runRef, err := parseLandedTarget([]string{"job/db-migrate-running"}, "prod")
	require.NoError(t, err)
	runSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", runRef)
	runEval := agent.EvaluateLanding(anchor, runSnap)
	require.Equal(t, agent.LandingStateConverging, runEval.State)
	require.Equal(t, agent.LandingReasonWorkloadRollingOut, runEval.Reason)

	// Job suspended without completion -> not-observable (never false landed).
	suspRef, err := parseLandedTarget([]string{"job/db-migrate-suspended"}, "prod")
	require.NoError(t, err)
	suspSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", suspRef)
	suspEval := agent.EvaluateLanding(anchor, suspSnap)
	require.Equal(t, agent.LandingStateNotObservable, suspEval.State)

	// CronJob with lastScheduleTime and no active Jobs -> landed.
	cjOKSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", cjRef)
	cjOKEval := agent.EvaluateLanding(anchor, cjOKSnap)
	require.Equal(t, agent.LandingStateLanded, cjOKEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, cjOKEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(cjOKEval, nil).Answer, landedStates, landedReasons)

	// CronJob with active Job -> converging.
	cjActiveRef, err := parseLandedTarget([]string{"cronjob/hourly-sync"}, "prod")
	require.NoError(t, err)
	cjActiveSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", cjActiveRef)
	cjActiveEval := agent.EvaluateLanding(anchor, cjActiveSnap)
	require.Equal(t, agent.LandingStateConverging, cjActiveEval.State)
	require.Equal(t, agent.LandingReasonWorkloadRollingOut, cjActiveEval.Reason)

	// CronJob never scheduled -> converging (never false landed).
	cjNeverRef, err := parseLandedTarget([]string{"cronjobs", "never-scheduled"}, "prod")
	require.NoError(t, err)
	cjNeverSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", cjNeverRef)
	cjNeverEval := agent.EvaluateLanding(anchor, cjNeverSnap)
	require.Equal(t, agent.LandingStateConverging, cjNeverEval.State)

	// CronJob suspended -> not-observable (never false landed).
	cjPausedRef, err := parseLandedTarget([]string{"cronjob/paused-report"}, "prod")
	require.NoError(t, err)
	cjPausedSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", cjPausedRef)
	cjPausedEval := agent.EvaluateLanding(anchor, cjPausedSnap)
	require.Equal(t, agent.LandingStateNotObservable, cjPausedEval.State)
}

const recordedKroLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: lane-web
  namespace: kro-lane
  generation: 1
  labels:
    app.kubernetes.io/managed-by: kro
    kro.run/owned: "true"
    kro.run/kro-version: v0.10.0
    kro.run/instance-group: kro.run
    kro.run/instance-version: v1alpha1
    kro.run/instance-kind: LaneApp
    kro.run/instance-name: lane
    kro.run/instance-namespace: kro-lane
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
apiVersion: kro.run/v1alpha1
kind: LaneApp
metadata:
  name: lane
  namespace: kro-lane
  generation: 2
status:
  state: ACTIVE
  conditions:
    - type: Ready
      status: "True"
      observedGeneration: 2
      reason: Ready
      message: Instance is ready
`

func TestLandedOfflineRecording_Kro(t *testing.T) {
	kroSnapObj, err := loadRecordedObjectSnapshot(strings.NewReader(recordedKroLandingYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: kroSnapObj.Objects}
	target := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "kro-lane", Name: "lane-web"}

	snap := fetchLandingSnapshotFromStore(context.Background(), store, "", target)
	require.Equal(t, agent.OwnerKro, snap.Owner)
	require.NotNil(t, snap.Deployer)
	require.Equal(t, "LaneApp", snap.Deployer.Ref.Kind)
	require.Equal(t, "lane", snap.Deployer.Ref.Name)
	require.Equal(t, "2", snap.Deployer.Revision)

	okEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, snap)
	require.Equal(t, agent.LandingStateLanded, okEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, okEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(okEval, nil).Answer, landedStates, landedReasons)

	wrongEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "99"}, snap)
	require.Equal(t, agent.LandingStateNotLanded, wrongEval.State)
	require.Equal(t, agent.LandingReasonControllerSettledOtherRevision, wrongEval.Reason)

	// Generation lag (metadata.generation 3 > observedGeneration 2) -> converging.
	lagYAML := strings.Replace(recordedKroLandingYAML, "  generation: 2", "  generation: 3", 1)
	lagObj, err := loadRecordedObjectSnapshot(strings.NewReader(lagYAML))
	require.NoError(t, err)
	lagSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: lagObj.Objects}, "", target)
	lagEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "3"}, lagSnap)
	require.Equal(t, agent.LandingStateConverging, lagEval.State)
	require.Equal(t, agent.LandingReasonControllerReconciling, lagEval.Reason)

	// Ready=False -> not-landed.
	failYAML := strings.Replace(recordedKroLandingYAML, "    - type: Ready\n      status: \"True\"", "    - type: Ready\n      status: \"False\"", 1)
	failObj, err := loadRecordedObjectSnapshot(strings.NewReader(failYAML))
	require.NoError(t, err)
	failSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: failObj.Objects}, "", target)
	failEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, failSnap)
	require.Equal(t, agent.LandingStateNotLanded, failEval.State)
	require.Equal(t, agent.LandingReasonControllerReconcileFailed, failEval.Reason)

	// Missing observedGeneration -> not-observable with explicit unknown.
	noGenYAML := strings.Replace(recordedKroLandingYAML, "      observedGeneration: 2\n", "", 1)
	noGenObj, err := loadRecordedObjectSnapshot(strings.NewReader(noGenYAML))
	require.NoError(t, err)
	noGenSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: noGenObj.Objects}, "", target)
	noGenEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "2"}, noGenSnap)
	require.Equal(t, agent.LandingStateNotObservable, noGenEval.State)
	require.Equal(t, agent.LandingReasonUnsupportedController, noGenEval.Reason)
	noGenOut := buildLandedOutput(noGenEval, nil)
	requireConformingAnswer(t, noGenOut.Answer, landedStates, landedReasons)
	require.NotEmpty(t, noGenOut.Answer.Unknowns)
	require.Contains(t, noGenOut.Answer.Unknowns[0].Reason, "observedGeneration")
}

const recordedCrossplaneLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: lane-abc12
  namespace: xp-lane
  generation: 1
  labels:
    crossplane.io/composite: lane
  annotations:
    crossplane.io/composition-resource-name: deployment
  ownerReferences:
    - apiVersion: lane.example.org/v1
      kind: XLaneApp
      name: lane
      controller: true
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
apiVersion: lane.example.org/v1
kind: XLaneApp
metadata:
  name: lane
  namespace: xp-lane
  generation: 1
status:
  conditions:
    - type: Synced
      status: "True"
      reason: ReconcileSuccess
      observedGeneration: 1
    - type: Ready
      status: "True"
      reason: Available
      observedGeneration: 1
`

func TestLandedOfflineRecording_Crossplane(t *testing.T) {
	xpSnapObj, err := loadRecordedObjectSnapshot(strings.NewReader(recordedCrossplaneLandingYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: xpSnapObj.Objects}
	target := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "xp-lane", Name: "lane-abc12"}

	snap := fetchLandingSnapshotFromStore(context.Background(), store, "", target)
	require.Equal(t, agent.OwnerCrossplane, snap.Owner)
	require.NotNil(t, snap.Deployer)
	require.Equal(t, "XLaneApp", snap.Deployer.Ref.Kind)
	require.Equal(t, "lane", snap.Deployer.Ref.Name)
	require.Equal(t, "1", snap.Deployer.Revision)

	okEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "1"}, snap)
	require.Equal(t, agent.LandingStateLanded, okEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, okEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(okEval, nil).Answer, landedStates, landedReasons)

	wrongEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "99"}, snap)
	require.Equal(t, agent.LandingStateNotLanded, wrongEval.State)
	require.Equal(t, agent.LandingReasonControllerSettledOtherRevision, wrongEval.Reason)

	// Ready=False with reason Creating -> converging.
	creatingYAML := strings.Replace(recordedCrossplaneLandingYAML, "    - type: Ready\n      status: \"True\"\n      reason: Available", "    - type: Ready\n      status: \"False\"\n      reason: Creating", 1)
	creatingObj, err := loadRecordedObjectSnapshot(strings.NewReader(creatingYAML))
	require.NoError(t, err)
	creatingSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: creatingObj.Objects}, "", target)
	creatingEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "1"}, creatingSnap)
	require.Equal(t, agent.LandingStateConverging, creatingEval.State)
	require.Equal(t, agent.LandingReasonControllerReconciling, creatingEval.Reason)

	// Synced=False -> not-landed.
	failYAML := strings.Replace(recordedCrossplaneLandingYAML, "    - type: Synced\n      status: \"True\"\n      reason: ReconcileSuccess", "    - type: Synced\n      status: \"False\"\n      reason: ReconcileError", 1)
	failObj, err := loadRecordedObjectSnapshot(strings.NewReader(failYAML))
	require.NoError(t, err)
	failSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: failObj.Objects}, "", target)
	failEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "1"}, failSnap)
	require.Equal(t, agent.LandingStateNotLanded, failEval.State)
	require.Equal(t, agent.LandingReasonControllerReconcileFailed, failEval.Reason)

	// Missing observedGeneration -> not-observable (never false landed).
	noGenYAML := strings.ReplaceAll(recordedCrossplaneLandingYAML, "      observedGeneration: 1\n", "")
	noGenObj, err := loadRecordedObjectSnapshot(strings.NewReader(noGenYAML))
	require.NoError(t, err)
	noGenSnap := fetchLandingSnapshotFromStore(context.Background(), recordedLandingStore{objects: noGenObj.Objects}, "", target)
	noGenEval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "1"}, noGenSnap)
	require.Equal(t, agent.LandingStateNotObservable, noGenEval.State)
	require.Equal(t, agent.LandingReasonUnsupportedController, noGenEval.Reason)
	noGenOut := buildLandedOutput(noGenEval, nil)
	requireConformingAnswer(t, noGenOut.Answer, landedStates, landedReasons)
	require.NotEmpty(t, noGenOut.Answer.Unknowns)
}

const recordedSveltosLandingYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: webster
  namespace: prod
  generation: 1
  annotations:
    projectsveltos.io/owner-kind: ClusterProfile
    projectsveltos.io/owner-name: config-to-production
    projectsveltos.io/deployed-by-sveltos: "true"
    projectsveltos.io/hash: sha256-internal-policy-hash
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
`

func TestLandedOfflineRecording_Sveltos(t *testing.T) {
	svSnapObj, err := loadRecordedObjectSnapshot(strings.NewReader(recordedSveltosLandingYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: svSnapObj.Objects}
	target := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "webster"}

	snap := fetchLandingSnapshotFromStore(context.Background(), store, "", target)
	require.Equal(t, agent.OwnerSveltos, snap.Owner)
	require.NotNil(t, snap.Deployer)
	require.Equal(t, "ClusterProfile", snap.Deployer.Ref.Kind)
	require.Equal(t, "config-to-production", snap.Deployer.Ref.Name)

	eval := agent.EvaluateLanding(agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "a1b2c3d"}, snap)
	require.Equal(t, agent.LandingStateNotObservable, eval.State)
	require.Equal(t, agent.LandingReasonUnsupportedController, eval.Reason)

	out := buildLandedOutput(eval, nil)
	requireConformingAnswer(t, out.Answer, landedStates, landedReasons)
	require.NotEmpty(t, out.Answer.Unknowns)
	require.Equal(t, "Sveltos revision anchor", out.Answer.Unknowns[0].What)
	require.Contains(t, out.Answer.Unknowns[0].Reason, "Sveltos")
	require.Contains(t, out.Answer.Unknowns[0].Reason, "Git or OCI")
}

const recordedArgoRolloutLandingYAML = `apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout
  namespace: shop
  generation: 3
  labels:
    argocd.argoproj.io/instance: checkout-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: checkout
  template:
    metadata:
      labels:
        app: checkout
    spec:
      containers:
        - name: checkout
          image: ghcr.io/org/checkout:v2.0.0
status:
  observedGeneration: "3"
  phase: Healthy
  message: Rollout is healthy
---
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout-paused
  namespace: shop
  generation: 3
  labels:
    argocd.argoproj.io/instance: checkout-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: checkout-paused
status:
  observedGeneration: "3"
  phase: Paused
  message: CanaryPauseStep
---
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout-progressing
  namespace: shop
  generation: 3
  labels:
    argocd.argoproj.io/instance: checkout-app
spec:
  replicas: 2
status:
  observedGeneration: "3"
  phase: Progressing
  message: ReplicaSet updated
---
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout-lag
  namespace: shop
  generation: 4
  labels:
    argocd.argoproj.io/instance: checkout-app
spec:
  replicas: 2
status:
  observedGeneration: "3"
  phase: Healthy
---
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout-degraded
  namespace: shop
  generation: 3
  labels:
    argocd.argoproj.io/instance: checkout-app
spec:
  replicas: 2
status:
  observedGeneration: "3"
  phase: Degraded
  message: ProgressDeadlineExceeded
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: checkout-app
  namespace: argocd
status:
  sync:
    status: Synced
    revision: a1b2c3d4e5f6789012345678901234567890abcd
  health:
    status: Healthy
---
apiVersion: v1
kind: Pod
metadata:
  name: checkout-pod-1
  namespace: shop
  labels:
    app: checkout
status:
  phase: Running
  containerStatuses:
    - name: checkout
      image: ghcr.io/org/checkout:v2.0.0
      imageID: docker-pullable://ghcr.io/org/checkout@sha256:aaaa1111bbbb2222
`

func TestLandedOfflineRecording_ArgoRollout(t *testing.T) {
	// Verify parseLandedTarget canonicalization for ro/rollout/rollouts.
	for _, args := range [][]string{
		{"ro/checkout"},
		{"rollout/checkout"},
		{"rollouts", "checkout"},
	} {
		ref, err := parseLandedTarget(args, "shop")
		require.NoError(t, err)
		require.Equal(t, "argoproj.io/v1alpha1", ref.APIVersion)
		require.Equal(t, "Rollout", ref.Kind)
		require.Equal(t, "shop", ref.Namespace)
		require.Equal(t, "checkout", ref.Name)
	}

	recorded, err := loadRecordedObjectSnapshot(strings.NewReader(recordedArgoRolloutLandingYAML))
	require.NoError(t, err)
	store := recordedLandingStore{objects: recorded.Objects}
	anchor := agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: "a1b2c3d4e5f6789012345678901234567890abcd"}

	// 1. Healthy Rollout with observedGeneration == generation -> landed.
	healthyRef, err := parseLandedTarget([]string{"ro/checkout"}, "shop")
	require.NoError(t, err)
	healthySnap := fetchLandingSnapshotFromStore(context.Background(), store, "", healthyRef)
	healthyEval := agent.EvaluateLanding(anchor, healthySnap)
	require.Equal(t, agent.LandingStateLanded, healthyEval.State)
	require.Equal(t, agent.LandingReasonRevisionAndWorkloadConverged, healthyEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(healthyEval, nil).Answer, landedStates, landedReasons)

	// 2. Paused canary Rollout -> converging (never false landed!).
	pausedRef, err := parseLandedTarget([]string{"rollout/checkout-paused"}, "shop")
	require.NoError(t, err)
	pausedSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", pausedRef)
	pausedEval := agent.EvaluateLanding(anchor, pausedSnap)
	require.Equal(t, agent.LandingStateConverging, pausedEval.State)
	require.Equal(t, agent.LandingReasonWorkloadRollingOut, pausedEval.Reason)
	requireConformingAnswer(t, buildLandedOutput(pausedEval, nil).Answer, landedStates, landedReasons)

	// 3. Progressing Rollout -> converging.
	progRef, err := parseLandedTarget([]string{"rollouts", "checkout-progressing"}, "shop")
	require.NoError(t, err)
	progSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", progRef)
	progEval := agent.EvaluateLanding(anchor, progSnap)
	require.Equal(t, agent.LandingStateConverging, progEval.State)
	require.Equal(t, agent.LandingReasonWorkloadRollingOut, progEval.Reason)

	// 4. Healthy phase with observedGeneration lag -> converging (never false landed!).
	lagRef, err := parseLandedTarget([]string{"ro/checkout-lag"}, "shop")
	require.NoError(t, err)
	lagSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", lagRef)
	lagEval := agent.EvaluateLanding(anchor, lagSnap)
	require.Equal(t, agent.LandingStateConverging, lagEval.State)
	require.Equal(t, agent.LandingReasonWorkloadRollingOut, lagEval.Reason)

	// 5. Degraded Rollout -> not-landed (workload-failed).
	degRef, err := parseLandedTarget([]string{"rollout/checkout-degraded"}, "shop")
	require.NoError(t, err)
	degSnap := fetchLandingSnapshotFromStore(context.Background(), store, "", degRef)
	degEval := agent.EvaluateLanding(anchor, degSnap)
	require.Equal(t, agent.LandingStateNotLanded, degEval.State)
	require.Equal(t, agent.LandingReasonWorkloadFailed, degEval.Reason)
	require.NotNil(t, degEval.BrokenLink)
	require.Equal(t, agent.LandingStageWorkload, degEval.BrokenLink.Stage)
	requireConformingAnswer(t, buildLandedOutput(degEval, nil).Answer, landedStates, landedReasons)

	// 6. --verify-pod-images positive match on Healthy Rollout -> landed.
	podOKSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), store, "", healthyRef, landingPodVerifyOptions{
		Verify:        true,
		ExpectedImage: "sha256:aaaa1111bbbb2222",
	})
	podOKEval := agent.EvaluateLanding(anchor, podOKSnap)
	require.Equal(t, agent.LandingStateLanded, podOKEval.State)
	require.NotNil(t, podOKSnap.PodImages)
	require.Equal(t, "ready", podOKSnap.PodImages.Status)

	// 7. --verify-pod-images mismatch on Healthy Rollout -> not-landed (workload-failed).
	podBadSnap := fetchLandingSnapshotFromStoreWithOptions(context.Background(), store, "", healthyRef, landingPodVerifyOptions{
		Verify:        true,
		ExpectedImage: "sha256:0000000000000000",
	})
	podBadEval := agent.EvaluateLanding(anchor, podBadSnap)
	require.Equal(t, agent.LandingStateNotLanded, podBadEval.State)
	require.Equal(t, agent.LandingReasonWorkloadFailed, podBadEval.Reason)
	require.NotNil(t, podBadEval.BrokenLink)
	require.Equal(t, agent.LandingStageWorkload, podBadEval.BrokenLink.Stage)
	require.NotNil(t, podBadEval.BrokenLink.Ref)
	require.Equal(t, "Pod", podBadEval.BrokenLink.Ref.Kind)
	require.Equal(t, "checkout-pod-1", podBadEval.BrokenLink.Ref.Name)
	requireConformingAnswer(t, buildLandedOutput(podBadEval, nil).Answer, landedStates, landedReasons)
}
