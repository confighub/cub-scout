// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ControllerRevisionEvidence compares a user-supplied immutable identifier
// with one controller's report. Comparison is not a delivery/health verdict.
type ControllerRevisionEvidence struct {
	ExpectedRevision string `json:"expectedRevision"`
	ReportedRevision string `json:"reportedRevision,omitempty"`
	Revision         string `json:"revision,omitempty"`
	Source           string `json:"source,omitempty"`
	Comparison       string `json:"comparison"`
	ControllerStatus string `json:"controllerStatus,omitempty"`
	ReportedAt       string `json:"reportedAt,omitempty"`
	Reason           string `json:"reason"`
}

var (
	controllerGitCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
	controllerOCIDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

func ValidateExpectedRevision(value string) error {
	if !controllerGitCommit.MatchString(value) && !controllerOCIDigest.MatchString(value) {
		return fmt.Errorf("expected revision must be a full lowercase 40-hex Git commit or sha256: followed by 64 lowercase hex digits; branches, tags and short hashes are not supported")
	}
	return nil
}

func (e ControllerRevisionEvidence) Summary() string {
	reportedAt := e.ReportedAt
	if reportedAt == "" {
		reportedAt = "not-reported"
	}
	return fmt.Sprintf("%s: expected=%s reported=%s status=%s reported-at=%s; %s", e.Comparison, e.ExpectedRevision, e.ReportedRevision, e.ControllerStatus, reportedAt, e.Reason)
}

func BuildControllerRevisionEvidence(obj *unstructured.Unstructured, expected string, observedAt time.Time) ControllerRevisionEvidence {
	e := ControllerRevisionEvidence{ExpectedRevision: expected, Comparison: "unknown", Reason: "Controller evidence unavailable."}
	if err := ValidateExpectedRevision(expected); err != nil {
		e.Reason = err.Error()
		return e
	}
	if obj == nil {
		return e
	}
	if deleted, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "deletionTimestamp"); found && deleted != nil {
		e.Reason = "Selected controller is being deleted; no revision comparison is available."
		return e
	}
	if obj.GetUID() == "" || obj.GetGeneration() <= 0 || observedAt.IsZero() {
		e.Reason = "Controller UID, generation or observation time is missing."
		return e
	}
	var reason string
	switch {
	case obj.GetAPIVersion() == "argoproj.io/v1alpha1" && obj.GetKind() == "Application":
		reason = e.readApplication(obj, observedAt)
	case obj.GetAPIVersion() == "kustomize.toolkit.fluxcd.io/v1" && obj.GetKind() == "Kustomization":
		reason = e.readKustomization(obj)
	default:
		reason = "Revision comparison is not supported for this API version and Kind."
	}
	if reason != "" {
		e.Reason = reason
		return e
	}
	if controllerGitCommit.MatchString(expected) != controllerGitCommit.MatchString(e.Revision) {
		e.Reason = "Expected and reported identifiers describe different revision types."
		return e
	}
	e.Comparison = "mismatch"
	if expected == e.Revision {
		e.Comparison = "match"
	}
	e.Reason = "Immutable identifier comparison with the selected controller report only; not workload convergence, current controller liveness or application success."
	return e
}

func revisionString(obj *unstructured.Unstructured, path ...string) string {
	value, _, _ := unstructured.NestedString(obj.Object, path...)
	// Never echo unbounded or control-bearing controller text into a terminal.
	if len(value) > 512 {
		return ""
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return ""
		}
	}
	return value
}

func (e *ControllerRevisionEvidence) readApplication(obj *unstructured.Unstructured, observedAt time.Time) string {
	e.Source = "status.sync.revision"
	e.ReportedRevision = revisionString(obj, "status", "sync", "revision")
	e.ControllerStatus = revisionString(obj, "status", "sync", "status")
	e.ReportedAt = revisionString(obj, "status", "reconciledAt")
	for _, path := range [][]string{{"spec", "sources"}, {"status", "sync", "revisions"}, {"status", "sync", "comparedTo", "sources"}} {
		values, _, err := unstructured.NestedSlice(obj.Object, path...)
		if err != nil || len(values) > 0 {
			return "Multi-source or malformed source evidence is not supported; no source index is inferred."
		}
	}
	if hydrator, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "sourceHydrator"); found && hydrator != nil {
		return "Hydrated source evidence is not supported."
	}
	for _, field := range []string{"source", "destination"} {
		spec, found, err := unstructured.NestedMap(obj.Object, "spec", field)
		compared, comparedFound, comparedErr := unstructured.NestedMap(obj.Object, "status", "sync", "comparedTo", field)
		if !found || !comparedFound || err != nil || comparedErr != nil || len(spec) == 0 || !reflect.DeepEqual(spec, compared) {
			return "Reported comparison does not identify the current source and destination."
		}
	}
	ignored, _, err := unstructured.NestedSlice(obj.Object, "spec", "ignoreDifferences")
	comparedIgnored, _, comparedErr := unstructured.NestedSlice(obj.Object, "status", "sync", "comparedTo", "ignoreDifferences")
	if err != nil || comparedErr != nil || (len(ignored)+len(comparedIgnored) > 0 && !reflect.DeepEqual(ignored, comparedIgnored)) {
		return "Reported comparison does not identify the current ignore-differences policy."
	}
	if e.ControllerStatus != "Synced" {
		return "Controller does not report Synced; its comparison revision is not treated as an applied revision."
	}
	if operation, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "operation"); found && operation != nil {
		return "An operation is pending; re-read after it completes."
	}
	phase := revisionString(obj, "status", "operationState", "phase")
	if _, _, err := unstructured.NestedString(obj.Object, "status", "operationState", "phase"); err != nil {
		return "Malformed operation status."
	}
	if phase != "" && phase != "Succeeded" {
		return "Operation status is not Succeeded; no settled revision comparison is available."
	}
	conditions, _, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || len(conditions) > 0 {
		return "Application conditions require inspection before trusting its revision report."
	}
	reconciled, err := time.Parse(time.RFC3339Nano, e.ReportedAt)
	if err != nil || reconciled.IsZero() || reconciled.After(observedAt) || reconciled.Before(observedAt.Add(-15*time.Minute)) {
		return "Reconciliation time is missing, invalid, future or older than 15 minutes."
	}
	repo := revisionString(obj, "spec", "source", "repoURL")
	chart, _, chartErr := unstructured.NestedString(obj.Object, "spec", "source", "chart")
	if repo == "" || chartErr != nil || chart != "" {
		return "A supported Git or OCI source is required; chart versions are not immutable revision proof."
	}
	if strings.HasPrefix(repo, "oci://") {
		if !controllerOCIDigest.MatchString(e.ReportedRevision) {
			return "Reported OCI revision is not an exact SHA-256 digest."
		}
	} else if !controllerGitCommit.MatchString(e.ReportedRevision) {
		return "Reported Git revision is not a full commit."
	}
	e.Revision = e.ReportedRevision
	return ""
}

func (e *ControllerRevisionEvidence) readKustomization(obj *unstructured.Unstructured) string {
	e.Source = "status.lastAppliedRevision"
	e.ReportedRevision = revisionString(obj, "status", "lastAppliedRevision")
	generation, found, err := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
	if err != nil || !found || generation != obj.GetGeneration() {
		return "Controller has not reported the current generation."
	}
	suspended, _, err := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if err != nil || suspended {
		return "Controller is suspended or suspension status is malformed."
	}
	conditions, _, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return "Malformed controller conditions."
	}
	ready := 0
	seen := map[string]bool{}
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if !ok {
			return "Malformed controller condition."
		}
		kind, _ := condition["type"].(string)
		if kind != "Ready" && kind != "Reconciling" && kind != "Stalled" {
			continue
		}
		if seen[kind] {
			return "Duplicate controller conditions are ambiguous."
		}
		seen[kind] = true
		status, _ := condition["status"].(string)
		if status != "True" && status != "False" && status != "Unknown" {
			return "Malformed controller condition status."
		}
		if kind == "Ready" {
			e.ControllerStatus = "Ready=" + status
			observed, ok := condition["observedGeneration"].(int64)
			if status != "True" || !ok || observed != generation {
				return "Ready is missing, false or not for the current generation."
			}
			ready++
		} else if status != "False" {
			return "Controller is reconciling, stalled or has unknown condition state."
		}
	}
	if ready != 1 {
		return "A current-generation Ready condition is required."
	}
	attempted, _, err := unstructured.NestedString(obj.Object, "status", "lastAttemptedRevision")
	if err != nil || (attempted != "" && attempted != e.ReportedRevision) {
		return "Last attempted and last applied revisions disagree."
	}
	if revisionString(obj, "spec", "sourceRef", "name") == "" {
		return "Source reference name is missing."
	}
	// Compare the Artifact revision, never the optional OCI origin annotation.
	kind := revisionString(obj, "spec", "sourceRef", "kind")
	prefix, suffix, hasPrefix := strings.Cut(e.ReportedRevision, "@")
	if hasPrefix && prefix == "" {
		return "Artifact revision prefix is missing."
	}
	switch kind {
	case "GitRepository":
		if !hasPrefix || !strings.HasPrefix(suffix, "sha1:") || !controllerGitCommit.MatchString(strings.TrimPrefix(suffix, "sha1:")) {
			return "Reported Git artifact revision is not <ref>@sha1:<full-commit>."
		}
		e.Revision = strings.TrimPrefix(suffix, "sha1:")
	case "OCIRepository":
		if !hasPrefix {
			suffix = e.ReportedRevision
		}
		if !controllerOCIDigest.MatchString(suffix) {
			return "Reported OCI artifact revision is not an exact SHA-256 digest."
		}
		e.Revision = suffix
	default:
		return "Source kind is not supported for immutable revision comparison."
	}
	return ""
}
