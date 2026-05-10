package remedy

import (
	"fmt"
	"sync"
)

// Registry holds all available suggesters.
type Registry struct {
	mu         sync.RWMutex
	suggesters map[RemedyType]Suggester
}

// NewRegistry creates a new suggester registry.
func NewRegistry() *Registry {
	return &Registry{
		suggesters: make(map[RemedyType]Suggester),
	}
}

// Register adds a suggester to the registry.
func (r *Registry) Register(s Suggester) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suggesters[s.Type()] = s
}

// Get returns a suggester by type.
func (r *Registry) Get(t RemedyType) (Suggester, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.suggesters[t]
	return s, ok
}

// SuggesterFor returns the suggester that can describe a fix for a finding.
func (r *Registry) SuggesterFor(finding *Finding) (Suggester, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.suggesters[finding.RemedyType]
	if !ok {
		return nil, fmt.Errorf("no suggester registered for type %s", finding.RemedyType)
	}

	if !s.CanSuggest(finding) {
		return nil, fmt.Errorf("suggester for %s cannot describe this finding", finding.RemedyType)
	}

	return s, nil
}

// Types returns all registered remedy types.
func (r *Registry) Types() []RemedyType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]RemedyType, 0, len(r.suggesters))
	for t := range r.suggesters {
		types = append(types, t)
	}
	return types
}

// IsAutoFixable reports whether a remedy type can be fully described as a
// single auto-suggestable fix. (cub-scout never applies the fix; this only
// gates whether a structured suggestion is producible.)
func IsAutoFixable(t RemedyType) bool {
	for _, autoType := range AutoFixableTypes {
		if t == autoType {
			return true
		}
	}
	return false
}

// DefaultRegistry creates a registry with all standard suggesters.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewConfigFixSuggester())
	r.Register(NewTriggerActionSuggester())
	r.Register(NewDeleteResourceSuggester())
	r.Register(NewRestartSuggester())
	return r
}
