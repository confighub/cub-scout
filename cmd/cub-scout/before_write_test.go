// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

func TestBeforeWrite_FluxSuspendedAndActive(t *testing.T) {
	dep := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "api",
			"namespace": "prod",
			"labels": map[string]interface{}{
				"kustomize.toolkit.fluxcd.io/name":      "apps",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
			"managedFields": []interface{}{
				map[string]interface{}{
					"manager":    "kustomize-controller",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]interface{}{
						"f:spec": map[string]interface{}{
							"f:replicas": map[string]interface{}{},
						},
					},
				},
			},
		},
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "api"},
				},
			},
		},
	}}

	ksSuspended := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]interface{}{
			"suspend": true,
		},
	}}

	out := evaluateBeforeWrite(context.Background(), recordedBeforeWriteStore{
		objects: []*unstructured.Unstructured{dep, ksSuspended},
	}, beforeWriteOptions{
		Context:    "prod-east",
		Kind:       "Deployment",
		Name:       "api",
		Namespace:  "prod",
		FieldPath:  ".spec.replicas",
		ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	})

	require.Equal(t, agent.OwnerFlux, out.Owner.Type)
	require.NotNil(t, out.Deployer)
	require.NotNil(t, out.Deployer.Suspended)
	require.True(t, *out.Deployer.Suspended)
	require.NotNil(t, out.FieldAttribution)
	require.True(t, out.FieldAttribution.ValueFound)
	require.Equal(t, int64(3), out.FieldAttribution.Value)
	require.NotNil(t, out.FieldAttribution.RevertPrediction)
	require.Equal(t, agent.RevertVerdictNotExpected, out.FieldAttribution.RevertPrediction.Verdict)
	require.Equal(t, agent.RevertReasonFluxSuspended, out.FieldAttribution.RevertPrediction.Reason)

	requireConformingAnswer(t, out.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, out.Answer.State)
	rawAns, err := json.Marshal(out.Answer)
	require.NoError(t, err)
	require.Less(t, len(rawAns), 1024, "answer block must stay under 1 KB, got %d bytes", len(rawAns))

	// Active Flux deployer (suspend=false) predicts likely-reverted.
	ksActive := ksSuspended.DeepCopy()
	_ = unstructured.SetNestedField(ksActive.Object, false, "spec", "suspend")
	activeOut := evaluateBeforeWrite(context.Background(), recordedBeforeWriteStore{
		objects: []*unstructured.Unstructured{dep, ksActive},
	}, beforeWriteOptions{
		Context:    "prod-east",
		Kind:       "Deployment",
		Name:       "api",
		Namespace:  "prod",
		FieldPath:  ".spec.replicas",
		ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	})
	require.NotNil(t, activeOut.Deployer.Suspended)
	require.False(t, *activeOut.Deployer.Suspended)
	require.Equal(t, agent.RevertVerdictLikelyReverted, activeOut.FieldAttribution.RevertPrediction.Verdict)
	require.Equal(t, agent.RevertReasonFluxActive, activeOut.FieldAttribution.RevertPrediction.Reason)
}

func TestBeforeWrite_ArgoSelfHealSyncWindowsAndPDBAndContention(t *testing.T) {
	dep := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "checkout",
			"namespace": "shop",
			"labels": map[string]interface{}{
				"argocd.argoproj.io/instance": "checkout-app",
			},
			"managedFields": []interface{}{
				map[string]interface{}{
					"manager":    "argocd-controller",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]interface{}{
						"f:spec": map[string]interface{}{
							"f:replicas": map[string]interface{}{},
						},
					},
				},
				map[string]interface{}{
					"manager":    "agent/remediator/run-42",
					"operation":  "Apply",
					"fieldsType": "FieldsV1",
					"fieldsV1": map[string]interface{}{
						"f:spec": map[string]interface{}{
							"f:replicas": map[string]interface{}{},
						},
					},
				},
			},
		},
		"spec": map[string]interface{}{
			"replicas": int64(2),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "checkout", "tier": "backend"},
				},
			},
		},
	}}

	argoApp := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "checkout-app",
			"namespace": "argocd",
		},
		"spec": map[string]interface{}{
			"project": "payments",
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    true,
					"selfHeal": true,
				},
				"syncOptions": []interface{}{"CreateNamespace=true"},
			},
			"syncWindows": []interface{}{
				map[string]interface{}{
					"kind":       "deny",
					"schedule":   "0 22 * * *",
					"duration":   "4h",
					"timeZone":   "UTC",
					"manualSync": false,
				},
			},
		},
	}}

	matchingPDB := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policy/v1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      "checkout-pdb",
			"namespace": "shop",
		},
		"spec": map[string]interface{}{
			"minAvailable": int64(1),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "checkout"},
			},
		},
		"status": map[string]interface{}{
			"disruptionsAllowed": int64(1),
			"currentHealthy":     int64(2),
			"desiredHealthy":     int64(1),
		},
	}}

	unrelatedPDB := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policy/v1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      "other-pdb",
			"namespace": "shop",
		},
		"spec": map[string]interface{}{
			"maxUnavailable": "25%",
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "billing"},
			},
		},
	}}

	out := evaluateBeforeWrite(context.Background(), recordedBeforeWriteStore{
		objects: []*unstructured.Unstructured{dep, argoApp, matchingPDB, unrelatedPDB},
	}, beforeWriteOptions{
		Context:    "prod-west",
		Kind:       "Deployment",
		Name:       "checkout",
		Namespace:  "shop",
		FieldPath:  ".spec.replicas",
		ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	})

	require.Equal(t, agent.OwnerArgo, out.Owner.Type)
	require.NotNil(t, out.Deployer)
	require.NotNil(t, out.Deployer.AutoSync)
	require.True(t, *out.Deployer.AutoSync)
	require.NotNil(t, out.Deployer.SelfHeal)
	require.True(t, *out.Deployer.SelfHeal)
	require.NotNil(t, out.Deployer.Prune)
	require.True(t, *out.Deployer.Prune)
	require.Equal(t, []string{"CreateNamespace=true"}, out.Deployer.SyncOptions)
	require.Len(t, out.Deployer.SyncWindows, 1)
	require.Equal(t, "deny", out.Deployer.SyncWindows[0].Kind)
	require.Equal(t, "0 22 * * *", out.Deployer.SyncWindows[0].Schedule)
	require.Equal(t, "4h", out.Deployer.SyncWindows[0].Duration)

	// Field attribution: revert prediction + writer contention.
	require.NotNil(t, out.FieldAttribution)
	require.Equal(t, agent.RevertVerdictLikelyReverted, out.FieldAttribution.RevertPrediction.Verdict)
	require.Equal(t, agent.RevertReasonArgoSelfHealEnabled, out.FieldAttribution.RevertPrediction.Reason)
	require.NotNil(t, out.FieldAttribution.Contention)
	require.True(t, out.FieldAttribution.Contention.Contested)
	require.Equal(t, []string{"agent/remediator", "controller"}, out.FieldAttribution.Contention.Categories)

	// Matching PDBs only.
	require.Len(t, out.PDBs, 1)
	require.Equal(t, "checkout-pdb", out.PDBs[0].Name)
	require.Equal(t, "1", out.PDBs[0].MinAvailable)
	require.NotNil(t, out.PDBs[0].DisruptionsAllowed)
	require.Equal(t, int64(1), *out.PDBs[0].DisruptionsAllowed)

	// Conformance + < 1 KB answer envelope.
	requireConformingAnswer(t, out.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, out.Answer.State)
	rawAns, err := json.Marshal(out.Answer)
	require.NoError(t, err)
	require.Less(t, len(rawAns), 1024, "answer block must stay under 1 KB, got %d bytes", len(rawAns))

	// Render ASCII and Markdown cleanly.
	var asciiBuf, mdBuf bytes.Buffer
	require.NoError(t, renderBeforeWriteOutput(&asciiBuf, out, "ascii"))
	require.Contains(t, asciiBuf.String(), "checkout-pdb")
	require.Contains(t, asciiBuf.String(), "SyncWindow: kind=deny")
	require.Contains(t, asciiBuf.String(), "Contention: contested")
	require.NoError(t, renderBeforeWriteOutput(&mdBuf, out, "md"))
	require.Contains(t, mdBuf.String(), "## Before-Write: `Deployment/checkout`")
}

func TestBeforeWrite_RecordingOfflineCLI(t *testing.T) {
	recordingYAML := `# observedAt: 2026-11-24T20:00:00Z
# sourceContext: kind-recorded
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: api
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  suspend: true
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
	tmpDir := t.TempDir()
	recPath := filepath.Join(tmpDir, "snapshot.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(recordingYAML), 0o600))

	// Save & restore package globals.
	origNS, origFP, origRec, origCtx, origFmt := beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat
	t.Cleanup(func() {
		beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat = origNS, origFP, origRec, origCtx, origFmt
		beforeWriteCmd.SetOut(os.Stdout)
	})

	// Reject --recording combined with --kube-context.
	beforeWriteNamespace = "prod"
	beforeWriteFieldPath = ""
	beforeWriteRecording = recPath
	beforeWriteKubeContext = "prod-east"
	beforeWriteFormat = "json"
	require.NoError(t, beforeWriteCmd.Flags().Set("kube-context", "prod-east"))
	t.Cleanup(func() {
		_ = beforeWriteCmd.Flags().Set("kube-context", "")
		beforeWriteCmd.Flags().Lookup("kube-context").Changed = false
	})
	err := runBeforeWrite(beforeWriteCmd, []string{"deploy/api"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--recording cannot be combined with --kube-context")

	// Valid offline --recording run.
	beforeWriteKubeContext = ""
	beforeWriteCmd.Flags().Lookup("kube-context").Changed = false
	beforeWriteFieldPath = ".spec.replicas"
	var outBuf bytes.Buffer
	beforeWriteCmd.SetOut(&outBuf)

	require.NoError(t, runBeforeWrite(beforeWriteCmd, []string{"deploy/api"}))

	var decoded beforeWriteOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &decoded))
	require.Equal(t, "kind-recorded", decoded.Context)
	require.Equal(t, "Deployment", decoded.Target.Kind)
	require.Equal(t, "api", decoded.Target.Name)
	require.NotNil(t, decoded.Deployer)
	require.True(t, *decoded.Deployer.Suspended)
	require.Len(t, decoded.PDBs, 1)
	require.Equal(t, "api-pdb", decoded.PDBs[0].Name)
	require.NotNil(t, decoded.Answer)
	requireConformingAnswer(t, decoded.Answer, explainStates, explainReasons)
	require.Equal(t, "2026-11-24T20:00:00Z", decoded.Answer.ObservedAt)
	require.Equal(t, "offline-recording", decoded.Answer.Cost.Coverage)

	hasStale := false
	for _, u := range decoded.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownStale && strings.Contains(u.Reason, "2026-11-24T20:00:00Z") {
			hasStale = true
		}
	}
	require.True(t, hasStale, "offline recording must surface an explicit stale unknown")

	rawAns, err := json.Marshal(decoded.Answer)
	require.NoError(t, err)
	require.Less(t, len(rawAns), 1024)
}

type forbiddenPDBBeforeWriteStore struct {
	recordedBeforeWriteStore
}

func (s forbiddenPDBBeforeWriteStore) ListPDBs(_ context.Context, _ string) ([]unstructured.Unstructured, error) {
	return nil, apierrors.NewForbidden(schema.GroupResource{Group: "policy", Resource: "poddisruptionbudgets"}, "", os.ErrPermission)
}

func TestBeforeWrite_GracefulDegradation(t *testing.T) {
	// 1. Target not found -> not-observable / object-not-read.
	missing := evaluateBeforeWrite(context.Background(), recordedBeforeWriteStore{}, beforeWriteOptions{
		Kind:       "Deployment",
		Name:       "ghost",
		Namespace:  "prod",
		FieldPath:  ".spec.replicas",
		ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	})
	requireConformingAnswer(t, missing.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateNotObservable, missing.Answer.State)
	require.Equal(t, explainReasonObjectNotRead, missing.Answer.Reason)
	require.NotNil(t, missing.FieldAttribution)
	require.Equal(t, agent.CauseUnknown, missing.FieldAttribution.Cause)

	// 2. Native resource (no owner marker) + forbidden PDB list -> no-owner-marker + denied unknown.
	nativeDep := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "standalone",
			"namespace": "prod",
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "standalone"},
				},
			},
		},
	}}
	nativeOut := evaluateBeforeWrite(context.Background(), forbiddenPDBBeforeWriteStore{
		recordedBeforeWriteStore: recordedBeforeWriteStore{objects: []*unstructured.Unstructured{nativeDep}},
	}, beforeWriteOptions{
		Kind:       "Deployment",
		Name:       "standalone",
		Namespace:  "prod",
		ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	})
	requireConformingAnswer(t, nativeOut.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateNoOwnerMarker, nativeOut.Answer.State)
	hasDenied := false
	for _, u := range nativeOut.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownDenied && u.What == "PodDisruptionBudgets" {
			hasDenied = true
		}
	}
	require.True(t, hasDenied, "forbidden PDB list must be reported as denied unknown")
}

func TestBeforeWrite_MCPToolRegistrationAndBuildArgs(t *testing.T) {
	var capturedArgs []string
	runner := func(_ context.Context, args []string) (string, error) {
		capturedArgs = append([]string(nil), args...)
		return `{"target":{"kind":"Deployment","name":"api","namespace":"prod"},"owner":{"type":"flux","display":"Flux"},"answer":{"state":"explained","reason":"owner-and-chain-read","summary":"Deployment/api owned by Flux.","observedAt":"2026-11-25T10:00:00Z","cluster":{"context":"prod-east","idSource":"not-read"},"basis":[],"unknowns":[],"next":[],"cost":{"requestsMade":2,"responseBodyBytes":1024,"durationMillis":12,"coverage":"before-write-session-client"}}}`, nil
	}

	gateway := newMCPGateway(runner)
	tool, ok := gateway.tools["before_write"]
	require.True(t, ok, "before_write must be registered as a standalone MCP tool")
	require.NotNil(t, tool.Descriptor.Annotations)
	require.True(t, tool.Descriptor.Annotations.ReadOnlyHint)

	reply := gateway.runNamedTool(context.Background(), "before_write", tool, map[string]interface{}{
		"resource":   "deploy/api",
		"namespace":  "prod",
		"field_path": ".spec.replicas",
		"context":    "prod-east",
	})
	require.Equal(t, false, reply["isError"], "reply: %v", reply)
	require.Equal(t, []string{"before-write", "deploy/api", "--kube-context", "prod-east", "-n", "prod", "--field-path", ".spec.replicas", "--format", "json"}, capturedArgs)
	require.NotNil(t, reply["structuredContent"])

	// Reject malformed field_path.
	_, err := tool.BuildArgs(map[string]interface{}{"resource": "deploy/api", "field_path": "spec.replicas"})
	require.Error(t, err)

	// include_blast_radius: true forwards --include-blast-radius.
	replyWithBlast := gateway.runNamedTool(context.Background(), "before_write", tool, map[string]interface{}{
		"resource":             "configmap/app-cfg",
		"namespace":            "prod",
		"include_blast_radius": true,
	})
	require.Equal(t, false, replyWithBlast["isError"], "reply: %v", replyWithBlast)
	require.Equal(t, []string{"before-write", "configmap/app-cfg", "-n", "prod", "--include-blast-radius", "--format", "json"}, capturedArgs)
	require.NotNil(t, replyWithBlast["structuredContent"])

	// Reject non-boolean include_blast_radius.
	_, err = tool.BuildArgs(map[string]interface{}{"resource": "deploy/api", "include_blast_radius": "yes"})
	require.Error(t, err)
}

func TestBeforeWrite_IncludeBlastRadiusRecordingAndCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`# observedAt: 2026-11-26T00:00:00Z
# sourceContext: prod-east
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  suspend: false
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: shared-cfg
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
data:
  MODE: prod
`)
	for i := 1; i <= 7; i++ {
		b.WriteString(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: svc-0` + string(rune('0'+i)) + `
  namespace: prod
spec:
  template:
    spec:
      containers:
        - name: app
          image: ghcr.io/example/svc:v1
          envFrom:
            - configMapRef:
                name: shared-cfg
`)
	}

	tmpDir := t.TempDir()
	recPath := filepath.Join(tmpDir, "blast-cap.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(b.String()), 0o600))

	origNS, origFP, origRec, origCtx, origFmt, origBR := beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat, beforeWriteIncludeBlastRadius
	t.Cleanup(func() {
		beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat, beforeWriteIncludeBlastRadius = origNS, origFP, origRec, origCtx, origFmt, origBR
		beforeWriteCmd.SetOut(os.Stdout)
		if f := beforeWriteCmd.Flags().Lookup("kube-context"); f != nil {
			f.Changed = false
		}
	})

	beforeWriteNamespace = "prod"
	beforeWriteFieldPath = ""
	beforeWriteRecording = recPath
	beforeWriteKubeContext = ""
	beforeWriteFormat = "json"
	beforeWriteCmd.Flags().Lookup("kube-context").Changed = false

	// 1. Default (include-blast-radius=false): blastRadius omitted, zero regression.
	beforeWriteIncludeBlastRadius = false
	var defaultBuf bytes.Buffer
	beforeWriteCmd.SetOut(&defaultBuf)
	require.NoError(t, runBeforeWrite(beforeWriteCmd, []string{"configmap/shared-cfg"}))
	require.NotContains(t, defaultBuf.String(), `"blastRadius"`)

	var defaultOut beforeWriteOutput
	require.NoError(t, json.Unmarshal(defaultBuf.Bytes(), &defaultOut))
	require.Nil(t, defaultOut.BlastRadius)
	requireConformingAnswer(t, defaultOut.Answer, explainStates, explainReasons)

	// 2. Enabled (include-blast-radius=true): 7 workloads -> count=7, capped at 5, truncated=true.
	beforeWriteIncludeBlastRadius = true
	var blastBuf bytes.Buffer
	beforeWriteCmd.SetOut(&blastBuf)
	require.NoError(t, runBeforeWrite(beforeWriteCmd, []string{"configmap/shared-cfg"}))

	var blastOut beforeWriteOutput
	require.NoError(t, json.Unmarshal(blastBuf.Bytes(), &blastOut))
	require.NotNil(t, blastOut.BlastRadius)
	require.Equal(t, 7, blastOut.BlastRadius.AffectedWorkloadsCount)
	require.Equal(t, []string{
		"Deployment/prod/svc-01",
		"Deployment/prod/svc-02",
		"Deployment/prod/svc-03",
		"Deployment/prod/svc-04",
		"Deployment/prod/svc-05",
	}, blastOut.BlastRadius.AffectedWorkloads)
	require.True(t, blastOut.BlastRadius.Truncated)
	requireConformingAnswer(t, blastOut.Answer, explainStates, explainReasons)
	rawAns, err := json.Marshal(blastOut.Answer)
	require.NoError(t, err)
	require.LessOrEqual(t, len(rawAns), 700, "answer block must stay <= 700B, got %d", len(rawAns))

	// Render ASCII and Markdown with blastRadius present.
	var asciiBuf, mdBuf bytes.Buffer
	require.NoError(t, renderBeforeWriteOutput(&asciiBuf, blastOut, "ascii"))
	require.Contains(t, asciiBuf.String(), "Blast Radius: 7 affected workload(s) (truncated)")
	require.NoError(t, renderBeforeWriteOutput(&mdBuf, blastOut, "md"))
	require.Contains(t, mdBuf.String(), "**Blast Radius:** 7 affected workload(s) (truncated)")

	// 3. Uncapped case (<= 5 workloads) sets truncated=false.
	smallPath := filepath.Join(tmpDir, "blast-small.yaml")
	require.NoError(t, os.WriteFile(smallPath, []byte(fluxMultiHopBlastFixture), 0o600))
	beforeWriteRecording = smallPath
	var smallBuf bytes.Buffer
	beforeWriteCmd.SetOut(&smallBuf)
	require.NoError(t, runBeforeWrite(beforeWriteCmd, []string{"configmap/app-cfg"}))

	var smallOut beforeWriteOutput
	require.NoError(t, json.Unmarshal(smallBuf.Bytes(), &smallOut))
	require.NotNil(t, smallOut.BlastRadius)
	require.Equal(t, 1, smallOut.BlastRadius.AffectedWorkloadsCount)
	require.Equal(t, []string{"Deployment/prod/api"}, smallOut.BlastRadius.AffectedWorkloads)
	require.False(t, smallOut.BlastRadius.Truncated)
}

func TestBeforeWrite_RecordingOfflineCLI_ArgoRollout(t *testing.T) {
	recordingYAML := `# observedAt: 2026-11-25T12:00:00Z
# sourceContext: kind-rollouts
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: checkout
  namespace: shop
  labels:
    argocd.argoproj.io/instance: checkout-app
  managedFields:
    - manager: argocd-controller
      operation: Apply
      fieldsType: FieldsV1
      fieldsV1:
        f:spec:
          f:replicas: {}
spec:
  replicas: 4
  selector:
    matchLabels:
      app: checkout
  template:
    metadata:
      labels:
        app: checkout
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: checkout-app
  namespace: argocd
spec:
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: checkout-pdb
  namespace: shop
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: checkout
`
	tmpDir := t.TempDir()
	recPath := filepath.Join(tmpDir, "rollout-snapshot.yaml")
	require.NoError(t, os.WriteFile(recPath, []byte(recordingYAML), 0o600))

	origNS, origFP, origRec, origCtx, origFmt := beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat
	t.Cleanup(func() {
		beforeWriteNamespace, beforeWriteFieldPath, beforeWriteRecording, beforeWriteKubeContext, beforeWriteFormat = origNS, origFP, origRec, origCtx, origFmt
		beforeWriteCmd.SetOut(os.Stdout)
	})

	beforeWriteNamespace = "shop"
	beforeWriteFieldPath = ".spec.replicas"
	beforeWriteRecording = recPath
	beforeWriteKubeContext = ""
	beforeWriteFormat = "json"
	beforeWriteCmd.Flags().Lookup("kube-context").Changed = false

	var outBuf bytes.Buffer
	beforeWriteCmd.SetOut(&outBuf)

	require.NoError(t, runBeforeWrite(beforeWriteCmd, []string{"ro/checkout"}))

	var decoded beforeWriteOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &decoded))
	require.Equal(t, "kind-rollouts", decoded.Context)
	require.Equal(t, "Rollout", decoded.Target.Kind)
	require.Equal(t, "checkout", decoded.Target.Name)
	require.Equal(t, "shop", decoded.Target.Namespace)
	require.Equal(t, agent.OwnerArgo, decoded.Owner.Type)
	require.NotNil(t, decoded.Deployer)
	require.NotNil(t, decoded.Deployer.SelfHeal)
	require.True(t, *decoded.Deployer.SelfHeal)
	require.NotNil(t, decoded.FieldAttribution)
	require.NotNil(t, decoded.FieldAttribution.RevertPrediction)
	require.Equal(t, agent.RevertVerdictLikelyReverted, decoded.FieldAttribution.RevertPrediction.Verdict)
	require.Equal(t, agent.RevertReasonArgoSelfHealEnabled, decoded.FieldAttribution.RevertPrediction.Reason)
	require.Len(t, decoded.PDBs, 1)
	require.Equal(t, "checkout-pdb", decoded.PDBs[0].Name)
	require.NotNil(t, decoded.Answer)
	requireConformingAnswer(t, decoded.Answer, explainStates, explainReasons)
	require.Equal(t, explainStateExplained, decoded.Answer.State)
	require.Equal(t, "offline-recording", decoded.Answer.Cost.Coverage)

	rawAns, err := json.Marshal(decoded.Answer)
	require.NoError(t, err)
	require.Less(t, len(rawAns), 1024)
}
