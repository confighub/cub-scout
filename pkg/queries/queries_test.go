// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package queries

import (
	"os"
	"path/filepath"
	"testing"
)

// --- BuiltinQueries Tests ---

func TestBuiltinQueries_NotEmpty(t *testing.T) {
	if len(BuiltinQueries) == 0 {
		t.Fatal("BuiltinQueries should not be empty")
	}
}

func TestBuiltinQueries_AllHaveCategory(t *testing.T) {
	for _, q := range BuiltinQueries {
		if q.Category != "builtin" {
			t.Errorf("builtin query %q has category %q, want %q", q.Name, q.Category, "builtin")
		}
	}
}

func TestBuiltinQueries_UniqueNames(t *testing.T) {
	seen := map[string]bool{}
	for _, q := range BuiltinQueries {
		if seen[q.Name] {
			t.Errorf("duplicate builtin query name: %q", q.Name)
		}
		seen[q.Name] = true
	}
}

func TestBuiltinQueries_AllHaveRequiredFields(t *testing.T) {
	for _, q := range BuiltinQueries {
		if q.Name == "" {
			t.Error("builtin query has empty Name")
		}
		if q.Description == "" {
			t.Errorf("builtin query %q has empty Description", q.Name)
		}
		if q.Query == "" {
			t.Errorf("builtin query %q has empty Query", q.Name)
		}
	}
}

func TestBuiltinQueries_KnownQueriesExist(t *testing.T) {
	names := map[string]bool{}
	for _, q := range BuiltinQueries {
		names[q.Name] = true
	}

	expected := []string{"unmanaged", "gitops", "helm-only", "flux", "argo", "confighub", "deployments", "services", "prod"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected builtin query %q not found", name)
		}
	}
}

// --- QueryStore Tests ---

func TestNewQueryStore_ContainsBuiltins(t *testing.T) {
	store, err := NewQueryStore()
	if err != nil {
		t.Fatalf("NewQueryStore: %v", err)
	}

	builtins := store.ListBuiltin()
	if len(builtins) != len(BuiltinQueries) {
		t.Errorf("ListBuiltin() returned %d queries, want %d", len(builtins), len(BuiltinQueries))
	}
}

func TestQueryStore_List_ReturnsAll(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "a", Category: "builtin"},
			{Name: "b", Category: "user"},
		},
	}

	all := store.List()
	if len(all) != 2 {
		t.Errorf("List() returned %d, want 2", len(all))
	}
}

func TestQueryStore_ListBuiltin_FiltersCorrectly(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "a", Category: "builtin"},
			{Name: "b", Category: "user"},
			{Name: "c", Category: "builtin"},
		},
	}

	builtins := store.ListBuiltin()
	if len(builtins) != 2 {
		t.Errorf("ListBuiltin() returned %d, want 2", len(builtins))
	}
	for _, q := range builtins {
		if q.Category != "builtin" {
			t.Errorf("ListBuiltin() returned query with category %q", q.Category)
		}
	}
}

func TestQueryStore_ListUser_FiltersCorrectly(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "a", Category: "builtin"},
			{Name: "b", Category: "user"},
			{Name: "c", Category: "user"},
		},
	}

	users := store.ListUser()
	if len(users) != 2 {
		t.Errorf("ListUser() returned %d, want 2", len(users))
	}
	for _, q := range users {
		if q.Category != "user" {
			t.Errorf("ListUser() returned query with category %q", q.Category)
		}
	}
}

func TestQueryStore_ListUser_EmptyWhenNoUserQueries(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "a", Category: "builtin"},
		},
	}

	users := store.ListUser()
	if len(users) != 0 {
		t.Errorf("ListUser() returned %d, want 0", len(users))
	}
}

func TestQueryStore_Get_Found(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "flux", Query: "owner=Flux", Category: "builtin"},
			{Name: "my-query", Query: "namespace=prod", Category: "user"},
		},
	}

	q, found := store.Get("flux")
	if !found {
		t.Fatal("Get('flux') not found")
	}
	if q.Query != "owner=Flux" {
		t.Errorf("Get('flux').Query = %q, want %q", q.Query, "owner=Flux")
	}
}

func TestQueryStore_Get_NotFound(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "flux", Query: "owner=Flux"},
		},
	}

	_, found := store.Get("nonexistent")
	if found {
		t.Error("Get('nonexistent') should return false")
	}
}

func TestQueryStore_Get_ReturnsFirstMatch(t *testing.T) {
	store := &QueryStore{
		queries: []SavedQuery{
			{Name: "flux", Query: "owner=Flux", Description: "first"},
			{Name: "flux", Query: "owner=Flux2", Description: "second"},
		},
	}

	q, found := store.Get("flux")
	if !found {
		t.Fatal("Get('flux') not found")
	}
	if q.Description != "first" {
		t.Errorf("Get should return first match, got Description=%q", q.Description)
	}
}

// --- File I/O Tests (using temp directory) ---

// withTempHome sets HOME to a temp directory for the duration of the test,
// restoring the original value afterward. Returns the temp home path.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpHome)
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
	})
	return tmpHome
}

func TestUserQueriesFile_ReturnsPath(t *testing.T) {
	path := UserQueriesFile()
	if path == "" {
		t.Fatal("UserQueriesFile() returned empty string")
	}
	if filepath.Base(path) != "queries.yaml" {
		t.Errorf("UserQueriesFile() base = %q, want %q", filepath.Base(path), "queries.yaml")
	}
}

func TestLoadUserQueries_NoFile(t *testing.T) {
	_ = withTempHome(t)

	queries, err := LoadUserQueries()
	if err != nil {
		t.Fatalf("LoadUserQueries with no file: %v", err)
	}
	if queries != nil {
		t.Errorf("LoadUserQueries with no file returned %v, want nil", queries)
	}
}

func TestSaveUserQuery_CreatesFile(t *testing.T) {
	tmpHome := withTempHome(t)

	q := SavedQuery{
		Name:        "test-query",
		Description: "test description",
		Query:       "owner=Flux",
	}

	err := SaveUserQuery(q)
	if err != nil {
		t.Fatalf("SaveUserQuery: %v", err)
	}

	// Verify file was created
	expectedPath := filepath.Join(tmpHome, ".confighub", "queries.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("queries file not created at %s", expectedPath)
	}

	// Verify content
	loaded, err := LoadUserQueries()
	if err != nil {
		t.Fatalf("LoadUserQueries: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d queries, want 1", len(loaded))
	}
	if loaded[0].Name != "test-query" {
		t.Errorf("loaded query name = %q, want %q", loaded[0].Name, "test-query")
	}
	if loaded[0].Query != "owner=Flux" {
		t.Errorf("loaded query = %q, want %q", loaded[0].Query, "owner=Flux")
	}
}

func TestSaveUserQuery_UpdatesExisting(t *testing.T) {
	_ = withTempHome(t)

	// Save initial
	SaveUserQuery(SavedQuery{Name: "q1", Query: "owner=Flux"})

	// Update
	err := SaveUserQuery(SavedQuery{Name: "q1", Query: "owner=Argo"})
	if err != nil {
		t.Fatalf("SaveUserQuery update: %v", err)
	}

	loaded, _ := LoadUserQueries()
	if len(loaded) != 1 {
		t.Fatalf("after update: loaded %d queries, want 1", len(loaded))
	}
	if loaded[0].Query != "owner=Argo" {
		t.Errorf("after update: query = %q, want %q", loaded[0].Query, "owner=Argo")
	}
}

func TestSaveUserQuery_AppendsNew(t *testing.T) {
	_ = withTempHome(t)

	SaveUserQuery(SavedQuery{Name: "q1", Query: "owner=Flux"})
	SaveUserQuery(SavedQuery{Name: "q2", Query: "owner=Argo"})

	loaded, _ := LoadUserQueries()
	if len(loaded) != 2 {
		t.Fatalf("after append: loaded %d queries, want 2", len(loaded))
	}
}

func TestDeleteUserQuery_RemovesQuery(t *testing.T) {
	_ = withTempHome(t)

	SaveUserQuery(SavedQuery{Name: "q1", Query: "owner=Flux"})
	SaveUserQuery(SavedQuery{Name: "q2", Query: "owner=Argo"})

	err := DeleteUserQuery("q1")
	if err != nil {
		t.Fatalf("DeleteUserQuery: %v", err)
	}

	loaded, _ := LoadUserQueries()
	if len(loaded) != 1 {
		t.Fatalf("after delete: loaded %d queries, want 1", len(loaded))
	}
	if loaded[0].Name != "q2" {
		t.Errorf("remaining query = %q, want %q", loaded[0].Name, "q2")
	}
}

func TestDeleteUserQuery_NotFound(t *testing.T) {
	_ = withTempHome(t)

	SaveUserQuery(SavedQuery{Name: "q1", Query: "owner=Flux"})

	err := DeleteUserQuery("nonexistent")
	if err == nil {
		t.Fatal("DeleteUserQuery('nonexistent') should return error")
	}
}

func TestDeleteUserQuery_NoFile(t *testing.T) {
	_ = withTempHome(t)

	err := DeleteUserQuery("anything")
	if err == nil {
		t.Fatal("DeleteUserQuery with no file should return error")
	}
}

func TestLoadUserQueries_MalformedYAML(t *testing.T) {
	tmpHome := withTempHome(t)

	dir := filepath.Join(tmpHome, ".confighub")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "queries.yaml"), []byte("{{invalid yaml"), 0644)

	_, err := LoadUserQueries()
	if err == nil {
		t.Fatal("LoadUserQueries with malformed YAML should return error")
	}
}

func TestNewQueryStore_IncludesUserQueries(t *testing.T) {
	_ = withTempHome(t)

	// Save a user query
	SaveUserQuery(SavedQuery{Name: "my-custom", Query: "namespace=staging"})

	store, err := NewQueryStore()
	if err != nil {
		t.Fatalf("NewQueryStore: %v", err)
	}

	// Should have builtins + 1 user query
	all := store.List()
	if len(all) != len(BuiltinQueries)+1 {
		t.Errorf("List() returned %d, want %d", len(all), len(BuiltinQueries)+1)
	}

	// User query should be retrievable
	q, found := store.Get("my-custom")
	if !found {
		t.Fatal("user query 'my-custom' not found in store")
	}
	if q.Category != "user" {
		t.Errorf("user query category = %q, want %q", q.Category, "user")
	}
}

func TestNewQueryStore_UserQueryCategorySetToUser(t *testing.T) {
	_ = withTempHome(t)

	// Save a query without explicit category
	SaveUserQuery(SavedQuery{Name: "no-cat", Query: "kind=Pod"})

	store, _ := NewQueryStore()
	q, found := store.Get("no-cat")
	if !found {
		t.Fatal("query 'no-cat' not found")
	}
	if q.Category != "user" {
		t.Errorf("category = %q, want 'user' (should be force-set by NewQueryStore)", q.Category)
	}
}

// --- Round-trip Tests ---

func TestSaveAndLoadRoundTrip(t *testing.T) {
	_ = withTempHome(t)

	original := SavedQuery{
		Name:        "complex-query",
		Description: "A query with special characters: owner=Flux OR namespace=prod-*",
		Query:       "owner=Flux OR namespace=prod-*",
		Category:    "user",
	}

	if err := SaveUserQuery(original); err != nil {
		t.Fatalf("SaveUserQuery: %v", err)
	}

	loaded, err := LoadUserQueries()
	if err != nil {
		t.Fatalf("LoadUserQueries: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("loaded %d queries, want 1", len(loaded))
	}

	got := loaded[0]
	if got.Name != original.Name {
		t.Errorf("Name = %q, want %q", got.Name, original.Name)
	}
	if got.Description != original.Description {
		t.Errorf("Description = %q, want %q", got.Description, original.Description)
	}
	if got.Query != original.Query {
		t.Errorf("Query = %q, want %q", got.Query, original.Query)
	}
}
