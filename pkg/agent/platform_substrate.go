// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PlatformSubstrateEvidence records explicit proof that one platform resource
// is implemented through another controller substrate. It is supporting
// evidence only: ownership detection still decides the top-level owner.
type PlatformSubstrateEvidence struct {
	Platform            string                  `json:"platform"`
	Substrate           string                  `json:"substrate"`
	Composite           string                  `json:"composite,omitempty"`
	Claim               *PlatformSubstrateClaim `json:"claim,omitempty"`
	CompositionResource string                  `json:"compositionResource,omitempty"`
	FieldManager        string                  `json:"fieldManager,omitempty"`
	Sources             []string                `json:"sources,omitempty"`
}

type PlatformSubstrateClaim struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (e PlatformSubstrateEvidence) Summary() string {
	parts := []string{}
	if e.Composite != "" {
		parts = append(parts, "composite="+e.Composite)
	}
	if e.Claim != nil && e.Claim.Name != "" {
		claim := e.Claim.Name
		if e.Claim.Namespace != "" {
			claim = e.Claim.Namespace + "/" + claim
		}
		parts = append(parts, "claim="+claim)
	}
	if e.CompositionResource != "" {
		parts = append(parts, "compositionResource="+e.CompositionResource)
	}
	if e.FieldManager != "" {
		parts = append(parts, "fieldManager="+e.FieldManager)
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s-on-%s evidence: %s.", titleToken(e.Platform), titleToken(e.Substrate), strings.Join(parts, ", "))
}

// BuildModelplaneCrossplaneEvidence extracts deterministic Modelplane-on-
// Crossplane substrate facts from explicit labels, annotations, and verified
// Crossplane field-manager strings. It never treats Crossplane evidence as
// ownership by itself; callers should pass the result of DetectOwnership.
func BuildModelplaneCrossplaneEvidence(resource *unstructured.Unstructured, ownership Ownership) (*PlatformSubstrateEvidence, bool) {
	if resource == nil || ownership.Type != OwnerModelplane {
		return nil, false
	}

	labels := resource.GetLabels()
	annotations := resource.GetAnnotations()
	evidence := &PlatformSubstrateEvidence{
		Platform:  OwnerModelplane,
		Substrate: OwnerCrossplane,
	}

	if value, source := firstLabeledValue(labels, "crossplane.io/composite", "apiextensions.crossplane.io/composite"); value != "" {
		evidence.Composite = value
		evidence.Sources = append(evidence.Sources, source)
	}
	if claimName := strings.TrimSpace(labels["crossplane.io/claim-name"]); claimName != "" {
		evidence.Claim = &PlatformSubstrateClaim{Name: claimName}
		evidence.Sources = append(evidence.Sources, "label:crossplane.io/claim-name")
		if claimNamespace := strings.TrimSpace(labels["crossplane.io/claim-namespace"]); claimNamespace != "" {
			evidence.Claim.Namespace = claimNamespace
			evidence.Sources = append(evidence.Sources, "label:crossplane.io/claim-namespace")
		}
	}
	if value, source := firstMetadataValue(
		labels,
		annotations,
		"crossplane.io/composition-resource-name",
	); value != "" {
		evidence.CompositionResource = value
		evidence.Sources = append(evidence.Sources, source)
	}
	if manager := firstModelplaneCrossplaneManager(resource, ownership); manager != "" {
		evidence.FieldManager = manager
		evidence.Sources = append(evidence.Sources, "managedFields:"+manager)
	}

	if evidence.Composite == "" &&
		evidence.Claim == nil &&
		evidence.CompositionResource == "" &&
		evidence.FieldManager == "" {
		return nil, false
	}

	return evidence, true
}

func firstLabeledValue(labels map[string]string, keys ...string) (string, string) {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value, "label:" + key
		}
	}
	return "", ""
}

func firstMetadataValue(labels, annotations map[string]string, key string) (string, string) {
	if value := strings.TrimSpace(labels[key]); value != "" {
		return value, "label:" + key
	}
	if value := strings.TrimSpace(annotations[key]); value != "" {
		return value, "annotation:" + key
	}
	return "", ""
}

func firstModelplaneCrossplaneManager(resource *unstructured.Unstructured, ownership Ownership) string {
	if resource == nil {
		return ""
	}
	managers := []string{}
	for _, field := range resource.GetManagedFields() {
		manager := strings.TrimSpace(field.Manager)
		if manager == "" || !strings.Contains(manager, "crossplane.io") {
			continue
		}
		if IsControllerManagerFor(manager, OwnerModelplane, ownership.SubType) {
			managers = append(managers, manager)
		}
	}
	sort.Strings(managers)
	if len(managers) == 0 {
		return ""
	}
	return managers[0]
}

func titleToken(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OwnerModelplane:
		return "Modelplane"
	case OwnerCrossplane:
		return "Crossplane"
	default:
		if value == "" {
			return ""
		}
		return strings.ToUpper(value[:1]) + value[1:]
	}
}
