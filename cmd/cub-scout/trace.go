// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	traceNamespace string
	traceJSON      bool
	traceApp       string // For direct Argo app tracing
	traceReverse   bool   // Reverse trace - walk ownerReferences up
	traceDiff      bool   // Show diff between live and desired state
	traceExplain   bool   // Show explanatory content for learning
	traceHistory   bool   // Show deployment history
	traceLimit     int    // Limit number of history entries
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
	Short: "Trace any resource to its Git source (Flux, ArgoCD, or Helm)",
	Long: `Trace any resource back to its Git source - works with Flux, ArgoCD, or Helm.

You don't need to know which tool manages a resource. Just run trace and
cub-scout auto-detects the owner and shows the full delivery chain.

Under the hood:
  - Flux resources: uses 'flux trace'
  - ArgoCD resources: uses 'argocd app get'
  - Helm resources: reads release metadata

The value: In mixed environments with multiple GitOps tools, one command
traces any resource without switching between flux/argocd/helm CLIs.

Examples:
  # Trace a deployment
  cub-scout trace deployment/nginx -n demo

  # Trace with kind and name separately
  cub-scout trace Deployment nginx -n demo

  # Trace an Argo CD application directly
  cub-scout trace --app frontend-app

  # Reverse trace - start from any resource (e.g., a Pod) and walk up
  cub-scout trace pod/nginx-7d9b8c-x4k2p -n prod --reverse

  # Show diff between live state and desired state from Git
  cub-scout trace deployment/nginx -n demo --diff

  # Output as JSON
  cub-scout trace deployment/nginx -n demo --json

  # Show deployment history (who deployed what, when)
  cub-scout trace deployment/nginx -n demo --history

The output shows:
  - The full chain from GitRepository → Kustomization/HelmRelease → Resource
  - Status and revision at each level
  - Where in the chain something is broken (if applicable)

Reverse trace (--reverse) walks ownerReferences to find:
  - The K8s ownership chain (Pod → ReplicaSet → Deployment)
  - The GitOps owner (Flux, ArgoCD, Helm, or Native)

Diff mode (--diff) shows what would change if GitOps reconciled:
  - For Flux: runs 'flux diff kustomization' or 'flux diff helmrelease'
  - For ArgoCD: runs 'argocd app diff'
  - Useful for debugging "why isn't my change applying?" and upgrade tracing
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
	traceCmd.Flags().BoolVarP(&traceDiff, "diff", "d", false, "Show diff between live state and desired state from Git")
	traceCmd.Flags().BoolVar(&traceExplain, "explain", false, "Show explanatory content to help learn GitOps concepts")
	traceCmd.Flags().BoolVar(&traceHistory, "history", false, "Show deployment history (who deployed what, when)")
	traceCmd.Flags().IntVar(&traceLimit, "limit", 10, "Limit number of history entries (default: 10)")
}

func runTrace(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

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
		return fmt.Errorf("usage: cub-scout trace <kind/name> or cub-scout trace <kind> <name>")
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

	// Handle diff mode
	if traceDiff {
		return runTraceDiff(ctx, kind, name, traceNamespace)
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

	case agent.OwnerHelm:
		// Get k8s client for Helm tracing (reads release secrets)
		cfg, cfgErr := buildConfig()
		if cfgErr != nil {
			return fmt.Errorf("failed to build kubeconfig: %w", cfgErr)
		}
		clientset, clientErr := kubernetes.NewForConfig(cfg)
		if clientErr != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", clientErr)
		}
		tracer := agent.NewHelmTracer(clientset)
		if ownership.Name != "" {
			result, err = tracer.TraceRelease(ctx, ownership.Name, traceNamespace)
		} else {
			result, err = tracer.Trace(ctx, kind, name, traceNamespace)
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
	case "OCIRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"}
	case "HelmRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}
	case "Bucket":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "buckets"}
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

	// Explanatory content when --explain is used
	if traceExplain {
		fmt.Printf("%s%sOWNERSHIP CHAIN EXPLAINED%s\n", colorBold, colorWhite, colorReset)
		fmt.Printf("%s════════════════════════════════════════════════════════════════════%s\n", colorDim, colorReset)
		fmt.Printf("GitOps creates a chain from Git to running pods:\n\n")
		fmt.Printf("  %sGit Repository%s (source of truth)\n", colorPurple, colorReset)
		fmt.Printf("       %s↓%s GitOps controller watches for changes\n", colorDim, colorReset)
		fmt.Printf("  %sKustomization/HelmRelease%s (applies manifests)\n", colorCyan, colorReset)
		fmt.Printf("       %s↓%s Creates/updates\n", colorDim, colorReset)
		fmt.Printf("  %sDeployment%s (desired state)\n", colorGreen, colorReset)
		fmt.Printf("       %s↓%s K8s controller creates\n", colorDim, colorReset)
		fmt.Printf("  %sReplicaSet → Pods%s (running containers)\n", colorYellow, colorReset)
		fmt.Printf("\n")
		fmt.Printf("%sThe trace below shows this chain for your resource:%s\n", colorDim, colorReset)
		fmt.Printf("\n")
	}

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
		case "ConfigHub OCI":
			kindColor = colorBlue // ConfigHub gets blue
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

		// Show OCI source details for ConfigHub OCI sources
		if link.OCISource != nil && link.OCISource.IsConfigHub {
			if link.OCISource.Space != "" {
				fmt.Printf("%s%sSpace:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorCyan, link.OCISource.Space, colorReset)
			}
			if link.OCISource.Target != "" {
				fmt.Printf("%s%sTarget:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorCyan, link.OCISource.Target, colorReset)
			}
			if link.OCISource.Instance != "" {
				fmt.Printf("%s%sRegistry:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorBlue, link.OCISource.Registry, colorReset)
			}
		} else if link.URL != "" {
			// Show URL for non-ConfigHub sources
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

	// Show history if requested and available
	if traceHistory && len(result.History) > 0 {
		fmt.Printf("\n")
		fmt.Printf("%s%sHistory:%s\n", colorBold, colorWhite, colorReset)
		limit := traceLimit
		if limit > len(result.History) {
			limit = len(result.History)
		}
		for i := 0; i < limit; i++ {
			h := result.History[i]
			timeStr := h.Timestamp.Format("2006-01-02 15:04")
			statusColor := colorGreen
			if h.Status == "failed" || h.Status == "superseded" {
				statusColor = colorYellow
			}
			fmt.Printf("  %s%-16s%s  %s%-20s%s  %s%s%s",
				colorDim, timeStr, colorReset,
				colorPurple, truncate(h.Revision, 20), colorReset,
				statusColor, h.Status, colorReset)
			if h.Source != "" {
				fmt.Printf("  %s%s%s", colorDim, h.Source, colorReset)
			}
			fmt.Printf("\n")
		}
		if len(result.History) > limit {
			fmt.Printf("  %s... and %d more (use --limit to show more)%s\n", colorDim, len(result.History)-limit, colorReset)
		}
	} else if traceHistory {
		fmt.Printf("\n")
		fmt.Printf("%s%sHistory:%s %sNo history available%s\n", colorBold, colorWhite, colorReset, colorDim, colorReset)
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

	// Next steps and diagram link when --explain is used
	if traceExplain {
		fmt.Printf("\n")
		fmt.Printf("%sNEXT STEPS:%s\n", colorBold, colorReset)
		fmt.Printf("→ See orphan resources:    cub-scout map orphans\n")
		fmt.Printf("→ Show diff from Git:      cub-scout trace %s -n %s --diff\n", result.Object.String(), result.Object.Namespace)
		fmt.Printf("→ Visual guide:            docs/diagrams/ownership-detection.svg\n")
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

	// Explanatory content when --explain is used
	if traceExplain {
		fmt.Printf("%s%sREVERSE TRACE EXPLAINED%s\n", colorBold, colorWhite, colorReset)
		fmt.Printf("%s════════════════════════════════════════════════════════════════════%s\n", colorDim, colorReset)
		fmt.Printf("Reverse trace walks UP the ownership chain:\n\n")
		fmt.Printf("  %sPod%s (running container)\n", colorYellow, colorReset)
		fmt.Printf("       %s↑%s K8s ownerReference\n", colorDim, colorReset)
		fmt.Printf("  %sReplicaSet%s (manages pod replicas)\n", colorBlue, colorReset)
		fmt.Printf("       %s↑%s K8s ownerReference\n", colorDim, colorReset)
		fmt.Printf("  %sDeployment%s (desired state)\n", colorGreen, colorReset)
		fmt.Printf("       %s↑%s GitOps labels detected\n", colorDim, colorReset)
		fmt.Printf("  %sGitOps Owner%s (Flux/ArgoCD/Helm)\n", colorCyan, colorReset)
		fmt.Printf("\n")
		fmt.Printf("%sThis shows how your resource is managed:%s\n", colorDim, colorReset)
		fmt.Printf("\n")
	}

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

	// If native, show warning and orphan metadata
	if result.Owner == "native" {
		fmt.Printf("\n")
		fmt.Printf("%s⚠ This resource is NOT managed by GitOps%s\n", colorYellow, colorReset)
		fmt.Printf("%s  • It will be lost if the cluster is rebuilt%s\n", colorDim, colorReset)
		fmt.Printf("%s  • No audit trail in Git%s\n", colorDim, colorReset)
		fmt.Printf("%s  • Consider importing to GitOps: cub-scout import%s\n", colorDim, colorReset)

		// Show orphan metadata if available
		if result.OrphanMeta != nil {
			fmt.Printf("\n")
			fmt.Printf("%s%sOrphan Metadata:%s\n", colorBold, colorWhite, colorReset)

			if result.OrphanMeta.CreatedAt != nil {
				fmt.Printf("  %sCreated:%s %s\n", colorDim, colorReset, result.OrphanMeta.CreatedAt.Format("2006-01-02 15:04:05 MST"))
			}

			// Show relevant labels
			if len(result.OrphanMeta.Labels) > 0 {
				fmt.Printf("  %sLabels:%s\n", colorDim, colorReset)
				for k, v := range result.OrphanMeta.Labels {
					// Skip internal labels
					if strings.HasPrefix(k, "kubernetes.io/") ||
						strings.HasPrefix(k, "k8s.io/") {
						continue
					}
					fmt.Printf("    %s=%s\n", k, v)
				}
			}

			// Show last-applied-configuration hint
			if result.OrphanMeta.LastAppliedConfig != "" {
				fmt.Printf("\n")
				fmt.Printf("%s%slast-applied-configuration found%s\n", colorBold, colorGreen, colorReset)
				fmt.Printf("%s  This resource was created via 'kubectl apply'.%s\n", colorDim, colorReset)
				fmt.Printf("%s  The original manifest is available in the annotation.%s\n", colorDim, colorReset)

				// Show a truncated preview
				config := result.OrphanMeta.LastAppliedConfig
				if len(config) > 200 {
					fmt.Printf("\n  %sManifest preview (first 200 chars):%s\n", colorDim, colorReset)
					fmt.Printf("  %s%s...%s\n", colorDim, config[:200], colorReset)
				}

				fmt.Printf("\n  %s💡 To see full manifest:%s\n", colorDim, colorReset)
				if result.TopResource != nil {
					fmt.Printf("  kubectl get %s %s -n %s -o jsonpath='{.metadata.annotations.kubectl\\.kubernetes\\.io/last-applied-configuration}' | jq .\n",
						strings.ToLower(result.TopResource.Kind),
						result.TopResource.Name,
						result.TopResource.Namespace)
				}
			} else {
				fmt.Printf("\n")
				fmt.Printf("%s%sNo last-applied-configuration%s\n", colorBold, colorYellow, colorReset)
				fmt.Printf("%s  This resource was likely created via 'kubectl create' (not 'kubectl apply').%s\n", colorDim, colorReset)
				fmt.Printf("%s  The original manifest is not recoverable from the cluster.%s\n", colorDim, colorReset)
			}
		}
	}

	// If GitOps managed, suggest full trace
	if result.Owner == "flux" || result.Owner == "argo" {
		fmt.Printf("\n")
		fmt.Printf("%s💡 For full GitOps chain, run:%s\n", colorDim, colorReset)
		if result.TopResource != nil {
			fmt.Printf("   cub-scout trace %s/%s -n %s\n",
				strings.ToLower(result.TopResource.Kind),
				result.TopResource.Name,
				result.TopResource.Namespace)
		}
	}

	fmt.Printf("\n")
	return nil
}

// runTraceDiff shows the diff between live state and desired state from Git
func runTraceDiff(ctx context.Context, kind, name, namespace string) error {
	// Print header
	fmt.Printf("\n")
	fmt.Printf("%s%sDIFF:%s %s%s/%s in %s%s\n", colorBold, colorCyan, colorReset, colorBold, kind, name, namespace, colorReset)
	fmt.Printf("%s%s%s\n", colorDim, strings.Repeat("─", 60), colorReset)
	fmt.Printf("\n")

	// Handle ArgoCD Application directly (used with --app flag)
	if kind == "Application" {
		return runArgoDiff(ctx, name, &agent.Ownership{Type: agent.OwnerArgo, Name: name})
	}

	// Handle Flux Kustomization directly
	if kind == "Kustomization" {
		return runFluxDiff(ctx, kind, name, namespace, &agent.Ownership{
			Type:      agent.OwnerFlux,
			SubType:   "kustomization",
			Name:      name,
			Namespace: namespace,
		})
	}

	// Handle Flux HelmRelease directly
	if kind == "HelmRelease" {
		return runFluxDiff(ctx, kind, name, namespace, &agent.Ownership{
			Type:      agent.OwnerFlux,
			SubType:   "helmrelease",
			Name:      name,
			Namespace: namespace,
		})
	}

	// For other resources, detect ownership to choose the right diff tool
	ownership, err := detectResourceOwnership(ctx, kind, name, namespace)
	if err != nil {
		// Try to infer from kind
		ownership = &agent.Ownership{Type: agent.OwnerUnknown}
	}

	switch ownership.Type {
	case agent.OwnerFlux:
		return runFluxDiff(ctx, kind, name, namespace, ownership)
	case agent.OwnerArgo:
		return runArgoDiff(ctx, name, ownership)
	case agent.OwnerHelm:
		return runHelmDiff(ctx, name, namespace)
	default:
		fmt.Printf("%s⚠ Resource is not managed by GitOps (owner: %s)%s\n", colorYellow, ownership.Type, colorReset)
		fmt.Printf("%s  Cannot show diff for unmanaged resources.%s\n", colorDim, colorReset)
		fmt.Printf("%s  Consider importing to GitOps: cub-scout import%s\n", colorDim, colorReset)
		fmt.Printf("\n")
		return nil
	}
}

// runFluxDiff runs flux diff for Kustomizations or HelmReleases
func runFluxDiff(ctx context.Context, kind, name, namespace string, ownership *agent.Ownership) error {
	// Check if flux CLI is available
	if _, err := exec.LookPath("flux"); err != nil {
		return fmt.Errorf("flux CLI not found - install from https://fluxcd.io/docs/installation/")
	}

	// Determine the deployer type and name from ownership
	deployerKind := "kustomization"
	deployerName := ownership.Name
	deployerNamespace := ownership.Namespace

	// Check SubType to determine if it's a HelmRelease
	if ownership.SubType == "helmrelease" {
		deployerKind = "helmrelease"
	}

	// Fallback to flux-system if namespace not detected
	if deployerNamespace == "" {
		deployerNamespace = "flux-system"
	}

	// If we don't have a deployer name, try to find it
	if deployerName == "" {
		// Try to find the Kustomization or HelmRelease that manages this resource
		fmt.Printf("%sSearching for GitOps deployer...%s\n\n", colorDim, colorReset)
		deployerName = name // fallback to resource name
	}

	fmt.Printf("%sRunning: flux diff %s %s -n %s%s\n\n", colorDim, deployerKind, deployerName, deployerNamespace, colorReset)

	// Run flux diff
	cmd := exec.CommandContext(ctx, "flux", "diff", deployerKind, deployerName, "-n", deployerNamespace)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		// Exit code 1 means there are differences, which is expected
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				fmt.Printf("\n%s%s⚠ Differences detected!%s\n", colorBold, colorYellow, colorReset)
				fmt.Printf("%s  The live state differs from what's in Git.%s\n", colorDim, colorReset)
				fmt.Printf("%s  Next Flux reconciliation will apply these changes.%s\n", colorDim, colorReset)
				fmt.Printf("\n")
				return nil
			}
			// Exit code 2 often means path issue - flux diff requires local manifests
			if exitErr.ExitCode() == 2 {
				fmt.Printf("\n%s%s⚠ flux diff requires local manifests%s\n", colorBold, colorYellow, colorReset)
				fmt.Printf("%s  To compare local changes against cluster:%s\n", colorDim, colorReset)
				fmt.Printf("%s  flux diff %s %s -n %s --path ./path/to/manifests%s\n\n", colorCyan, deployerKind, deployerName, deployerNamespace, colorReset)
				fmt.Printf("%s  Alternative: Use 'flux get %s %s -n %s' to see current status%s\n", colorDim, deployerKind, deployerName, deployerNamespace, colorReset)
				fmt.Printf("\n")
				return nil
			}
		}
		return fmt.Errorf("flux diff failed: %w", err)
	}

	fmt.Printf("\n%s%s✓ No differences - live state matches Git%s\n\n", colorBold, colorGreen, colorReset)
	return nil
}

// runArgoDiff runs argocd app diff for ArgoCD Applications
func runArgoDiff(ctx context.Context, name string, ownership *agent.Ownership) error {
	// Check if argocd CLI is available
	if _, err := exec.LookPath("argocd"); err != nil {
		return fmt.Errorf("argocd CLI not found - install from https://argo-cd.readthedocs.io/en/stable/cli_installation/")
	}

	appName := ownership.Name
	if appName == "" {
		appName = name
	}

	fmt.Printf("%sRunning: argocd app diff %s%s\n\n", colorDim, appName, colorReset)

	// Run argocd app diff
	cmd := exec.CommandContext(ctx, "argocd", "app", "diff", appName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		// Exit code 1 means there are differences
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				fmt.Printf("\n%s%s⚠ Differences detected!%s\n", colorBold, colorYellow, colorReset)
				fmt.Printf("%s  The live state differs from what's in Git.%s\n", colorDim, colorReset)
				fmt.Printf("%s  Run 'argocd app sync %s' to apply changes.%s\n", colorDim, appName, colorReset)
				fmt.Printf("\n")
				return nil
			}
		}
		return fmt.Errorf("argocd diff failed: %w", err)
	}

	fmt.Printf("\n%s%s✓ No differences - live state matches Git%s\n\n", colorBold, colorGreen, colorReset)
	return nil
}

// truncate truncates a string to the given length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// runHelmDiff shows diff for Helm-managed resources
func runHelmDiff(ctx context.Context, name, namespace string) error {
	// Check if helm-diff plugin is available
	cmd := exec.CommandContext(ctx, "helm", "plugin", "list")
	output, err := cmd.Output()
	if err != nil || !strings.Contains(string(output), "diff") {
		fmt.Printf("%s⚠ helm-diff plugin not installed%s\n", colorYellow, colorReset)
		fmt.Printf("%s  Install with: helm plugin install https://github.com/databus23/helm-diff%s\n", colorDim, colorReset)
		fmt.Printf("\n")
		fmt.Printf("%sAlternative: Compare live values with chart defaults:%s\n", colorDim, colorReset)
		fmt.Printf("  helm get values %s -n %s\n", name, namespace)
		fmt.Printf("  helm show values <chart>\n")
		fmt.Printf("\n")
		return nil
	}

	fmt.Printf("%sRunning: helm diff upgrade %s -n %s%s\n\n", colorDim, name, namespace, colorReset)

	// For helm diff, we need the chart reference which we may not have
	// This is a limitation - helm diff needs the chart to compare against
	fmt.Printf("%s⚠ Helm diff requires the original chart reference.%s\n", colorYellow, colorReset)
	fmt.Printf("%s  To see what values are currently set:%s\n", colorDim, colorReset)
	fmt.Printf("    helm get values %s -n %s\n", name, namespace)
	fmt.Printf("%s  To see the manifest:%s\n", colorDim, colorReset)
	fmt.Printf("    helm get manifest %s -n %s\n", name, namespace)
	fmt.Printf("\n")

	return nil
}
