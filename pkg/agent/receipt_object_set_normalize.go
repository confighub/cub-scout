// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Server-normalization profiles for object-set comparison.
//
// Kubernetes API servers drop or normalize certain authored zero-value
// fields on admission (probe initialDelaySeconds: 0, empty caBundle, empty
// RBAC rules, ...). Without normalization, object-set-matches honestly
// reports those as "missing field in live object" — true at the byte level,
// noise at the intent level. A profile is a NAMED, VERSIONED set of such
// rules applied symmetrically to the desired and live sides before
// projection and digesting, and recorded on the receipt so the claim says
// exactly which normalization it assumed.
//
// This profile absorbs the consumer-side prune list that helm-expt
// maintained (scripts/lib/cub-scout-live.mjs pruneApiDroppedNoops) plus the
// RBAC empty-rules case its strict witness surfaced on grafana. Producers
// and verifiers that share a profile name compute identical canonical
// digests for the same rendered set — the basis of the cross-tool digest
// convention (see ComputeRenderedObjectSetDigest).
const NormalizationProfileK8sZeroDefaultsV1 = "k8s-zero-defaults/v1"

// KnownNormalizationProfiles lists the accepted --normalization-profile
// values. Adding a profile is additive; renaming or changing rules in an
// existing profile is a breaking change and requires a new version suffix.
func KnownNormalizationProfiles() []string {
	return []string{NormalizationProfileK8sZeroDefaultsV1}
}

// NormalizeObjectForProfile returns a deep-pruned copy of obj.Object under
// the named profile. Unknown profiles error. The empty profile returns the
// object unchanged.
func NormalizeObjectForProfile(profile string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if profile == "" || obj == nil {
		return obj, nil
	}
	if profile != NormalizationProfileK8sZeroDefaultsV1 {
		return nil, fmt.Errorf("unknown normalization profile %q (known: %v)", profile, KnownNormalizationProfiles())
	}
	cloned := obj.DeepCopy()
	pruned, _ := pruneZeroDefaults(cloned.Object, nil, cloned.GetKind())
	if m, ok := pruned.(map[string]interface{}); ok {
		cloned.Object = m
	}
	return cloned, nil
}

func stringAt(path []string, fromEnd int) string {
	idx := len(path) - fromEnd
	if idx < 0 || idx >= len(path) {
		return ""
	}
	return path[idx]
}

func isIndexSegment(segment string) bool {
	for _, r := range segment {
		if r < '0' || r > '9' {
			return false
		}
	}
	return segment != ""
}

// pruneZeroDefaults walks value with a path of segments (list indices appear
// as decimal strings) and removes the profile's server-dropped zero-value
// noops. Returns the pruned value and whether it should be dropped entirely.
func pruneZeroDefaults(value interface{}, path []string, kind string) (interface{}, bool) {
	last := stringAt(path, 1)
	parent := stringAt(path, 2)
	grandparent := stringAt(path, 3)
	if isIndexSegment(parent) {
		// For list members, the structural parent is one level further out
		// (volumeMounts.3.subPath -> parent volumeMounts).
		parent = grandparent
		grandparent = stringAt(path, 4)
	}

	switch typed := value.(type) {
	case int64:
		if typed == 0 && last == "initialDelaySeconds" &&
			(parent == "livenessProbe" || parent == "readinessProbe" || parent == "startupProbe") {
			return nil, true
		}
		if typed == 0 && last == "minReadySeconds" && parent == "spec" && len(path) == 2 {
			return nil, true
		}
		return value, false
	case string:
		if typed == "" && last == "caBundle" && (parent == "clientConfig" || parent == "spec") {
			return nil, true
		}
		if typed == "" && last == "subPath" && parent == "volumeMounts" {
			return nil, true
		}
		return value, false
	case bool:
		if !typed {
			if (last == "hostIPC" || last == "hostNetwork" || last == "hostPID") &&
				parent == "spec" && grandparent == "template" {
				return nil, true
			}
			if last == "publishNotReadyAddresses" && parent == "spec" && len(path) == 2 {
				return nil, true
			}
		}
		return value, false
	case []interface{}:
		if len(typed) == 0 {
			if (last == "supplementalGroups" || last == "sysctls") && parent == "securityContext" {
				return nil, true
			}
			if last == "rules" && len(path) == 1 && (kind == "Role" || kind == "ClusterRole") {
				// The API server normalizes authored `rules: []` on RBAC
				// objects to an absent field (the grafana watch case).
				return nil, true
			}
		}
		out := make([]interface{}, 0, len(typed))
		for i, item := range typed {
			pruned, drop := pruneZeroDefaults(item, append(path, fmt.Sprintf("%d", i)), kind)
			if !drop {
				out = append(out, pruned)
			}
		}
		return out, false
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, item := range typed {
			pruned, drop := pruneZeroDefaults(item, append(path, key), kind)
			if !drop {
				out[key] = pruned
			}
		}
		if len(out) == 0 && last != "" && parent == "metadata" &&
			(last == "annotations" || last == "labels") {
			return nil, true
		}
		return out, false
	default:
		// YAML decoding may produce int (not int64) in some paths; treat the
		// same as int64 for the zero rules.
		if n, ok := value.(int); ok {
			pruned, drop := pruneZeroDefaults(int64(n), path, kind)
			return pruned, drop
		}
		if f, ok := value.(float64); ok && f == 0 {
			pruned, drop := pruneZeroDefaults(int64(0), path, kind)
			if drop {
				return pruned, true
			}
		}
		return value, false
	}
}

// NormalizeObservedObjects applies the profile to every desired and live
// object in observed, returning a new slice. A nil/empty profile returns
// observed unchanged.
func NormalizeObservedObjects(profile string, observed []ObjectSetObservedObject) ([]ObjectSetObservedObject, error) {
	if profile == "" {
		return observed, nil
	}
	out := make([]ObjectSetObservedObject, 0, len(observed))
	for _, obs := range observed {
		desired, err := NormalizeObjectForProfile(profile, obs.Desired)
		if err != nil {
			return nil, err
		}
		live, err := NormalizeObjectForProfile(profile, obs.Live)
		if err != nil {
			return nil, err
		}
		obs.Desired = desired
		obs.Live = live
		out = append(out, obs)
	}
	return out, nil
}

// ComputeRenderedObjectSetDigest computes the canonical digest of a rendered
// object set: the SHA-256 over RFC 8785 canonical JSON of the sorted
// (id, comparable-object) entries, after optional profile normalization.
//
// CONTRACT: this is byte-identical to the `desiredDigest` an
// object-set-matches receipt records for the same inputs and profile
// (locked by TestRenderedObjectSetDigest_MatchesEvidenceDesiredDigest).
// Producers (e.g. helm-expt render receipts) call `cub-scout receipt digest`
// to stamp this value; verifiers compare it to the receipt subject digest,
// which is what makes cross-tool chaining by digest equality possible.
func ComputeRenderedObjectSetDigest(profile string, objs []*unstructured.Unstructured) (string, int, error) {
	entries := make([]interface{}, 0, len(objs))
	seen := map[string]struct{}{}
	for _, obj := range objs {
		if obj == nil {
			continue
		}
		normalized, err := NormalizeObjectForProfile(profile, obj)
		if err != nil {
			return "", 0, err
		}
		id := NewObjectSetObjectID(normalized)
		if _, dup := seen[id.Key()]; dup {
			return "", 0, fmt.Errorf("duplicate desired object identity %s", id.String())
		}
		seen[id.Key()] = struct{}{}
		entries = append(entries, objectSetDigestEntry(id, ComparableObject(normalized)))
	}
	sortDigestEntries(entries)
	digest, err := digestJSON(entries)
	if err != nil {
		return "", 0, err
	}
	return digest, len(entries), nil
}

func sortDigestEntries(entries []interface{}) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && digestEntryKey(entries[j]) < digestEntryKey(entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}
