// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HelmTracer implements Tracer for standalone Helm releases
// (not managed by Flux HelmRelease)
type HelmTracer struct {
	client kubernetes.Interface
}

// Helm release Secrets are untrusted compressed input. Keep each decoding
// stage bounded even though the Kubernetes API has already returned the Secret.
// These limits intentionally leave room above ordinary release records while
// bounding a single candidate to 32 MiB of decoded JSON.
const (
	maxHelmReleaseEncodedBytes    = 9 << 20
	maxHelmReleaseCompressedBytes = 6 << 20
	maxHelmReleaseJSONBytes       = 32 << 20
	helmReleaseSecretPrefix       = "sh.helm.release.v1."
	helmReleaseSecretType         = corev1.SecretType("helm.sh/release.v1")
)

// NewHelmTracer creates a new Helm tracer
func NewHelmTracer(client kubernetes.Interface) *HelmTracer {
	return &HelmTracer{
		client: client,
	}
}

// ToolName returns "helm"
func (h *HelmTracer) ToolName() string {
	return "helm"
}

// Available checks if we can trace Helm releases (always true if we have a k8s client)
func (h *HelmTracer) Available() bool {
	return h.client != nil
}

// Trace finds the Helm release that manages a resource and builds the ownership chain
func (h *HelmTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("Helm resource identity is incomplete")
	}
	if err := requireHelmNamespace(namespace); err != nil {
		return nil, err
	}

	// Find the Helm release in the namespace
	releases, err := h.listReleases(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("list helm releases: %w", err)
	}

	// Match structured manifest identities. A partial or ambiguous match must
	// never be promoted to an ownership claim.
	var matchedRelease *helmRelease
	for _, rel := range releases {
		matched, err := releaseManifestMatchesResource(rel, kind, name, namespace)
		if err != nil {
			return nil, fmt.Errorf("cannot establish Helm manifest identity: %w", err)
		}
		if matched && matchedRelease != nil {
			return nil, fmt.Errorf("multiple Helm releases contain the requested manifest identity")
		}
		if matched {
			matchedRelease = rel
		}
	}

	if matchedRelease == nil {
		return &TraceResult{
			Object: ResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "helm",
			TracedAt:     time.Now(),
			Error:        "no Helm release found managing this resource among Secrets returned by the owner=helm query",
		}, nil
	}

	return h.buildTraceResult(matchedRelease, kind, name, namespace)
}

// TraceRelease traces a Helm release by name
func (h *HelmTracer) TraceRelease(ctx context.Context, releaseName, namespace string) (*TraceResult, error) {
	if err := requireHelmNamespace(namespace); err != nil {
		return nil, err
	}
	release, err := h.getRelease(ctx, releaseName, namespace)
	if err != nil {
		return nil, err
	}

	if release == nil {
		return &TraceResult{
			Object: ResourceRef{
				Kind:      "Release",
				Name:      releaseName,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "helm",
			TracedAt:     time.Now(),
			Error:        fmt.Sprintf("Helm release '%s' not found among Secrets returned by owner=helm in namespace '%s'", releaseName, namespace),
		}, nil
	}

	return h.buildTraceResult(release, "Release", releaseName, namespace)
}

// helmRelease represents a Helm release stored in a Kubernetes secret
type helmRelease struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Version   int               `json:"version"`
	Info      helmReleaseInfo   `json:"info"`
	Chart     helmChart         `json:"chart"`
	Config    map[string]any    `json:"config"`
	Manifest  string            `json:"manifest"`
	Labels    map[string]string `json:"labels"`
}

type helmReleaseInfo struct {
	FirstDeployed time.Time `json:"first_deployed"`
	LastDeployed  time.Time `json:"last_deployed"`
	Deleted       time.Time `json:"deleted"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
}

type helmChart struct {
	Metadata helmChartMetadata `json:"metadata"`
}

type helmChartMetadata struct {
	Name        string   `json:"name"`
	Home        string   `json:"home"`
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}

// listReleases finds all Helm releases in a namespace
func (h *HelmTracer) listReleases(ctx context.Context, namespace string) ([]*helmRelease, error) {
	if err := requireHelmNamespace(namespace); err != nil {
		return nil, err
	}
	// Helm stores releases in secrets with owner=helm label
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		return nil, err
	}

	var candidates []helmReleaseCandidate

	for _, secret := range secrets.Items {
		release, err := h.decodeReleaseSecret(secret, namespace)
		if err != nil {
			return nil, err
		}

		// Validate every returned candidate before selecting a latest revision.
		candidates = append(candidates, newHelmReleaseCandidate(secret, release))
	}
	unique, err := dedupeHelmReleaseCandidates(candidates)
	if err != nil {
		return nil, err
	}
	releaseMap := make(map[string]*helmRelease)
	for _, candidate := range unique {
		release := candidate.release
		if existing, ok := releaseMap[release.Name]; !ok || release.Version > existing.Version {
			releaseMap[release.Name] = release
		}
	}
	var releases []*helmRelease

	for _, rel := range releaseMap {
		releases = append(releases, rel)
	}

	// Sort by name for consistent output
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})

	return releases, nil
}

// getRelease gets a specific Helm release by name
func (h *HelmTracer) getRelease(ctx context.Context, name, namespace string) (*helmRelease, error) {
	if err := requireHelmNamespace(namespace); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("Helm release name is unresolved")
	}
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		return nil, err
	}

	var candidates []helmReleaseCandidate
	for _, secret := range secrets.Items {
		release, err := h.decodeReleaseSecret(secret, namespace)
		if err != nil {
			return nil, err
		}
		if release.Name != name {
			continue
		}

		candidates = append(candidates, newHelmReleaseCandidate(secret, release))
	}

	unique, err := dedupeHelmReleaseCandidates(candidates)
	if err != nil {
		return nil, err
	}
	var latestRelease *helmRelease
	for _, candidate := range unique {
		release := candidate.release
		if latestRelease == nil || release.Version > latestRelease.Version {
			latestRelease = release
		}
	}
	return latestRelease, nil
}

// decodeReleaseSecret verifies the Helm storage driver's redundant identity
// fields before the decoded payload can influence release selection. Helm's
// v3.17.3 and v4.0.0 Secret drivers use this key, type, and label shape.
func (h *HelmTracer) decodeReleaseSecret(secret corev1.Secret, expectedNamespace string) (*helmRelease, error) {
	release, err := h.decodeRelease(secret.Data["release"])
	if err != nil {
		return nil, fmt.Errorf("decode candidate Helm release Secret %q: %w", secret.Name, err)
	}
	expectedName := fmt.Sprintf("%s%s.v%d", helmReleaseSecretPrefix, release.Name, release.Version)
	if expectedNamespace == "" || secret.Namespace != expectedNamespace || release.Namespace != expectedNamespace ||
		secret.Name != expectedName || secret.Type != helmReleaseSecretType ||
		secret.Labels["owner"] != "helm" || secret.Labels["name"] != release.Name ||
		secret.Labels["version"] != strconv.Itoa(release.Version) {
		return nil, inconsistentHelmReleaseIdentity(secret.Name)
	}
	return release, nil
}

func inconsistentHelmReleaseIdentity(secretName string) error {
	return fmt.Errorf("Helm release Secret %q has incomplete or inconsistent identity metadata", secretName)
}

func ambiguousHelmReleaseCandidates() error {
	return fmt.Errorf("ambiguous Helm release Secret candidates at the same revision")
}

// decodeRelease decodes a Helm release from the secret data
// Helm stores releases as base64(gzip(json))
func (h *HelmTracer) decodeRelease(data []byte) (*helmRelease, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty Helm release data")
	}
	if len(data) > maxHelmReleaseEncodedBytes {
		return nil, fmt.Errorf("encoded Helm release exceeds size limit (%d bytes)", maxHelmReleaseEncodedBytes)
	}
	if base64.StdEncoding.DecodedLen(len(data)) > maxHelmReleaseCompressedBytes {
		return nil, fmt.Errorf("compressed Helm release exceeds size limit (%d bytes)", maxHelmReleaseCompressedBytes)
	}

	// Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid base64 data in Helm release")
	}
	if len(decoded) > maxHelmReleaseCompressedBytes {
		return nil, fmt.Errorf("compressed Helm release exceeds size limit (%d bytes)", maxHelmReleaseCompressedBytes)
	}

	// Gzip decompress
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip data in Helm release")
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(io.LimitReader(reader, maxHelmReleaseJSONBytes+1))
	if err != nil {
		return nil, fmt.Errorf("unable to decompress Helm release data")
	}
	if len(decompressed) > maxHelmReleaseJSONBytes {
		return nil, fmt.Errorf("expanded Helm release exceeds size limit (%d bytes)", maxHelmReleaseJSONBytes)
	}

	// JSON unmarshal
	var release helmRelease
	if err := json.Unmarshal(decompressed, &release); err != nil {
		return nil, fmt.Errorf("invalid JSON in Helm release data")
	}
	if strings.TrimSpace(release.Name) == "" || strings.TrimSpace(release.Namespace) == "" || release.Version <= 0 {
		return nil, fmt.Errorf("Helm release identity metadata is incomplete")
	}

	return &release, nil
}

type helmManifestIdentity struct {
	apiVersion       string
	kind             string
	name             string
	namespace        string
	namespacePresent bool
}

// releaseManifestMatchesResource compares parsed manifest metadata rather than
// YAML substrings. Trace currently receives no apiVersion, so all candidate
// documents at that identity are counted and duplicates remain ambiguous.
func releaseManifestMatchesResource(release *helmRelease, kind, name, namespace string) (bool, error) {
	identities, err := parseHelmManifestIdentities(release.Manifest)
	if err != nil {
		return false, err
	}

	matches := 0
	matchedAPIVersions := make(map[string]struct{})
	for _, identity := range identities {
		if identity.kind != kind || identity.name != name {
			continue
		}
		if !identity.namespacePresent || strings.TrimSpace(identity.namespace) == "" {
			return false, fmt.Errorf("manifest namespace is absent for the requested kind and name")
		}
		if identity.namespace == namespace {
			matches++
			matchedAPIVersions[identity.apiVersion] = struct{}{}
		}
	}
	if matches > 1 {
		if len(matchedAPIVersions) > 1 {
			return false, fmt.Errorf("multiple manifest API versions match the requested identity; Trace has no apiVersion input")
		}
		return false, fmt.Errorf("multiple manifest documents match the requested identity")
	}
	return matches == 1, nil
}

// parseHelmManifestIdentities validates every nonempty document before any
// absence claim. It retains only the fields needed for identity matching and
// reports generic errors so stored manifest content is never echoed.
func parseHelmManifestIdentities(manifest string) ([]helmManifestIdentity, error) {
	decoder := yaml.NewDecoder(strings.NewReader(manifest))
	var identities []helmManifestIdentity
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("malformed Helm manifest document")
		}
		if len(document.Content) == 0 {
			continue
		}
		root := document.Content[0]
		if root.Kind == yaml.ScalarNode && root.Tag == "!!null" && root.Value == "" {
			continue
		}
		if root.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("unsupported non-object Helm manifest document")
		}
		apiVersion, _, err := helmYAMLStringField(root, "apiVersion", true)
		if err != nil || strings.TrimSpace(apiVersion) == "" {
			return nil, fmt.Errorf("unsupported Helm manifest identity document")
		}
		kind, _, err := helmYAMLStringField(root, "kind", true)
		if err != nil || strings.TrimSpace(kind) == "" {
			return nil, fmt.Errorf("unsupported Helm manifest identity document")
		}
		if kind == "List" {
			return nil, fmt.Errorf("unsupported Kubernetes List manifest document")
		}
		metadata, present, err := helmYAMLMappingField(root, "metadata")
		if err != nil || !present {
			return nil, fmt.Errorf("unsupported Helm manifest identity document")
		}
		name, _, err := helmYAMLStringField(metadata, "name", true)
		if err != nil || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("unsupported Helm manifest identity document")
		}
		namespace, namespacePresent, err := helmYAMLStringField(metadata, "namespace", false)
		if err != nil {
			return nil, fmt.Errorf("unsupported Helm manifest identity document")
		}
		identity := helmManifestIdentity{
			apiVersion:       apiVersion,
			kind:             kind,
			name:             name,
			namespace:        namespace,
			namespacePresent: namespacePresent,
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

func helmYAMLMappingField(mapping *yaml.Node, field string) (*yaml.Node, bool, error) {
	if mapping.Kind != yaml.MappingNode || len(mapping.Content)%2 != 0 {
		return nil, false, fmt.Errorf("invalid mapping")
	}
	var found *yaml.Node
	seen := make(map[string]struct{}, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, false, fmt.Errorf("mapping key is not a string")
		}
		if _, exists := seen[key.Value]; exists {
			return nil, false, fmt.Errorf("duplicate mapping key")
		}
		seen[key.Value] = struct{}{}
		if key.Value == field {
			found = mapping.Content[i+1]
		}
	}
	return found, found != nil, nil
}

func helmYAMLStringField(mapping *yaml.Node, field string, required bool) (string, bool, error) {
	value, present, err := helmYAMLMappingField(mapping, field)
	if err != nil {
		return "", false, err
	}
	if !present {
		if required {
			return "", false, fmt.Errorf("required field absent")
		}
		return "", false, nil
	}
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
		return "", false, fmt.Errorf("field is not a string scalar")
	}
	return value.Value, true, nil
}

// buildTraceResult builds a TraceResult from a Helm release
func (h *HelmTracer) buildTraceResult(release *helmRelease, kind, name, namespace string) (*TraceResult, error) {
	result := &TraceResult{
		Object: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Chain:        []ChainLink{},
		FullyManaged: true,
		Tool:         "helm",
		TracedAt:     time.Now(),
	}

	// Determine chart source URL if available
	chartURL := ""
	if len(release.Chart.Metadata.Sources) > 0 {
		chartURL = release.Chart.Metadata.Sources[0]
	}

	// Add chart as source link
	chartLink := ChainLink{
		Kind:     "HelmChart",
		Name:     release.Chart.Metadata.Name,
		Ready:    true,
		Status:   fmt.Sprintf("v%s", release.Chart.Metadata.Version),
		Revision: release.Chart.Metadata.Version,
		URL:      chartURL,
	}
	if release.Chart.Metadata.AppVersion != "" {
		chartLink.Status = fmt.Sprintf("v%s (app: %s)", release.Chart.Metadata.Version, release.Chart.Metadata.AppVersion)
	}
	result.Chain = append(result.Chain, chartLink)

	// Add release link
	releaseReady := release.Info.Status == "deployed"
	releaseLink := ChainLink{
		Kind:      "Release",
		Name:      release.Name,
		Namespace: release.Namespace,
		Ready:     releaseReady,
		Status:    release.Info.Status,
		Revision:  fmt.Sprintf("v%d", release.Version),
		Message:   release.Info.Description,
	}
	if !release.Info.LastDeployed.IsZero() {
		t := release.Info.LastDeployed
		releaseLink.LastTransitionTime = &t
	}
	result.Chain = append(result.Chain, releaseLink)

	if !releaseReady {
		result.FullyManaged = false
	}

	// Add the target resource link (only if not tracing the release itself)
	if kind == "Release" {
		// When tracing a release directly, don't add a redundant resource link
		return result, nil
	}

	resourceLink := ChainLink{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Ready:     releaseReady, // Inherit from release status
		Status:    "Managed by Helm",
	}
	result.Chain = append(result.Chain, resourceLink)

	return result, nil
}

// FormatHelmContextError detects cluster connectivity or permission errors
// during Helm tracing and returns a remediation-focused message. The bool
// return is true when a context issue was detected.
func FormatHelmContextError(output string) (string, bool) {
	out := strings.ToLower(output)

	containsAny := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(out, p) {
				return true
			}
		}
		return false
	}

	reason := ""
	switch {
	case containsAny(
		"connection refused",
		"i/o timeout",
		"dial tcp",
		"x509:",
		"certificate signed by unknown authority",
		"tls:",
		"no such host",
	):
		reason = "cluster endpoint is unreachable or certificate is invalid"
	case containsAny(
		"forbidden",
		"cannot list resource",
		"secrets is forbidden",
		"unauthorized",
	):
		reason = "insufficient permissions to read Helm release secrets"
	default:
		return "", false
	}

	msg := fmt.Sprintf(
		"helm trace context appears stale or invalid (%s).\n\n"+
			"Trace context troubleshooting:\n"+
			"  1) kubectl cluster-info\n"+
			"  2) kubectl auth can-i list secrets -n <namespace>\n"+
			"  3) kubectl config use-context <context>\n"+
			"  4) helm list -n <namespace>\n\n"+
			"Then retry:\n"+
			"  cub-scout trace <kind>/<name> -n <namespace>",
		reason,
	)
	return msg, true
}

// TraceByOwnership traces a resource by its Helm ownership labels
func (h *HelmTracer) TraceByOwnership(ctx context.Context, ownership Ownership) (*TraceResult, error) {
	if ownership.Type != OwnerHelm {
		return nil, fmt.Errorf("resource not owned by Helm")
	}

	// The ownership.Name is the release name
	return h.TraceRelease(ctx, ownership.Name, ownership.Namespace)
}

// GetReleaseHistory returns the deployment history for a Helm release
// History is returned sorted by version descending (most recent first)
func (h *HelmTracer) GetReleaseHistory(ctx context.Context, releaseName, namespace string) ([]HistoryEntry, error) {
	if err := requireHelmNamespace(namespace); err != nil {
		return nil, err
	}
	if strings.TrimSpace(releaseName) == "" {
		return nil, fmt.Errorf("Helm release name is unresolved")
	}
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		return nil, err
	}

	var releases []helmReleaseCandidate
	for _, secret := range secrets.Items {
		release, err := h.decodeReleaseSecret(secret, namespace)
		if err != nil {
			return nil, err
		}
		if release.Name != releaseName {
			continue
		}

		releases = append(releases, newHelmReleaseCandidate(secret, release))
	}

	if len(releases) == 0 {
		return nil, nil
	}

	unique, err := dedupeHelmReleaseCandidates(releases)
	if err != nil {
		return nil, err
	}
	releases = unique

	// Sort by version descending (most recent first)
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].release.Version > releases[j].release.Version
	})

	// Convert to HistoryEntry
	history := make([]HistoryEntry, 0, len(releases))
	for _, candidate := range releases {
		rel := candidate.release
		entry := HistoryEntry{
			Timestamp: rel.Info.LastDeployed,
			Revision:  fmt.Sprintf("v%d", rel.Version),
			Status:    rel.Info.Status,
			Message:   rel.Info.Description,
			Source:    fmt.Sprintf("chart %s-%s", rel.Chart.Metadata.Name, rel.Chart.Metadata.Version),
		}
		history = append(history, entry)
	}

	return history, nil
}

func requireHelmNamespace(namespace string) error {
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("Helm resource namespace is unresolved; refusing to infer namespace or scope")
	}
	return nil
}

// Keep the exact stored release bytes alongside the decoded projection. The
// projection intentionally ignores parts of Helm's payload, so it cannot
// prove that two records with equal projections are actually identical.
type helmReleaseCandidate struct {
	release         *helmRelease
	secretName      string
	secretNamespace string
	secretType      corev1.SecretType
	secretUID       string
	resourceVersion string
	labels          map[string]string
	releaseData     []byte
}

func newHelmReleaseCandidate(secret corev1.Secret, release *helmRelease) helmReleaseCandidate {
	labels := make(map[string]string, len(secret.Labels))
	for key, value := range secret.Labels {
		labels[key] = value
	}
	return helmReleaseCandidate{
		release:         release,
		secretName:      secret.Name,
		secretNamespace: secret.Namespace,
		secretType:      secret.Type,
		secretUID:       string(secret.UID),
		resourceVersion: secret.ResourceVersion,
		labels:          labels,
		releaseData:     secret.Data["release"],
	}
}

func sameHelmReleaseEvidence(a, b helmReleaseCandidate) bool {
	return a.secretName == b.secretName && a.secretNamespace == b.secretNamespace &&
		a.secretType == b.secretType && a.secretUID == b.secretUID &&
		a.resourceVersion == b.resourceVersion && reflect.DeepEqual(a.labels, b.labels) &&
		bytes.Equal(a.releaseData, b.releaseData)
}

func dedupeHelmReleaseCandidates(releases []helmReleaseCandidate) ([]helmReleaseCandidate, error) {
	byRevision := make(map[string]helmReleaseCandidate, len(releases))
	unique := make([]helmReleaseCandidate, 0, len(releases))
	for _, candidate := range releases {
		release := candidate.release
		key := fmt.Sprintf("%s\x00%d", release.Name, release.Version)
		if existing, ok := byRevision[key]; ok {
			if !sameHelmReleaseEvidence(existing, candidate) {
				return nil, ambiguousHelmReleaseCandidates()
			}
			continue
		}
		byRevision[key] = candidate
		unique = append(unique, candidate)
	}
	return unique, nil
}
