// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

type compareSideSummary struct {
	Source          string   `json:"source"`
	APIVersion      string   `json:"apiVersion,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Name            string   `json:"name,omitempty"`
	Namespace       string   `json:"namespace,omitempty"`
	UnitSlug        string   `json:"unitSlug,omitempty"`
	UnitID          string   `json:"unitId,omitempty"`
	SpaceName       string   `json:"spaceName,omitempty"`
	SpaceID         string   `json:"spaceId,omitempty"`
	HeadRevisionNum int      `json:"headRevisionNum,omitempty"`
	LiveRevisionNum int      `json:"liveRevisionNum,omitempty"`
	LastAppliedRev  int      `json:"lastAppliedRevisionNum,omitempty"`
	Generation      int64    `json:"generation,omitempty"`
	ResourceVersion string   `json:"resourceVersion,omitempty"`
	Replicas        *int64   `json:"replicas,omitempty"`
	Images          []string `json:"images,omitempty"`
	LabelCount      int      `json:"labelCount,omitempty"`
	AnnotationCount int      `json:"annotationCount,omitempty"`
}

type compareResourceResult struct {
	Resource   string                 `json:"resource"`
	Namespace  string                 `json:"namespace,omitempty"`
	Mode       string                 `json:"mode"`
	Connected  bool                   `json:"connected"`
	Dry        *compareSideSummary    `json:"dry,omitempty"`
	Wet        *compareSideSummary    `json:"wet,omitempty"`
	Live       compareSideSummary     `json:"live"`
	Mismatches []compareFieldMismatch `json:"mismatches,omitempty"`
	Notes      []string               `json:"notes,omitempty"`
}

type compareFieldMismatch struct {
	Field string `json:"field"`
	Dry   string `json:"dry"`
	Wet   string `json:"wet"`
	Live  string `json:"live"`
}

type compareResourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

type compareDryWetResult struct {
	Dry   *compareSideSummary
	Wet   *compareSideSummary
	Notes []string
}

type compareConfigHubLink struct {
	UnitSlug  string
	SpaceName string
	SpaceID   string
}

type compareUnitMetadata struct {
	UnitSlug            string
	UnitID              string
	SpaceName           string
	SpaceID             string
	HeadRevisionNum     int
	LiveRevisionNum     int
	LastAppliedRevision int
}

var (
	loadCompareLiveSnapshotFn     = loadCompareLiveSnapshot
	loadCompareDryWetSnapshotFn   = loadCompareDryWetSnapshots
	resolveCompareConfigHubLinkFn = resolveCompareConfigHubLink
	compareConnectedFn            = isCompareConnected
	compareDefaultSpaceFn         = detectCompareSpace
	runCompareCubCommand          = runCompareCubCommandImpl
)

func runCombinedResourceCompare(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("resource compare mode expects exactly one resource argument (kind/name)")
	}
	if err := validateCombinedResourceCompareFlags(); err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(combinedFormat))
	if format == "" {
		format = "ascii"
	}
	if combinedJSON {
		format = "json"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", combinedFormat)
	}

	result, err := buildCompareResourceResult(cmd.Context(), args[0], strings.TrimSpace(combinedNamespace))
	if err != nil {
		return err
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "md":
		fmt.Print(renderCompareResourceMarkdown(result))
		return nil
	default:
		fmt.Print(renderCompareResourceASCII(result))
		return nil
	}
}

func validateCombinedResourceCompareFlags() error {
	if strings.TrimSpace(combinedGitURL) != "" ||
		strings.TrimSpace(combinedGitPath) != "" ||
		strings.TrimSpace(combinedGitURLCompare) != "" ||
		strings.TrimSpace(combinedGitPathCompare) != "" ||
		strings.TrimSpace(combinedBundle) != "" ||
		combinedSuggest ||
		combinedApply ||
		combinedDryRun {
		return fmt.Errorf("resource compare mode cannot be combined with --git-* / --bundle / --suggest / --apply / --dry-run")
	}
	return nil
}

func buildCompareResourceResult(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
	kindRaw, name, err := parseResourceArg(resourceArg)
	if err != nil {
		return compareResourceResult{}, err
	}

	kind := normalizeKind(kindRaw)
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}

	live, err := loadCompareLiveSnapshotFn(ctx, kind, name, ns)
	if err != nil {
		return compareResourceResult{}, fmt.Errorf("load LIVE snapshot for %s/%s in namespace %s: %w", kind, name, ns, err)
	}
	if strings.TrimSpace(live.Source) == "" {
		live.Source = "cluster"
	}

	connected := compareConnectedFn()
	notes := make([]string, 0, 2)
	mode := "live-only"
	if connected {
		unitSlug := strings.TrimSpace(live.UnitSlug)
		if unitSlug == "" {
			notes = append(notes, "Connected mode detected, but this LIVE resource is not linked to a ConfigHub unit (`confighub.com/UnitSlug` missing).")
		} else {
			space := strings.TrimSpace(live.SpaceName)
			if space == "" {
				space = compareDefaultSpaceFn()
			}
			dryWet, err := loadCompareDryWetSnapshotFn(ctx, unitSlug, space, compareResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			})
			if err != nil {
				notes = append(notes, fmt.Sprintf("Connected DRY/WET lookup failed for unit %s: %v", unitSlug, err))
			} else {
				if dryWet.Dry != nil || dryWet.Wet != nil {
					mode = "dry-wet-live"
				}
				notes = append(notes, dryWet.Notes...)
				if dryWet.Dry == nil {
					notes = append(notes, "DRY snapshot unavailable for linked unit.")
				}
				if dryWet.Wet == nil {
					notes = append(notes, "WET snapshot unavailable for linked unit.")
				}
				return finalizeCompareResourceResult(compareResourceResult{
					Resource:  kind + "/" + name,
					Namespace: ns,
					Mode:      mode,
					Connected: connected,
					Dry:       dryWet.Dry,
					Wet:       dryWet.Wet,
					Live:      live,
					Notes:     notes,
				}), nil
			}
		}
	} else {
		notes = append(notes, "Connect to ConfigHub to unlock DRY/WET/LIVE expected-state comparison.")
	}

	return finalizeCompareResourceResult(compareResourceResult{
		Resource:  kind + "/" + name,
		Namespace: ns,
		Mode:      mode,
		Connected: connected,
		Live:      live,
		Notes:     notes,
	}), nil
}

func isCompareConnected() bool {
	return hub.NewClient().RequireConnected() == nil
}

func detectCompareSpace() string {
	cubCtx, _, err := getStatusCubContext()
	if err != nil || cubCtx == nil {
		return ""
	}
	return strings.TrimSpace(cubCtx.Settings.DefaultSpace)
}

var errCompareResourceNotFoundInManifest = errors.New("resource not found in manifest")

func loadCompareDryWetSnapshots(ctx context.Context, unitSlug, space string, target compareResourceRef) (compareDryWetResult, error) {
	unitSlug = strings.TrimSpace(unitSlug)
	if unitSlug == "" {
		return compareDryWetResult{}, fmt.Errorf("missing unit slug")
	}

	dryRaw, err := runCompareCubCommand(ctx, compareUnitGetArgs(unitSlug, space))
	if err != nil {
		return compareDryWetResult{}, fmt.Errorf("cub unit get: %w", err)
	}
	unitMeta, _ := decodeCompareUnitMetadataFromGetJSON(dryRaw)

	dryYAML, err := decodeCompareUnitDataFromGetJSON(dryRaw)
	if err != nil {
		return compareDryWetResult{}, fmt.Errorf("decode unit get data: %w", err)
	}
	drySummary, dryErr := extractCompareSummaryFromManifestYAML("dry", dryYAML, target)
	if dryErr != nil && !errors.Is(dryErr, errCompareResourceNotFoundInManifest) {
		return compareDryWetResult{}, fmt.Errorf("extract DRY snapshot: %w", dryErr)
	}

	wetRaw, err := runCompareCubCommand(ctx, compareUnitLivedataArgs(unitSlug, space))
	if err != nil {
		return compareDryWetResult{
			Dry:   drySummary,
			Notes: []string{fmt.Sprintf("WET lookup failed for unit %s: %v", unitSlug, err)},
		}, nil
	}
	wetSummary, wetErr := extractCompareSummaryFromManifestYAML("wet", wetRaw, target)
	if wetErr != nil && !errors.Is(wetErr, errCompareResourceNotFoundInManifest) {
		return compareDryWetResult{}, fmt.Errorf("extract WET snapshot: %w", wetErr)
	}

	notes := make([]string, 0, 3)
	if errors.Is(dryErr, errCompareResourceNotFoundInManifest) {
		notes = append(notes, "DRY manifest found but target resource was not present in unit intent data.")
	}
	if errors.Is(wetErr, errCompareResourceNotFoundInManifest) {
		notes = append(notes, "WET manifest found but target resource was not present in rendered output.")
	}
	if drySummary != nil && drySummary.SpaceName == "" {
		drySummary.SpaceName = space
	}
	if wetSummary != nil && wetSummary.SpaceName == "" {
		wetSummary.SpaceName = space
	}
	applyCompareUnitMetadata(drySummary, unitMeta)
	applyCompareUnitMetadata(wetSummary, unitMeta)
	return compareDryWetResult{
		Dry:   drySummary,
		Wet:   wetSummary,
		Notes: notes,
	}, nil
}

func compareUnitGetArgs(unitSlug, space string) []string {
	args := []string{"unit", "get", unitSlug, "--json", "--quiet"}
	if strings.TrimSpace(space) != "" {
		args = append(args, "--space", strings.TrimSpace(space))
	}
	return args
}

func compareUnitLivedataArgs(unitSlug, space string) []string {
	args := []string{"unit", "livedata", unitSlug}
	if strings.TrimSpace(space) != "" {
		args = append(args, "--space", strings.TrimSpace(space))
	}
	return args
}

func runCompareCubCommandImpl(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "cub", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("cub %s failed: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func decodeCompareUnitDataFromGetJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty unit get output")
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("parse unit get json: %w", err)
	}

	base64Value := findCompareUnitDataBase64(payload)
	if base64Value == "" {
		return "", fmt.Errorf("unit get output missing Unit.Data")
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(base64Value)
		if err != nil {
			return "", fmt.Errorf("decode Unit.Data base64: %w", err)
		}
	}
	return strings.TrimSpace(string(decoded)), nil
}

func decodeCompareUnitMetadataFromGetJSON(raw string) (compareUnitMetadata, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return compareUnitMetadata{}, fmt.Errorf("empty unit get output")
	}

	var payload struct {
		Space struct {
			Slug    string `json:"Slug"`
			SpaceID string `json:"SpaceID"`
		} `json:"Space"`
		Unit struct {
			Slug                   string `json:"Slug"`
			UnitID                 string `json:"UnitID"`
			SpaceID                string `json:"SpaceID"`
			HeadRevisionNum        int    `json:"HeadRevisionNum"`
			LiveRevisionNum        int    `json:"LiveRevisionNum"`
			LastAppliedRevisionNum int    `json:"LastAppliedRevisionNum"`
		} `json:"Unit"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return compareUnitMetadata{}, fmt.Errorf("parse unit get json: %w", err)
	}

	spaceID := strings.TrimSpace(payload.Space.SpaceID)
	if spaceID == "" {
		spaceID = strings.TrimSpace(payload.Unit.SpaceID)
	}

	return compareUnitMetadata{
		UnitSlug:            strings.TrimSpace(payload.Unit.Slug),
		UnitID:              strings.TrimSpace(payload.Unit.UnitID),
		SpaceName:           strings.TrimSpace(payload.Space.Slug),
		SpaceID:             spaceID,
		HeadRevisionNum:     payload.Unit.HeadRevisionNum,
		LiveRevisionNum:     payload.Unit.LiveRevisionNum,
		LastAppliedRevision: payload.Unit.LastAppliedRevisionNum,
	}, nil
}

func applyCompareUnitMetadata(summary *compareSideSummary, meta compareUnitMetadata) {
	if summary == nil {
		return
	}
	if summary.UnitSlug == "" {
		summary.UnitSlug = meta.UnitSlug
	}
	if summary.UnitID == "" {
		summary.UnitID = meta.UnitID
	}
	if summary.SpaceName == "" {
		summary.SpaceName = meta.SpaceName
	}
	if summary.SpaceID == "" {
		summary.SpaceID = meta.SpaceID
	}
	if summary.HeadRevisionNum == 0 && meta.HeadRevisionNum > 0 {
		summary.HeadRevisionNum = meta.HeadRevisionNum
	}
	if summary.LiveRevisionNum == 0 && meta.LiveRevisionNum > 0 {
		summary.LiveRevisionNum = meta.LiveRevisionNum
	}
	if summary.LastAppliedRev == 0 && meta.LastAppliedRevision > 0 {
		summary.LastAppliedRev = meta.LastAppliedRevision
	}
}

func findCompareUnitDataBase64(payload interface{}) string {
	switch typed := payload.(type) {
	case map[string]interface{}:
		for _, key := range []string{"Unit", "unit"} {
			if nested, ok := typed[key].(map[string]interface{}); ok {
				if value, ok := nested["Data"].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
				if value, ok := nested["data"].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
		for _, key := range []string{"Data", "data"} {
			if value, ok := typed[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	case []interface{}:
		for _, item := range typed {
			if value := findCompareUnitDataBase64(item); value != "" {
				return value
			}
		}
	}
	return ""
}

func extractCompareSummaryFromManifestYAML(source string, manifestYAML string, target compareResourceRef) (*compareSideSummary, error) {
	docs := strings.Split(manifestYAML, "\n---")
	for _, doc := range docs {
		rawDoc := strings.TrimSpace(doc)
		if rawDoc == "" {
			continue
		}
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(rawDoc), &obj); err != nil {
			return nil, fmt.Errorf("parse manifest YAML: %w", err)
		}
		summary := summarizeCompareManifestObject(source, obj)
		if !matchesCompareSummaryTarget(summary, target) {
			continue
		}
		return summary, nil
	}
	return nil, errCompareResourceNotFoundInManifest
}

func summarizeCompareManifestObject(source string, obj map[string]interface{}) *compareSideSummary {
	summary := &compareSideSummary{
		Source: source,
		Images: extractCompareImages(obj),
	}

	if apiVersion, ok := obj["apiVersion"].(string); ok {
		summary.APIVersion = strings.TrimSpace(apiVersion)
	}
	if kind, ok := obj["kind"].(string); ok {
		summary.Kind = strings.TrimSpace(kind)
	}
	if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			summary.Name = strings.TrimSpace(name)
		}
		if namespace, ok := metadata["namespace"].(string); ok {
			summary.Namespace = strings.TrimSpace(namespace)
		}
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			summary.LabelCount = len(labels)
		}
		if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
			summary.AnnotationCount = len(annotations)
		}
	}
	if replicas, ok := readCompareReplicas(obj); ok {
		summary.Replicas = &replicas
	}
	return summary
}

func readCompareReplicas(obj map[string]interface{}) (int64, bool) {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	raw, ok := spec["replicas"]
	if !ok || raw == nil {
		return 0, false
	}
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func matchesCompareSummaryTarget(summary *compareSideSummary, target compareResourceRef) bool {
	if summary == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(summary.Kind), strings.TrimSpace(target.Kind)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(summary.Name), strings.TrimSpace(target.Name)) {
		return false
	}
	docNamespace := strings.TrimSpace(summary.Namespace)
	targetNamespace := strings.TrimSpace(target.Namespace)
	return docNamespace == "" || strings.EqualFold(docNamespace, targetNamespace)
}

func loadCompareLiveSnapshot(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
	cfg, err := buildConfig()
	if err != nil {
		return compareSideSummary{}, fmt.Errorf("build kubernetes config: %w", err)
	}

	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return compareSideSummary{}, fmt.Errorf("unsupported resource kind %q for compare mode", kind)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return compareSideSummary{}, fmt.Errorf("build dynamic client: %w", err)
	}

	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return compareSideSummary{}, err
	}

	summary := summarizeCompareLiveObject(obj)
	if summary.UnitSlug != "" {
		return summary, nil
	}

	link, err := resolveCompareConfigHubLinkFn(ctx, dynClient, obj)
	if err != nil {
		return summary, nil
	}
	if summary.UnitSlug == "" {
		summary.UnitSlug = strings.TrimSpace(link.UnitSlug)
	}
	if summary.SpaceName == "" {
		summary.SpaceName = strings.TrimSpace(link.SpaceName)
	}
	if summary.SpaceID == "" {
		summary.SpaceID = strings.TrimSpace(link.SpaceID)
	}

	return summary, nil
}

func summarizeCompareLiveObject(obj *unstructured.Unstructured) compareSideSummary {
	labels := obj.GetLabels()
	annotations := obj.GetAnnotations()
	unitSlug := strings.TrimSpace(labels["confighub.com/UnitSlug"])
	if unitSlug == "" {
		unitSlug = strings.TrimSpace(annotations["confighub.com/UnitSlug"])
	}
	unitID := strings.TrimSpace(annotations["confighub.com/UnitID"])
	if unitID == "" {
		unitID = strings.TrimSpace(labels["confighub.com/UnitID"])
	}
	spaceName := strings.TrimSpace(annotations["confighub.com/SpaceName"])
	if spaceName == "" {
		spaceName = strings.TrimSpace(labels["confighub.com/SpaceName"])
	}
	spaceID := strings.TrimSpace(annotations["confighub.com/SpaceID"])
	if spaceID == "" {
		spaceID = strings.TrimSpace(labels["confighub.com/SpaceID"])
	}

	out := compareSideSummary{
		Source:          "cluster",
		APIVersion:      obj.GetAPIVersion(),
		Kind:            obj.GetKind(),
		Name:            obj.GetName(),
		Namespace:       obj.GetNamespace(),
		UnitSlug:        unitSlug,
		UnitID:          unitID,
		SpaceName:       spaceName,
		SpaceID:         spaceID,
		Generation:      obj.GetGeneration(),
		ResourceVersion: obj.GetResourceVersion(),
		LabelCount:      len(labels),
		AnnotationCount: len(annotations),
		Images:          extractCompareImages(obj.Object),
	}
	if replicas, ok, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); ok {
		out.Replicas = &replicas
	}
	return out
}

func resolveCompareConfigHubLink(ctx context.Context, dynClient dynamic.Interface, obj *unstructured.Unstructured) (compareConfigHubLink, error) {
	if dynClient == nil || obj == nil {
		return compareConfigHubLink{}, nil
	}

	appName := strings.TrimSpace(argoApplicationNameFromResource(obj))
	if appName == "" {
		return compareConfigHubLink{}, nil
	}

	appGVR := kindToGVR("Application")
	if appGVR.Resource == "" {
		return compareConfigHubLink{}, nil
	}

	appObj, err := dynClient.Resource(appGVR).Namespace("argocd").Get(ctx, appName, v1.GetOptions{})
	if err != nil {
		return compareConfigHubLink{}, err
	}

	annotations := appObj.GetAnnotations()
	labels := appObj.GetLabels()
	link := compareConfigHubLink{
		UnitSlug: strings.TrimSpace(annotations["confighub.com/UnitSlug"]),
		SpaceID:  strings.TrimSpace(annotations["confighub.com/SpaceID"]),
	}
	if link.UnitSlug == "" {
		link.UnitSlug = strings.TrimSpace(labels["confighub.com/UnitSlug"])
	}
	if link.SpaceID == "" {
		link.SpaceID = strings.TrimSpace(labels["confighub.com/SpaceID"])
	}

	repoURL, _, _ := unstructured.NestedString(appObj.Object, "spec", "source", "repoURL")
	repoSpace, repoUnit := parseCompareConfigHubUnitRepoURL(repoURL)
	if link.UnitSlug == "" {
		link.UnitSlug = repoUnit
	}
	link.SpaceName = repoSpace

	return link, nil
}

func parseCompareConfigHubUnitRepoURL(raw string) (spaceName, unitSlug string) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "oci://") {
		return "", ""
	}

	remainder := strings.TrimPrefix(raw, "oci://")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}

	repository := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(repository, "unit/") {
		return "", ""
	}

	repository = strings.TrimPrefix(repository, "unit/")
	repoParts := strings.SplitN(repository, "/", 2)
	if len(repoParts) != 2 {
		return "", ""
	}

	return strings.TrimSpace(repoParts[0]), strings.TrimSpace(repoParts[1])
}

func extractCompareImages(obj map[string]interface{}) []string {
	containers, ok, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", "containers")
	if !ok || len(containers) == 0 {
		return nil
	}

	images := make([]string, 0, len(containers))
	for _, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		raw, ok := container["image"].(string)
		if !ok {
			continue
		}
		image := strings.TrimSpace(raw)
		if image == "" {
			continue
		}
		images = append(images, image)
	}
	sort.Strings(images)
	return images
}

func finalizeCompareResourceResult(result compareResourceResult) compareResourceResult {
	result.Mismatches = detectCompareFieldMismatches(result.Dry, result.Wet, result.Live)
	return result
}

func detectCompareFieldMismatches(dry, wet *compareSideSummary, live compareSideSummary) []compareFieldMismatch {
	type compareFieldDescriptor struct {
		Name    string
		Extract func(*compareSideSummary) string
	}
	fields := []compareFieldDescriptor{
		{
			Name: "apiVersion",
			Extract: func(side *compareSideSummary) string {
				if side == nil {
					return ""
				}
				return strings.TrimSpace(side.APIVersion)
			},
		},
		{
			Name: "kind",
			Extract: func(side *compareSideSummary) string {
				if side == nil {
					return ""
				}
				return strings.TrimSpace(side.Kind)
			},
		},
		{
			Name: "namespace",
			Extract: func(side *compareSideSummary) string {
				if side == nil {
					return ""
				}
				return strings.TrimSpace(side.Namespace)
			},
		},
		{
			Name: "replicas",
			Extract: func(side *compareSideSummary) string {
				if side == nil || side.Replicas == nil {
					return ""
				}
				return fmt.Sprintf("%d", *side.Replicas)
			},
		},
		{
			Name: "images",
			Extract: func(side *compareSideSummary) string {
				if side == nil || len(side.Images) == 0 {
					return ""
				}
				return strings.Join(side.Images, ", ")
			},
		},
	}

	out := make([]compareFieldMismatch, 0, len(fields))
	for _, field := range fields {
		dryValue := field.Extract(dry)
		wetValue := field.Extract(wet)
		liveValue := field.Extract(&live)
		if !compareFieldHasDivergence(dryValue, wetValue, liveValue) {
			continue
		}
		out = append(out, compareFieldMismatch{
			Field: field.Name,
			Dry:   displayCompareFieldValue(dryValue),
			Wet:   displayCompareFieldValue(wetValue),
			Live:  displayCompareFieldValue(liveValue),
		})
	}
	return out
}

func compareFieldHasDivergence(values ...string) bool {
	unique := make(map[string]struct{}, len(values))
	nonEmpty := 0
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		nonEmpty++
		unique[value] = struct{}{}
	}
	return nonEmpty >= 2 && len(unique) >= 2
}

func displayCompareFieldValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "-"
	}
	return value
}

func renderCompareResourceASCII(result compareResourceResult) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Compare Resource: %s", result.Resource))
	if result.Namespace != "" {
		b.WriteString(fmt.Sprintf(" (namespace: %s)", result.Namespace))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Mode: %s\n", result.Mode))
	if result.Connected {
		b.WriteString("Connection: connected\n")
	} else {
		b.WriteString("Connection: standalone\n")
	}

	if result.Dry != nil {
		renderCompareASCIISection(&b, "DRY (unit intent)", *result.Dry)
	}
	if result.Wet != nil {
		renderCompareASCIISection(&b, "WET (rendered target)", *result.Wet)
	}
	renderCompareASCIISection(&b, "LIVE (cluster)", result.Live)
	if len(result.Mismatches) > 0 {
		b.WriteString("\nDiff Highlights\n")
		for _, mismatch := range result.Mismatches {
			b.WriteString(fmt.Sprintf("  - %s: DRY=%s | WET=%s | LIVE=%s\n",
				mismatch.Field, mismatch.Dry, mismatch.Wet, mismatch.Live))
		}
	}

	if len(result.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, note := range result.Notes {
			b.WriteString(fmt.Sprintf("  - %s\n", note))
		}
	}

	return b.String()
}

func renderCompareResourceMarkdown(result compareResourceResult) string {
	var b strings.Builder

	b.WriteString("## Compare Resource\n\n")
	b.WriteString(fmt.Sprintf("- Resource: `%s`\n", result.Resource))
	if result.Namespace != "" {
		b.WriteString(fmt.Sprintf("- Namespace: `%s`\n", result.Namespace))
	}
	b.WriteString(fmt.Sprintf("- Mode: `%s`\n", result.Mode))
	if result.Connected {
		b.WriteString("- Connection: `connected`\n\n")
	} else {
		b.WriteString("- Connection: `standalone`\n\n")
	}

	if result.Dry != nil {
		renderCompareMarkdownSection(&b, "DRY (unit intent)", *result.Dry)
	}
	if result.Wet != nil {
		renderCompareMarkdownSection(&b, "WET (rendered target)", *result.Wet)
	}
	renderCompareMarkdownSection(&b, "LIVE (cluster)", result.Live)
	if len(result.Mismatches) > 0 {
		b.WriteString("\n### Mismatches\n\n")
		b.WriteString("| Field | DRY | WET | LIVE |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, mismatch := range result.Mismatches {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				mismatch.Field,
				mdCompareFieldValue(mismatch.Dry),
				mdCompareFieldValue(mismatch.Wet),
				mdCompareFieldValue(mismatch.Live),
			))
		}
		b.WriteString("\n")
	}

	if len(result.Notes) > 0 {
		b.WriteString("\n### Notes\n\n")
		for _, note := range result.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", note))
		}
	}

	return b.String()
}

func renderCompareASCIISection(b *strings.Builder, title string, side compareSideSummary) {
	b.WriteString("\n" + title + "\n")
	b.WriteString(fmt.Sprintf("  apiVersion: %s\n", valueOrDash(side.APIVersion)))
	b.WriteString(fmt.Sprintf("  kind: %s\n", valueOrDash(side.Kind)))
	b.WriteString(fmt.Sprintf("  name: %s\n", valueOrDash(side.Name)))
	b.WriteString(fmt.Sprintf("  namespace: %s\n", valueOrDash(side.Namespace)))
	if side.Generation > 0 {
		b.WriteString(fmt.Sprintf("  generation: %d\n", side.Generation))
	}
	if side.Replicas != nil {
		b.WriteString(fmt.Sprintf("  replicas: %d\n", *side.Replicas))
	}
	if len(side.Images) > 0 {
		b.WriteString("  images:\n")
		for _, image := range side.Images {
			b.WriteString(fmt.Sprintf("    - %s\n", image))
		}
	}
}

func renderCompareMarkdownSection(b *strings.Builder, title string, side compareSideSummary) {
	b.WriteString("### " + title + "\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("|---|---|\n")
	b.WriteString(fmt.Sprintf("| apiVersion | %s |\n", mdValueOrDash(side.APIVersion)))
	b.WriteString(fmt.Sprintf("| kind | %s |\n", mdValueOrDash(side.Kind)))
	b.WriteString(fmt.Sprintf("| name | %s |\n", mdValueOrDash(side.Name)))
	b.WriteString(fmt.Sprintf("| namespace | %s |\n", mdValueOrDash(side.Namespace)))
	if side.Generation > 0 {
		b.WriteString(fmt.Sprintf("| generation | %d |\n", side.Generation))
	}
	if side.Replicas != nil {
		b.WriteString(fmt.Sprintf("| replicas | %d |\n", *side.Replicas))
	}
	if len(side.Images) > 0 {
		b.WriteString(fmt.Sprintf("| images | `%s` |\n", strings.Join(side.Images, "`, `")))
	}
	b.WriteString("\n")
}

func valueOrDash(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "-"
	}
	return raw
}

func mdValueOrDash(raw string) string {
	value := valueOrDash(raw)
	if value == "-" {
		return value
	}
	return "`" + value + "`"
}

func mdCompareFieldValue(raw string) string {
	value := displayCompareFieldValue(raw)
	if value == "-" {
		return value
	}
	return "`" + strings.ReplaceAll(value, "`", "\\`") + "`"
}
