// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package clierr provides error classification and user-friendly error formatting for the CLI.
// It helps distinguish between different error types and provides actionable hints.
package clierr

import (
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Common error types for CLI output.
const (
	TypeNotFound   = "not_found"  // Resource or CRD not found
	TypeForbidden  = "forbidden"  // RBAC access denied
	TypeNetwork    = "network"    // Connection/network errors
	TypeInternal   = "internal"   // Internal/unexpected errors
	TypeValidation = "validation" // Input validation errors
)

// IsForbidden checks if the error is an access denied (RBAC) error.
func IsForbidden(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsForbidden(err) {
		return true
	}
	// Also check for common forbidden error patterns in messages
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "access denied") ||
		strings.Contains(msg, "unauthorized")
}

// IsNotFound checks if the error indicates a missing resource or CRD.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	// Check for CRD not found patterns
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "the server could not find")
}

// IsNetworkError checks if the error is a connection/network error.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded")
}

// ClassifyError determines the type of error for appropriate handling.
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}
	if IsForbidden(err) {
		return TypeForbidden
	}
	if IsNotFound(err) {
		return TypeNotFound
	}
	if IsNetworkError(err) {
		return TypeNetwork
	}
	return TypeInternal
}

// Pretty formats an error with a user-friendly message and actionable hints.
func Pretty(err error) string {
	if err == nil {
		return ""
	}

	errType := ClassifyError(err)
	baseMsg := err.Error()

	switch errType {
	case TypeForbidden:
		return fmt.Sprintf("Access denied: %s\n\nHint: Check your RBAC permissions. You may need:\n"+
			"  - ClusterRole with get/list permissions for the resources you're accessing\n"+
			"  - kubectl auth can-i list <resource> to verify permissions", baseMsg)

	case TypeNotFound:
		if strings.Contains(strings.ToLower(baseMsg), "no matches for kind") ||
			strings.Contains(strings.ToLower(baseMsg), "the server could not find") {
			return fmt.Sprintf("CRD not installed: %s\n\nHint: The Custom Resource Definition may not be installed.\n"+
				"  - For Flux resources: flux install\n"+
				"  - For ArgoCD resources: kubectl apply -k github.com/argoproj/argo-cd/manifests/crds", baseMsg)
		}
		return fmt.Sprintf("Not found: %s", baseMsg)

	case TypeNetwork:
		return fmt.Sprintf("Connection error: %s\n\nHint: Check your cluster connectivity:\n"+
			"  - kubectl cluster-info to verify connection\n"+
			"  - Ensure your kubeconfig is correct", baseMsg)

	default:
		return fmt.Sprintf("Error: %s", baseMsg)
	}
}

// WrapWithHint wraps an error with an additional hint message.
func WrapWithHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w\n\nHint: %s", err, hint)
}

// NothingFound returns a user-friendly message when a query returns no results.
// This is different from an error - it's a valid "empty" result.
func NothingFound(resource string) string {
	return fmt.Sprintf("No %s found matching your criteria.\n\n"+
		"This might mean:\n"+
		"  - No resources of this type exist in the cluster\n"+
		"  - Your filter/query is too restrictive\n"+
		"  - You may not have permission to list these resources", resource)
}

// Unwrap returns the underlying error, stripping any wrapper.
func Unwrap(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}
