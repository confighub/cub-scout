// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package mapsvc provides the core data types and business logic for the cub-scout map command.
// It separates the data model and processing logic from the CLI and TUI rendering.
package mapsvc

import (
	"strings"
	"time"
)

// Entry represents a resource in the fleet map.
// This is the core data type that represents discovered Kubernetes resources.
type Entry struct {
	ID           string            `json:"id"`
	ClusterName  string            `json:"clusterName"`
	Namespace    string            `json:"namespace"`
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	APIVersion   string            `json:"apiVersion"`
	Owner        string            `json:"owner"`
	OwnerDetails map[string]string `json:"ownerDetails,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Status       string            `json:"status"` // Ready, NotReady, Failed, Pending, Unknown
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// GetField implements query.Matchable for Entry.
// This enables flexible querying of entry fields.
func (e Entry) GetField(field string) (string, bool) {
	// Handle labels[key] syntax
	if len(field) > 7 && field[:7] == "labels[" && field[len(field)-1] == ']' {
		key := field[7 : len(field)-1]
		if e.Labels == nil {
			return "", false
		}
		v, ok := e.Labels[key]
		return v, ok
	}
	switch field {
	case "kind":
		return e.Kind, true
	case "namespace":
		return e.Namespace, true
	case "name":
		return e.Name, true
	case "owner":
		return e.Owner, true
	case "status":
		return e.Status, true
	case "cluster", "clusterName":
		return e.ClusterName, true
	case "apiVersion":
		return e.APIVersion, true
	default:
		return "", false
	}
}

// DisplayOwner returns the canonical display name for an owner type.
// Internal names are lowercase (flux, argo, helm, etc.) but display names are capitalized.
func DisplayOwner(owner string) string {
	switch strings.ToLower(owner) {
	case "flux":
		return "Flux"
	case "argo":
		return "ArgoCD"
	case "helm":
		return "Helm"
	case "confighub":
		return "ConfigHub"
	case "k8s", "native", "unknown", "":
		return "Native"
	default:
		return owner
	}
}

// OwnerStats tracks counts by owner type.
type OwnerStats struct {
	ByOwner  map[string]int
	ByKind   map[string]int
	ByStatus map[string]int
	Total    int
}

// NewOwnerStats creates an initialized OwnerStats.
func NewOwnerStats() *OwnerStats {
	return &OwnerStats{
		ByOwner:  make(map[string]int),
		ByKind:   make(map[string]int),
		ByStatus: make(map[string]int),
	}
}

// Add records an entry in the stats.
func (s *OwnerStats) Add(e Entry) {
	s.Total++
	s.ByOwner[e.Owner]++
	s.ByKind[e.Kind]++
	s.ByStatus[e.Status]++
}
