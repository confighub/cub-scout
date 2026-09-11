// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kjson "sigs.k8s.io/json"
)

const ConfigHubOriginAnnotation = "confighub.com/origin"

var originIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

// ConfigHubOriginEvidence is observed annotation metadata, not verified source,
// ownership, release membership, or a binding from a Target to a kube context.
type ConfigHubOriginEvidence struct {
	Source      string `json:"source"`
	SpaceID     string `json:"spaceId"`
	SpaceSlug   string `json:"spaceSlug,omitempty"`
	UnitID      string `json:"unitId,omitempty"`
	UnitSlug    string `json:"unitSlug"`
	RevisionNum *int64 `json:"revisionNum,omitempty"`
}

func (e ConfigHubOriginEvidence) Summary() string {
	parts := []string{"spaceId=" + e.SpaceID, "unitSlug=" + e.UnitSlug}
	if e.SpaceSlug != "" {
		parts = append(parts, "spaceSlug="+e.SpaceSlug)
	}
	if e.UnitID != "" {
		parts = append(parts, "unitId="+e.UnitID)
	}
	if e.RevisionNum != nil {
		parts = append(parts, "revisionNum="+strconv.FormatInt(*e.RevisionNum, 10))
	}
	return strings.Join(parts, " ") + " (observed annotation; not independently verified)"
}

// BuildConfigHubOriginEvidence reads the current bundle format only. Legacy
// fields can invalidate conflicting evidence, but cannot fill gaps in it.
func BuildConfigHubOriginEvidence(obj *unstructured.Unstructured) (*ConfigHubOriginEvidence, *Omission) {
	omit := func(reason, severity string) (*ConfigHubOriginEvidence, *Omission) {
		return nil, &Omission{Missing: "confighub-origin", Reason: reason, Severity: severity}
	}
	if obj == nil {
		return omit("No object available for origin annotation evidence.", "info")
	}
	annotations, labels := obj.GetAnnotations(), obj.GetLabels()
	raw, exists := annotations[ConfigHubOriginAnnotation]
	if !exists {
		return omit("No combined origin annotation observed; source provenance is not established by this read.", "info")
	}
	if len(raw) == 0 || len(raw) > 8192 {
		return omit("Origin annotation is empty or exceeds the 8 KiB evidence limit.", "warning")
	}
	// Use case-sensitive, duplicate-rejecting parsing with integer preservation.
	// Unknown future keys are ignored and never copied into evidence or errors.
	var fields map[string]interface{}
	strictErrors, err := kjson.UnmarshalStrict([]byte(raw), &fields, kjson.DisallowDuplicateFields)
	if err != nil || len(strictErrors) > 0 || fields == nil {
		return omit("Origin annotation must be an unambiguous JSON object.", "warning")
	}
	evidence := &ConfigHubOriginEvidence{Source: "annotation:" + ConfigHubOriginAnnotation}
	for _, field := range []struct {
		key      string
		value    *string
		required bool
	}{
		{"spaceId", &evidence.SpaceID, true},
		{"unitSlug", &evidence.UnitSlug, true},
		{"spaceSlug", &evidence.SpaceSlug, false},
		{"unitId", &evidence.UnitID, false},
	} {
		value, present := fields[field.key]
		if !present && !field.required {
			continue
		}
		text, ok := value.(string)
		if !ok || (text == "" && field.required) || (text != "" && !originIdentifier.MatchString(text)) {
			return omit(fmt.Sprintf("Origin %s must be a supported identifier (1-256 ASCII letters, digits, dots, underscores, or hyphens; starts with a letter or digit).", field.key), "warning")
		}
		*field.value = text
	}
	if value, present := fields["revisionNum"]; present {
		revision, ok := value.(int64)
		if !ok || revision < 0 {
			return omit("Origin revisionNum must be a nonnegative int64 JSON integer.", "warning")
		}
		evidence.RevisionNum = &revision
	}
	for _, field := range []struct{ key, value string }{
		{"SpaceID", evidence.SpaceID}, {"UnitID", evidence.UnitID}, {"UnitSlug", evidence.UnitSlug},
	} {
		if field.value == "" {
			continue
		}
		key := "confighub.com/" + field.key
		for _, metadata := range []map[string]string{annotations, labels} {
			if legacy, present := metadata[key]; present && legacy != field.value {
				return omit("Origin annotation conflicts with legacy "+key+" metadata.", "warning")
			}
		}
	}
	if evidence.RevisionNum != nil {
		for _, metadata := range []map[string]string{annotations, labels} {
			if legacy, present := metadata["confighub.com/RevisionNum"]; present {
				revision, err := strconv.ParseInt(legacy, 10, 64)
				if err != nil || revision != *evidence.RevisionNum {
					return omit("Origin annotation conflicts with legacy confighub.com/RevisionNum metadata.", "warning")
				}
			}
		}
	}
	return evidence, nil
}
