// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// The scope comes from the actual successful LIST, never a guessed Kind/GVR
// mapping. Retained entries below are private diff history, not current reads.
type watchInventoryScope struct {
	APIVersion string
	Resource   string
	Namespace  string
}

func watchInventoryUnreadable(prev, curr watchState, id string) bool {
	if len(curr.omissions) == 0 {
		return false
	}
	scope, known := prev.entryScopes[id]
	if !known {
		return true
	} // Missing provenance cannot authorize a deletion.
	for _, omission := range curr.omissions {
		if omission.APIVersion == scope.APIVersion && omission.Resource == scope.Resource && omission.Namespace == scope.Namespace {
			return true
		}
	}
	return false
}

// Do not mutate curr: its successfully observed inventory and event evidence
// stay separate from old observations retained only to bridge unreadable cycles.
func watchDiffBaseline(prev, curr watchState) watchState {
	result := curr
	result.entriesByID = make(map[string]MapEntry, len(curr.entriesByID))
	result.entryScopes = make(map[string]watchInventoryScope, len(curr.entryScopes))
	for id, entry := range curr.entriesByID {
		result.entriesByID[id] = entry
	}
	for id, scope := range curr.entryScopes {
		result.entryScopes[id] = scope
	}
	for id, entry := range prev.entriesByID {
		if _, present := curr.entriesByID[id]; !present && watchInventoryUnreadable(prev, curr, id) {
			result.entriesByID[id] = entry
			if scope, known := prev.entryScopes[id]; known {
				result.entryScopes[id] = scope
			}
		}
	}
	return result
}
