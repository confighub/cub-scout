// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/monadic/confighub-agent/pkg/agent"
)

var (
	traceNamespace string
	traceJSON      bool
	traceApp       string // For direct Argo app tracing
	traceReverse   bool   // Reverse trace - walk ownerReferences up
)

// ANSI color codes for colorful output
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

var traceCmd = &cobra.Command{
	Use:   "trace <kind/name> or <kind> <name>",
	Short: "Show the full GitOps ownership chain for a resource",
	Long: `Trace the full ownership chain from Git source to deployed resource.

This command uses 'flux trace' or 'argocd app get' (depending on the resource's
owner) to show the complete delivery pipeline.

Examples:
  # Trace a deployment
  cub-agent trace deployment/nginx -n demo

  # Trace with kind and name separately
  cub-agent trace Deployment nginx -n demo

  # Trace an Argo CD application directly
  cub-agent trace --app frontend-app

  # Reverse trace - start from any resource (e.g., a Pod) and walk up
  cub-agent trace pod/nginx-7d9b8c-x4k2p -n prod --reverse

  # Output as JSON
  cub-agent trace deployment/nginx -n demo --json

The output shows:
  - The full chain from GitRepository → Kustomization/HelmRelease → Resource
  - Status and revision at each level
  - Where in the chain something is broken (if applicable)

Reverse trace (--reverse) walks ownerReferences to find:
  - The K8s ownership chain (Pod → ReplicaSet → Deployment)
  - The GitOps owner (Flux, ArgoCD, Helm, or Native)
`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runTrace,
}

func init() {
	rootCmd.AddCommand(traceCmd)

	traceCmd.Flags().StringVarP(&traceNamespace, "namespace", "n", "", "Namespace of the resource (default: flux-system)")
	traceCmd.Flags().BoolVar(&traceJSON, "json", false, "Output as JSON")
	traceCmd.Flags().StringVar(&traceApp, "app", "", "Trace Argo CD application by name")
	traceCmd.Flags().BoolVarP(&traceReverse, "reverse", "r", false, "Reverse trace - walk ownerReferences up to find GitOps source")
}

func runTrace(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse resource reference
	var kind, name string

	if traceApp != "" {
		// Direct Argo app trace
		kind = "Application"
		name = traceApp
		if traceNamespace == "" {
			traceNamespace = "argocd"
		}
	} else if len(args) == 0 {
		return fmt.Errorf("usage: cub-agent trace <kind/name> or cub-agent trace <kind> <name>")
	} else if len(args) == 1 {
		// Parse kind/name format
		parts := strings.SplitN(args[0], "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid resource format: use kind/name (e.g., deployment/nginx)")
		}
		kind = parts[0]
		name = parts[1]
	} else {
		kind = args[0]
		name = args[1]
	}

	// Normalize kind
	kind = normalizeKind(kind)

	// Default namespace
	if traceNamespace == "" {
		traceNamespace = "flux-system"
	}

	// Handle reverse trace
	if traceReverse {
		return runReverseTrace(ctx, kind, name, traceNamespace)
	}

	// Create appropriate tracer
	var result *agent.TraceResult

	// If --app flag was used, go directly to Argo tracer
	if traceApp != "" {
		tracer := agent.NewArgoTracer()
		if !tracer.Available() {
			return fmt.Errorf("argocd CLI not found - install from https://argo-cd.readthedocs.io/en/stable/cli_installation/")
		}
		appResult, appErr := tracer.TraceApplication(ctx, name)
		if appErr != nil {
			return fmt.Errorf("trace failed: %w", appErr)
		}
		if traceJSON {
			return outputTraceJSON(appResult)
		}
		return outputTraceHuman(appResult)
	}

	// Detect ownership to choose the right tracer
	ownership, err := detectResourceOwnership(ctx, kind, name, traceNamespace)
	if err != nil {
		// Ownership detection failed, try Flux tracer as default
		ownership = &agent.Ownership{Type: agent.OwnerFlux}
	}

	switch ownership.Type {
	case agent.OwnerFlux:
		tracer := agent.NewFluxTracer()
		if !tracer.Available() {
			return fmt.Errorf("flux CLI not found - install from https://fluxcd.io/docs/installation/")
		}
		result, err = tracer.Trace(ctx, kind, name, traceNamespace)

	case agent.OwnerArgo:
		tracer := agent.NewArgoTracer()
		if !tracer.Available() {
			return fmt.Errorf("argocd CLI not found - install from https://argo-cd.readthedocs.io/en/stable/cli_installation/")
		}
		// For Argo, we trace the Application
		if kind == "Application" {
			result, err = tracer.TraceApplication(ctx, name)
		} else {
			// Need to find the owning Application
			if ownership.Name != "" {
				result, err = tracer.TraceApplication(ctx, ownership.Name)
			} else {
				return fmt.Errorf("for Argo-managed resources, use --app flag to specify the Application")
			}
		}

	default:
		// Try Flux first, then Argo, then report not managed
		fluxTracer := agent.NewFluxTracer()
		argoTracer := agent.NewArgoTracer()

		if fluxTracer.Available() {
			result, err = fluxTracer.Trace(ctx, kind, name, traceNamespace)
			if err == nil && result.Error == "" {
				break
			}
		}

		if argoTracer.Available() && kind == "Application" {
			result, err = argoTracer.TraceApplication(ctx, name)
			if err == nil && result.Error == "" {
				break
			}
		}

		if result == nil || (result.Error != "" && !strings.Contains(result.Error, "not managed")) {
			return fmt.Errorf("resource not managed by a detected GitOps tool")
		}
	}

	if err != nil {
		return fmt.Errorf("trace failed: %w", err)
	}

	// Output results
	if traceJSON {
		return outputTraceJSON(result)
	}
	return outputTraceHuman(result)
}

// normalizeKind normalizes resource kind names
func normalizeKind(kind string) string {
	kind = strings.ToLower(kind)
	switch kind {
	case "deploy", "deployment", "deployments":
		return "Deployment"
	case "svc", "service", "services":
		return "Service"
	case "cm", "configmap", "configmaps":
		return "ConfigMap"
	case "secret", "secrets":
		return "Secret"
	case "sts", "statefulset", "statefulsets":
		return "StatefulSet"
	case "ds", "daemonset", "daemonsets":
		return "DaemonSet"
	case "ing", "ingress", "ingresses":
		return "Ingress"
	case "ks", "kustomization", "kustomizations":
		return "Kustomization"
	case "hr", "helmrelease", "helmreleases":
		return "HelmRelease"
	case "gitrepo", "gitrepository", "gitrepositories":
		return "GitRepository"
	case "app", "application", "applications":
		return "Application"
	default:
		// Capitalize first letter
		if len(kind) > 0 {
			return strings.ToUpper(kind[:1]) + kind[1:]
		}
		return kind
	}
}

// detectResourceOwnership fetches the resource and detects its owner
func detectResourceOwnership(ctx context.Context, kind, name, namespace string) (*agent.Ownership, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Map kind to GVR
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unknown resource kind: %s", kind)
	}

	// Fetch the resource
	resource, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Detect ownership
	ownership := agent.DetectOwnership(resource)
	return &ownership, nil
}

// kindToGVR maps a kind to its GroupVersionResource
func kindToGVR(kind string) schema.GroupVersionResource {
	switch kind {
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	case "Service":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	case "ConfigMap":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	case "Secret":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	case "Ingress":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	case "Kustomization":
		return schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	case "HelmRelease":
		return schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	case "GitRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	case "Application":
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	default:
		return schema.GroupVersionResource{}
	}
}

// outputTraceJSON outputs the trace result as JSON
func outputTraceJSON(result *agent.TraceResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputTraceHuman outputs the trace result in human-readable format with colors
func outputTraceHuman(result *agent.TraceResult) error {
	// Header
	fmt.Printf("\n")
	fmt.Printf("%s%sTRACE:%s %s%s%s\n", colorBold, colorCyan, colorReset, colorBold, result.Object.String(), colorReset)
	fmt.Printf("\n")

	if result.Error != "" && len(result.Chain) == 0 {
		fmt.Printf("  %s⚠ %s%s\n\n", colorYellow, result.Error, colorReset)
		return nil
	}

	// Print chain
	for i, link := range result.Chain {
		prefix := "  "
		if i > 0 {
			// Add tree connector with color
			prefix = strings.Repeat("    ", i-1) + fmt.Sprintf("    %s└─▶%s ", colorDim, colorReset)
		}

		// Status icon with color
		var icon, iconColor string
		if link.Ready {
			icon = "✓"
			iconColor = colorGreen
		} else {
			icon = "✗"
			iconColor = colorRed
		}

		// Kind color based on type
		kindColor := colorWhite
		switch link.Kind {
		case "GitRepository", "OCIRepository", "HelmRepository", "Bucket", "Source":
			kindColor = colorPurple
		case "Kustomization", "HelmRelease", "HelmChart":
			kindColor = colorCyan
		case "Application":
			kindColor = colorBlue
		case "Deployment", "StatefulSet", "DaemonSet":
			kindColor = colorGreen
		case "Service", "ConfigMap", "Secret":
			kindColor = colorYellow
		}

		// Main line with colors
		fmt.Printf("%s%s%s%s %s%s%s/%s%s\n", prefix, iconColor, icon, colorReset, kindColor, link.Kind, colorReset, colorBold, link.Name+colorReset)

		// Details (indented) with dim colors
		detailPrefix := strings.Repeat("    ", i) + fmt.Sprintf("    %s│%s ", colorDim, colorReset)
		if i == len(result.Chain)-1 {
			detailPrefix = strings.Repeat("    ", i) + "      "
		}

		if link.Namespace != "" && link.Namespace != result.Object.Namespace {
			fmt.Printf("%s%sNamespace:%s %s\n", detailPrefix, colorDim, colorReset, link.Namespace)
		}
		if link.URL != "" {
			fmt.Printf("%s%sURL:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorBlue, link.URL, colorReset)
		}
		if link.Path != "" {
			fmt.Printf("%s%sPath:%s %s\n", detailPrefix, colorDim, colorReset, link.Path)
		}
		if link.Revision != "" {
			fmt.Printf("%s%sRevision:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorPurple, link.Revision, colorReset)
		}
		if link.Status != "" {
			statusColor := colorGreen
			if !link.Ready {
				statusColor = colorYellow
			}
			fmt.Printf("%s%sStatus:%s %s%s%s\n", detailPrefix, colorDim, colorReset, statusColor, link.Status, colorReset)
		}
		if link.Message != "" && !link.Ready {
			fmt.Printf("%s%sError:%s %s%s%s\n", detailPrefix, colorRed, colorReset, colorRed, link.Message, colorReset)
		}
		// Add spacing line
		if i < len(result.Chain)-1 {
			fmt.Printf("%s%s│%s\n", strings.Repeat("    ", i)+"    ", colorDim, colorReset)
		}
	}

	// Summary
	fmt.Printf("\n")
	if result.FullyManaged {
		fmt.Printf("%s%s✓ All levels in sync.%s Managed by %s%s%s.\n", colorBold, colorGreen, colorReset, colorCyan, result.Tool, colorReset)
	} else {
		// Find the broken link
		for _, link := range result.Chain {
			if !link.Ready {
				fmt.Printf("%s%s⚠ Chain broken at %s/%s%s\n", colorBold, colorYellow, link.Kind, link.Name, colorReset)
				if link.Message != "" {
					fmt.Printf("  %s%s%s\n", colorRed, link.Message, colorReset)
				}
				break
			}
		}
	}
	fmt.Printf("\n")

	return nil
}

// runReverseTrace performs a reverse trace - walking ownerReferences up to find GitOps source
func runReverseTrace(ctx context.Context, kind, name, namespace string) error {
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	tracer := agent.NewReverseTracer(dynClient)
	result, err := tracer.Trace(ctx, kind, name, namespace)
	if err != nil {
		return fmt.Errorf("reverse trace failed: %w", err)
	}

	if traceJSON {
		return outputReverseTraceJSON(result)
	}
	return outputReverseTraceHuman(result)
}

// outputReverseTraceJSON outputs the reverse trace result as JSON
func outputReverseTraceJSON(result *agent.ReverseTraceResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputReverseTraceHuman outputs the reverse trace result in human-readable format
func outputReverseTraceHuman(result *agent.ReverseTraceResult) error {
	fmt.Printf("\n")
	fmt.Printf("%s%sREVERSE TRACE:%s %s%s%s\n", colorBold, colorCyan, colorReset, colorBold, result.Object.String(), colorReset)
	fmt.Printf("\n")

	if result.Error != "" {
		fmt.Printf("  %s⚠ %s%s\n\n", colorYellow, result.Error, colorReset)
		return nil
	}

	// Print K8s ownership chain
	fmt.Printf("%s%sK8s Ownership Chain:%s\n", colorBold, colorWhite, colorReset)
	for i, link := range result.K8sChain {
		prefix := ""
		if i > 0 {
			prefix = strings.Repeat("  ", i-1) + "  └─▶ "
		}

		// Status icon
		var icon, iconColor string
		if link.Ready {
			icon = "✓"
			iconColor = colorGreen
		} else {
			icon = "✗"
			iconColor = colorRed
		}

		// Kind color
		kindColor := colorWhite
		switch link.Kind {
		case "Pod":
			kindColor = colorYellow
		case "ReplicaSet":
			kindColor = colorBlue
		case "Deployment", "StatefulSet", "DaemonSet":
			kindColor = colorGreen
		case "Service", "ConfigMap", "Secret":
			kindColor = colorCyan
		}

		fmt.Printf("%s%s%s%s %s%s%s/%s%s%s", prefix, iconColor, icon, colorReset, kindColor, link.Kind, colorReset, colorBold, link.Name, colorReset)
		if link.Status != "" {
			fmt.Printf(" %s(%s)%s", colorDim, link.Status, colorReset)
		}
		fmt.Printf("\n")
	}

	// Print ownership detection result
	fmt.Printf("\n")
	fmt.Printf("%s%sDetected Owner:%s ", colorBold, colorWhite, colorReset)

	ownerColor := colorWhite
	switch result.Owner {
	case "flux":
		ownerColor = colorCyan
	case "argo":
		ownerColor = colorPurple
	case "helm":
		ownerColor = colorYellow
	case "confighub":
		ownerColor = colorBlue
	case "native":
		ownerColor = colorRed
	}

	fmt.Printf("%s%s%s", ownerColor, strings.ToUpper(result.Owner), colorReset)
	if result.OwnerDetails != nil && result.OwnerDetails.Name != "" {
		fmt.Printf(" %s(managed by %s)%s", colorDim, result.OwnerDetails.Name, colorReset)
	}
	fmt.Printf("\n")

	// If native, show warning
	if result.Owner == "native" {
		fmt.Printf("\n")
		fmt.Printf("%s⚠ This resource is NOT managed by GitOps%s\n", colorYellow, colorReset)
		fmt.Printf("%s  • It will be lost if the cluster is rebuilt%s\n", colorDim, colorReset)
		fmt.Printf("%s  • No audit trail in Git%s\n", colorDim, colorReset)
		fmt.Printf("%s  • Consider importing to GitOps: cub-agent import%s\n", colorDim, colorReset)
	}

	// If GitOps managed, suggest full trace
	if result.Owner == "flux" || result.Owner == "argo" {
		fmt.Printf("\n")
		fmt.Printf("%s💡 For full GitOps chain, run:%s\n", colorDim, colorReset)
		if result.TopResource != nil {
			fmt.Printf("   cub-agent trace %s/%s -n %s\n",
				strings.ToLower(result.TopResource.Kind),
				result.TopResource.Name,
				result.TopResource.Namespace)
		}
	}

	fmt.Printf("\n")
	return nil
}
