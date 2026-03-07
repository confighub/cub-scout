// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
)

func isSyntheticChangeSet(item map[string]interface{}) bool {
	for _, kv := range syntheticMetadataMaps(item) {
		for rawKey, rawVal := range kv {
			key := strings.ToLower(strings.TrimSpace(rawKey))
			val := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", rawVal)))

			switch key {
			case "synthetic", "demo":
				if isTruthyValue(val) {
					return true
				}
			case "source":
				if val == "cub-scout-demo-seed" {
					return true
				}
			}
		}
	}
	return false
}

func syntheticMetadataMaps(item map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, 8)

	appendMaps := func(container map[string]interface{}) {
		for _, key := range []string{"Labels", "labels", "Annotations", "annotations"} {
			if m, ok := container[key].(map[string]interface{}); ok && m != nil {
				out = append(out, m)
			}
		}
	}

	appendMaps(item)
	if meta, ok := item["Metadata"].(map[string]interface{}); ok && meta != nil {
		appendMaps(meta)
	}
	if meta, ok := item["metadata"].(map[string]interface{}); ok && meta != nil {
		appendMaps(meta)
	}

	return out
}

func isTruthyValue(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}
