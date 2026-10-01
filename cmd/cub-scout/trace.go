// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

var (
	traceKubeContext         string
	traceNamespace           string
	traceJSON                bool   // deprecated: use --format json
	traceFormat              string // output format: ascii, json
	traceApp                 string // For direct Argo app tracing
	traceReverse             bool   // Reverse trace - walk ownerReferences up
	traceDiff                bool   // Show diff between live and desired state
	traceExplain             bool   // Show explanatory content for learning
	traceHistory             bool   // Show deployment history
	traceLimit               int    // Limit number of history entries
	traceArtifacts           bool   // Include source artifact provenance in output
	tracePresentation        string // Presentation mode: human, ai, paired
	traceWithConfigHub       bool
	traceConfigHubSpace      string
	traceConfigHubSince      string
	traceConfigHubStaleAfter string
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
	Short: "Trace any resource to its controller/source evidence",
	Long: `Trace any resource back to its controller/source evidence.

You don't need to know which tool manages a resource. Just run trace and
cub-scout auto-detects the owner and shows the full delivery chain.

Under the hood:
  - Flux resources: uses 'flux trace'
  - ArgoCD resources: reads Application CRs from the selected Kubernetes cluster
  - Helm resources: reads release metadata
  - Sveltos resources: reads Profile/ClusterProfile owner and reference annotations
  - Modelplane resources: reads Modelplane API objects, labels, ownerRefs, and status

The value: In mixed environments with multiple GitOps tools, one command
traces any resource without switching between controller-specific CLIs.

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
  - The full chain from source/controller metadata → deployer/controller → resource
  - Status and revision at each level
  - Where in the chain something is broken (if applicable)

Reverse trace (--reverse) walks ownerReferences to find:
  - The K8s ownership chain (Pod → ReplicaSet → Deployment)
  - The controller owner (Flux, ArgoCD, Sveltos, Modelplane, Helm, or Native)

Diff mode (--diff) shows what would change if GitOps reconciled:
  - For Flux: runs 'flux diff kustomization' or 'flux diff helmrelease'
  - For ArgoCD: runs 'argocd app diff'
  - Useful for debugging "why isn't my change applying?" and upgrade tracing

Trace context troubleshooting (ArgoCD):
  - Select the Kubernetes context with --kube-context
  - Specify the Application namespace with -n
  - Kubernetes Application read permission is required; Argo server login is not used
  - Explicit context is not supported with the legacy delegated --diff path
`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runTrace,
}

func init() {
	rootCmd.AddCommand(traceCmd)

	traceCmd.Flags().StringVar(&traceKubeContext, "kube-context", "", "Use this exact Kubernetes context for trace reads")
	traceCmd.Flags().StringVarP(&traceNamespace, "namespace", "n", "", "Namespace of the resource (default: flux-system; Application names must be unique if omitted)")
	traceCmd.Flags().StringVar(&traceFormat, "format", "ascii", "Output format: ascii, json, md")
	traceCmd.Flags().BoolVar(&traceJSON, "json", false, "Output as JSON (deprecated: use --format json)")
	traceCmd.Flags().StringVar(&traceApp, "app", "", "Trace Argo CD application by name")
	traceCmd.Flags().BoolVarP(&traceReverse, "reverse", "r", false, "Reverse trace - walk ownerReferences up to find GitOps source")
	traceCmd.Flags().BoolVarP(&traceDiff, "diff", "d", false, "Show diff between live state and desired state from Git")
	traceCmd.Flags().BoolVar(&traceExplain, "explain", false, "Show explanatory content to help learn GitOps concepts")
	traceCmd.Flags().BoolVar(&traceHistory, "history", false, "Show deployment history (who deployed what, when)")
	traceCmd.Flags().IntVar(&traceLimit, "limit", 10, "Limit number of history entries (default: 10)")
	traceCmd.Flags().BoolVar(&traceArtifacts, "artifacts", false, "Include source artifact provenance (url/revision/digest/update time)")
	traceCmd.Flags().StringVar(&tracePresentation, "presentation", "", PresentationModeHelp())
	traceCmd.Flags().BoolVar(&traceWithConfigHub, "with-confighub", false, "Include bounded ConfigHub delivery evidence when the resource exposes exact ConfigHub correlation")
	traceCmd.Flags().StringVar(&traceConfigHubSpace, "confighub-space", "", "ConfigHub space for delivery evidence (default: resource ConfigHub space; use '*' explicitly for all spaces)")
	traceCmd.Flags().StringVar(&traceConfigHubSince, "confighub-since", "24h", "Lookback window for ConfigHub release/event evidence (examples: 24h, 7d, 2w)")
	traceCmd.Flags().StringVar(&traceConfigHubStaleAfter, "confighub-stale-after", "15m", "Treat ConfigHub live-status observations older than this as stale")

	// Mark --json as deprecated
	_ = traceCmd.Flags().MarkDeprecated("json", "use --format json instead")
}

func runTrace(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return err
	}
	if selection.explicit && (os.Getenv("CUB_SCOUT_TEST_TRACE_JSON") != "" || os.Getenv("CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON") != "") {
		return fmt.Errorf("--kube-context cannot be combined with trace fixture input")
	}
	if selection.explicit && traceDiff {
		return fmt.Errorf("--kube-context cannot be used with delegated --diff: controller diff binding is not supported")
	}
	effectiveFormat := traceFormat
	if traceJSON && effectiveFormat == "ascii" {
		effectiveFormat = "json"
	}
	if effectiveFormat != "ascii" && effectiveFormat != "json" && effectiveFormat != "md" {
		return fmt.Errorf("unsupported trace format %q (supported: ascii, json, md)", effectiveFormat)
	}

	// Build invocation context with presentation mode resolution
	invCtx, err := NewInvocationContext(tracePresentation, TransportCLI)
	if err != nil {
		return err
	}

	// TEST HOOK: Load trace data from JSON file to bypass cluster access in tests.
	// In production this env var is never set, so real tracing is always used.
	if traceJSONFile := os.Getenv("CUB_SCOUT_TEST_TRACE_JSON"); traceJSONFile != "" {
		return loadAndRenderTraceFromJSON(traceJSONFile, invCtx)
	}

	// Parse resource reference
	var kind, name string

	if traceApp != "" {
		// Direct Argo app trace
		kind = "Application"
		name = traceApp
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
	if traceNamespace == "" && kind != "Application" {
		traceNamespace = "flux-system"
	}

	// Preserve legacy reverse precedence when both mode flags are supplied.
	if traceDiff && !traceReverse {
		return runTraceDiff(ctx, kind, name, traceNamespace)
	}

	// The shared observer owns all reads; presentation only projects its result.
	session, err := newTraceSessionForSelection(selection)
	if err != nil {
		return fmt.Errorf("failed to capture Kubernetes trace session: %w", err)
	}
	if traceReverse {
		return runReverseTraceWithSession(ctx, session, kind, name, traceNamespace)
	}
	observation, err := observeTrace(ctx, session, kind, name, traceNamespace, traceObservationOptions{
		DirectApplication: kind == "Application",
		Artifacts:         traceArtifacts,
		Flux:              capturedTraceFluxFactory,
		Delivery: traceConfigHubDeliveryFlags{
			Enabled: traceWithConfigHub, Namespace: traceNamespace,
			Space: traceConfigHubSpace, Since: traceConfigHubSince, StaleAfter: traceConfigHubStaleAfter,
		},
	})
	if err != nil {
		return err
	}
	result, artifacts := observation.Result, observation.Artifacts
	if traceNamespace == "" && result.Object.Namespace != "" {
		traceNamespace = result.Object.Namespace
	}

	// Output results (effectiveFormat was resolved earlier)
	switch effectiveFormat {
	case "json":
		return outputTraceJSONv014(result, kind, name, traceNamespace, artifacts)
	case "md":
		return outputTraceMarkdown(result, artifacts, invCtx)
	default:
		return outputTraceHuman(result, artifacts, invCtx)
	}
}

type traceApplicationFunc func(ctx context.Context, appName, appNamespace string) (*agent.TraceResult, error)

func shouldAttemptHelmViaArgoFallback(ownership *agent.Ownership, helmResult *agent.TraceResult) bool {
	if ownership == nil || ownership.Type != agent.OwnerHelm {
		return false
	}
	if helmResult == nil {
		return false
	}
	errMsg := strings.ToLower(strings.TrimSpace(helmResult.Error))
	if errMsg == "" {
		return false
	}
	return (strings.Contains(errMsg, "helm release") && strings.Contains(errMsg, "not found")) ||
		strings.Contains(errMsg, "no helm release found managing this resource")
}

func tryHelmViaArgoFallback(
	ctx context.Context,
	dynClient dynamic.Interface,
	kind, name, namespace string,
	ownership *agent.Ownership,
	helmResult *agent.TraceResult,
	traceApp traceApplicationFunc,
) (*agent.TraceResult, *agent.Ownership, bool) {
	if !shouldAttemptHelmViaArgoFallback(ownership, helmResult) {
		return nil, nil, false
	}
	if dynClient == nil || traceApp == nil {
		return nil, nil, false
	}

	resource, err := fetchTraceResource(ctx, dynClient, kind, name, namespace)
	if err != nil || resource == nil {
		return nil, nil, false
	}
	if !isHelmManagedResource(resource) {
		return nil, nil, false
	}

	apps, err := listArgoApplications(ctx, dynClient)
	if err != nil || len(apps) == 0 {
		return nil, nil, false
	}

	appName, appNamespace, ok := selectArgoApplicationForResource(resource, apps)
	if !ok {
		return nil, nil, false
	}

	argoResult, traceErr := traceApp(ctx, appName, appNamespace)
	if traceErr != nil || argoResult == nil || len(argoResult.Chain) == 0 {
		return nil, nil, false
	}

	annotateHelmViaArgoTrace(argoResult, resource, kind, name, namespace)

	return argoResult, &agent.Ownership{
		Type:       agent.OwnerArgo,
		SubType:    "application",
		Name:       appName,
		Namespace:  appNamespace,
		Source:     "fallback:helm-via-argo",
		Confidence: "medium",
	}, true
}

func fetchTraceResource(ctx context.Context, dynClient dynamic.Interface, kind, name, namespace string) (*unstructured.Unstructured, error) {
	if spec, ok := controllerResourceByKind(kind); ok {
		return getControllerResource(ctx, dynClient, spec, name, namespace)
	}
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unknown resource kind: %s", kind)
	}
	return dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
}

type traceResourceLocator struct {
	GVR        schema.GroupVersionResource
	Namespaced bool
}

func fetchProviderConfigResourceWithTraceSession(ctx context.Context, session *traceSession, dynClient dynamic.Interface, name, namespace string) (*unstructured.Unstructured, error) {
	if session == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	client, err := session.discoveryClient()
	if err != nil {
		return nil, err
	}
	resourceLists, err := client.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, err
	}
	locators := providerConfigLocatorsFromAPIResourceLists(resourceLists)
	if len(locators) == 0 {
		return nil, fmt.Errorf("ProviderConfig CRD not found in API discovery")
	}
	return fetchResourceWithLocators(ctx, dynClient, "ProviderConfig", name, namespace, locators)
}

func providerConfigLocatorsFromAPIResourceLists(resourceLists []*v1.APIResourceList) []traceResourceLocator {
	locators := make([]traceResourceLocator, 0)
	seen := make(map[string]struct{})

	for _, resourceList := range resourceLists {
		if resourceList == nil {
			continue
		}

		gv, err := schema.ParseGroupVersion(strings.TrimSpace(resourceList.GroupVersion))
		if err != nil {
			continue
		}
		if !strings.HasSuffix(gv.Group, ".crossplane.io") && !strings.HasSuffix(gv.Group, ".upbound.io") {
			continue
		}

		for _, resource := range resourceList.APIResources {
			if resource.Kind != "ProviderConfig" || resource.Name != "providerconfigs" || strings.Contains(resource.Name, "/") {
				continue
			}

			locator := traceResourceLocator{
				GVR: schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: resource.Name,
				},
				Namespaced: resource.Namespaced,
			}
			key := fmt.Sprintf("%s|%t", locator.GVR.String(), locator.Namespaced)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			locators = append(locators, locator)
		}
	}

	sort.Slice(locators, func(i, j int) bool {
		if locators[i].Namespaced != locators[j].Namespaced {
			return !locators[i].Namespaced
		}
		if locators[i].GVR.Group != locators[j].GVR.Group {
			return locators[i].GVR.Group < locators[j].GVR.Group
		}
		if locators[i].GVR.Version != locators[j].GVR.Version {
			return locators[i].GVR.Version < locators[j].GVR.Version
		}
		return locators[i].GVR.Resource < locators[j].GVR.Resource
	})

	return locators
}

func fetchResourceWithLocators(
	ctx context.Context,
	dynClient dynamic.Interface,
	kind, name, namespace string,
	locators []traceResourceLocator,
) (*unstructured.Unstructured, error) {
	var (
		matches      []*unstructured.Unstructured
		matchSources []traceResourceLocator
		lastErr      error
	)

	for _, locator := range locators {
		var resourceClient dynamic.ResourceInterface
		if locator.Namespaced {
			if strings.TrimSpace(namespace) == "" {
				continue
			}
			resourceClient = dynClient.Resource(locator.GVR).Namespace(namespace)
		} else {
			resourceClient = dynClient.Resource(locator.GVR)
		}

		resource, err := resourceClient.Get(ctx, name, v1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			lastErr = err
			continue
		}

		matches = append(matches, resource)
		matchSources = append(matchSources, locator)
	}

	switch len(matches) {
	case 0:
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: strings.ToLower(kind)}, name)
	case 1:
		return matches[0], nil
	default:
		candidates := make([]string, 0, len(matchSources))
		for _, locator := range matchSources {
			scope := "cluster"
			if locator.Namespaced {
				scope = "namespace"
			}
			candidates = append(candidates, fmt.Sprintf("%s/%s (%s)", locator.GVR.Group, locator.GVR.Version, scope))
		}
		sort.Strings(candidates)
		return nil, fmt.Errorf("ambiguous %s %q found in multiple API groups: %s", kind, name, strings.Join(candidates, ", "))
	}
}

func isHelmManagedResource(resource *unstructured.Unstructured) bool {
	labels := resource.GetLabels()
	if labels == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(labels["app.kubernetes.io/managed-by"]), "helm") {
		return true
	}
	return strings.TrimSpace(labels["helm.sh/chart"]) != ""
}

func listArgoApplications(ctx context.Context, dynClient dynamic.Interface) ([]unstructured.Unstructured, error) {
	appGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
	list, err := dynClient.Resource(appGVR).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func selectArgoApplicationForResource(resource *unstructured.Unstructured, apps []unstructured.Unstructured) (string, string, bool) {
	targetKind := strings.TrimSpace(resource.GetKind())
	targetName := strings.TrimSpace(resource.GetName())
	targetNamespace := strings.TrimSpace(resource.GetNamespace())
	if targetKind == "" || targetName == "" {
		return "", "", false
	}

	matches := make([]unstructured.Unstructured, 0, len(apps))
	for _, app := range apps {
		if !argoApplicationManagesResource(&app, targetKind, targetName, targetNamespace) {
			continue
		}
		if !argoApplicationTargetsNamespace(&app, targetNamespace) {
			continue
		}
		matches = append(matches, app)
	}
	if len(matches) == 0 {
		return "", "", false
	}

	preferred := preferredArgoApplicationNames(resource)
	if len(preferred) > 0 {
		preferredMatches := make([]unstructured.Unstructured, 0, len(matches))
		for _, app := range matches {
			if _, ok := preferred[strings.TrimSpace(app.GetName())]; ok {
				preferredMatches = append(preferredMatches, app)
			}
		}
		matches = preferredMatches
	}

	if len(matches) != 1 {
		return "", "", false
	}

	return strings.TrimSpace(matches[0].GetName()), strings.TrimSpace(matches[0].GetNamespace()), true
}

func preferredArgoApplicationNames(resource *unstructured.Unstructured) map[string]struct{} {
	preferred := make(map[string]struct{})

	if labelApp := strings.TrimSpace(resource.GetLabels()["argocd.argoproj.io/instance"]); labelApp != "" {
		preferred[labelApp] = struct{}{}
	}

	if trackingID := strings.TrimSpace(resource.GetAnnotations()["argocd.argoproj.io/tracking-id"]); trackingID != "" {
		if appName := parseArgoTrackingIDApplication(trackingID); appName != "" {
			preferred[appName] = struct{}{}
		}
	}

	return preferred
}

func parseArgoTrackingIDApplication(trackingID string) string {
	parts := strings.SplitN(strings.TrimSpace(trackingID), ":", 2)
	if len(parts) < 1 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func argoApplicationTargetsNamespace(app *unstructured.Unstructured, targetNamespace string) bool {
	if targetNamespace == "" {
		return true
	}
	destNamespace, found, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")
	if !found {
		return true
	}
	destNamespace = strings.TrimSpace(destNamespace)
	if destNamespace == "" {
		return true
	}
	return destNamespace == targetNamespace
}

func argoApplicationManagesResource(app *unstructured.Unstructured, targetKind, targetName, targetNamespace string) bool {
	resources, found, _ := unstructured.NestedSlice(app.Object, "status", "resources")
	if !found || len(resources) == 0 {
		return false
	}

	for _, raw := range resources {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		resKind := strings.TrimSpace(fmt.Sprintf("%v", item["kind"]))
		resName := strings.TrimSpace(fmt.Sprintf("%v", item["name"]))
		resNamespace := strings.TrimSpace(fmt.Sprintf("%v", item["namespace"]))

		if !strings.EqualFold(resKind, targetKind) {
			continue
		}
		if resName != targetName {
			continue
		}
		if targetNamespace != "" && resNamespace != targetNamespace {
			continue
		}
		return true
	}

	return false
}

func annotateHelmViaArgoTrace(result *agent.TraceResult, resource *unstructured.Unstructured, kind, name, namespace string) {
	if result == nil || resource == nil {
		return
	}

	labels := resource.GetLabels()
	chartName, chartVersion := parseHelmChartLabel(strings.TrimSpace(labels["helm.sh/chart"]))
	if chartName == "" {
		chartName = strings.TrimSpace(labels["app.kubernetes.io/instance"])
	}
	if chartName == "" {
		chartName = name
	}

	helmLink := agent.ChainLink{
		Kind:      "HelmChart",
		Name:      chartName,
		Namespace: namespace,
		Ready:     true,
		Status:    "template mode",
		Revision:  chartVersion,
	}

	if !traceChainHasResource(result.Chain, helmLink.Kind, helmLink.Name, helmLink.Namespace) {
		result.Chain = insertHelmChartAfterApplication(result.Chain, helmLink)
	}
	if !traceChainHasResource(result.Chain, kind, name, namespace) {
		result.Chain = append(result.Chain, agent.ChainLink{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
			Ready:     true,
			Status:    "Observed",
		})
	}

	explanation := "Detected Helm-via-ArgoCD template mode: resource has Helm metadata but no Helm release secret. ArgoCD renders and applies this chart, so Helm CLI cannot manage it directly."
	if strings.TrimSpace(result.Error) == "" {
		result.Error = explanation
	} else if !strings.Contains(result.Error, explanation) {
		result.Error = strings.TrimSpace(result.Error + " " + explanation)
	}

	result.Object = agent.ResourceRef{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}
	result.Tool = "argocd"
}

func parseHelmChartLabel(chartLabel string) (string, string) {
	chartLabel = strings.TrimSpace(chartLabel)
	if chartLabel == "" {
		return "", ""
	}
	lastDash := strings.LastIndex(chartLabel, "-")
	if lastDash <= 0 || lastDash == len(chartLabel)-1 {
		return chartLabel, ""
	}

	name := strings.TrimSpace(chartLabel[:lastDash])
	version := strings.TrimSpace(chartLabel[lastDash+1:])
	if name == "" || version == "" || !containsDigit(version) {
		return chartLabel, ""
	}
	return name, version
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func insertHelmChartAfterApplication(chain []agent.ChainLink, helmLink agent.ChainLink) []agent.ChainLink {
	out := make([]agent.ChainLink, 0, len(chain)+1)
	inserted := false

	for _, link := range chain {
		out = append(out, link)
		if !inserted && link.Kind == "Application" {
			out = append(out, helmLink)
			inserted = true
		}
	}

	if !inserted {
		out = append(out, helmLink)
	}

	return out
}

func traceChainHasResource(chain []agent.ChainLink, kind, name, namespace string) bool {
	for _, link := range chain {
		if link.Kind == kind && link.Name == name && link.Namespace == namespace {
			return true
		}
	}
	return false
}

// normalizeKind normalizes resource kind names
func normalizeKind(kind string) string {
	kind = strings.ToLower(kind)
	if normalized, ok := normalizeFirstClassControllerKind(kind); ok {
		return normalized
	}
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
	case "providerconfig", "providerconfigs":
		return "ProviderConfig"
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

func enrichTraceWithTimingSession(ctx context.Context, session *traceSession, result *agent.TraceResult) {
	if result == nil {
		return
	}
	if session == nil {
		_, readErrors := agent.NewTimingEnricher(nil).EnrichChainWithTimingAndErrors(ctx, result.Chain)
		for _, readErr := range readErrors {
			result.Error = appendSentence(result.Error, readErr.Error())
		}
		return
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		result.Error = appendSentence(result.Error, "timing enrichment unavailable: Kubernetes client is unavailable")
		return
	}
	enricher := agent.NewTimingEnricher(dynClient)
	var readErrors []error
	result.Chain, readErrors = enricher.EnrichChainWithTimingAndErrors(ctx, result.Chain)
	for _, readErr := range readErrors {
		result.Error = appendSentence(result.Error, readErr.Error())
	}
}

func detectCrossOwnerReferencesWithTraceSession(ctx context.Context, session *traceSession, kind, name, namespace string, resourceOwner *agent.Ownership) ([]agent.CrossReference, error) {
	if session == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return nil, err
	}
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unknown resource kind: %s", kind)
	}
	resource, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return agent.NewCrossRefDetector(dynClient).DetectCrossReferences(ctx, resource, resourceOwner)
}

// detectResourceOwnership fetches the resource and detects its owner
func detectResourceOwnership(ctx context.Context, kind, name, namespace string) (*agent.Ownership, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, err
	}
	session, err := newTraceSession(cfg, "")
	if err != nil {
		return nil, err
	}
	return detectResourceOwnershipWithTraceSession(ctx, session, kind, name, namespace)
}

func detectResourceOwnershipWithTraceSession(ctx context.Context, session *traceSession, kind, name, namespace string) (*agent.Ownership, error) {
	if session == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return nil, err
	}
	var resource *unstructured.Unstructured
	if kind == "ProviderConfig" {
		resource, err = fetchProviderConfigResourceWithTraceSession(ctx, session, dynClient, name, namespace)
	} else if spec, ok := controllerResourceByKind(kind); ok {
		resource, err = getControllerResource(ctx, dynClient, spec, name, namespace)
	} else {
		gvr := kindToGVR(kind)
		if gvr.Resource == "" {
			return nil, fmt.Errorf("unknown resource kind: %s", kind)
		}
		resource, err = dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}
	ownership := agent.DetectOwnership(resource)
	if ownership.Type == agent.OwnerUnknown && isCrossplaneProviderConfig(resource) {
		ownership = agent.Ownership{Type: agent.OwnerCrossplane, SubType: "providerconfig", Name: resource.GetName(), Namespace: resource.GetNamespace(), Source: "apiGroup:" + resource.GroupVersionKind().Group, Confidence: "high"}
	}
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
	case "Pod":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	case "ConfigMap":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	case "Secret":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	case "Job":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	case "CronJob":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
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
	case "ConfigHub OCI":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"}
	case "HelmRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}
	case "Bucket":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "buckets"}
	case "Application":
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	default:
		if spec, ok := controllerResourceByKind(kind); ok {
			return spec.GVR
		}
		return schema.GroupVersionResource{}
	}
}

// outputTraceJSONv014 outputs the trace result using the v0.14 JSON schema.
func outputTraceJSONv014(result *agent.TraceResult, kind, name, namespace string, artifacts map[string]mapsvc.TraceArtifactRef) error {
	output := convertTraceToV014(result, kind, name, namespace, artifacts)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return err
	}

	// Per cli-contract.md: exit code 1 for "not managed"
	if result.Error != "" && len(result.Chain) == 0 {
		os.Exit(1)
	}
	return nil
}

// convertTraceToV014 converts agent.TraceResult to mapsvc.TraceOutput.
func convertTraceToV014(result *agent.TraceResult, kind, name, namespace string, artifacts map[string]mapsvc.TraceArtifactRef) mapsvc.TraceOutput {
	// Build target from the traced resource
	target := mapsvc.ResourceID{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}

	// Convert chain links to v0.14 chain nodes
	var chain []mapsvc.ChainNode
	for _, link := range result.Chain {
		role := mapsvc.InferRole(link.Kind)
		node := mapsvc.ChainNode{
			ID: mapsvc.ResourceID{
				Kind:      link.Kind,
				Namespace: link.Namespace,
				Name:      link.Name,
			},
			Role: role,
			// Relationship is based on the node's role, not the edge
			Relationship: mapsvc.RelationshipForRole(role),
		}

		// Delivery chain metadata (v1.1+)
		node.DeliveryStage = mapsvc.InferDeliveryStage(link.Kind)
		node.RenderedFrom = link.RenderedFrom
		node.OriginalSource = link.OriginalSource

		// Populate OriginalSource for source nodes with URLs
		if link.URL != "" && node.OriginalSource == "" && mapsvc.InferDeliveryStage(link.Kind) == mapsvc.StageSource {
			node.OriginalSource = "git:" + link.URL
		}

		// Populate RenderedFrom for ConfigHub OCI chains
		if link.OCISource != nil && link.OCISource.IsConfigHub && node.RenderedFrom == "" {
			node.RenderedFrom = fmt.Sprintf("confighub:space/%s/target/%s", link.OCISource.Space, link.OCISource.Target)
		}

		// Build evidence from available metadata
		node.Evidence = buildEvidence(link, result.Tool)

		chain = append(chain, node)
	}

	// Build summary
	summary := buildTraceSummary(result, chain, artifacts)

	output := mapsvc.TraceOutput{
		Command: "trace",
		Target:  target,
		Chain:   chain,
		Summary: summary,
	}

	// Add secret evidence if present
	if result.Secrets != nil && len(result.Secrets.Secrets) > 0 {
		output.Secrets = convertSecretEvidence(result.Secrets)
	}

	output.Context = result.Context
	if result.Error != "" {
		output.Warnings = []string{result.Error}
	}

	// Add events if present
	if result.Events != nil && len(result.Events.Events) > 0 {
		output.Events = convertResourceEvents(result.Events)
	}
	if result.DeliveryEvidence != nil {
		output.DeliveryEvidence = result.DeliveryEvidence
	}

	return output
}

// convertSecretEvidence converts agent.SecretEvidenceResult to mapsvc.TraceSecrets.
func convertSecretEvidence(evidence *agent.SecretEvidenceResult) *mapsvc.TraceSecrets {
	if evidence == nil {
		return nil
	}

	secrets := make([]mapsvc.TraceSecretEvidence, len(evidence.Secrets))
	for i, s := range evidence.Secrets {
		se := mapsvc.TraceSecretEvidence{
			Name:         s.Name,
			Namespace:    s.Namespace,
			RefType:      string(s.RefType),
			RefPath:      s.RefPath,
			Status:       string(s.Status),
			StatusReason: s.StatusReason,
			SecretType:   s.SecretType,
			Optional:     s.Optional,
		}

		// Carry safe metadata when present
		if s.CreatedAt != nil {
			se.CreatedAt = s.CreatedAt.Format(time.RFC3339)
		}
		if s.Owner != nil {
			se.Owner = &mapsvc.TraceSecretOwner{
				Type:      s.Owner.Type,
				SubType:   s.Owner.SubType,
				Name:      s.Owner.Name,
				Namespace: s.Owner.Namespace,
			}
		}

		secrets[i] = se
	}

	return &mapsvc.TraceSecrets{
		Secrets: secrets,
		Summary: mapsvc.TraceSecretSummary{
			Total:      evidence.Summary.Total,
			Present:    evidence.Summary.Present,
			Missing:    evidence.Summary.Missing,
			Unreadable: evidence.Summary.Unreadable,
			Unresolved: evidence.Summary.Unresolved,
		},
	}
}

// convertResourceEvents converts agent.ResourceEventSummary to mapsvc.TraceEvents.
func convertResourceEvents(events *agent.ResourceEventSummary) *mapsvc.TraceEvents {
	if events == nil {
		return nil
	}

	traceEvents := make([]mapsvc.TraceEvent, len(events.Events))
	for i, ev := range events.Events {
		te := mapsvc.TraceEvent{
			Type:     ev.Type,
			Reason:   ev.Reason,
			Message:  ev.Message,
			Count:    ev.Count,
			Age:      ev.Age,
			Severity: ev.Severity,
			Source:   ev.Source,
		}
		if ev.Action != nil {
			te.Action = &mapsvc.TraceActionEvent{
				Action:  ev.Action.Action,
				Actor:   ev.Action.Actor,
				Groups:  ev.Action.Groups,
				Subject: ev.Action.Subject,
				Raw:     ev.Action.Raw,
			}
		}
		if ev.FirstSeen != nil {
			te.FirstSeen = ev.FirstSeen.Format(time.RFC3339)
		}
		if ev.LastSeen != nil {
			te.LastSeen = ev.LastSeen.Format(time.RFC3339)
		}
		traceEvents[i] = te
	}

	return &mapsvc.TraceEvents{
		Events:       traceEvents,
		TotalCount:   events.TotalCount,
		WarningCount: events.WarningCount,
		ErrorCount:   events.ErrorCount,
	}
}

// buildEvidence constructs evidence from a chain link.
func buildEvidence(link agent.ChainLink, tool string) []mapsvc.Evidence {
	var evidence []mapsvc.Evidence

	role := mapsvc.InferRole(link.Kind)

	switch role {
	case mapsvc.RoleSource:
		// Kubernetes-backed source resources expose spec.url. Argo's synthetic
		// Source link comes from Application.spec.source(s), whose exact path is
		// not retained here, so do not claim that the URL was a standalone
		// Kubernetes spec.url observation.
		if link.Kind != "Source" && link.URL != "" {
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceField,
				Key:   "spec.url",
				Value: link.URL,
				Path:  "spec.url",
			})
		}

	case mapsvc.RoleDeployer:
		// Deployer: evidence from path and inventory
		if link.Path != "" {
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceField,
				Key:   "spec.path",
				Value: link.Path,
				Path:  "spec.path",
			})
		}
		// Add inventory evidence if we have children
		for _, child := range link.Children {
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceInventory,
				Key:   "status.inventory",
				Value: fmt.Sprintf("%s/%s/%s", child.Kind, child.Namespace, child.Name),
				Path:  "status.inventory.entries",
			})
			// Limit to first 3 children to avoid huge evidence lists
			if len(evidence) >= 4 {
				break
			}
		}

	case mapsvc.RoleWorkload, mapsvc.RoleIntermediate:
		// Workload: evidence from labels
		switch tool {
		case "flux":
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceLabel,
				Key:   "kustomize.toolkit.fluxcd.io/name",
				Value: findDeployerName(link),
				Path:  "metadata.labels",
			})
		case "argocd":
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceLabel,
				Key:   "argocd.argoproj.io/instance",
				Value: findDeployerName(link),
				Path:  "metadata.labels",
			})
		case "helm":
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceLabel,
				Key:   "app.kubernetes.io/managed-by",
				Value: "Helm",
				Path:  "metadata.labels",
			})
		}
	}

	return evidence
}

// findDeployerName extracts deployer name from chain context.
// This is a simplified version; full implementation would parse the trace chain.
func findDeployerName(link agent.ChainLink) string {
	// For now, return the link name if it's the deployer
	if mapsvc.InferRole(link.Kind) == mapsvc.RoleDeployer {
		return link.Name
	}
	return ""
}

// buildTraceSummary constructs the summary from trace result.
func buildTraceSummary(result *agent.TraceResult, chain []mapsvc.ChainNode, artifacts map[string]mapsvc.TraceArtifactRef) mapsvc.TraceSummary {
	summary := mapsvc.TraceSummary{
		OwnerType: traceSummaryOwner(result),
	}

	// Find source and deployer in chain
	for _, node := range chain {
		switch node.Role {
		case mapsvc.RoleSource:
			// Find URL from original chain
			var url string
			var revision string
			for _, link := range result.Chain {
				if link.Kind == node.ID.Kind && link.Name == node.ID.Name && link.Namespace == node.ID.Namespace {
					url = link.URL
					if link.Kind == "Source" {
						revision = link.Revision
					}
					break
				}
			}
			summary.Source = &mapsvc.TraceSourceRef{
				Kind:      node.ID.Kind,
				Namespace: node.ID.Namespace,
				Name:      node.ID.Name,
				URL:       url,
				Revision:  revision,
			}
			if traceArtifacts {
				if art, ok := lookupTraceArtifact(node.ID.Kind, node.ID.Namespace, node.ID.Name, artifacts); ok {
					artCopy := art
					summary.Source.Artifact = &artCopy
				} else {
					unknown := traceArtifactUnknownForKind(node.ID.Kind)
					summary.Source.Artifact = &unknown
				}
			}
		case mapsvc.RoleDeployer:
			summary.Deployer = &mapsvc.ResourceID{
				Kind:      node.ID.Kind,
				Namespace: node.ID.Namespace,
				Name:      node.ID.Name,
			}
		}
	}

	return summary
}

// normalizeToolToOwner converts tool name to owner type.
// traceSummaryOwner is the owner trace JSON reports. The tracer that ran is
// the owner only when it produced a complete chain; a tracer answering "not
// managed by me" (the default branch runs Flux's for any unlabelled resource)
// is not evidence of ownership, so the detected owner stands (#617).
func traceSummaryOwner(result *agent.TraceResult) string {
	complete := len(result.Chain) > 0 && strings.TrimSpace(result.Error) == ""
	detected := strings.TrimSpace(result.DetectedOwner)
	if detected == agent.OwnerCustom && !complete {
		// Custom owners have no tracer; the name travels in the error, as
		// explain reads it. Before #617 this came out as "Native".
		if name := customOwnerFromTraceError(result.Error); name != "" {
			return name
		}
		return "Custom"
	}
	if detected != "" && !complete {
		return mapsvc.DisplayOwner(detected)
	}
	return normalizeToolToOwner(result.Tool)
}

func normalizeToolToOwner(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "flux":
		return "Flux"
	case "argocd", "argo":
		return "ArgoCD"
	case "helm":
		return "Helm"
	case "sveltos":
		return "Sveltos"
	case "modelplane":
		return "Modelplane"
	case "crossplane":
		return "Crossplane"
	case "kro":
		return "kro"
	case "terraform":
		return "Terraform"
	case "confighub":
		return "ConfigHub"
	default:
		return "Native"
	}
}

// outputTraceHuman preserves the CLI stdout and exit-code contract while
// delegating all content to the shared writer-based renderer.
func outputTraceHuman(result *agent.TraceResult, artifacts map[string]mapsvc.TraceArtifactRef, invCtx InvocationContext) error {
	err := renderTraceHuman(os.Stdout, result, artifacts, invCtx, traceHumanOptions{
		Explain: traceExplain, Artifacts: traceArtifacts, History: traceHistory, Limit: traceLimit,
	})
	if _, noChain := err.(traceNoChainError); noChain {
		// Preserve the historical CLI contract: the warning is on stdout and
		// this condition exits 1 without Cobra printing a second error line.
		os.Exit(1)
	}
	return err
}

// outputTraceMarkdown outputs the trace in markdown format (thin wrapper over ASCII, no colors).
// The invCtx parameter is accepted for API consistency but markdown output doesn't vary by presentation mode.
func outputTraceMarkdown(result *agent.TraceResult, artifacts map[string]mapsvc.TraceArtifactRef, invCtx InvocationContext) error {
	_ = invCtx // Markdown output is uniform across presentation modes
	// Markdown header
	fmt.Printf("## Trace: %s\n\n", result.Object.String())
	if result.Context != "" {
		fmt.Printf("Selected Kubernetes context: %s\n\n", result.Context)
	}
	fmt.Println("```")

	if result.Error != "" && len(result.Chain) == 0 {
		fmt.Printf("  [warning] %s\n", result.Error)
		fmt.Println("```")
		os.Exit(1)
		return nil
	}
	if result.Error != "" {
		fmt.Printf("  [warning] %s\n\n", result.Error)
	}

	// Print chain without colors
	for i, link := range result.Chain {
		prefix := "  "
		if i > 0 {
			prefix = strings.Repeat("    ", i-1) + "    └─▶ "
		}

		// Status icon
		icon := SymOK
		if !link.Ready {
			icon = SymError
		}

		fmt.Printf("%s%s %s/%s\n", prefix, icon, link.Kind, link.Name)

		// Details
		detailPrefix := strings.Repeat("    ", i) + "    │ "
		if i == len(result.Chain)-1 {
			detailPrefix = strings.Repeat("    ", i) + "      "
		}

		if link.Namespace != "" && link.Namespace != result.Object.Namespace {
			fmt.Printf("%sNamespace: %s\n", detailPrefix, link.Namespace)
		}
		if link.URL != "" {
			fmt.Printf("%sURL: %s\n", detailPrefix, link.URL)
		}
		if link.Path != "" {
			fmt.Printf("%sPath: %s\n", detailPrefix, link.Path)
		}
		if link.Revision != "" {
			fmt.Printf("%sRevision: %s\n", detailPrefix, link.Revision)
		}
		if traceArtifacts && isTraceSourceKind(link.Kind) {
			artifact := artifactForLink(link, artifacts)
			fmt.Printf("%sArtifact URL: %s\n", detailPrefix, artifact.URL)
			fmt.Printf("%sArtifact Revision: %s\n", detailPrefix, artifact.Revision)
			fmt.Printf("%sArtifact Digest: %s\n", detailPrefix, artifact.Digest)
			fmt.Printf("%sArtifact Updated: %s\n", detailPrefix, artifact.LastUpdateTime)
		}
		if link.Status != "" {
			fmt.Printf("%sStatus: %s\n", detailPrefix, link.Status)
		}
		if link.Message != "" && !link.Ready {
			fmt.Printf("%sError: %s\n", detailPrefix, link.Message)
		}
		if i < len(result.Chain)-1 {
			fmt.Printf("%s│\n", strings.Repeat("    ", i)+"    ")
		}
	}

	renderTraceDeliveryEvidenceMarkdown(result.DeliveryEvidence)

	// Summary
	fmt.Println()
	if result.FullyManaged {
		fmt.Printf("✓ All levels in sync. Managed by %s.\n", result.Tool)
	} else {
		for _, link := range result.Chain {
			if !link.Ready {
				fmt.Printf("⚠ Chain broken at %s/%s\n", link.Kind, link.Name)
				if link.Message != "" {
					fmt.Printf("  %s\n", link.Message)
				}
				break
			}
		}
	}

	fmt.Println("```")
	return nil
}

// runReverseTraceWithSession performs every Kubernetes read through the supplied
// invocation binding and renders the requested reverse-trace representation.
func runReverseTraceWithSession(ctx context.Context, session *traceSession, kind, name, namespace string) error {
	format := traceFormat
	if traceJSON && format == "ascii" {
		format = "json"
	}
	switch format {
	case "ascii", "json", "md":
	default:
		return fmt.Errorf("unsupported trace format %q (supported: ascii, json, md)", format)
	}
	if session == nil {
		return fmt.Errorf("reverse trace requires a captured trace session")
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return fmt.Errorf("failed to create dynamic client for reverse trace: %w", err)
	}
	if kind == "Application" && namespace == "" {
		// Match normal direct Application tracing: resolve only a unique name
		// through the captured API, never guess an Application namespace.
		application, err := agent.NewArgoTracerWithKubernetesClient(dynClient).TraceApplicationInNamespace(ctx, name, "")
		if err != nil {
			return fmt.Errorf("resolve reverse Application namespace: %w", err)
		}
		if application == nil || application.Object.Namespace == "" {
			return fmt.Errorf("reverse Application namespace is unavailable; specify -n")
		}
		namespace = application.Object.Namespace
	}
	result, err := agent.NewReverseTracer(dynClient).Trace(ctx, kind, name, namespace)
	if err != nil {
		return fmt.Errorf("reverse trace failed: %w", err)
	}
	result.Context = session.contextLabel()

	switch format {
	case "json":
		return outputReverseTraceJSON(result)
	case "md":
		return renderReverseTraceMarkdown(os.Stdout, result)
	default:
		return outputReverseTraceHuman(result)
	}
}

// outputReverseTraceJSON outputs the reverse trace result in the existing JSON model.
func outputReverseTraceJSON(result *agent.ReverseTraceResult) error {
	return outputReverseTraceJSONTo(os.Stdout, result)
}

func outputReverseTraceJSONTo(w io.Writer, result *agent.ReverseTraceResult) error {
	if result == nil {
		return fmt.Errorf("reverse trace result is nil")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputReverseTraceHuman preserves the CLI stdout wrapper for the shared renderer.
func outputReverseTraceHuman(result *agent.ReverseTraceResult) error {
	return renderReverseTraceHuman(os.Stdout, result, traceExplain)
}

// renderReverseTraceHuman writes the complete human projection without reading
// global flags, accessing cluster state, or exiting the process.
func renderReverseTraceHuman(w io.Writer, result *agent.ReverseTraceResult, explain bool) error {
	if w == nil {
		return fmt.Errorf("reverse trace output writer is nil")
	}
	if result == nil {
		return fmt.Errorf("reverse trace result is nil")
	}
	trackedWriter := &traceHumanWriter{writer: w}
	w = trackedWriter
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s%sREVERSE TRACE:%s %s%s%s\n", colorBold, colorCyan, colorReset, colorBold, result.Object.String(), colorReset)
	fmt.Fprintf(w, "\n")
	if result.Context != "" {
		fmt.Fprintf(w, "Kubernetes context: %s (selection label; not a stable cluster ID)\n\n", result.Context)
	}

	// Explanatory content when --explain is used
	if explain {
		fmt.Fprintf(w, "%s%sREVERSE TRACE EXPLAINED%s\n", colorBold, colorWhite, colorReset)
		fmt.Fprintf(w, "%s════════════════════════════════════════════════════════════════════%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "Reverse trace walks UP the ownership chain:\n\n")
		fmt.Fprintf(w, "  %sPod%s (running container)\n", colorYellow, colorReset)
		fmt.Fprintf(w, "       %s↑%s K8s ownerReference\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sReplicaSet%s (manages pod replicas)\n", colorBlue, colorReset)
		fmt.Fprintf(w, "       %s↑%s K8s ownerReference\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sDeployment%s (desired state)\n", colorGreen, colorReset)
		fmt.Fprintf(w, "       %s↑%s GitOps labels detected\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sGitOps Owner%s (Flux/ArgoCD/Helm)\n", colorCyan, colorReset)
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%sThis shows how your resource is managed:%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "\n")
	}

	if result.Error != "" {
		fmt.Fprintf(w, "  %s⚠ %s%s\n\n", colorYellow, result.Error, colorReset)
		if len(result.K8sChain) == 0 && len(result.GitOpsChain) == 0 {
			return trackedWriter.err
		}
	}

	// Print K8s ownership chain
	fmt.Fprintf(w, "%s%sK8s Ownership Chain:%s\n", colorBold, colorWhite, colorReset)
	for i, link := range result.K8sChain {
		prefix := ""
		if i > 0 {
			prefix = strings.Repeat("  ", i-1) + "  └─▶ "
		}

		// Status icon
		var icon, iconColor string
		if link.Ready {
			icon = SymOK
			iconColor = colorGreen
		} else {
			icon = SymError
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

		fmt.Fprintf(w, "%s%s%s%s %s%s%s/%s%s%s", prefix, iconColor, icon, colorReset, kindColor, link.Kind, colorReset, colorBold, link.Name, colorReset)
		if link.Status != "" {
			fmt.Fprintf(w, " %s(%s)%s", colorDim, link.Status, colorReset)
		}
		fmt.Fprintf(w, "\n")
	}

	// If this looks platform-composition managed, show resolver lineage.
	// This does not alter ownership detection; it only surfaces what the resolver can infer
	// from already-fetched objects.
	if len(result.Objects) > 0 {
		if lineage, ok := agent.ResolveCrossplaneLineage(result.Objects[0], result.Objects); ok {
			fmt.Fprint(w, renderCrossplaneLineageHuman(lineage))
		} else if lineage, ok := agent.ResolveKroLineage(result.Objects[0], result.Objects); ok {
			fmt.Fprint(w, renderKroLineageHuman(lineage))
		}
	}

	// Print ownership detection result
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s%sDetected Owner:%s ", colorBold, colorWhite, colorReset)

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
	case "kro":
		ownerColor = colorBlue
	case "native":
		ownerColor = colorRed
	}

	fmt.Fprintf(w, "%s%s%s", ownerColor, strings.ToUpper(result.Owner), colorReset)
	if result.OwnerDetails != nil && result.OwnerDetails.Name != "" {
		fmt.Fprintf(w, " %s(managed by %s)%s", colorDim, result.OwnerDetails.Name, colorReset)
	}
	fmt.Fprintf(w, "\n")

	// If native, show warning and orphan metadata
	if result.Owner == "native" {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s⚠ No recognized GitOps ownership metadata was found for this resource.%s\n", colorYellow, colorReset)

		// Show orphan metadata if available
		if result.OrphanMeta != nil {
			fmt.Fprintf(w, "\n")
			fmt.Fprintf(w, "%s%sOrphan Metadata:%s\n", colorBold, colorWhite, colorReset)

			if result.OrphanMeta.CreatedAt != nil {
				fmt.Fprintf(w, "  %sCreated:%s %s\n", colorDim, colorReset, result.OrphanMeta.CreatedAt.Format("2006-01-02 15:04:05 MST"))
			}

			// Show relevant labels
			if len(result.OrphanMeta.Labels) > 0 {
				fmt.Fprintf(w, "  %sLabels:%s\n", colorDim, colorReset)
				for k, v := range result.OrphanMeta.Labels {
					// Skip internal labels
					if strings.HasPrefix(k, "kubernetes.io/") ||
						strings.HasPrefix(k, "k8s.io/") {
						continue
					}
					fmt.Fprintf(w, "    %s=%s\n", k, v)
				}
			}

			// Show last-applied-configuration hint
			if result.OrphanMeta.LastAppliedConfigOmission != "" {
				fmt.Fprintf(w, "\n  %s\n", result.OrphanMeta.LastAppliedConfigOmission)
			} else if result.OrphanMeta.LastAppliedConfig != "" {
				fmt.Fprintf(w, "\n")
				fmt.Fprintf(w, "%s%slast-applied-configuration annotation is present%s\n", colorBold, colorGreen, colorReset)
				fmt.Fprintf(w, "%s  Its presence does not establish how this resource was created.%s\n", colorDim, colorReset)
				if result.TopResource == nil || !strings.EqualFold(result.TopResource.Kind, "Secret") {
					fmt.Fprintf(w, "\n  %sTo inspect the annotation, query this resource explicitly.%s\n", colorDim, colorReset)
				}
			} else {
				fmt.Fprintf(w, "\n")
				fmt.Fprintf(w, "%s%sNo last-applied-configuration annotation was found.%s\n", colorBold, colorYellow, colorReset)
				fmt.Fprintf(w, "%s  Its absence does not establish how this resource was created or whether its source is recoverable.%s\n", colorDim, colorReset)
			}
		}
	}

	// If GitOps managed, suggest full trace
	if result.Owner == "flux" || result.Owner == "argo" {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s💡 For full GitOps chain, run:%s\n", colorDim, colorReset)
		if result.TopResource != nil {
			if result.Context == "in-cluster" {
				fmt.Fprintf(w, "   Follow-up command withheld: this label does not distinguish a named context from in-cluster credentials.\n")
			} else {
				selector := ""
				if result.Context != "" {
					selector = " --kube-context '" + strings.ReplaceAll(result.Context, "'", "'\"'\"'") + "'"
				}
				fmt.Fprintf(w, "   ./cub-scout trace %s/%s -n %s%s\n",
					strings.ToLower(result.TopResource.Kind), result.TopResource.Name,
					result.TopResource.Namespace, selector)
			}
		}
	}

	fmt.Fprintf(w, "\n")
	return trackedWriter.err

}

func renderReverseTraceMarkdown(w io.Writer, result *agent.ReverseTraceResult) error {
	if w == nil {
		return fmt.Errorf("reverse trace output writer is nil")
	}
	if result == nil {
		return fmt.Errorf("reverse trace result is nil")
	}
	trackedWriter := &traceHumanWriter{writer: w}
	w = trackedWriter
	fmt.Fprintf(w, "## Reverse trace: %s\n\n", result.Object.String())
	if result.Context != "" {
		fmt.Fprintf(w, "Kubernetes context: %s (selection label; not a stable cluster ID)\n\n", result.Context)
	}
	if result.Error != "" {
		fmt.Fprintf(w, "> [warning] %s\n\n", result.Error)
	}
	if len(result.K8sChain) > 0 {
		fmt.Fprintf(w, "### Kubernetes ownership chain\n\n")
		for _, link := range result.K8sChain {
			fmt.Fprintf(w, "- %s/%s", link.Kind, link.Name)
			if link.Namespace != "" {
				fmt.Fprintf(w, " in %s", link.Namespace)
			}
			if link.Status != "" {
				fmt.Fprintf(w, " — %s", link.Status)
			}
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "\n")
	}
	if len(result.GitOpsChain) > 0 {
		fmt.Fprintf(w, "### GitOps chain\n\n")
		for _, link := range result.GitOpsChain {
			fmt.Fprintf(w, "- %s/%s", link.Kind, link.Name)
			if link.Namespace != "" {
				fmt.Fprintf(w, " in %s", link.Namespace)
			}
			if link.Status != "" {
				fmt.Fprintf(w, " — %s", link.Status)
			}
			fmt.Fprintf(w, "\n")
		}
		fmt.Fprintf(w, "\n")
	}
	if result.Owner != "" {
		fmt.Fprintf(w, "Detected owner: **%s**", strings.ToUpper(result.Owner))
		if result.OwnerDetails != nil && result.OwnerDetails.Name != "" {
			fmt.Fprintf(w, " (managed by %s)", result.OwnerDetails.Name)
		}
		fmt.Fprintf(w, "\n")
	}
	if result.OrphanMeta != nil && result.OrphanMeta.LastAppliedConfigOmission != "" {
		fmt.Fprintf(w, "\n> [omission] %s\n", result.OrphanMeta.LastAppliedConfigOmission)
	}
	return trackedWriter.err
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
	session, err := newDefaultTraceSession()
	if err != nil {
		return err
	}
	ownership, err := detectResourceOwnershipWithTraceSession(ctx, session, kind, name, namespace)
	if err != nil {
		// Missing access is not evidence that this resource is unmanaged.
		return fmt.Errorf("cannot establish ownership for diff of %s/%s in %s: %w", kind, name, namespace, err)
	}

	switch ownership.Type {
	case agent.OwnerFlux:
		return runFluxDiff(ctx, kind, name, namespace, ownership)
	case agent.OwnerArgo:
		return runArgoDiff(ctx, name, ownership)
	case agent.OwnerHelm:
		return runHelmDiff(ctx, name, namespace)
	case agent.OwnerSveltos:
		return runObservedControllerDiffNotice("Sveltos", kind, name, namespace, []string{
			fmt.Sprintf("cub-scout trace %s/%s -n %s", strings.ToLower(kind), name, namespace),
			"cub-scout map activity --owner Sveltos",
		})
	case agent.OwnerModelplane:
		return runObservedControllerDiffNotice("Modelplane", kind, name, namespace, []string{
			fmt.Sprintf("cub-scout trace %s/%s -n %s", strings.ToLower(kind), name, namespace),
			"cub-scout map activity --owner Modelplane",
		})
	default:
		fmt.Printf("%s⚠ Resource is not managed by GitOps (owner: %s)%s\n", colorYellow, ownershipLabel(ownership), colorReset)
		fmt.Printf("%s  Cannot show diff for unmanaged resources.%s\n", colorDim, colorReset)
		fmt.Printf("%s  Consider importing to GitOps: cub-scout import%s\n", colorDim, colorReset)
		fmt.Printf("\n")
		return nil
	}
}

func runObservedControllerDiffNotice(owner, kind, name, namespace string, next []string) error {
	fmt.Printf("%sObserved controller mode: %s%s\n", colorCyan, owner, colorReset)
	fmt.Printf("%s  cub-scout can trace this controller's live ownership/provenance, but there is no %s-specific desired-state diff command wired here.%s\n", colorDim, owner, colorReset)
	fmt.Printf("%s  Use trace/activity to inspect owner, references, status, and recent condition changes.%s\n\n", colorDim, colorReset)
	for _, cmd := range next {
		cmd = strings.ReplaceAll(cmd, " -n ", " --namespace ")
		if strings.Contains(cmd, "--namespace ") && strings.HasSuffix(cmd, "--namespace ") {
			cmd = strings.TrimSuffix(cmd, "--namespace ")
		}
		fmt.Printf("  %s%s%s\n", colorCyan, strings.TrimSpace(cmd), colorReset)
	}
	fmt.Printf("\n")
	return nil
}

func ownershipLabel(ownership *agent.Ownership) string {
	if ownership == nil {
		return "unknown"
	}
	if ownership.Type == agent.OwnerCustom {
		if name := strings.TrimSpace(ownership.Name); name != "" {
			return name
		}
		return "custom"
	}
	if ownership.Type == "" {
		return "unknown"
	}
	return ownership.Type
}

func buildCustomOwnerUnsupportedTraceResult(kind, name, namespace string, ownership *agent.Ownership) *agent.TraceResult {
	ownerName := ownershipLabel(ownership)
	if ownerName == "" || ownerName == "unknown" {
		ownerName = "Custom"
	}

	return &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		FullyManaged: false,
		Tool:         "",
		Error:        fmt.Sprintf("custom owner detected: %s (trace chain unavailable for custom owners)", ownerName),
		TracedAt:     time.Now(),
	}
}

func buildCrossplaneObservedTraceResult(kind, name, namespace string, ownership *agent.Ownership) *agent.TraceResult {
	objectNamespace := namespace
	if kind == "ProviderConfig" {
		objectNamespace = strings.TrimSpace(ownership.Namespace)
	} else if trimmed := strings.TrimSpace(ownership.Namespace); trimmed != "" {
		objectNamespace = trimmed
	}

	return &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: objectNamespace,
		},
		Chain: []agent.ChainLink{
			{
				Kind:      kind,
				Name:      name,
				Namespace: objectNamespace,
				Ready:     true,
				Status:    "Observed",
			},
		},
		FullyManaged: false,
		Tool:         "crossplane",
		Error:        "Crossplane resource detected: GitOps trace chain unavailable; showing direct resource evidence only.",
		TracedAt:     time.Now(),
	}
}

func buildSveltosObservedTraceResult(ctx context.Context, dynClient dynamic.Interface, kind, name, namespace string, ownership *agent.Ownership) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Tool:     "sveltos",
		TracedAt: time.Now(),
	}

	resource, err := fetchTraceResource(ctx, dynClient, kind, name, namespace)
	if err != nil {
		result.Chain = append(result.Chain, unreadableChainLink(kind, name, namespace, err))
		return result
	}
	result.Object = resourceRefFromObject(resource)
	annotations := resource.GetAnnotations()

	if ref := sveltosReferenceLink(annotations); ref != nil {
		result.Chain = append(result.Chain, *ref)
	}

	ownerKind := strings.TrimSpace(annotations["projectsveltos.io/owner-kind"])
	ownerName := strings.TrimSpace(annotations["projectsveltos.io/owner-name"])
	if ownerKind == "" && ownership != nil && ownership.Type == agent.OwnerSveltos {
		ownerKind = sveltosKindFromSubtype(ownership.SubType)
		ownerName = strings.TrimSpace(ownership.Name)
	}
	if ownerKind != "" && ownerName != "" && !sameObject(resource, ownerKind, ownerName) {
		result.Chain = append(result.Chain, sveltosOwnerLink(ctx, dynClient, ownerKind, ownerName, resource.GetNamespace()))
	}

	link := chainLinkFromObject(resource)
	if ownerKind != "" && ownerName != "" {
		link.Message = appendSentence(link.Message, fmt.Sprintf("Sveltos owner annotation points to %s/%s.", ownerKind, ownerName))
	}
	if refKind := strings.TrimSpace(annotations["projectsveltos.io/reference-kind"]); refKind != "" {
		link.Message = appendSentence(link.Message, "Reference annotations explain which ConfigMap/Secret supplied the deployed manifest.")
	}
	result.Chain = append(result.Chain, link)
	result.FullyManaged = ownerKind != "" && ownerName != ""
	return result
}

func buildModelplaneObservedTraceResult(ctx context.Context, dynClient dynamic.Interface, kind, name, namespace string, ownership *agent.Ownership) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Tool:     "modelplane",
		TracedAt: time.Now(),
	}

	resource, err := fetchTraceResource(ctx, dynClient, kind, name, namespace)
	if err != nil {
		result.Chain = append(result.Chain, unreadableChainLink(kind, name, namespace, err))
		return result
	}
	result.Object = resourceRefFromObject(resource)

	if parent := modelplaneParentLink(ctx, dynClient, resource, ownership); parent != nil {
		result.Chain = append(result.Chain, *parent)
	}

	link := chainLinkFromObject(resource)
	if children := modelplaneChildren(ctx, dynClient, resource); len(children) > 0 {
		link.Children = children
	}
	link.Message = appendSentence(link.Message, modelplaneEvidenceMessage(resource, ownership))
	result.Chain = append(result.Chain, link)
	result.FullyManaged = ownership != nil && ownership.Type == agent.OwnerModelplane
	return result
}

func unreadableChainLink(kind, name, namespace string, err error) agent.ChainLink {
	return agent.ChainLink{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Ready:     false,
		Status:    "Unreadable",
		Message:   err.Error(),
	}
}

func resourceRefFromObject(obj *unstructured.Unstructured) agent.ResourceRef {
	if obj == nil {
		return agent.ResourceRef{}
	}
	gv := obj.GroupVersionKind().GroupVersion()
	return agent.ResourceRef{
		Kind:      obj.GetKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Group:     gv.Group,
		Version:   gv.Version,
	}
}

func chainLinkFromObject(obj *unstructured.Unstructured) agent.ChainLink {
	ready, status, reason, message := observedObjectStatus(obj)
	link := agent.ChainLink{
		Kind:         obj.GetKind(),
		Name:         obj.GetName(),
		Namespace:    obj.GetNamespace(),
		Ready:        ready,
		Status:       status,
		StatusReason: reason,
		Message:      message,
	}
	if transition := observedLastTransitionTime(obj); transition != nil {
		link.LastTransitionTime = transition
	}
	return link
}

func observedObjectStatus(obj *unstructured.Unstructured) (bool, string, string, string) {
	if obj == nil {
		return false, "Unknown", "", "resource not loaded"
	}

	if failed, found, _ := unstructured.NestedSlice(obj.Object, "status", "failedClusters"); found && len(failed) > 0 {
		return false, "Failed", "FailedClusters", fmt.Sprintf("%d cluster(s) failed", len(failed))
	}
	if updating, found, _ := unstructured.NestedSlice(obj.Object, "status", "updatingClusters"); found && len(updating) > 0 {
		return false, "Updating", "UpdatingClusters", fmt.Sprintf("%d cluster(s) updating", len(updating))
	}
	if updated, found, _ := unstructured.NestedSlice(obj.Object, "status", "updatedClusters"); found && len(updated) > 0 {
		return true, "Ready", "UpdatedClusters", fmt.Sprintf("%d cluster(s) updated", len(updated))
	}
	if matching, found, _ := unstructured.NestedSlice(obj.Object, "status", "matchingClusters"); found && len(matching) > 0 {
		return true, "Matched", "MatchingClusters", fmt.Sprintf("%d matching cluster(s)", len(matching))
	}

	if phase, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found && phase != "" {
		switch strings.ToLower(phase) {
		case "ready", "available", "succeeded", "bound":
			return true, phase, "phase", ""
		case "failed", "error":
			return false, phase, "phase", ""
		default:
			return false, phase, "phase", ""
		}
	}

	total, totalFound, _ := unstructured.NestedInt64(obj.Object, "status", "replicas", "total")
	readyReplicas, readyFound, _ := unstructured.NestedInt64(obj.Object, "status", "replicas", "ready")
	if totalFound || readyFound {
		msg := fmt.Sprintf("%d/%d replicas ready", readyReplicas, total)
		if total > 0 && readyReplicas >= total {
			return true, "Ready", "ReplicasReady", msg
		}
		return false, "NotReady", "ReplicasReady", msg
	}

	if ready, status, reason, message, ok := statusFromConditions(obj); ok {
		return ready, status, reason, message
	}

	return true, "Observed", "", ""
}

func statusFromConditions(obj *unstructured.Unstructured) (bool, string, string, string, bool) {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return false, "", "", "", false
	}
	preferred := []string{"Ready", "Synced", "Healthy", "ControllerReady", "RoutingReady", "ClusterReady", "BackendReady"}
	for _, want := range preferred {
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if !strings.EqualFold(stringValue(cond["type"]), want) {
				continue
			}
			status := stringValue(cond["status"])
			reason := stringValue(cond["reason"])
			message := stringValue(cond["message"])
			return strings.EqualFold(status, "True"), conditionDisplayStatus(want, status), reason, message, true
		}
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		status := stringValue(cond["status"])
		if strings.EqualFold(status, "False") {
			condType := stringValue(cond["type"])
			return false, conditionDisplayStatus(condType, status), stringValue(cond["reason"]), stringValue(cond["message"]), true
		}
	}
	return true, "Observed", "conditions", fmt.Sprintf("%d condition(s) reported", len(conditions)), true
}

func conditionDisplayStatus(conditionType, status string) string {
	if strings.EqualFold(status, "True") {
		return "Ready"
	}
	if conditionType == "" {
		return "NotReady"
	}
	return conditionType + "=" + status
}

func observedLastTransitionTime(obj *unstructured.Unstructured) *time.Time {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return nil
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		value := stringValue(cond["lastTransitionTime"])
		if value == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			return &t
		}
	}
	return nil
}

func sveltosReferenceLink(annotations map[string]string) *agent.ChainLink {
	refKind := strings.TrimSpace(annotations["projectsveltos.io/reference-kind"])
	refName := strings.TrimSpace(annotations["projectsveltos.io/reference-name"])
	if refKind == "" || refName == "" {
		return nil
	}
	refNS := strings.TrimSpace(annotations["projectsveltos.io/reference-namespace"])
	refTier := strings.TrimSpace(annotations["projectsveltos.io/reference-tier"])
	name := refKind + "/" + refName
	if refNS != "" {
		name = refKind + "/" + refNS + "/" + refName
	}
	msg := "Sveltos reference that supplied the manifest/template."
	if refTier != "" {
		msg = appendSentence(msg, "Reference tier "+refTier+".")
	}
	return &agent.ChainLink{
		Kind:      "SveltosReference",
		Name:      name,
		Namespace: refNS,
		Ready:     true,
		Status:    "Referenced",
		Message:   msg,
	}
}

func sveltosOwnerLink(ctx context.Context, dynClient dynamic.Interface, ownerKind, ownerName, fallbackNamespace string) agent.ChainLink {
	namespace := ""
	if strings.EqualFold(ownerKind, "Profile") {
		namespace = fallbackNamespace
	}
	if spec, ok := controllerResourceByKind(ownerKind); ok {
		if obj, err := getControllerResource(ctx, dynClient, spec, ownerName, namespace); err == nil {
			link := chainLinkFromObject(obj)
			link.Message = appendSentence(link.Message, "Sveltos deployment owner.")
			return link
		} else if !apierrors.IsNotFound(err) {
			return agent.ChainLink{
				Kind:      ownerKind,
				Name:      ownerName,
				Namespace: namespace,
				Ready:     false,
				Status:    "Unreadable",
				Message:   err.Error(),
			}
		}
	}
	return agent.ChainLink{
		Kind:      ownerKind,
		Name:      ownerName,
		Namespace: namespace,
		Ready:     true,
		Status:    "Observed",
		Message:   "Sveltos owner identified from projectsveltos.io/owner-kind and owner-name annotations.",
	}
}

func sveltosKindFromSubtype(subType string) string {
	switch strings.ToLower(strings.TrimSpace(subType)) {
	case "clusterprofile":
		return "ClusterProfile"
	case "profile":
		return "Profile"
	case "eventsource":
		return "EventSource"
	case "eventtrigger":
		return "EventTrigger"
	case "clusterhealthcheck":
		return "ClusterHealthCheck"
	default:
		return ""
	}
}

func modelplaneParentLink(ctx context.Context, dynClient dynamic.Interface, resource *unstructured.Unstructured, ownership *agent.Ownership) *agent.ChainLink {
	if resource == nil {
		return nil
	}
	for _, ownerRef := range resource.GetOwnerReferences() {
		if !strings.Contains(ownerRef.APIVersion, "modelplane.ai") {
			continue
		}
		if sameObject(resource, ownerRef.Kind, ownerRef.Name) {
			continue
		}
		if link := fetchModelplaneLink(ctx, dynClient, ownerRef.Kind, ownerRef.Name, resource.GetNamespace()); link != nil {
			link.Message = appendSentence(link.Message, "Modelplane ownerReference.")
			return link
		}
		return &agent.ChainLink{
			Kind:      ownerRef.Kind,
			Name:      ownerRef.Name,
			Namespace: resource.GetNamespace(),
			Ready:     true,
			Status:    "Observed",
			Message:   "Modelplane ownerReference present; owner object was not readable from this context.",
		}
	}

	labels := resource.GetLabels()
	candidates := []struct {
		label string
		kind  string
	}{
		{label: "modelplane.ai/deployment", kind: "ModelDeployment"},
		{label: "modelplane.ai/modelcache", kind: "ModelCache"},
		{label: "modelplane.ai/serving", kind: "ModelReplica"},
		{label: "modelplane.ai/cluster", kind: "InferenceCluster"},
	}
	for _, candidate := range candidates {
		value := strings.TrimSpace(labels[candidate.label])
		if value == "" || sameObject(resource, candidate.kind, value) {
			continue
		}
		if link := fetchModelplaneLink(ctx, dynClient, candidate.kind, value, resource.GetNamespace()); link != nil {
			link.Message = appendSentence(link.Message, "Modelplane label "+candidate.label+" points here.")
			return link
		}
		return &agent.ChainLink{
			Kind:      candidate.kind,
			Name:      value,
			Namespace: resource.GetNamespace(),
			Ready:     true,
			Status:    "Observed",
			Message:   "Modelplane label " + candidate.label + " points here; owner object was not readable from this context.",
		}
	}

	if ownership != nil && ownership.Type == agent.OwnerModelplane {
		kind := modelplaneKindFromSubtype(ownership.SubType)
		if kind != "" && ownership.Name != "" && !sameObject(resource, kind, ownership.Name) {
			if link := fetchModelplaneLink(ctx, dynClient, kind, ownership.Name, firstNonEmpty(ownership.Namespace, resource.GetNamespace())); link != nil {
				link.Message = appendSentence(link.Message, "Modelplane ownership signal.")
				return link
			}
		}
	}
	return nil
}

func fetchModelplaneLink(ctx context.Context, dynClient dynamic.Interface, kind, name, namespace string) *agent.ChainLink {
	spec, ok := controllerResourceByKind(kind)
	if !ok {
		return nil
	}
	obj, err := getControllerResource(ctx, dynClient, spec, name, namespace)
	if err != nil {
		return nil
	}
	link := chainLinkFromObject(obj)
	if children := modelplaneChildren(ctx, dynClient, obj); len(children) > 0 {
		link.Children = children
	}
	return &link
}

func modelplaneChildren(ctx context.Context, dynClient dynamic.Interface, resource *unstructured.Unstructured) []agent.ResourceRef {
	if resource == nil {
		return nil
	}
	var out []agent.ResourceRef
	ns := resource.GetNamespace()
	name := resource.GetName()
	switch resource.GetKind() {
	case "ModelDeployment":
		out = append(out, modelplaneChildrenByLabel(ctx, dynClient, "ModelReplica", ns, "modelplane.ai/deployment", name)...)
		out = append(out, modelplaneChildrenByLabel(ctx, dynClient, "ModelEndpoint", ns, "modelplane.ai/deployment", name)...)
	case "ModelCache":
		out = append(out, modelplaneChildrenByLabel(ctx, dynClient, "ModelReplica", ns, "modelplane.ai/modelcache", name)...)
	case "ModelService":
		selector, _, _ := unstructured.NestedStringMap(resource.Object, "spec", "selector", "matchLabels")
		if deployment := strings.TrimSpace(selector["modelplane.ai/deployment"]); deployment != "" {
			out = append(out, modelplaneChildrenByLabel(ctx, dynClient, "ModelEndpoint", ns, "modelplane.ai/deployment", deployment)...)
		}
	case "InferenceCluster":
		out = append(out, modelplaneChildrenByLabel(ctx, dynClient, "ModelReplica", ns, "modelplane.ai/cluster", name)...)
	}
	return out
}

func modelplaneChildrenByLabel(ctx context.Context, dynClient dynamic.Interface, kind, namespace, key, value string) []agent.ResourceRef {
	spec, ok := controllerResourceByKind(kind)
	if !ok {
		return nil
	}
	list, err := listControllerResource(ctx, dynClient, spec, namespace)
	if err != nil {
		return nil
	}
	out := make([]agent.ResourceRef, 0, len(list.Items))
	for i := range list.Items {
		item := list.Items[i]
		if item.GetLabels()[key] != value {
			continue
		}
		out = append(out, resourceRefFromObject(&item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func modelplaneEvidenceMessage(resource *unstructured.Unstructured, ownership *agent.Ownership) string {
	if resource == nil {
		return ""
	}
	message := ""
	if ownership != nil && ownership.Source != "" {
		message = appendSentence(message, "Ownership evidence: "+ownership.Source+".")
	}
	if crossplaneEvidence := modelplaneCrossplaneEvidenceMessage(resource, ownership); crossplaneEvidence != "" {
		message = appendSentence(message, crossplaneEvidence)
	}
	if message != "" {
		return message
	}
	for key, value := range resource.GetLabels() {
		if strings.HasPrefix(key, "modelplane.ai/") && value != "" {
			return "Modelplane label " + key + " is present."
		}
	}
	if group := resource.GroupVersionKind().Group; group == "modelplane.ai" || group == "infrastructure.modelplane.ai" {
		return "Modelplane API resource."
	}
	return ""
}

func modelplaneCrossplaneEvidenceMessage(resource *unstructured.Unstructured, ownership *agent.Ownership) string {
	if ownership == nil {
		return ""
	}
	evidence, ok := agent.BuildModelplaneCrossplaneEvidence(resource, *ownership)
	if !ok {
		return ""
	}
	return evidence.Summary()
}

func modelplaneKindFromSubtype(subType string) string {
	switch strings.ToLower(strings.TrimSpace(subType)) {
	case "modeldeployment":
		return "ModelDeployment"
	case "modelservice":
		return "ModelService"
	case "modelendpoint":
		return "ModelEndpoint"
	case "modelcache":
		return "ModelCache"
	case "modelreplica":
		return "ModelReplica"
	case "inferencecluster":
		return "InferenceCluster"
	case "inferenceclass":
		return "InferenceClass"
	case "inferencegateway":
		return "InferenceGateway"
	default:
		return ""
	}
}

func sameObject(obj *unstructured.Unstructured, kind, name string) bool {
	if obj == nil {
		return false
	}
	return strings.EqualFold(obj.GetKind(), kind) && obj.GetName() == name
}

func stringValue(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func appendSentence(base, sentence string) string {
	base = strings.TrimSpace(base)
	sentence = strings.TrimSpace(sentence)
	if sentence == "" {
		return base
	}
	if base == "" {
		return sentence
	}
	return strings.TrimRight(base, ".") + ". " + sentence
}

func isCrossplaneProviderConfig(resource *unstructured.Unstructured) bool {
	if resource == nil || resource.GetKind() != "ProviderConfig" {
		return false
	}
	group := resource.GroupVersionKind().Group
	return strings.HasSuffix(group, ".crossplane.io") || strings.HasSuffix(group, ".upbound.io")
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
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Run()
	if err != nil {
		combinedOutput := stderrBuf.String() + stdoutBuf.String()
		if help, ok := agent.FormatArgoContextError(combinedOutput); ok {
			return fmt.Errorf("%s", help)
		}

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

// formatElapsed formats a duration in a human-readable way
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

func traceArtifactKey(kind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

func isTraceSourceKind(kind string) bool {
	switch kind {
	case "GitRepository", "OCIRepository", "ConfigHub OCI", "HelmRepository", "Bucket":
		return true
	default:
		return false
	}
}

func traceArtifactUnknownForKind(kind string) mapsvc.TraceArtifactRef {
	sourceKind := kind
	if strings.TrimSpace(sourceKind) == "" {
		sourceKind = "unknown"
	}
	return mapsvc.TraceArtifactRef{
		URL:            "unknown",
		Revision:       "unknown",
		Digest:         "unknown",
		LastUpdateTime: "unknown",
		SourceKind:     sourceKind,
	}
}

func normalizeTraceArtifact(kind string, artifact mapsvc.TraceArtifactRef) mapsvc.TraceArtifactRef {
	out := artifact
	if strings.TrimSpace(out.URL) == "" {
		out.URL = "unknown"
	}
	if strings.TrimSpace(out.Revision) == "" {
		out.Revision = "unknown"
	}
	if strings.TrimSpace(out.Digest) == "" {
		out.Digest = "unknown"
	}
	if strings.TrimSpace(out.LastUpdateTime) == "" {
		out.LastUpdateTime = "unknown"
	}
	if strings.TrimSpace(out.SourceKind) == "" {
		out.SourceKind = firstNonEmpty(kind, "unknown")
	}
	return out
}

func buildUnknownTraceArtifacts(result *agent.TraceResult) map[string]mapsvc.TraceArtifactRef {
	artifacts := make(map[string]mapsvc.TraceArtifactRef)
	if result == nil {
		return artifacts
	}
	for _, link := range result.Chain {
		if !isTraceSourceKind(link.Kind) {
			continue
		}
		key := traceArtifactKey(link.Kind, link.Namespace, link.Name)
		if _, ok := artifacts[key]; ok {
			continue
		}
		artifacts[key] = traceArtifactUnknownForKind(link.Kind)
	}
	return artifacts
}

func mergeTraceArtifacts(base, updates map[string]mapsvc.TraceArtifactRef) map[string]mapsvc.TraceArtifactRef {
	merged := make(map[string]mapsvc.TraceArtifactRef, len(base)+len(updates))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range updates {
		kind := strings.SplitN(key, "/", 2)[0]
		merged[key] = normalizeTraceArtifact(kind, value)
	}
	return merged
}

func artifactForLink(link agent.ChainLink, artifacts map[string]mapsvc.TraceArtifactRef) mapsvc.TraceArtifactRef {
	if len(artifacts) > 0 {
		if artifact, ok := lookupTraceArtifact(link.Kind, link.Namespace, link.Name, artifacts); ok {
			return normalizeTraceArtifact(link.Kind, artifact)
		}
	}
	return traceArtifactUnknownForKind(link.Kind)
}

func lookupTraceArtifact(kind, namespace, name string, artifacts map[string]mapsvc.TraceArtifactRef) (mapsvc.TraceArtifactRef, bool) {
	if len(artifacts) == 0 {
		return mapsvc.TraceArtifactRef{}, false
	}

	var fallback mapsvc.TraceArtifactRef
	var foundFallback bool

	for _, key := range traceArtifactLookupKeys(kind, namespace, name) {
		if artifact, ok := artifacts[key]; ok {
			norm := normalizeTraceArtifact(kind, artifact)
			if isUnknownTraceArtifact(norm) {
				if !foundFallback {
					fallback = norm
					foundFallback = true
				}
				continue
			}
			return norm, true
		}
	}
	if foundFallback {
		return fallback, true
	}
	return mapsvc.TraceArtifactRef{}, false
}

func traceArtifactLookupKeys(kind, namespace, name string) []string {
	keys := []string{traceArtifactKey(kind, namespace, name)}
	switch kind {
	case "ConfigHub OCI":
		keys = append(keys, traceArtifactKey("OCIRepository", namespace, name))
	case "OCIRepository":
		keys = append(keys, traceArtifactKey("ConfigHub OCI", namespace, name))
	}
	return keys
}

func isUnknownTraceArtifact(artifact mapsvc.TraceArtifactRef) bool {
	return strings.TrimSpace(artifact.URL) == "unknown" &&
		strings.TrimSpace(artifact.Revision) == "unknown" &&
		strings.TrimSpace(artifact.Digest) == "unknown" &&
		strings.TrimSpace(artifact.LastUpdateTime) == "unknown"
}

type traceArtifactFixtureItem struct {
	Kind           string `json:"kind"`
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Revision       string `json:"revision"`
	Digest         string `json:"digest"`
	LastUpdateTime string `json:"lastUpdateTime"`
	SourceKind     string `json:"sourceKind"`
}

func loadTraceArtifactsFromJSON(path string) (map[string]mapsvc.TraceArtifactRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read trace artifact JSON: %w", err)
	}

	artifacts := make(map[string]mapsvc.TraceArtifactRef)

	var items []traceArtifactFixtureItem
	if err := json.Unmarshal(data, &items); err == nil {
		for _, item := range items {
			kind := firstNonEmpty(item.Kind, item.SourceKind)
			if kind == "" || item.Name == "" {
				continue
			}
			key := traceArtifactKey(kind, item.Namespace, item.Name)
			artifacts[key] = normalizeTraceArtifact(kind, mapsvc.TraceArtifactRef{
				URL:            item.URL,
				Revision:       item.Revision,
				Digest:         item.Digest,
				LastUpdateTime: item.LastUpdateTime,
				SourceKind:     firstNonEmpty(item.SourceKind, kind),
			})
		}
		return artifacts, nil
	}

	var byKey map[string]mapsvc.TraceArtifactRef
	if err := json.Unmarshal(data, &byKey); err != nil {
		return nil, fmt.Errorf("failed to parse trace artifact JSON: %w", err)
	}
	for key, artifact := range byKey {
		kind := strings.SplitN(key, "/", 2)[0]
		artifacts[key] = normalizeTraceArtifact(kind, artifact)
	}
	return artifacts, nil
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

// loadAndRenderTraceFromJSON loads trace data from a JSON file and renders it.
// This is used for golden testing to bypass cluster access.
func loadAndRenderTraceFromJSON(path string, invCtx InvocationContext) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read trace JSON: %w", err)
	}

	var result agent.TraceResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to parse trace JSON: %w", err)
	}

	// Resolve effective format
	effectiveFormat := traceFormat
	if traceJSON && effectiveFormat == "ascii" {
		effectiveFormat = "json"
	}

	artifacts := buildUnknownTraceArtifacts(&result)
	if traceArtifacts {
		if fixture := os.Getenv("CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON"); fixture != "" {
			loaded, err := loadTraceArtifactsFromJSON(fixture)
			if err != nil {
				return err
			}
			artifacts = mergeTraceArtifacts(artifacts, loaded)
		}
	}

	switch effectiveFormat {
	case "json":
		return outputTraceJSONv014(&result, result.Object.Kind, result.Object.Name, result.Object.Namespace, artifacts)
	case "md":
		return outputTraceMarkdown(&result, artifacts, invCtx)
	default:
		return outputTraceHuman(&result, artifacts, invCtx)
	}
}
