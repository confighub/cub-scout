package patterns

import "sort"

// registry holds all registered patterns.
var registry = make(map[string]Pattern)

// Register adds a pattern to the registry.
// Panics if a pattern with the same ID is already registered.
func Register(p Pattern) {
	if _, exists := registry[p.ID]; exists {
		panic("pattern already registered: " + p.ID)
	}
	registry[p.ID] = p
}

// Get returns a pattern by ID, or nil if not found.
func Get(id string) *Pattern {
	if p, ok := registry[id]; ok {
		return &p
	}
	return nil
}

// List returns all registered patterns in deterministic order (sorted by ID).
func List() []Pattern {
	patterns := make([]Pattern, 0, len(registry))
	for _, p := range registry {
		patterns = append(patterns, p)
	}

	// Sort by ID for deterministic output
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].ID < patterns[j].ID
	})

	return patterns
}

// IDs returns all registered pattern IDs in deterministic order.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
