// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

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
	Status              string                     `json:"status"`
	Comparison          string                     `json:"comparison"`
	Coverage            string                     `json:"coverage"`
	Source              traceDiffSource            `json:"source"`
	Context             string                     `json:"context"`
	Resource            agent.BoundedResourceRef   `json:"resource"`
	ScopeDiscoveryReads int                        `json:"scopeDiscoveryReads"`
	Read                *agent.BoundedReadEvidence `json:"read,omitempty"`
	Summary             agent.ObjectSetDiffSummary `json:"summary"`
	Differences         []agent.ObjectSetFieldDiff `json:"differences,omitempty"`
	Omissions           []string                   `json:"omissions,omitempty"`
}

// observeTraceDiff compares an exact object from an already-rendered local
// file/directory with the corresponding object read through session. It does
// not render, invoke controller clients, list for extras, sign, or persist.
func observeTraceDiff(ctx context.Context, session *traceSession, kind, name, namespace, alreadyRenderedPath string) (*traceDiffObservation, error) {
	return observeTraceDiffWithAPIVersion(ctx, session, kind, name, namespace, "", alreadyRenderedPath)
}

func observeTraceDiffWithAPIVersion(ctx context.Context, session *traceSession, kind, name, namespace, apiVersion, alreadyRenderedPath string) (*traceDiffObservation, error) {
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
	desired, err := selectTraceDiffDesiredVersion(objects, kind, name, strings.TrimSpace(namespace), strings.TrimSpace(apiVersion))
	if err != nil {
		return nil, err
	}
	requestedNamespace := strings.TrimSpace(namespace)
	if requestedNamespace != "" && len(validation.IsDNS1123Label(requestedNamespace)) != 0 {
		return nil, fmt.Errorf("requested namespace is invalid")
	}
	if err := validateTraceDiffDesired(desired); err != nil {
		return nil, err
	}
	desired, scopeReads, err := resolveTraceDiffNamespace(ctx, session, desired, requestedNamespace)
	if err != nil {
		return nil, fmt.Errorf("%w (scope discovery GETs=%d)", err, scopeReads)
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
		Comparison:          "authored-fields-only",
		Coverage:            "one-selected-object",
		Source:              traceDiffSource{Kind: "local-rendered", Reference: source.Ref, Digest: source.Digest, ObjectCount: len(objects)},
		Context:             session.contextLabel(),
		Resource:            resource,
		ScopeDiscoveryReads: scopeReads,
		Summary:             agent.ObjectSetDiffSummary{Total: 1},
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

func resolveTraceDiffNamespace(ctx context.Context, session *traceSession, desired *unstructured.Unstructured, requestedNamespace string) (*unstructured.Unstructured, int, error) {
	if desired == nil {
		return nil, 0, fmt.Errorf("desired object identity is incomplete")
	}
	if desired.GetNamespace() != "" {
		if requestedNamespace != "" && desired.GetNamespace() != requestedNamespace {
			return nil, 0, fmt.Errorf("desired object namespace does not match the requested namespace")
		}
		return desired, 0, nil
	}
	gv, err := schema.ParseGroupVersion(desired.GetAPIVersion())
	if err != nil {
		return nil, 0, fmt.Errorf("desired API version is invalid")
	}
	client, err := session.discoveryClient()
	if err != nil {
		return nil, 0, fmt.Errorf("unable to determine desired resource scope")
	}
	resources, err := client.ServerResourcesForGroupVersion(gv.String())
	if err != nil {
		return nil, 1, fmt.Errorf("unable to determine desired resource scope")
	}
	var namespaced *bool
	for _, resource := range resources.APIResources {
		if resource.Kind == desired.GetKind() {
			scope := resource.Namespaced
			if namespaced != nil && *namespaced != scope {
				return nil, 1, fmt.Errorf("desired resource scope is ambiguous")
			}
			namespaced = &scope
		}
	}
	if namespaced == nil {
		return nil, 1, fmt.Errorf("desired resource scope is unavailable for %s %s", desired.GetAPIVersion(), desired.GetKind())
	}
	if *namespaced {
		if requestedNamespace == "" {
			return nil, 1, fmt.Errorf("desired namespaced object has no namespace; pass -n to supply its exact namespace")
		}
		desired.SetNamespace(requestedNamespace)
		return desired, 1, nil
	}
	if requestedNamespace != "" {
		return nil, 1, fmt.Errorf("cluster-scoped desired object cannot be combined with -n")
	}
	return desired, 1, nil
}

func runTraceDiffObservation(ctx context.Context, selection clusterContextSelection, kind, name, namespace, apiVersion, desiredFile string) error {
	format := traceFormat
	if traceJSON && format == "ascii" {
		format = "json"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("unsupported trace format %q (supported: ascii, json, md)", format)
	}
	session, err := newTraceSessionForSelection(selection)
	if err != nil {
		return fmt.Errorf("failed to capture Kubernetes trace session: %w", err)
	}
	result, err := observeTraceDiffWithAPIVersion(ctx, session, kind, name, namespace, apiVersion, desiredFile)
	if err != nil {
		return err
	}
	return renderTraceDiffObservation(os.Stdout, result, format)
}

func selectTraceDiffDesired(objects []*unstructured.Unstructured, kind, name, namespace string) (*unstructured.Unstructured, error) {
	return selectTraceDiffDesiredVersion(objects, kind, name, namespace, "")
}

func selectTraceDiffDesiredVersion(objects []*unstructured.Unstructured, kind, name, namespace, apiVersion string) (*unstructured.Unstructured, error) {
	if apiVersion != "" {
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil || gv.String() != apiVersion || !traceDiffVersionPattern.MatchString(gv.Version) || (gv.Group != "" && len(validation.IsDNS1123Subdomain(gv.Group)) != 0) {
			return nil, fmt.Errorf("desired API version is invalid")
		}
	}
	var matches []*unstructured.Unstructured
	var namespaceMismatch bool
	for _, obj := range objects {
		if obj == nil || !strings.EqualFold(obj.GetKind(), kind) || obj.GetName() != name {
			continue
		}
		if apiVersion != "" && obj.GetAPIVersion() != apiVersion {
			continue
		}
		if namespace != "" && obj.GetNamespace() != "" && obj.GetNamespace() != namespace {
			namespaceMismatch = true
			continue
		}
		matches = append(matches, obj)
	}
	switch len(matches) {
	case 0:
		if namespaceMismatch {
			return nil, fmt.Errorf("desired object namespace does not match the requested namespace")
		}
		return nil, fmt.Errorf("desired object %s/%s was not found in the rendered input", kind, name)
	case 1:
		return matches[0].DeepCopy(), nil
	default:
		return nil, fmt.Errorf("desired object %s/%s is ambiguous; specify a namespace and unique API version", kind, name)
	}
}

func renderTraceDiffObservation(w io.Writer, result *traceDiffObservation, format string) error {
	if result == nil {
		return fmt.Errorf("trace diff observation is unavailable")
	}
	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	case "ascii":
		return renderTraceDiffObservationHuman(w, result)
	case "md":
		return renderTraceDiffObservationMarkdown(w, result)
	default:
		return fmt.Errorf("unsupported trace diff format %q (supported: ascii, json, md)", format)
	}
}

func renderTraceDiffObservationHuman(w io.Writer, result *traceDiffObservation) error {
	resource := result.Resource
	if _, err := fmt.Fprintf(w, "Trace diff (local rendered input vs observed live; %s)\nSource: %s %s", result.Comparison, result.Source.Kind, result.Source.Reference); err != nil {
		return err
	}
	if result.Source.Digest != "" {
		if _, err := fmt.Fprintf(w, " (sha256: %s)", result.Source.Digest); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\nContext: %s\nResource: %s %s", result.Context, resource.APIVersion, resource.Kind); err != nil {
		return err
	}
	if resource.Namespace != "" {
		if _, err := fmt.Fprintf(w, " %s/%s", resource.Namespace, resource.Name); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, " %s", resource.Name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "\nResult: %s\n", result.Status); err != nil {
		return err
	}
	if result.Read != nil {
		if _, err := fmt.Fprintf(w, "Live read: UID=%s resourceVersion=%s observedAt=%s (scope discovery GETs=%d; bounded-reader GETs discovery=%d object=%d)\n", result.Read.UID, result.Read.ResourceVersion, result.Read.ObservedAt.UTC().Format(time.RFC3339), result.ScopeDiscoveryReads, result.Read.Reads.Discovery, result.Read.Reads.Object); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Coverage: %s; no object-set closure or next-reconcile prediction\n", result.Coverage); err != nil {
		return err
	}
	for _, omission := range result.Omissions {
		if _, err := fmt.Fprintf(w, "Omission: %s\n", omission); err != nil {
			return err
		}
	}
	for _, difference := range result.Differences {
		if _, err := fmt.Fprintf(w, "- %s: desired=%q live=%q\n", difference.Field, difference.Desired, difference.Live); err != nil {
			return err
		}
	}
	return nil
}

func renderTraceDiffObservationMarkdown(w io.Writer, result *traceDiffObservation) error {
	resource := result.Resource
	if _, err := fmt.Fprintf(w, "## Trace diff: %s\n\n- Comparison: `%s`\n- Desired operand: `%s` (`%s`, ref `%s`)\n- Context: `%s`\n- Resource: `%s %s %s/%s`\n- Live UID: `%s`\n- Live resourceVersion: `%s`\n- Live observed at: `%s`\n- API GETs: scope discovery=%d; bounded reader discovery=%d object=%d\n- Result: **%s**\n- Coverage: `%s`; no object-set closure or next-reconcile prediction\n", result.Status, result.Comparison, result.Source.Kind, result.Source.Digest, result.Source.Reference, result.Context, resource.APIVersion, resource.Kind, resource.Namespace, resource.Name, readUID(result.Read), readResourceVersion(result.Read), readObservedAt(result.Read), result.ScopeDiscoveryReads, readDiscoveryCount(result.Read), readObjectCount(result.Read), result.Status, result.Coverage); err != nil {
		return err
	}
	for _, omission := range result.Omissions {
		if _, err := fmt.Fprintf(w, "\nOmission: %s\n", omission); err != nil {
			return err
		}
	}
	if len(result.Differences) > 0 {
		if _, err := fmt.Fprintln(w, "\n| Field | Desired | Live |\n|---|---|---|"); err != nil {
			return err
		}
		for _, difference := range result.Differences {
			if _, err := fmt.Fprintf(w, "| `%s` | `%s` | `%s` |\n", markdownCell(difference.Field), markdownCell(difference.Desired), markdownCell(difference.Live)); err != nil {
				return err
			}
		}
	}
	return nil
}

func readObservedAt(read *agent.BoundedReadEvidence) string {
	if read == nil || read.ObservedAt.IsZero() {
		return "unavailable"
	}
	return read.ObservedAt.UTC().Format(time.RFC3339)
}

func readDiscoveryCount(read *agent.BoundedReadEvidence) int {
	if read == nil {
		return 0
	}
	return read.Reads.Discovery
}

func readObjectCount(read *agent.BoundedReadEvidence) int {
	if read == nil {
		return 0
	}
	return read.Reads.Object
}

func markdownCell(value string) string {
	return strings.NewReplacer("\\", "\\\\", "|", "\\|", "\r", "", "\n", "<br>", "`", "&#96;").Replace(value)
}

func readUID(read *agent.BoundedReadEvidence) string {
	if read == nil {
		return ""
	}
	return read.UID
}

func readResourceVersion(read *agent.BoundedReadEvidence) string {
	if read == nil {
		return ""
	}
	return read.ResourceVersion
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
