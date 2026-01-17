package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TriggerActionExecutor handles trigger_action remedy type
// Executes kubectl rollout restart, scale, etc.
type TriggerActionExecutor struct {
	kubectl string
}

// NewTriggerActionExecutor creates a new trigger action executor
func NewTriggerActionExecutor() *TriggerActionExecutor {
	return &TriggerActionExecutor{
		kubectl: "kubectl",
	}
}

// Type returns TriggerAction
func (e *TriggerActionExecutor) Type() RemedyType {
	return TriggerAction
}

// CanExecute checks if we can execute this finding
func (e *TriggerActionExecutor) CanExecute(f *Finding) bool {
	for _, cmd := range f.Commands {
		if e.isActionCommand(cmd) {
			return true
		}
	}
	return false
}

// DryRun shows what would be done
func (e *TriggerActionExecutor) DryRun(ctx context.Context, f *Finding) (*RemedyPlan, error) {
	plan := &RemedyPlan{
		Finding:    f,
		Reversible: true,
		RiskLevel:  RiskMedium,
	}

	for _, cmd := range f.Commands {
		if e.isActionCommand(cmd) {
			execCmd := e.addNamespace(cmd, f.Namespace)
			plan.Actions = append(plan.Actions, PlannedAction{
				Description: e.describeAction(cmd),
				Command:     execCmd,
			})
		}
	}

	if len(plan.Actions) == 0 {
		return nil, fmt.Errorf("no executable actions found for %s", f.CCVE)
	}

	return plan, nil
}

// Execute runs the action commands
func (e *TriggerActionExecutor) Execute(ctx context.Context, f *Finding, opts *ExecuteOptions) (*RemedyResult, error) {
	result := &RemedyResult{Success: true}

	for _, cmd := range f.Commands {
		if !e.isActionCommand(cmd) {
			continue
		}

		execCmd := e.addNamespace(cmd, f.Namespace)

		// Note: rollout restart doesn't support --dry-run
		// We just skip execution in dry-run mode
		if opts.DryRun {
			result.Actions = append(result.Actions, ActionResult{
				Action: PlannedAction{
					Command:     execCmd,
					Description: e.describeAction(cmd),
				},
				Output:  "[dry-run: would execute]",
				Success: true,
			})
			continue
		}

		// Set timeout context
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
				Description: e.describeAction(cmd),
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

	// Generate rollback command if applicable
	if len(f.Commands) > 0 && strings.Contains(f.Commands[0], "rollout restart") {
		result.RollbackCmd = fmt.Sprintf("kubectl rollout undo %s/%s",
			strings.ToLower(f.Resource.Kind), f.Resource.Name)
		if f.Namespace != "" {
			result.RollbackCmd += " -n " + f.Namespace
		}
	}

	if result.Success {
		result.Message = fmt.Sprintf("Successfully triggered %d actions", len(result.Actions))
	} else {
		result.Message = "Action trigger failed"
	}

	return result, nil
}

// isActionCommand checks if this is a rollout/scale command
func (e *TriggerActionExecutor) isActionCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "rollout restart") ||
		strings.Contains(cmd, "rollout undo") ||
		strings.Contains(cmd, "kubectl scale") ||
		strings.Contains(cmd, "rollout status")
}

// addNamespace adds -n flag if not present
func (e *TriggerActionExecutor) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

// describeAction returns human-readable description
func (e *TriggerActionExecutor) describeAction(cmd string) string {
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
