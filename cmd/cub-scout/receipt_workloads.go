// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// loadWorkloadsConvergedLiveFn is the function-variable seam for tests.
// Production reads the cluster; tests swap in prefab observed objects.
var loadWorkloadsConvergedLiveFn = loadWorkloadsConvergedLiveObjects

func runReceiptVerifyWorkloadsConverged(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("--file workloads-converged receipt mode does not accept a positional subject; scope the install with --scope namespace/<ns> or -n <ns>")
	}

	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
	}

	// Parse --grace-window UPFRONT so a bad value rejects before any side
	// effect (stdout, --out, --save).
	var grace time.Duration
	if raw := strings.TrimSpace(receiptGraceWindow); raw != "" {
		parsed, perr := time.ParseDuration(raw)
		if perr != nil {
			return fmt.Errorf("invalid --grace-window %q (expected a Go duration like 5m or 90s): %w", raw, perr)
		}
		if parsed < 0 {
			return fmt.Errorf("invalid --grace-window %q (must not be negative)", raw)
		}
		grace = parsed
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

	observed, err := loadWorkloadsConvergedLiveFn(cmd.Context(), desired, defaultNamespace)
	if err != nil {
		return err
	}
	evidence, err := agent.BuildWorkloadsConvergedEvidence(source, scope, grace, observed, time.Now().UTC())
	if err != nil {
		return err
	}

	inputAttestations, iaErr := collectReceiptInputAttestations()
	if iaErr != nil {
		return iaErr
	}

	stmt, err := agent.BuildWorkloadsConvergedReceipt(agent.BuildWorkloadsConvergedReceiptInput{
		Evidence:          evidence,
		Verifier:          agent.Verifier{Tool: "cub-scout", Version: BuildTag},
		VerifiedAt:        time.Now().UTC(),
		InputAttestations: inputAttestations,
	})
	if err != nil {
		return fmt.Errorf("build workloads-converged receipt: %w", err)
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

// loadWorkloadsConvergedLiveObjects fetches each desired object live and, for
// workloads with a pod selector, the pods that belong to them — so pod-level
// failure reasons (CreateContainerConfigError, CrashLoopBackOff, …) can be
// surfaced even when the workload's own status only says "not ready".
func loadWorkloadsConvergedLiveObjects(ctx context.Context, desired []*unstructured.Unstructured, defaultNamespace string) ([]agent.WorkloadConvergedObservedObject, error) {
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

	podsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	podsByNamespace := map[string][]*unstructured.Unstructured{}
	listPods := func(ns string) []*unstructured.Unstructured {
		if ns == "" {
			return nil
		}
		if cached, ok := podsByNamespace[ns]; ok {
			return cached
		}
		var pods []*unstructured.Unstructured
		if list, lErr := dynClient.Resource(podsGVR).Namespace(ns).List(ctx, metav1.ListOptions{}); lErr == nil {
			for i := range list.Items {
				item := list.Items[i]
				pods = append(pods, &item)
			}
		}
		podsByNamespace[ns] = pods
		return pods
	}

	out := make([]agent.WorkloadConvergedObservedObject, 0, len(desired))
	for _, obj := range desired {
		normalized := obj.DeepCopy()
		mapping, mapErr := objectSetRESTMapping(mapper, normalized)
		if mapErr != nil {
			out = append(out, agent.WorkloadConvergedObservedObject{Desired: normalized, Error: mapErr.Error(), Inconclusive: true})
			continue
		}

		ns := ""
		var live *unstructured.Unstructured
		var getErr error
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			ns = strings.TrimSpace(normalized.GetNamespace())
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
			obs := agent.WorkloadConvergedObservedObject{Desired: normalized, Error: getErr.Error()}
			if !apierrors.IsNotFound(getErr) {
				obs.Inconclusive = true
			}
			out = append(out, obs)
			continue
		}

		obs := agent.WorkloadConvergedObservedObject{Desired: normalized, Live: live}
		if selector := workloadSelectorMatchLabels(live); len(selector) > 0 && ns != "" {
			obs.Pods = matchPodsBySelector(listPods(ns), selector)
		}
		out = append(out, obs)
	}
	return out, nil
}

// workloadSelectorMatchLabels reads spec.selector.matchLabels from a
// controller workload (Deployment/StatefulSet/DaemonSet/ReplicaSet/Job).
// Returns nil for kinds without a selector (Pod, PVC, …).
func workloadSelectorMatchLabels(obj *unstructured.Unstructured) map[string]string {
	if obj == nil {
		return nil
	}
	raw, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector", "matchLabels")
	if err != nil || !found {
		return nil
	}
	return raw
}

// matchPodsBySelector returns pods whose labels are a superset of every
// selector key/value pair.
func matchPodsBySelector(pods []*unstructured.Unstructured, selector map[string]string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, pod := range pods {
		labels := pod.GetLabels()
		match := true
		for k, v := range selector {
			if labels[k] != v {
				match = false
				break
			}
		}
		if match {
			out = append(out, pod)
		}
	}
	return out
}
