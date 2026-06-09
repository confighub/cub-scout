// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

var receiptObjectSetFile string

var loadObjectSetLiveObjectsFn = loadObjectSetLiveObjects

func runReceiptVerifyObjectSet(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("--file object-set receipt mode does not accept a positional subject; scope the install with --scope namespace/<ns> or -n <ns>")
	}

	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
	}

	predicate := agent.PredicateName(strings.TrimSpace(receiptPredicate))
	if predicate != "" && predicate != agent.PredicateObjectSetMatches {
		return fmt.Errorf("--file object-set receipt mode only supports --predicate %s", agent.PredicateObjectSetMatches)
	}

	var failOnSet map[agent.ReceiptVerdict]bool
	failOnRaw := strings.TrimSpace(receiptFailOn)
	if failOnRaw != "" {
		parsed, parseErr := parseReceiptFailOn(failOnRaw)
		if parseErr != nil {
			return parseErr
		}
		failOnSet = parsed
	}

	scope, defaultNamespace, err := resolveObjectSetScope(receiptScope, receiptNamespace)
	if err != nil {
		return err
	}

	desired, source, err := loadObjectSetDesiredManifests(receiptObjectSetFile)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		return fmt.Errorf("--file %q contained no Kubernetes objects", receiptObjectSetFile)
	}

	observed, err := loadObjectSetLiveObjectsFn(cmd.Context(), desired, defaultNamespace)
	if err != nil {
		return err
	}
	evidence, err := agent.BuildObjectSetEvidence(source, scope, observed)
	if err != nil {
		return err
	}

	var inputAttestations []agent.VerifiedAttestationRef
	if len(receiptInputAttestations) > 0 {
		refs, refErr := agent.BuildAttestationRefsFromPaths(receiptInputAttestations, nil)
		if refErr != nil {
			return fmt.Errorf("build input-attestations: %w", refErr)
		}
		inputAttestations = refs
	}

	stmt, err := agent.BuildObjectSetReceipt(agent.BuildObjectSetReceiptInput{
		Evidence: evidence,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt:        time.Now().UTC(),
		InputAttestations: inputAttestations,
	})
	if err != nil {
		return fmt.Errorf("build object-set receipt: %w", err)
	}
	if err := agent.ApplyFreshness(&stmt, receiptTTLDur); err != nil {
		return fmt.Errorf("apply freshness: %w", err)
	}

	var out []byte
	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt: %w", mErr)
		}
		out = buf
	case "ascii":
		out = []byte(renderReceiptASCII(stmt))
	}
	fmt.Println(string(out))

	if outPath := strings.TrimSpace(receiptOut); outPath != "" {
		jsonBytes, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt for --out: %w", mErr)
		}
		if err := writeReceiptOutFile(outPath, jsonBytes); err != nil {
			return err
		}
	}

	if receiptSave {
		if err := saveOneReceipt(cmd, stmt); err != nil {
			return fmt.Errorf("save receipt: %w", err)
		}
	}

	if failOnSet != nil && failOnSet[stmt.Predicate.Verdict] {
		return newExitCodeError(
			fmt.Errorf("receipt verdict %q matches --fail-on %q", stmt.Predicate.Verdict, failOnRaw),
			2,
		)
	}
	return nil
}

func resolveObjectSetScope(scopeFlag, namespaceFlag string) (agent.ObjectSetScope, string, error) {
	scopeFlag = strings.TrimSpace(scopeFlag)
	namespaceFlag = strings.TrimSpace(namespaceFlag)

	if scopeFlag == "" {
		if namespaceFlag != "" {
			return agent.ObjectSetScope{Kind: "namespace", Namespace: namespaceFlag}, namespaceFlag, nil
		}
		return agent.ObjectSetScope{Kind: "mixed"}, "", nil
	}

	if strings.HasPrefix(scopeFlag, "namespace/") {
		ns := strings.TrimSpace(strings.TrimPrefix(scopeFlag, "namespace/"))
		if ns == "" {
			return agent.ObjectSetScope{}, "", fmt.Errorf("--scope %q: empty namespace; expected namespace/<name>", scopeFlag)
		}
		if namespaceFlag != "" && namespaceFlag != ns {
			return agent.ObjectSetScope{}, "", fmt.Errorf("--scope namespace/%s conflicts with --namespace %s", ns, namespaceFlag)
		}
		return agent.ObjectSetScope{Kind: "namespace", Namespace: ns}, ns, nil
	}

	if scopeFlag == "cluster" {
		return agent.ObjectSetScope{Kind: "cluster"}, namespaceFlag, nil
	}

	return agent.ObjectSetScope{}, "", fmt.Errorf("--scope %q: object-set receipt mode supports namespace/<ns> or cluster", scopeFlag)
}

func loadObjectSetDesiredManifests(path string) ([]*unstructured.Unstructured, agent.ObjectSetSource, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, agent.ObjectSetSource{}, fmt.Errorf("--file is required for object-set receipt mode")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, agent.ObjectSetSource{}, fmt.Errorf("stat --file %s: %w", path, err)
	}

	var files []string
	sourceType := "file"
	if info.IsDir() {
		sourceType = "directory"
		if err := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, p)
			}
			return nil
		}); err != nil {
			return nil, agent.ObjectSetSource{}, fmt.Errorf("walk --file directory %s: %w", path, err)
		}
		sort.Strings(files)
	} else {
		files = []string{path}
	}
	if len(files) == 0 {
		return nil, agent.ObjectSetSource{}, fmt.Errorf("--file %s has no YAML files", path)
	}

	h := sha256.New()
	var objects []*unstructured.Unstructured
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			return nil, agent.ObjectSetSource{}, fmt.Errorf("read %s: %w", file, readErr)
		}
		hashObjectSetSourceFile(h, path, file, data)
		decoded, decErr := decodeObjectSetYAML(data, file)
		if decErr != nil {
			return nil, agent.ObjectSetSource{}, decErr
		}
		objects = append(objects, decoded...)
	}

	source := agent.ObjectSetSource{
		Type:   sourceType,
		Ref:    path,
		Digest: hex.EncodeToString(h.Sum(nil)),
	}
	return objects, source, nil
}

func hashObjectSetSourceFile(h hash.Hash, root, file string, data []byte) {
	rel := file
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		if r, relErr := filepath.Rel(root, file); relErr == nil {
			rel = r
		}
	}
	h.Write([]byte(rel))
	h.Write([]byte{0})
	h.Write(data)
	h.Write([]byte{0})
}

func decodeObjectSetYAML(data []byte, file string) ([]*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var out []*unstructured.Unstructured
	docIndex := 0
	for {
		var raw map[string]interface{}
		err := decoder.Decode(&raw)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode %s document %d: %w", file, docIndex+1, err)
		}
		docIndex++
		if len(raw) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{Object: raw}
		if obj.GetKind() == "List" {
			items, found, _ := unstructured.NestedSlice(raw, "items")
			if !found {
				continue
			}
			for i, item := range items {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("decode %s document %d list item %d: expected object", file, docIndex, i)
				}
				itemObj := &unstructured.Unstructured{Object: itemMap}
				if err := validateObjectSetDesiredObject(itemObj, file, docIndex); err != nil {
					return nil, err
				}
				out = append(out, itemObj)
			}
			continue
		}
		if err := validateObjectSetDesiredObject(obj, file, docIndex); err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}

func validateObjectSetDesiredObject(obj *unstructured.Unstructured, file string, docIndex int) error {
	if obj.GetAPIVersion() == "" || obj.GetKind() == "" || obj.GetName() == "" {
		return fmt.Errorf("decode %s document %d: Kubernetes object must have apiVersion, kind, and metadata.name", file, docIndex)
	}
	return nil
}

func loadObjectSetLiveObjects(ctx context.Context, desired []*unstructured.Unstructured, defaultNamespace string) ([]agent.ObjectSetObservedObject, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build discovery client: %w", err)
	}
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("discover API resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	out := make([]agent.ObjectSetObservedObject, 0, len(desired))
	for _, obj := range desired {
		normalized := obj.DeepCopy()
		mapping, mapErr := objectSetRESTMapping(mapper, normalized)
		if mapErr != nil {
			out = append(out, agent.ObjectSetObservedObject{
				Desired:      normalized,
				Error:        mapErr.Error(),
				Inconclusive: true,
			})
			continue
		}

		var live *unstructured.Unstructured
		var getErr error
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			ns := strings.TrimSpace(normalized.GetNamespace())
			if ns == "" {
				ns = strings.TrimSpace(defaultNamespace)
			}
			if ns == "" {
				ns = "default"
			}
			normalized.SetNamespace(ns)
			live, getErr = dynClient.Resource(mapping.Resource).Namespace(ns).Get(ctx, normalized.GetName(), metav1.GetOptions{})
		} else {
			normalized.SetNamespace("")
			live, getErr = dynClient.Resource(mapping.Resource).Get(ctx, normalized.GetName(), metav1.GetOptions{})
		}
		if getErr != nil {
			obs := agent.ObjectSetObservedObject{
				Desired: normalized,
				Error:   getErr.Error(),
			}
			if !apierrors.IsNotFound(getErr) {
				obs.Inconclusive = true
			}
			out = append(out, obs)
			continue
		}
		out = append(out, agent.ObjectSetObservedObject{Desired: normalized, Live: live})
	}
	return out, nil
}

type restMapping interface {
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

func objectSetRESTMapping(mapper restMapping, obj *unstructured.Unstructured) (*meta.RESTMapping, error) {
	gvk := schema.FromAPIVersionAndKind(obj.GetAPIVersion(), obj.GetKind())
	if gvk.Empty() {
		return nil, fmt.Errorf("empty apiVersion/kind for %s", obj.GetName())
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("map %s %s: %w", obj.GetAPIVersion(), obj.GetKind(), err)
	}
	return mapping, nil
}

func writeReceiptOutFile(outPath string, jsonBytes []byte) error {
	storeDir, sdErr := agent.DefaultStoreDir()
	if sdErr == nil {
		absOut, aErr := filepath.Abs(outPath)
		absStore, asErr := filepath.Abs(storeDir)
		if aErr == nil && asErr == nil {
			if rel, rErr := filepath.Rel(absStore, absOut); rErr == nil &&
				!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
				return fmt.Errorf(
					"refusing to write --out to %q because it is under the receipt store %q; use --save for immutable store writes, or pick a path outside the store for ad-hoc --out files",
					absOut, absStore,
				)
			}
		}
	}
	if writeErr := os.WriteFile(outPath, append(jsonBytes, '\n'), 0o644); writeErr != nil {
		return fmt.Errorf("write receipt to %s: %w", outPath, writeErr)
	}
	return nil
}
