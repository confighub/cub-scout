// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type releaseCheckOptions struct {
	Bundle, Layout, Controller, APIVersion, ControllerNamespace string
	Context, ControllerContext                                  string
	MaxObjects                                                  int
	CheckRunningImage                                           bool
	MaxPods                                                     int
}

// releasePodCap bounds the label-selected pod read the running-image tier makes
// per workload; coverage past it is reported as partial, not a whole-set claim.
const releasePodCap = 200

func (o releaseCheckOptions) validate() (agent.BoundedResourceRef, error) {
	if _, err := agent.ParseReleaseBundleReference(o.Bundle); err != nil {
		return agent.BoundedResourceRef{}, err
	}
	if o.MaxObjects < 1 || o.MaxObjects > agent.ReleaseMaxObjects {
		return agent.BoundedResourceRef{}, fmt.Errorf("max objects must be between 1 and %d", agent.ReleaseMaxObjects)
	}
	if o.MaxPods != 0 && (o.MaxPods < 1 || o.MaxPods > releasePodCap) {
		return agent.BoundedResourceRef{}, fmt.Errorf("max pods must be between 1 and %d", releasePodCap)
	}
	ref, err := boundedExplainRef([]string{o.Controller}, o.APIVersion, o.ControllerNamespace, o.Context)
	if err != nil {
		return ref, err
	}
	if ref.Namespace == "" {
		return ref, fmt.Errorf("--controller-namespace is required")
	}
	return ref, nil
}

func init() {
	release := &cobra.Command{Use: "release", Short: "Inspect an exact configuration release without deploying it"}
	release.AddCommand(newReleaseCheckCommand())
	rootCmd.AddCommand(release)
}

func newReleaseCheckCommand() *cobra.Command {
	var o releaseCheckOptions
	var format, out, failOn string
	var interactive bool
	cmd := &cobra.Command{Use: "check", Short: "Verify an OCI configuration bundle against a controller and live target", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		Long: "Read a digest-pinned literal OCI configuration bundle, exact controller/source evidence and the desired live objects. Reports authored-field agreement and workload-controller convergence, not application success. Running-image identity (the digest actually executing in live pods) is opt-in via --check-running-image, which adds one bounded, selector-scoped pod read per workload. Namespaced bundle objects must include metadata.namespace. No rendering, inventory LIST or mutation.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := o.validate(); err != nil {
				return err
			}
			if format != "ascii" && format != "json" && format != "md" {
				return fmt.Errorf("format must be ascii, json or md")
			}
			gates, err := parseReceiptFailOn(failOn)
			if failOn == "" {
				gates, err = nil, nil
			}
			if err != nil {
				return err
			}
			if interactive {
				if format != "ascii" || out != "" || failOn != "" {
					return fmt.Errorf("--interactive cannot be combined with --format, --out or --fail-on")
				}
				return runReleaseCheckTUI(cmd.Context(), o)
			}
			report, err := observeReleaseCheck(cmd.Context(), o)
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			if len(data) > 4<<20 {
				return fmt.Errorf("release report exceeds 4 MiB; narrow the configuration bundle")
			}
			if out != "" {
				if err := writeReceiptOutFile(out, data); err != nil {
					return err
				}
			}
			if format == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), renderReleaseCheck(report, format))
			}
			if gates[report.Verdict] {
				return newExitCodeError(fmt.Errorf("release check verdict %s matches --fail-on", report.Verdict), 2)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Bundle, "bundle", "", "Exact oci://repository@sha256:digest of literal configuration")
	f.StringVar(&o.Layout, "oci-layout", "", "Read the expected digest from this local OCI layout instead of a registry")
	f.StringVar(&o.Controller, "controller", "", "Exact controller Kind/name")
	f.StringVar(&o.APIVersion, "api-version", "", "Exact controller API version")
	f.StringVar(&o.ControllerNamespace, "controller-namespace", "", "Controller namespace")
	f.StringVar(&o.Context, "kube-context", "", "Explicit target Kubernetes context")
	f.StringVar(&o.ControllerContext, "controller-context", "", "Controller Kubernetes context (defaults to target context)")
	f.IntVar(&o.MaxObjects, "max-objects", agent.ReleaseMaxObjects, "Maximum desired objects (1-100); oversized bundles are rejected before cluster reads")
	f.BoolVar(&o.CheckRunningImage, "check-running-image", false, "Also compare the digest running in live pods against the intended image; adds one bounded, selector-scoped pod read per workload. Mutable tags stay UNKNOWN.")
	f.IntVar(&o.MaxPods, "max-pods", 50, "Maximum pods read per workload for --check-running-image (1-200); coverage past this is reported as partial")
	f.StringVar(&format, "format", "ascii", "Output format: ascii, json, md")
	f.StringVar(&out, "out", "", "Write the complete report as JSON (not an immutable receipt)")
	f.StringVar(&failOn, "fail-on", "", "Exit 2 for WATCH, BLOCK, INCONCLUSIVE, or any-non-pass; preserve report")
	f.BoolVar(&interactive, "interactive", false, "Open the release-check TUI with explicit refresh")
	return cmd
}

func observeReleaseCheck(ctx context.Context, o releaseCheckOptions) (agent.ReleaseCheckReport, error) {
	ref, err := o.validate()
	if err != nil {
		return agent.ReleaseCheckReport{}, err
	}
	if o.ControllerContext == "" {
		o.ControllerContext = o.Context
	}
	r := agent.NewReleaseCheckReport(ref, o.Context, o.ControllerContext, o.MaxObjects, time.Now().UTC())
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// A fresh pair of readers per check pins credentials and prevents old live
	// evidence from being silently reused after refresh or across contexts.
	targetSession, controllerSession := &boundedExplainSession{}, &boundedExplainSession{}
	target, err := targetSession.forContext(o.Context)
	if err != nil {
		return r, err
	}
	controllerReader := target
	if o.ControllerContext != o.Context {
		controllerReader, err = controllerSession.forContext(o.ControllerContext)
		if err != nil {
			return r, err
		}
	}
	bundle, err := agent.LoadReleaseBundle(ctx, o.Bundle, o.Layout, o.MaxObjects)
	r.Bundle = bundle.Evidence
	if err != nil {
		r.Stages[0].Reason = "Bundle unavailable or unverifiable: " + err.Error()
		r.Finish(time.Now().UTC())
		return r, nil
	}
	r.Stages[0] = agent.ReleaseCheckStage{Name: "bundle", Verdict: "PASS", Reason: fmt.Sprintf("Manifest and layer digests verified; %d literal objects.", len(bundle.Objects))}
	controller, ce, controllerErr := controllerReader.Read(ctx, ref, true)
	r.AddRead(ce)
	revision := agent.BuildControllerRevisionEvidence(controller, r.Bundle.Digest, ce.ObservedAt)
	r.ControllerRevision = &revision
	var source *unstructured.Unstructured
	var sourceRef agent.BoundedResourceRef
	if controller != nil && controller.GetAPIVersion() == "kustomize.toolkit.fluxcd.io/v1" && controller.GetKind() == "Kustomization" {
		kind, _, _ := unstructured.NestedString(controller.Object, "spec", "sourceRef", "kind")
		name, _, _ := unstructured.NestedString(controller.Object, "spec", "sourceRef", "name")
		ns, _, _ := unstructured.NestedString(controller.Object, "spec", "sourceRef", "namespace")
		if ns == "" {
			ns = controller.GetNamespace()
		}
		if kind == "OCIRepository" {
			sourceRef = agent.BoundedResourceRef{APIVersion: "source.toolkit.fluxcd.io/v1", Kind: kind, Namespace: ns, Name: name}
			var se agent.BoundedReadEvidence
			source, se, _ = controllerReader.Read(ctx, sourceRef, true)
			r.AddRead(se)
		}
	}
	binding := agent.ReleaseControllerBinding(controller, source, bundle, target.Server(), o.Context == o.ControllerContext)
	if binding == "" {
		binding = agent.ReleaseInventoryCoverage(controller, bundle.Objects)
	}
	switch {
	case controllerErr != nil:
		r.Stages[1].Reason = controllerErr.Error()
	case binding != "":
		r.Stages[1].Reason = binding
	case revision.Comparison == "match":
		r.Stages[1] = agent.ReleaseCheckStage{Name: "controller", Verdict: "PASS", Reason: "Exact OCI repository and bundle revision reported; target and desired inventory are bound. This is reported comparison/apply evidence, not execution history."}
	case revision.Comparison == "mismatch":
		r.Stages[1] = agent.ReleaseCheckStage{Name: "controller", Verdict: "WATCH", Reason: "Controller reports a different bundle revision: " + revision.Revision}
	default:
		r.Stages[1].Reason = revision.Reason
	}
	observed := make([]agent.ObjectSetObservedObject, 0, len(bundle.Objects))
	workloads := []agent.WorkloadConvergedObservedObject{}
	for _, desired := range bundle.Objects {
		id := agent.BoundedResourceRef{APIVersion: desired.GetAPIVersion(), Kind: desired.GetKind(), Namespace: desired.GetNamespace(), Name: desired.GetName()}
		live, read, readErr := target.Read(ctx, id, true)
		r.AddRead(read)
		obs := agent.ObjectSetObservedObject{Desired: desired, Live: live}
		if readErr != nil {
			obs.Error = readErr.Error()
			// Discovery failures do not establish absence of the desired object.
			obs.Inconclusive = read.Reads.Object == 0 || !apierrors.IsNotFound(readErr)
		}
		if live != nil && live.GetUID() == "" {
			obs.Inconclusive = true
			obs.Error = "Live object UID is missing."
		}
		if live != nil {
			if deletion, found, _ := unstructured.NestedFieldNoCopy(live.Object, "metadata", "deletionTimestamp"); found && deletion != nil {
				obs.Inconclusive = true
				obs.Error = "Live object has deletion metadata; stable configuration presence is not established."
			}
		}
		observed = append(observed, obs)
		if agent.ReleaseWorkloadSupported(desired) {
			w := agent.WorkloadConvergedObservedObject{Desired: desired, Live: live, Error: obs.Error, Inconclusive: obs.Inconclusive}
			if live != nil && live.GetAPIVersion() == "apps/v1" {
				gen, found, _ := unstructured.NestedInt64(live.Object, "status", "observedGeneration")
				if live.GetGeneration() <= 0 || !found || gen > live.GetGeneration() {
					w.Inconclusive = true
					w.Error = "Workload generation evidence is missing or invalid."
				}
			}
			workloads = append(workloads, w)
		}
	}
	scope := agent.ObjectSetScope{Kind: "mixed"}
	sourceInfo := agent.ObjectSetSource{Type: "oci", Ref: o.Bundle, Digest: r.Bundle.Digest, ObjectCount: len(bundle.Objects)}
	evidence, err := agent.BuildObjectSetEvidence(sourceInfo, scope, observed)
	if err != nil {
		return r, err
	}
	configReceipt, err := agent.BuildObjectSetReceipt(agent.BuildObjectSetReceiptInput{Evidence: evidence, Verifier: agent.Verifier{Tool: "cub-scout", Version: BuildTag}, VerifiedAt: time.Now().UTC()})
	if err != nil {
		return r, err
	}
	r.Configuration = &configReceipt
	r.Stages[2] = agent.ReleaseCheckStage{Name: "configuration", Verdict: string(configReceipt.Predicate.Verdict), Reason: fmt.Sprintf("%d/%d desired objects match authored fields; %d missing, %d different, %d unreadable/excluded. Extra live objects are not checked.", evidence.Summary.Matched, evidence.Summary.Desired, evidence.Summary.Missing, evidence.Summary.Mismatched, evidence.Summary.Inconclusive)}
	if len(workloads) > 0 {
		we, err := agent.BuildWorkloadsConvergedEvidence(sourceInfo, scope, 0, workloads, time.Now().UTC())
		if err != nil {
			return r, err
		}
		wr, err := agent.BuildWorkloadsConvergedReceipt(agent.BuildWorkloadsConvergedReceiptInput{Evidence: we, Verifier: agent.Verifier{Tool: "cub-scout", Version: BuildTag}, VerifiedAt: time.Now().UTC()})
		if err != nil {
			return r, err
		}
		r.Convergence = &wr
		r.Stages[3] = agent.ReleaseCheckStage{Name: "workloads", Verdict: string(wr.Predicate.Verdict), Reason: fmt.Sprintf("%d/%d supported workload controllers converged; %d progressing, %d failed, %d missing, %d inconclusive. No pod fan-out.", we.Summary.Converged, we.Summary.Desired, we.Summary.Progressing, we.Summary.Failed, we.Summary.Missing, we.Summary.Inconclusive)}
	} else {
		r.Stages[3].Reason = "Bundle contains no supported workloads; configuration agreement does not establish a running application."
	}
	if o.CheckRunningImage {
		assessRunningImage(ctx, &r, target, workloads, o.MaxPods)
	}
	// Detect controller/source changes while gathering the object set. Never
	// combine a previous revision report with a newly changed controller spec.
	for _, check := range []struct {
		ref    agent.BoundedResourceRef
		before *unstructured.Unstructured
	}{{ref, controller}, {sourceRef, source}} {
		if check.before == nil {
			continue
		}
		after, e, err := controllerReader.Read(ctx, check.ref, true)
		r.AddRead(e)
		if err != nil || after.GetUID() != check.before.GetUID() || after.GetResourceVersion() == "" || !reflect.DeepEqual(after.Object, check.before.Object) {
			r.Stages[1] = agent.ReleaseCheckStage{Name: "controller", Verdict: "INCONCLUSIVE", Reason: "Controller/source changed or became unreadable during this check; refresh before drawing a release conclusion."}
		}
	}
	r.Finish(time.Now().UTC())
	return r, nil
}

func renderReleaseCheck(r agent.ReleaseCheckReport, format string) string {
	var b strings.Builder
	if format == "md" {
		b.WriteString("## Configuration Release Check\n\n")
	} else {
		b.WriteString("CONFIGURATION RELEASE CHECK\n\n")
	}
	fmt.Fprintf(&b, "%s: %s\n\nBundle: %s\nTarget context: %q\nController: %s/%s in %q (context %q)\n\n", r.Verdict, r.Headline, r.Bundle.Reference, r.Context, r.Controller.Kind, r.Controller.Name, r.Controller.Namespace, r.ControllerContext)
	for _, stage := range r.Stages {
		fmt.Fprintf(&b, "%s [%s] %s\n", stage.Name, stage.Verdict, stage.Reason)
	}
	if r.ControllerRevision != nil {
		fmt.Fprintf(&b, "Controller report: %s\n", r.ControllerRevision.Summary())
	}
	if r.Configuration != nil && r.Configuration.Predicate.Evidence.ObjectSet != nil {
		for _, obj := range r.Configuration.Predicate.Evidence.ObjectSet.Objects {
			if obj.Status == agent.ObjectSetObjectMatched {
				continue
			}
			fmt.Fprintf(&b, "  %s/%s %s: %s %s\n", obj.ID.Kind, obj.ID.Name, obj.ID.Namespace, obj.Status, obj.Error)
			for _, diff := range obj.Differences {
				fmt.Fprintf(&b, "    authored field differs: %s\n", diff.Path)
			}
		}
	}
	if r.Convergence != nil && r.Convergence.Predicate.Evidence.Workloads != nil {
		for _, workload := range r.Convergence.Predicate.Evidence.Workloads.Workloads {
			if workload.Status == agent.WorkloadConvergedConverged {
				continue
			}
			fmt.Fprintf(&b, "  %s/%s %s: %s; %s %s\n", workload.ID.Kind, workload.ID.Name, workload.ID.Namespace, workload.Status, workload.KstatusMessage, workload.Error)
		}
	}
	if r.RunningImage != nil {
		for _, w := range r.RunningImage.Workloads {
			if w.Verdict == "match" {
				continue
			}
			fmt.Fprintf(&b, "  %s/%s %s: running-image %s (%s)\n", w.ID.Kind, w.ID.Name, w.ID.Namespace, w.Verdict, w.Reason)
			for _, c := range w.Containers {
				if c.Verdict == "match" {
					continue
				}
				line := fmt.Sprintf("    container %s: %s", c.Name, c.Verdict)
				if c.Reason != "" {
					line += " (" + c.Reason + ")"
				}
				if c.IntendedDigest != "" {
					line += "; intended " + shortDigest(c.IntendedDigest)
				}
				if len(c.RunningDigests) > 0 {
					line += "; running " + shortDigests(c.RunningDigests)
				}
				fmt.Fprintln(&b, line)
			}
		}
	}
	fmt.Fprintf(&b, "\nObserved: %s to %s\nKubernetes requests: discovery=%d object=%d; registry requests=%d bytes=%d\n", r.StartedAt.Format(time.RFC3339), r.FinishedAt.Format(time.RFC3339), r.RequestCounts.Discovery, r.RequestCounts.Object, r.Bundle.RegistryRequests, r.Bundle.RegistryBytes)
	if r.RunningImage != nil {
		fmt.Fprintf(&b, "Running-image pods inspected: %d\n", r.RunningImage.PodReads)
	}
	for _, omission := range r.Omissions {
		fmt.Fprintf(&b, "Not proved: %s\n", omission.Reason)
	}
	fmt.Fprintf(&b, "\nNext: %s\n", r.NextStep)
	return strings.Map(func(c rune) rune {
		if c == '\n' || c == '\t' || !unicode.IsControl(c) {
			return c
		}
		return -1
	}, b.String())
}

// assessRunningImage compares the digest running in live pods against the
// intended image for each supported workload. It is opt-in: the reads it makes
// (one bounded, selector-scoped pod list per workload) are additive and folded
// into the report's request counts. Mutable tags and unreadable pods degrade to
// UNKNOWN with a specific reason; nothing is guessed.
func assessRunningImage(ctx context.Context, r *agent.ReleaseCheckReport, reader *agent.BoundedResourceReader, workloads []agent.WorkloadConvergedObservedObject, maxPods int) {
	if maxPods < 1 {
		maxPods = releasePodCap
	}
	ev := &agent.RunningImageEvidence{Workloads: []agent.RunningImageWorkload{}}
	for _, wl := range workloads {
		desired, live := wl.Desired, wl.Live
		if desired == nil || !agent.ReleaseWorkloadSupported(desired) {
			continue
		}
		switch {
		case !agent.IntendedImageDigestPinned(desired):
			// Mutable-tag-only workload: no pod read can confirm identity.
			ev.Workloads = append(ev.Workloads, agent.BuildRunningImageWorkload(desired, nil, false, ""))
		case live == nil:
			ev.Workloads = append(ev.Workloads, agent.BuildRunningImageWorkload(desired, nil, false, "workload-missing"))
		case desired.GetKind() == "Pod":
			// The pod is its own live object; reuse the read already performed.
			ev.Workloads = append(ev.Workloads, agent.BuildRunningImageWorkload(desired, []*unstructured.Unstructured{live}, false, ""))
			ev.PodReads++
		default:
			labels, ok := agent.WorkloadSelectorLabels(live)
			if !ok {
				ev.Workloads = append(ev.Workloads, agent.BuildRunningImageWorkload(desired, nil, false, "selector-unsupported"))
				continue
			}
			pods, capped, e, err := reader.ListPods(ctx, live.GetNamespace(), labels, maxPods)
			r.AddRead(e)
			readErr := ""
			if err != nil {
				readErr = classifyPodReadError(err)
			}
			ev.Workloads = append(ev.Workloads, agent.BuildRunningImageWorkload(desired, pods, capped, readErr))
			ev.PodReads += len(pods)
		}
	}
	if len(ev.Workloads) == 0 {
		r.Stages = append(r.Stages, agent.ReleaseCheckStage{Name: "running-image", Verdict: "NOT_ASSESSED", Reason: "No supported workloads to inspect for running-image identity."})
		return
	}
	verdict, reason := agent.AggregateRunningImage(ev.Workloads)
	ev.Verdict, ev.Reason = verdict, reason
	r.RunningImage = ev
	r.Stages = append(r.Stages, agent.ReleaseCheckStage{Name: "running-image", Verdict: agent.RunningImageStageVerdict(verdict), Reason: runningImageStageReason(ev)})
	for i := range r.Omissions {
		if r.Omissions[i].Missing == "running-artifact-identity" {
			r.Omissions[i].Reason = "Running-image identity was assessed from pod containerStatuses. Multi-architecture index/manifest digests, initContainers, ephemeral containers and matchExpressions-only selectors are not covered in this slice; process-level configuration reload is not inspected."
		}
	}
}

func runningImageStageReason(ev *agent.RunningImageEvidence) string {
	matched := 0
	for _, w := range ev.Workloads {
		if w.Verdict == "match" {
			matched++
		}
	}
	base := fmt.Sprintf("%d/%d workload(s) run the intended image digest across %d pod(s).", matched, len(ev.Workloads), ev.PodReads)
	if ev.Verdict != "match" && ev.Reason != "" {
		base += " " + ev.Reason + "."
	}
	if ev.Verdict == "mismatch" {
		base += " Multi-architecture images can differ in digest form."
	}
	return base
}

func classifyPodReadError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		return "read-denied"
	}
	return err.Error()
}

func shortDigest(d string) string {
	if i := strings.Index(d, ":"); i >= 0 && len(d) > i+13 {
		return d[:i+13] + "..."
	}
	return d
}

func shortDigests(ds []string) string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, shortDigest(d))
	}
	return strings.Join(out, ",")
}
