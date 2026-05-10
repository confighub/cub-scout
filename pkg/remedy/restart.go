package remedy

import (
	"context"
	"fmt"
	"strings"
)

// RestartSuggester describes a restart remedy as a structured suggestion.
// It does not restart anything.
type RestartSuggester struct {
	kubectl string
}

// NewRestartSuggester creates a new restart suggester.
func NewRestartSuggester() *RestartSuggester {
	return &RestartSuggester{
		kubectl: "kubectl",
	}
}

// Type returns Restart.
func (s *RestartSuggester) Type() RemedyType {
	return Restart
}

// CanSuggest reports whether this suggester can describe the finding.
func (s *RestartSuggester) CanSuggest(f *Finding) bool {
	for _, cmd := range f.Commands {
		if s.isRestartCommand(cmd) {
			return true
		}
	}
	return isWorkload(f.Resource.Kind)
}

// Suggest describes the restart that would resolve the finding without
// performing it.
func (s *RestartSuggester) Suggest(_ context.Context, f *Finding) (*SuggestedRemedy, error) {
	suggestion := &SuggestedRemedy{
		Finding:    f,
		Reversible: true, // A rollout restart can be undone via `kubectl rollout undo`.
		RiskLevel:  RiskMedium,
	}

	for _, cmd := range f.Commands {
		if s.isRestartCommand(cmd) {
			suggestion.Actions = append(suggestion.Actions, SuggestedAction{
				Description: "Restart workload",
				Command:     s.addNamespace(cmd, f.Namespace),
			})
		}
	}

	if len(suggestion.Actions) == 0 && isWorkload(f.Resource.Kind) {
		cmd := fmt.Sprintf("kubectl rollout restart %s/%s",
			strings.ToLower(f.Resource.Kind), f.Resource.Name)
		suggestion.Actions = append(suggestion.Actions, SuggestedAction{
			Description: fmt.Sprintf("Restart %s", f.Resource.Kind),
			Command:     s.addNamespace(cmd, f.Namespace),
		})
	}

	if len(suggestion.Actions) == 0 {
		return nil, fmt.Errorf("no restart actions describable for %s", f.CCVE)
	}

	return suggestion, nil
}

func (s *RestartSuggester) isRestartCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "rollout restart") ||
		strings.Contains(cmd, "delete pod") ||
		strings.Contains(cmd, "kill") ||
		strings.Contains(cmd, "restart")
}

func (s *RestartSuggester) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

// isWorkload checks if the kind is a restartable workload
func isWorkload(kind string) bool {
	kind = strings.ToLower(kind)
	return kind == "deployment" ||
		kind == "statefulset" ||
		kind == "daemonset" ||
		kind == "replicaset"
}
