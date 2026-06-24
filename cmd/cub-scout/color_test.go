// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"
)

func TestColorEnabled(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	// Test with NO_COLOR unset - should enable color
	os.Unsetenv("NO_COLOR")
	if !colorEnabled() {
		t.Error("colorEnabled() should return true when NO_COLOR is unset")
	}

	// Test with NO_COLOR set to empty string - should disable color
	os.Setenv("NO_COLOR", "")
	if colorEnabled() {
		t.Error("colorEnabled() should return false when NO_COLOR is set (even empty)")
	}

	// Test with NO_COLOR set to "1" - should disable color
	os.Setenv("NO_COLOR", "1")
	if colorEnabled() {
		t.Error("colorEnabled() should return false when NO_COLOR is set to '1'")
	}
}

func TestColorFunctionsWithColorEnabled(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")

	tests := []struct {
		name     string
		fn       func(string) string
		input    string
		contains string // expected ANSI code
	}{
		{"Red", Red, "test", "\033[31m"},
		{"Green", Green, "test", "\033[32m"},
		{"Yellow", Yellow, "test", "\033[33m"},
		{"Blue", Blue, "test", "\033[34m"},
		{"Cyan", Cyan, "test", "\033[36m"},
		{"Bold", Bold, "test", "\033[1m"},
		{"Dim", Dim, "test", "\033[2m"},
		{"BoldRed", BoldRed, "test", "\033[1m\033[31m"},
		{"BoldGreen", BoldGreen, "test", "\033[1m\033[32m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result == tt.input {
				t.Errorf("%s() returned uncolored text when color is enabled", tt.name)
			}
			if len(result) <= len(tt.input) {
				t.Errorf("%s() result too short - expected ANSI codes", tt.name)
			}
		})
	}
}

func TestColorFunctionsWithColorDisabled(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Setenv("NO_COLOR", "1")

	tests := []struct {
		name  string
		fn    func(string) string
		input string
	}{
		{"Red", Red, "test"},
		{"Green", Green, "test"},
		{"Yellow", Yellow, "test"},
		{"Blue", Blue, "test"},
		{"Cyan", Cyan, "test"},
		{"Bold", Bold, "test"},
		{"Dim", Dim, "test"},
		{"BoldRed", BoldRed, "test"},
		{"BoldGreen", BoldGreen, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result != tt.input {
				t.Errorf("%s() should return plain text when NO_COLOR is set, got %q", tt.name, result)
			}
		})
	}
}

func TestStatusColor(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")

	tests := []struct {
		input         string
		expectedColor string // which color function should be applied
	}{
		{"Healthy", "green"},
		{"healthy", "green"},
		{"Ready", "green"},
		{"Running", "green"},
		{"Warning", "yellow"},
		{"Degraded", "yellow"},
		{"Error", "red"},
		{"Failed", "red"},
		{"Unhealthy", "red"},
		{"Unknown", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := StatusColor(tt.input)
			switch tt.expectedColor {
			case "green":
				if result == tt.input {
					t.Errorf("StatusColor(%q) should apply green color", tt.input)
				}
			case "yellow":
				if result == tt.input {
					t.Errorf("StatusColor(%q) should apply yellow color", tt.input)
				}
			case "red":
				if result == tt.input {
					t.Errorf("StatusColor(%q) should apply red color", tt.input)
				}
			case "none":
				if result != tt.input {
					t.Errorf("StatusColor(%q) should not apply color, got %q", tt.input, result)
				}
			}
		})
	}
}

func TestSeverityColor(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")

	tests := []struct {
		input     string
		isColored bool
	}{
		{"CRITICAL", true},
		{"HIGH", true},
		{"WARNING", true},
		{"MEDIUM", true},
		{"INFO", true},
		{"LOW", true},
		{"Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SeverityColor(tt.input)
			if tt.isColored && result == tt.input {
				t.Errorf("SeverityColor(%q) should apply color", tt.input)
			}
			if !tt.isColored && result != tt.input {
				t.Errorf("SeverityColor(%q) should not apply color", tt.input)
			}
		})
	}
}

func TestOwnerColor(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")

	owners := []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native", "Crossplane", "Terraform", "kro", "Sveltos", "Modelplane"}

	for _, owner := range owners {
		t.Run(owner, func(t *testing.T) {
			result := OwnerColor(owner)
			if result == owner {
				t.Errorf("OwnerColor(%q) should apply color", owner)
			}
		})
	}

	// Test unknown owner - should return unchanged
	t.Run("Unknown", func(t *testing.T) {
		result := OwnerColor("SomeUnknownOwner")
		if result != "SomeUnknownOwner" {
			t.Errorf("OwnerColor for unknown owner should not apply color")
		}
	})
}

func TestHealthIndicator(t *testing.T) {
	// Save original value
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")

	tests := []struct {
		input    string
		contains string
	}{
		{"Healthy", "✓"},
		{"Warning", "⚠"},
		{"Error", "✗"},
		{"Unknown", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := HealthIndicator(tt.input)
			// Result should contain the expected symbol
			found := false
			for _, r := range result {
				for _, c := range tt.contains {
					if r == c {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("HealthIndicator(%q) should contain %q, got %q", tt.input, tt.contains, result)
			}
		})
	}
}

func TestNoColorGoldenFileCompatibility(t *testing.T) {
	// This test ensures that when NO_COLOR is set, output matches golden files
	// (which are plain ASCII without ANSI codes)

	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		if hadNoColor {
			os.Setenv("NO_COLOR", origNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Setenv("NO_COLOR", "1")

	// All color functions should return input unchanged
	input := "test string"
	funcs := []func(string) string{
		Red, Green, Yellow, Blue, Cyan, Purple, White,
		Bold, Dim, BoldRed, BoldGreen, BoldYellow, BoldBlue, BoldCyan,
		StatusColor, SeverityColor, OwnerColor, KindColor, SectionHeader,
	}

	for i, fn := range funcs {
		result := fn(input)
		if result != input {
			t.Errorf("Color function %d changed input when NO_COLOR is set: %q -> %q", i, input, result)
		}
	}
}
