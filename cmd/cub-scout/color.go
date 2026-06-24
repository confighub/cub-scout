// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"strings"
)

// ANSI color codes for CLI output.
// These are only applied when color is enabled (NO_COLOR is not set).
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiPurple = "\033[35m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[37m"
)

// colorEnabled returns true if color output is enabled.
// Color is disabled if NO_COLOR environment variable is set (per no-color.org).
func colorEnabled() bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor
}

// Color wraps text in ANSI color codes if color is enabled.
// Returns the text unchanged if NO_COLOR is set.
func Color(text, colorCode string) string {
	if !colorEnabled() {
		return text
	}
	return colorCode + text + ansiReset
}

// Bold wraps text in bold if color is enabled.
func Bold(text string) string {
	return Color(text, ansiBold)
}

// Dim wraps text in dim/faint style if color is enabled.
func Dim(text string) string {
	return Color(text, ansiDim)
}

// Red wraps text in red color if color is enabled.
func Red(text string) string {
	return Color(text, ansiRed)
}

// Green wraps text in green color if color is enabled.
func Green(text string) string {
	return Color(text, ansiGreen)
}

// Yellow wraps text in yellow color if color is enabled.
func Yellow(text string) string {
	return Color(text, ansiYellow)
}

// Blue wraps text in blue color if color is enabled.
func Blue(text string) string {
	return Color(text, ansiBlue)
}

// Purple wraps text in purple color if color is enabled.
func Purple(text string) string {
	return Color(text, ansiPurple)
}

// Cyan wraps text in cyan color if color is enabled.
func Cyan(text string) string {
	return Color(text, ansiCyan)
}

// White wraps text in white color if color is enabled.
func White(text string) string {
	return Color(text, ansiWhite)
}

// BoldColor wraps text in bold + color if color is enabled.
func BoldColor(text, colorCode string) string {
	if !colorEnabled() {
		return text
	}
	return ansiBold + colorCode + text + ansiReset
}

// BoldRed wraps text in bold red if color is enabled.
func BoldRed(text string) string {
	return BoldColor(text, ansiRed)
}

// BoldGreen wraps text in bold green if color is enabled.
func BoldGreen(text string) string {
	return BoldColor(text, ansiGreen)
}

// BoldYellow wraps text in bold yellow if color is enabled.
func BoldYellow(text string) string {
	return BoldColor(text, ansiYellow)
}

// BoldBlue wraps text in bold blue if color is enabled.
func BoldBlue(text string) string {
	return BoldColor(text, ansiBlue)
}

// BoldCyan wraps text in bold cyan if color is enabled.
func BoldCyan(text string) string {
	return BoldColor(text, ansiCyan)
}

// StatusColor returns colored text based on health/status string.
// Healthy/Ready/True -> green, Warning/Degraded -> yellow, Error/Failed/False -> red.
func StatusColor(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case lower == "healthy" || lower == "ready" || lower == "true" || lower == "running" || lower == "succeeded":
		return Green(text)
	case lower == "warning" || lower == "degraded" || lower == "progressing" || lower == "pending":
		return Yellow(text)
	case lower == "error" || lower == "failed" || lower == "false" || lower == "unhealthy" || lower == "unavailable":
		return Red(text)
	default:
		return text
	}
}

// SeverityColor returns colored text based on severity level.
// CRITICAL -> bold red, HIGH -> red, WARNING/MEDIUM -> yellow, INFO/LOW -> dim.
func SeverityColor(text string) string {
	upper := strings.ToUpper(strings.TrimSpace(text))
	switch upper {
	case "CRITICAL":
		return BoldRed(text)
	case "HIGH":
		return Red(text)
	case "WARNING", "MEDIUM":
		return Yellow(text)
	case "INFO", "LOW":
		return Dim(text)
	default:
		return text
	}
}

// OwnerColor returns colored text based on GitOps owner.
// Each owner gets a distinct color for quick visual scanning.
func OwnerColor(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch lower {
	case "flux":
		return Purple(text)
	case "argocd", "argo":
		return Cyan(text)
	case "helm":
		return Blue(text)
	case "confighub":
		return BoldBlue(text)
	case "sveltos":
		return BoldCyan(text)
	case "modelplane":
		return Green(text)
	case "native":
		return Yellow(text)
	case "crossplane":
		return Green(text)
	case "kro":
		return Cyan(text)
	case "terraform":
		return Purple(text)
	default:
		return text
	}
}

// KindColor returns colored text based on Kubernetes resource kind.
// Workloads get one color, configs another, etc.
func KindColor(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch lower {
	case "deployment", "statefulset", "daemonset", "replicaset", "pod":
		return Blue(text)
	case "service", "ingress", "networkpolicy":
		return Cyan(text)
	case "configmap", "secret":
		return Yellow(text)
	case "namespace":
		return Green(text)
	case "application", "kustomization", "helmrelease", "gitrepository":
		return Purple(text)
	default:
		return text
	}
}

// CountColor returns colored text for counts - higher counts get warmer colors.
func CountColor(count int, text string) string {
	switch {
	case count == 0:
		return Dim(text)
	case count >= 10:
		return Yellow(text)
	case count >= 50:
		return Red(text)
	default:
		return text
	}
}

// HealthIndicator returns a colored health indicator symbol.
// healthy -> green checkmark, warning -> yellow warning, error -> red X.
func HealthIndicator(health string) string {
	lower := strings.ToLower(strings.TrimSpace(health))
	switch {
	case lower == "healthy" || lower == "ready" || lower == "true" || lower == "running":
		return Green("✓")
	case lower == "warning" || lower == "degraded" || lower == "progressing":
		return Yellow("⚠")
	case lower == "error" || lower == "failed" || lower == "false" || lower == "unhealthy":
		return Red("✗")
	default:
		return "?"
	}
}

// SectionHeader formats a section header with color.
func SectionHeader(text string) string {
	return BoldCyan(text)
}

// LabelValue formats a label: value pair with the label dimmed.
func LabelValue(label, value string) string {
	return Dim(label+":") + " " + value
}
