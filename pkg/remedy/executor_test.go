package remedy

import (
	"context"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Register executors
	reg.Register(NewConfigFixExecutor())
	reg.Register(NewTriggerActionExecutor())
	reg.Register(NewDeleteResourceExecutor())
	reg.Register(NewRestartExecutor())

	// Test Types() returns all registered
	types := reg.Types()
	if len(types) != 4 {
		t.Errorf("expected 4 types, got %d", len(types))
	}

	// Test Get()
	exec, ok := reg.Get(ConfigFix)
	if !ok {
		t.Error("expected to find ConfigFix executor")
	}
	if exec.Type() != ConfigFix {
		t.Errorf("expected ConfigFix, got %s", exec.Type())
	}
}

func TestDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry()
	types := reg.Types()
	if len(types) != 4 {
		t.Errorf("expected 4 types in default registry, got %d", len(types))
	}
}

func TestIsAutoFixable(t *testing.T) {
	tests := []struct {
		remedyType RemedyType
		expected   bool
	}{
		{ConfigFix, true},
		{TriggerAction, true},
		{Restart, true},
		{DeleteResource, true},
		{DiagnoseThenFix, false},
		{ExternalAction, false},
		{SourceFix, false},
	}

	for _, tc := range tests {
		result := IsAutoFixable(tc.remedyType)
		if result != tc.expected {
			t.Errorf("IsAutoFixable(%s) = %v, expected %v", tc.remedyType, result, tc.expected)
		}
	}
}

func TestConfigFixExecutor_CanExecute(t *testing.T) {
	exec := NewConfigFixExecutor()

	tests := []struct {
		name     string
		commands []string
		expected bool
	}{
		{"kubectl apply", []string{"kubectl apply -f config.yaml"}, true},
		{"kubectl patch", []string{"kubectl patch deployment nginx -p '{...}'"}, true},
		{"kubectl annotate", []string{"kubectl annotate pod nginx foo=bar"}, true},
		{"kubectl label", []string{"kubectl label node node1 env=prod"}, true},
		{"kubectl get", []string{"kubectl get pods"}, false},
		{"kubectl delete", []string{"kubectl delete pod nginx"}, false},
		{"empty", []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finding := &Finding{
				CCVE:     "CCVE-2025-TEST",
				Commands: tc.commands,
			}
			result := exec.CanExecute(finding)
			if result != tc.expected {
				t.Errorf("CanExecute() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestTriggerActionExecutor_CanExecute(t *testing.T) {
	exec := NewTriggerActionExecutor()

	tests := []struct {
		name     string
		commands []string
		expected bool
	}{
		{"rollout restart", []string{"kubectl rollout restart deployment/nginx"}, true},
		{"rollout undo", []string{"kubectl rollout undo deployment/nginx"}, true},
		{"scale", []string{"kubectl scale deployment nginx --replicas=3"}, true},
		{"apply", []string{"kubectl apply -f config.yaml"}, false},
		{"empty", []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finding := &Finding{
				CCVE:     "CCVE-2025-TEST",
				Commands: tc.commands,
			}
			result := exec.CanExecute(finding)
			if result != tc.expected {
				t.Errorf("CanExecute() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestDeleteResourceExecutor_CanExecute(t *testing.T) {
	exec := NewDeleteResourceExecutor()

	tests := []struct {
		name     string
		commands []string
		expected bool
	}{
		{"kubectl delete", []string{"kubectl delete pod nginx"}, true},
		{"kubectl apply", []string{"kubectl apply -f config.yaml"}, false},
		{"empty", []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finding := &Finding{
				CCVE:     "CCVE-2025-TEST",
				Commands: tc.commands,
			}
			result := exec.CanExecute(finding)
			if result != tc.expected {
				t.Errorf("CanExecute() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestRestartExecutor_CanExecute(t *testing.T) {
	exec := NewRestartExecutor()

	tests := []struct {
		name     string
		commands []string
		kind     string
		expected bool
	}{
		{"rollout restart", []string{"kubectl rollout restart deployment/nginx"}, "", true},
		{"deployment kind", []string{}, "Deployment", true},
		{"statefulset kind", []string{}, "StatefulSet", true},
		{"daemonset kind", []string{}, "DaemonSet", true},
		{"pod kind", []string{}, "Pod", false},
		{"no commands no workload", []string{}, "ConfigMap", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finding := &Finding{
				CCVE:     "CCVE-2025-TEST",
				Commands: tc.commands,
				Resource: ResourceRef{Kind: tc.kind},
			}
			result := exec.CanExecute(finding)
			if result != tc.expected {
				t.Errorf("CanExecute() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestDryRun_ConfigFix(t *testing.T) {
	exec := NewConfigFixExecutor()
	ctx := context.Background()

	finding := &Finding{
		CCVE:      "CCVE-2025-TEST",
		Namespace: "default",
		Commands:  []string{"kubectl patch deployment nginx -p '{\"spec\":{\"replicas\":3}}'"},
		Resource: ResourceRef{
			Kind: "Deployment",
			Name: "nginx",
		},
	}

	plan, err := exec.DryRun(ctx, finding)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	if plan.RiskLevel != RiskLow {
		t.Errorf("expected RiskLow, got %s", plan.RiskLevel)
	}

	if !plan.Reversible {
		t.Error("expected config_fix to be reversible")
	}

	if len(plan.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(plan.Actions))
	}
}

func TestDryRun_DeleteResource(t *testing.T) {
	exec := NewDeleteResourceExecutor()
	ctx := context.Background()

	finding := &Finding{
		CCVE:      "CCVE-2025-TEST",
		Namespace: "default",
		Commands:  []string{"kubectl delete pod nginx-orphaned"},
		Resource: ResourceRef{
			Kind: "Pod",
			Name: "nginx-orphaned",
		},
	}

	plan, err := exec.DryRun(ctx, finding)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	if plan.RiskLevel != RiskHigh {
		t.Errorf("expected RiskHigh, got %s", plan.RiskLevel)
	}

	if plan.Reversible {
		t.Error("expected delete_resource to NOT be reversible")
	}
}

func TestResourceRefString(t *testing.T) {
	tests := []struct {
		ref      ResourceRef
		expected string
	}{
		{
			ref:      ResourceRef{Kind: "Deployment", Name: "nginx", Namespace: "default"},
			expected: "Deployment/nginx -n default",
		},
		{
			ref:      ResourceRef{Kind: "ClusterRole", Name: "admin"},
			expected: "ClusterRole/admin",
		},
	}

	for _, tc := range tests {
		result := tc.ref.String()
		if result != tc.expected {
			t.Errorf("String() = %q, expected %q", result, tc.expected)
		}
	}
}
