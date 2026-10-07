// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// FluxAPITracer traces a Flux-managed object through the Kubernetes API. It
// follows the same objects `flux trace` reads: the object's Flux labels name
// the Kustomization or HelmRelease that applied it, and that object's
// sourceRef names its source. It is for callers without the flux CLI (#824);
// it makes exact GET reads only, never a LIST.
type FluxAPITracer struct {
	client dynamic.Interface
}

// NewFluxTracerWithKubernetesClient returns a Flux tracer that reads through
// client and needs no flux binary.
func NewFluxTracerWithKubernetesClient(client dynamic.Interface) *FluxAPITracer {
	return &FluxAPITracer{client: client}
}

// ToolName returns "flux".
func (t *FluxAPITracer) ToolName() string { return "flux" }

// Available reports whether the tracer has a client to read with.
func (t *FluxAPITracer) Available() bool { return t != nil && t.client != nil }

const (
	fluxKustomizeNameLabel      = "kustomize.toolkit.fluxcd.io/name"
	fluxKustomizeNamespaceLabel = "kustomize.toolkit.fluxcd.io/namespace"
	fluxHelmNameLabel           = "helm.toolkit.fluxcd.io/name"
	fluxHelmNamespaceLabel      = "helm.toolkit.fluxcd.io/namespace"
)

// fluxAPIVersions lists, newest first, the versions tried for each Flux kind.
// A cluster serves at least one of them; an older Flux may serve only the
// later entries.
var fluxAPIVersions = map[string][]schema.GroupVersionResource{
	"Kustomization": {
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
	},
	"HelmRelease": {
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"},
	},
	"GitRepository": {
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
	},
	"OCIRepository": {
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"},
	},
	"Bucket": {
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "buckets"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "buckets"},
	},
	"HelmRepository": {
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"},
	},
	"HelmChart": {
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmcharts"},
	},
}

// Trace returns the chain from source to the named object, ordered source
// first, as the flux CLI tracer does. It returns an error, and no partial
// chain, when any object on the way to the source cannot be read: a chain
// that stops short would present a deployer as the root.
func (t *FluxAPITracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	if !t.Available() {
		return nil, fmt.Errorf("flux API trace requires a Kubernetes client")
	}
	obj, err := t.getObject(ctx, kind, name, namespace)
	if err != nil {
		return nil, err
	}
	labels := obj.GetLabels()

	var chain []ChainLink
	switch {
	case labels[fluxKustomizeNameLabel] != "":
		chain, err = t.kustomizationChain(ctx, labels[fluxKustomizeNameLabel], labels[fluxKustomizeNamespaceLabel])
	case labels[fluxHelmNameLabel] != "":
		chain, err = t.helmReleaseChain(ctx, labels[fluxHelmNameLabel], labels[fluxHelmNamespaceLabel])
	default:
		// The wording the CLI tracer's callers already recognise.
		return nil, fmt.Errorf("resource not managed by Flux")
	}
	if err != nil {
		return nil, err
	}

	chain = append(chain, ChainLink{
		Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace(),
		Ready: true, Status: "Managed by Flux",
	})
	result := &TraceResult{
		Object:       ResourceRef{Kind: kind, Name: name, Namespace: namespace},
		Chain:        chain,
		FullyManaged: true,
		Tool:         "flux",
		TracedAt:     time.Now(),
	}
	for _, link := range chain {
		if !link.Ready {
			result.FullyManaged = false
		}
	}
	return result, nil
}

func (t *FluxAPITracer) getObject(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	if _, isFlux := fluxAPIVersions[canonicalFluxKind(kind)]; isFlux {
		return t.getFlux(ctx, canonicalFluxKind(kind), name, namespace)
	}
	gvr, err := KindToGVR(kind)
	if err != nil {
		return nil, fmt.Errorf("flux API trace does not know how to read kind %q", kind)
	}
	obj, err := t.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read %s %s/%s: %w", kind, namespace, name, err)
	}
	return obj, nil
}

func canonicalFluxKind(kind string) string {
	for known := range fluxAPIVersions {
		if strings.EqualFold(known, kind) {
			return known
		}
	}
	return kind
}

// getFlux reads one Flux object, trying each served version in turn. Only a
// "no such resource type" answer moves on to the next version; a missing
// object or a denied read is returned as it is.
func (t *FluxAPITracer) getFlux(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	versions := fluxAPIVersions[kind]
	if len(versions) == 0 {
		return nil, fmt.Errorf("flux API trace does not support source kind %q", kind)
	}
	var lastErr error
	for _, gvr := range versions {
		obj, err := t.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return obj, nil
		}
		lastErr = err
		if !meta.IsNoMatchError(err) && !isResourceTypeNotFound(err) {
			break
		}
	}
	return nil, fmt.Errorf("read %s %s/%s: %w", kind, namespace, name, lastErr)
}

// isResourceTypeNotFound distinguishes "this API version is not served" from
// "this object does not exist"; both are 404s.
func isResourceTypeNotFound(err error) bool {
	if !apierrors.IsNotFound(err) {
		return false
	}
	status, ok := err.(apierrors.APIStatus)
	if !ok {
		return false
	}
	details := status.Status().Details
	return details == nil || details.Name == ""
}

func (t *FluxAPITracer) kustomizationChain(ctx context.Context, name, namespace string) ([]ChainLink, error) {
	ks, err := t.getFlux(ctx, "Kustomization", name, namespace)
	if err != nil {
		return nil, err
	}
	link := fluxLinkFromObject(ks)
	link.Path, _, _ = unstructured.NestedString(ks.Object, "spec", "path")
	link.Revision, _, _ = unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	source, err := t.sourceLink(ctx, ks, namespace, "spec", "sourceRef")
	if err != nil {
		return nil, err
	}
	return []ChainLink{source, link}, nil
}

func (t *FluxAPITracer) helmReleaseChain(ctx context.Context, name, namespace string) ([]ChainLink, error) {
	hr, err := t.getFlux(ctx, "HelmRelease", name, namespace)
	if err != nil {
		return nil, err
	}
	link := fluxLinkFromObject(hr)
	link.Revision = helmReleaseRevision(hr)

	// spec.chartRef names an OCIRepository or HelmChart directly.
	if refKind, _, _ := unstructured.NestedString(hr.Object, "spec", "chartRef", "kind"); refKind != "" {
		if strings.EqualFold(refKind, "HelmChart") {
			refName, _, _ := unstructured.NestedString(hr.Object, "spec", "chartRef", "name")
			refNamespace, _, _ := unstructured.NestedString(hr.Object, "spec", "chartRef", "namespace")
			if refNamespace == "" {
				refNamespace = namespace
			}
			chain, err := t.helmChartChain(ctx, refName, refNamespace)
			if err != nil {
				return nil, err
			}
			return append(chain, link), nil
		}
		source, err := t.sourceLink(ctx, hr, namespace, "spec", "chartRef")
		if err != nil {
			return nil, err
		}
		return []ChainLink{source, link}, nil
	}

	// spec.chart: the controller records the HelmChart it created as
	// "<namespace>/<name>" in status.helmChart.
	recorded, _, _ := unstructured.NestedString(hr.Object, "status", "helmChart")
	chartNamespace, chartName, ok := strings.Cut(recorded, "/")
	if !ok || chartNamespace == "" || chartName == "" {
		return nil, fmt.Errorf("HelmRelease %s/%s has not recorded its HelmChart yet", namespace, name)
	}
	chain, err := t.helmChartChain(ctx, chartName, chartNamespace)
	if err != nil {
		return nil, err
	}
	return append(chain, link), nil
}

func (t *FluxAPITracer) helmChartChain(ctx context.Context, name, namespace string) ([]ChainLink, error) {
	chart, err := t.getFlux(ctx, "HelmChart", name, namespace)
	if err != nil {
		return nil, err
	}
	link := fluxLinkFromObject(chart)
	link.Revision, _, _ = unstructured.NestedString(chart.Object, "status", "artifact", "revision")
	source, err := t.sourceLink(ctx, chart, namespace, "spec", "sourceRef")
	if err != nil {
		return nil, err
	}
	return []ChainLink{source, link}, nil
}

// sourceLink reads the source object named by the reference at fields.
func (t *FluxAPITracer) sourceLink(ctx context.Context, owner *unstructured.Unstructured, ownerNamespace string, fields ...string) (ChainLink, error) {
	ref, found, _ := unstructured.NestedStringMap(owner.Object, fields...)
	if !found || ref["kind"] == "" || ref["name"] == "" {
		return ChainLink{}, fmt.Errorf("%s %s/%s names no source", owner.GetKind(), owner.GetNamespace(), owner.GetName())
	}
	namespace := ref["namespace"]
	if namespace == "" {
		namespace = ownerNamespace
	}
	source, err := t.getFlux(ctx, ref["kind"], ref["name"], namespace)
	if err != nil {
		return ChainLink{}, err
	}
	link := fluxLinkFromObject(source)
	link.URL, _, _ = unstructured.NestedString(source.Object, "spec", "url")
	link.Revision, _, _ = unstructured.NestedString(source.Object, "status", "artifact", "revision")
	finalizeFluxSourceLink(&link)
	return link, nil
}

// fluxLinkFromObject fills the identity and readiness of a Flux object from
// its Ready condition. Readiness is the condition's status, not a reading of
// its message.
func fluxLinkFromObject(obj *unstructured.Unstructured) ChainLink {
	link := ChainLink{Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace(), Status: "Unknown"}
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if !ok || condition["type"] != "Ready" {
			continue
		}
		status, _ := condition["status"].(string)
		reason, _ := condition["reason"].(string)
		link.Message, _ = condition["message"].(string)
		link.Ready = status == "True"
		if when, _ := condition["lastTransitionTime"].(string); when != "" {
			if parsed, err := time.Parse(time.RFC3339, when); err == nil {
				link.LastTransitionTime = &parsed
			}
		}
		if link.Ready {
			link.Status = "Ready"
		} else {
			link.Status = "Not ready"
			link.StatusReason = firstNonEmptyString(link.Message, reason)
		}
	}
	if suspended, _, _ := unstructured.NestedBool(obj.Object, "spec", "suspend"); suspended {
		link.Ready = false
		link.Status = "Suspended"
	}
	return link
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// helmReleaseRevision is the chart version of the release the controller last
// recorded, which is what `flux trace` prints for a HelmRelease.
func helmReleaseRevision(hr *unstructured.Unstructured) string {
	history, _, _ := unstructured.NestedSlice(hr.Object, "status", "history")
	if len(history) > 0 {
		if latest, ok := history[0].(map[string]interface{}); ok {
			if version, _ := latest["chartVersion"].(string); version != "" {
				return version
			}
		}
	}
	revision, _, _ := unstructured.NestedString(hr.Object, "status", "lastAttemptedRevision")
	return revision
}

// finalizeFluxSourceLink applies the OCI source parsing both Flux tracers
// share, so a ConfigHub OCI source is labelled the same way whichever read it.
func finalizeFluxSourceLink(link *ChainLink) {
	if link.Kind == "OCIRepository" && link.URL != "" && strings.HasPrefix(link.URL, "oci://") {
		ociInfo := ParseOCISource(link.URL)
		link.OCISource = &ociInfo
		if ociInfo.IsConfigHub {
			link.Kind = "ConfigHub OCI"
		}
	}
}

// fluxConditionReadinessTracer wraps the flux CLI tracer and replaces the
// readiness it inferred from text with each object's Ready condition.
type fluxConditionReadinessTracer struct {
	cli    Tracer
	reader *FluxAPITracer
}

// NewFluxTracerWithConditionReadiness returns cli with the readiness of every
// Flux link taken from the Kubernetes API.
//
// `flux trace` prints "Status: Last reconciled at <time>" for any object that
// has a Ready condition, whether that condition is True or False; the failure
// is only in the Message line. Read as text, a Kustomization whose build is
// failing looks ready (#826). The chain still comes from the CLI; only
// readiness, status and reason are replaced, by one exact GET per Flux link.
func NewFluxTracerWithConditionReadiness(cli Tracer, client dynamic.Interface) Tracer {
	if cli == nil || client == nil {
		return cli
	}
	return &fluxConditionReadinessTracer{cli: cli, reader: NewFluxTracerWithKubernetesClient(client)}
}

func (t *fluxConditionReadinessTracer) ToolName() string { return t.cli.ToolName() }
func (t *fluxConditionReadinessTracer) Available() bool  { return t.cli.Available() }

func (t *fluxConditionReadinessTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	result, err := t.cli.Trace(ctx, kind, name, namespace)
	if err != nil || result == nil {
		return result, err
	}
	result.FullyManaged = true
	for i := range result.Chain {
		link := &result.Chain[i]
		// A ConfigHub OCI source is an OCIRepository the parser relabelled.
		fluxKind := link.Kind
		if link.OCISource != nil && link.OCISource.IsConfigHub {
			fluxKind = "OCIRepository"
		}
		if _, isFlux := fluxAPIVersions[fluxKind]; isFlux {
			obj, readErr := t.reader.getFlux(ctx, fluxKind, link.Name, link.Namespace)
			if readErr != nil {
				// The text could not be checked. Say so; do not let an
				// unverified "ready" stand as verified.
				link.Ready = false
				link.Status = "Unknown"
				link.StatusReason = "readiness could not be read from the cluster: " + readErr.Error()
			} else {
				observed := fluxLinkFromObject(obj)
				link.Ready, link.Status, link.StatusReason = observed.Ready, observed.Status, observed.StatusReason
				link.LastTransitionTime = observed.LastTransitionTime
				if observed.Message != "" {
					link.Message = observed.Message
				}
			}
		}
		if !link.Ready {
			result.FullyManaged = false
		}
	}
	return result, nil
}
