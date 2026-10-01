// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func renderSourceTruthASCII(e agent.SourceTruthEvidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Source truth: %s (%s)\n", e.SourceTruth, e.Status)
	fmt.Fprintf(&b, "Strategy: %s\n", valueOrUnknown(e.DeclaredStrategy))
	if e.Context != "" {
		fmt.Fprintf(&b, "Selected Kubernetes context: %s (label, not stable cluster identity)\n", e.Context)
	}
	fmt.Fprintf(&b, "Outlier: %s\n", valueOrUnknown(string(e.Outlier)))
	b.WriteString("\nEvidence surfaces\n")
	if s := e.Surfaces.ConfigHub; s != nil {
		fmt.Fprintf(&b, "  ConfigHub: space=%s unit=%s revision=%s\n", valueOrUnknown(s.Space), valueOrUnknown(s.Unit), valueOrUnknown(s.Revision))
	} else {
		b.WriteString("  ConfigHub: unavailable\n")
	}
	if s := e.Surfaces.Controller; s != nil {
		fmt.Fprintf(&b, "  Controller: %s source=%s revision/digest=%s health=%s\n", valueOrUnknown(s.Kind), valueOrUnknown(s.Source), valueOrUnknown(s.RevisionOrDigest), valueOrUnknown(s.Health))
	} else {
		b.WriteString("  Controller: unavailable\n")
	}
	if s := e.Surfaces.Runtime; s != nil {
		fmt.Fprintf(&b, "  Runtime: %s %s=%s health=%s\n", valueOrUnknown(s.Resource), valueOrUnknown(s.Field), valueOrUnknown(s.Value), valueOrUnknown(s.Health))
	} else {
		b.WriteString("  Runtime: unavailable\n")
	}
	if len(e.ProofGaps) > 0 {
		fmt.Fprintf(&b, "\nProof gaps: %s\n", strings.Join(e.ProofGaps, ", "))
	}
	if len(e.CollectionErrors) > 0 {
		b.WriteString("Collection errors:\n")
		for _, err := range e.CollectionErrors {
			fmt.Fprintf(&b, "  - %s\n", err)
		}
	}
	if e.SafeNextAction != "" {
		fmt.Fprintf(&b, "Next: %s\n", e.SafeNextAction)
	}
	return b.String()
}

func renderSourceTruthMarkdown(e agent.SourceTruthEvidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Source truth: %s (%s)\n\n", e.SourceTruth, e.Status)
	fmt.Fprintf(&b, "- Strategy: %s\n", valueOrUnknown(e.DeclaredStrategy))
	if e.Context != "" {
		fmt.Fprintf(&b, "- Selected Kubernetes context: `%s` (a label, not a stable cluster identity)\n", e.Context)
	}
	fmt.Fprintf(&b, "- Outlier: %s\n\n", valueOrUnknown(string(e.Outlier)))
	b.WriteString("### Evidence surfaces\n\n")
	if s := e.Surfaces.ConfigHub; s != nil {
		fmt.Fprintf(&b, "- ConfigHub: space `%s`, unit `%s`, revision `%s`\n", valueOrUnknown(s.Space), valueOrUnknown(s.Unit), valueOrUnknown(s.Revision))
	} else {
		b.WriteString("- ConfigHub: unavailable\n")
	}
	if s := e.Surfaces.Controller; s != nil {
		fmt.Fprintf(&b, "- Controller: %s, source `%s`, revision/digest `%s`, health `%s`\n", valueOrUnknown(s.Kind), valueOrUnknown(s.Source), valueOrUnknown(s.RevisionOrDigest), valueOrUnknown(s.Health))
	} else {
		b.WriteString("- Controller: unavailable\n")
	}
	if s := e.Surfaces.Runtime; s != nil {
		fmt.Fprintf(&b, "- Runtime: %s, `%s` = `%s`, health `%s`\n", valueOrUnknown(s.Resource), valueOrUnknown(s.Field), valueOrUnknown(s.Value), valueOrUnknown(s.Health))
	} else {
		b.WriteString("- Runtime: unavailable\n")
	}
	if len(e.ProofGaps) > 0 {
		fmt.Fprintf(&b, "\nProof gaps: %s\n", strings.Join(e.ProofGaps, ", "))
	}
	if len(e.CollectionErrors) > 0 {
		b.WriteString("\nCollection errors:\n")
		for _, err := range e.CollectionErrors {
			fmt.Fprintf(&b, "- %s\n", err)
		}
	}
	if e.SafeNextAction != "" {
		fmt.Fprintf(&b, "\nNext: %s\n", e.SafeNextAction)
	}
	return b.String()
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
