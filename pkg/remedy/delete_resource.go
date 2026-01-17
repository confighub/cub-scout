package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DeleteResourceExecutor handles delete_resource remedy type
// Executes kubectl delete commands with safety checks
type DeleteResourceExecutor struct {
	kubectl string
}

// NewDeleteResourceExecutor creates a new delete resource executor
func NewDeleteResourceExecutor() *DeleteResourceExecutor {
	return &DeleteResourceExecutor{
		kubectl: "kubectl",
	}
}

// Type returns DeleteResource
func (e *DeleteResourceExecutor) Type() RemedyType {
	return DeleteResource
}

// CanExecute checks if we can delete
func (e *DeleteResourceExecutor) CanExecute(f *Finding) bool {
	for _, cmd := range f.Commands {
		if strings.Contains(strings.ToLower(cmd), "kubectl delete") {
			return true
		}
	}
	return false
}

// DryRun shows what would be deleted
func (e *DeleteResourceExecutor) DryRun(ctx context.Context, f *Finding) (*RemedyPlan, error) {
	plan := &RemedyPlan{
		Finding:    f,
		Reversible: false, // Deletes are NOT reversible!
		RiskLevel:  RiskHigh,
	}

	for _, cmd := range f.Commands {
		if strings.Contains(strings.ToLower(cmd), "kubectl delete") {
			// Get resource YAML before deletion (for backup)
			backup, _ := e.getResourceYAML(ctx, f.Resource)

			plan.Actions = append(plan.Actions, PlannedAction{
				Description: "DELETE resource (irreversible!)",
				Command:     e.addNamespace(cmd, f.Namespace),
				DiffBefore:  backup,
				DiffAfter:   "[resource will be deleted]",
			})
		}
	}

	if len(plan.Actions) == 0 {
		return nil, fmt.Errorf("no delete commands found for %s", f.CCVE)
	}

	return plan, nil
}

// Execute deletes the resources
func (e *DeleteResourceExecutor) Execute(ctx context.Context, f *Finding, opts *ExecuteOptions) (*RemedyResult, error) {
	result := &RemedyResult{Success: true}

	// Backup resource first (for potential manual restore)
	backup, err := e.getResourceYAML(ctx, f.Resource)
	if err == nil && backup != "" {
		result.RollbackCmd = fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", backup)
	}

	for _, cmd := range f.Commands {
		if !strings.Contains(strings.ToLower(cmd), "kubectl delete") {
			continue
		}

		execCmd := e.addNamespace(cmd, f.Namespace)

		// Add --dry-run for dry-run mode
		if opts.DryRun {
			execCmd = execCmd + " --dry-run=client"
		}

		// Always add --wait=false to avoid hanging on finalizers
		if !strings.Contains(execCmd, "--wait") {
			execCmd = execCmd + " --wait=false"
		}

		// Set timeout
		execCtx := ctx
		if opts.Timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}

		out, err := exec.CommandContext(execCtx, "sh", "-c", execCmd).CombinedOutput()

		actionResult := ActionResult{
			Action: PlannedAction{
				Command:     cmd,
				Description: "Delete resource",
			},
			Output:  string(out),
			Success: err == nil,
		}

		if err != nil {
			actionResult.Error = err.Error()
			result.Success = false
		}

		result.Actions = append(result.Actions, actionResult)

		if !result.Success && !opts.Force {
			break
		}
	}

	if result.Success {
		result.Message = fmt.Sprintf("Successfully deleted %d resources", len(result.Actions))
	} else {
		result.Message = "Delete failed"
	}

	return result, nil
}

// addNamespace adds -n flag if not present
func (e *DeleteResourceExecutor) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

// getResourceYAML gets the current YAML for backup
func (e *DeleteResourceExecutor) getResourceYAML(ctx context.Context, ref ResourceRef) (string, error) {
	cmd := fmt.Sprintf("kubectl get %s %s -o yaml",
		strings.ToLower(ref.Kind), ref.Name)
	if ref.Namespace != "" {
		cmd += " -n " + ref.Namespace
	}

	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
