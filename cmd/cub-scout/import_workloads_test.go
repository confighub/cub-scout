// Direct integration coverage for discoverWorkloadsFrom and
// getManagedResources — back-fill for #394's narrow kstatus migration
// per #396. Both functions previously had no fake-clientset coverage,
// so the kstatus tightening (Stalled → Failed) only had wrapper-level
// tests; this file proves the readiness verdict actually flows through
// the call sites end-to-end.

package main

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

// ----------------------------------------------------------------------
// Helpers — workload-shape builders. The kstatus wrapper (#394) needs
// both replica counts AND the right Conditions to call a workload
// "Current"; happy fixtures populate both, sad fixtures omit one.
// ----------------------------------------------------------------------

func ptrI32(v int32) *int32 { return &v }

func happyDeployment(name, ns string, labels, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Generation:  1,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrI32(3),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "ghcr.io/example/app:1.0.0"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           3,
			ReadyReplicas:      3,
			AvailableReplicas:  3,
			UpdatedReplicas:    3,
			ObservedGeneration: 1,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
		},
	}
}

// stalledDeployment is the council's canonical trap: replica counts
// match (which would have slipped through the old ReadyReplicas==Replicas
// check) but Progressing=False with reason ProgressDeadlineExceeded.
// kstatus correctly classifies this as Failed, so Ready must be false
// after the migration.
func stalledDeployment(name, ns string) *appsv1.Deployment {
	d := happyDeployment(name, ns, nil, nil)
	d.Status.Conditions = []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"},
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	return d
}

// rollingDeployment is the in-progress shape: only 1/3 ready replicas,
// but conditions are still healthy. kstatus → InProgress → Ready=false.
func rollingDeployment(name, ns string) *appsv1.Deployment {
	d := happyDeployment(name, ns, nil, nil)
	d.Status.ReadyReplicas = 1
	d.Status.AvailableReplicas = 1
	d.Status.UpdatedReplicas = 1
	return d
}

func happyStatefulSet(name, ns string) *appsv1.StatefulSet {
	rev := name + "-1"
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrI32(3),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "ghcr.io/example/app:1.0.0"}},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:           3,
			ReadyReplicas:      3,
			AvailableReplicas:  3,
			CurrentReplicas:    3,
			CurrentRevision:    rev,
			UpdateRevision:     rev,
			ObservedGeneration: 1,
		},
	}
}

func rollingStatefulSet(name, ns string) *appsv1.StatefulSet {
	s := happyStatefulSet(name, ns)
	s.Status.ReadyReplicas = 1
	s.Status.AvailableReplicas = 1
	return s
}

func happyDaemonSet(name, ns string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "ghcr.io/example/app:1.0.0"}},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            5,
			NumberAvailable:        5,
			UpdatedNumberScheduled: 5,
			ObservedGeneration:     1,
		},
	}
}

func rollingDaemonSet(name, ns string) *appsv1.DaemonSet {
	d := happyDaemonSet(name, ns)
	d.Status.NumberReady = 3
	d.Status.NumberAvailable = 3
	d.Status.UpdatedNumberScheduled = 3
	return d
}

// emptyDynClient produces a dynamic client backed by an empty scheme +
// no objects. discoverWorkloadsFrom uses it only for getKustomizationPath
// / getApplicationPath lookups, which fail gracefully when nothing is
// present — exactly what we want for readiness-only tests.
func emptyDynClient(t *testing.T) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClient(scheme)
}

// ----------------------------------------------------------------------
// discoverWorkloadsFrom coverage — 3 kinds × happy/sad + Stalled trap.
// ----------------------------------------------------------------------

func TestDiscoverWorkloadsFrom_Readiness(t *testing.T) {
	const ns = "demo"

	tests := []struct {
		name       string
		objects    []runtime.Object
		wantKind   string
		wantName   string
		wantReady  bool
	}{
		{
			name:      "Deployment fully rolled out → Ready=true",
			objects:   []runtime.Object{happyDeployment("rag-server", ns, nil, nil)},
			wantKind:  "Deployment",
			wantName:  "rag-server",
			wantReady: true,
		},
		{
			name: "Deployment Stalled (kstatus Failed) → Ready=false (#394 trap)",
			// Pre-#394 this slipped through as ready because replica
			// counts matched. kstatus reads the condition and rejects.
			objects:   []runtime.Object{stalledDeployment("rag-server", ns)},
			wantKind:  "Deployment",
			wantName:  "rag-server",
			wantReady: false,
		},
		{
			name:      "Deployment rollout in progress → Ready=false",
			objects:   []runtime.Object{rollingDeployment("rag-server", ns)},
			wantKind:  "Deployment",
			wantName:  "rag-server",
			wantReady: false,
		},
		{
			name:      "StatefulSet fully rolled out → Ready=true",
			objects:   []runtime.Object{happyStatefulSet("vector-db", ns)},
			wantKind:  "StatefulSet",
			wantName:  "vector-db",
			wantReady: true,
		},
		{
			name:      "StatefulSet rolling → Ready=false",
			objects:   []runtime.Object{rollingStatefulSet("vector-db", ns)},
			wantKind:  "StatefulSet",
			wantName:  "vector-db",
			wantReady: false,
		},
		{
			name:      "DaemonSet fully scheduled → Ready=true",
			objects:   []runtime.Object{happyDaemonSet("node-exporter", ns)},
			wantKind:  "DaemonSet",
			wantName:  "node-exporter",
			wantReady: true,
		},
		{
			name:      "DaemonSet rolling → Ready=false",
			objects:   []runtime.Object{rollingDaemonSet("node-exporter", ns)},
			wantKind:  "DaemonSet",
			wantName:  "node-exporter",
			wantReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(tt.objects...)
			dyn := emptyDynClient(t)

			got, err := discoverWorkloadsFrom(context.Background(), cs, dyn, ns)
			if err != nil {
				t.Fatalf("discoverWorkloadsFrom error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d workloads, want 1: %+v", len(got), got)
			}
			w := got[0]
			if w.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", w.Kind, tt.wantKind)
			}
			if w.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", w.Name, tt.wantName)
			}
			if w.Ready != tt.wantReady {
				t.Errorf("Ready = %v, want %v (Status: %+v)", w.Ready, tt.wantReady, tt.objects[0])
			}
		})
	}
}

// ----------------------------------------------------------------------
// getManagedResources coverage — 3 kinds × happy/sad on Argo-managed
// resources. Argo ownership is keyed by the
// `argocd.argoproj.io/instance` label — see isArgoManagedResource.
// ----------------------------------------------------------------------

func argoLabels(appName string) map[string]string {
	return map[string]string{"argocd.argoproj.io/instance": appName}
}

func TestGetManagedResources_Health(t *testing.T) {
	const (
		ns      = "demo"
		appName = "rag-stack"
	)

	tests := []struct {
		name       string
		objects    []runtime.Object
		wantKind   string
		wantHealth string
	}{
		{
			name:       "Argo-managed Deployment fully rolled out → Healthy",
			objects:    []runtime.Object{happyDeployment("rag-server", ns, argoLabels(appName), nil)},
			wantKind:   "Deployment",
			wantHealth: "Healthy",
		},
		{
			name: "Argo-managed Deployment Stalled → Progressing (#394 trap)",
			objects: func() []runtime.Object {
				d := stalledDeployment("rag-server", ns)
				d.Labels = argoLabels(appName)
				return []runtime.Object{d}
			}(),
			wantKind:   "Deployment",
			wantHealth: "Progressing",
		},
		{
			name: "Argo-managed StatefulSet fully rolled out → Healthy",
			objects: func() []runtime.Object {
				s := happyStatefulSet("vector-db", ns)
				s.Labels = argoLabels(appName)
				return []runtime.Object{s}
			}(),
			wantKind:   "StatefulSet",
			wantHealth: "Healthy",
		},
		{
			name: "Argo-managed StatefulSet rolling → Progressing",
			objects: func() []runtime.Object {
				s := rollingStatefulSet("vector-db", ns)
				s.Labels = argoLabels(appName)
				return []runtime.Object{s}
			}(),
			wantKind:   "StatefulSet",
			wantHealth: "Progressing",
		},
		{
			name: "Argo-managed DaemonSet fully scheduled → Healthy",
			objects: func() []runtime.Object {
				d := happyDaemonSet("node-exporter", ns)
				d.Labels = argoLabels(appName)
				return []runtime.Object{d}
			}(),
			wantKind:   "DaemonSet",
			wantHealth: "Healthy",
		},
		{
			name: "Argo-managed DaemonSet rolling → Progressing",
			objects: func() []runtime.Object {
				d := rollingDaemonSet("node-exporter", ns)
				d.Labels = argoLabels(appName)
				return []runtime.Object{d}
			}(),
			wantKind:   "DaemonSet",
			wantHealth: "Progressing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(tt.objects...)
			dyn := emptyDynClient(t)

			got, err := getManagedResources(context.Background(), cs, dyn, appName, ns)
			if err != nil {
				t.Fatalf("getManagedResources error: %v", err)
			}

			// Find the resource of the expected kind. The function may
			// also pick up Services / ConfigMaps if seeded; we only
			// assert on workloads.
			var workload *ManagedResource
			for i := range got {
				if got[i].Kind == tt.wantKind {
					workload = &got[i]
					break
				}
			}
			if workload == nil {
				t.Fatalf("no %s in result: %+v", tt.wantKind, got)
			}
			if workload.Health != tt.wantHealth {
				t.Errorf("Health = %q, want %q", workload.Health, tt.wantHealth)
			}
		})
	}
}

// TestGetManagedResources_FilterByOwnership locks in that resources
// without the Argo instance label are excluded from the result, even
// when present in the cluster snapshot.
func TestGetManagedResources_FilterByOwnership(t *testing.T) {
	const ns = "demo"
	const appName = "rag-stack"

	owned := happyDeployment("rag-server", ns, argoLabels(appName), nil)
	unowned := happyDeployment("unrelated", ns, nil, nil)

	cs := fake.NewSimpleClientset(owned, unowned)
	dyn := emptyDynClient(t)

	got, err := getManagedResources(context.Background(), cs, dyn, appName, ns)
	if err != nil {
		t.Fatalf("getManagedResources error: %v", err)
	}

	for _, r := range got {
		if r.Name == "unrelated" {
			t.Fatalf("unowned resource %q included; ownership filter regression", r.Name)
		}
	}
}
