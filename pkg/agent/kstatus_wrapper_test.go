package agent

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// ptrInt32 returns a pointer to an int32 — needed because Spec.Replicas is
// *int32 in the apps/v1 API.
func ptrInt32(v int32) *int32 { return &v }

// makeDeployment builds a Deployment whose .status reflects the supplied
// generation/observedGeneration/ready/spec values. Helper keeps the table
// tests below readable.
//
// Caveat: kstatus's deployment classifier wants either Progressing=True
// reason=NewReplicaSetAvailable AND Available=True conditions, or no
// progressDeadlineSeconds set on the spec. Real Deployments populate these
// conditions in normal operation. Pass extra conditions to override.
func makeDeployment(replicas, ready, available int32, gen, observedGen int64, conditions ...appsv1.DeploymentCondition) *appsv1.Deployment {
	if len(conditions) == 0 {
		conditions = []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
			{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
		}
	}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "default",
			Generation: gen,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(replicas),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           replicas,
			ReadyReplicas:      ready,
			AvailableReplicas:  available,
			UpdatedReplicas:    ready,
			ObservedGeneration: observedGen,
			Conditions:         conditions,
		},
	}
	return d
}

func TestIsDeploymentReady(t *testing.T) {
	tests := []struct {
		name   string
		dep    *appsv1.Deployment
		wantUp bool
	}{
		{
			name:   "fully rolled out — kstatus Current",
			dep:    makeDeployment(3, 3, 3, 1, 1),
			wantUp: true,
		},
		{
			name:   "rollout in progress — kstatus InProgress",
			dep:    makeDeployment(3, 1, 1, 1, 1),
			wantUp: false,
		},
		{
			name: "observedGeneration lag — kstatus InProgress (status reflects old spec)",
			dep:  makeDeployment(3, 3, 3, 2, 1), // gen=2, observed=1 ⇒ stale
			wantUp: false,
		},
		{
			name: "stalled deployment — kstatus Failed (was 'ready or unknown' under prior helper, now correctly false)",
			// #394 behaviour delta: a Stalled=True / Progressing=False ReplicaFailure
			// previously slipped through the ad-hoc check because ReadyReplicas matched
			// Spec.Replicas. kstatus reads the conditions and returns Failed.
			dep: makeDeployment(3, 3, 3, 1, 1,
				appsv1.DeploymentCondition{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "ProgressDeadlineExceeded",
				},
			),
			wantUp: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDeploymentReady(tt.dep); got != tt.wantUp {
				t.Fatalf("IsDeploymentReady = %v, want %v", got, tt.wantUp)
			}
		})
	}
}

func TestIsStatefulSetReady(t *testing.T) {
	curRev := "demo-7d9"
	makeSS := func(replicas, ready, available, currentReplicas int32, gen, observedGen int64) *appsv1.StatefulSet {
		return &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "demo",
				Namespace:  "default",
				Generation: gen,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas:            ptrInt32(replicas),
				UpdateStrategy:      appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType},
				PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			},
			Status: appsv1.StatefulSetStatus{
				Replicas:           replicas,
				ReadyReplicas:      ready,
				AvailableReplicas:  available,
				CurrentReplicas:    currentReplicas,
				CurrentRevision:    curRev,
				UpdateRevision:     curRev,
				ObservedGeneration: observedGen,
			},
		}
	}

	tests := []struct {
		name string
		ss   *appsv1.StatefulSet
		want bool
	}{
		{name: "fully rolled out", ss: makeSS(3, 3, 3, 3, 1, 1), want: true},
		{name: "rollout in progress (only 1 ready)", ss: makeSS(3, 1, 1, 3, 1, 1), want: false},
		{name: "generation lag", ss: makeSS(3, 3, 3, 3, 2, 1), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStatefulSetReady(tt.ss); got != tt.want {
				t.Fatalf("IsStatefulSetReady = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDaemonSetReady(t *testing.T) {
	makeDS := func(desired, ready, available, updated int32, gen, observedGen int64) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "demo",
				Namespace:  "default",
				Generation: gen,
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: func() *intstr.IntOrString { v := intstr.FromInt(1); return &v }(),
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: desired,
				CurrentNumberScheduled: desired,
				NumberReady:            ready,
				NumberAvailable:        available,
				UpdatedNumberScheduled: updated,
				ObservedGeneration:     observedGen,
			},
		}
	}

	tests := []struct {
		name string
		ds   *appsv1.DaemonSet
		want bool
	}{
		{name: "fully scheduled and ready", ds: makeDS(5, 5, 5, 5, 1, 1), want: true},
		{name: "rollout in progress", ds: makeDS(5, 3, 3, 3, 1, 1), want: false},
		{name: "generation lag", ds: makeDS(5, 5, 5, 5, 2, 1), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDaemonSetReady(tt.ds); got != tt.want {
				t.Fatalf("IsDaemonSetReady = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkloadStatus_DistinguishesInProgressFromFailed(t *testing.T) {
	// Same readiness verdict (false) under IsDeploymentReady, but
	// WorkloadStatus must let callers distinguish the two cases for evidence
	// surfacing (e.g. #393's source-truth contract).
	rolling := makeDeployment(3, 1, 1, 1, 1) // 1/3 pods ready ⇒ InProgress
	stalled := makeDeployment(3, 3, 3, 1, 1,
		appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionFalse,
			Reason: "ProgressDeadlineExceeded",
		},
	)

	if s, _ := WorkloadStatus(rolling, "Deployment"); s != status.InProgressStatus {
		t.Fatalf("rolling deployment: want InProgress, got %s", s)
	}
	if s, _ := WorkloadStatus(stalled, "Deployment"); s != status.FailedStatus {
		t.Fatalf("stalled deployment: want Failed, got %s", s)
	}
}

// IsResourceReadyOrUnknown — the second slice of the #394 migration.
// Covers the behaviour table the helper documents; specifically locks in
// the Stalled=True delta vs. the prior receiver-method implementation.

func unstructuredFlux(kind string, conditions []map[string]interface{}) unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      "demo",
			"namespace": "flux-system",
		},
		"status": map[string]interface{}{},
	}
	if conditions != nil {
		// unstructured.NestedSlice expects []interface{}.
		condSlice := make([]interface{}, len(conditions))
		for i, c := range conditions {
			condSlice[i] = c
		}
		obj["status"].(map[string]interface{})["conditions"] = condSlice
	}
	return unstructured.Unstructured{Object: obj}
}

func TestIsResourceReadyOrUnknown(t *testing.T) {
	cases := []struct {
		name string
		obj  unstructured.Unstructured
		want bool
	}{
		{
			name: "no conditions → true (lenient, matches old helper)",
			obj:  unstructuredFlux("Kustomization", nil),
			want: true,
		},
		{
			name: "Ready=True → true",
			obj: unstructuredFlux("Kustomization", []map[string]interface{}{
				{"type": "Ready", "status": "True", "reason": "ReconciliationSucceeded"},
			}),
			want: true,
		},
		{
			name: "Ready=False → false",
			obj: unstructuredFlux("HelmRelease", []map[string]interface{}{
				{"type": "Ready", "status": "False", "reason": "InstallFailed"},
			}),
			want: false,
		},
		{
			name: "Stalled=True (Ready unset) → false (#394 delta — was true under old helper)",
			// Old helper only inspected Ready; with no Ready it returned
			// true. kstatus reads Stalled and reports Failed → false.
			obj: unstructuredFlux("HelmRelease", []map[string]interface{}{
				{"type": "Stalled", "status": "True", "reason": "RetriesExhausted"},
			}),
			want: false,
		},
		{
			name: "Stalled=True with Ready=True → false (Stalled wins under kstatus)",
			obj: unstructuredFlux("HelmRelease", []map[string]interface{}{
				{"type": "Ready", "status": "True", "reason": "ReconciliationSucceeded"},
				{"type": "Stalled", "status": "True", "reason": "RetriesExhausted"},
			}),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsResourceReadyOrUnknown(tc.obj); got != tc.want {
				t.Errorf("IsResourceReadyOrUnknown = %v, want %v", got, tc.want)
			}
		})
	}
}
