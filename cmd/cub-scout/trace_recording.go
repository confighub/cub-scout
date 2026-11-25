// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

// outputOfflineTraceJSON renders the v0.14 trace JSON output for an offline
// recorded snapshot, reusing buildTraceAnswer and appending an explicit
// kind:"stale" unknown noting the snapshot's observedAt.
func outputOfflineTraceJSON(
	result *agent.TraceResult,
	snapshot recordedObjectSnapshot,
	kind, name, namespace string,
	artifacts map[string]mapsvc.TraceArtifactRef,
) error {
	output := convertTraceToV014(result, kind, name, namespace, artifacts)
	if output.Answer != nil {
		output.Answer.Unknowns = append(output.Answer.Unknowns, offlineRecordingUnknown(snapshot))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// observeTraceFromSnapshot deterministically reconstructs a resource's delivery
// chain offline from a recorded multi-object YAML/JSON snapshot without
// contacting any Kubernetes cluster or external network.
func observeTraceFromSnapshot(
	snapshot recordedObjectSnapshot,
	kind, name, namespace string,
) (*agent.TraceResult, map[string]mapsvc.TraceArtifactRef, error) {
	targetObj, err := findObjectInRecordedSnapshot(snapshot.Objects, kind, name, namespace)
	if err != nil {
		return nil, nil, err
	}

	observedAt := offlineRecordingObservedAt(snapshot)
	readCost := &agent.KubernetesReadCost{Coverage: "offline-recording"}
	artifacts := make(map[string]mapsvc.TraceArtifactRef)

	// Direct Argo CD Application trace (e.g. --app or `trace Application/<name>`).
	if strings.EqualFold(targetObj.GetKind(), "Application") {
		result := buildOfflineArgoTraceResult(snapshot, targetObj, targetObj, artifacts)
		result.Context = snapshot.SourceContext
		result.DetectedOwner = agent.OwnerArgo
		result.TracedAt = observedAt
		result.ReadCost = readCost
		return result, mergeTraceArtifacts(buildUnknownTraceArtifacts(result), artifacts), nil
	}

	ownership := agent.DetectOwnership(targetObj)
	ownership = withHelmReleaseAnnotations(ownership, targetObj.GetAnnotations())

	var result *agent.TraceResult
	switch ownership.Type {
	case agent.OwnerFlux:
		result = buildOfflineFluxTraceResult(snapshot, targetObj, artifacts)
	case agent.OwnerArgo:
		appObj, lookupErr := findArgoApplicationInSnapshot(snapshot.Objects, targetObj, ownership)
		if appObj != nil {
			result = buildOfflineArgoTraceResult(snapshot, targetObj, appObj, artifacts)
		} else {
			result = &agent.TraceResult{
				Object: resourceRefFromObject(targetObj),
				Tool:   "argocd",
				Chain:  []agent.ChainLink{chainLinkFromObject(targetObj)},
				Error:  lookupErr,
			}
		}
	case agent.OwnerHelm:
		result = buildOfflineHelmTraceResult(snapshot, targetObj, ownership)
	case agent.OwnerKro:
		result = buildOfflineKroTraceResult(snapshot, targetObj)
	case agent.OwnerCrossplane:
		result = buildOfflineCrossplaneTraceResult(snapshot, targetObj)
	default:
		result = noOwnerMarkerTraceResult(targetObj.GetKind(), targetObj.GetName(), targetObj.GetNamespace())
	}

	result.Context = snapshot.SourceContext
	result.DetectedOwner = ownership.Type
	result.TracedAt = observedAt
	result.ReadCost = readCost

	return result, mergeTraceArtifacts(buildUnknownTraceArtifacts(result), artifacts), nil
}

func findObjectInRecordedSnapshot(objects []*unstructured.Unstructured, kind, name, namespace string) (*unstructured.Unstructured, error) {
	var matches []*unstructured.Unstructured
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		if !strings.EqualFold(obj.GetKind(), kind) || obj.GetName() != name {
			continue
		}
		if namespace != "" && obj.GetNamespace() != namespace {
			continue
		}
		matches = append(matches, obj)
	}
	switch len(matches) {
	case 0:
		if namespace != "" {
			return nil, fmt.Errorf("resource %s/%s in namespace %q not found in recorded snapshot", kind, name, namespace)
		}
		return nil, fmt.Errorf("resource %s/%s not found in recorded snapshot", kind, name)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous resource %s/%s in recorded snapshot (%d matches); specify --namespace", kind, name, len(matches))
	}
}

func findSnapshotObjectByKind(objects []*unstructured.Unstructured, kind, namespace, name string) *unstructured.Unstructured {
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		if !strings.EqualFold(obj.GetKind(), kind) || obj.GetName() != name {
			continue
		}
		if namespace != "" && obj.GetNamespace() != namespace {
			continue
		}
		return obj
	}
	return nil
}

func findSnapshotObjectByGVK(objects []*unstructured.Unstructured, gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		if obj.GetName() != name {
			continue
		}
		if namespace != "" && obj.GetNamespace() != namespace {
			continue
		}
		objGVK := obj.GroupVersionKind()
		if !strings.EqualFold(objGVK.Kind, gvk.Kind) {
			continue
		}
		if gvk.Group != "" && objGVK.Group != gvk.Group {
			continue
		}
		if gvk.Version != "" && objGVK.Version != gvk.Version {
			continue
		}
		return obj
	}
	return nil
}

func fluxReadyConditionLink(obj *unstructured.Unstructured) agent.ChainLink {
	link := agent.ChainLink{
		Kind:      obj.GetKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Ready:     false,
		Status:    "Unknown",
	}
	if transition := observedLastTransitionTime(obj); transition != nil {
		link.LastTransitionTime = transition
	}
	conds, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conds {
		m, ok := raw.(map[string]interface{})
		if !ok || !strings.EqualFold(stringValue(m["type"]), "Ready") {
			continue
		}
		st := stringValue(m["status"])
		link.StatusReason = stringValue(m["reason"])
		link.Message = stringValue(m["message"])
		link.Ready = strings.EqualFold(st, "True")
		link.Status = conditionDisplayStatus("Ready", st)
		return link
	}
	return link
}

func buildOfflineFluxTraceResult(
	snapshot recordedObjectSnapshot,
	targetObj *unstructured.Unstructured,
	artifacts map[string]mapsvc.TraceArtifactRef,
) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: resourceRefFromObject(targetObj),
		Tool:   "flux",
	}
	labels := targetObj.GetLabels()

	var deployerKind, deployerName, deployerNS string
	if ksName := strings.TrimSpace(labels["kustomize.toolkit.fluxcd.io/name"]); ksName != "" {
		deployerKind = "Kustomization"
		deployerName = ksName
		deployerNS = strings.TrimSpace(labels["kustomize.toolkit.fluxcd.io/namespace"])
	} else if hrName := strings.TrimSpace(labels["helm.toolkit.fluxcd.io/name"]); hrName != "" {
		deployerKind = "HelmRelease"
		deployerName = hrName
		deployerNS = strings.TrimSpace(labels["helm.toolkit.fluxcd.io/namespace"])
	}
	if deployerNS == "" {
		deployerNS = "flux-system"
	}

	if deployerName != "" {
		deployerObj := findSnapshotObjectByKind(snapshot.Objects, deployerKind, deployerNS, deployerName)
		if deployerObj == nil {
			result.Error = appendSentence(result.Error, fmt.Sprintf("Flux %s/%s in %s not present in recorded snapshot", deployerKind, deployerName, deployerNS))
		} else {
			deployerLink := fluxReadyConditionLink(deployerObj)
			if rev, _, _ := unstructured.NestedString(deployerObj.Object, "status", "lastAppliedRevision"); rev != "" {
				deployerLink.Revision = rev
			} else if rev, _, _ := unstructured.NestedString(deployerObj.Object, "status", "lastAttemptedRevision"); rev != "" {
				deployerLink.Revision = rev
			}
			if p, _, _ := unstructured.NestedString(deployerObj.Object, "spec", "path"); p != "" {
				deployerLink.Path = p
			}

			// Resolve sourceRef (GitRepository / OCIRepository / HelmRepository / Bucket).
			srcKind, srcName, srcNS := extractFluxSourceRef(deployerObj, deployerKind, deployerNS)
			if srcKind != "" && srcName != "" {
				srcObj := findSnapshotObjectByKind(snapshot.Objects, srcKind, srcNS, srcName)
				if srcObj == nil {
					result.Error = appendSentence(result.Error, fmt.Sprintf("Flux source %s/%s in %s not present in recorded snapshot", srcKind, srcName, srcNS))
				} else {
					srcLink := fluxReadyConditionLink(srcObj)
					if u, _, _ := unstructured.NestedString(srcObj.Object, "spec", "url"); u != "" {
						srcLink.URL = u
					}
					if rev, _, _ := unstructured.NestedString(srcObj.Object, "status", "artifact", "revision"); rev != "" {
						srcLink.Revision = rev
					}
					digest, _, _ := unstructured.NestedString(srcObj.Object, "status", "artifact", "digest")
					lastUpdate, _, _ := unstructured.NestedString(srcObj.Object, "status", "artifact", "lastUpdateTime")
					if isTraceSourceKind(srcLink.Kind) {
						artifacts[traceArtifactKey(srcLink.Kind, srcLink.Namespace, srcLink.Name)] = normalizeTraceArtifact(srcLink.Kind, mapsvc.TraceArtifactRef{
							URL:            srcLink.URL,
							Revision:       srcLink.Revision,
							Digest:         digest,
							LastUpdateTime: lastUpdate,
							SourceKind:     srcLink.Kind,
						})
					}
					result.Chain = append(result.Chain, srcLink)
				}
			}

			result.Chain = append(result.Chain, deployerLink)
			result.FullyManaged = deployerLink.Ready
		}
	}

	result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
	return result
}

func extractFluxSourceRef(deployer *unstructured.Unstructured, deployerKind, deployerNS string) (kind, name, namespace string) {
	if deployerKind == "Kustomization" {
		kind, _, _ = unstructured.NestedString(deployer.Object, "spec", "sourceRef", "kind")
		name, _, _ = unstructured.NestedString(deployer.Object, "spec", "sourceRef", "name")
		namespace, _, _ = unstructured.NestedString(deployer.Object, "spec", "sourceRef", "namespace")
	} else {
		// HelmRelease: check spec.chartRef first (Flux v2 OCIRepository/HelmChart), then spec.chart.spec.sourceRef.
		kind, _, _ = unstructured.NestedString(deployer.Object, "spec", "chartRef", "kind")
		name, _, _ = unstructured.NestedString(deployer.Object, "spec", "chartRef", "name")
		namespace, _, _ = unstructured.NestedString(deployer.Object, "spec", "chartRef", "namespace")
		if name == "" {
			kind, _, _ = unstructured.NestedString(deployer.Object, "spec", "chart", "spec", "sourceRef", "kind")
			name, _, _ = unstructured.NestedString(deployer.Object, "spec", "chart", "spec", "sourceRef", "name")
			namespace, _, _ = unstructured.NestedString(deployer.Object, "spec", "chart", "spec", "sourceRef", "namespace")
		}
	}
	if strings.TrimSpace(namespace) == "" {
		namespace = deployerNS
	}
	return strings.TrimSpace(kind), strings.TrimSpace(name), strings.TrimSpace(namespace)
}

func findArgoApplicationInSnapshot(objects []*unstructured.Unstructured, targetObj *unstructured.Unstructured, ownership agent.Ownership) (*unstructured.Unstructured, string) {
	appName := strings.TrimSpace(ownership.Name)
	appNS := strings.TrimSpace(ownership.Namespace)
	if appName == "" {
		if labelApp := strings.TrimSpace(targetObj.GetLabels()["argocd.argoproj.io/instance"]); labelApp != "" {
			appName = labelApp
		} else if tid := strings.TrimSpace(targetObj.GetAnnotations()["argocd.argoproj.io/tracking-id"]); tid != "" {
			appName = parseArgoTrackingIDApplication(tid)
		}
	}
	if appName == "" {
		return nil, "Argo CD ownership metadata does not identify an Application"
	}

	var matches []*unstructured.Unstructured
	for _, obj := range objects {
		if obj == nil || !strings.EqualFold(obj.GetKind(), "Application") {
			continue
		}
		if obj.GetName() != appName {
			continue
		}
		if appNS != "" && obj.GetNamespace() != appNS {
			continue
		}
		matches = append(matches, obj)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf("Argo CD Application %q not present in recorded snapshot", appName)
	case 1:
		return matches[0], ""
	default:
		return nil, fmt.Sprintf("multiple Argo CD Applications named %q found in recorded snapshot", appName)
	}
}

func buildOfflineArgoTraceResult(
	snapshot recordedObjectSnapshot,
	targetObj *unstructured.Unstructured,
	appObj *unstructured.Unstructured,
	artifacts map[string]mapsvc.TraceArtifactRef,
) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: resourceRefFromObject(targetObj),
		Tool:   "argocd",
	}

	repoURL, path, targetRev, chart := extractArgoSourceFromUnstructured(appObj)
	syncStatus, _, _ := unstructured.NestedString(appObj.Object, "status", "sync", "status")
	healthStatus, _, _ := unstructured.NestedString(appObj.Object, "status", "health", "status")
	syncRev, _, _ := unstructured.NestedString(appObj.Object, "status", "sync", "revision")
	opPhase, _, _ := unstructured.NestedString(appObj.Object, "status", "operationState", "phase")
	opMsg, _, _ := unstructured.NestedString(appObj.Object, "status", "operationState", "message")
	healthMsg, _, _ := unstructured.NestedString(appObj.Object, "status", "health", "message")

	if repoURL != "" {
		srcKind := "GitRepository"
		if strings.HasPrefix(strings.ToLower(repoURL), "oci://") {
			srcKind = "OCIRepository"
		} else if chart != "" {
			srcKind = "HelmRepository"
		}
		rev := firstNonEmpty(syncRev, targetRev)
		srcLink := agent.ChainLink{
			Kind:      srcKind,
			Name:      repoURL,
			Namespace: appObj.GetNamespace(),
			Ready:     true,
			Status:    "Observed",
			URL:       repoURL,
			Path:      path,
			Revision:  rev,
		}
		artifacts[traceArtifactKey(srcLink.Kind, srcLink.Namespace, srcLink.Name)] = normalizeTraceArtifact(srcLink.Kind, mapsvc.TraceArtifactRef{
			URL:        repoURL,
			Revision:   rev,
			SourceKind: srcKind,
		})
		result.Chain = append(result.Chain, srcLink)
	}

	statusStr := "Unknown"
	switch {
	case syncStatus != "" && healthStatus != "":
		statusStr = syncStatus + "/" + healthStatus
	case syncStatus != "":
		statusStr = syncStatus
	case healthStatus != "":
		statusStr = healthStatus
	}
	appReady := strings.EqualFold(syncStatus, "Synced") &&
		(healthStatus == "" || strings.EqualFold(healthStatus, "Healthy")) &&
		!strings.EqualFold(opPhase, "Failed") &&
		!strings.EqualFold(opPhase, "Error")

	appLink := agent.ChainLink{
		Kind:      "Application",
		Name:      appObj.GetName(),
		Namespace: appObj.GetNamespace(),
		Ready:     appReady,
		Status:    statusStr,
		Revision:  syncRev,
		Path:      path,
		Message:   firstNonEmpty(healthMsg, opMsg),
	}
	result.Chain = append(result.Chain, appLink)
	result.FullyManaged = appReady

	if !strings.EqualFold(targetObj.GetKind(), "Application") {
		result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
	}

	return result
}

func extractArgoSourceFromUnstructured(app *unstructured.Unstructured) (repoURL, path, targetRevision, chart string) {
	repoURL, _, _ = unstructured.NestedString(app.Object, "spec", "source", "repoURL")
	path, _, _ = unstructured.NestedString(app.Object, "spec", "source", "path")
	targetRevision, _, _ = unstructured.NestedString(app.Object, "spec", "source", "targetRevision")
	chart, _, _ = unstructured.NestedString(app.Object, "spec", "source", "chart")
	if repoURL != "" {
		return repoURL, path, targetRevision, chart
	}
	sources, _, _ := unstructured.NestedSlice(app.Object, "spec", "sources")
	if len(sources) > 0 {
		if first, ok := sources[0].(map[string]interface{}); ok {
			repoURL = stringValue(first["repoURL"])
			path = stringValue(first["path"])
			targetRevision = stringValue(first["targetRevision"])
			chart = stringValue(first["chart"])
		}
	}
	return repoURL, path, targetRevision, chart
}

func buildOfflineHelmTraceResult(
	snapshot recordedObjectSnapshot,
	targetObj *unstructured.Unstructured,
	ownership agent.Ownership,
) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: resourceRefFromObject(targetObj),
		Tool:   "helm",
	}

	releaseName := strings.TrimSpace(ownership.Name)
	releaseNS := strings.TrimSpace(ownership.Namespace)
	if releaseNS == "" {
		releaseNS = targetObj.GetNamespace()
	}

	var latest *unstructured.Unstructured
	latestVer := -1
	if releaseName != "" {
		for _, obj := range snapshot.Objects {
			if obj == nil || !strings.EqualFold(obj.GetKind(), "Secret") {
				continue
			}
			if releaseNS != "" && obj.GetNamespace() != releaseNS {
				continue
			}
			secLabels := obj.GetLabels()
			if secLabels["owner"] != "helm" || secLabels["name"] != releaseName {
				continue
			}
			ver, err := strconv.Atoi(strings.TrimSpace(secLabels["version"]))
			if err != nil {
				continue
			}
			if ver > latestVer {
				latestVer = ver
				latest = obj
			}
		}
	}

	if latest == nil {
		result.Error = appendSentence(result.Error, fmt.Sprintf("Helm release Secret for %q in namespace %q not present in recorded snapshot", releaseName, releaseNS))
		result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
		return result
	}

	if chartLabel := strings.TrimSpace(targetObj.GetLabels()["helm.sh/chart"]); chartLabel != "" {
		result.Chain = append(result.Chain, agent.ChainLink{
			Kind:      "HelmChart",
			Name:      chartLabel,
			Namespace: latest.GetNamespace(),
			Ready:     true,
			Status:    "Observed",
		})
	}

	secLabels := latest.GetLabels()
	relStatus := strings.TrimSpace(secLabels["status"])
	if relStatus == "" {
		relStatus = "Unknown"
	}
	relReady := strings.EqualFold(relStatus, "deployed")
	result.Chain = append(result.Chain, agent.ChainLink{
		Kind:      "Release",
		Name:      releaseName,
		Namespace: latest.GetNamespace(),
		Ready:     relReady,
		Status:    relStatus,
		Revision:  strings.TrimSpace(secLabels["version"]),
	})
	result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
	result.FullyManaged = relReady
	return result
}

func buildOfflineKroTraceResult(snapshot recordedObjectSnapshot, targetObj *unstructured.Unstructured) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: resourceRefFromObject(targetObj),
		Tool:   "kro",
	}
	labels := targetObj.GetLabels()

	instName := strings.TrimSpace(labels["kro.run/instance-name"])
	instNS := strings.TrimSpace(labels["kro.run/instance-namespace"])
	instKind := strings.TrimSpace(labels["kro.run/instance-kind"])
	instGroup := strings.TrimSpace(labels["kro.run/instance-group"])
	instVersion := strings.TrimSpace(labels["kro.run/instance-version"])

	if instName == "" || instKind == "" || instGroup == "" || instVersion == "" {
		for _, or := range targetObj.GetOwnerReferences() {
			gv, err := schema.ParseGroupVersion(or.APIVersion)
			if err != nil || !strings.Contains(gv.Group, "kro.run") || strings.EqualFold(or.Kind, "ResourceGraphDefinition") {
				continue
			}
			if instName == "" {
				instName = or.Name
			}
			if instKind == "" {
				instKind = or.Kind
			}
			if instGroup == "" {
				instGroup = gv.Group
			}
			if instVersion == "" {
				instVersion = gv.Version
			}
			break
		}
	}
	if instNS == "" {
		instNS = targetObj.GetNamespace()
	}

	if rgdName := strings.TrimSpace(labels["kro.run/resource-graph-definition-name"]); rgdName != "" {
		if rgd := findSnapshotObjectByKind(snapshot.Objects, "ResourceGraphDefinition", "", rgdName); rgd != nil {
			result.Chain = append(result.Chain, chainLinkFromObject(rgd))
		}
	}

	if instName != "" && instKind != "" {
		gvk := schema.GroupVersionKind{Group: instGroup, Version: instVersion, Kind: instKind}
		inst := findSnapshotObjectByGVK(snapshot.Objects, gvk, instNS, instName)
		if inst == nil && instNS != "" {
			inst = findSnapshotObjectByGVK(snapshot.Objects, gvk, "", instName)
		}
		if inst != nil {
			instLink := chainLinkFromObject(inst)
			if rgdName := strings.TrimSpace(inst.GetLabels()["kro.run/resource-graph-definition-name"]); rgdName != "" && len(result.Chain) == 0 {
				if rgd := findSnapshotObjectByKind(snapshot.Objects, "ResourceGraphDefinition", "", rgdName); rgd != nil {
					result.Chain = append(result.Chain, chainLinkFromObject(rgd))
				}
			}
			result.Chain = append(result.Chain, instLink)
			result.FullyManaged = instLink.Ready
		} else {
			result.Error = appendSentence(result.Error, fmt.Sprintf("kro instance %s/%s in %s not present in recorded snapshot", instKind, instName, instNS))
		}
	}

	result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
	return result
}

func buildOfflineCrossplaneTraceResult(snapshot recordedObjectSnapshot, targetObj *unstructured.Unstructured) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: resourceRefFromObject(targetObj),
		Tool:   "crossplane",
	}
	labels := targetObj.GetLabels()

	// Optional Claim link if present in snapshot.
	claimName := strings.TrimSpace(labels["crossplane.io/claim-name"])
	claimNS := strings.TrimSpace(labels["crossplane.io/claim-namespace"])
	if claimName != "" {
		for _, obj := range snapshot.Objects {
			if obj == nil || obj.GetName() != claimName {
				continue
			}
			if claimNS != "" && obj.GetNamespace() != claimNS {
				continue
			}
			if obj.GetUID() == targetObj.GetUID() && obj.GetKind() == targetObj.GetKind() {
				continue
			}
			if strings.Contains(obj.GroupVersionKind().Group, "crossplane.io") || strings.Contains(obj.GroupVersionKind().Group, "upbound.io") || obj.GetNamespace() == claimNS {
				if !strings.EqualFold(obj.GetKind(), targetObj.GetKind()) {
					result.Chain = append(result.Chain, chainLinkFromObject(obj))
					break
				}
			}
		}
	}

	compName := strings.TrimSpace(labels["crossplane.io/composite"])
	var gvk schema.GroupVersionKind
	var xrName string
	for _, or := range targetObj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(or.APIVersion)
		if err != nil || gv.Group == "" || gv.Version == "" || or.Kind == "" || or.Name == "" {
			continue
		}
		if compName != "" && or.Name == compName {
			gvk = schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: or.Kind}
			xrName = or.Name
			break
		}
		if strings.Contains(gv.Group, "crossplane.io") || strings.Contains(gv.Group, "upbound.io") {
			gvk = schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: or.Kind}
			xrName = or.Name
		}
	}

	if xrName != "" && gvk.Kind != "" {
		xr := findSnapshotObjectByGVK(snapshot.Objects, gvk, targetObj.GetNamespace(), xrName)
		if xr == nil {
			xr = findSnapshotObjectByGVK(snapshot.Objects, gvk, "", xrName)
		}
		if xr != nil {
			xrLink := chainLinkFromObject(xr)
			if hasReadyCondition(xr) {
				xrLink.ReadyReporter = traceReadyReporterCrossplane
			}
			result.Chain = append(result.Chain, xrLink)
			result.FullyManaged = xrLink.Ready
		} else {
			result.Error = appendSentence(result.Error, fmt.Sprintf("Crossplane composite %s/%s not present in recorded snapshot", gvk.Kind, xrName))
		}
	}

	result.Chain = append(result.Chain, chainLinkFromObject(targetObj))
	return result
}

func hasReadyCondition(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	conds, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conds {
		if m, ok := raw.(map[string]interface{}); ok && strings.EqualFold(stringValue(m["type"]), "Ready") {
			return true
		}
	}
	return false
}
