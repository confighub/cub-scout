// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
)

// renderKroLineageHuman renders a compact kro lineage section.
// It is intended for reverse trace UX where we already have a local object set.
func renderKroLineageHuman(lineage *agent.KroLineage) string {
	if lineage == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s%skro lineage:%s\n", colorBold, colorWhite, colorReset))

	b.WriteString(fmt.Sprintf("  %smanaged:%s    %s\n", colorDim, colorReset, lineage.Managed.Ref.String()))

	if lineage.Instance.Ref.Name != "" {
		label := lineage.Instance.Ref.String()
		if !lineage.Instance.Present {
			label += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		b.WriteString(fmt.Sprintf("  %sinstance:%s   %s\n", colorDim, colorReset, label))
	}

	if lineage.Definition != nil && lineage.Definition.Ref.Name != "" {
		label := lineage.Definition.Ref.String()
		if !lineage.Definition.Present {
			label += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		b.WriteString(fmt.Sprintf("  %sdefinition:%s %s\n", colorDim, colorReset, label))
	}

	if len(lineage.Evidence) > 0 {
		b.WriteString(fmt.Sprintf("  %sevidence:%s   %s\n", colorDim, colorReset, strings.Join(lineage.Evidence, ", ")))
	}

	b.WriteString("\n")
	return b.String()
}
