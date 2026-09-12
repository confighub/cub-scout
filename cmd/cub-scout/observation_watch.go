// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// watchBackedClient is a read-only dynamic.Interface whose List, for a fixed set
// of watched resource types, is served from a Kubernetes watch-backed informer
// cache instead of a live LIST. This is Slice 2 of observation efficiency (#539):
// once the informers have synced, an idle poll cycle re-reads inventory from the
// in-process cache and makes no API calls for those types, so a long-running
// watch/bot no longer re-pulls unchanged full inventories every cycle.
//
// Safety and honesty:
//   - Only types whose informer synced are served from cache; every other type,
//     and any List that carries a selector/limit/continue/resourceVersion, falls
//     through to the underlying live client. An unsynced type never reports an
//     empty inventory (which would look like "everything was deleted").
//   - Deletions are reflected in the cache, so the existing prev/curr diff emits
//     the new resource.deleted event (client-go's reflector handles relist and
//     410 Gone resyncs).
//   - It is read-only; only List is intercepted, every other method passes
//     straight through.
type watchBackedClient struct {
	dynamic.Interface
	listers   map[schema.GroupVersionResource]cache.GenericLister
	namespace string
}

func (c *watchBackedClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &watchBackedNamespaceable{NamespaceableResourceInterface: c.Interface.Resource(gvr), parent: c, gvr: gvr}
}

type watchBackedNamespaceable struct {
	dynamic.NamespaceableResourceInterface
	parent *watchBackedClient
	gvr    schema.GroupVersionResource
}

func (n *watchBackedNamespaceable) Namespace(ns string) dynamic.ResourceInterface {
	return &watchBackedResource{ResourceInterface: n.NamespaceableResourceInterface.Namespace(ns), parent: n.parent, gvr: n.gvr, namespace: ns}
}

func (n *watchBackedNamespaceable) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if lister, ok := cacheableList(n.parent, n.gvr, opts); ok {
		return listFromCache(lister, "")
	}
	return n.NamespaceableResourceInterface.List(ctx, opts)
}

type watchBackedResource struct {
	dynamic.ResourceInterface
	parent    *watchBackedClient
	gvr       schema.GroupVersionResource
	namespace string
}

func (r *watchBackedResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if lister, ok := cacheableList(r.parent, r.gvr, opts); ok {
		return listFromCache(lister, r.namespace)
	}
	return r.ResourceInterface.List(ctx, opts)
}

// cacheableList returns the cache lister for a gvr only when the request can be
// served correctly from cache: the type is watched-and-synced and the options
// carry no selector/paging/resourceVersion that the plain lister cannot honor.
func cacheableList(c *watchBackedClient, gvr schema.GroupVersionResource, opts metav1.ListOptions) (cache.GenericLister, bool) {
	if opts.LabelSelector != "" || opts.FieldSelector != "" || opts.Limit != 0 || opts.Continue != "" || opts.ResourceVersion != "" {
		return nil, false
	}
	lister, ok := c.listers[gvr]
	return lister, ok
}

func listFromCache(lister cache.GenericLister, namespace string) (*unstructured.UnstructuredList, error) {
	var listed []runtime.Object
	var err error
	if namespace != "" {
		listed, err = lister.ByNamespace(namespace).List(labels.Everything())
	} else {
		listed, err = lister.List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}
	out := &unstructured.UnstructuredList{}
	for _, o := range listed {
		if u, ok := o.(*unstructured.Unstructured); ok {
			out.Items = append(out.Items, *u.DeepCopy())
		}
	}
	return out, nil
}

// newWatchBackedClient starts informers for the given resource types and returns
// a dynamic.Interface that serves their List from cache. It returns the types
// that actually synced (coverage) and a stop function. Types that do not sync
// within the deadline are not cached (they fall through to live reads).
func newWatchBackedClient(ctx context.Context, base dynamic.Interface, gvrs []schema.GroupVersionResource, namespace string) (*watchBackedClient, []schema.GroupVersionResource, func(), error) {
	scope := namespace
	if scope == "" {
		scope = metav1.NamespaceAll
	}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(base, 0, scope, nil)
	for _, gvr := range gvrs {
		factory.ForResource(gvr).Informer() // register before Start
	}
	stopCh := make(chan struct{})
	factory.Start(stopCh)

	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	synced := factory.WaitForCacheSync(syncCtx.Done())

	listers := map[schema.GroupVersionResource]cache.GenericLister{}
	var syncedGVRs []schema.GroupVersionResource
	for _, gvr := range gvrs {
		if synced[gvr] {
			listers[gvr] = factory.ForResource(gvr).Lister()
			syncedGVRs = append(syncedGVRs, gvr)
		}
	}
	stop := func() { close(stopCh) }
	if len(listers) == 0 {
		stop()
		return nil, nil, func() {}, fmt.Errorf("no watched resource types synced")
	}
	return &watchBackedClient{Interface: base, listers: listers, namespace: namespace}, syncedGVRs, stop, nil
}

// scannerWatchGVRs are resource types the state scan reads but that are NOT part
// of the inventory sweep. They are watch-backed so the scan's per-cycle reads
// (currently the runtime-failure pod read) come from cache too, taking a
// watch-backed idle cycle close to zero API calls. They are deliberately kept
// out of collectWatchResourceList(): pods must never enter the inventory /
// ownership map or emit resource.discovered / resource.deleted events.
func scannerWatchGVRs() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{{Group: "", Version: "v1", Resource: "pods"}}
}

// watchBackedCandidateGVRs is the full set a watch-backed run may watch: the
// inventory sweep types plus the scanner-only types, de-duplicated.
func watchBackedCandidateGVRs() []schema.GroupVersionResource {
	seen := map[schema.GroupVersionResource]bool{}
	out := make([]schema.GroupVersionResource, 0)
	for _, gvr := range append(collectWatchResourceList(), scannerWatchGVRs()...) {
		if !seen[gvr] {
			seen[gvr] = true
			out = append(out, gvr)
		}
	}
	return out
}

// watchableGVRs filters candidate types to those the API server currently
// serves, so a watch-backed run only opens informers for resources that exist.
func watchableGVRs(cfg *rest.Config, candidate []schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	served, err := servedGVRs(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]schema.GroupVersionResource, 0, len(candidate))
	for _, gvr := range candidate {
		if served[gvr] {
			out = append(out, gvr)
		}
	}
	return out, nil
}

// servedGVRs returns the resource types the API server currently serves, so a
// watch-backed run only opens informers for types that exist (absent CRDs and
// optional controller families are skipped rather than error-looping).
func servedGVRs(cfg *rest.Config) (map[schema.GroupVersionResource]bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	_, lists, err := dc.ServerGroupsAndResources()
	// ServerGroupsAndResources can return a partial-failure error while still
	// yielding usable lists for the groups that responded; tolerate that.
	served := map[schema.GroupVersionResource]bool{}
	for _, l := range lists {
		gv, e := schema.ParseGroupVersion(l.GroupVersion)
		if e != nil {
			continue
		}
		for _, r := range l.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // subresource
			}
			served[gv.WithResource(r.Name)] = true
		}
	}
	if len(served) == 0 && err != nil {
		return nil, err
	}
	return served, nil
}
