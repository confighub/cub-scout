// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestParsePresentationMode(t *testing.T) {
	tests := []struct {
		input    string
		expected PresentationMode
		wantErr  bool
	}{
		{"human", PresentationHuman, false},
		{"HUMAN", PresentationHuman, false},
		{"Human", PresentationHuman, false},
		{"ai", PresentationAI, false},
		{"AI", PresentationAI, false},
		{"paired", PresentationPaired, false},
		{"PAIRED", PresentationPaired, false},
		{"", PresentationHuman, false}, // empty defaults to human
		{"  human  ", PresentationHuman, false},
		{"invalid", "", true},
		{"machine", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePresentationMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePresentationMode(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParsePresentationMode(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.expected {
				t.Errorf("ParsePresentationMode(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPresentationModeString(t *testing.T) {
	tests := []struct {
		mode     PresentationMode
		expected string
	}{
		{PresentationHuman, "human"},
		{PresentationAI, "ai"},
		{PresentationPaired, "paired"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("PresentationMode.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDoctorHeading(t *testing.T) {
	// All modes should produce a non-empty heading
	for _, mode := range ValidPresentationModes {
		heading := DoctorHeading(mode)
		if heading == "" {
			t.Errorf("DoctorHeading(%q) returned empty string", mode)
		}
	}

	// AI mode should be uppercase
	aiHeading := DoctorHeading(PresentationAI)
	if aiHeading != "CLUSTER HEALTH SUMMARY" {
		t.Errorf("DoctorHeading(ai) = %q, want uppercase heading", aiHeading)
	}
}

func TestDoctorIntro(t *testing.T) {
	cluster := "kind-dev"
	namespace := "prod"

	// All modes should include cluster and namespace
	for _, mode := range ValidPresentationModes {
		intro := DoctorIntro(mode, cluster, namespace)
		if intro == "" {
			t.Errorf("DoctorIntro(%q) returned empty string", mode)
		}
	}

	// AI mode should use bracket format
	aiIntro := DoctorIntro(PresentationAI, cluster, namespace)
	if aiIntro[0] != '[' {
		t.Errorf("DoctorIntro(ai) should use bracket format, got %q", aiIntro)
	}
}

func TestExplainHeading(t *testing.T) {
	resource := "Deployment/api"
	namespace := "prod"

	// All modes should produce a non-empty heading
	for _, mode := range ValidPresentationModes {
		heading := ExplainHeading(mode, resource, namespace)
		if heading == "" {
			t.Errorf("ExplainHeading(%q) returned empty string", mode)
		}
	}

	// AI mode should use bracket format
	aiHeading := ExplainHeading(PresentationAI, resource, namespace)
	if aiHeading[0] != '[' {
		t.Errorf("ExplainHeading(ai) should use bracket format, got %q", aiHeading)
	}
}

func TestSectionLabel(t *testing.T) {
	label := "Ownership"

	// Human and paired should preserve case
	humanLabel := SectionLabel(PresentationHuman, label)
	if humanLabel != "Ownership:" {
		t.Errorf("SectionLabel(human, %q) = %q, want %q", label, humanLabel, "Ownership:")
	}

	// AI should uppercase
	aiLabel := SectionLabel(PresentationAI, label)
	if aiLabel != "OWNERSHIP:" {
		t.Errorf("SectionLabel(ai, %q) = %q, want %q", label, aiLabel, "OWNERSHIP:")
	}
}

func TestTryNextHeading(t *testing.T) {
	// AI mode should use "RECOMMENDED ACTIONS:"
	aiHeading := TryNextHeading(PresentationAI)
	if aiHeading != "RECOMMENDED ACTIONS:" {
		t.Errorf("TryNextHeading(ai) = %q, want %q", aiHeading, "RECOMMENDED ACTIONS:")
	}

	// Human and paired should use "TRY NEXT:"
	humanHeading := TryNextHeading(PresentationHuman)
	if humanHeading != "TRY NEXT:" {
		t.Errorf("TryNextHeading(human) = %q, want %q", humanHeading, "TRY NEXT:")
	}

	pairedHeading := TryNextHeading(PresentationPaired)
	if pairedHeading != "TRY NEXT:" {
		t.Errorf("TryNextHeading(paired) = %q, want %q", pairedHeading, "TRY NEXT:")
	}
}

func TestDefaultPresentationMode(t *testing.T) {
	if DefaultPresentationMode != PresentationHuman {
		t.Errorf("DefaultPresentationMode = %q, want %q", DefaultPresentationMode, PresentationHuman)
	}
}
