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
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

var (
	sourceTruthNamespace string
	sourceTruthStrategy  string
	sourceTruthFormat    string
)

var sourceTruthCmd = &cobra.Command{
	Use:   "source-truth <kind>/<name> | <kind> <name>",
	Short: "Read-only source-truth evidence for a single workload (v0.1)",
	Long: `source-truth emits read-only evidence about a single workload's intended,
controller-observed, and runtime state. Output is the structured JSON
contract Pilot's acceptance kernel consumes (#393).

cub-scout is the *evidence provider*; Pilot is the acceptance judge.
This command never mutates, repairs, approves, or infers authority.

Strategy is required input — cub-scout never infers the delivery path.
v0.1 strategies:

  confighub-oci-argo   ConfigHub -> OCI -> Argo CD -> Kubernetes
  confighub-oci-flux   ConfigHub -> OCI -> Flux    -> Kubernetes
  git-argo             Git      -> Argo CD -> Kubernetes
  git-flux             Git      -> Flux    -> Kubernetes

Examples:
  cub-scout compare source-truth deploy/rag-server -n demo --strategy confighub-oci-flux
  cub scout compare source-truth Deployment rag-server -n demo --strategy git-argo

v0.1 supports Deployment, StatefulSet, DaemonSet. Other kinds emit ASK.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSourceTruth,
}

func init() {
	// Lives under `compare` per the council's "extend the existing three-
	// way comparison" framing. Keeps the top-level command surface lean.
	combinedCmd.AddCommand(sourceTruthCmd)
	sourceTruthCmd.Flags().StringVarP(&sourceTruthNamespace, "namespace", "n", "", "Namespace of the resource (required for namespaced kinds)")
	sourceTruthCmd.Flags().StringVar(&sourceTruthStrategy, "strategy", "", "Declared delivery path. Required. One of: confighub-oci-argo, confighub-oci-flux, git-argo, git-flux")
	sourceTruthCmd.Flags().StringVar(&sourceTruthFormat, "format", "json", "Output format. v0.1 supports: json")
}

func runSourceTruth(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(sourceTruthFormat))
	if format != "json" {
		return fmt.Errorf("invalid --format %q (v0.1 supports: json)", sourceTruthFormat)
	}

	// Parse positional arg(s) using the same helper explain uses, so the
	// "kind/name" or "kind name" surface is consistent.
	kind, name, err := parseExplainArgs(args)
	if err != nil {
		return err
	}

	// Strategy validation. Empty/unknown still produces a valid evidence
	// document (ASK + UNKNOWN per the council); the CLI surfaces it as
	// JSON output and exits 0 so Pilot receives the structured "I don't
	// know" rather than a CLI error.
	strategy, ok := agent.ParseStrategy(sourceTruthStrategy)
	if !ok {
		return emitEvidence(agent.Derive("", agent.SourceTruthSurfaces{}))
	}

	// Connected-mode gate: source-truth is meaningless without the
	// ConfigHub surface. Refuse early rather than emit a half-evidence
	// document — the operator should know they are not connected.
	if hub.QuickMode() != hub.Connected {
		return fmt.Errorf("source-truth requires ConfigHub authentication (run `cub auth login` or set CONFIGHUB_API_KEY)")
	}

	ctx := cmd.Context()

	// Fetch the runtime surface first because we need its labels to look
	// up the ConfigHub Unit, and the runtime image is the canonical
	// "what is actually running" value the contract asks for.
	runtime, runtimeObj, runtimeErr := collectRuntimeSurface(ctx, kind, name, sourceTruthNamespace)
	if runtimeErr != nil {
		// Runtime is unfetchable — BLOCK. Pilot must not accept evidence
		// where we cannot read the cluster.
		return emitEvidence(agent.Derive(strategy, agent.SourceTruthSurfaces{
			Controller: nil, // also unfetched in this branch; caller sees
			Runtime:    nil, // paired naming in BLOCK message
		}))
	}

	// ConfigHub surface from the runtime object's labels. If the workload
	// is not labelled as ConfigHub-managed, the surface is absent — that
	// is a hard BLOCK, since the strategy declares ConfigHub authority.
	chSurface, chErr := collectConfigHubSurface(ctx, runtimeObj)
	_ = chErr // surfaced via nil

	// Controller surface from the existing tracers. Tracer selection is
	// strategy-aware: Argo strategies try the Argo tracer; Flux strategies
	// try Flux. We do *not* fall back across controllers — that would
	// hide the strategy-mismatch trap the contract is supposed to catch.
	ctrlSurface := collectControllerSurface(ctx, strategy, kind, name, sourceTruthNamespace)

	ev := agent.Derive(strategy, agent.SourceTruthSurfaces{
		ConfigHub:  chSurface,
		Controller: ctrlSurface,
		Runtime:    runtime,
	})
	return emitEvidence(ev)
}

// emitEvidence prints the JSON contract to stdout. v0.1 is JSON-only.
// Indented for readability — CI / fixture diffs benefit from stable
// indentation.
func emitEvidence(ev agent.SourceTruthEvidence) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(ev)
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

	unitJSON, err := exec.CommandContext(ctx, "cub", "unit", "get", unitSlug, "--space", space, "--json").Output()
	if err != nil {
		return nil, &agent.CollectionError{Surface: "confighub", Reason: "cub unit get failed: " + err.Error()}
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
	}
}

func controllerSurfaceFromFlux(ctx context.Context, kind, name, namespace string) *agent.ControllerSurface {
	tr := agent.NewFluxTracer()
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
