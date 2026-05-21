// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// BackResolveResult identifies the source file and line where a resource's
// field is defined in a local git checkout. Stage B of the attribution
// layer.
type BackResolveResult struct {
	// File is the path relative to the search root.
	File string

	// Line is the 1-based line number in File where the field is set.
	Line int
}

// BackResolveGitSource walks the YAML/YML manifest files under rootDir,
// finds the multi-doc YAML document matching (kind, name, namespace), and
// returns the file + line for the requested canonical field path.
//
// fieldPath uses the canonical-path string form ".a.b.c"; only top-level
// and dotted nested keys are supported at stage B. List-key selectors
// like `[name="foo"]` (for container images, etc.) are deferred to a
// later refinement of this function.
//
// Returns (nil, false) when:
//   - rootDir is empty or unreadable
//   - no manifest file contains a matching document
//   - the document does contain the path but it lives in a non-locatable form
//     (e.g., rendered Helm output where the value came from values.yaml)
//
// Helm/Kustomize templated sources are explicitly out of scope at stage B;
// callers should treat absence of a result as "we can show repo+revision
// from A2 but not a file:line."
func BackResolveGitSource(rootDir, kind, name, namespace, fieldPath string) (*BackResolveResult, bool) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, false
	}
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		return nil, false
	}

	pathParts := splitCanonicalPath(fieldPath)
	if len(pathParts) == 0 {
		return nil, false
	}

	var hit *BackResolveResult
	_ = filepath.Walk(rootDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !isYAMLFile(p) {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
		for {
			var node yaml.Node
			if err := decoder.Decode(&node); err != nil {
				break
			}
			if !documentMatches(&node, kind, name, namespace) {
				continue
			}
			if line, ok := findFieldLine(&node, pathParts); ok {
				rel, relErr := filepath.Rel(rootDir, p)
				if relErr != nil {
					rel = p
				}
				hit = &BackResolveResult{File: rel, Line: line}
				return filepath.SkipAll
			}
		}
		return nil
	})

	if hit == nil {
		return nil, false
	}
	return hit, true
}

// splitCanonicalPath splits a canonical field-path string like ".spec.replicas"
// into ["spec", "replicas"]. Leading dot is stripped; empty input yields nil.
func splitCanonicalPath(path string) []string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isYAMLFile(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".yaml" || ext == ".yml"
}

// documentMatches returns true when the YAML document at the given node
// represents a K8s object matching the given kind/name/namespace. A blank
// namespace argument matches any namespace (useful for cluster-scoped
// resources whose namespace metadata is absent).
func documentMatches(node *yaml.Node, kind, name, namespace string) bool {
	mapping := documentMapping(node)
	if mapping == nil {
		return false
	}
	if v := mappingScalarValue(mapping, "kind"); v != kind {
		return false
	}
	metadata := mappingChildMapping(mapping, "metadata")
	if metadata == nil {
		return false
	}
	if v := mappingScalarValue(metadata, "name"); v != name {
		return false
	}
	if namespace != "" {
		if v := mappingScalarValue(metadata, "namespace"); v != namespace {
			return false
		}
	}
	return true
}

// findFieldLine navigates the mapping nodes for the given canonical path
// parts and returns the 1-based line number where the leaf is set. Returns
// (0, false) when any segment is not a mapping or the leaf key is absent.
func findFieldLine(node *yaml.Node, parts []string) (int, bool) {
	mapping := documentMapping(node)
	if mapping == nil {
		return 0, false
	}
	current := mapping
	for i, key := range parts {
		valueNode := mappingValue(current, key)
		if valueNode == nil {
			return 0, false
		}
		if i == len(parts)-1 {
			// Leaf — return the line of the value node (where the actual
			// value appears in the file).
			return valueNode.Line, true
		}
		if valueNode.Kind != yaml.MappingNode {
			return 0, false
		}
		current = valueNode
	}
	return 0, false
}

// documentMapping unwraps a yaml document node down to its mapping.
func documentMapping(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// mappingValue returns the value node for a given key in a mapping, or nil.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// mappingChildMapping returns the value as a mapping node if it's a mapping.
func mappingChildMapping(mapping *yaml.Node, key string) *yaml.Node {
	v := mappingValue(mapping, key)
	if v == nil || v.Kind != yaml.MappingNode {
		return nil
	}
	return v
}

// mappingScalarValue returns the scalar string value at key, or "" if absent.
func mappingScalarValue(mapping *yaml.Node, key string) string {
	v := mappingValue(mapping, key)
	if v == nil || v.Kind != yaml.ScalarNode {
		return ""
	}
	return v.Value
}
