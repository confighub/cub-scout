// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const sveltosObservationLimit = 32
const sveltosEntryLimit = 16
const sveltosStringLimit = 512

// SveltosControllerEvidence preserves controller reports from one management
// context. References are reported names, never joined workload/cluster reads.
type SveltosControllerEvidence struct {
	Schema                  string                         `json:"schema"`
	Observations            []SveltosControllerObservation `json:"observations"`
	OmittedObservationCount int                            `json:"omittedObservationCount"`
	WorkloadHealth          string                         `json:"workloadHealth"`
	CheckFreshness          string                         `json:"checkFreshness"`
}

type SveltosSourceIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	UID        string `json:"uid,omitempty"`
}

type SveltosControllerObservation struct {
	Source                       SveltosSourceIdentity                `json:"source"`
	Category                     string                               `json:"category"`
	Coverage                     string                               `json:"coverage"`
	ReportedCluster              map[string]string                    `json:"reportedCluster,omitempty"`
	Features                     []SveltosFeatureObservation          `json:"features,omitempty"`
	FeatureCount                 int                                  `json:"featureCount,omitempty"`
	OmittedFeatureCount          int                                  `json:"omittedFeatureCount,omitempty"`
	ClusterConditions            []SveltosClusterConditionObservation `json:"clusterConditions,omitempty"`
	ClusterConditionCount        int                                  `json:"clusterConditionCount,omitempty"`
	OmittedClusterConditionCount int                                  `json:"omittedClusterConditionCount,omitempty"`
	Omissions                    []string                             `json:"omissions,omitempty"`
}

type SveltosFeatureObservation struct {
	FeatureID       string `json:"featureId"`
	ReportedStatus  string `json:"reportedStatus"`
	FailureReason   string `json:"failureReason,omitempty"`
	FailureMessage  string `json:"failureMessage,omitempty"`
	LastAppliedTime string `json:"lastAppliedTime,omitempty"`
}

type SveltosClusterConditionObservation struct {
	ReportedCluster       map[string]string                   `json:"reportedCluster"`
	Conditions            []SveltosHealthConditionObservation `json:"conditions"`
	ConditionCount        int                                 `json:"conditionCount"`
	OmittedConditionCount int                                 `json:"omittedConditionCount"`
}

type SveltosHealthConditionObservation struct {
	Type               string `json:"type"`
	ReportedStatus     string `json:"reportedStatus"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// projectSveltosControllerObservation uses only fields read by the pinned
// sveltos-confighub reporter at 8187910f9fe226e109e55c4d9c7c0e21297ff424.
// It preserves reports without adopting that reporter's time/prefix joins or
// interpreting Provisioned/True as workload health, release or gate acceptance.
func projectSveltosControllerObservation(obj *unstructured.Unstructured) *SveltosControllerObservation {
	if obj == nil {
		return nil
	}
	kind := obj.GetKind()
	if kind != "ClusterSummary" && kind != "ClusterHealthCheck" {
		return nil
	}
	row := &SveltosControllerObservation{
		Source:   SveltosSourceIdentity{obj.GetAPIVersion(), kind, obj.GetNamespace(), obj.GetName(), string(obj.GetUID())},
		Coverage: "reported",
	}
	omit := func(value string) { row.Omissions = append(row.Omissions, value) }
	text := func(values map[string]interface{}, key, path string, required bool) string {
		raw, present := values[key]
		if !present || raw == nil {
			if required {
				omit(path + ": missing")
			}
			return ""
		}
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			omit(path + ": invalid string")
			return ""
		}
		if !utf8.ValidString(value) {
			omit(path + ": invalid UTF-8")
			return ""
		}
		if len(value) > sveltosStringLimit {
			omit(path + ": truncated")
			value = value[:sveltosStringLimit]
			for !utf8.ValidString(value) {
				value = value[:len(value)-1]
			}
		}
		return value
	}
	reportedTime := func(values map[string]interface{}, key, path string) string {
		value := text(values, key, path, false)
		if value != "" {
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				omit(path + ": invalid reported timestamp")
			}
		}
		return value
	}
	entries := func(path ...string) []interface{} {
		values, found, err := unstructured.NestedSlice(obj.Object, path...)
		if err != nil {
			omit(strings.Join(path, ".") + ": invalid array")
			return nil
		}
		if !found || len(values) == 0 {
			omit(strings.Join(path, ".") + ": unavailable")
			return nil
		}
		return values
	}
	if kind == "ClusterSummary" {
		row.Category = "delivery"
	} else {
		row.Category = "continuous-health"
	}
	for key, value := range map[string]*string{"apiVersion": &row.Source.APIVersion, "namespace": &row.Source.Namespace, "name": &row.Source.Name, "uid": &row.Source.UID} {
		if len(*value) > sveltosStringLimit || !utf8.ValidString(*value) {
			*value = ""
			omit("source." + key + ": invalid or oversized identity")
		}
	}
	if row.Source.Name == "" {
		omit("source.name: missing")
	}
	if kind == "ClusterSummary" && row.Source.Namespace == "" {
		omit("source.namespace: missing for namespaced ClusterSummary")
	}
	if row.Source.UID == "" {
		omit("source.uid: missing; source instance identity incomplete")
	}
	expectedAPI := "config.projectsveltos.io/v1beta1"
	if kind == "ClusterHealthCheck" {
		expectedAPI = "lib.projectsveltos.io/v1beta1"
	}
	if row.Source.APIVersion != expectedAPI {
		row.Coverage = "unknown"
		omit("source.apiVersion: unsupported report contract")
		sort.Strings(row.Omissions)
		return row
	}
	if kind == "ClusterSummary" {
		row.Category = "delivery"
		row.ReportedCluster = map[string]string{}
		spec, _, err := unstructured.NestedMap(obj.Object, "spec")
		if err != nil {
			omit("spec: invalid object")
		}
		for _, key := range []string{"clusterNamespace", "clusterName", "clusterType"} {
			if value := text(spec, key, "spec."+key, key != "clusterType"); value != "" {
				row.ReportedCluster[key] = value
			}
		}
		values := entries("status", "featureSummaries")
		row.FeatureCount = len(values)
		sortSveltosJSON(values)
		seen := map[string]bool{}
		for i, raw := range values {
			if i >= sveltosEntryLimit {
				row.OmittedFeatureCount++
				continue
			}
			value, ok := raw.(map[string]interface{})
			if !ok {
				row.OmittedFeatureCount++
				omit("status.featureSummaries[]: invalid object")
				continue
			}
			feature := SveltosFeatureObservation{
				FeatureID:       text(value, "featureID", "feature.featureID", true),
				ReportedStatus:  text(value, "status", "feature.status", true),
				FailureReason:   text(value, "failureReason", "feature.failureReason", false),
				FailureMessage:  text(value, "failureMessage", "feature.failureMessage", false),
				LastAppliedTime: reportedTime(value, "lastAppliedTime", "feature.lastAppliedTime"),
			}
			if feature.FeatureID != "" && seen[feature.FeatureID] {
				omit("feature.featureID: duplicate; reports not reconciled")
			}
			seen[feature.FeatureID] = true
			row.Features = append(row.Features, feature)
		}
		sortSveltosJSON(row.Features)
		if row.OmittedFeatureCount > 0 {
			omit("status.featureSummaries: entries omitted")
		}
		if len(row.Features) == 0 {
			row.Coverage = "unknown"
		}
	} else {
		row.Category = "continuous-health"
		values := entries("status", "clusterCondition")
		row.ClusterConditionCount = len(values)
		sortSveltosJSON(values)
		remainingConditions := sveltosEntryLimit
		for i, raw := range values {
			if i >= sveltosEntryLimit {
				row.OmittedClusterConditionCount++
				continue
			}
			value, ok := raw.(map[string]interface{})
			if !ok {
				row.OmittedClusterConditionCount++
				omit("status.clusterCondition[]: invalid object")
				continue
			}
			cluster := SveltosClusterConditionObservation{ReportedCluster: map[string]string{}, Conditions: []SveltosHealthConditionObservation{}}
			ref, _, err := unstructured.NestedMap(value, "clusterInfo", "cluster")
			if err != nil {
				omit("clusterInfo.cluster: invalid object")
			}
			for _, key := range []string{"apiVersion", "kind", "namespace", "name", "uid"} {
				if field := text(ref, key, "clusterInfo.cluster."+key, key == "namespace" || key == "name"); field != "" {
					cluster.ReportedCluster[key] = field
				}
			}
			conditions, found, err := unstructured.NestedSlice(value, "conditions")
			if err != nil || !found || len(conditions) == 0 {
				omit("cluster.conditions: unavailable or invalid")
			}
			cluster.ConditionCount = len(conditions)
			sortSveltosJSON(conditions)
			for j, rawCondition := range conditions {
				if remainingConditions == 0 {
					cluster.OmittedConditionCount += len(conditions) - j
					break
				}
				remainingConditions--
				condition, ok := rawCondition.(map[string]interface{})
				if !ok {
					cluster.OmittedConditionCount++
					omit("cluster.conditions[]: invalid object")
					continue
				}
				cluster.Conditions = append(cluster.Conditions, SveltosHealthConditionObservation{
					Type:               text(condition, "type", "condition.type", true),
					ReportedStatus:     text(condition, "status", "condition.status", true),
					Reason:             text(condition, "reason", "condition.reason", false),
					Message:            text(condition, "message", "condition.message", false),
					LastTransitionTime: reportedTime(condition, "lastTransitionTime", "condition.lastTransitionTime"),
				})
			}
			if cluster.OmittedConditionCount > 0 {
				omit("cluster.conditions: entries omitted")
			}
			sortSveltosJSON(cluster.Conditions)
			row.ClusterConditions = append(row.ClusterConditions, cluster)
		}
		sortSveltosJSON(row.ClusterConditions)
		if row.OmittedClusterConditionCount > 0 {
			omit("status.clusterCondition: entries omitted")
		}
		if len(row.ClusterConditions) == 0 {
			row.Coverage = "unknown"
		}
	}
	sort.Strings(row.Omissions)
	row.Omissions = uniqueSveltosStrings(row.Omissions)
	if len(row.Omissions) > 0 && row.Coverage == "reported" {
		row.Coverage = "partial"
	}
	return row
}

func sortSveltosJSON[T any](values []T) {
	sort.Slice(values, func(i, j int) bool {
		a, _ := json.Marshal(values[i])
		b, _ := json.Marshal(values[j])
		return string(a) < string(b)
	})
}

func uniqueSveltosStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func appendSveltosObservation(evidence **SveltosControllerEvidence, obj *unstructured.Unstructured) {
	row := projectSveltosControllerObservation(obj)
	if row == nil {
		return
	}
	if *evidence == nil {
		*evidence = &SveltosControllerEvidence{Schema: "sveltos.controllerObservations.v1", WorkloadHealth: "unknown", CheckFreshness: "unknown", Observations: []SveltosControllerObservation{}}
	}
	if len((*evidence).Observations) >= sveltosObservationLimit {
		(*evidence).OmittedObservationCount++
		return
	}
	(*evidence).Observations = append((*evidence).Observations, *row)
}

// Both renderers use the same rows. Markdown code spans and quoted ASCII
// fields prevent externally reported text from adding structure or controls.
func renderSveltosControllerEvidence(evidence *SveltosControllerEvidence, markdown bool) string {
	if evidence == nil {
		return ""
	}
	quote := strconv.Quote
	if markdown {
		quote = func(value string) string { return gitOpsMarkdownCodeSpan(safeGitOpsTUIContent(value)) }
	}
	var b strings.Builder
	if markdown {
		b.WriteString("\n## Sveltos Controller Reports\n\n")
	} else {
		b.WriteString("Sveltos controller reports\n")
	}
	b.WriteString("Workload health: unknown; underlying check freshness: unknown. Provisioned and transition time are reported controller facts.\n")
	if evidence.OmittedObservationCount > 0 {
		fmt.Fprintf(&b, "Omitted controller observations: %d\n", evidence.OmittedObservationCount)
	}
	for _, row := range evidence.Observations {
		fmt.Fprintf(&b, "\n- %s %s %s/%s UID=%s: %s coverage=%s\n", quote(row.Category), quote(row.Source.Kind), quote(row.Source.Namespace), quote(row.Source.Name), quote(row.Source.UID), quote(row.Source.APIVersion), quote(row.Coverage))
		if len(row.ReportedCluster) > 0 {
			raw, _ := json.Marshal(row.ReportedCluster)
			fmt.Fprintf(&b, "  Reported cluster fields (not a verified cluster binding): %s\n", quote(string(raw)))
		}
		for _, feature := range row.Features {
			fmt.Fprintf(&b, "  Feature %s: reported status=%s lastAppliedTime=%s failureReason=%s message=%s\n", quote(feature.FeatureID), quote(feature.ReportedStatus), quote(feature.LastAppliedTime), quote(feature.FailureReason), quote(feature.FailureMessage))
		}
		for _, cluster := range row.ClusterConditions {
			raw, _ := json.Marshal(cluster.ReportedCluster)
			fmt.Fprintf(&b, "  Reported cluster reference (no delivery join): %s\n", quote(string(raw)))
			for _, condition := range cluster.Conditions {
				fmt.Fprintf(&b, "  Condition %s: reported status=%s lastTransitionTime=%s reason=%s message=%s\n", quote(condition.Type), quote(condition.ReportedStatus), quote(condition.LastTransitionTime), quote(condition.Reason), quote(condition.Message))
			}
			if cluster.OmittedConditionCount > 0 {
				fmt.Fprintf(&b, "  Omitted conditions: %d\n", cluster.OmittedConditionCount)
			}
		}
		if row.OmittedFeatureCount > 0 || row.OmittedClusterConditionCount > 0 {
			fmt.Fprintf(&b, "  Omitted feature/cluster entries: %d/%d\n", row.OmittedFeatureCount, row.OmittedClusterConditionCount)
		}
		for _, omission := range row.Omissions {
			fmt.Fprintf(&b, "  Omission: %s\n", quote(omission))
		}
	}
	b.WriteString("\nNo release identity, workload-health verdict, gate acceptance or current check time is inferred.\n\n")
	return b.String()
}
