// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
)

// renderCrossplaneLineageHuman renders a compact XR-first Crossplane lineage section.
// It is intended for the reverse trace UX where we already have a small local object set.
func renderCrossplaneLineageHuman(lineage *agent.CrossplaneLineage) string {
	if lineage == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s%sCrossplane lineage:%s\n", colorBold, colorWhite, colorReset))

	// Managed resource (always present)
	b.WriteString(fmt.Sprintf("  %smanaged:%s   %s\n", colorDim, colorReset, lineage.Managed.Ref.String()))

	// XR-first platform owner (Composite/XR)
	if lineage.Composite.Ref.Name != "" {
		label := lineage.Composite.Ref.String()
		if !lineage.Composite.Present {
			label += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		b.WriteString(fmt.Sprintf("  %sxr:%s       %s\n", colorDim, colorReset, label))
	}

	// Optional claim (enrichment)
	if lineage.Claim != nil && lineage.Claim.Ref.Name != "" {
		label := lineage.Claim.Ref.String()
		if !lineage.Claim.Present {
			label += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		b.WriteString(fmt.Sprintf("  %sclaim:%s    %s\n", colorDim, colorReset, label))
	}

	if len(lineage.Evidence) > 0 {
		b.WriteString(fmt.Sprintf("  %sevidence:%s %s\n", colorDim, colorReset, strings.Join(lineage.Evidence, ", ")))
	}

	b.WriteString("\n")
	return b.String()
}
