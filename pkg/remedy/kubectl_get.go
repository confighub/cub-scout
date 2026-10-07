// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package remedy

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

var kindPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.]*$`)

// ValidateResourceRef rejects a kind, name or namespace that is not a valid
// Kubernetes identifier. These values come from command-line flags and from
// files, so they are checked before they go anywhere near a subprocess.
func ValidateResourceRef(ref ResourceRef) error {
	if ref.Kind != "" && !kindPattern.MatchString(ref.Kind) {
		return fmt.Errorf("invalid resource kind %q", ref.Kind)
	}
	if ref.Name != "" {
		if problems := validation.IsDNS1123Subdomain(ref.Name); len(problems) > 0 {
			return fmt.Errorf("invalid resource name %q: %s", ref.Name, strings.Join(problems, "; "))
		}
	}
	return ValidateNamespace(ref.Namespace)
}

// ValidateNamespace rejects a namespace that is not a valid Kubernetes
// namespace name. An empty namespace is allowed.
func ValidateNamespace(namespace string) error {
	if namespace == "" {
		return nil
	}
	if problems := validation.IsDNS1123Label(namespace); len(problems) > 0 {
		return fmt.Errorf("invalid namespace %q: %s", namespace, strings.Join(problems, "; "))
	}
	return nil
}

// KubectlGet runs a read-only `kubectl get` for ref and returns its output.
//
// The values are validated, then passed as an argument vector: there is no
// shell, so nothing in them can be interpreted as a command. They follow "--"
// so that none can be read as a flag either.
func KubectlGet(ctx context.Context, kubectl, output string, ref ResourceRef) (string, error) {
	if ref.Kind == "" {
		return "", fmt.Errorf("resource kind is required")
	}
	if err := ValidateResourceRef(ref); err != nil {
		return "", err
	}
	args := []string{"get", "-o", output}
	if ref.Namespace != "" {
		args = append(args, "-n", ref.Namespace)
	}
	args = append(args, "--", strings.ToLower(ref.Kind))
	if ref.Name != "" {
		args = append(args, ref.Name)
	}
	out, err := exec.CommandContext(ctx, kubectl, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
