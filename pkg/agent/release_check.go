// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ReleaseCheckStage struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

type ReleaseCheckReport struct {
	Version            string                      `json:"version"`
	Context            string                      `json:"context"`
	ControllerContext  string                      `json:"controllerContext"`
	Controller         BoundedResourceRef          `json:"controller"`
	StartedAt          time.Time                   `json:"startedAt"`
	FinishedAt         time.Time                   `json:"finishedAt"`
	Bundle             ReleaseBundleEvidence       `json:"bundle"`
	Verdict            ReceiptVerdict              `json:"verdict"`
	Headline           string                      `json:"headline"`
	Stages             []ReleaseCheckStage         `json:"stages"`
	ControllerRevision *ControllerRevisionEvidence `json:"controllerRevision,omitempty"`
	Configuration      *Statement                  `json:"configuration,omitempty"`
	Convergence        *Statement                  `json:"convergence,omitempty"`
	RunningImage       *RunningImageEvidence       `json:"runningImage,omitempty"`
	Reads              []BoundedReadEvidence       `json:"reads"`
	RequestCounts      BoundedReadCounts           `json:"requestCounts"`
	MaxObjects         int                         `json:"maxObjects"`
	Omissions          []Omission                  `json:"omissions"`
	NextStep           string                      `json:"nextStep"`
}

func NewReleaseCheckReport(ref BoundedResourceRef, targetContext, controllerContext string, maxObjects int, now time.Time) ReleaseCheckReport {
	return ReleaseCheckReport{Version: "v1", Context: targetContext, ControllerContext: controllerContext, Controller: ref, StartedAt: now, MaxObjects: maxObjects,
		Reads: []BoundedReadEvidence{}, Stages: []ReleaseCheckStage{
			{Name: "bundle", Verdict: "INCONCLUSIVE", Reason: "Bundle has not been verified."},
			{Name: "controller", Verdict: "INCONCLUSIVE", Reason: "Controller/source/target binding has not been verified."},
			{Name: "configuration", Verdict: "INCONCLUSIVE", Reason: "Live configuration has not been compared."},
			{Name: "workloads", Verdict: "NOT_ASSESSED", Reason: "Workload-controller convergence has not been assessed."},
		}, Omissions: []Omission{
			{Missing: "application-success", Reason: "No traffic, functional or SLO checks are performed.", Severity: "info"},
			{Missing: "running-artifact-identity", Reason: "Configuration bundle digest is not a container image digest. Pods and process-level configuration reload are not inspected.", Severity: "info"},
			{Missing: "atomic-snapshot", Reason: "These are sequential dated reads, not an atomic cluster snapshot. A final controller/source read detects changes during the check.", Severity: "info"},
			{Missing: "release-authority", Reason: "The caller supplies the intended immutable bundle and context binding; publication history is not queried.", Severity: "info"},
		}}
}

func (r *ReleaseCheckReport) AddRead(e BoundedReadEvidence) {
	r.Reads = append(r.Reads, e)
	r.RequestCounts.Discovery += e.Reads.Discovery
	r.RequestCounts.Object += e.Reads.Object
}

func (r *ReleaseCheckReport) Finish(now time.Time) {
	r.FinishedAt = now
	r.Verdict = VerdictPASS
	priority := map[string]int{"PASS": 0, "NOT_ASSESSED": 0, "WATCH": 1, "INCONCLUSIVE": 2, "BLOCK": 3}
	for _, stage := range r.Stages {
		if priority[stage.Verdict] > priority[string(r.Verdict)] {
			r.Verdict = ReceiptVerdict(stage.Verdict)
		}
	}
	switch r.Verdict {
	case VerdictPASS:
		r.Headline = "Expected bundle reported; authored configuration matches."
		if r.Convergence != nil {
			r.Headline += " Workload controllers report convergence."
		}
		r.NextStep = "Inspect external application checks if you need functional or traffic success."
	case VerdictBLOCK:
		r.Headline = "Configuration differs or a workload reports failure."
		r.NextStep = "Inspect configuration differences and workload reasons below; this check performs no repair."
	case VerdictWATCH:
		r.Headline = "Expected release is not yet confirmed converged."
		r.NextStep = "Inspect the controller revision and rollout progress, then refresh the check."
	default:
		r.Headline = "Release verification is incomplete."
		r.NextStep = "Resolve the missing source, target or read evidence shown below, then refresh."
	}
	r.composeRunningImageHeadline()
}

// RunningImageStageVerdict maps the tier's match/mismatch/unknown to the
// report's ReceiptVerdict vocabulary used by the weakest-link Finish loop.
func RunningImageStageVerdict(verdict string) string {
	switch verdict {
	case "match":
		return "PASS"
	case "mismatch":
		return "BLOCK"
	case "unknown":
		return "INCONCLUSIVE"
	default:
		return "NOT_ASSESSED"
	}
}

// composeRunningImageHeadline refines the headline once the opt-in running-image
// tier is present. It never masks an equally or more severe configuration or
// workload problem, and never upgrades a headline.
func (r *ReleaseCheckReport) composeRunningImageHeadline() {
	if r.RunningImage == nil {
		return
	}
	switch r.RunningImage.Verdict {
	case "match":
		if r.Verdict == VerdictPASS {
			r.Headline += " Running pods report the intended image digest."
		}
	case "mismatch":
		if r.nonRunningImageAtLeast("BLOCK") {
			r.Headline += " Running pods also do not run the intended image."
		} else {
			r.Headline = "Running pods do not run the intended image."
		}
		r.NextStep = "Live pods report an image digest other than the intended configuration's; verify the workload image before trusting this release. Note: multi-architecture images can legitimately differ in digest form. This check performs no repair."
	case "unknown":
		if !r.nonRunningImageAtLeast("INCONCLUSIVE") {
			r.Headline = "Configuration checks completed; the running image could not be confirmed."
		} else {
			r.Headline += " The running image could not be confirmed."
		}
		r.NextStep = runningImageNextStep(r.RunningImage.Reason)
	}
}

// nonRunningImageAtLeast reports whether any stage other than running-image is at
// least as severe as the given ReceiptVerdict.
func (r *ReleaseCheckReport) nonRunningImageAtLeast(verdict string) bool {
	priority := map[string]int{"PASS": 0, "NOT_ASSESSED": 0, "WATCH": 1, "INCONCLUSIVE": 2, "BLOCK": 3}
	for _, stage := range r.Stages {
		if stage.Name == "running-image" {
			continue
		}
		if priority[stage.Verdict] >= priority[verdict] && priority[verdict] > 0 {
			return true
		}
	}
	return false
}

func runningImageNextStep(reason string) string {
	switch reason {
	case "mutable-tag":
		return "The intended image is a mutable tag, so the running artifact cannot be tied to it. Pin the workload image to a digest (name@sha256:...) or supply build provenance, then re-run."
	case "read-denied", "bounded pod list unavailable":
		return "Pod reads were unavailable; grant read access to the workload's pods in this context, then re-run with --check-running-image."
	case "selector-unsupported":
		return "The workload selector could not be resolved to pods in this slice (matchLabels required); running-image identity stays unconfirmed."
	default:
		return "Running-image identity could not be confirmed (" + reason + "); resolve the noted gap, then re-run with --check-running-image."
	}
}

// ReleaseControllerBinding joins repository + render scope + destination +
// inventory. A matching digest alone is deliberately insufficient.
func ReleaseControllerBinding(controller, source *unstructured.Unstructured, bundle ReleaseBundle, targetServer string, sameContext bool) string {
	if controller == nil {
		return "Controller object is unavailable."
	}
	ref, err := ParseReleaseBundleReference(bundle.Evidence.Reference)
	if err != nil {
		return err.Error()
	}
	repo := "oci://" + ref.Registry + "/" + ref.Repository
	switch controller.GetAPIVersion() + "/" + controller.GetKind() {
	case "argoproj.io/v1alpha1/Application":
		if revisionString(controller, "spec", "source", "repoURL") != repo {
			return "Application source repository differs from the expected bundle repository."
		}
		p := revisionString(controller, "spec", "source", "path")
		if p != "." && p != "" {
			return "Application selects a subpath; whole-bundle comparison is not supported for this shape."
		}
		for _, field := range []string{"helm", "kustomize", "plugin"} {
			if value, found, _ := unstructured.NestedFieldNoCopy(controller.Object, "spec", "source", field); found && value != nil {
				return "Application rendering options are not supported for a literal-bundle check."
			}
		}
		directory, _, e := unstructured.NestedMap(controller.Object, "spec", "source", "directory")
		if e != nil {
			return "Malformed directory source options."
		}
		for key := range directory {
			if key != "recurse" {
				return "Directory filters or rendering options prevent whole-bundle coverage."
			}
		}
		recurse, _, e := unstructured.NestedBool(controller.Object, "spec", "source", "directory", "recurse")
		if e != nil || (bundle.NestedFiles && !recurse) {
			return "Nested bundle files are not covered by the Application directory settings."
		}
		server := revisionString(controller, "spec", "destination", "server")
		name := revisionString(controller, "spec", "destination", "name")
		local := server == "https://kubernetes.default.svc" && sameContext
		if name != "" || server == "" || (!local && strings.TrimSuffix(server, "/") != strings.TrimSuffix(targetServer, "/")) {
			return "Application destination cannot be bound to the selected target context; named destinations require an explicit adapter."
		}
	case "kustomize.toolkit.fluxcd.io/v1/Kustomization":
		if !sameContext {
			return "Kustomization and target must use the same explicit context for this adapter."
		}
		for _, field := range []string{"kubeConfig", "patches", "images", "components", "postBuild", "commonMetadata", "namePrefix", "nameSuffix", "targetNamespace", "decryption", "buildMetadata"} {
			if value, found, _ := unstructured.NestedFieldNoCopy(controller.Object, "spec", field); found && value != nil {
				return "Kustomization transforms or remote kubeConfig require an adapter; literal whole-bundle binding is unavailable."
			}
		}
		p := revisionString(controller, "spec", "path")
		if p != "" && p != "." && p != "./" {
			return "Kustomization selects a subpath; whole-bundle binding is unavailable."
		}
		if source == nil || source.GetAPIVersion() != "source.toolkit.fluxcd.io/v1" || source.GetKind() != "OCIRepository" || source.GetUID() == "" {
			return "Exact OCIRepository evidence is unavailable."
		}
		sourceNamespace := revisionString(controller, "spec", "sourceRef", "namespace")
		if sourceNamespace == "" {
			sourceNamespace = controller.GetNamespace()
		}
		if revisionString(controller, "spec", "sourceRef", "kind") != source.GetKind() || revisionString(controller, "spec", "sourceRef", "name") != source.GetName() || sourceNamespace != source.GetNamespace() {
			return "Source identity differs from the Kustomization source reference."
		}
		if source.GetDeletionTimestamp() != nil {
			return "OCIRepository is being deleted."
		}
		suspend, _, err := unstructured.NestedBool(source.Object, "spec", "suspend")
		if err != nil || suspend {
			return "OCIRepository is suspended or its suspension field is invalid."
		}
		if revisionString(source, "spec", "url") != repo {
			return "OCIRepository URL differs from the expected bundle repository."
		}
		if _, found, _ := unstructured.NestedFieldNoCopy(source.Object, "spec", "layerSelector"); found {
			return "OCI layer selection requires an adapter; whole-bundle binding is unavailable."
		}
		if _, found, _ := unstructured.NestedFieldNoCopy(source.Object, "spec", "ignore"); found {
			return "OCI source ignore rules prevent a whole-bundle binding."
		}
		if source.GetGeneration() <= 0 {
			return "OCIRepository generation is unavailable."
		}
		gen, _, _ := unstructured.NestedInt64(source.Object, "status", "observedGeneration")
		if gen != source.GetGeneration() {
			return "OCIRepository status is not for its current generation."
		}
		conditions, found, err := unstructured.NestedSlice(source.Object, "status", "conditions")
		if err != nil || !found {
			return "OCIRepository conditions are unavailable."
		}
		ready := 0
		for _, item := range conditions {
			condition, ok := item.(map[string]interface{})
			if !ok {
				return "Malformed OCIRepository conditions."
			}
			if condition["type"] == "Ready" {
				g, _ := condition["observedGeneration"].(int64)
				if condition["status"] != "True" || g != gen {
					return "OCIRepository does not report current-generation readiness."
				}
				ready++
			}
			if (condition["type"] == "Reconciling" || condition["type"] == "Stalled") && condition["status"] != "False" {
				return "OCIRepository is reconciling, stalled or uncertain."
			}
		}
		if ready != 1 {
			return "OCIRepository needs one current-generation Ready condition."
		}
		artifact := revisionString(source, "status", "artifact", "revision")
		if _, suffix, ok := strings.Cut(artifact, "@"); ok {
			artifact = suffix
		}
		if artifact != bundle.Evidence.Digest {
			return "OCIRepository does not report the expected bundle artifact revision."
		}
	default:
		return "Controller API/Kind is not supported by the literal-bundle adapter."
	}
	return ""
}

func ReleaseInventoryCoverage(controller *unstructured.Unstructured, objects []*unstructured.Unstructured) string {
	if controller == nil {
		return "Controller inventory is unavailable."
	}
	ids := map[string]bool{}
	if controller.GetKind() == "Application" {
		items, found, err := unstructured.NestedSlice(controller.Object, "status", "resources")
		if err != nil || !found {
			return "Application resource inventory is unavailable."
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				return "Malformed Application resource inventory."
			}
			get := func(k string) string { s, _ := m[k].(string); return s }
			ids[get("namespace")+"_"+get("name")+"_"+get("group")+"_"+get("kind")] = true
		}
	} else {
		items, found, err := unstructured.NestedSlice(controller.Object, "status", "inventory", "entries")
		if err != nil || !found {
			return "Kustomization resource inventory is unavailable."
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				return "Malformed Kustomization resource inventory."
			}
			id, _ := m["id"].(string)
			version, _ := m["v"].(string)
			ids[id+"_"+version] = true
		}
	}
	for _, obj := range objects {
		gv, _ := schema.ParseGroupVersion(obj.GetAPIVersion())
		key := obj.GetNamespace() + "_" + obj.GetName() + "_" + gv.Group + "_" + obj.GetKind()
		if controller.GetKind() != "Application" {
			key += "_" + gv.Version
		}
		if !ids[key] {
			return fmt.Sprintf("Controller inventory does not cover %s/%s in namespace %q.", obj.GetKind(), obj.GetName(), obj.GetNamespace())
		}
	}
	return ""
}

func ReleaseWorkloadSupported(obj *unstructured.Unstructured) bool {
	switch obj.GetAPIVersion() + "/" + obj.GetKind() {
	case "apps/v1/Deployment", "apps/v1/StatefulSet", "apps/v1/DaemonSet", "batch/v1/Job", "v1/Pod":
		return true
	}
	return false
}
