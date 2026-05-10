package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DeleteResourceSuggester describes a delete_resource remedy as a structured
// suggestion. It does not delete anything.
type DeleteResourceSuggester struct {
	kubectl string
}

// NewDeleteResourceSuggester creates a new delete-resource suggester.
func NewDeleteResourceSuggester() *DeleteResourceSuggester {
	return &DeleteResourceSuggester{
		kubectl: "kubectl",
	}
}

// Type returns DeleteResource.
func (s *DeleteResourceSuggester) Type() RemedyType {
	return DeleteResource
}

// CanSuggest reports whether this suggester can describe the finding.
func (s *DeleteResourceSuggester) CanSuggest(f *Finding) bool {
	for _, cmd := range f.Commands {
		if strings.Contains(strings.ToLower(cmd), "kubectl delete") {
			return true
		}
	}
	return false
}

// Suggest describes the deletion that would resolve the finding without
// performing it. Reading the current YAML via `kubectl get` is permitted
// because it is read-only.
func (s *DeleteResourceSuggester) Suggest(ctx context.Context, f *Finding) (*SuggestedRemedy, error) {
	suggestion := &SuggestedRemedy{
		Finding:    f,
		Reversible: false, // Deletes are NOT reversible if applied.
		RiskLevel:  RiskHigh,
	}

	for _, cmd := range f.Commands {
		if strings.Contains(strings.ToLower(cmd), "kubectl delete") {
			// Capture pre-delete YAML so a downstream applier can build a
			// rollback command if it chooses to apply.
			backup, _ := s.getResourceYAML(ctx, f.Resource)

			suggestion.Actions = append(suggestion.Actions, SuggestedAction{
				Description: "DELETE resource (irreversible if applied)",
				Command:     s.addNamespace(cmd, f.Namespace),
				DiffBefore:  backup,
				DiffAfter:   "[resource would be deleted]",
			})
		}
	}

	if len(suggestion.Actions) == 0 {
		return nil, fmt.Errorf("no delete commands found for %s", f.CCVE)
	}

	return suggestion, nil
}

func (s *DeleteResourceSuggester) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

// getResourceYAML reads the resource via `kubectl get`. Read-only.
func (s *DeleteResourceSuggester) getResourceYAML(ctx context.Context, ref ResourceRef) (string, error) {
	cmd := fmt.Sprintf("%s get %s %s -o yaml",
		s.kubectl, strings.ToLower(ref.Kind), ref.Name)
	if ref.Namespace != "" {
		cmd += " -n " + ref.Namespace
	}

	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
