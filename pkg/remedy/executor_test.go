package remedy

import (
	"context"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	reg.Register(NewConfigFixSuggester())
	reg.Register(NewTriggerActionSuggester())
	reg.Register(NewDeleteResourceSuggester())
	reg.Register(NewRestartSuggester())

	types := reg.Types()
	if len(types) != 4 {
		t.Errorf("expected 4 types, got %d", len(types))
	}

	s, ok := reg.Get(ConfigFix)
	if !ok {
		t.Error("expected to find ConfigFix suggester")
	}
	if s.Type() != ConfigFix {
		t.Errorf("expected ConfigFix, got %s", s.Type())
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

func TestConfigFixSuggester_CanSuggest(t *testing.T) {
	s := NewConfigFixSuggester()

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
			result := s.CanSuggest(finding)
			if result != tc.expected {
				t.Errorf("CanSuggest() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestTriggerActionSuggester_CanSuggest(t *testing.T) {
	s := NewTriggerActionSuggester()

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
			result := s.CanSuggest(finding)
			if result != tc.expected {
				t.Errorf("CanSuggest() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestDeleteResourceSuggester_CanSuggest(t *testing.T) {
	s := NewDeleteResourceSuggester()

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
			result := s.CanSuggest(finding)
			if result != tc.expected {
				t.Errorf("CanSuggest() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestRestartSuggester_CanSuggest(t *testing.T) {
	s := NewRestartSuggester()

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
			result := s.CanSuggest(finding)
			if result != tc.expected {
				t.Errorf("CanSuggest() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestSuggest_ConfigFix(t *testing.T) {
	s := NewConfigFixSuggester()
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

	suggestion, err := s.Suggest(ctx, finding)
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}

	if suggestion.RiskLevel != RiskLow {
		t.Errorf("expected RiskLow, got %s", suggestion.RiskLevel)
	}

	if !suggestion.Reversible {
		t.Error("expected config_fix to be reversible")
	}

	if len(suggestion.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(suggestion.Actions))
	}
}

func TestSuggest_DeleteResource(t *testing.T) {
	s := NewDeleteResourceSuggester()
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

	suggestion, err := s.Suggest(ctx, finding)
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}

	if suggestion.RiskLevel != RiskHigh {
		t.Errorf("expected RiskHigh, got %s", suggestion.RiskLevel)
	}

	if suggestion.Reversible {
		t.Error("expected delete_resource suggestion to be marked NOT reversible")
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
