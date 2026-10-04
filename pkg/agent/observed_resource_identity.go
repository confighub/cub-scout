// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ObservedResourceScope must come from the served API's scope, not a guess from
// an absent metadata.namespace. This helper performs no discovery or reads.
type ObservedResourceScope string

const (
	ObservedResourceNamespaced ObservedResourceScope = "namespaced"
	ObservedResourceCluster    ObservedResourceScope = "cluster"
)

// ObservedResourceIdentity retains an observed object's version separately from
// the group/kind/instance merge key. It is a P4 library foundation, not a command
// output contract. The caller must collect the object and cluster evidence using
// the same captured client. This value cannot prove that association, atomicity,
// current state, ownership or a connected Target binding.
type ObservedResourceIdentity struct {
	ClusterID       string                `json:"clusterId"`
	ClusterIDSource string                `json:"clusterIdSource"`
	APIVersion      string                `json:"apiVersion"`
	Group           string                `json:"group"`
	Kind            string                `json:"kind"`
	Namespace       string                `json:"namespace"`
	Name            string                `json:"name"`
	UID             string                `json:"uid"`
	Scope           ObservedResourceScope `json:"scope"`
}

// NewObservedResourceIdentity refuses unknown cluster/object identity. Context
// and server labels never substitute for an observed UID. No fields are derived
// or written into obj, including missing kind/apiVersion or namespace.
func NewObservedResourceIdentity(cluster ClusterIdentityEvidence, obj *unstructured.Unstructured, scope ObservedResourceScope) (ObservedResourceIdentity, error) {
	if cluster.Identity != "verified" || cluster.ObservedAt == nil || cluster.ObservedAt.IsZero() || cluster.Omission != "" || obj == nil {
		return ObservedResourceIdentity{}, fmt.Errorf("observed cluster and object identity required")
	}
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return ObservedResourceIdentity{}, fmt.Errorf("observed object API version required")
	}
	ref := ObservedResourceIdentity{
		ClusterID: cluster.ID, ClusterIDSource: cluster.IDSource,
		APIVersion: obj.GetAPIVersion(), Group: gv.Group, Kind: obj.GetKind(),
		Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), Scope: scope,
	}
	if err := ref.validate(); err != nil {
		return ObservedResourceIdentity{}, err
	}
	// GetNamespace returns empty for a malformed non-string too. For a
	// cluster-scoped object, absence is permitted; an explicitly malformed field
	// is not.
	if raw, present, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "namespace"); err != nil {
		return ObservedResourceIdentity{}, fmt.Errorf("invalid observed namespace")
	} else if present {
		if _, ok := raw.(string); !ok {
			return ObservedResourceIdentity{}, fmt.Errorf("invalid observed namespace")
		}
	}
	return ref, nil
}

func (ref ObservedResourceIdentity) validate() error {
	validUID := func(value string) bool {
		return strings.TrimSpace(value) != "" && len(value) <= 512 && utf8.ValidString(value) && !strings.ContainsRune(value, utf8.RuneError)
	}
	if ref.ClusterIDSource != "v1/Namespace/kube-system" || !validUID(ref.ClusterID) || !validUID(ref.UID) {
		return fmt.Errorf("observed cluster and object UIDs required")
	}
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || gv.Group != ref.Group || !boundedVersionPattern.MatchString(gv.Version) || len(ref.APIVersion) > 512 || (gv.Group != "" && len(validation.IsDNS1123Subdomain(gv.Group)) != 0) || len(ref.Kind) > 512 || !boundedKindPattern.MatchString(ref.Kind) {
		return fmt.Errorf("exact observed API version, group and kind required")
	}
	if len(validation.IsDNS1123Subdomain(ref.Name)) != 0 {
		return fmt.Errorf("exact observed object name required")
	}
	switch ref.Scope {
	case ObservedResourceNamespaced:
		if len(validation.IsDNS1123Label(ref.Namespace)) != 0 {
			return fmt.Errorf("exact observed namespace required")
		}
	case ObservedResourceCluster:
		if ref.Namespace != "" {
			return fmt.Errorf("cluster-scoped identity cannot contain a namespace")
		}
	default:
		return fmt.Errorf("explicit served resource scope required")
	}
	return nil
}

// MergeKey distinguishes clusters, API groups and recreated object instances.
// Served API version is deliberately excluded: it does not change a group/kind
// object's UID. Canonical JSON avoids delimiter ambiguity; exported fields are
// revalidated so a caller cannot accidentally key a mutated or zero reference.
func (ref ObservedResourceIdentity) MergeKey() (string, error) {
	if err := ref.validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal([7]string{ref.ClusterIDSource, ref.ClusterID, ref.Group, ref.Kind, ref.Namespace, ref.Name, ref.UID})
	return string(encoded), err
}
