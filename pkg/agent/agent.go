// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides a read-only Kubernetes cluster observer that feeds the ConfigHub Map.
// Unlike BridgeWorker which has read+write capabilities, Agent only watches and reports.
package agent

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Resource represents a discovered Kubernetes resource with ownership info
type Resource struct {
	// Identity
	ID        string                  `json:"id"` // "{cluster}/{namespace}/{group}/{version}/{kind}/{name}"
	GVK       schema.GroupVersionKind `json:"gvk"`
	Namespace string                  `json:"namespace"`
	Name      string                  `json:"name"`
	UID       string                  `json:"uid"`

	// Ownership
	Ownership Ownership `json:"ownership"`

	// Metadata
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	// State (extracted key fields)
	State map[string]interface{} `json:"state,omitempty"`

	// Timestamps
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	LastModified time.Time `json:"lastModified"`

	// Raw resource for detailed inspection
	Raw *unstructured.Unstructured `json:"-"`
}

// Ownership describes who/what manages a resource
type Ownership struct {
	// Type of owner: flux, argo, helm, terraform, confighub, k8s, unknown
	Type string `json:"type"`

	// SubType provides more detail: kustomization, helmrelease, application, etc.
	SubType string `json:"subType,omitempty"`

	// Name of the owner resource
	Name string `json:"name,omitempty"`

	// Namespace of the owner resource
	Namespace string `json:"namespace,omitempty"`
}

// Relation describes a relationship between two resources
type Relation struct {
	// From is the source resource ID
	From string `json:"from"`

	// To is the target resource ID
	To string `json:"to"`

	// Type of relation: owned-by, selects, mounts, references
	Type string `json:"type"`
}
