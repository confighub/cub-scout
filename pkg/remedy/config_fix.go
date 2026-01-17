package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ConfigFixExecutor handles config_fix remedy type
// Executes kubectl apply/patch commands
type ConfigFixExecutor struct {
	kubectl string
}

// NewConfigFixExecutor creates a new config fix executor
func NewConfigFixExecutor() *ConfigFixExecutor {
	return &ConfigFixExecutor{
		kubectl: "kubectl",
	}
}

// Type returns ConfigFix
func (e *ConfigFixExecutor) Type() RemedyType {
	return ConfigFix
}

// CanExecute checks if we can fix this finding
func (e *ConfigFixExecutor) CanExecute(f *Finding) bool {
	// Can execute if we have kubectl commands
	for _, cmd := range f.Commands {
		if e.isConfigCommand(cmd) {
			return true
		}
	}
	return false
}

// DryRun shows what would be changed
func (e *ConfigFixExecutor) DryRun(ctx context.Context, f *Finding) (*RemedyPlan, error) {
	plan := &RemedyPlan{
		Finding:    f,
		Reversible: true,
		RiskLevel:  RiskLow,
	}

	// Get current state
	current, err := e.getCurrentState(ctx, f.Resource)
	if err != nil {
		// Resource might not exist yet, that's ok for apply
		current = "[resource not found]"
	}

	// Build planned actions
	for _, cmd := range f.Commands {
		if e.isConfigCommand(cmd) {
			// Add namespace if not present
			execCmd := e.addNamespace(cmd, f.Namespace)

			plan.Actions = append(plan.Actions, PlannedAction{
				Description: e.describeCommand(cmd),
				Command:     execCmd,
				DiffBefore:  current,
				DiffAfter:   "[computed at execution time]",
			})
		}
	}

	if len(plan.Actions) == 0 {
		return nil, fmt.Errorf("no executable commands found for %s", f.CCVE)
	}

	return plan, nil
}

// Execute applies the remedy
func (e *ConfigFixExecutor) Execute(ctx context.Context, f *Finding, opts *ExecuteOptions) (*RemedyResult, error) {
	result := &RemedyResult{
		Success: true,
	}

	// Capture current state for rollback
	if opts.Rollback {
		current, err := e.getCurrentState(ctx, f.Resource)
		if err == nil && current != "" {
			result.RollbackCmd = fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", current)
		}
	}

	for _, cmd := range f.Commands {
		if !e.isConfigCommand(cmd) {
			continue
		}

		// Build execution command
		execCmd := e.addNamespace(cmd, f.Namespace)

		// Add dry-run flag if requested
		if opts.DryRun {
			if strings.Contains(execCmd, "apply") {
				execCmd = execCmd + " --dry-run=client"
			} else if strings.Contains(execCmd, "patch") {
				execCmd = execCmd + " --dry-run=client"
			}
		}

		// Set timeout context
		execCtx := ctx
		if opts.Timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}

		// Execute
		out, err := exec.CommandContext(execCtx, "sh", "-c", execCmd).CombinedOutput()

		actionResult := ActionResult{
			Action: PlannedAction{
				Command:     cmd,
				Description: e.describeCommand(cmd),
			},
			Output:  string(out),
			Success: err == nil,
		}

		if err != nil {
			actionResult.Error = err.Error()
			result.Success = false
		}

		result.Actions = append(result.Actions, actionResult)

		// Stop on first failure unless Force is set
		if !result.Success && !opts.Force {
			break
		}
	}

	if result.Success {
		result.Message = fmt.Sprintf("Successfully applied %d config fixes", len(result.Actions))
	} else {
		result.Message = "Config fix failed"
	}

	return result, nil
}

// isConfigCommand checks if this is a kubectl apply/patch/annotate/label command
func (e *ConfigFixExecutor) isConfigCommand(cmd string) bool {
	cmd = strings.ToLower(cmd)
	return strings.Contains(cmd, "kubectl apply") ||
		strings.Contains(cmd, "kubectl patch") ||
		strings.Contains(cmd, "kubectl annotate") ||
		strings.Contains(cmd, "kubectl label")
}

// addNamespace adds -n flag if namespace is set and not already present
func (e *ConfigFixExecutor) addNamespace(cmd, namespace string) string {
	if namespace != "" && !strings.Contains(cmd, " -n ") && !strings.Contains(cmd, " --namespace") {
		return cmd + " -n " + namespace
	}
	return cmd
}

// describeCommand returns a human-readable description
func (e *ConfigFixExecutor) describeCommand(cmd string) string {
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

// getCurrentState gets the current YAML of a resource
func (e *ConfigFixExecutor) getCurrentState(ctx context.Context, ref ResourceRef) (string, error) {
	cmd := fmt.Sprintf("kubectl get %s %s -o yaml",
		strings.ToLower(ref.Kind), ref.Name)
	if ref.Namespace != "" {
		cmd += " -n " + ref.Namespace
	}

	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("get resource: %w", err)
	}
	return string(out), nil
}
