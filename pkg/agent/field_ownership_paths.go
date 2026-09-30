// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// AttributeFieldsByManagedFields extends AttributeFieldMutation by computing
// per-field-path classifications when the K8s metadata.managedFields entries
// carry FieldsV1 data. Returns both the per-path map and the resource-level
// rollup (the latter matching AttributeFieldMutation).
//
// Keys in the returned map are canonical field-path strings as rendered by
// sigs.k8s.io/structured-merge-diff/v4/fieldpath.Path.String — for example
// ".spec.replicas" or ".spec.template.spec.containers[name=\"app\"].image".
//
// Per-path classification follows the same co-signal rule as the
// resource-level classifier:
//   - Any manager owning this path matches the expected owner's known
//     controller string → CauseControllerDrift.
//   - Only kubectl-*/bare kubectl owners on this path → CauseManualEdit.
//   - Both → CauseManualEdit (operator edited on top of controller).
//   - Otherwise → CauseUnknown.
//
// FieldsV1 entries that fail to decode are skipped silently; they do not
// produce a classification of their own (graceful degradation).
func AttributeFieldsByManagedFields(resource *unstructured.Unstructured, expectedOwner Ownership) (FieldMutationAttribution, map[string]FieldMutationAttribution) {
	rollup, paths, _ := attributeFieldsByManagedFields(resource, expectedOwner, false)
	return rollup, paths
}

// AttributeFieldPath classifies exactly one canonical path from managedFields.
// Unlike the resource rollup, it never falls back to other fields. Unknown
// managers are retained in Managers but do not receive a guessed class.
func AttributeFieldPath(resource *unstructured.Unstructured, expectedOwner Ownership, path string) (FieldMutationAttribution, bool, bool) {
	if err := ValidateCanonicalFieldPath(path); err != nil {
		return FieldMutationAttribution{Cause: CauseUnknown}, false, false
	}
	if resource == nil {
		return FieldMutationAttribution{Cause: CauseUnknown}, false, false
	}
	byPath, incomplete := fieldAttributionsByEntries(resource.GetManagedFields(), expectedOwner, true)
	attr, ok := byPath[path]
	if !ok {
		return FieldMutationAttribution{Cause: CauseUnknown}, false, incomplete
	}
	return attr, ok, incomplete
}

// ValidateCanonicalFieldPath rejects selectors that could match more than one
// field. A path is exact only when it is a concrete fieldpath.Path.String key;
// callers then require an exact match in the parsed FieldsV1 entries.
func ValidateCanonicalFieldPath(path string) error {
	if len(path) == 0 || len(path) > 1024 || !strings.HasPrefix(path, ".") {
		return fmt.Errorf("field path must be a canonical absolute path")
	}
	if strings.ContainsAny(path, "\n\r\t") {
		return fmt.Errorf("field path must be exact; wildcard selectors are not supported")
	}
	if hasUnquotedWildcard(path) {
		return fmt.Errorf("field path must be exact; wildcard selectors are not supported")
	}
	if strings.TrimSpace(path) != path {
		return fmt.Errorf("field path must not contain surrounding whitespace")
	}
	return nil
}

func hasUnquotedWildcard(path string) bool {
	quoted, escaped := false, false
	for _, r := range path {
		if quoted {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				quoted = false
			}
			continue
		}
		if r == '"' {
			quoted = true
			continue
		}
		if r == '*' || r == '?' {
			return true
		}
	}
	return false
}

func attributeFieldsByManagedFields(resource *unstructured.Unstructured, expectedOwner Ownership, includeUnknown bool) (FieldMutationAttribution, map[string]FieldMutationAttribution, bool) {
	resourceLevel := AttributeFieldMutation(resource, expectedOwner)
	if resource == nil {
		return resourceLevel, nil, false
	}
	paths, incomplete := fieldAttributionsByEntries(resource.GetManagedFields(), expectedOwner, includeUnknown)
	return resourceLevel, paths, incomplete
}

func fieldAttributionsByEntries(entries []metav1.ManagedFieldsEntry, expectedOwner Ownership, includeUnknown bool) (map[string]FieldMutationAttribution, bool) {
	if len(entries) == 0 {
		return nil, false
	}

	type pathAccum struct {
		sawController   bool
		sawInteractive  bool
		controllerHint  string
		interactiveHint string
		managers        map[string]struct{}
	}
	byPath := make(map[string]*pathAccum)
	incomplete := false

	for _, e := range entries {
		m := e.Manager
		if m == "" {
			if e.FieldsV1 != nil && len(e.FieldsV1.Raw) > 0 {
				incomplete = true
			}
			continue
		}
		if e.FieldsV1 == nil || len(e.FieldsV1.Raw) == 0 {
			incomplete = true
			continue
		}
		set := &fieldpath.Set{}
		if err := set.FromJSON(bytes.NewReader(e.FieldsV1.Raw)); err != nil {
			incomplete = true
			continue
		}

		isController := IsControllerManagerFor(m, expectedOwner.Type, expectedOwner.SubType)
		isInteractive := !isController && IsInteractiveManager(m)
		if !includeUnknown && !isController && !isInteractive {
			continue
		}

		set.Iterate(func(p fieldpath.Path) {
			key := p.String()
			acc, ok := byPath[key]
			if !ok {
				acc = &pathAccum{managers: make(map[string]struct{})}
				byPath[key] = acc
			}
			acc.managers[m] = struct{}{}
			if isController {
				acc.sawController = true
				if acc.controllerHint == "" {
					acc.controllerHint = m
				}
			} else if isInteractive {
				acc.sawInteractive = true
				if acc.interactiveHint == "" {
					acc.interactiveHint = m
				}
			}
		})
	}

	if len(byPath) == 0 {
		return nil, incomplete
	}

	out := make(map[string]FieldMutationAttribution, len(byPath))
	for path, acc := range byPath {
		var managers []string
		if includeUnknown {
			managers = make([]string, 0, len(acc.managers))
			for manager := range acc.managers {
				managers = append(managers, manager)
			}
			sort.Strings(managers)
		}
		switch {
		case acc.sawController && acc.sawInteractive:
			if includeUnknown {
				// For exact-field evidence, shared ownership is ambiguous. Keep
				// every observed manager but do not infer a cause from co-ownership.
				out[path] = FieldMutationAttribution{Cause: CauseUnknown, Managers: managers}
			} else {
				out[path] = FieldMutationAttribution{
					Cause:       CauseManualEdit,
					ManagerHint: acc.interactiveHint,
				}
			}
		case acc.sawController:
			out[path] = FieldMutationAttribution{
				Cause:       CauseControllerDrift,
				ManagerHint: acc.controllerHint,
				Managers:    managers,
			}
		case acc.sawInteractive:
			out[path] = FieldMutationAttribution{
				Cause:       CauseManualEdit,
				ManagerHint: acc.interactiveHint,
				Managers:    managers,
			}
		default:
			if includeUnknown {
				out[path] = FieldMutationAttribution{Cause: CauseUnknown, Managers: managers}
			}
		}
	}
	return out, incomplete
}
