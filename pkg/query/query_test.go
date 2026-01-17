// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package query

import (
	"testing"
)

// mockEntry implements Matchable for testing
type mockEntry struct {
	data   map[string]string
	labels map[string]string
}

func (m mockEntry) GetField(field string) (string, bool) {
	// Handle labels[key] syntax
	if len(field) > 7 && field[:7] == "labels[" && field[len(field)-1] == ']' {
		key := field[7 : len(field)-1]
		v, ok := m.labels[key]
		return v, ok
	}
	v, ok := m.data[field]
	return v, ok
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantLen int // number of conditions
	}{
		{
			name:    "empty query",
			input:   "",
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "simple equal",
			input:   "kind=Deployment",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "two conditions with AND",
			input:   "kind=Deployment AND namespace=production",
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "two conditions with OR",
			input:   "owner=Flux OR owner=Argo",
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "not equal",
			input:   "owner!=Native",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "regex",
			input:   "name~=payment.*",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "IN list",
			input:   "kind=Deployment,StatefulSet,DaemonSet",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "labels",
			input:   "labels[app]=nginx",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "complex query",
			input:   "kind=Deployment AND namespace=production AND owner!=Native",
			wantErr: false,
			wantLen: 3,
		},
		{
			name:    "invalid regex",
			input:   "name~=[invalid",
			wantErr: true,
		},
		{
			name:    "invalid syntax",
			input:   "kind",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
				return
			}
			if len(q.Conditions) != tt.wantLen {
				t.Errorf("Parse(%q) got %d conditions, want %d", tt.input, len(q.Conditions), tt.wantLen)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	deployment := mockEntry{
		data: map[string]string{
			"kind":      "Deployment",
			"namespace": "production",
			"name":      "payment-api",
			"owner":     "Flux",
			"cluster":   "prod-east",
		},
		labels: map[string]string{
			"app":     "payment",
			"version": "v1",
		},
	}

	service := mockEntry{
		data: map[string]string{
			"kind":      "Service",
			"namespace": "production",
			"name":      "payment-api",
			"owner":     "Native",
			"cluster":   "prod-east",
		},
		labels: map[string]string{
			"app": "payment",
		},
	}

	tests := []struct {
		name    string
		query   string
		entry   mockEntry
		matches bool
	}{
		{
			name:    "empty query matches all",
			query:   "",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "exact match kind",
			query:   "kind=Deployment",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "exact match fails",
			query:   "kind=Service",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "case insensitive match",
			query:   "kind=deployment",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "AND both true",
			query:   "kind=Deployment AND owner=Flux",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "AND one false",
			query:   "kind=Deployment AND owner=Argo",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "OR first true",
			query:   "owner=Flux OR owner=Argo",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "OR second true",
			query:   "owner=Argo OR owner=Flux",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "OR both false",
			query:   "owner=Argo OR owner=Helm",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "not equal matches",
			query:   "owner!=Native",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "not equal fails",
			query:   "owner!=Flux",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "regex matches",
			query:   "name~=payment.*",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "regex fails",
			query:   "name~=order.*",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "IN list matches",
			query:   "kind=Deployment,StatefulSet,DaemonSet",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "IN list fails",
			query:   "kind=Service,ConfigMap",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "wildcard matches",
			query:   "namespace=prod*",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "wildcard fails",
			query:   "namespace=staging*",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "label matches",
			query:   "labels[app]=payment",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "label fails",
			query:   "labels[app]=order",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "label missing",
			query:   "labels[missing]=value",
			entry:   deployment,
			matches: false,
		},
		{
			name:    "complex query matches",
			query:   "kind=Deployment AND namespace=production AND owner!=Native",
			entry:   deployment,
			matches: true,
		},
		{
			name:    "complex query fails",
			query:   "kind=Deployment AND namespace=production AND owner!=Native",
			entry:   service,
			matches: false,
		},
		{
			name:    "service native owner",
			query:   "owner=Native",
			entry:   service,
			matches: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			got := q.Matches(tt.entry)
			if got != tt.matches {
				t.Errorf("Query(%q).Matches() = %v, want %v", tt.query, got, tt.matches)
			}
		})
	}
}

func TestQueryString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"kind=Deployment", "kind=Deployment"},
		{"kind=Deployment AND namespace=prod", "kind=Deployment AND namespace=prod"},
		{"owner!=Native", "owner!=Native"},
		{"name~=payment.*", "name~=payment.*"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			got := q.String()
			if got != tt.want {
				t.Errorf("Query.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
