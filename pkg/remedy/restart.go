package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RestartExecutor handles restart remedy type
// Similar to TriggerAction but specifically for restarts
type RestartExecutor struct {
	kubectl string
}

// NewRestartExecutor creates a new restart executor
func NewRestartExecutor() *RestartExecutor {
	return &RestartExecutor{
		kubectl: "kubectl",
	}
}

// Type returns Restart
func (e *RestartExecutor) Type() RemedyType {
	return Restart
}

// CanExecute checks if we can restart
func (e *RestartExecutor) CanExecute(f *Finding) bool {
	// Can execute if commands mention restart, or resource is a workload
	for _, cmd := range f.Commands {
		if e.isRestartCommand(cmd) {
			return true
		}
	}
	// Also can execute for Deployment/StatefulSet/DaemonSet if no specific command
	return isWorkload(f.Resource.Kind)
}

// DryRun shows what would be restarted
func (e *RestartExecutor) DryRun(ctx context.Context, f *Finding) (*RemedyPlan, error) {
	plan := &RemedyPlan{
		Finding:    f,
		Reversible: true, // Can rollback restart
		RiskLevel:  RiskMedium,
	}

	// Use explicit commands if available
	for _, cmd := range f.Commands {
		if e.isRestartCommand(cmd) {
			plan.Actions = append(plan.Actions, PlannedAction{
				Description: "Restart workload",
				Command:     e.addNamespace(cmd, f.Namespace),
			})
		}
	}

	// If no commands, generate rollout restart for workloads
	if len(plan.Actions) == 0 && isWorkload(f.Resource.Kind) {
		cmd := fmt.Sprintf("kubectl rollout restart %s/%s",
			strings.ToLower(f.Resource.Kind), f.Resource.Name)
		plan.Actions = append(plan.Actions, PlannedAction{
			Description: fmt.Sprintf("Restart %s", f.Resource.Kind),
			Command:     e.addNamespace(cmd, f.Namespace),
		})
	}

	if len(plan.Actions) == 0 {
		return nil, fmt.Errorf("no restart actions available for %s", f.CCVE)
	}

	return plan, nil
}

// Execute restarts the workload
func (e *RestartExecutor) Execute(ctx context.Context, f *Finding, opts *ExecuteOptions) (*RemedyResult, error) {
	result := &RemedyResult{Success: true}

	// Generate rollback command
	if isWorkload(f.Resource.Kind) {
		result.RollbackCmd = fmt.Sprintf("kubectl rollout undo %s/%s",
			strings.ToLower(f.Resource.Kind), f.Resource.Name)
		if f.Namespace != "" {
			result.RollbackCmd += " -n " + f.Namespace
		}
	}

	// Build list of commands to execute
	var commands []string
	for _, cmd := range f.Commands {
		if e.isRestartCommand(cmd) {
			commands = append(commands, cmd)
		}
	}

	// If no explicit commands, generate rollout restart
	if len(commands) == 0 && isWorkload(f.Resource.Kind) {
		commands = append(commands, fmt.Sprintf("kubectl rollout restart %s/%s",
			strings.ToLower(f.Resource.Kind), f.Resource.Name))
	}

	for _, cmd := range commands {
		execCmd := e.addNamespace(cmd, f.Namespace)

		// Dry-run just reports what would happen
		if opts.DryRun {
			result.Actions = append(result.Actions, ActionResult{
				Action: PlannedAction{
					Command:     execCmd,
					Description: "Restart workload",
				},
				Output:  "[dry-run: would restart]",
				Success: true,
			})
			continue
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
				Description: "Restart workload",
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
		result.Message = "Successfully restarted workload"
	} else {
		result.Message = "Restart failed"
	}

	return result, nil
}

// isRestartCommand checks if this is a restart-related command
func (e *RestartExecutor) isRestartCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "rollout restart") ||
		strings.Contains(cmd, "delete pod") || // Deleting pod triggers restart
		strings.Contains(cmd, "kill") ||
		strings.Contains(cmd, "restart")
}

// addNamespace adds -n flag if not present
func (e *RestartExecutor) addNamespace(cmd, namespace string) string {
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
