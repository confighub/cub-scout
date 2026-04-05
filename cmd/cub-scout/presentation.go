// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
)

// PresentationMode defines how text/markdown output is framed.
// This affects narrative framing only - JSON output remains unchanged.
// Per the semantic contract, presentation changes must remain narrative-only
// and not introduce machine-relevant meaning absent from JSON.
type PresentationMode string

const (
	// PresentationHuman is the default mode optimized for direct operator reading.
	// Uses standard headings, full explanatory text, and operator-oriented framing.
	PresentationHuman PresentationMode = "human"

	// PresentationAI is optimized for AI assistant consumption.
	// Uses concise section markers, stable ordering, explicit next-step framing,
	// and handoff-oriented language suitable for machine processing.
	PresentationAI PresentationMode = "ai"

	// PresentationPaired is for human-plus-assistant workflows.
	// Balances human readability with structure suitable for copy-paste
	// into an AI assistant. Slightly more compact than human mode.
	PresentationPaired PresentationMode = "paired"
)

// DefaultPresentationMode is the default when no mode is specified.
const DefaultPresentationMode = PresentationHuman

// ValidPresentationModes lists all valid presentation mode values.
var ValidPresentationModes = []PresentationMode{PresentationHuman, PresentationAI, PresentationPaired}

// ParsePresentationMode parses a string into a PresentationMode.
// Returns an error if the value is not valid.
func ParsePresentationMode(s string) (PresentationMode, error) {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "human", "":
		return PresentationHuman, nil
	case "ai":
		return PresentationAI, nil
	case "paired":
		return PresentationPaired, nil
	default:
		return "", fmt.Errorf("invalid presentation mode %q (valid: human, ai, paired)", s)
	}
}

// String returns the string representation of the presentation mode.
func (m PresentationMode) String() string {
	return string(m)
}

// PresentationModeHelp returns help text for the --presentation flag.
func PresentationModeHelp() string {
	return "Presentation mode: human (default), ai (concise for AI assistants), paired (human+AI workflow)"
}

// --- Section heading helpers ---
// These provide mode-specific framing for section headers.
// The content remains the same; only the narrative wrapper changes.

// DoctorHeading returns the appropriate heading for doctor output sections.
func DoctorHeading(mode PresentationMode) string {
	switch mode {
	case PresentationAI:
		return "CLUSTER HEALTH SUMMARY"
	case PresentationPaired:
		return "Cluster Health Summary"
	default:
		return "Cluster Health Summary"
	}
}

// DoctorIntro returns an optional intro line for doctor output.
func DoctorIntro(mode PresentationMode, cluster, namespace string) string {
	switch mode {
	case PresentationAI:
		// AI mode: terse, machine-friendly framing
		return fmt.Sprintf("[scope: cluster=%s namespace=%s]", cluster, namespace)
	case PresentationPaired:
		// Paired mode: brief but human-readable
		return fmt.Sprintf("Cluster: %s (namespace: %s)", cluster, namespace)
	default:
		// Human mode: standard framing
		return fmt.Sprintf("Cluster: %s (namespace: %s)", cluster, namespace)
	}
}

// DoctorOutro returns an optional outro/handoff line for doctor output.
func DoctorOutro(mode PresentationMode) string {
	switch mode {
	case PresentationAI:
		return "[end summary - see RECOMMENDED ACTIONS below]"
	case PresentationPaired:
		return "" // paired mode uses standard TRY NEXT
	default:
		return "" // human mode uses standard TRY NEXT
	}
}

// ExplainHeading returns the appropriate heading for explain output.
func ExplainHeading(mode PresentationMode, resource, namespace string) string {
	switch mode {
	case PresentationAI:
		return fmt.Sprintf("[resource: %s namespace: %s]", resource, namespace)
	case PresentationPaired:
		return fmt.Sprintf("%s in namespace %s:", resource, namespace)
	default:
		return fmt.Sprintf("%s in namespace %s:", resource, namespace)
	}
}

// ExplainOutro returns an optional outro/handoff line for explain output.
func ExplainOutro(mode PresentationMode) string {
	switch mode {
	case PresentationAI:
		return "[end resource context - see RECOMMENDED ACTIONS below]"
	case PresentationPaired:
		return ""
	default:
		return ""
	}
}

// SectionLabel returns a section label with mode-appropriate formatting.
func SectionLabel(mode PresentationMode, label string) string {
	switch mode {
	case PresentationAI:
		// AI mode: uppercase markers for easy parsing
		return strings.ToUpper(label) + ":"
	default:
		return label + ":"
	}
}

// TryNextHeading returns the TRY NEXT section heading.
func TryNextHeading(mode PresentationMode) string {
	switch mode {
	case PresentationAI:
		return "RECOMMENDED ACTIONS:"
	case PresentationPaired:
		return "TRY NEXT:"
	default:
		return "TRY NEXT:"
	}
}
