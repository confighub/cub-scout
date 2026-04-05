// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// SecretStatus represents the resolution status of a referenced secret.
type SecretStatus string

const (
	// SecretStatusPresent indicates the secret exists and is readable.
	SecretStatusPresent SecretStatus = "present"

	// SecretStatusMissing indicates the secret does not exist.
	SecretStatusMissing SecretStatus = "missing"

	// SecretStatusUnreadable indicates the secret exists but cannot be read (RBAC denied).
	SecretStatusUnreadable SecretStatus = "unreadable"

	// SecretStatusUnresolved indicates the reference could not be resolved (e.g., invalid ref).
	SecretStatusUnresolved SecretStatus = "unresolved"
)

// SecretRefType describes how a secret is referenced.
type SecretRefType string

const (
	SecretRefTypeEnvFrom           SecretRefType = "envFrom.secretRef"
	SecretRefTypeEnvValueFrom      SecretRefType = "env.valueFrom.secretKeyRef"
	SecretRefTypeVolume            SecretRefType = "volume.secret"
	SecretRefTypeVolumeProjected   SecretRefType = "volume.projected.secret"
	SecretRefTypeImagePullSecret   SecretRefType = "imagePullSecrets"
	SecretRefTypeServiceAccount    SecretRefType = "serviceAccount.imagePullSecrets"
	SecretRefTypeFluxSecretRef     SecretRefType = "spec.secretRef"
	SecretRefTypeCrossplaneCredRef SecretRefType = "spec.credentials.secretRef"
)

// SecretEvidence represents evidence about a referenced secret.
// IMPORTANT: This struct intentionally excludes secret data/values.
// Only safe metadata is exposed.
type SecretEvidence struct {
	// Name is the secret name.
	Name string `json:"name"`

	// Namespace is the secret namespace.
	Namespace string `json:"namespace"`

	// RefType describes how the secret is referenced.
	RefType SecretRefType `json:"refType"`

	// RefPath is the specific path where this reference was found (e.g., "containers[0].envFrom[0]").
	RefPath string `json:"refPath,omitempty"`

	// Status is the resolution status (present, missing, unreadable, unresolved).
	Status SecretStatus `json:"status"`

	// StatusReason provides additional context for the status.
	StatusReason string `json:"statusReason,omitempty"`

	// SecretType is the Kubernetes secret type (e.g., "Opaque", "kubernetes.io/tls").
	// Only populated when Status is "present".
	SecretType string `json:"secretType,omitempty"`

	// CreatedAt is when the secret was created.
	// Only populated when Status is "present".
	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// Owner describes the ownership of the secret (if detected).
	// Only populated when Status is "present".
	Owner *Ownership `json:"owner,omitempty"`

	// Optional indicates if the reference is marked as optional.
	Optional bool `json:"optional,omitempty"`
}

// SecretEvidenceResult contains all secret evidence for a resource.
type SecretEvidenceResult struct {
	// Resource identifies the resource these secrets belong to.
	Resource ResourceRef `json:"resource"`

	// Secrets is the list of secret references and their evidence.
	Secrets []SecretEvidence `json:"secrets"`

	// Summary provides counts by status.
	Summary SecretEvidenceSummary `json:"summary"`
}

// SecretEvidenceSummary summarizes secret evidence by status.
type SecretEvidenceSummary struct {
	Total      int `json:"total"`
	Present    int `json:"present"`
	Missing    int `json:"missing"`
	Unreadable int `json:"unreadable"`
	Unresolved int `json:"unresolved"`
}

// SecretEvidenceCollector collects and resolves secret evidence from resources.
type SecretEvidenceCollector struct {
	client dynamic.Interface
}

// NewSecretEvidenceCollector creates a new secret evidence collector.
func NewSecretEvidenceCollector(client dynamic.Interface) *SecretEvidenceCollector {
	return &SecretEvidenceCollector{client: client}
}

// CollectFromResource extracts and resolves secret references from a resource.
func (c *SecretEvidenceCollector) CollectFromResource(ctx context.Context, resource *unstructured.Unstructured) (*SecretEvidenceResult, error) {
	if resource == nil {
		return nil, fmt.Errorf("resource is nil")
	}

	kind := resource.GetKind()
	namespace := resource.GetNamespace()
	name := resource.GetName()

	result := &SecretEvidenceResult{
		Resource: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Secrets: []SecretEvidence{},
	}

	// Extract secret references based on resource kind
	var refs []secretReference
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		refs = c.extractWorkloadSecretRefs(resource)
	case "Pod":
		refs = c.extractPodSecretRefs(resource)
	case "GitRepository", "HelmRepository", "Bucket":
		refs = c.extractFluxSourceSecretRefs(resource)
	case "Kustomization":
		refs = c.extractFluxKustomizationSecretRefs(resource)
	case "HelmRelease":
		refs = c.extractFluxHelmReleaseSecretRefs(resource)
	case "ProviderConfig":
		refs = c.extractCrossplaneSecretRefs(resource)
	default:
		// Unknown kind - no secret extraction
		return result, nil
	}

	// Resolve each reference
	for _, ref := range refs {
		evidence := c.resolveSecretRef(ctx, ref, namespace)
		result.Secrets = append(result.Secrets, evidence)
	}

	// Calculate summary
	result.Summary = c.calculateSummary(result.Secrets)

	return result, nil
}

// secretReference is an internal struct for tracking extracted references.
type secretReference struct {
	name      string
	namespace string
	refType   SecretRefType
	refPath   string
	optional  bool
}

// extractWorkloadSecretRefs extracts secret references from a workload spec.
func (col *SecretEvidenceCollector) extractWorkloadSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference
	seen := make(map[string]bool)

	// Get pod template spec
	template, found, _ := unstructured.NestedMap(resource.Object, "spec", "template", "spec")
	if !found {
		return refs
	}

	// Extract from containers
	containers, _, _ := unstructured.NestedSlice(template, "containers")
	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, _ := container["name"].(string)
		prefix := fmt.Sprintf("containers[%d]", i)
		if containerName != "" {
			prefix = fmt.Sprintf("containers[%s]", containerName)
		}
		refs = append(refs, col.extractContainerSecretRefs(container, prefix, seen)...)
	}

	// Extract from init containers
	initContainers, _, _ := unstructured.NestedSlice(template, "initContainers")
	for i, ic := range initContainers {
		container, ok := ic.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, _ := container["name"].(string)
		prefix := fmt.Sprintf("initContainers[%d]", i)
		if containerName != "" {
			prefix = fmt.Sprintf("initContainers[%s]", containerName)
		}
		refs = append(refs, col.extractContainerSecretRefs(container, prefix, seen)...)
	}

	// Extract from volumes
	volumes, _, _ := unstructured.NestedSlice(template, "volumes")
	refs = append(refs, col.extractVolumeSecretRefs(volumes, seen)...)

	// Extract imagePullSecrets
	imagePullSecrets, _, _ := unstructured.NestedSlice(template, "imagePullSecrets")
	refs = append(refs, col.extractImagePullSecretRefs(imagePullSecrets, seen)...)

	return refs
}

// extractPodSecretRefs extracts secret references from a Pod spec.
func (col *SecretEvidenceCollector) extractPodSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference
	seen := make(map[string]bool)

	spec, found, _ := unstructured.NestedMap(resource.Object, "spec")
	if !found {
		return refs
	}

	// Extract from containers
	containers, _, _ := unstructured.NestedSlice(spec, "containers")
	for i, cont := range containers {
		container, ok := cont.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, _ := container["name"].(string)
		prefix := fmt.Sprintf("containers[%d]", i)
		if containerName != "" {
			prefix = fmt.Sprintf("containers[%s]", containerName)
		}
		refs = append(refs, col.extractContainerSecretRefs(container, prefix, seen)...)
	}

	// Extract from init containers
	initContainers, _, _ := unstructured.NestedSlice(spec, "initContainers")
	for i, ic := range initContainers {
		container, ok := ic.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, _ := container["name"].(string)
		prefix := fmt.Sprintf("initContainers[%d]", i)
		if containerName != "" {
			prefix = fmt.Sprintf("initContainers[%s]", containerName)
		}
		refs = append(refs, col.extractContainerSecretRefs(container, prefix, seen)...)
	}

	// Extract from volumes
	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")
	refs = append(refs, col.extractVolumeSecretRefs(volumes, seen)...)

	// Extract imagePullSecrets
	imagePullSecrets, _, _ := unstructured.NestedSlice(spec, "imagePullSecrets")
	refs = append(refs, col.extractImagePullSecretRefs(imagePullSecrets, seen)...)

	return refs
}

// extractContainerSecretRefs extracts secret references from a container spec.
func (c *SecretEvidenceCollector) extractContainerSecretRefs(container map[string]interface{}, pathPrefix string, seen map[string]bool) []secretReference {
	var refs []secretReference

	// Extract from envFrom
	envFrom, _, _ := unstructured.NestedSlice(container, "envFrom")
	for i, ef := range envFrom {
		envFromEntry, ok := ef.(map[string]interface{})
		if !ok {
			continue
		}

		if secretRef, found, _ := unstructured.NestedMap(envFromEntry, "secretRef"); found {
			if name, ok := secretRef["name"].(string); ok && name != "" {
				key := "envFrom:" + name
				if !seen[key] {
					seen[key] = true
					optional, _ := secretRef["optional"].(bool)
					refs = append(refs, secretReference{
						name:     name,
						refType:  SecretRefTypeEnvFrom,
						refPath:  fmt.Sprintf("%s.envFrom[%d].secretRef", pathPrefix, i),
						optional: optional,
					})
				}
			}
		}
	}

	// Extract from env[].valueFrom.secretKeyRef
	env, _, _ := unstructured.NestedSlice(container, "env")
	for i, e := range env {
		envEntry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		valueFrom, found, _ := unstructured.NestedMap(envEntry, "valueFrom")
		if !found {
			continue
		}

		if secretKeyRef, found, _ := unstructured.NestedMap(valueFrom, "secretKeyRef"); found {
			if name, ok := secretKeyRef["name"].(string); ok && name != "" {
				key := "secretKeyRef:" + name
				if !seen[key] {
					seen[key] = true
					optional, _ := secretKeyRef["optional"].(bool)
					envName, _ := envEntry["name"].(string)
					refs = append(refs, secretReference{
						name:     name,
						refType:  SecretRefTypeEnvValueFrom,
						refPath:  fmt.Sprintf("%s.env[%d:%s].valueFrom.secretKeyRef", pathPrefix, i, envName),
						optional: optional,
					})
				}
			}
		}
	}

	return refs
}

// extractVolumeSecretRefs extracts secret references from volumes.
func (c *SecretEvidenceCollector) extractVolumeSecretRefs(volumes []interface{}, seen map[string]bool) []secretReference {
	var refs []secretReference

	for i, v := range volumes {
		volume, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		volumeName, _ := volume["name"].(string)
		prefix := fmt.Sprintf("volumes[%d]", i)
		if volumeName != "" {
			prefix = fmt.Sprintf("volumes[%s]", volumeName)
		}

		// secret volume
		if secret, found, _ := unstructured.NestedMap(volume, "secret"); found {
			if name, ok := secret["secretName"].(string); ok && name != "" {
				key := "volume:" + name
				if !seen[key] {
					seen[key] = true
					optional, _ := secret["optional"].(bool)
					refs = append(refs, secretReference{
						name:     name,
						refType:  SecretRefTypeVolume,
						refPath:  fmt.Sprintf("%s.secret", prefix),
						optional: optional,
					})
				}
			}
		}

		// projected volumes with secrets
		if projected, found, _ := unstructured.NestedMap(volume, "projected"); found {
			sources, _, _ := unstructured.NestedSlice(projected, "sources")
			for j, s := range sources {
				source, ok := s.(map[string]interface{})
				if !ok {
					continue
				}

				if secret, found, _ := unstructured.NestedMap(source, "secret"); found {
					if name, ok := secret["name"].(string); ok && name != "" {
						key := "projected:" + name
						if !seen[key] {
							seen[key] = true
							optional, _ := secret["optional"].(bool)
							refs = append(refs, secretReference{
								name:     name,
								refType:  SecretRefTypeVolumeProjected,
								refPath:  fmt.Sprintf("%s.projected.sources[%d].secret", prefix, j),
								optional: optional,
							})
						}
					}
				}
			}
		}
	}

	return refs
}

// extractImagePullSecretRefs extracts imagePullSecrets references.
func (c *SecretEvidenceCollector) extractImagePullSecretRefs(imagePullSecrets []interface{}, seen map[string]bool) []secretReference {
	var refs []secretReference

	for i, ips := range imagePullSecrets {
		ipsMap, ok := ips.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := ipsMap["name"].(string); ok && name != "" {
			key := "imagePullSecret:" + name
			if !seen[key] {
				seen[key] = true
				refs = append(refs, secretReference{
					name:    name,
					refType: SecretRefTypeImagePullSecret,
					refPath: fmt.Sprintf("imagePullSecrets[%d]", i),
				})
			}
		}
	}

	return refs
}

// extractFluxSourceSecretRefs extracts spec.secretRef from Flux source objects.
func (c *SecretEvidenceCollector) extractFluxSourceSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference

	secretRef, found, _ := unstructured.NestedMap(resource.Object, "spec", "secretRef")
	if !found {
		return refs
	}

	if name, ok := secretRef["name"].(string); ok && name != "" {
		refs = append(refs, secretReference{
			name:    name,
			refType: SecretRefTypeFluxSecretRef,
			refPath: "spec.secretRef",
		})
	}

	return refs
}

// extractFluxKustomizationSecretRefs extracts secret refs from Flux Kustomization.
func (c *SecretEvidenceCollector) extractFluxKustomizationSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference

	// decryption.secretRef
	decryptionSecretRef, found, _ := unstructured.NestedMap(resource.Object, "spec", "decryption", "secretRef")
	if found {
		if name, ok := decryptionSecretRef["name"].(string); ok && name != "" {
			refs = append(refs, secretReference{
				name:    name,
				refType: SecretRefTypeFluxSecretRef,
				refPath: "spec.decryption.secretRef",
			})
		}
	}

	// kubeConfig.secretRef
	kubeConfigSecretRef, found, _ := unstructured.NestedMap(resource.Object, "spec", "kubeConfig", "secretRef")
	if found {
		if name, ok := kubeConfigSecretRef["name"].(string); ok && name != "" {
			refs = append(refs, secretReference{
				name:    name,
				refType: SecretRefTypeFluxSecretRef,
				refPath: "spec.kubeConfig.secretRef",
			})
		}
	}

	return refs
}

// extractFluxHelmReleaseSecretRefs extracts secret refs from Flux HelmRelease.
func (c *SecretEvidenceCollector) extractFluxHelmReleaseSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference

	// kubeConfig.secretRef
	kubeConfigSecretRef, found, _ := unstructured.NestedMap(resource.Object, "spec", "kubeConfig", "secretRef")
	if found {
		if name, ok := kubeConfigSecretRef["name"].(string); ok && name != "" {
			refs = append(refs, secretReference{
				name:    name,
				refType: SecretRefTypeFluxSecretRef,
				refPath: "spec.kubeConfig.secretRef",
			})
		}
	}

	// valuesFrom secrets
	valuesFrom, found, _ := unstructured.NestedSlice(resource.Object, "spec", "valuesFrom")
	if found {
		for i, vf := range valuesFrom {
			vfMap, ok := vf.(map[string]interface{})
			if !ok {
				continue
			}
			kind, _ := vfMap["kind"].(string)
			if kind != "Secret" {
				continue
			}
			if name, ok := vfMap["name"].(string); ok && name != "" {
				optional, _ := vfMap["optional"].(bool)
				refs = append(refs, secretReference{
					name:     name,
					refType:  SecretRefTypeFluxSecretRef,
					refPath:  fmt.Sprintf("spec.valuesFrom[%d]", i),
					optional: optional,
				})
			}
		}
	}

	return refs
}

// extractCrossplaneSecretRefs extracts credential secret refs from Crossplane ProviderConfig.
func (c *SecretEvidenceCollector) extractCrossplaneSecretRefs(resource *unstructured.Unstructured) []secretReference {
	var refs []secretReference

	// credentials.secretRef
	credSecretRef, found, _ := unstructured.NestedMap(resource.Object, "spec", "credentials", "secretRef")
	if found {
		if name, ok := credSecretRef["name"].(string); ok && name != "" {
			namespace, _ := credSecretRef["namespace"].(string)
			ref := secretReference{
				name:      name,
				namespace: strings.TrimSpace(namespace),
				refType:   SecretRefTypeCrossplaneCredRef,
				refPath:   "spec.credentials.secretRef",
			}
			refs = append(refs, ref)
		}
	}

	return refs
}

// resolveSecretRef resolves a secret reference and returns evidence.
func (c *SecretEvidenceCollector) resolveSecretRef(ctx context.Context, ref secretReference, namespace string) SecretEvidence {
	evidence := SecretEvidence{
		Name:      ref.name,
		RefType:   ref.refType,
		RefPath:   ref.refPath,
		Optional:  ref.optional,
		Namespace: strings.TrimSpace(ref.namespace),
	}

	if evidence.Namespace == "" {
		evidence.Namespace = strings.TrimSpace(namespace)
	}

	// Default namespace
	if evidence.Namespace == "" {
		evidence.Namespace = "default"
	}

	// Try to fetch the secret
	secretNamespace := evidence.Namespace
	secretGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	secret, err := c.client.Resource(secretGVR).Namespace(secretNamespace).Get(ctx, ref.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			evidence.Status = SecretStatusMissing
			evidence.StatusReason = "secret not found in namespace"
		} else if apierrors.IsForbidden(err) {
			evidence.Status = SecretStatusUnreadable
			evidence.StatusReason = "RBAC denied: cannot read secrets in namespace"
		} else {
			evidence.Status = SecretStatusUnresolved
			evidence.StatusReason = fmt.Sprintf("error fetching secret: %v", err)
		}
		return evidence
	}

	// Secret exists and is readable
	evidence.Status = SecretStatusPresent

	// Extract safe metadata only (NEVER expose .data or .stringData)
	if secretType, ok := secret.Object["type"].(string); ok {
		evidence.SecretType = secretType
	}

	creationTime := secret.GetCreationTimestamp()
	if !creationTime.IsZero() {
		t := creationTime.Time
		evidence.CreatedAt = &t
	}

	// Detect ownership (only include if managed by something other than native K8s)
	ownership := DetectOwnership(secret)
	if ownership.Type != "" && ownership.Type != OwnerUnknown && ownership.Type != OwnerKubernetes {
		evidence.Owner = &ownership
	}

	return evidence
}

// calculateSummary calculates summary counts from evidence list.
func (c *SecretEvidenceCollector) calculateSummary(secrets []SecretEvidence) SecretEvidenceSummary {
	summary := SecretEvidenceSummary{
		Total: len(secrets),
	}

	for _, s := range secrets {
		switch s.Status {
		case SecretStatusPresent:
			summary.Present++
		case SecretStatusMissing:
			summary.Missing++
		case SecretStatusUnreadable:
			summary.Unreadable++
		case SecretStatusUnresolved:
			summary.Unresolved++
		}
	}

	return summary
}

// HasIssues returns true if any secrets are missing, unreadable, or unresolved.
func (r *SecretEvidenceResult) HasIssues() bool {
	return r.Summary.Missing > 0 || r.Summary.Unreadable > 0 || r.Summary.Unresolved > 0
}

// IssueCount returns the total number of problematic secret references.
func (r *SecretEvidenceResult) IssueCount() int {
	return r.Summary.Missing + r.Summary.Unreadable + r.Summary.Unresolved
}

// String returns a human-readable summary of the secret evidence.
func (r *SecretEvidenceResult) String() string {
	if r.Summary.Total == 0 {
		return "no secret references"
	}

	var parts []string
	if r.Summary.Present > 0 {
		parts = append(parts, fmt.Sprintf("%d present", r.Summary.Present))
	}
	if r.Summary.Missing > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", r.Summary.Missing))
	}
	if r.Summary.Unreadable > 0 {
		parts = append(parts, fmt.Sprintf("%d unreadable", r.Summary.Unreadable))
	}
	if r.Summary.Unresolved > 0 {
		parts = append(parts, fmt.Sprintf("%d unresolved", r.Summary.Unresolved))
	}

	return fmt.Sprintf("%d secrets (%s)", r.Summary.Total, strings.Join(parts, ", "))
}
