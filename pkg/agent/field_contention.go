// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Writer-class category labels used by FieldContention.
const (
	WriterCategoryController  = "controller"
	WriterCategoryInteractive = "interactive"
	WriterCategoryTerraform   = "terraform"
)

// FieldContention reports whether multiple distinct recognized writer classes
// claim the same canonical field path in metadata.managedFields.
//
// Contention is strictly deterministic and conservative: unrecognized managers
// are never guessed into a class, so an unrecognized manager alone (or paired
// with only one recognized writer class) yields Contested: false.
type FieldContention struct {
	Contested  bool     `json:"contested"`
	Writers    []string `json:"writers,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// DetectFieldContention classifies metadata.managedFields[].manager strings
// observed on one exact canonical field path into writer classes and flags
// Contested: true when two or more distinct recognized writer classes claim
// the same field.
//
// Writer classes:
//   - "controller": manager matching the expected owner's controller
//   - "agent/<tool>": self-reported G5 agent manager (each distinct <tool> is
//     its own writer class; multiple runs of the same <tool> share one class)
//   - "interactive": kubectl/* interactive managers (when not the expected controller)
//   - "terraform": Terraform Kubernetes provider managers ("HashiCorp", "Terraform")
//
// Unrecognized managers are preserved in Writers for transparency but are never
// assigned a guessed category.
func DetectFieldContention(owner Ownership, managers []string) FieldContention {
	if len(managers) == 0 {
		return FieldContention{}
	}

	writerSet := make(map[string]struct{}, len(managers))
	categorySet := make(map[string]struct{})

	for _, raw := range managers {
		m := strings.TrimSpace(raw)
		if m == "" {
			continue
		}
		writerSet[m] = struct{}{}
		if cat := classifyFieldWriterCategory(owner, m); cat != "" {
			categorySet[cat] = struct{}{}
		}
	}

	if len(writerSet) == 0 {
		return FieldContention{}
	}

	writers := make([]string, 0, len(writerSet))
	for w := range writerSet {
		writers = append(writers, w)
	}
	sort.Strings(writers)

	var categories []string
	if len(categorySet) > 0 {
		categories = make([]string, 0, len(categorySet))
		for c := range categorySet {
			categories = append(categories, c)
		}
		sort.Strings(categories)
	}

	out := FieldContention{
		Writers:    writers,
		Categories: categories,
	}
	if len(categories) >= 2 {
		out.Contested = true
		out.Reason = fmt.Sprintf("multiple writer classes claim this field: %s", strings.Join(categories, ", "))
	}
	return out
}

func classifyFieldWriterCategory(owner Ownership, manager string) string {
	if IsControllerManagerFor(manager, owner.Type, owner.SubType) {
		return WriterCategoryController
	}
	if tool, _, ok := ParseAgentFieldManager(manager); ok {
		return ManagerAgentPrefix + tool
	}
	if IsInteractiveManager(manager) {
		return WriterCategoryInteractive
	}
	if manager == ManagerTerraformKubernetesProvider || manager == ManagerTerraformManifest {
		return WriterCategoryTerraform
	}
	return ""
}
