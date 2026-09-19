// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"reflect"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var traceOCISourceReadFn = func(ctx context.Context, ref agent.BoundedResourceRef) (*unstructured.Unstructured, agent.BoundedReadEvidence, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, agent.BoundedReadEvidence{}, err
	}
	reader, err := agent.NewBoundedResourceReader(cfg, getCurrentContext())
	if err != nil {
		return nil, agent.BoundedReadEvidence{}, err
	}
	return reader.Read(ctx, ref, true)
}

// A trace can carry a new spec URL beside an old status revision. Bind the
// reported revision to its current source before joining connected history.
func confirmTraceOCISource(ctx context.Context, result *agent.TraceResult, c *agent.TraceDeliveryCorrelation) {
	if result == nil || c.OCIIdentityStatus != "exact" {
		return
	}
	c.OCISourceVerified = false
	var ref agent.BoundedResourceRef
	var sourceURL string
	for _, link := range result.Chain {
		if link.OCISource != nil && link.OCISource.IsConfigHub {
			sourceURL = link.OCISource.Raw
			if result.Tool == "flux" {
				ref = agent.BoundedResourceRef{APIVersion: "source.toolkit.fluxcd.io/v1", Kind: "OCIRepository", Namespace: link.Namespace, Name: link.Name}
			}
		}
		if result.Tool == "argocd" && link.Kind == "Application" && ref.Kind == "" {
			ref = agent.BoundedResourceRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: link.Namespace, Name: link.Name}
		}
	}
	obj, e, err := traceOCISourceReadFn(ctx, ref)
	c.OCISourceRead = &e
	if err != nil || !traceOCISourceBindingMatches(obj, sourceURL, c.OCIDigest) {
		c.OCIIdentityStatus = "unverified-source"
		return
	}
	c.OCISourceVerified = true
}

func traceOCISourceBindingMatches(obj *unstructured.Unstructured, sourceURL, digest string) bool {
	if obj == nil || obj.GetUID() == "" || sourceURL == "" || !strictSHA256Digest(digest) {
		return false
	}
	if deleted, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "deletionTimestamp"); err != nil || (found && deleted != nil) {
		return false
	}
	switch obj.GetAPIVersion() + "/" + obj.GetKind() {
	case "argoproj.io/v1alpha1/Application":
		for _, path := range [][]string{{"spec", "sources"}, {"status", "sync", "comparedTo", "sources"}, {"status", "sync", "revisions"}} {
			v, _, err := unstructured.NestedSlice(obj.Object, path...)
			if err != nil || len(v) > 0 {
				return false
			}
		}
		spec, found, err := unstructured.NestedMap(obj.Object, "spec", "source")
		compared, cf, ce := unstructured.NestedMap(obj.Object, "status", "sync", "comparedTo", "source")
		if !found || !cf || err != nil || ce != nil || !reflect.DeepEqual(spec, compared) || spec["repoURL"] != sourceURL {
			return false
		}
		revision, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "revision")
		return firstOCIDigest(revision) == digest
	case "source.toolkit.fluxcd.io/v1/OCIRepository":
		url, _, _ := unstructured.NestedString(obj.Object, "spec", "url")
		generation, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		if url != sourceURL || obj.GetGeneration() <= 0 || generation != obj.GetGeneration() {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		ready := 0
		for _, item := range conditions {
			m, ok := item.(map[string]interface{})
			if !ok {
				return false
			}
			if m["type"] == "Ready" {
				if m["status"] != "True" || m["observedGeneration"] != generation {
					return false
				}
				ready++
			}
			if (m["type"] == "Reconciling" || m["type"] == "Stalled") && m["status"] == "True" {
				return false
			}
		}
		revision, _, _ := unstructured.NestedString(obj.Object, "status", "artifact", "revision")
		return ready == 1 && strings.EqualFold(firstOCIDigest(revision), digest)
	}
	return false
}
