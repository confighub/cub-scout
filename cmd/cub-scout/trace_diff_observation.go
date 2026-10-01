// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	traceDiffStatusMatched      = "matched"
	traceDiffStatusChanged      = "changed"
	traceDiffStatusMissing      = "missing"
	traceDiffStatusInconclusive = "inconclusive"
)

var traceDiffKindPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var traceDiffVersionPattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// traceDiffSource describes a local, already-rendered operand. This is not a
// claim about a controller's current render or intended state.
type traceDiffSource struct {
	Kind        string `json:"kind"`
	Reference   string `json:"reference"`
	Digest      string `json:"digest,omitempty"`
	ObjectCount int    `json:"objectCount"`
}

// traceDiffObservation is the bounded result of comparing one selected
// rendered object with one object read through a captured Kubernetes session.
type traceDiffObservation struct {
	Status      string                     `json:"status"`
	Source      traceDiffSource            `json:"source"`
	Context     string                     `json:"context"`
	Resource    agent.BoundedResourceRef   `json:"resource"`
	Read        *agent.BoundedReadEvidence `json:"read,omitempty"`
	Summary     agent.ObjectSetDiffSummary `json:"summary"`
	Differences []agent.ObjectSetFieldDiff `json:"differences,omitempty"`
	Omissions   []string                   `json:"omissions,omitempty"`
}

// observeTraceDiff compares an exact object from an already-rendered local
// file/directory with the corresponding object read through session. It does
// not render, invoke controller clients, list for extras, sign, or persist.
func observeTraceDiff(ctx context.Context, session *traceSession, kind, name, namespace, alreadyRenderedPath string) (*traceDiffObservation, error) {
	if session == nil {
		return nil, fmt.Errorf("trace diff requires a captured Kubernetes session")
	}
	kind = normalizeKind(strings.TrimSpace(kind))
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		return nil, fmt.Errorf("trace diff requires an exact kind and name")
	}
	if strings.TrimSpace(alreadyRenderedPath) == "" {
		return nil, fmt.Errorf("trace diff requires an already-rendered local manifest path")
	}

	objects, source, err := loadObjectSetDesiredManifests(alreadyRenderedPath)
	if err != nil {
		// Parser diagnostics can contain snippets from user-supplied YAML.
		return nil, fmt.Errorf("unable to parse already-rendered trace diff input")
	}
	desired, err := selectTraceDiffDesired(objects, kind, name, strings.TrimSpace(namespace))
	if err != nil {
		return nil, err
	}
	if err := validateTraceDiffDesired(desired); err != nil {
		return nil, err
	}

	resource := agent.BoundedResourceRef{
		APIVersion: desired.GetAPIVersion(),
		Kind:       desired.GetKind(),
		Namespace:  desired.GetNamespace(),
		Name:       desired.GetName(),
	}
	result := &traceDiffObservation{
		Source:   traceDiffSource{Kind: "local-rendered", Reference: source.Ref, Digest: source.Digest, ObjectCount: len(objects)},
		Context:  session.contextLabel(),
		Resource: resource,
		Summary:  agent.ObjectSetDiffSummary{Total: 1},
	}
	// The source digest covers whole files, so withhold it if a Secret document
	// appears anywhere in the operand. The bounded reader deliberately refuses
	// Secret reads; do not report a false clean comparison or payload hashes.
	containsSecret := false
	for _, obj := range objects {
		if obj != nil && strings.EqualFold(obj.GetKind(), "Secret") {
			containsSecret = true
			break
		}
	}
	if containsSecret {
		result.Source.Digest = ""
	}
	if strings.EqualFold(resource.Kind, "Secret") {
		result.Status = traceDiffStatusInconclusive
		result.Omissions = []string{"Secret payload comparison omitted; Secret data and source-content digests are withheld"}
		return result, nil
	}
	if err := resource.Validate(); err != nil {
		return nil, fmt.Errorf("selected desired object has an invalid Kubernetes identity")
	}

	config, err := session.restConfig()
	if err != nil {
		return nil, fmt.Errorf("captured Kubernetes session is unavailable")
	}
	reader, err := agent.NewBoundedResourceReader(config, session.contextLabel())
	if err != nil {
		return nil, fmt.Errorf("unable to initialize bounded trace diff reader")
	}
	live, readEvidence, readErr := reader.Read(ctx, resource, false)
	result.Read = &readEvidence
	if readErr != nil {
		if readEvidence.Reads.Object > 0 && apierrors.IsNotFound(readErr) {
			result.Status = traceDiffStatusMissing
			result.Summary.Removed = 1
			return result, nil
		}
		result.Status = traceDiffStatusInconclusive
		result.Omissions = []string{"live object could not be read; comparison is inconclusive"}
		return result, nil
	}

	scope := agent.ObjectSetScope{Kind: "cluster"}
	if resource.Namespace != "" {
		scope = agent.ObjectSetScope{Kind: "namespace", Namespace: resource.Namespace}
	}
	base, err := agent.BuildObjectSetEvidence(
		agent.ObjectSetSource{Type: "file", Ref: source.Ref, ObjectCount: 1},
		scope,
		[]agent.ObjectSetObservedObject{{Desired: desired, Live: live}},
	)
	if err != nil || len(base.Objects) != 1 {
		return nil, fmt.Errorf("unable to compare selected desired and live objects")
	}
	summary := base.Objects[0]
	switch summary.Status {
	case agent.ObjectSetObjectMatched:
		result.Status = traceDiffStatusMatched
	case agent.ObjectSetObjectMismatched:
		result.Status = traceDiffStatusChanged
		result.Summary.Changed = 1
		result.Differences = mapObjectSetFieldDiffs(summary, desired, live)
	default:
		// A successful read cannot produce a missing/inconclusive summary here;
		// retain a conservative state if that invariant changes in the helper.
		result.Status = traceDiffStatusInconclusive
		result.Omissions = []string{"authored-field comparison was inconclusive"}
	}
	return result, nil
}

func selectTraceDiffDesired(objects []*unstructured.Unstructured, kind, name, namespace string) (*unstructured.Unstructured, error) {
	var matches []*unstructured.Unstructured
	for _, obj := range objects {
		if obj == nil || !strings.EqualFold(obj.GetKind(), kind) || obj.GetName() != name {
			continue
		}
		if namespace != "" && obj.GetNamespace() != "" && obj.GetNamespace() != namespace {
			continue
		}
		matches = append(matches, obj)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("desired object %s/%s was not found in the rendered input", kind, name)
	case 1:
		selected := matches[0].DeepCopy()
		if namespace != "" {
			if selected.GetNamespace() == "" {
				selected.SetNamespace(namespace)
			} else if selected.GetNamespace() != namespace {
				return nil, fmt.Errorf("desired object namespace does not match the requested namespace")
			}
		}
		return selected, nil
	default:
		return nil, fmt.Errorf("desired object %s/%s is ambiguous; specify a namespace and unique API version", kind, name)
	}
}

func validateTraceDiffDesired(obj *unstructured.Unstructured) error {
	if obj == nil || strings.TrimSpace(obj.GetAPIVersion()) == "" || strings.TrimSpace(obj.GetKind()) == "" || strings.TrimSpace(obj.GetName()) == "" {
		return fmt.Errorf("selected desired object has an incomplete Kubernetes identity")
	}
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil || !traceDiffVersionPattern.MatchString(gv.Version) || (gv.Group != "" && len(validation.IsDNS1123Subdomain(gv.Group)) != 0) {
		return fmt.Errorf("selected desired object has an invalid Kubernetes identity")
	}
	if !traceDiffKindPattern.MatchString(obj.GetKind()) || len(validation.IsDNS1123Subdomain(obj.GetName())) != 0 {
		return fmt.Errorf("selected desired object has an invalid Kubernetes identity")
	}
	if namespace := obj.GetNamespace(); namespace != "" && len(validation.IsDNS1123Label(namespace)) != 0 {
		return fmt.Errorf("selected desired object has an invalid Kubernetes identity")
	}
	return nil
}
