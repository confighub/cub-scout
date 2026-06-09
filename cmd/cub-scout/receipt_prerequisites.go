// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// prerequisiteSpec is the declared set of required cluster facts (the
// "target facts" helm-expt models). cub-scout consumes this list; it does
// not infer prerequisites from a chart.
type prerequisiteSpec struct {
	RequiredCRDs           []string         `json:"requiredCRDs"`
	RequiredSecrets        []requiredSecret `json:"requiredSecrets"`
	RequiredNamespaces     []string         `json:"requiredNamespaces"`
	RequiredStorageClasses []string         `json:"requiredStorageClasses"`
	RequiredIngressClasses []string         `json:"requiredIngressClasses"`
}

type requiredSecret struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Keys      []string `json:"keys"`
}

func (s prerequisiteSpec) count() int {
	return len(s.RequiredCRDs) + len(s.RequiredSecrets) + len(s.RequiredNamespaces) +
		len(s.RequiredStorageClasses) + len(s.RequiredIngressClasses)
}

var (
	prereqCRDGVR          = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	prereqSecretGVR       = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	prereqNamespaceGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	prereqStorageClassGVR = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
	prereqIngressClassGVR = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingressclasses"}
)

// loadPrerequisitesLiveFn is the function-variable seam for tests.
var loadPrerequisitesLiveFn = loadPrerequisitesLive

func runReceiptVerifyPrerequisites(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("--prerequisites receipt mode does not accept a positional subject; scope with --scope namespace/<ns> or -n <ns>")
	}
	if strings.TrimSpace(receiptObjectSetFile) != "" {
		return fmt.Errorf("--prerequisites cannot be combined with --file (object-set / workloads-converged); run prerequisites-met as its own receipt")
	}
	if strings.TrimSpace(receiptPrerequisitesFile) == "" {
		return fmt.Errorf("--predicate prerequisites-met requires --prerequisites <file>")
	}

	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
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

	spec, source, err := loadPrerequisitesSpec(receiptPrerequisitesFile)
	if err != nil {
		return err
	}
	if spec.count() == 0 {
		return fmt.Errorf("--prerequisites %q declared no required facts (requiredCRDs / requiredSecrets / requiredNamespaces / requiredStorageClasses / requiredIngressClasses)", receiptPrerequisitesFile)
	}

	facts, err := loadPrerequisitesLiveFn(cmd.Context(), spec, defaultNamespace)
	if err != nil {
		return err
	}
	evidence, err := agent.BuildPrerequisitesEvidence(source, scope, facts)
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

	stmt, err := agent.BuildPrerequisitesReceipt(agent.BuildPrerequisitesReceiptInput{
		Evidence:          evidence,
		Verifier:          agent.Verifier{Tool: "cub-scout", Version: BuildTag},
		VerifiedAt:        time.Now().UTC(),
		InputAttestations: inputAttestations,
	})
	if err != nil {
		return fmt.Errorf("build prerequisites receipt: %w", err)
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

func loadPrerequisitesSpec(path string) (prerequisiteSpec, agent.ObjectSetSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return prerequisiteSpec{}, agent.ObjectSetSource{}, fmt.Errorf("read --prerequisites %s: %w", path, err)
	}
	var spec prerequisiteSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return prerequisiteSpec{}, agent.ObjectSetSource{}, fmt.Errorf("parse --prerequisites %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	source := agent.ObjectSetSource{Type: "file", Ref: path, Digest: hex.EncodeToString(sum[:])}
	return spec, source, nil
}

// loadPrerequisitesLive checks each declared fact against the live cluster.
func loadPrerequisitesLive(ctx context.Context, spec prerequisiteSpec, defaultNamespace string) ([]agent.PrerequisiteFactResult, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}

	clusterPresence := func(kind, name string, gvr schema.GroupVersionResource, detail string) agent.PrerequisiteFactResult {
		f := agent.PrerequisiteFactResult{Kind: kind, Name: name}
		obj, gErr := dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		switch {
		case gErr == nil:
			f.Status = agent.PrerequisitePresent
			if detail == "crd" {
				if crdEstablished(obj) {
					f.Detail = "established"
				} else {
					f.Status = agent.PrerequisiteMissing
					f.Detail = "exists but not Established"
				}
			}
		case apierrors.IsNotFound(gErr):
			f.Status = agent.PrerequisiteMissing
			f.Error = gErr.Error()
		default:
			f.Status = agent.PrerequisiteInconclusive
			f.Error = gErr.Error()
		}
		return f
	}

	var facts []agent.PrerequisiteFactResult
	for _, name := range spec.RequiredCRDs {
		facts = append(facts, clusterPresence(agent.PrerequisiteKindCRD, name, prereqCRDGVR, "crd"))
	}
	for _, rs := range spec.RequiredSecrets {
		ns := strings.TrimSpace(rs.Namespace)
		if ns == "" {
			ns = strings.TrimSpace(defaultNamespace)
		}
		if ns == "" {
			ns = "default"
		}
		f := agent.PrerequisiteFactResult{Kind: agent.PrerequisiteKindSecret, Name: rs.Name, Namespace: ns}
		obj, gErr := dyn.Resource(prereqSecretGVR).Namespace(ns).Get(ctx, rs.Name, metav1.GetOptions{})
		switch {
		case gErr == nil:
			missingKeys := secretMissingKeys(obj, rs.Keys)
			if len(missingKeys) == 0 {
				f.Status = agent.PrerequisitePresent
				if len(rs.Keys) > 0 {
					f.Detail = "keys: " + strings.Join(rs.Keys, ",")
				}
			} else {
				f.Status = agent.PrerequisiteMissing
				f.Detail = "missing keys: " + strings.Join(missingKeys, ",")
			}
		case apierrors.IsNotFound(gErr):
			f.Status = agent.PrerequisiteMissing
			f.Error = gErr.Error()
		default:
			f.Status = agent.PrerequisiteInconclusive
			f.Error = gErr.Error()
		}
		facts = append(facts, f)
	}
	for _, name := range spec.RequiredNamespaces {
		facts = append(facts, clusterPresence(agent.PrerequisiteKindNamespace, name, prereqNamespaceGVR, ""))
	}
	for _, name := range spec.RequiredStorageClasses {
		facts = append(facts, clusterPresence(agent.PrerequisiteKindStorageClass, name, prereqStorageClassGVR, ""))
	}
	for _, name := range spec.RequiredIngressClasses {
		facts = append(facts, clusterPresence(agent.PrerequisiteKindIngressClass, name, prereqIngressClassGVR, ""))
	}
	return facts, nil
}

func crdEstablished(crd *unstructured.Unstructured) bool {
	conditions, found, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if fmt.Sprintf("%v", cond["type"]) == "Established" && fmt.Sprintf("%v", cond["status"]) == "True" {
			return true
		}
	}
	return false
}

func secretMissingKeys(secret *unstructured.Unstructured, keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	data, _, _ := unstructured.NestedMap(secret.Object, "data")
	var missing []string
	for _, k := range keys {
		if _, ok := data[k]; !ok {
			missing = append(missing, k)
		}
	}
	return missing
}
