// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

const blastRadiusMaxNodes = 50

var graphBlastRadiusCmd = &cobra.Command{
	Use:   "blast-radius <kind>/<name>",
	Short: "Compute offline dependency blast radius from a recorded snapshot",
	Long: `Compute an ephemeral, in-memory dependency blast radius for a resource
from a single recorded multi-object YAML/JSON snapshot.

Strictly read-only and offline: never contacts a Kubernetes cluster, never
persists graph state, and never writes to disk.

Traverses:
  - metadata.ownerReferences
  - Flux spec.sourceRef / spec.chartRef / spec.dependsOn
  - Flux kustomize.toolkit.fluxcd.io/* and helm.toolkit.fluxcd.io/* labels
  - Argo CD argocd.argoproj.io/instance and argocd.argoproj.io/tracking-id
  - ConfigMap and Secret volume/envFrom/env/imagePullSecrets references
  - PodDisruptionBudget label selectors

Example:
  cub-scout graph blast-radius ConfigMap/app-cfg -n prod --recording snapshot.yaml --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runGraphBlastRadius,
}

var (
	graphBlastRadiusNamespace string
	graphBlastRadiusRecording string
	graphBlastRadiusFormat    string
)

func init() {
	graphCmd.AddCommand(graphBlastRadiusCmd)

	graphBlastRadiusCmd.Flags().StringVarP(&graphBlastRadiusNamespace, "namespace", "n", "", "Namespace of the target resource")
	graphBlastRadiusCmd.Flags().StringVar(&graphBlastRadiusRecording, "recording", "", "Path to a recorded multi-object YAML/JSON snapshot (required)")
	graphBlastRadiusCmd.Flags().StringVar(&graphBlastRadiusFormat, "format", "ascii", "Output format: ascii|json")
}

// blastRadiusNode is a bounded, compact node entry in ancestors or descendants.
type blastRadiusNode struct {
	Ref  string `json:"ref"`
	Via  string `json:"via"`
	Hops int    `json:"hops"`
}

// blastRadiusOutput is the deterministic, bounded offline blast-radius answer.
type blastRadiusOutput struct {
	Target            string            `json:"target"`
	ObservedAt        string            `json:"observedAt"`
	Ancestors         []blastRadiusNode `json:"ancestors"`
	Descendants       []blastRadiusNode `json:"descendants"`
	AffectedWorkloads []string          `json:"affectedWorkloads"`
	Truncated         bool              `json:"truncated"`
	Answer            *mapsvc.Answer    `json:"answer"`
}

func runGraphBlastRadius(cmd *cobra.Command, args []string) error {
	kind, name, err := parseBlastRadiusTarget(args[0])
	if err != nil {
		return err
	}
	format := strings.ToLower(strings.TrimSpace(graphBlastRadiusFormat))
	if format == "" {
		format = "ascii"
	}
	if format != "ascii" && format != "json" {
		return fmt.Errorf("unsupported --format %q (expected ascii or json)", graphBlastRadiusFormat)
	}
	if strings.TrimSpace(graphBlastRadiusRecording) == "" {
		return fmt.Errorf("--recording is required for offline graph blast-radius")
	}

	snapshot, err := readRecordedObjectSnapshot(graphBlastRadiusRecording)
	if err != nil {
		return err
	}

	out := computeBlastRadius(snapshot, kind, name, strings.TrimSpace(graphBlastRadiusNamespace))
	return writeBlastRadiusOutput(cmd.OutOrStdout(), out, format)
}

func parseBlastRadiusTarget(raw string) (kind, name string, err error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid target %q: expected <kind>/<name>", raw)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func writeBlastRadiusOutput(w io.Writer, out blastRadiusOutput, format string) error {
	if w == nil {
		w = os.Stdout
	}
	if format == "json" {
		raw, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("marshal blast-radius json: %w", err)
		}
		_, err = fmt.Fprintln(w, string(raw))
		return err
	}
	_, err := fmt.Fprint(w, renderBlastRadiusASCII(out))
	return err
}

func renderBlastRadiusASCII(out blastRadiusOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Blast Radius: %s\n", out.Target)
	fmt.Fprintf(&b, "Observed At:  %s\n", out.ObservedAt)
	if out.Answer != nil {
		fmt.Fprintf(&b, "State:        %s (%s)\n", out.Answer.State, out.Answer.Reason)
	}
	fmt.Fprintf(&b, "Ancestors (%d):\n", len(out.Ancestors))
	if len(out.Ancestors) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, n := range out.Ancestors {
			fmt.Fprintf(&b, "  - %s [via=%s, hops=%d]\n", n.Ref, n.Via, n.Hops)
		}
	}
	fmt.Fprintf(&b, "Descendants (%d):\n", len(out.Descendants))
	if len(out.Descendants) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, n := range out.Descendants {
			fmt.Fprintf(&b, "  - %s [via=%s, hops=%d]\n", n.Ref, n.Via, n.Hops)
		}
	}
	fmt.Fprintf(&b, "Affected Workloads (%d):\n", len(out.AffectedWorkloads))
	if len(out.AffectedWorkloads) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, wl := range out.AffectedWorkloads {
			fmt.Fprintf(&b, "  - %s\n", wl)
		}
	}
	if out.Truncated {
		fmt.Fprintf(&b, "Truncated:    true (capped at %d nodes)\n", blastRadiusMaxNodes)
	}
	return b.String()
}

type blastEdge struct {
	to  string
	via string
}

type ephemeralBlastGraph struct {
	objects  map[string]*unstructured.Unstructured
	forward  map[string][]blastEdge // upstream -> downstream
	backward map[string][]blastEdge // downstream -> upstream
	edgeSeen map[string]struct{}
}

func newEphemeralBlastGraph(objects []*unstructured.Unstructured) *ephemeralBlastGraph {
	g := &ephemeralBlastGraph{
		objects:  make(map[string]*unstructured.Unstructured, len(objects)),
		forward:  make(map[string][]blastEdge),
		backward: make(map[string][]blastEdge),
		edgeSeen: make(map[string]struct{}),
	}
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		ref := blastObjectRef(obj.GetKind(), obj.GetNamespace(), obj.GetName())
		if _, exists := g.objects[ref]; !exists {
			g.objects[ref] = obj
		}
	}

	// Build deterministic index of Argo CD Applications by name for label/annotation lookup.
	argoAppsByName := make(map[string][]string)
	pdbsByNamespace := make(map[string][]*unstructured.Unstructured)

	keys := make([]string, 0, len(g.objects))
	for k := range g.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, ref := range keys {
		obj := g.objects[ref]
		if strings.EqualFold(obj.GetKind(), "Application") {
			argoAppsByName[obj.GetName()] = append(argoAppsByName[obj.GetName()], ref)
		}
		if strings.EqualFold(obj.GetKind(), "PodDisruptionBudget") {
			pdbsByNamespace[obj.GetNamespace()] = append(pdbsByNamespace[obj.GetNamespace()], obj)
		}
	}

	for _, ref := range keys {
		obj := g.objects[ref]
		ns := obj.GetNamespace()
		kind := obj.GetKind()

		// 1. ownerReferences: owner -> owned
		for _, or := range obj.GetOwnerReferences() {
			if or.Kind == "" || or.Name == "" {
				continue
			}
			if parentRef, ok := g.resolveRef(or.Kind, ns, or.Name); ok {
				g.addEdge(parentRef, ref, "owner-ref")
			}
		}

		// 2. Flux spec.sourceRef / spec.chartRef / spec.dependsOn
		if strings.EqualFold(kind, "Kustomization") {
			srcKind, srcName, srcNS := extractFluxSourceRef(obj, "Kustomization", ns)
			if srcKind != "" && srcName != "" {
				if srcRef, ok := g.resolveRef(srcKind, srcNS, srcName); ok {
					g.addEdge(srcRef, ref, "source-ref")
				}
			}
			g.addFluxDependsOnEdges(obj, ref, "Kustomization")
		} else if strings.EqualFold(kind, "HelmRelease") {
			srcKind, srcName, srcNS := extractFluxSourceRef(obj, "HelmRelease", ns)
			if srcKind != "" && srcName != "" {
				if srcRef, ok := g.resolveRef(srcKind, srcNS, srcName); ok {
					g.addEdge(srcRef, ref, "source-ref")
				}
			}
			g.addFluxDependsOnEdges(obj, ref, "HelmRelease")
		}

		// 3. Flux ownership labels: deployer -> managed object
		objLabels := obj.GetLabels()
		if ksName := strings.TrimSpace(objLabels["kustomize.toolkit.fluxcd.io/name"]); ksName != "" {
			ksNS := strings.TrimSpace(objLabels["kustomize.toolkit.fluxcd.io/namespace"])
			if ksNS == "" {
				ksNS = "flux-system"
			}
			if ksRef, ok := g.resolveRef("Kustomization", ksNS, ksName); ok {
				g.addEdge(ksRef, ref, "flux-kustomization")
			}
		}
		if hrName := strings.TrimSpace(objLabels["helm.toolkit.fluxcd.io/name"]); hrName != "" {
			hrNS := strings.TrimSpace(objLabels["helm.toolkit.fluxcd.io/namespace"])
			if hrNS == "" {
				hrNS = "flux-system"
			}
			if hrRef, ok := g.resolveRef("HelmRelease", hrNS, hrName); ok {
				g.addEdge(hrRef, ref, "flux-helmrelease")
			}
		}

		// 4. Argo CD instance label / tracking-id annotation: Application -> managed object
		if appName := strings.TrimSpace(objLabels["argocd.argoproj.io/instance"]); appName != "" {
			if appRef, ok := resolveArgoAppRef(argoAppsByName, appName); ok {
				g.addEdge(appRef, ref, "argocd-instance")
			}
		} else if tid := strings.TrimSpace(obj.GetAnnotations()["argocd.argoproj.io/tracking-id"]); tid != "" {
			if appName := parseArgoTrackingIDApplication(tid); appName != "" {
				if appRef, ok := resolveArgoAppRef(argoAppsByName, appName); ok {
					g.addEdge(appRef, ref, "argocd-tracking")
				}
			}
		}

		// 5. ConfigMap / Secret references from workload pod specs: ConfigMap/Secret -> Workload
		if podSpec := extractPodSpecForBlastRadius(obj); podSpec != nil {
			cmNames, secNames := extractPodSpecConfigRefs(podSpec)
			for _, cm := range cmNames {
				if cmRef, ok := g.resolveRef("ConfigMap", ns, cm); ok {
					g.addEdge(cmRef, ref, "configmap-ref")
				}
			}
			for _, sec := range secNames {
				if secRef, ok := g.resolveRef("Secret", ns, sec); ok {
					g.addEdge(secRef, ref, "secret-ref")
				}
			}
		}

		// 6. PodDisruptionBudget label selectors: Workload -> PDB
		if podLabels := extractPodTemplateLabels(obj); len(podLabels) > 0 && ns != "" {
			if pdbs := pdbsByNamespace[ns]; len(pdbs) > 0 {
				labelSet := labels.Set(podLabels)
				for _, pdb := range pdbs {
					if pdbMatchesLabels(pdb, labelSet) {
						pdbRef := blastObjectRef(pdb.GetKind(), pdb.GetNamespace(), pdb.GetName())
						g.addEdge(ref, pdbRef, "pdb-selector")
					}
				}
			}
		}
	}

	// Sort adjacency lists for deterministic traversal.
	for k := range g.forward {
		sortBlastEdges(g.forward[k])
	}
	for k := range g.backward {
		sortBlastEdges(g.backward[k])
	}
	return g
}

func (g *ephemeralBlastGraph) addFluxDependsOnEdges(obj *unstructured.Unstructured, ref, kind string) {
	deps, found, err := unstructured.NestedSlice(obj.Object, "spec", "dependsOn")
	if err != nil || !found {
		return
	}
	for _, raw := range deps {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		depName := strings.TrimSpace(stringValue(m["name"]))
		if depName == "" {
			continue
		}
		depNS := strings.TrimSpace(stringValue(m["namespace"]))
		if depNS == "" {
			depNS = obj.GetNamespace()
		}
		if depRef, ok := g.resolveRef(kind, depNS, depName); ok {
			g.addEdge(depRef, ref, "depends-on")
		}
	}
}

func (g *ephemeralBlastGraph) resolveRef(kind, namespace, name string) (string, bool) {
	if kind == "" || name == "" {
		return "", false
	}
	if namespace != "" {
		exact := blastObjectRef(kind, namespace, name)
		if _, ok := g.objects[exact]; ok {
			return exact, true
		}
	}
	// Fallback to cluster-scoped object with matching kind/name.
	clusterScoped := blastObjectRef(kind, "", name)
	if _, ok := g.objects[clusterScoped]; ok {
		return clusterScoped, true
	}
	// Case-insensitive kind match in exact namespace if present.
	for ref, obj := range g.objects {
		if strings.EqualFold(obj.GetKind(), kind) && obj.GetName() == name && (namespace == "" || obj.GetNamespace() == namespace) {
			return ref, true
		}
	}
	return "", false
}

func resolveArgoAppRef(byName map[string][]string, appName string) (string, bool) {
	refs := byName[appName]
	if len(refs) == 0 {
		return "", false
	}
	if len(refs) == 1 {
		return refs[0], true
	}
	// Prefer argocd namespace if multiple Applications share the same name.
	for _, r := range refs {
		if strings.HasPrefix(r, "Application/argocd/") {
			return r, true
		}
	}
	return refs[0], true
}

func (g *ephemeralBlastGraph) addEdge(from, to, via string) {
	if from == "" || to == "" || from == to {
		return
	}
	key := from + "->" + to + "|" + via
	if _, exists := g.edgeSeen[key]; exists {
		return
	}
	g.edgeSeen[key] = struct{}{}
	g.forward[from] = append(g.forward[from], blastEdge{to: to, via: via})
	g.backward[to] = append(g.backward[to], blastEdge{to: from, via: via})
}

func sortBlastEdges(edges []blastEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].to != edges[j].to {
			return edges[i].to < edges[j].to
		}
		return edges[i].via < edges[j].via
	})
}

func blastObjectRef(kind, namespace, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return kind + "/" + namespace + "/" + name
}

func extractPodSpecForBlastRadius(obj *unstructured.Unstructured) map[string]interface{} {
	if obj == nil {
		return nil
	}
	switch obj.GetKind() {
	case "Pod":
		spec, found, err := unstructured.NestedMap(obj.Object, "spec")
		if err == nil && found {
			return spec
		}
	case "CronJob":
		spec, found, err := unstructured.NestedMap(obj.Object, "spec", "jobTemplate", "spec", "template", "spec")
		if err == nil && found {
			return spec
		}
	default:
		spec, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
		if err == nil && found {
			return spec
		}
	}
	return nil
}

func extractPodSpecConfigRefs(podSpec map[string]interface{}) (configMaps, secrets []string) {
	cmSet := make(map[string]struct{})
	secSet := make(map[string]struct{})
	addCM := func(name string) {
		if s := strings.TrimSpace(name); s != "" {
			cmSet[s] = struct{}{}
		}
	}
	addSec := func(name string) {
		if s := strings.TrimSpace(name); s != "" {
			secSet[s] = struct{}{}
		}
	}

	if vols, ok := podSpec["volumes"].([]interface{}); ok {
		for _, rawVol := range vols {
			vol, ok := rawVol.(map[string]interface{})
			if !ok {
				continue
			}
			if cm, ok := vol["configMap"].(map[string]interface{}); ok {
				addCM(stringValue(cm["name"]))
			}
			if sec, ok := vol["secret"].(map[string]interface{}); ok {
				addSec(stringValue(sec["secretName"]))
			}
			if proj, ok := vol["projected"].(map[string]interface{}); ok {
				if sources, ok := proj["sources"].([]interface{}); ok {
					for _, rawSrc := range sources {
						src, ok := rawSrc.(map[string]interface{})
						if !ok {
							continue
						}
						if cm, ok := src["configMap"].(map[string]interface{}); ok {
							addCM(stringValue(cm["name"]))
						}
						if sec, ok := src["secret"].(map[string]interface{}); ok {
							addSec(stringValue(sec["name"]))
						}
					}
				}
			}
		}
	}

	if pullSecrets, ok := podSpec["imagePullSecrets"].([]interface{}); ok {
		for _, rawPS := range pullSecrets {
			if ps, ok := rawPS.(map[string]interface{}); ok {
				addSec(stringValue(ps["name"]))
			}
		}
	}

	for _, containerKey := range []string{"containers", "initContainers", "ephemeralContainers"} {
		containers, ok := podSpec[containerKey].([]interface{})
		if !ok {
			continue
		}
		for _, rawC := range containers {
			c, ok := rawC.(map[string]interface{})
			if !ok {
				continue
			}
			if envFroms, ok := c["envFrom"].([]interface{}); ok {
				for _, rawEF := range envFroms {
					ef, ok := rawEF.(map[string]interface{})
					if !ok {
						continue
					}
					if cmRef, ok := ef["configMapRef"].(map[string]interface{}); ok {
						addCM(stringValue(cmRef["name"]))
					}
					if secRef, ok := ef["secretRef"].(map[string]interface{}); ok {
						addSec(stringValue(secRef["name"]))
					}
				}
			}
			if envs, ok := c["env"].([]interface{}); ok {
				for _, rawEnv := range envs {
					envMap, ok := rawEnv.(map[string]interface{})
					if !ok {
						continue
					}
					vf, ok := envMap["valueFrom"].(map[string]interface{})
					if !ok {
						continue
					}
					if cmKeyRef, ok := vf["configMapKeyRef"].(map[string]interface{}); ok {
						addCM(stringValue(cmKeyRef["name"]))
					}
					if secKeyRef, ok := vf["secretKeyRef"].(map[string]interface{}); ok {
						addSec(stringValue(secKeyRef["name"]))
					}
				}
			}
		}
	}

	configMaps = make([]string, 0, len(cmSet))
	for k := range cmSet {
		configMaps = append(configMaps, k)
	}
	sort.Strings(configMaps)

	secrets = make([]string, 0, len(secSet))
	for k := range secSet {
		secrets = append(secrets, k)
	}
	sort.Strings(secrets)
	return configMaps, secrets
}

func pdbMatchesLabels(pdb *unstructured.Unstructured, podLabels labels.Set) bool {
	if pdb == nil || len(podLabels) == 0 {
		return false
	}
	rawSel, found, err := unstructured.NestedMap(pdb.Object, "spec", "selector")
	if err != nil || !found || rawSel == nil {
		return false
	}
	var sel metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSel, &sel); err != nil {
		return false
	}
	selector, err := metav1.LabelSelectorAsSelector(&sel)
	if err != nil {
		return false
	}
	return selector.Matches(podLabels)
}

func computeBlastRadius(snapshot recordedObjectSnapshot, kind, name, namespace string) blastRadiusOutput {
	observedAt := strings.TrimSpace(snapshot.ObservedAt)
	if observedAt == "" {
		observedAt = "unknown"
	}
	targetLabel := blastObjectRef(kind, namespace, name)

	targetObj, err := findObjectInRecordedSnapshot(snapshot.Objects, kind, name, namespace)
	if err != nil {
		out := blastRadiusOutput{
			Target:            targetLabel,
			ObservedAt:        observedAt,
			Ancestors:         []blastRadiusNode{},
			Descendants:       []blastRadiusNode{},
			AffectedWorkloads: []string{},
			Truncated:         false,
		}
		out.Answer = buildBlastRadiusAnswer(out, snapshot, false, err.Error())
		return out
	}

	targetRef := blastObjectRef(targetObj.GetKind(), targetObj.GetNamespace(), targetObj.GetName())
	g := newEphemeralBlastGraph(snapshot.Objects)

	ancestors, ancTruncated := traverseBlastDirection(g.backward, targetRef, blastRadiusMaxNodes)
	remaining := blastRadiusMaxNodes - len(ancestors)
	descendants, descTruncated := traverseBlastDirection(g.forward, targetRef, remaining)
	truncated := ancTruncated || descTruncated

	affected := collectAffectedWorkloads(g, targetRef, descendants)

	out := blastRadiusOutput{
		Target:            targetRef,
		ObservedAt:        observedAt,
		Ancestors:         ancestors,
		Descendants:       descendants,
		AffectedWorkloads: affected,
		Truncated:         truncated,
	}
	out.Answer = buildBlastRadiusAnswer(out, snapshot, true, "")
	return out
}

type bfsItem struct {
	ref  string
	hops int
}

func traverseBlastDirection(adj map[string][]blastEdge, start string, maxNodes int) ([]blastRadiusNode, bool) {
	result := []blastRadiusNode{}
	if maxNodes < 0 {
		maxNodes = 0
	}
	visited := map[string]struct{}{start: {}}
	queue := []bfsItem{{ref: start, hops: 0}}
	truncated := false

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, edge := range adj[cur.ref] {
			if _, seen := visited[edge.to]; seen {
				continue
			}
			visited[edge.to] = struct{}{}
			if len(result) >= maxNodes {
				truncated = true
				continue
			}
			result = append(result, blastRadiusNode{
				Ref:  edge.to,
				Via:  edge.via,
				Hops: cur.hops + 1,
			})
			queue = append(queue, bfsItem{ref: edge.to, hops: cur.hops + 1})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Hops != result[j].Hops {
			return result[i].Hops < result[j].Hops
		}
		if result[i].Ref != result[j].Ref {
			return result[i].Ref < result[j].Ref
		}
		return result[i].Via < result[j].Via
	})
	return result, truncated
}

func collectAffectedWorkloads(g *ephemeralBlastGraph, targetRef string, descendants []blastRadiusNode) []string {
	set := make(map[string]struct{})
	check := func(ref string) {
		obj := g.objects[ref]
		if obj == nil {
			return
		}
		if isBlastWorkload(obj) {
			set[ref] = struct{}{}
		}
	}
	check(targetRef)
	for _, d := range descendants {
		check(d.Ref)
	}
	out := make([]string, 0, len(set))
	for ref := range set {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func isBlastWorkload(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	switch obj.GetKind() {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Rollout":
		return true
	case "Pod":
		return len(obj.GetOwnerReferences()) == 0
	default:
		return false
	}
}

func buildBlastRadiusAnswer(out blastRadiusOutput, snapshot recordedObjectSnapshot, found bool, lookupErr string) *mapsvc.Answer {
	ans := &mapsvc.Answer{
		ObservedAt: out.ObservedAt,
		Cluster: mapsvc.AnswerCluster{
			Context:  snapshot.SourceContext,
			IDSource: "not-read",
		},
		Basis:    []mapsvc.AnswerBasis{},
		Unknowns: []mapsvc.AnswerUnknown{},
		Next:     []mapsvc.AnswerNext{},
		Cost:     mapsvc.AnswerCost{Coverage: "offline-recording"},
	}

	if !found {
		ans.State = explainStateNotObservable
		ans.Reason = explainReasonObjectNotRead
		ans.Summary = fmt.Sprintf("%s was not found in the recorded snapshot.", out.Target)
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "target resource",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "not present in recorded snapshot",
			Quoted: lookupErr,
		})
		ans.Unknowns = append(ans.Unknowns, offlineRecordingUnknown(snapshot))
		return ans
	}

	ans.State = explainStateExplained
	ans.Reason = explainReasonChainRead
	ans.Summary = fmt.Sprintf("%s has %d ancestor(s), %d descendant(s), %d workload(s).", out.Target, len(out.Ancestors), len(out.Descendants), len(out.AffectedWorkloads))
	ans.Basis = append(ans.Basis, mapsvc.AnswerBasis{
		Claim:   "ephemeral in-memory graph from recorded snapshot",
		RestsOn: mapsvc.RestsOnObjectMetadata,
	})
	if out.Truncated {
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "additional reachable nodes",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: fmt.Sprintf("truncated at %d nodes", blastRadiusMaxNodes),
		})
	}
	ans.Unknowns = append(ans.Unknowns, offlineRecordingUnknown(snapshot))
	return ans
}
