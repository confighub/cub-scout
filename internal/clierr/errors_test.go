// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package clierr

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsForbidden(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "K8s forbidden error",
			err:      apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "test", nil),
			expected: true,
		},
		{
			name:     "error with forbidden in message",
			err:      errors.New("forbidden: user cannot list pods"),
			expected: true,
		},
		{
			name:     "error with access denied",
			err:      errors.New("access denied to resource"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsForbidden(tt.err)
			if got != tt.expected {
				t.Errorf("IsForbidden() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "K8s not found error",
			err:      apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "test"),
			expected: true,
		},
		{
			name:     "CRD not installed error",
			err:      errors.New("no matches for kind \"HelmRelease\" in version \"helm.toolkit.fluxcd.io/v2\""),
			expected: true,
		},
		{
			name:     "server could not find error",
			err:      errors.New("the server could not find the requested resource"),
			expected: true,
		},
		{
			name:     "regular not found message",
			err:      errors.New("resource not found"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFound(tt.err)
			if got != tt.expected {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:6443: connection refused"),
			expected: true,
		},
		{
			name:     "no such host",
			err:      errors.New("dial tcp: lookup kubernetes.local: no such host"),
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("read tcp 192.168.1.1:443: i/o timeout"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNetworkError(tt.err)
			if got != tt.expected {
				t.Errorf("IsNetworkError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "forbidden error",
			err:      apierrors.NewForbidden(schema.GroupResource{}, "", nil),
			expected: TypeForbidden,
		},
		{
			name:     "not found error",
			err:      apierrors.NewNotFound(schema.GroupResource{}, ""),
			expected: TypeNotFound,
		},
		{
			name:     "network error",
			err:      errors.New("connection refused"),
			expected: TypeNetwork,
		},
		{
			name:     "internal error",
			err:      errors.New("unexpected error"),
			expected: TypeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.expected {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPretty(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantContain string
	}{
		{
			name:        "nil error",
			err:         nil,
			wantContain: "",
		},
		{
			name:        "forbidden error includes RBAC hint",
			err:         errors.New("forbidden: access denied"),
			wantContain: "RBAC",
		},
		{
			name:        "CRD not found includes install hint",
			err:         errors.New("no matches for kind \"HelmRelease\""),
			wantContain: "CRD not installed",
		},
		{
			name:        "network error includes connectivity hint",
			err:         errors.New("connection refused"),
			wantContain: "cluster connectivity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pretty(tt.err)
			if tt.wantContain != "" && !containsString(got, tt.wantContain) {
				t.Errorf("Pretty() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && contains(s, substr)))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNothingFound(t *testing.T) {
	result := NothingFound("deployments")
	if !containsString(result, "deployments") {
		t.Errorf("NothingFound() should contain resource name")
	}
	if !containsString(result, "No ") {
		t.Errorf("NothingFound() should start with 'No '")
	}
}
