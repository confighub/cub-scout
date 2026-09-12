// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strconv"

	"github.com/confighub/cub-scout/pkg/agent"
)

func releaseCheckMCPTool() mcpTool {
	properties := map[string]interface{}{}
	for key, description := range map[string]string{
		"bundle":               "Exact oci://repository@sha256:digest of literal configuration; not a container image.",
		"oci_layout":           "Optional read-only local OCI layout containing the expected manifest and layer.",
		"controller":           "Exact controller Kind/name (Application or Kustomization supported initially).",
		"api_version":          "Exact controller API version.",
		"controller_namespace": "Controller namespace.",
		"context":              "Explicit target Kubernetes context; never inferred from an intended-state Target name.",
		"controller_context":   "Controller Kubernetes context; defaults to context.",
	} {
		properties[key] = map[string]interface{}{"type": "string", "description": description}
	}
	properties["max_objects"] = map[string]interface{}{"type": "integer", "minimum": 1, "maximum": agent.ReleaseMaxObjects, "description": "Maximum literal desired objects; oversized input fails before cluster reads."}
	properties["check_running_image"] = map[string]interface{}{"type": "boolean", "description": "Also compare the digest running in live pods against the intended image; adds one bounded, selector-scoped pod read per workload. Mutable tags stay UNKNOWN."}
	properties["max_pods"] = map[string]interface{}{"type": "integer", "minimum": 1, "maximum": releasePodCap, "description": "Maximum pods read per workload for check_running_image; coverage past this is reported as partial."}
	return mcpTool{Descriptor: mcpToolDescriptor{Name: "release_check", Description: "Verify an exact OCI configuration release: bundle digest, controller/source/target binding, live authored fields and workload-controller convergence. One-shot bounded reads, no inventory LIST. Optionally compares running-image identity (the digest executing in live pods) with check_running_image, which adds one bounded, selector-scoped pod read per workload and keeps mutable tags UNKNOWN. Returns stage verdicts, omissions, dated request counts and receipts. Not application success, release publication history, or a deployment command.", Annotations: &mcpToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]interface{}{"type": "object", "properties": properties, "required": []string{"bundle", "controller", "api_version", "controller_namespace", "context"}, "additionalProperties": false}}, BuildArgs: func(a map[string]interface{}) ([]string, error) {
		for key, value := range a {
			if _, ok := properties[key]; !ok {
				return nil, fmt.Errorf("unsupported release_check argument %s", key)
			}
			if key != "max_objects" && key != "max_pods" && key != "check_running_image" {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s must be a string", key)
				}
			}
		}
		maxObjects := agent.ReleaseMaxObjects
		if value, ok := a["max_objects"]; ok {
			n, valid := value.(float64)
			if !valid || n < 1 || n > agent.ReleaseMaxObjects || n != float64(int(n)) {
				return nil, fmt.Errorf("max_objects must be an integer from 1 to %d", agent.ReleaseMaxObjects)
			}
			maxObjects = int(n)
		}
		checkRunningImage := false
		if value, ok := a["check_running_image"]; ok {
			b, valid := value.(bool)
			if !valid {
				return nil, fmt.Errorf("check_running_image must be a boolean")
			}
			checkRunningImage = b
		}
		maxPods := 0
		if value, ok := a["max_pods"]; ok {
			n, valid := value.(float64)
			if !valid || n < 1 || n > releasePodCap || n != float64(int(n)) {
				return nil, fmt.Errorf("max_pods must be an integer from 1 to %d", releasePodCap)
			}
			maxPods = int(n)
		}
		o := releaseCheckOptions{Bundle: argString(a, "bundle"), Layout: argString(a, "oci_layout"), Controller: argString(a, "controller"), APIVersion: argString(a, "api_version"), ControllerNamespace: argString(a, "controller_namespace"), Context: argString(a, "context"), ControllerContext: argString(a, "controller_context"), MaxObjects: maxObjects, CheckRunningImage: checkRunningImage, MaxPods: maxPods}
		if _, err := o.validate(); err != nil {
			return nil, err
		}
		args := []string{"release", "check", "--bundle", o.Bundle, "--controller", o.Controller, "--api-version", o.APIVersion, "--controller-namespace", o.ControllerNamespace, "--kube-context", o.Context, "--max-objects", strconv.Itoa(o.MaxObjects), "--format", "json"}
		if o.Layout != "" {
			args = append(args, "--oci-layout", o.Layout)
		}
		if o.ControllerContext != "" {
			args = append(args, "--controller-context", o.ControllerContext)
		}
		if o.CheckRunningImage {
			args = append(args, "--check-running-image")
			if o.MaxPods > 0 {
				args = append(args, "--max-pods", strconv.Itoa(o.MaxPods))
			}
		}
		return args, nil
	}}
}
