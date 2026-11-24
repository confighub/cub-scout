// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	RevertVerdictLikelyReverted = "likely-reverted"
	RevertVerdictNotExpected    = "not-expected"
	RevertVerdictUnknown        = "unknown"

	RevertReasonArgoSelfHealEnabled  = "argo-self-heal-enabled"
	RevertReasonArgoSelfHealDisabled = "argo-self-heal-disabled"
	RevertReasonArgoFieldIgnored     = "argo-field-ignored"
	RevertReasonFluxActive           = "flux-reconciling-active"
	RevertReasonFluxSuspended        = "flux-deployer-suspended"
	RevertReasonNoOwnerMarker        = "no-owner-marker"
	RevertReasonDeployerNotRead      = "deployer-settings-not-read"
	RevertReasonUnsupported          = "unsupported-controller"

	RevertRestsOnControllerSpec = "controller-spec"
	RevertRestsOnObjectMetadata = "object-metadata"
)

// FieldRevertPrediction states whether a direct change to a field is expected
// to be reverted by the owning GitOps controller, based strictly on observed
// controller spec settings (never guessing when settings were not read).
type FieldRevertPrediction struct {
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason"`
	RestsOn     string `json:"restsOn"`
	DeployerRef string `json:"deployerRef,omitempty"`
}

// PredictFieldRevert computes whether a direct edit to path on a resource of
// targetKind with expected owner is likely to be reverted by its controller,
// using only the observed deployer object's spec. When deployer is nil or the
// owner is unsupported, it returns RevertVerdictUnknown.
func PredictFieldRevert(owner Ownership, deployer *unstructured.Unstructured, targetKind, path string) FieldRevertPrediction {
	switch owner.Type {
	case "", OwnerUnknown, OwnerKubernetes:
		return FieldRevertPrediction{
			Verdict: RevertVerdictUnknown,
			Reason:  RevertReasonNoOwnerMarker,
			RestsOn: RevertRestsOnObjectMetadata,
		}
	case OwnerFlux:
		if deployer == nil {
			return FieldRevertPrediction{
				Verdict: RevertVerdictUnknown,
				Reason:  RevertReasonDeployerNotRead,
				RestsOn: RevertRestsOnObjectMetadata,
			}
		}
		ref := formatDeployerRef(deployer)
		suspended, _, _ := unstructured.NestedBool(deployer.Object, "spec", "suspend")
		if suspended {
			return FieldRevertPrediction{
				Verdict:     RevertVerdictNotExpected,
				Reason:      RevertReasonFluxSuspended,
				RestsOn:     RevertRestsOnControllerSpec,
				DeployerRef: ref,
			}
		}
		return FieldRevertPrediction{
			Verdict:     RevertVerdictLikelyReverted,
			Reason:      RevertReasonFluxActive,
			RestsOn:     RevertRestsOnControllerSpec,
			DeployerRef: ref,
		}
	case OwnerArgo:
		if deployer == nil {
			return FieldRevertPrediction{
				Verdict: RevertVerdictUnknown,
				Reason:  RevertReasonDeployerNotRead,
				RestsOn: RevertRestsOnObjectMetadata,
			}
		}
		ref := formatDeployerRef(deployer)
		if argoIgnoresFieldPath(deployer, targetKind, path) {
			return FieldRevertPrediction{
				Verdict:     RevertVerdictNotExpected,
				Reason:      RevertReasonArgoFieldIgnored,
				RestsOn:     RevertRestsOnControllerSpec,
				DeployerRef: ref,
			}
		}
		selfHeal, _, _ := unstructured.NestedBool(deployer.Object, "spec", "syncPolicy", "automated", "selfHeal")
		if selfHeal {
			return FieldRevertPrediction{
				Verdict:     RevertVerdictLikelyReverted,
				Reason:      RevertReasonArgoSelfHealEnabled,
				RestsOn:     RevertRestsOnControllerSpec,
				DeployerRef: ref,
			}
		}
		return FieldRevertPrediction{
			Verdict:     RevertVerdictNotExpected,
			Reason:      RevertReasonArgoSelfHealDisabled,
			RestsOn:     RevertRestsOnControllerSpec,
			DeployerRef: ref,
		}
	default:
		return FieldRevertPrediction{
			Verdict: RevertVerdictUnknown,
			Reason:  RevertReasonUnsupported,
			RestsOn: RevertRestsOnObjectMetadata,
		}
	}
}

func formatDeployerRef(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}
	kind := strings.TrimSpace(obj.GetKind())
	ns := strings.TrimSpace(obj.GetNamespace())
	name := strings.TrimSpace(obj.GetName())
	if kind == "" || name == "" {
		return ""
	}
	if ns != "" {
		return kind + "/" + ns + "/" + name
	}
	return kind + "/" + name
}

func argoIgnoresFieldPath(app *unstructured.Unstructured, targetKind, canonicalPath string) bool {
	if app == nil || canonicalPath == "" {
		return false
	}
	entries, found, err := unstructured.NestedSlice(app.Object, "spec", "ignoreDifferences")
	if err != nil || !found || len(entries) == 0 {
		return false
	}
	pointer := canonicalPathToJSONPointer(canonicalPath)
	for _, raw := range entries {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if ruleKind, _ := rule["kind"].(string); ruleKind != "" && targetKind != "" && !strings.EqualFold(ruleKind, targetKind) {
			continue
		}
		if pointers, ok := rule["jsonPointers"].([]interface{}); ok {
			for _, p := range pointers {
				if ps, ok := p.(string); ok && ps != "" && (ps == pointer || strings.HasPrefix(pointer, strings.TrimSuffix(ps, "/")+"/")) {
					return true
				}
			}
		}
		if jqExprs, ok := rule["jqPathExpressions"].([]interface{}); ok {
			for _, expr := range jqExprs {
				if es, ok := expr.(string); ok && strings.TrimSpace(es) == canonicalPath {
					return true
				}
			}
		}
	}
	return false
}

func canonicalPathToJSONPointer(canonicalPath string) string {
	segs, err := parseCanonicalPathSegments(canonicalPath)
	if err != nil || len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		if seg.kind != pathSegField {
			return ""
		}
		b.WriteByte('/')
		escaped := strings.ReplaceAll(seg.field, "~", "~0")
		escaped = strings.ReplaceAll(escaped, "/", "~1")
		b.WriteString(escaped)
	}
	return b.String()
}

type pathSegKind int

const (
	pathSegField pathSegKind = iota
	pathSegIndex
	pathSegKeyMatch
)

type pathSegment struct {
	kind    pathSegKind
	field   string
	index   int
	matchK  string
	matchV  string
}

// LookupCanonicalFieldValue walks obj.Object along a canonical fieldpath string
// (for example `.spec.replicas`, `.metadata.labels["app"]`, or
// `.spec.template.spec.containers[name="checkout"].image`).
// Returns (value, true) if the exact path exists, or (nil, false) otherwise.
func LookupCanonicalFieldValue(obj *unstructured.Unstructured, path string) (interface{}, bool) {
	if obj == nil || obj.Object == nil {
		return nil, false
	}
	if err := ValidateCanonicalFieldPath(path); err != nil {
		return nil, false
	}
	segs, err := parseCanonicalPathSegments(path)
	if err != nil || len(segs) == 0 {
		return nil, false
	}

	var cur interface{} = obj.Object
	for _, seg := range segs {
		switch seg.kind {
		case pathSegField:
			m, ok := cur.(map[string]interface{})
			if !ok {
				return nil, false
			}
			next, exists := m[seg.field]
			if !exists {
				return nil, false
			}
			cur = next
		case pathSegIndex:
			s, ok := cur.([]interface{})
			if !ok || seg.index < 0 || seg.index >= len(s) {
				return nil, false
			}
			cur = s[seg.index]
		case pathSegKeyMatch:
			s, ok := cur.([]interface{})
			if !ok {
				return nil, false
			}
			matched := false
			for _, item := range s {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if fmt.Sprint(m[seg.matchK]) == seg.matchV {
					cur = m
					matched = true
					break
				}
			}
			if !matched {
				return nil, false
			}
		}
	}
	return cur, true
}

func parseCanonicalPathSegments(path string) ([]pathSegment, error) {
	if !strings.HasPrefix(path, ".") {
		return nil, fmt.Errorf("path must start with '.'")
	}
	var segs []pathSegment
	i := 0
	n := len(path)
	for i < n {
		switch path[i] {
		case '.':
			i++
			start := i
			for i < n && path[i] != '.' && path[i] != '[' {
				i++
			}
			if i == start {
				return nil, fmt.Errorf("empty field segment at %d", i)
			}
			segs = append(segs, pathSegment{kind: pathSegField, field: path[start:i]})
		case '[':
			i++
			if i >= n {
				return nil, fmt.Errorf("unterminated '['")
			}
			if path[i] == '"' {
				// Map key: ["foo.bar"]
				val, next, err := readQuotedSegment(path, i)
				if err != nil || next >= n || path[next] != ']' {
					return nil, fmt.Errorf("malformed quoted key at %d", i)
				}
				segs = append(segs, pathSegment{kind: pathSegField, field: val})
				i = next + 1
			} else {
				// Either numeric index [0] or key match [name="checkout"]
				start := i
				for i < n && path[i] != ']' && path[i] != '=' {
					i++
				}
				if i >= n {
					return nil, fmt.Errorf("unterminated '['")
				}
				if path[i] == ']' {
					idx, err := strconv.Atoi(path[start:i])
					if err != nil {
						return nil, fmt.Errorf("invalid index %q", path[start:i])
					}
					segs = append(segs, pathSegment{kind: pathSegIndex, index: idx})
					i++
				} else {
					key := path[start:i]
					i++ // skip '='
					if i >= n || path[i] != '"' {
						return nil, fmt.Errorf("expected quoted value after '='")
					}
					val, next, err := readQuotedSegment(path, i)
					if err != nil || next >= n || path[next] != ']' {
						return nil, fmt.Errorf("malformed keyed selector")
					}
					segs = append(segs, pathSegment{kind: pathSegKeyMatch, matchK: key, matchV: val})
					i = next + 1
				}
			}
		default:
			return nil, fmt.Errorf("unexpected char %q at %d", path[i], i)
		}
	}
	return segs, nil
}

func readQuotedSegment(s string, start int) (string, int, error) {
	if start >= len(s) || s[start] != '"' {
		return "", start, fmt.Errorf("expected '\"'")
	}
	var b strings.Builder
	i := start + 1
	for i < len(s) {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			i++
			continue
		}
		if ch == '"' {
			return b.String(), i + 1, nil
		}
		b.WriteByte(ch)
		i++
	}
	return "", i, fmt.Errorf("unterminated quote")
}
