// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const recordedMapSchema = "map-list-recorded.v1"
const recordedMapSummarySchema = "map-list-recorded-summary.v1"

// RecordedMapScope selects objects from an immutable recording. A nil
// Namespace means any namespace; a non-nil pointer selects that exact value,
// including the empty namespace. APIVersion, Kind, and Namespace matching are
// exact and case-sensitive. NamespacePrefix is a literal, case-sensitive
// starts-with filter and cannot be combined with Namespace.
type RecordedMapScope struct {
	APIVersion      string  `json:"apiVersion"`
	Kind            string  `json:"kind"`
	Namespace       *string `json:"namespace,omitempty"`
	NamespacePrefix string  `json:"namespacePrefix,omitempty"`
	Owner           string  `json:"owner,omitempty"`
}

// RecordedMapProvenance identifies only the bytes supplied to this model. It
// deliberately records no path, context, wall-clock time, or live observation.
type RecordedMapProvenance struct {
	Kind                    string `json:"kind"`
	SHA256                  string `json:"sha256"`
	Bytes                   int    `json:"bytes"`
	Documents               int    `json:"documents"`
	ObjectCount             int    `json:"objectCount"`
	CaptureTime             string `json:"captureTime"`
	CaptureCompleteness     string `json:"captureCompleteness"`
	TypedListDerivedObjects int    `json:"typedListDerivedObjects,omitempty"`
}

// RecordedMapResource is a compact identity and built-in ownership
// projection. Health, timestamps, labels, and inferred cluster identity are
// intentionally not part of this recorded inventory model.
type RecordedMapResource struct {
	APIVersion         string                             `json:"apiVersion"`
	Kind               string                             `json:"kind"`
	Namespace          string                             `json:"namespace"`
	Name               string                             `json:"name"`
	Owner              string                             `json:"owner"`
	OwnerDetails       map[string]string                  `json:"ownerDetails,omitempty"`
	OwnershipDetection *mapsvc.OwnershipDetectionEvidence `json:"ownershipDetection"`
}

// RecordedMapReport is an input-scoped projection, not a complete cluster
// inventory. ExcludedFromScope counts parsed objects filtered out by Scope;
// it says nothing about resources absent from the recording.
type RecordedMapReport struct {
	Schema            string                `json:"schema"`
	Provenance        RecordedMapProvenance `json:"provenance"`
	Scope             RecordedMapScope      `json:"scope"`
	SelectedCount     int                   `json:"selectedCount"`
	ExcludedFromScope int                   `json:"excludedFromScopeCount"`
	OwnerCounts       map[string]int        `json:"ownerCounts"`
	Resources         []RecordedMapResource `json:"resources"`
}

// RecordedMapSummary intentionally has no resource rows. Its separate schema
// and view marker distinguish an omitted-row summary from an empty inventory.
type RecordedMapSummary struct {
	Schema                 string                `json:"schema"`
	View                   string                `json:"view"`
	Provenance             RecordedMapProvenance `json:"provenance"`
	Scope                  RecordedMapScope      `json:"scope"`
	SelectedCount          int                   `json:"selectedCount"`
	ExcludedFromScope      int                   `json:"excludedFromScopeCount"`
	OwnerCounts            map[string]int        `json:"ownerCounts"`
	PerObjectEvidenceGuide string                `json:"perObjectEvidenceGuide"`
}

// buildRecordedMapReport is a pure projection of a previously parsed
// snapshot. It does not consult client, clock, environment, user config, or
// live inventory code. Duplicate full identities fail before scope filtering
// so a duplicate outside the requested slice cannot go unnoticed.
func buildRecordedMapReport(snapshot recordedObjectSnapshot, scope RecordedMapScope) (RecordedMapReport, error) {
	if err := validateRecordedMapScope(scope); err != nil {
		return RecordedMapReport{}, err
	}
	if err := validateRecordedMapSnapshot(snapshot); err != nil {
		return RecordedMapReport{}, err
	}

	seen := make(map[recordedObjectIdentity]struct{}, len(snapshot.Objects))
	for _, obj := range snapshot.Objects {
		if obj == nil {
			return RecordedMapReport{}, fmt.Errorf("recorded inventory contains an invalid object identity")
		}
		identity := recordedObjectIdentity{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Namespace:  obj.GetNamespace(),
			Name:       obj.GetName(),
		}
		if !validRecordedIdentityValue(identity.APIVersion, false) ||
			!validRecordedIdentityValue(identity.Kind, false) ||
			!validRecordedIdentityValue(identity.Namespace, true) ||
			!validRecordedIdentityValue(identity.Name, false) {
			return RecordedMapReport{}, fmt.Errorf("recorded inventory contains an invalid object identity")
		}
		if _, ok := seen[identity]; ok {
			return RecordedMapReport{}, fmt.Errorf("recorded inventory contains a duplicate full identity")
		}
		seen[identity] = struct{}{}
	}

	resources := make([]RecordedMapResource, 0, len(snapshot.Objects))
	ownerCounts := make(map[string]int)
	for _, obj := range snapshot.Objects {
		if !recordedMapScopeMatches(scope, obj) {
			continue
		}
		ownership := agent.DetectOwnershipBuiltin(obj)
		owner := recordedMapOwner(ownership)
		if scope.Owner != "" && owner != scope.Owner {
			continue
		}
		resource := RecordedMapResource{
			APIVersion:         obj.GetAPIVersion(),
			Kind:               obj.GetKind(),
			Namespace:          obj.GetNamespace(),
			Name:               obj.GetName(),
			Owner:              owner,
			OwnershipDetection: mapsvc.NewOwnershipDetectionEvidence(ownership),
		}
		if ownership.Name != "" || ownership.Namespace != "" || ownership.SubType != "" {
			resource.OwnerDetails = make(map[string]string, 3)
			if ownership.Name != "" {
				resource.OwnerDetails["name"] = ownership.Name
			}
			if ownership.Namespace != "" {
				resource.OwnerDetails["namespace"] = ownership.Namespace
			}
			if ownership.SubType != "" {
				resource.OwnerDetails["subType"] = ownership.SubType
			}
		}
		resources = append(resources, resource)
		ownerCounts[owner]++
	}
	sort.Slice(resources, func(i, j int) bool {
		a, b := resources[i], resources[j]
		if a.APIVersion != b.APIVersion {
			return a.APIVersion < b.APIVersion
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})

	var namespace *string
	if scope.Namespace != nil {
		value := *scope.Namespace
		namespace = &value
	}
	return RecordedMapReport{
		Schema: recordedMapSchema,
		Provenance: RecordedMapProvenance{
			Kind:                    "kubernetes-object-recording",
			SHA256:                  snapshot.Provenance.SHA256,
			Bytes:                   snapshot.Provenance.Bytes,
			Documents:               snapshot.Provenance.Documents,
			ObjectCount:             snapshot.Provenance.ObjectCount,
			CaptureTime:             "unknown",
			CaptureCompleteness:     "unknown",
			TypedListDerivedObjects: snapshot.Provenance.TypedListDerivedObjects,
		},
		Scope:             RecordedMapScope{APIVersion: scope.APIVersion, Kind: scope.Kind, Namespace: namespace, NamespacePrefix: scope.NamespacePrefix, Owner: scope.Owner},
		SelectedCount:     len(resources),
		ExcludedFromScope: len(snapshot.Objects) - len(resources),
		OwnerCounts:       ownerCounts,
		Resources:         resources,
	}, nil
}

func buildRecordedMapSummary(snapshot recordedObjectSnapshot, scope RecordedMapScope) (RecordedMapSummary, error) {
	report, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		return RecordedMapSummary{}, err
	}
	return RecordedMapSummary{
		Schema:                 recordedMapSummarySchema,
		View:                   "summary",
		Provenance:             report.Provenance,
		Scope:                  report.Scope,
		SelectedCount:          report.SelectedCount,
		ExcludedFromScope:      report.ExcludedFromScope,
		OwnerCounts:            report.OwnerCounts,
		PerObjectEvidenceGuide: "Per-object detector evidence is omitted in this summary; request the full recorded inventory without summary to inspect selected resources.",
	}, nil
}

func validateRecordedMapScope(scope RecordedMapScope) error {
	if !validRecordedIdentityValue(scope.APIVersion, false) || !validRecordedIdentityValue(scope.Kind, false) {
		return fmt.Errorf("recorded inventory scope requires exact apiVersion and kind")
	}
	if scope.Namespace != nil && scope.NamespacePrefix != "" {
		return fmt.Errorf("recorded inventory scope cannot combine exact namespace and namespace prefix")
	}
	if scope.Namespace != nil && !validRecordedIdentityValue(*scope.Namespace, true) {
		return fmt.Errorf("recorded inventory scope has an invalid namespace")
	}
	if !validRecordedIdentityValue(scope.NamespacePrefix, true) {
		return fmt.Errorf("recorded inventory scope has an invalid namespace prefix")
	}
	if scope.Owner != "" && !isCanonicalRecordedOwner(scope.Owner) {
		return fmt.Errorf("recorded inventory owner must be one of the canonical built-in owners: %s", strings.Join(recordedMapOwnerNames, ", "))
	}
	return nil
}

func validateRecordedMapSnapshot(snapshot recordedObjectSnapshot) error {
	if len(snapshot.Objects) == 0 {
		return fmt.Errorf("recorded inventory contains no objects")
	}
	if len(snapshot.Objects) > maxRecordedObjectCount || snapshot.Provenance.ObjectCount != len(snapshot.Objects) {
		return fmt.Errorf("recorded inventory object count is invalid")
	}
	if snapshot.Provenance.Bytes <= 0 || snapshot.Provenance.Bytes > maxRecordedObjectBytes ||
		snapshot.Provenance.Documents <= 0 || snapshot.Provenance.Documents > maxRecordedObjectDocuments {
		return fmt.Errorf("recorded inventory provenance is invalid")
	}
	digest, err := hex.DecodeString(snapshot.Provenance.SHA256)
	if err != nil || len(digest) != 32 {
		return fmt.Errorf("recorded inventory provenance is invalid")
	}
	return nil
}

func recordedMapScopeMatches(scope RecordedMapScope, obj *unstructured.Unstructured) bool {
	if obj.GetAPIVersion() != scope.APIVersion || obj.GetKind() != scope.Kind {
		return false
	}
	if scope.Namespace != nil {
		return obj.GetNamespace() == *scope.Namespace
	}
	return scope.NamespacePrefix == "" || strings.HasPrefix(obj.GetNamespace(), scope.NamespacePrefix)
}

var recordedMapOwnerNames = []string{"Flux", "ArgoCD", "Sveltos", "Modelplane", "Crossplane", "kro", "Helm", "Terraform", "ConfigHub", "Kubernetes", "Native"}

func isCanonicalRecordedOwner(owner string) bool {
	for _, canonical := range recordedMapOwnerNames {
		if owner == canonical {
			return true
		}
	}
	return false
}

func recordedMapOwner(ownership agent.Ownership) string {
	if ownership.Type == "" || ownership.Type == agent.OwnerUnknown {
		return "Native"
	}
	if ownership.Type == agent.OwnerKubernetes {
		// A Kubernetes owner reference is a built-in marker. Keep it distinct
		// from Native, which means no built-in owner marker was detected.
		return "Kubernetes"
	}
	return mapsvc.DisplayOwner(ownership.Type)
}
