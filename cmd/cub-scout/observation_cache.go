// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strconv"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// cycleCachingDynamicClient wraps a dynamic.Interface and memoizes List results
// for the lifetime of ONE observation cycle. The watch/bot poll loop runs an
// inventory sweep and a state scan that independently LIST overlapping resource
// types (the application list twice, the release and reconciliation lists three
// times each per the #533 baseline). Within a single cycle those duplicate LISTs
// become cache hits, lowering per-cycle request and byte cost with no change to
// the observed data.
//
// A fresh wrapper is created for each cycle and discarded at cycle end, so it can
// never serve data across cycles — idle cycles still perform their reads. It is
// read-only: only List is memoized; every other method passes straight through to
// the underlying client. Errors are never cached, so a transient failure on one
// caller does not suppress a later caller's own attempt.
type cycleCachingDynamicClient struct {
	dynamic.Interface
	mu    sync.Mutex
	cache map[listCacheKey]*unstructured.UnstructuredList
	hits  int
}

type listCacheKey struct {
	group, version, resource string
	namespace                string
	options                  string
}

func newCycleCachingDynamicClient(inner dynamic.Interface) *cycleCachingDynamicClient {
	return &cycleCachingDynamicClient{Interface: inner, cache: map[listCacheKey]*unstructured.UnstructuredList{}}
}

func (c *cycleCachingDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &cachingNamespaceable{NamespaceableResourceInterface: c.Interface.Resource(gvr), parent: c, gvr: gvr}
}

type cachingNamespaceable struct {
	dynamic.NamespaceableResourceInterface
	parent *cycleCachingDynamicClient
	gvr    schema.GroupVersionResource
}

func (n *cachingNamespaceable) Namespace(ns string) dynamic.ResourceInterface {
	return &cachingResource{ResourceInterface: n.NamespaceableResourceInterface.Namespace(ns), parent: n.parent, gvr: n.gvr, namespace: ns}
}

func (n *cachingNamespaceable) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return n.parent.list(ctx, n.gvr, "", opts, n.NamespaceableResourceInterface.List)
}

type cachingResource struct {
	dynamic.ResourceInterface
	parent    *cycleCachingDynamicClient
	gvr       schema.GroupVersionResource
	namespace string
}

func (r *cachingResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return r.parent.list(ctx, r.gvr, r.namespace, opts, r.ResourceInterface.List)
}

func (c *cycleCachingDynamicClient) list(ctx context.Context, gvr schema.GroupVersionResource, namespace string, opts metav1.ListOptions, fetch func(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error)) (*unstructured.UnstructuredList, error) {
	key := listCacheKey{group: gvr.Group, version: gvr.Version, resource: gvr.Resource, namespace: namespace, options: listOptionsKey(opts)}
	c.mu.Lock()
	if cached, ok := c.cache[key]; ok {
		c.hits++
		c.mu.Unlock()
		return cached.DeepCopy(), nil
	}
	c.mu.Unlock()
	res, err := fetch(ctx, opts)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[key] = res.DeepCopy()
	c.mu.Unlock()
	return res, nil
}

// listOptionsKey distinguishes list requests that are not interchangeable.
// Pagination (Continue/Limit) and an explicit ResourceVersion change the result,
// so they are part of the key; the poller passes empty options, so in practice
// this collapses to one cache entry per (gvr, namespace).
func listOptionsKey(opts metav1.ListOptions) string {
	return strings.Join([]string{opts.LabelSelector, opts.FieldSelector, opts.ResourceVersion, opts.Continue, strconv.FormatInt(opts.Limit, 10)}, "\x00")
}
