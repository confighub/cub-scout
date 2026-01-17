// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package queries provides saved query management for the ConfigHub Agent.
//
// Saved queries are named, reusable query expressions that can be:
// - Built-in (shipped with the agent)
// - User-defined (~/.confighub/queries.yaml)
//
// Example queries help users get started with common patterns:
// - unmanaged: Resources with no GitOps owner
// - failed: Failed reconciliations
// - drift: Resources where live != desired
package queries

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SavedQuery represents a named, reusable query
type SavedQuery struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Query       string `yaml:"query" json:"query"`
	Category    string `yaml:"category,omitempty" json:"category,omitempty"` // "builtin" or "user"
}

// QueryStore manages saved queries from multiple sources
type QueryStore struct {
	queries []SavedQuery
}

// BuiltinQueries are shipped with the agent to help users get started
var BuiltinQueries = []SavedQuery{
	{
		Name:        "unmanaged",
		Description: "Resources with no GitOps owner (kubectl apply, etc.)",
		Query:       "owner=Native",
		Category:    "builtin",
	},
	{
		Name:        "gitops",
		Description: "Resources managed by GitOps (Flux or Argo CD)",
		Query:       "owner=Flux OR owner=Argo",
		Category:    "builtin",
	},
	{
		Name:        "helm-only",
		Description: "Helm-managed resources (no GitOps reconciliation)",
		Query:       "owner=Helm",
		Category:    "builtin",
	},
	{
		Name:        "flux",
		Description: "All Flux-managed resources",
		Query:       "owner=Flux",
		Category:    "builtin",
	},
	{
		Name:        "argo",
		Description: "All Argo CD-managed resources",
		Query:       "owner=Argo",
		Category:    "builtin",
	},
	{
		Name:        "confighub",
		Description: "Resources managed by ConfigHub",
		Query:       "owner=ConfigHub",
		Category:    "builtin",
	},
	{
		Name:        "deployments",
		Description: "All Deployments across namespaces",
		Query:       "kind=Deployment",
		Category:    "builtin",
	},
	{
		Name:        "services",
		Description: "All Services across namespaces",
		Query:       "kind=Service",
		Category:    "builtin",
	},
	{
		Name:        "prod",
		Description: "Resources in production namespaces",
		Query:       "namespace=prod* OR namespace=production*",
		Category:    "builtin",
	},
}

// UserQueriesFile is the path to user-defined queries
func UserQueriesFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".confighub", "queries.yaml")
}

// UserQueriesConfig is the structure of the user queries file
type UserQueriesConfig struct {
	Queries []SavedQuery `yaml:"queries"`
}

// NewQueryStore creates a new QueryStore with built-in and user queries
func NewQueryStore() (*QueryStore, error) {
	store := &QueryStore{
		queries: make([]SavedQuery, 0, len(BuiltinQueries)),
	}

	// Add built-in queries
	store.queries = append(store.queries, BuiltinQueries...)

	// Load user queries if they exist
	userQueries, err := LoadUserQueries()
	if err == nil && len(userQueries) > 0 {
		for i := range userQueries {
			userQueries[i].Category = "user"
		}
		store.queries = append(store.queries, userQueries...)
	}

	return store, nil
}

// LoadUserQueries loads queries from the user's config file
func LoadUserQueries() ([]SavedQuery, error) {
	path := UserQueriesFile()
	if path == "" {
		return nil, fmt.Errorf("could not determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No user queries file
		}
		return nil, fmt.Errorf("read queries file: %w", err)
	}

	var config UserQueriesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse queries file: %w", err)
	}

	return config.Queries, nil
}

// SaveUserQuery adds or updates a user query
func SaveUserQuery(query SavedQuery) error {
	path := UserQueriesFile()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Load existing queries
	queries, _ := LoadUserQueries()
	if queries == nil {
		queries = []SavedQuery{}
	}

	// Update or append
	found := false
	for i, q := range queries {
		if q.Name == query.Name {
			queries[i] = query
			found = true
			break
		}
	}
	if !found {
		queries = append(queries, query)
	}

	// Write back
	config := UserQueriesConfig{Queries: queries}
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("marshal queries: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write queries file: %w", err)
	}

	return nil
}

// DeleteUserQuery removes a user query
func DeleteUserQuery(name string) error {
	queries, err := LoadUserQueries()
	if err != nil {
		return err
	}

	// Filter out the query to delete
	filtered := make([]SavedQuery, 0, len(queries))
	for _, q := range queries {
		if q.Name != name {
			filtered = append(filtered, q)
		}
	}

	if len(filtered) == len(queries) {
		return fmt.Errorf("query %q not found", name)
	}

	// Write back
	path := UserQueriesFile()
	config := UserQueriesConfig{Queries: filtered}
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("marshal queries: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// List returns all queries (built-in + user)
func (s *QueryStore) List() []SavedQuery {
	return s.queries
}

// ListBuiltin returns only built-in queries
func (s *QueryStore) ListBuiltin() []SavedQuery {
	result := make([]SavedQuery, 0)
	for _, q := range s.queries {
		if q.Category == "builtin" {
			result = append(result, q)
		}
	}
	return result
}

// ListUser returns only user-defined queries
func (s *QueryStore) ListUser() []SavedQuery {
	result := make([]SavedQuery, 0)
	for _, q := range s.queries {
		if q.Category == "user" {
			result = append(result, q)
		}
	}
	return result
}

// Get returns a query by name
func (s *QueryStore) Get(name string) (*SavedQuery, bool) {
	for _, q := range s.queries {
		if q.Name == name {
			return &q, true
		}
	}
	return nil, false
}

