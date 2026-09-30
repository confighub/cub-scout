// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	maxRecordedObjectBytes     = 4 << 20
	maxRecordedObjectDocuments = 128
	maxRecordedObjectCount     = 512
	maxRecordedObjectDepth     = 64
)

// recordedObjectIdentity is an exact Kubernetes identity. Namespace may be
// empty only when the source explicitly records an empty namespace string.
type recordedObjectIdentity struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// recordedObjectProvenance describes the exact input bytes, not when they were
// captured. Capture time must come from separately trusted source metadata.
type recordedObjectProvenance struct {
	SHA256      string
	Bytes       int
	Documents   int
	ObjectCount int
}

type recordedObject struct {
	Object     *unstructured.Unstructured
	Provenance recordedObjectProvenance
}

// loadRecordedObject parses a bounded YAML/JSON object stream or generic
// Kubernetes v1/List. It is deliberately byte-reader-only: it has no path,
// kubeconfig, client, clock, or fallback dependencies.
func loadRecordedObject(input io.Reader, want recordedObjectIdentity) (recordedObject, error) {
	if input == nil {
		return recordedObject{}, fmt.Errorf("recorded input is required")
	}
	if !validRecordedIdentityValue(want.APIVersion, false) || !validRecordedIdentityValue(want.Kind, false) ||
		!validRecordedIdentityValue(want.Namespace, true) || !validRecordedIdentityValue(want.Name, false) {
		return recordedObject{}, fmt.Errorf("complete recorded object identity is required")
	}
	raw, err := io.ReadAll(io.LimitReader(input, maxRecordedObjectBytes+1))
	if err != nil {
		return recordedObject{}, fmt.Errorf("read recorded input: %w", err)
	}
	if len(raw) > maxRecordedObjectBytes {
		return recordedObject{}, fmt.Errorf("recorded input exceeds byte limit")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return recordedObject{}, fmt.Errorf("recorded input contains no objects")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var selected *unstructured.Unstructured
	var documents, objectCount int
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			return recordedObject{}, fmt.Errorf("recorded input contains malformed document")
		}
		documents++
		if documents > maxRecordedObjectDocuments {
			return recordedObject{}, fmt.Errorf("recorded input exceeds document limit")
		}
		if len(document.Content) == 0 {
			continue
		}
		root := document.Content[0]
		if err := validateRecordedYAMLNode(root, 0); err != nil {
			return recordedObject{}, err
		}
		object, err := decodeRecordedMapping(root)
		if err != nil {
			return recordedObject{}, err
		}
		apiVersion, kind, err := recordedTypeIdentity(root)
		if err != nil {
			return recordedObject{}, err
		}
		if kind == "List" {
			if apiVersion != "v1" {
				return recordedObject{}, fmt.Errorf("recorded input contains unsupported List type")
			}
			itemsNode := mappingValue(root, "items")
			if itemsNode == nil || itemsNode.Kind != yaml.SequenceNode {
				return recordedObject{}, fmt.Errorf("recorded List has invalid items")
			}
			for _, item := range itemsNode.Content {
				objectCount++
				if objectCount > maxRecordedObjectCount {
					return recordedObject{}, fmt.Errorf("recorded input exceeds object limit")
				}
				candidate, err := decodeRecordedObject(item)
				if err != nil {
					return recordedObject{}, err
				}
				selected, err = selectRecordedObject(selected, candidate, want)
				if err != nil {
					return recordedObject{}, err
				}
			}
			continue
		}
		objectCount++
		if objectCount > maxRecordedObjectCount {
			return recordedObject{}, fmt.Errorf("recorded input exceeds object limit")
		}
		candidate, err := decodeRecordedObjectNode(root, object)
		if err != nil {
			return recordedObject{}, err
		}
		selected, err = selectRecordedObject(selected, candidate, want)
		if err != nil {
			return recordedObject{}, err
		}
	}
	if selected == nil {
		return recordedObject{}, fmt.Errorf("recorded input has no exact identity match")
	}
	digest := sha256.Sum256(raw)
	return recordedObject{
		Object: selected,
		Provenance: recordedObjectProvenance{
			SHA256: hex.EncodeToString(digest[:]), Bytes: len(raw), Documents: documents, ObjectCount: objectCount,
		},
	}, nil
}

func validateRecordedYAMLNode(node *yaml.Node, depth int) error {
	if depth > maxRecordedObjectDepth {
		return fmt.Errorf("recorded input exceeds nesting limit")
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("recorded input aliases are unsupported")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("recorded input has a non-string mapping key")
			}
			if _, ok := seen[key.Value]; ok {
				return fmt.Errorf("recorded input has duplicate mapping keys")
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateRecordedYAMLNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func decodeRecordedMapping(node *yaml.Node) (map[string]interface{}, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("recorded document is not an object")
	}
	serialized, err := yaml.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("recorded document is not a supported object")
	}
	jsonBytes, err := sigsyaml.YAMLToJSON(serialized)
	if err != nil {
		return nil, fmt.Errorf("recorded document is not a supported object")
	}
	var object map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &object); err != nil || object == nil {
		return nil, fmt.Errorf("recorded document is not a supported object")
	}
	return object, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func recordedTypeIdentity(node *yaml.Node) (string, string, error) {
	apiVersion, err := requiredRecordedString(node, "apiVersion")
	if err != nil {
		return "", "", err
	}
	kind, err := requiredRecordedString(node, "kind")
	return apiVersion, kind, err
}

func requiredRecordedString(node *yaml.Node, key string) (string, error) {
	value := mappingValue(node, key)
	if value == nil || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || !validRecordedIdentityValue(value.Value, false) {
		return "", fmt.Errorf("recorded object has missing or invalid identity metadata")
	}
	return value.Value, nil
}

func validRecordedIdentityValue(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return strings.TrimSpace(value) == value
}

func decodeRecordedObject(node *yaml.Node) (*unstructured.Unstructured, error) {
	object, err := decodeRecordedMapping(node)
	if err != nil {
		return nil, err
	}
	return decodeRecordedObjectNode(node, object)
}

func decodeRecordedObjectNode(node *yaml.Node, object map[string]interface{}) (*unstructured.Unstructured, error) {
	if _, kind, err := recordedTypeIdentity(node); err != nil {
		return nil, err
	} else if kind == "List" {
		return nil, fmt.Errorf("nested recorded Lists are unsupported")
	}
	name, _, err := recordedMetadataIdentity(node)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("recorded object has missing or invalid identity metadata")
	}
	return &unstructured.Unstructured{Object: object}, nil
}

func recordedMetadataIdentity(node *yaml.Node) (name, namespace string, err error) {
	metadata := mappingValue(node, "metadata")
	if metadata == nil || metadata.Kind != yaml.MappingNode {
		return "", "", fmt.Errorf("recorded object has missing or invalid identity metadata")
	}
	name, err = requiredRecordedString(metadata, "name")
	if err != nil {
		return "", "", err
	}
	namespaceNode := mappingValue(metadata, "namespace")
	if namespaceNode == nil || namespaceNode.Kind != yaml.ScalarNode || namespaceNode.Tag != "!!str" || !validRecordedIdentityValue(namespaceNode.Value, true) {
		return "", "", fmt.Errorf("recorded object has missing or invalid identity metadata")
	}
	return name, namespaceNode.Value, nil
}

func selectRecordedObject(selected, candidate *unstructured.Unstructured, want recordedObjectIdentity) (*unstructured.Unstructured, error) {
	if candidate.GetAPIVersion() != want.APIVersion || candidate.GetKind() != want.Kind ||
		candidate.GetNamespace() != want.Namespace || candidate.GetName() != want.Name {
		return selected, nil
	}
	if selected != nil {
		return nil, fmt.Errorf("recorded input contains an ambiguous exact identity")
	}
	return candidate.DeepCopy(), nil
}
