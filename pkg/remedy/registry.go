package remedy

import (
	"fmt"
	"sync"
)

// Registry holds all available executors
type Registry struct {
	mu        sync.RWMutex
	executors map[RemedyType]Executor
}

// NewRegistry creates a new executor registry
func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[RemedyType]Executor),
	}
}

// Register adds an executor to the registry
func (r *Registry) Register(e Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[e.Type()] = e
}

// Get returns an executor by type
func (r *Registry) Get(t RemedyType) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[t]
	return e, ok
}

// ExecutorFor returns the executor that can handle a finding
func (r *Registry) ExecutorFor(finding *Finding) (Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.executors[finding.RemedyType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for type %s", finding.RemedyType)
	}

	if !e.CanExecute(finding) {
		return nil, fmt.Errorf("executor for %s cannot handle this finding", finding.RemedyType)
	}

	return e, nil
}

// Types returns all registered remedy types
func (r *Registry) Types() []RemedyType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]RemedyType, 0, len(r.executors))
	for t := range r.executors {
		types = append(types, t)
	}
	return types
}

// IsAutoFixable checks if a remedy type can be fully automated
func IsAutoFixable(t RemedyType) bool {
	for _, autoType := range AutoFixableTypes {
		if t == autoType {
			return true
		}
	}
	return false
}

// DefaultRegistry creates a registry with all standard executors
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewConfigFixExecutor())
	r.Register(NewTriggerActionExecutor())
	r.Register(NewDeleteResourceExecutor())
	r.Register(NewRestartExecutor())
	return r
}
