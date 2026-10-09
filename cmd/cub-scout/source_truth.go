// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// cub-scout source-truth — v0.1 of the source-truth evidence contract
// per the council verdict on #393. This file owns the CLI surface and the
// live collectors that populate the three surfaces; the contract types
// and decision logic live in pkg/agent/source_truth.go and
// pkg/agent/source_truth_logic.go.
//
// v0.1 supports the workload kinds the kstatus wrapper covers
// (Deployment, StatefulSet, DaemonSet) — those are the kinds the
// source-truth contract surfaces today. Other kinds emit ASK because
// cub-scout does not have a runtime surface for them yet.
//
// The command is deliberately read-only and refuses to run unless
// connected mode auth is in place — the contract is meaningless without
// the ConfigHub surface.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

var (
	sourceTruthNamespace string
	sourceTruthStrategy  string
	sourceTruthFormat    string
	sourceTruthContext   string
)

type sourceTruthObservation struct {
	Evidence        agent.SourceTruthEvidence
	RuntimeError    error
	ControllerError error
	ConfigHubError  error
}

var sourceTruthUnitGet = func(ctx context.Context, unit, space string) ([]byte, error) {
	return cubStdout(ctx, withConfigHubSpace([]string{"unit", "get", unit, "-o", "json"}, space)...)
}

var sourceTruthCmd = &cobra.Command{
	Use:   "source-truth <kind>/<name> | <kind> <name>",
	Short: "Read-only source-truth evidence for a single workload",
	Long: `source-truth emits read-only evidence about a single workload's intended,
controller-observed, and runtime state. Output is the structured JSON
contract Pilot's acceptance kernel consumes (#393).

cub-scout is the *evidence provider*; Pilot is the acceptance judge.
This command never mutates, repairs, approves, or infers authority.

Strategy is required input — cub-scout never infers the delivery path.
Strategies (Phase 1 + Phase 2 per #418):

  confighub-oci-argo   ConfigHub -> OCI -> Argo CD          -> Kubernetes (unit revision anchor)
  confighub-oci-flux   ConfigHub -> OCI -> Flux             -> Kubernetes (unit revision anchor)
  git-argo             Git       -> Argo CD                 -> Kubernetes (Git SHA anchor)
  git-flux             Git       -> Flux                    -> Kubernetes (Git SHA anchor)
  helm-argo            Helm chart -> Argo CD                -> Kubernetes (chart version anchor)
  helm-flux            Helm chart -> Flux HelmRelease       -> Kubernetes (chart version anchor)
  kustomize-flux       Git source -> Flux Kustomization     -> Kubernetes (Git SHA + overlay)
  oci-argo             OCI (non-ConfigHub) -> Argo CD       -> Kubernetes (OCI digest anchor)
  oci-flux             OCI (non-ConfigHub) -> Flux OCIRepo  -> Kubernetes (OCI digest anchor)

Examples:
  cub-scout compare source-truth deploy/rag-server -n demo --strategy confighub-oci-flux
  cub scout compare source-truth Deployment rag-server -n demo --strategy git-argo
  cub-scout compare source-truth statefulset/db -n prod --strategy helm-flux
  cub-scout compare source-truth deploy/api -n prod --strategy git-argo --kube-context prod-west

Supports Deployment, StatefulSet, DaemonSet. Other kinds emit ASK.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSourceTruth,
}

// sourceTruthStrategyFlagHelp builds the --strategy flag help string from the
// canonical enum in pkg/agent so help text never drifts from code. Adding a
// new strategy to agent.AllStrategies() automatically extends the help.
func sourceTruthStrategyFlagHelp() string {
	all := agent.AllStrategies()
	values := make([]string, 0, len(all))
	for _, s := range all {
		values = append(values, string(s))
	}
	return fmt.Sprintf("Declared delivery path. Required. One of: %s", strings.Join(values, ", "))
}

func init() {
	// Lives under `compare` per the council's "extend the existing three-
	// way comparison" framing. Keeps the top-level command surface lean.
	combinedCmd.AddCommand(sourceTruthCmd)
	sourceTruthCmd.Flags().StringVarP(&sourceTruthNamespace, "namespace", "n", "", "Namespace of the resource (required for namespaced kinds)")
	sourceTruthCmd.Flags().StringVar(&sourceTruthStrategy, "strategy", "", sourceTruthStrategyFlagHelp())
	sourceTruthCmd.Flags().StringVar(&sourceTruthFormat, "format", "json", "Output format: ascii, json, md")
	sourceTruthCmd.Flags().StringVar(&sourceTruthContext, "kube-context", "", "Select one exact kubeconfig context for runtime and controller reads")
}

func runSourceTruth(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(sourceTruthFormat))
	if format != "json" && format != "ascii" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", sourceTruthFormat)
	}

	// Parse positional arg(s) using the same helper explain uses, so the
	// "kind/name" or "kind name" surface is consistent.
	kind, name, err := parseExplainArgs(args)
	if err != nil {
		return err
	}
	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return err
	}
	// An unknown strategy remains an offline ASK result when the option is
	// omitted. When the caller explicitly names a context, still validate that
	// selection before returning the ASK document so invalid input never gets
	// silently accepted or mistaken for the default binding.
	strategy, ok := agent.ParseStrategy(sourceTruthStrategy)
	if !ok {
		evidence := agent.Derive("", agent.SourceTruthSurfaces{})
		if selection.explicit {
			session, err := newTraceSessionForSelection(selection)
			if err != nil {
				return fmt.Errorf("resolve selected Kubernetes context: %w", err)
			}
			evidence.Context = session.contextLabel()
		}
		return outputSourceTruth(os.Stdout, evidence, format)
	}

	var session *traceSession
	if selection.explicit {
		// Validate a named context before any Kubernetes request. This only
		// resolves local kubeconfig data and does not contact the selected API.
		session, err = newTraceSessionForSelection(selection)
		if err != nil {
			return fmt.Errorf("resolve selected Kubernetes context: %w", err)
		}
	}
	if err := requireConfigHubFor("compare source-truth"); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("source-truth requires a non-empty workload name")
	}
	if strings.TrimSpace(sourceTruthNamespace) == "" {
		return fmt.Errorf("source-truth requires a workload namespace with -n/--namespace")
	}
	if session == nil {
		session, err = newTraceSessionForSelection(selection)
		if err != nil {
			return fmt.Errorf("resolve selected Kubernetes context: %w", err)
		}
	}
	observation := collectSourceTruthObservation(cmd.Context(), session, kind, name, sourceTruthNamespace, strategy)
	return outputSourceTruth(os.Stdout, observation.Evidence, format)
}

func outputSourceTruth(w io.Writer, ev agent.SourceTruthEvidence, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return agent.EncodeEvidence(w, ev)
	case "md":
		_, err := io.WriteString(w, renderSourceTruthMarkdown(ev))
		return err
	default:
		_, err := io.WriteString(w, renderSourceTruthASCII(ev))
		return err
	}
}

// collectSourceTruthObservation shares the complete evidence path between CLI
// and TUI. Kubernetes reads use one captured session; `cub unit get` remains a
// distinct ConfigHub server-side read.
func collectSourceTruthObservation(ctx context.Context, session *traceSession, kind, name, namespace string, strategy agent.SourceTruthStrategy) sourceTruthObservation {
	observation := sourceTruthObservation{}
	var runtimeSurface *agent.RuntimeSurface
	var workload *runtimeWorkload
	if session == nil {
		observation.RuntimeError = fmt.Errorf("selected Kubernetes session is unavailable")
	} else {
		runtimeSurface, workload, observation.RuntimeError = collectRuntimeSurfaceWithTraceSession(ctx, session, kind, name, namespace)
	}
	var configHubSurface *agent.ConfigHubSurface
	if observation.RuntimeError == nil {
		configHubSurface, observation.ConfigHubError = collectConfigHubSurface(ctx, workload)
	} else {
		observation.ConfigHubError = fmt.Errorf("ConfigHub lookup skipped because the runtime object was unavailable")
	}
	var controllerSurface *agent.ControllerSurface
	if observation.RuntimeError == nil && session != nil {
		controllerSurface, observation.ControllerError = collectControllerSurfaceWithTraceSession(ctx, session, strategy, kind, name, namespace, workload)
	} else {
		observation.ControllerError = fmt.Errorf("controller lookup skipped because the runtime object was unavailable")
	}
	observation.Evidence = agent.Derive(strategy, agent.SourceTruthSurfaces{ConfigHub: configHubSurface, Controller: controllerSurface, Runtime: runtimeSurface})
	if session != nil {
		observation.Evidence.Context = session.contextLabel()
	}
	for _, item := range []struct {
		name string
		err  error
	}{{"runtime", observation.RuntimeError}, {"ConfigHub", observation.ConfigHubError}, {"controller", observation.ControllerError}} {
		if item.err != nil {
			observation.Evidence.CollectionErrors = append(observation.Evidence.CollectionErrors, item.name+": "+item.err.Error())
		}
	}
	return observation
}

// runtimeWorkload is a thin sum type over the typed apps/v1 workloads
// v0.1 supports. Carrying the typed value (rather than unstructured)
// lets us use the kstatus wrapper directly and keeps image extraction
// straightforward.
type runtimeWorkload struct {
	Kind        string
	Namespace   string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Object      *unstructured.Unstructured

	// Exactly one of these is set.
	Deployment  *appsv1.Deployment
	StatefulSet *appsv1.StatefulSet
	DaemonSet   *appsv1.DaemonSet
}

// collectRuntimeSurface fetches the live workload via the typed
// clientset. Returns the contract surface plus the typed object so the
// caller can extract ConfigHub labels without a second API call.
func collectRuntimeSurface(ctx context.Context, kind, name, namespace string) (*agent.RuntimeSurface, *runtimeWorkload, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, nil, &agent.CollectionError{Surface: "runtime", Reason: "build kubeconfig: " + err.Error()}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, &agent.CollectionError{Surface: "runtime", Reason: "create clientset: " + err.Error()}
	}
	return collectRuntimeSurfaceWithClient(ctx, cs, kind, name, namespace)
}

func collectRuntimeSurfaceWithTraceSession(ctx context.Context, session *traceSession, kind, name, namespace string) (*agent.RuntimeSurface, *runtimeWorkload, error) {
	if session == nil {
		return nil, nil, &agent.CollectionError{Surface: "runtime", Reason: "selected Kubernetes session is unavailable"}
	}
	client, err := session.kubernetesClient()
	if err != nil {
		return nil, nil, &agent.CollectionError{Surface: "runtime", Reason: "create clientset: " + err.Error()}
	}
	return collectRuntimeSurfaceWithClient(ctx, client, kind, name, namespace)
}

func collectRuntimeSurfaceWithClient(ctx context.Context, cs kubernetes.Interface, kind, name, namespace string) (*agent.RuntimeSurface, *runtimeWorkload, error) {
	rw := &runtimeWorkload{Kind: kind, Namespace: namespace, Name: name}

	switch normalizeKind(kind) {
	case "Deployment":
		d, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fetchError("runtime", err)
		}
		rw.Deployment = d
		rw.Labels = d.Labels
		rw.Annotations = d.Annotations
		rw.Object = sourceTruthObjectFromTyped("Deployment", d.Name, d.Namespace, d.Labels, d.Annotations)
		return &agent.RuntimeSurface{
			Resource: fmt.Sprintf("Deployment/%s in %s", name, namespace),
			Field:    "spec.template.spec.containers[0].image",
			Value:    firstContainerImage(d.Spec.Template.Spec.Containers),
			Health:   workloadHealthLabel(agent.IsDeploymentReady(d)),
		}, rw, nil

	case "StatefulSet":
		s, err := cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fetchError("runtime", err)
		}
		rw.StatefulSet = s
		rw.Labels = s.Labels
		rw.Annotations = s.Annotations
		rw.Object = sourceTruthObjectFromTyped("StatefulSet", s.Name, s.Namespace, s.Labels, s.Annotations)
		return &agent.RuntimeSurface{
			Resource: fmt.Sprintf("StatefulSet/%s in %s", name, namespace),
			Field:    "spec.template.spec.containers[0].image",
			Value:    firstContainerImage(s.Spec.Template.Spec.Containers),
			Health:   workloadHealthLabel(agent.IsStatefulSetReady(s)),
		}, rw, nil

	case "DaemonSet":
		ds, err := cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fetchError("runtime", err)
		}
		rw.DaemonSet = ds
		rw.Labels = ds.Labels
		rw.Annotations = ds.Annotations
		rw.Object = sourceTruthObjectFromTyped("DaemonSet", ds.Name, ds.Namespace, ds.Labels, ds.Annotations)
		return &agent.RuntimeSurface{
			Resource: fmt.Sprintf("DaemonSet/%s in %s", name, namespace),
			Field:    "spec.template.spec.containers[0].image",
			Value:    firstContainerImage(ds.Spec.Template.Spec.Containers),
			Health:   workloadHealthLabel(agent.IsDaemonSetReady(ds)),
		}, rw, nil

	default:
		// Unsupported kinds intentionally surface as a CollectionError
		// rather than a partial Runtime surface. Pilot sees a clear BLOCK
		// rather than a misleadingly-populated runtime block.
		return nil, nil, &agent.CollectionError{
			Surface: "runtime",
			Reason:  fmt.Sprintf("kind %q not supported in v0.1 (supported: Deployment, StatefulSet, DaemonSet)", kind),
		}
	}
}

func sourceTruthObjectFromTyped(kind, name, namespace string, labels, annotations map[string]string) *unstructured.Unstructured {
	stringMap := func(values map[string]string) map[string]interface{} {
		out := make(map[string]interface{}, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": namespace, "labels": stringMap(labels), "annotations": stringMap(annotations)},
	}}
}

// collectConfigHubSurface looks up the ConfigHub unit identified by
// labels/annotations on the runtime object and shells out to `cub unit
// get` to read the latest revision. v0.1 reads only the head revision —
// live/last-applied are deferred to v0.2 alongside the cross-surface
// equality check.
func collectConfigHubSurface(ctx context.Context, rw *runtimeWorkload) (*agent.ConfigHubSurface, error) {
	if rw == nil {
		return nil, &agent.CollectionError{Surface: "confighub", Reason: "no runtime object available"}
	}

	unitSlug := firstNonEmpty(rw.Labels["confighub.com/UnitSlug"], rw.Annotations["confighub.com/UnitSlug"])
	if unitSlug == "" {
		return nil, &agent.CollectionError{
			Surface: "confighub",
			Reason:  "runtime object has no confighub.com/UnitSlug label or annotation; not ConfigHub-managed",
		}
	}
	space := firstNonEmpty(rw.Annotations["confighub.com/SpaceName"], rw.Labels["confighub.com/SpaceName"])
	if space == "" {
		return nil, &agent.CollectionError{
			Surface: "confighub",
			Reason:  "runtime object has no confighub.com/SpaceName annotation",
		}
	}

	unitJSON, err := sourceTruthUnitGet(ctx, unitSlug, space)
	if err != nil {
		// Name the route that failed: under the SDK route no cub ran.
		failed := "cub unit get failed: "
		if failedOnSDKRoute(err) {
			failed = "ConfigHub unit read failed: "
		}
		return nil, &agent.CollectionError{Surface: "confighub", Reason: failed + err.Error()}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(unitJSON, &raw); err != nil {
		return nil, &agent.CollectionError{Surface: "confighub", Reason: "parse cub output: " + err.Error()}
	}

	surface := &agent.ConfigHubSurface{
		Space: space,
		Unit:  unitSlug,
	}

	// `cub unit get` JSON is mixed-case; pull the head revision number
	// using the same tolerant lookup mcp_structured.go uses elsewhere.
	if rev, ok := lookupAnyCase(raw, "HeadRevisionNum"); ok {
		surface.Revision = stringify(rev)
	} else if rev, ok := lookupAnyCase(raw, "Revision"); ok {
		surface.Revision = stringify(rev)
	}

	// URL: best-effort. The IDs may live under different keys; absence
	// is a soft gap (URL is informational, not a contract proof).
	spaceID := stringify(firstAny(raw, "SpaceID", "spaceId"))
	unitID := stringify(firstAny(raw, "UnitID", "unitId"))
	if spaceID != "" && unitID != "" {
		surface.URL = configHubUnitDetailURL(spaceID, unitID)
	}

	return surface, nil
}

// collectControllerSurface fetches the GitOps-controller observation via
// the existing tracers in pkg/agent. Strategy-aware tracer selection is
// load-bearing: under a Flux strategy we never silently fall back to
// Argo (and vice versa), because that would mask the controller-kind
// mismatch the contract is supposed to surface.
func collectControllerSurface(ctx context.Context, strategy agent.SourceTruthStrategy, kind, name, namespace string) *agent.ControllerSurface {
	if strategy.ExpectsArgoController() {
		return controllerSurfaceFromArgo(ctx, kind, name, namespace)
	}
	return controllerSurfaceFromFlux(ctx, kind, name, namespace)
}

func collectControllerSurfaceWithTraceSession(ctx context.Context, session *traceSession, strategy agent.SourceTruthStrategy, kind, name, namespace string, workload *runtimeWorkload) (*agent.ControllerSurface, error) {
	if session == nil {
		return nil, fmt.Errorf("selected Kubernetes session is unavailable")
	}
	if strategy.ExpectsArgoController() {
		return controllerSurfaceFromArgoWithSession(ctx, session, workload)
	}
	return controllerSurfaceFromFluxWithSession(ctx, session, kind, name, namespace)
}

// controllerSurfaceFromArgoWithSession resolves the Application from explicit
// workload tracking metadata, uses a name field selector, and verifies exact
// resource membership. It never consults an Argo CD server context or guesses
// an Application namespace.
func controllerSurfaceFromArgoWithSession(ctx context.Context, session *traceSession, workload *runtimeWorkload) (*agent.ControllerSurface, error) {
	if workload == nil || workload.Object == nil {
		return nil, fmt.Errorf("Argo controller lookup requires the observed runtime object")
	}
	dyn, err := session.dynamicClient()
	if err != nil {
		return nil, err
	}
	names := preferredArgoApplicationNames(workload.Object)
	if len(names) == 0 {
		return nil, fmt.Errorf("runtime metadata does not identify an Argo Application")
	}
	appResource := dyn.Resource(schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"})
	apps := make([]unstructured.Unstructured, 0, len(names))
	for appName := range names {
		list, listErr := appResource.Namespace("").List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", appName).String()})
		if listErr != nil {
			return nil, fmt.Errorf("list Argo Applications named %q from selected cluster: %w", appName, listErr)
		}
		if list == nil {
			return nil, fmt.Errorf("list Argo Applications named %q returned an empty response", appName)
		}
		for i := range list.Items {
			if list.Items[i].GetName() == appName {
				apps = append(apps, list.Items[i])
			}
		}
	}
	appName, appNamespace, ok := selectArgoApplicationForResource(workload.Object, apps)
	if !ok {
		return nil, fmt.Errorf("runtime resource does not identify exactly one matching Argo Application")
	}
	tracer := agent.NewArgoTracerWithKubernetesClient(dyn)
	result, err := tracer.TraceApplicationInNamespace(ctx, appName, appNamespace)
	if err != nil {
		return nil, fmt.Errorf("read Argo Application %s/%s from selected cluster: %w", appNamespace, appName, err)
	}
	if result == nil || len(result.Chain) == 0 {
		return nil, fmt.Errorf("Argo Application %s/%s did not provide a source chain", appNamespace, appName)
	}
	root := result.Chain[0]
	// ChainLink.Revision is spec.source.targetRevision (the desired selector,
	// often a branch such as "main"). Source-truth compares the controller's
	// observed anchor, which Argo exposes as status.sync.revision. Multi-source
	// Applications require per-source observed revisions and remain unanchored.
	observedRevision := ""
	if !result.MultiSource {
		// Use the same exact GET that supplied source and health, not the
		// earlier namespace-discovery LIST, which may contain an older revision.
		for _, link := range result.Chain {
			if link.Kind == "Application" && link.Name == appName && link.Namespace == appNamespace {
				observedRevision = link.Revision
				break
			}
		}
	}
	return &agent.ControllerSurface{Kind: "Argo", Source: strings.TrimSpace(firstNonEmpty(root.URL, root.Kind)), RevisionOrDigest: strings.TrimSpace(observedRevision), Health: controllerHealthLabel(root.Ready, root.Status), MultiSource: result.MultiSource}, nil
}

func controllerSurfaceFromFluxWithSession(ctx context.Context, session *traceSession, kind, name, namespace string) (*agent.ControllerSurface, error) {
	return controllerSurfaceFromFluxWithFactory(ctx, session, kind, name, namespace, capturedTraceFluxFactory)
}

func controllerSurfaceFromFluxWithFactory(ctx context.Context, session *traceSession, kind, name, namespace string, factory func(*traceSession) (agent.Tracer, func() error, error)) (surface *agent.ControllerSurface, returnErr error) {
	tracer, cleanup, err := factory(session)
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				// Do not expose private credential paths or return successful
				// collection when those credentials could not be removed.
				surface = nil
				returnErr = errors.Join(returnErr, fmt.Errorf("unable to remove private Flux credentials"))
			}
		}()
	}
	if err != nil {
		return nil, fmt.Errorf("bind Flux to selected Kubernetes context: %w", err)
	}
	if tracer == nil || !tracer.Available() {
		return nil, fmt.Errorf("Flux CLI is unavailable for the selected Kubernetes context")
	}
	result, err := tracer.Trace(ctx, kind, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("Flux trace failed in selected Kubernetes context: %w", err)
	}
	if result == nil || strings.TrimSpace(result.Error) != "" || len(result.Chain) == 0 {
		if result != nil && strings.TrimSpace(result.Error) != "" {
			return nil, fmt.Errorf("Flux returned no usable controller chain: %s", strings.TrimSpace(result.Error))
		}
		return nil, fmt.Errorf("Flux returned no usable controller chain")
	}
	root := result.Chain[0]
	return &agent.ControllerSurface{Kind: "Flux", Source: strings.TrimSpace(firstNonEmpty(root.URL, root.Kind)), RevisionOrDigest: strings.TrimSpace(root.Revision), Health: controllerHealthLabel(root.Ready, root.Status)}, nil
}

func controllerSurfaceFromArgo(ctx context.Context, kind, name, namespace string) *agent.ControllerSurface {
	tr := agent.NewArgoTracer()
	if !tr.Available() {
		return nil
	}
	res, err := tr.Trace(ctx, kind, name, namespace)
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	root := res.Chain[0]
	return &agent.ControllerSurface{
		Kind:             "Argo",
		Source:           strings.TrimSpace(firstNonEmpty(root.URL, root.Kind)),
		RevisionOrDigest: strings.TrimSpace(root.Revision),
		Health:           controllerHealthLabel(root.Ready, root.Status),
		MultiSource:      res.MultiSource,
	}
}

func controllerSurfaceFromFlux(ctx context.Context, kind, name, namespace string) *agent.ControllerSurface {
	tr := ambientFluxTracer(ctx)
	if !tr.Available() {
		return nil
	}
	res, err := tr.Trace(ctx, kind, name, namespace)
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	root := res.Chain[0]
	return &agent.ControllerSurface{
		Kind:             "Flux",
		Source:           strings.TrimSpace(firstNonEmpty(root.URL, root.Kind)),
		RevisionOrDigest: strings.TrimSpace(root.Revision),
		Health:           controllerHealthLabel(root.Ready, root.Status),
	}
}

// fetchError converts client-go errors into a typed CollectionError with
// a concrete reason. NotFound is most common and worth saying so.
func fetchError(surface string, err error) error {
	if apierrors.IsNotFound(err) {
		return &agent.CollectionError{Surface: surface, Reason: "resource not found"}
	}
	if apierrors.IsForbidden(err) {
		return &agent.CollectionError{Surface: surface, Reason: "forbidden (RBAC): " + err.Error()}
	}
	return &agent.CollectionError{Surface: surface, Reason: err.Error()}
}

// firstContainerImage returns the image string for the first container in
// a workload's pod template, or "" if there are no containers.
func firstContainerImage(containers []corev1.Container) string {
	if len(containers) == 0 {
		return ""
	}
	return containers[0].Image
}

func workloadHealthLabel(ready bool) string {
	if ready {
		return "Current"
	}
	return "NotReady"
}

func controllerHealthLabel(ready bool, status string) string {
	if ready {
		return "Ready"
	}
	if s := strings.TrimSpace(status); s != "" {
		return s
	}
	return "NotReady"
}

func lookupAnyCase(m map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	for _, k := range keys {
		lower := strings.ToLower(k)
		for mk, mv := range m {
			if strings.ToLower(mk) == lower {
				return mv, true
			}
		}
	}
	if outer, ok := m["Unit"].(map[string]interface{}); ok {
		return lookupAnyCase(outer, keys...)
	}
	if outer, ok := m["unit"].(map[string]interface{}); ok {
		return lookupAnyCase(outer, keys...)
	}
	return nil, false
}

func firstAny(m map[string]interface{}, keys ...string) interface{} {
	v, _ := lookupAnyCase(m, keys...)
	return v
}

func stringify(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		// JSON numbers come back as float64; trim trailing zero noise
		// for integer-valued revisions.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
