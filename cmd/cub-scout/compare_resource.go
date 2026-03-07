// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type compareSideSummary struct {
	Source          string   `json:"source"`
	APIVersion      string   `json:"apiVersion,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Name            string   `json:"name,omitempty"`
	Namespace       string   `json:"namespace,omitempty"`
	Generation      int64    `json:"generation,omitempty"`
	ResourceVersion string   `json:"resourceVersion,omitempty"`
	Replicas        *int64   `json:"replicas,omitempty"`
	Images          []string `json:"images,omitempty"`
	LabelCount      int      `json:"labelCount,omitempty"`
	AnnotationCount int      `json:"annotationCount,omitempty"`
}

type compareResourceResult struct {
	Resource  string              `json:"resource"`
	Namespace string              `json:"namespace,omitempty"`
	Mode      string              `json:"mode"`
	Connected bool                `json:"connected"`
	Dry       *compareSideSummary `json:"dry,omitempty"`
	Wet       *compareSideSummary `json:"wet,omitempty"`
	Live      compareSideSummary  `json:"live"`
	Notes     []string            `json:"notes,omitempty"`
}

var (
	loadCompareLiveSnapshotFn = loadCompareLiveSnapshot
	compareConnectedFn        = isCompareConnected
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
	if connected {
		notes = append(notes, "Connected mode detected; DRY/WET expected-state comparison wiring is pending in this command.")
	} else {
		notes = append(notes, "Connect to ConfigHub to unlock DRY/WET/LIVE expected-state comparison.")
	}

	return compareResourceResult{
		Resource:  kind + "/" + name,
		Namespace: ns,
		Mode:      "live-only",
		Connected: connected,
		Live:      live,
		Notes:     notes,
	}, nil
}

func isCompareConnected() bool {
	return hub.NewClient().RequireConnected() == nil
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

	return summarizeCompareLiveObject(obj), nil
}

func summarizeCompareLiveObject(obj *unstructured.Unstructured) compareSideSummary {
	out := compareSideSummary{
		Source:          "cluster",
		APIVersion:      obj.GetAPIVersion(),
		Kind:            obj.GetKind(),
		Name:            obj.GetName(),
		Namespace:       obj.GetNamespace(),
		Generation:      obj.GetGeneration(),
		ResourceVersion: obj.GetResourceVersion(),
		LabelCount:      len(obj.GetLabels()),
		AnnotationCount: len(obj.GetAnnotations()),
		Images:          extractCompareImages(obj.Object),
	}
	if replicas, ok, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); ok {
		out.Replicas = &replicas
	}
	return out
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

	b.WriteString("\nLIVE (cluster)\n")
	b.WriteString(fmt.Sprintf("  apiVersion: %s\n", valueOrDash(result.Live.APIVersion)))
	b.WriteString(fmt.Sprintf("  kind: %s\n", valueOrDash(result.Live.Kind)))
	b.WriteString(fmt.Sprintf("  name: %s\n", valueOrDash(result.Live.Name)))
	b.WriteString(fmt.Sprintf("  namespace: %s\n", valueOrDash(result.Live.Namespace)))
	if result.Live.Generation > 0 {
		b.WriteString(fmt.Sprintf("  generation: %d\n", result.Live.Generation))
	}
	if result.Live.Replicas != nil {
		b.WriteString(fmt.Sprintf("  replicas: %d\n", *result.Live.Replicas))
	}
	if len(result.Live.Images) > 0 {
		b.WriteString("  images:\n")
		for _, image := range result.Live.Images {
			b.WriteString(fmt.Sprintf("    - %s\n", image))
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

	b.WriteString("### LIVE (cluster)\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("|---|---|\n")
	b.WriteString(fmt.Sprintf("| apiVersion | %s |\n", mdValueOrDash(result.Live.APIVersion)))
	b.WriteString(fmt.Sprintf("| kind | %s |\n", mdValueOrDash(result.Live.Kind)))
	b.WriteString(fmt.Sprintf("| name | %s |\n", mdValueOrDash(result.Live.Name)))
	b.WriteString(fmt.Sprintf("| namespace | %s |\n", mdValueOrDash(result.Live.Namespace)))
	if result.Live.Generation > 0 {
		b.WriteString(fmt.Sprintf("| generation | %d |\n", result.Live.Generation))
	}
	if result.Live.Replicas != nil {
		b.WriteString(fmt.Sprintf("| replicas | %d |\n", *result.Live.Replicas))
	}
	if len(result.Live.Images) > 0 {
		b.WriteString(fmt.Sprintf("| images | `%s` |\n", strings.Join(result.Live.Images, "`, `")))
	}

	if len(result.Notes) > 0 {
		b.WriteString("\n### Notes\n\n")
		for _, note := range result.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", note))
		}
	}

	return b.String()
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
