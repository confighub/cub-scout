// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func sveltosFixture(t *testing.T) []*unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "evals", "sveltos-controller-facts", "fixtures", "cluster", "observations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Objects
}

func TestSveltosControllerFactsSeparateReports(t *testing.T) {
	objects := sveltosFixture(t)
	if len(objects) != 2 {
		t.Fatalf("fixture objects=%d", len(objects))
	}
	var facts *SveltosControllerEvidence
	for _, object := range objects {
		appendSveltosObservation(&facts, object)
	}
	if facts.WorkloadHealth != "unknown" || facts.CheckFreshness != "unknown" {
		t.Fatalf("unsupported health: %+v", facts)
	}
	delivery, health := facts.Observations[0], facts.Observations[1]
	if delivery.Category != "delivery" || delivery.Features[0].ReportedStatus != "Provisioned" || delivery.Source.UID != "summary-instance" || delivery.ReportedCluster["clusterName"] != "prod" {
		t.Fatalf("delivery=%+v", delivery)
	}
	if health.Category != "continuous-health" || health.ClusterConditions[0].Conditions[0].ReportedStatus != "False" || health.Source.UID != "watch-instance" || health.ClusterConditions[0].ReportedCluster["uid"] != "target-instance" {
		t.Fatalf("health=%+v", health)
	}
	if delivery.Coverage != "reported" || health.Coverage != "reported" {
		t.Fatalf("coverage %s/%s", delivery.Coverage, health.Coverage)
	}
	summary := GitOpsSummary{Backend: "controllers", Transport: "unknown", SveltosControllerReports: facts}
	example, err := os.ReadFile(filepath.Join("..", "..", "examples", "sveltos-controller-facts", "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected GitOpsSummary
	if err := json.Unmarshal(example, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summary, expected) {
		t.Fatalf("authored example does not match projection: got=%+v want=%+v", summary, expected)
	}
	raw, _ := json.Marshal(summary)
	if !strings.Contains(string(raw), `"sveltosControllerReports"`) || strings.Contains(string(raw), `"joined"`) {
		t.Fatalf("JSON=%s", raw)
	}
	for _, view := range []string{renderSveltosControllerEvidence(facts, false), renderGitOpsStatusMarkdown(summary), safeGitOpsTUIContent(renderGitOpsStatusMarkdown(summary))} {
		for _, want := range []string{"Provisioned", "False", "lastAppliedTime", "lastTransitionTime", "no delivery join", "underlying check freshness: unknown"} {
			if !strings.Contains(view, want) {
				t.Errorf("view missing %q: %s", want, view)
			}
		}
	}
	if renderSveltosControllerEvidence(nil, false) != "" || strings.Contains(renderGitOpsStatusMarkdown(GitOpsSummary{}), "Sveltos Controller Reports") {
		t.Fatal("absent facts changed default output")
	}
}

func TestSveltosControllerFactsDegradeAndBound(t *testing.T) {
	original := sveltosFixture(t)[0]
	for _, mutation := range []func(*unstructured.Unstructured){
		func(o *unstructured.Unstructured) { o.SetUID("") },
		func(o *unstructured.Unstructured) { o.SetNamespace("") },
		func(o *unstructured.Unstructured) { unstructured.RemoveNestedField(o.Object, "status") },
		func(o *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(o.Object, "bad", "status", "featureSummaries")
		},
		func(o *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(o.Object, []interface{}{true}, "status", "featureSummaries")
		},
		func(o *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(o.Object, []interface{}{map[string]interface{}{"featureID": "Resources", "status": true}}, "status", "featureSummaries")
		},
		func(o *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(o.Object, []interface{}{map[string]interface{}{"featureID": "Resources", "status": "FutureStatus", "lastAppliedTime": "bad"}}, "status", "featureSummaries")
		},
	} {
		object := original.DeepCopy()
		mutation(object)
		row := projectSveltosControllerObservation(object)
		if row.Coverage == "reported" || len(row.Omissions) == 0 {
			t.Fatalf("missing degradation: %+v", row)
		}
	}
	object := original.DeepCopy()
	values := []interface{}{}
	for i := 0; i < sveltosEntryLimit+3; i++ {
		values = append(values, map[string]interface{}{"featureID": string(rune('a' + i)), "status": "Provisioned"})
	}
	_ = unstructured.SetNestedSlice(object.Object, values, "status", "featureSummaries")
	first := projectSveltosControllerObservation(object)
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
	_ = unstructured.SetNestedSlice(object.Object, values, "status", "featureSummaries")
	second := projectSveltosControllerObservation(object)
	if !reflect.DeepEqual(first, second) || first.OmittedFeatureCount != 3 || first.Coverage != "partial" {
		t.Fatalf("unstable bounds: %+v / %+v", first, second)
	}
	var facts *SveltosControllerEvidence
	for i := 0; i < sveltosObservationLimit+2; i++ {
		appendSveltosObservation(&facts, original)
	}
	if len(facts.Observations) != sveltosObservationLimit || facts.OmittedObservationCount != 2 {
		t.Fatalf("root bounds: %+v", facts)
	}
	object = original.DeepCopy()
	object.SetAPIVersion("bad\x1b[31m")
	row := projectSveltosControllerObservation(object)
	view := renderSveltosControllerEvidence(&SveltosControllerEvidence{Observations: []SveltosControllerObservation{*row}}, false)
	if strings.ContainsRune(view, '\x1b') || strings.ContainsRune(view, '\u009b') || row.Coverage != "unknown" {
		t.Fatalf("unsafe unsupported API: %s", view)
	}
}

func TestSveltosControllerCollectorUsesExistingScopedReads(t *testing.T) {
	objects := sveltosFixture(t)
	kinds := map[schema.GroupVersionResource]string{}
	for _, spec := range firstClassControllerResources() {
		kinds[spec.GVR] = spec.Kind + "List"
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objects[0], objects[1])
	summary := buildGitOpsSummary(context.Background(), client, &agent.ApplyBackendInfo{Backend: agent.BackendNone, Transport: agent.TransportUnknown})
	if summary.SveltosControllerReports == nil || len(summary.SveltosControllerReports.Observations) != 2 {
		t.Fatalf("facts missing: %+v", summary)
	}
	if len(client.Actions()) != len(firstClassControllerResources()) {
		t.Fatalf("added requests: %d", len(client.Actions()))
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" {
			t.Fatalf("unexpected request: %v", action)
		}
	}
	// Existing controller Ready/stage projection remains the only deployer
	// verdict input; new feature/health report facts do not change its values.
	for _, dep := range summary.Deployers {
		if dep.Kind == "ClusterHealthCheck" {
			want := controllerDeployerStatus(objects[1], controllerResourceSpec{Kind: "ClusterHealthCheck", Owner: "Sveltos"})
			if !reflect.DeepEqual(dep, want) {
				t.Fatalf("legacy verdict changed: %+v", dep)
			}
		}
	}
	denied := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds)
	denied.PrependReactor("list", "clustersummaries", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "config.projectsveltos.io", Resource: "clustersummaries"}, "", nil)
	})
	_, coverage, facts := collectFirstClassControllerStatus(context.Background(), denied, "management")
	if facts != nil {
		t.Fatal("denial manufactured empty observation evidence")
	}
	family := controllerCoverageForFamily(t, coverage, "Sveltos")
	if len(family.Omissions) == 0 {
		t.Fatal("denial missing coverage omission")
	}

}

func TestSveltosControllerRenderingQuotesExternalMarkup(t *testing.T) {
	object := sveltosFixture(t)[0]
	hostile := "[click](https://example.invalid) <img src=x> `code`\n\x1b[31m\u009b1m"
	object.SetName(hostile)
	_ = unstructured.SetNestedField(object.Object, hostile, "spec", "clusterName")
	_ = unstructured.SetNestedSlice(object.Object, []interface{}{map[string]interface{}{"featureID": "Resources", "status": "Provisioned", "failureMessage": hostile}}, "status", "featureSummaries")
	row := projectSveltosControllerObservation(object)
	facts := &SveltosControllerEvidence{Observations: []SveltosControllerObservation{*row}}
	for _, markdown := range []bool{false, true} {
		view := renderSveltosControllerEvidence(facts, markdown)
		if strings.ContainsRune(view, '\x1b') || strings.ContainsRune(view, '\u009b') {
			t.Fatal("terminal escape was emitted")
		}
		if markdown && !strings.Contains(view, gitOpsMarkdownCodeSpan(safeGitOpsTUIContent(hostile))) {
			t.Fatalf("Markdown value escaped incorrectly: %s", view)
		}
	}
}

func TestSveltosHealthConditionsBoundAndUnknownCoverage(t *testing.T) {
	object := sveltosFixture(t)[1]
	cluster := map[string]interface{}{"clusterInfo": map[string]interface{}{"cluster": map[string]interface{}{"namespace": "projectsveltos", "name": "prod"}}}
	conditions := []interface{}{}
	for i := 0; i < sveltosEntryLimit+3; i++ {
		conditions = append(conditions, map[string]interface{}{"type": string(rune('a' + i)), "status": "False"})
	}
	cluster["conditions"] = conditions
	_ = unstructured.SetNestedSlice(object.Object, []interface{}{cluster, cluster}, "status", "clusterCondition")
	row := projectSveltosControllerObservation(object)
	count, omitted := 0, 0
	for _, value := range row.ClusterConditions {
		count += len(value.Conditions)
		omitted += value.OmittedConditionCount
	}
	if count != sveltosEntryLimit || omitted != 2*(sveltosEntryLimit+3)-sveltosEntryLimit || row.Coverage != "partial" {
		t.Fatalf("health condition bounds incorrect: %+v", row)
	}
	for i, j := 0, len(conditions)-1; i < j; i, j = i+1, j-1 {
		conditions[i], conditions[j] = conditions[j], conditions[i]
	}
	cluster["conditions"] = conditions
	_ = unstructured.SetNestedSlice(object.Object, []interface{}{cluster, cluster}, "status", "clusterCondition")
	if !reflect.DeepEqual(row, projectSveltosControllerObservation(object)) {
		t.Fatal("health condition ordering unstable")
	}
	unstructured.RemoveNestedField(object.Object, "status")
	unknown := projectSveltosControllerObservation(object)
	if unknown.Coverage != "unknown" || len(unknown.Omissions) == 0 {
		t.Fatalf("missing health became a successful empty report: %+v", unknown)
	}
}
