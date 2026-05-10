package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ConfigFixSuggester describes a config_fix remedy as a structured patch
// suggestion. It does not apply the patch.
type ConfigFixSuggester struct {
	kubectl string
}

// NewConfigFixSuggester creates a new config-fix suggester.
func NewConfigFixSuggester() *ConfigFixSuggester {
	return &ConfigFixSuggester{
		kubectl: "kubectl",
	}
}

// Type returns ConfigFix.
func (s *ConfigFixSuggester) Type() RemedyType {
	return ConfigFix
}

// CanSuggest reports whether this suggester can describe the finding.
func (s *ConfigFixSuggester) CanSuggest(f *Finding) bool {
	for _, cmd := range f.Commands {
		if s.isConfigCommand(cmd) {
			return true
		}
	}
	return false
}

// Suggest describes the fix that would resolve the finding without applying
// it. Reading current cluster state via `kubectl get` is permitted because
// it is read-only; no kubectl apply / patch is invoked.
func (s *ConfigFixSuggester) Suggest(ctx context.Context, f *Finding) (*SuggestedRemedy, error) {
	suggestion := &SuggestedRemedy{
		Finding:    f,
		Reversible: true,
		RiskLevel:  RiskLow,
	}

	// Capture current state for the diff (read-only).
	current, err := s.getCurrentState(ctx, f.Resource)
	if err != nil {
		current = "[resource not found]"
	}

	for _, cmd := range f.Commands {
		if s.isConfigCommand(cmd) {
			suggestion.Actions = append(suggestion.Actions, SuggestedAction{
				Description: s.describeCommand(cmd),
				Command:     s.addNamespace(cmd, f.Namespace),
				DiffBefore:  current,
				DiffAfter:   "[computed at apply time by the executing tool]",
			})
		}
	}

	if len(suggestion.Actions) == 0 {
		return nil, fmt.Errorf("no describable commands found for %s", f.CCVE)
	}

	return suggestion, nil
}

func (s *ConfigFixSuggester) isConfigCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "kubectl apply") ||
		strings.Contains(cmd, "kubectl patch") ||
		strings.Contains(cmd, "kubectl annotate") ||
		strings.Contains(cmd, "kubectl label")
}

func (s *ConfigFixSuggester) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

func (s *ConfigFixSuggester) describeCommand(cmd string) string {
	cmd = strings.ToLower(cmd)
	if strings.Contains(cmd, "kubectl apply") {
		return "Apply configuration change"
	}
	if strings.Contains(cmd, "kubectl patch") {
		return "Patch resource configuration"
	}
	if strings.Contains(cmd, "kubectl annotate") {
		return "Update resource annotations"
	}
	if strings.Contains(cmd, "kubectl label") {
		return "Update resource labels"
	}
	return "Execute kubectl command"
}

// getCurrentState reads the resource via `kubectl get`. This is read-only
// and is the only shell-out the suggester performs.
func (s *ConfigFixSuggester) getCurrentState(ctx context.Context, ref ResourceRef) (string, error) {
	cmd := fmt.Sprintf("%s get %s %s -o yaml",
		s.kubectl, strings.ToLower(ref.Kind), ref.Name)
	if ref.Namespace != "" {
		cmd += " -n " + ref.Namespace
	}

	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("get resource: %w", err)
	}
	return string(out), nil
}
