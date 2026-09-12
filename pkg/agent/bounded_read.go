// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// BoundedResourceRef is an exact API identity, not a kind/plural heuristic.
type BoundedResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

type BoundedReadCounts struct {
	Discovery int `json:"discovery"`
	Object    int `json:"object"`
}

type BoundedReadEvidence struct {
	Available       bool               `json:"available"`
	Context         string             `json:"context"`
	Resource        BoundedResourceRef `json:"resource"`
	UID             string             `json:"uid,omitempty"`
	ResourceVersion string             `json:"resourceVersion,omitempty"`
	ObservedAt      time.Time          `json:"observedAt"`
	ExpiresAt       time.Time          `json:"expiresAt"`
	Cache           string             `json:"cache"`
	Reads           BoundedReadCounts  `json:"reads"`
}

type boundedReadEntry struct {
	object     *unstructured.Unstructured
	observedAt time.Time
	expiresAt  time.Time
	used       uint64
}

// BoundedResourceReader pins one client/context for a short-lived explorer
// session. Construct a new reader whenever context or credential configuration
// changes. It never reads other objects, events, controllers, or ConfigHub.
type BoundedResourceReader struct {
	client     rest.Interface
	context    string
	server     string
	gate       chan struct{}
	cache      map[BoundedResourceRef]boundedReadEntry
	sequence   uint64
	now        func() time.Time
	ttl        time.Duration
	maxEntries int
	maxBytes   int64
}

func NewBoundedResourceReader(config *rest.Config, contextName string) (*BoundedResourceReader, error) {
	if config == nil || strings.TrimSpace(contextName) == "" {
		return nil, fmt.Errorf("bounded read requires an explicit client and context identity")
	}
	clientConfig := dynamic.ConfigFor(rest.CopyConfig(config))
	httpClient, err := rest.HTTPClientFor(clientConfig)
	if err != nil {
		return nil, err
	}
	// HTTP redirects can otherwise add requests despite REST retries being off.
	// Copy the client because HTTPClientFor may return http.DefaultClient.
	boundedHTTP := *httpClient
	boundedHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client, err := rest.UnversionedRESTClientForConfigAndClient(clientConfig, &boundedHTTP)
	if err != nil {
		return nil, err
	}
	return &BoundedResourceReader{
		client: client, context: contextName, server: config.Host, gate: make(chan struct{}, 1),
		cache: make(map[BoundedResourceRef]boundedReadEntry), now: time.Now,
		ttl: 15 * time.Second, maxEntries: 16, maxBytes: 2 << 20,
	}, nil
}

// Server identifies the API endpoint pinned to this reader's credentials.
func (r *BoundedResourceReader) Server() string { return r.server }

var boundedKindPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var boundedVersionPattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

func (ref BoundedResourceRef) Validate() error {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || !boundedVersionPattern.MatchString(gv.Version) || (gv.Group != "" && len(validation.IsDNS1123Subdomain(gv.Group)) != 0) {
		return fmt.Errorf("invalid bounded API version %q", ref.APIVersion)
	}
	if !boundedKindPattern.MatchString(ref.Kind) || strings.EqualFold(ref.Kind, "Secret") {
		return fmt.Errorf("bounded read does not support kind %q (Secret payloads and subresources are excluded)", ref.Kind)
	}
	if len(validation.IsDNS1123Subdomain(ref.Name)) != 0 || (ref.Namespace != "" && len(validation.IsDNS1123Label(ref.Namespace)) != 0) {
		return fmt.Errorf("bounded read requires an exact valid name and namespace")
	}
	return nil
}

// Read performs at most one discovery request and one object request. REST
// retries are disabled; authentication transport traffic is outside this count.
// Calls serialize per reader, including cache access, with cancellable waiting.
// Cached observations are copies, never silently returned after a failed refresh.
func (r *BoundedResourceReader) Read(ctx context.Context, ref BoundedResourceRef, refresh bool) (*unstructured.Unstructured, BoundedReadEvidence, error) {
	evidence := BoundedReadEvidence{Context: r.context, Resource: ref, Cache: "miss"}
	if err := ref.Validate(); err != nil {
		return nil, evidence, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, evidence, err
	}
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return nil, evidence, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, evidence, err
	}
	now := r.now().UTC()
	r.sequence++
	if entry, ok := r.cache[ref]; ok && !refresh && !now.Before(entry.observedAt) && now.Before(entry.expiresAt) {
		entry.used = r.sequence
		r.cache[ref] = entry
		evidence.Cache = "hit"
		return entry.object.DeepCopy(), boundedEvidenceForEntry(evidence, entry), nil
	}
	delete(r.cache, ref)
	if refresh {
		evidence.Cache = "refresh"
	}
	gv, _ := schema.ParseGroupVersion(ref.APIVersion)
	base := "/apis/" + ref.APIVersion
	if gv.Group == "" {
		base = "/api/" + gv.Version
	}
	evidence.Reads.Discovery++
	data, err := r.get(ctx, base)
	if err != nil {
		return nil, evidence, fmt.Errorf("bounded discovery unavailable: %w", err)
	}
	var resources metav1.APIResourceList
	if err := json.Unmarshal(data, &resources); err != nil || resources.GroupVersion != ref.APIVersion {
		return nil, evidence, fmt.Errorf("bounded discovery is malformed or has a different API version")
	}
	var matches []metav1.APIResource
	for _, resource := range resources.APIResources {
		if resource.Kind == ref.Kind && !strings.Contains(resource.Name, "/") {
			matches = append(matches, resource)
		}
	}
	if len(matches) != 1 {
		return nil, evidence, fmt.Errorf("bounded discovery found %d matches for %s %s", len(matches), ref.APIVersion, ref.Kind)
	}
	resource := matches[0]
	if len(validation.IsDNS1123Subdomain(resource.Name)) != 0 || !hasBoundedGetVerb(resource.Verbs) {
		return nil, evidence, fmt.Errorf("bounded discovery does not expose a readable resource")
	}
	if resource.Namespaced && ref.Namespace == "" {
		return nil, evidence, fmt.Errorf("bounded read requires an explicit namespace for %s", ref.Kind)
	}
	if !resource.Namespaced && ref.Namespace != "" {
		return nil, evidence, fmt.Errorf("bounded read refuses a namespace for cluster-scoped %s", ref.Kind)
	}
	path := base
	if resource.Namespaced {
		path += "/namespaces/" + ref.Namespace
	}
	path += "/" + resource.Name + "/" + ref.Name
	evidence.Reads.Object++
	data, err = r.get(ctx, path)
	if err != nil {
		return nil, evidence, fmt.Errorf("bounded object unavailable: %w", err)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(data); err != nil {
		return nil, evidence, fmt.Errorf("bounded object response is malformed")
	}
	if obj.GetAPIVersion() != ref.APIVersion || obj.GetKind() != ref.Kind || obj.GetName() != ref.Name || obj.GetNamespace() != ref.Namespace {
		return nil, evidence, fmt.Errorf("bounded object response does not match the requested API identity")
	}
	if err := ctx.Err(); err != nil {
		return nil, evidence, err
	}
	observedAt := r.now().UTC()
	entry := boundedReadEntry{object: obj, observedAt: observedAt, expiresAt: observedAt.Add(r.ttl), used: r.sequence}
	if len(r.cache) >= r.maxEntries {
		var oldest BoundedResourceRef
		oldestUse := ^uint64(0)
		for key, cached := range r.cache {
			if cached.used < oldestUse {
				oldest, oldestUse = key, cached.used
			}
		}
		delete(r.cache, oldest)
	}
	r.cache[ref] = entry
	return obj.DeepCopy(), boundedEvidenceForEntry(evidence, entry), nil
}

// ListPods performs at most one namespaced, label-selected pod list request,
// capped at `limit` pods and the reader byte cap. It never lists cluster-wide,
// never paginates beyond one page, and reports partial coverage via `capped`
// when the server signals more matching pods than were returned. Pod is a
// guaranteed core resource, so no discovery request is made. The read serializes
// on the same gate as Read. This is the only list this reader performs; it is
// used solely by the opt-in running-image identity tier.
func (r *BoundedResourceReader) ListPods(ctx context.Context, namespace string, matchLabels map[string]string, limit int) (pods []*unstructured.Unstructured, capped bool, evidence BoundedReadEvidence, err error) {
	evidence = BoundedReadEvidence{Context: r.context, Resource: BoundedResourceRef{APIVersion: "v1", Kind: "Pod", Namespace: namespace}, Cache: "list"}
	if len(validation.IsDNS1123Label(namespace)) != 0 {
		return nil, false, evidence, fmt.Errorf("bounded pod list requires an explicit valid namespace")
	}
	if len(matchLabels) == 0 {
		return nil, false, evidence, fmt.Errorf("bounded pod list requires a non-empty label selector")
	}
	if limit < 1 {
		limit = 1
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return nil, false, evidence, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, evidence, err
	}
	keys := make([]string, 0, len(matchLabels))
	for k := range matchLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+matchLabels[k])
	}
	selector := strings.Join(pairs, ",")
	evidence.Reads.Object++
	stream, err := r.client.Get().AbsPath("/api/v1/namespaces/"+namespace+"/pods").
		Param("labelSelector", selector).Param("limit", strconv.Itoa(limit)).
		MaxRetries(0).Stream(ctx)
	if err != nil {
		return nil, false, evidence, fmt.Errorf("bounded pod list unavailable: %w", err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, r.maxBytes+1))
	if err != nil {
		return nil, false, evidence, err
	}
	if int64(len(data)) > r.maxBytes {
		return nil, false, evidence, fmt.Errorf("pod list response exceeds %d bytes", r.maxBytes)
	}
	list := &unstructured.UnstructuredList{}
	if err := list.UnmarshalJSON(data); err != nil {
		return nil, false, evidence, fmt.Errorf("bounded pod list response is malformed")
	}
	for i := range list.Items {
		if kind := list.Items[i].GetKind(); kind != "" && kind != "Pod" {
			continue
		}
		if list.Items[i].GetNamespace() != namespace {
			continue
		}
		pods = append(pods, &list.Items[i])
	}
	capped = list.GetContinue() != ""
	now := r.now().UTC()
	evidence.Available = true
	evidence.ObservedAt, evidence.ExpiresAt = now, now.Add(r.ttl)
	return pods, capped, evidence, nil
}

func (r *BoundedResourceReader) get(ctx context.Context, path string) ([]byte, error) {
	stream, err := r.client.Get().AbsPath(path).MaxRetries(0).Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, r.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > r.maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", r.maxBytes)
	}
	return data, nil
}

func hasBoundedGetVerb(verbs metav1.Verbs) bool {
	for _, verb := range verbs {
		if verb == "get" {
			return true
		}
	}
	return false
}

func boundedEvidenceForEntry(evidence BoundedReadEvidence, entry boundedReadEntry) BoundedReadEvidence {
	evidence.Available = true
	evidence.ObservedAt, evidence.ExpiresAt = entry.observedAt, entry.expiresAt
	evidence.UID, evidence.ResourceVersion = string(entry.object.GetUID()), entry.object.GetResourceVersion()
	return evidence
}
