package remedy

import (
	"context"
	"fmt"
	"strings"
)

// TriggerActionSuggester describes a trigger_action remedy as a structured
// suggestion (rollout restart, scale, etc.). It does not perform the action.
type TriggerActionSuggester struct {
	kubectl string
}

// NewTriggerActionSuggester creates a new trigger-action suggester.
func NewTriggerActionSuggester() *TriggerActionSuggester {
	return &TriggerActionSuggester{
		kubectl: "kubectl",
	}
}

// Type returns TriggerAction.
func (s *TriggerActionSuggester) Type() RemedyType {
	return TriggerAction
}

// CanSuggest reports whether this suggester can describe the finding.
func (s *TriggerActionSuggester) CanSuggest(f *Finding) bool {
	for _, cmd := range f.Commands {
		if s.isActionCommand(cmd) {
			return true
		}
	}
	return false
}

// Suggest describes the action that would resolve the finding without
// performing it.
func (s *TriggerActionSuggester) Suggest(_ context.Context, f *Finding) (*SuggestedRemedy, error) {
	suggestion := &SuggestedRemedy{
		Finding:    f,
		Reversible: true,
		RiskLevel:  RiskMedium,
	}

	for _, cmd := range f.Commands {
		if s.isActionCommand(cmd) {
			suggestion.Actions = append(suggestion.Actions, SuggestedAction{
				Description: s.describeAction(cmd),
				Command:     s.addNamespace(cmd, f.Namespace),
			})
		}
	}

	if len(suggestion.Actions) == 0 {
		return nil, fmt.Errorf("no describable actions found for %s", f.CCVE)
	}

	return suggestion, nil
}

func (s *TriggerActionSuggester) isActionCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "rollout restart") ||
		strings.Contains(cmd, "rollout undo") ||
		strings.Contains(cmd, "kubectl scale") ||
		strings.Contains(cmd, "rollout status")
}

func (s *TriggerActionSuggester) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

func (s *TriggerActionSuggester) describeAction(cmd string) string {
	cmd = strings.ToLower(cmd)
	if strings.Contains(cmd, "rollout restart") {
		return "Restart pods with rolling update"
	}
	if strings.Contains(cmd, "rollout undo") {
		return "Rollback to previous version"
	}
	if strings.Contains(cmd, "scale") {
		return "Scale deployment replicas"
	}
	if strings.Contains(cmd, "rollout status") {
		return "Check rollout status"
	}
	return "Trigger action"
}
