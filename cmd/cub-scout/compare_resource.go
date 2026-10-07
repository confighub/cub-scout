// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"path/filepath"
	"sigs.k8s.io/yaml"
)

type compareSideSummary struct {
	Source                 string   `json:"source"`
	APIVersion             string   `json:"apiVersion,omitempty"`
	Kind                   string   `json:"kind,omitempty"`
	Name                   string   `json:"name,omitempty"`
	Namespace              string   `json:"namespace,omitempty"`
	UnitSlug               string   `json:"unitSlug,omitempty"`
	UnitID                 string   `json:"unitId,omitempty"`
	SpaceName              string   `json:"spaceName,omitempty"`
	SpaceID                string   `json:"spaceId,omitempty"`
	HeadRevisionNum        int      `json:"headRevisionNum,omitempty"`
	LiveRevisionNum        int      `json:"liveRevisionNum,omitempty"`
	LastAppliedRevisionNum int      `json:"lastAppliedRevisionNum,omitempty"`
	Generation             int64    `json:"generation,omitempty"`
	ResourceVersion        string   `json:"resourceVersion,omitempty"`
	Replicas               *int64   `json:"replicas,omitempty"`
	Images                 []string `json:"images,omitempty"`
	LabelCount             int      `json:"labelCount,omitempty"`
	AnnotationCount        int      `json:"annotationCount,omitempty"`

	// Attribution carries the resource-level mutation-source classification
	// computed from metadata.managedFields. Only populated on the live side
	// (Source == "cluster"); dry/wet sides do not have managedFields data.
	Attribution *agent.FieldMutationAttribution `json:"attribution,omitempty"`

	// AttributionByPath carries per-field-path mutation-source classification
	// keyed by canonical field-path strings (e.g., ".spec.replicas"). Populated
	// when managedFields entries include decodable FieldsV1 data. Used by the
	// mismatch detector to give per-field cause/managerHint, falling back to
	// Attribution above when a field doesn't map to a known path.
	AttributionByPath map[string]agent.FieldMutationAttribution `json:"attributionByPath,omitempty"`

	// GitSource is the source-control anchor (repo URL, revision, path) the
	// GitOps controller is reconciling from. Resolved at A2 via the existing
	// Argo / Flux tracers; nil when no GitOps owner is detected, when tracer
	// CLIs are unavailable, or when the chain root carries no useful data.
	// Stage B will refine to per-field anchors (file path + line) via
	// rendering-aware back-resolution.
	GitSource *agent.GitSourceAnchor `json:"gitSource,omitempty"`
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

	// IncomingBindings lists ConfigHub Links whose downstream (consumer) unit
	// is this resource's unit — i.e., where its config data is influenced
	// from. Populated only in connected mode when the live unit slug is
	// known and the `cub link list` call succeeds (C1 of the attribution
	// layer, issue #435). Per-field binding attribution is C2.
	IncomingBindings []IncomingBinding `json:"incomingBindings,omitempty"`
}

type compareFieldMismatch struct {
	Field string `json:"field"`
	Dry   string `json:"dry"`
	Wet   string `json:"wet"`
	Live  string `json:"live"`

	// Cause classifies the mutation source for this field mismatch. At A1
	// the same value is set for every mismatch on a resource (resource-level
	// classification from managedFields + ownership co-signal). A1.5 will
	// refine to per-field-path resolution.
	Cause agent.FieldMutationCause `json:"cause,omitempty"`

	// ManagerHint is a representative manager string for transparency — see
	// agent.FieldMutationAttribution.ManagerHint.
	ManagerHint string `json:"managerHint,omitempty"`

	// GitSource is the source-control anchor (repo URL, revision, path) for
	// this field's value, copied from the resource-level GitSource at A2.
	// Per-field anchors (different paths/lines per field) are stage B.
	GitSource *agent.GitSourceAnchor `json:"gitSource,omitempty"`

	// BindingSource identifies the ConfigHub Link binding that produces
	// this field's value (C2 of the attribution layer). Set when the field
	// name maps to a known canonical path and an incoming binding's
	// downstream path matches.
	BindingSource *FieldBindingSource `json:"bindingSource,omitempty"`
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
	runCompareCubCommand          = runCompareCubCommandImpl
	loadCompareDryFromPathFn      = loadCompareDryFromPath
)

// compareThreeWayDrySummaries holds the DRY-side summaries loaded from a local
// --dry-from file/dir (standalone git/file-as-DRY mode, #479). Non-nil only
// while a `compare three-way --dry-from` run is in flight; buildCompareResourceResult
// sources DRY from here instead of ConfigHub when set.
var compareThreeWayDrySummaries []*compareSideSummary

// compareSourcePath holds the value of --source-path (stage B back-resolution).
// Read by detectCompareFieldMismatches when populating per-mismatch
// gitSource.file:line. Empty means no back-resolution is attempted.
var compareSourcePath string

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

type compareResourceOptions struct {
	ReportBindingErrors bool
	Live                func(context.Context, string, string, string) (compareSideSummary, error)
	DrySummaries        []*compareSideSummary
	SourcePath          string
}

func buildCompareResourceResult(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
	return buildCompareResourceResultWithOptions(ctx, resourceArg, namespace, compareResourceOptions{Live: loadCompareLiveSnapshotFn, DrySummaries: compareThreeWayDrySummaries, SourcePath: compareSourcePath})
}

func buildCompareResourceResultWithOptions(ctx context.Context, resourceArg, namespace string, options compareResourceOptions) (compareResourceResult, error) {
	kindRaw, name, err := parseResourceArg(resourceArg)
	if err != nil {
		return compareResourceResult{}, err
	}

	kind := normalizeKind(kindRaw)
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}

	live, err := options.Live(ctx, kind, name, ns)
	if err != nil && live.Kind == "" {
		return compareResourceResult{}, fmt.Errorf("load LIVE snapshot for %s/%s in namespace %s: %w", kind, name, ns, err)
	}
	if strings.TrimSpace(live.Source) == "" {
		live.Source = "cluster"
	}

	partialNotes := []string{}
	if err != nil {
		partialNotes = append(partialNotes, fmt.Sprintf("LIVE enrichment incomplete: %v", err))
	}
	finalize := func(result compareResourceResult) compareResourceResult {
		result.Notes = append(result.Notes, partialNotes...)
		return finalizeCompareResourceResultWithBindingsOptions(ctx, result, options.SourcePath, options.ReportBindingErrors)
	}

	// Standalone git/file-as-DRY (#479): when a --dry-from source is loaded,
	// source the DRY side from it instead of ConfigHub and compare DRY vs LIVE.
	// WET is unavailable without a deployer/ConfigHub, so it stays nil — the
	// mismatch detector skips empty sides, so this is a clean DRY-vs-LIVE diff.
	if options.DrySummaries != nil {
		dry := matchCompareDryFromSummaries(options.DrySummaries, kind, name, ns)
		notes := make([]string, 0, 1)
		mode := "live-only"
		if dry != nil {
			mode = "dry-live"
		} else {
			notes = append(notes, fmt.Sprintf("No DRY manifest for %s/%s in %s found in the --dry-from source.", kind, name, ns))
		}
		return finalize(compareResourceResult{
			Resource:  kind + "/" + name,
			Namespace: ns,
			Mode:      mode,
			Connected: false,
			Dry:       dry,
			Live:      live,
			Notes:     notes,
		}), nil
	}

	connected := compareConnectedFn()
	notes := make([]string, 0, 2)
	mode := "live-only"
	if connected {
		unitSlug := strings.TrimSpace(live.UnitSlug)
		if unitSlug == "" {
			notes = append(notes, "Connected mode detected, but this LIVE resource is not linked to a ConfigHub unit (`confighub.com/UnitSlug` missing).")
		} else {
			// The live object's own space: its name, else its ID, which cub
			// accepts for --space. Current ConfigHub releases stamp the ID.
			space := configHubSpace{Slug: firstNonEmpty(strings.TrimSpace(live.SpaceName), strings.TrimSpace(live.SpaceID)), Source: spaceSourceResource}
			if !space.IsSet() {
				space = resolveConfigHubSpace("")
			}
			if !space.IsSet() || space.IsAll() {
				// A unit slug is unique only within a space, and cub looks up a
				// slug with no space across the whole organization. DRY/WET from
				// the wrong unit is worse than none.
				notes = append(notes, fmt.Sprintf("DRY/WET lookup skipped for unit %s: the LIVE resource carries no ConfigHub space and CUB_SPACE does not name one space.", unitSlug))
				return finalize(compareResourceResult{
					Resource:  kind + "/" + name,
					Namespace: ns,
					Mode:      mode,
					Connected: connected,
					Live:      live,
					Notes:     notes,
				}), nil
			}
			if space.Source != spaceSourceResource {
				notes = append(notes, fmt.Sprintf("DRY/WET for unit %s is read from space %s, named by %s: the LIVE resource carries no ConfigHub space, so the unit found there may not be the one that applied this resource.", unitSlug, space.Slug, space.Source))
			}
			dryWet, err := loadCompareDryWetSnapshotFn(ctx, unitSlug, space.Slug, compareResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			})
			if err != nil {
				notes = append(notes, fmt.Sprintf("Connected DRY/WET lookup failed for unit %s: %v", unitSlug, err))
			} else {
				// The mode names the sides actually compared.
				switch {
				case dryWet.Dry != nil && dryWet.Wet != nil:
					mode = "dry-wet-live"
				case dryWet.Dry != nil:
					mode = "dry-live"
				case dryWet.Wet != nil:
					mode = "wet-live"
				}
				notes = append(notes, dryWet.Notes...)
				if dryWet.Dry == nil {
					notes = append(notes, "DRY snapshot unavailable for linked unit.")
				}
				return finalize(compareResourceResult{
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
		notes = append(notes, compareNoteConfigHubReadsUnavailable)
	}

	return finalize(compareResourceResult{
		Resource:  kind + "/" + name,
		Namespace: ns,
		Mode:      mode,
		Connected: connected,
		Live:      live,
		Notes:     notes,
	}), nil
}

// compareNoteConfigHubReadsUnavailable is the note a live-only comparison adds
// when ConfigHub reads are unavailable.
//
// It replaces "Connect to ConfigHub to unlock ...", which offered one remedy
// for every cause, including the causes it cannot fix: an expired session, no
// `cub` on PATH, or reads turned off. `status` reports which one it is.
//
// classifyThreeWayResult buckets a result on this exact note, so it is a
// constant rather than a string at each end: rewording it once emptied the
// "disconnected" bucket with nothing failing.
const compareNoteConfigHubReadsUnavailable = "DRY/WET/LIVE expected-state comparison needs ConfigHub reads; run `cub-scout status` for the reason they are unavailable."

// isCompareConnected uses the same gate as `compare three-way --view` and
// `compare source-truth`, so one command cannot admit a user at the door and
// then report every resource as live-only because a second, different check
// (a probe of hub.confighub.com) disagreed. It is asked once per resource, so
// the answer is computed once per process rather than one `cub` run each.
func isCompareConnected() bool {
	return configHubReadsAvailable()
}

var errCompareResourceNotFoundInManifest = errors.New("resource not found in manifest")

func loadCompareDryWetSnapshots(ctx context.Context, unitSlug, space string, target compareResourceRef) (compareDryWetResult, error) {
	unitSlug = strings.TrimSpace(unitSlug)
	if unitSlug == "" {
		return compareDryWetResult{}, fmt.Errorf("missing unit slug")
	}

	metaRaw, err := runCompareCubCommand(ctx, compareUnitGetArgs(unitSlug, space))
	if err != nil {
		return compareDryWetResult{}, fmt.Errorf("cub unit get: %w", err)
	}
	// Best-effort only: trust hints should degrade gracefully if the unit-get
	// envelope changes, while DRY/WET extraction still remains authoritative.
	unitMeta, _ := decodeCompareUnitMetadataFromGetJSON(metaRaw)

	// The configuration is not part of the unit envelope; it has its own
	// endpoint, which `cub unit data` reads. The payload is plain text.
	dryYAML, err := runCompareCubCommand(ctx, compareUnitDataArgs(unitSlug, space))
	if err != nil {
		return compareDryWetResult{}, fmt.Errorf("cub unit data: %w", err)
	}
	dryYAML = strings.TrimSpace(dryYAML)
	if dryYAML == "" {
		return compareDryWetResult{}, fmt.Errorf("unit %s has no configuration data", unitSlug)
	}
	drySummary, dryErr := extractCompareSummaryFromManifestYAML("dry", dryYAML, target)
	if dryErr != nil && !errors.Is(dryErr, errCompareResourceNotFoundInManifest) {
		return compareDryWetResult{}, fmt.Errorf("extract DRY snapshot: %w", dryErr)
	}

	// WET came from `cub unit livedata`, which cub removed in April 2026 along
	// with `unit livestate`; ConfigHub no longer exposes a unit's live data. Run
	// anyway, cub exits 0 and prints the unit help text, which failed to parse
	// and discarded the DRY side with it. So WET is not read, and the result
	// says so.
	notes := []string{fmt.Sprintf("WET is not available for unit %s: ConfigHub no longer exposes a unit's live data (cub unit livedata was removed), so this compares DRY with LIVE.", unitSlug)}
	if errors.Is(dryErr, errCompareResourceNotFoundInManifest) {
		notes = append(notes, "DRY manifest found but target resource was not present in unit intent data.")
	}
	for _, summary := range []*compareSideSummary{drySummary} {
		switch {
		case summary == nil:
		case agent.IsUUID(space):
			if summary.SpaceID == "" {
				summary.SpaceID = space
			}
		case summary.SpaceName == "":
			summary.SpaceName = space
		}
	}
	applyCompareUnitMetadata(drySummary, unitMeta)
	return compareDryWetResult{
		Dry:   drySummary,
		Notes: notes,
	}, nil
}

// The compare unit reads always name the space; the caller skips the lookup
// when it has none, rather than let the cub context choose.
func compareUnitGetArgs(unitSlug, space string) []string {
	return withConfigHubSpace([]string{"unit", "get", unitSlug, "-o", "json", "--quiet"}, space)
}

func compareUnitDataArgs(unitSlug, space string) []string {
	return withConfigHubSpace([]string{"unit", "data", unitSlug}, space)
}

func runCompareCubCommandImpl(ctx context.Context, args []string) (string, error) {
	return cubText(ctx, args...)
}

func decodeCompareUnitMetadataFromGetJSON(raw string) (compareUnitMetadata, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return compareUnitMetadata{}, fmt.Errorf("empty unit get output")
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return compareUnitMetadata{}, fmt.Errorf("parse unit get json: %w", err)
	}
	items := cubExtractItems(payload)
	if len(items) == 0 {
		return compareUnitMetadata{}, fmt.Errorf("unit get output missing object payload")
	}

	ref := mcpUnitRefFromItem(items[0])
	state, _ := mcpExtractUnitRevisionState(items[0])

	return compareUnitMetadata{
		UnitSlug:            ref.UnitSlug,
		UnitID:              ref.UnitID,
		SpaceName:           ref.SpaceSlug,
		SpaceID:             ref.SpaceID,
		HeadRevisionNum:     state.HeadRevision,
		LiveRevisionNum:     state.LiveRevision,
		LastAppliedRevision: state.LastAppliedRevision,
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
	if summary.LastAppliedRevisionNum == 0 && meta.LastAppliedRevision > 0 {
		summary.LastAppliedRevisionNum = meta.LastAppliedRevision
	}
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

// loadCompareDryFromPath loads DRY-side summaries from a local rendered YAML
// file or directory (#479, standalone git/file-as-DRY). It reuses the
// object-set loader so a directory (e.g. a git checkout), multi-doc YAML, and
// kind:List are all handled, then summarizes each object into the same
// compareSideSummary shape the ConfigHub DRY path produces — so the downstream
// three-way comparison is identical regardless of where DRY came from.
func loadCompareDryFromPath(path string) ([]*compareSideSummary, error) {
	objs, _, err := loadObjectSetDesiredManifests(path)
	if err != nil {
		return nil, err
	}
	summaries := make([]*compareSideSummary, 0, len(objs))
	for _, obj := range objs {
		summaries = append(summaries, summarizeCompareManifestObject("dry", obj.Object))
	}
	return summaries, nil
}

// matchCompareDryFromSummaries finds the DRY summary for a target resource,
// using the same Kind+Name(+Namespace) matching as the ConfigHub DRY path.
func matchCompareDryFromSummaries(summaries []*compareSideSummary, kind, name, ns string) *compareSideSummary {
	target := compareResourceRef{Kind: kind, Name: name, Namespace: ns}
	for _, s := range summaries {
		if matchesCompareSummaryTarget(s, target) {
			return s
		}
	}
	return nil
}

func loadCompareLiveSnapshot(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
	// Every read goes through one session: the explicit selection when there
	// is one, otherwise the ambient context. A summary returned with an error
	// means LIVE was read and its enrichment is incomplete; the caller records
	// it in the result's notes. The ambient path used to read separately and
	// drop that error, so it alone said nothing when it found no source (#823).
	var (
		session *traceSession
		err     error
	)
	if binding := treeContextBinding(ctx); binding != nil {
		session, err = newTraceSessionFromBinding(binding)
	} else {
		session, err = newDefaultTraceSession()
	}
	if err != nil {
		return compareSideSummary{}, fmt.Errorf("build kubernetes config: %w", err)
	}
	return loadCompareLiveSnapshotWithTraceSession(ctx, session, kind, name, namespace)
}

// loadCompareLiveSnapshotWithTraceSession is an internal shared-reader
// foundation. Every Kubernetes read, including source tracing, uses session.
// A non-empty summary with an error means LIVE was read but enrichment is
// incomplete; adapters must retain that error rather than claim full coverage.
// The ambient loader above deliberately preserves its existing behavior.
func loadCompareLiveSnapshotWithTraceSession(ctx context.Context, session *traceSession, kind, name, namespace string) (compareSideSummary, error) {
	return loadCompareLiveSnapshotWithTraceSessionAndFlux(ctx, session, kind, name, namespace, capturedTraceFluxFactory)
}

func loadCompareLiveSnapshotWithTraceSessionAndFlux(ctx context.Context, session *traceSession, kind, name, namespace string, fluxFactory func(*traceSession) (agent.Tracer, func() error, error)) (compareSideSummary, error) {
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return compareSideSummary{}, fmt.Errorf("unsupported resource kind %q for compare mode", kind)
	}
	dyn, err := session.dynamicClient()
	if err != nil {
		return compareSideSummary{}, fmt.Errorf("build dynamic client: %w", err)
	}
	obj, err := dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return compareSideSummary{}, err
	}
	summary := summarizeCompareLiveObject(obj)
	summary.GitSource, err = collectCompareGitSourceWithTraceSession(ctx, session, obj, fluxFactory)
	if summary.UnitSlug != "" {
		return summary, err
	}
	link, linkErr := resolveCompareConfigHubLinkFn(ctx, dyn, obj)
	if linkErr != nil {
		return summary, errors.Join(err, fmt.Errorf("ConfigHub link enrichment unavailable: %w", linkErr))
	}
	summary.UnitSlug = strings.TrimSpace(link.UnitSlug)
	if summary.SpaceName == "" {
		summary.SpaceName = strings.TrimSpace(link.SpaceName)
	}
	if summary.SpaceID == "" {
		summary.SpaceID = strings.TrimSpace(link.SpaceID)
	}
	return summary, err
}

func collectCompareGitSourceWithTraceSession(ctx context.Context, session *traceSession, obj *unstructured.Unstructured, fluxFactory func(*traceSession) (agent.Tracer, func() error, error)) (anchor *agent.GitSourceAnchor, returnErr error) {
	if obj == nil {
		return nil, nil
	}
	if session == nil || session.config == nil {
		return nil, fmt.Errorf("Git source enrichment unavailable: trace session is unavailable")
	}
	owner := agent.DetectOwnership(obj)
	var result *agent.TraceResult
	var err error
	var argoErr error
	if obj.GetKind() == "Application" || owner.Type == agent.OwnerArgo || owner.Type == agent.OwnerConfigHub {
		dyn, clientErr := session.dynamicClient()
		if clientErr != nil {
			return nil, clientErr
		}
		tracer := agent.NewArgoTracerWithKubernetesClient(dyn)
		if obj.GetKind() == "Application" {
			result, err = tracer.TraceApplicationInNamespace(ctx, obj.GetName(), obj.GetNamespace())
		} else if owner.Type == agent.OwnerArgo && owner.Name != "" {
			result, err = tracer.TraceApplicationInNamespace(ctx, owner.Name, owner.Namespace)
		} else {
			// A ConfigHub unit name/space is not an Application identity. Only
			// explicit Argo metadata can authorize an Argo source read.
			appName := argoApplicationNameFromResource(obj)
			if appName != "" {
				result, err = tracer.TraceApplicationInNamespace(ctx, appName, "")
			}
		}
		if err == nil {
			anchor = agent.GitSourceAnchorFromTrace(result)
		}
		if owner.Type != agent.OwnerConfigHub || obj.GetKind() == "Application" || anchor != nil {
			return anchor, compareGitSourceObservationError(result, err)
		}
		if result != nil || err != nil {
			argoErr = compareGitSourceObservationError(result, err)
		}
		// ConfigHub without an Argo anchor keeps the established Flux
		// fallback, on this same captured session and the actual workload.
	} else if owner.Type != agent.OwnerFlux {
		return nil, nil
	}
	// Even a successful Flux fallback must retain an attempted Argo read's
	// omission; that failure is not evidence of absent Argo provenance.
	defer func() { returnErr = errors.Join(argoErr, returnErr) }()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("Git source enrichment unavailable: %w", ctx.Err())
	}
	if fluxFactory == nil {
		return nil, fmt.Errorf("Git source enrichment unavailable: captured Flux adapter is unavailable")
	}
	tracer, cleanup, setupErr := fluxFactory(session)
	if cleanup != nil {
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				anchor = nil
				returnErr = errors.Join(returnErr, fmt.Errorf("unable to remove temporary Trace credentials"))
			}
		}()
	}
	if setupErr != nil {
		return nil, fmt.Errorf("Git source enrichment unavailable: %w", setupErr)
	}
	if tracer == nil || !tracer.Available() {
		return nil, fmt.Errorf("Git source enrichment unavailable: flux CLI is unavailable")
	}
	// Trace the object itself, never its owner. `flux trace` on the owning
	// Kustomization or HelmRelease answers what manages that object: nothing
	// for a root one, and the parent and fleet repository under a parent
	// Kustomization, which is not where this object's manifests came from
	// (#814).
	result, err = tracer.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
	if err != nil {
		return nil, compareGitSourceObservationError(result, err)
	}
	return agent.GitSourceAnchorFromTrace(result), compareGitSourceObservationError(result, nil)
}

func compareGitSourceObservationError(result *agent.TraceResult, err error) error {
	if err != nil {
		return fmt.Errorf("Git source enrichment unavailable: %w", err)
	}
	if result == nil {
		return fmt.Errorf("Git source enrichment unavailable: trace contains no source anchor")
	}
	var partialErr error
	if result.Error != "" {
		partialErr = fmt.Errorf("Git source enrichment incomplete: %s", result.Error)
	}
	if result.MultiSource {
		partialErr = errors.Join(partialErr, fmt.Errorf("Git source enrichment incomplete: only the first of multiple declared sources was parsed"))
	}
	if partialErr != nil {
		return partialErr
	}
	if agent.GitSourceAnchorFromTrace(result) == nil {
		return fmt.Errorf("Git source enrichment unavailable: trace contains no source anchor")
	}
	return nil
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

	// Compute mutation-source attribution from metadata.managedFields, using
	// the owner detected from labels/annotations as the co-signal. See
	// pkg/agent/field_ownership.go. AttributeFieldsByManagedFields also
	// returns per-field-path classifications when FieldsV1 data is present.
	owner := agent.DetectOwnership(obj)
	resourceAttr, byPath := agent.AttributeFieldsByManagedFields(obj, owner)
	out.Attribution = &resourceAttr
	if len(byPath) > 0 {
		out.AttributionByPath = byPath
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

// finalizeCompareResourceResultWithBindingsOptions adds C1 binding enrichment
// and falls through cleanly when the live unit is unknown.
// C2: after IncomingBindings is populated, walks each mismatch and attaches
// the matching FieldBindingSource when the field maps to a known canonical
// path.
func finalizeCompareResourceResultWithBindingsOptions(ctx context.Context, result compareResourceResult, sourcePath string, reportErrors bool) compareResourceResult {
	result.Mismatches = detectCompareFieldMismatchesWithSourcePath(result.Dry, result.Wet, result.Live, sourcePath)
	if !result.Connected {
		return result
	}
	unitID := strings.TrimSpace(result.Live.UnitID)
	if unitID == "" {
		return result
	}
	bindings, err := collectIncomingBindingsWithError(ctx, unitID, firstNonEmpty(result.Live.SpaceName, result.Live.SpaceID))
	if err != nil {
		if reportErrors {
			result.Notes = append(result.Notes, "ConfigHub binding enrichment incomplete: "+threeWayFailureReason(err))
		}
		return result
	}
	if len(bindings) == 0 {
		return result
	}
	result.IncomingBindings = bindings

	// C2: walk mismatches and attach FieldBindingSource for fields whose
	// canonical paths match an incoming binding's downstream path.
	for i := range result.Mismatches {
		path, ok := compareFieldToPath[result.Mismatches[i].Field]
		if !ok {
			continue
		}
		if src := LookupFieldBindingSource(path, bindings); src != nil {
			result.Mismatches[i].BindingSource = src
		}
	}
	return result
}

func detectCompareFieldMismatches(dry, wet *compareSideSummary, live compareSideSummary) []compareFieldMismatch {
	return detectCompareFieldMismatchesWithSourcePath(dry, wet, live, compareSourcePath)
}

func detectCompareFieldMismatchesWithSourcePath(dry, wet *compareSideSummary, live compareSideSummary, sourcePath string) []compareFieldMismatch {
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
		mismatch := compareFieldMismatch{
			Field: field.Name,
			Dry:   displayCompareFieldValue(dryValue),
			Wet:   displayCompareFieldValue(wetValue),
			Live:  displayCompareFieldValue(liveValue),
		}
		// Prefer per-field-path attribution (A1.5); fall back to the
		// resource-level rollup (A1) when no per-path entry is available
		// (e.g., for fields like "images" that don't map to a single path,
		// or when FieldsV1 data is missing from managedFields).
		if attr, ok := lookupFieldAttribution(field.Name, live); ok {
			mismatch.Cause = attr.Cause
			mismatch.ManagerHint = attr.ManagerHint
		} else if live.Attribution != nil {
			mismatch.Cause = live.Attribution.Cause
			mismatch.ManagerHint = live.Attribution.ManagerHint
		}
		// Carry the resource-level git source onto every field mismatch (A2).
		// At stage B, attempt to back-resolve a per-field File:Line when a
		// local source checkout is provided via --source-path. A successful
		// back-resolution clones the GitSource and sets the new File:Line
		// so the resource-level anchor is preserved while each mismatch
		// carries its own per-field provenance.
		if live.GitSource != nil {
			mismatch.GitSource = live.GitSource
			if strings.TrimSpace(sourcePath) != "" {
				if anchor := backResolveFieldGitSourceFromPath(live, field.Name, sourcePath); anchor != nil {
					mismatch.GitSource = anchor
				}
			}
		}
		out = append(out, mismatch)
	}
	return out
}

// backResolveFieldGitSource clones live.GitSource and populates File/Line
// for the given field name when:
//   - compareSourcePath is set
//   - the field name has a canonical-path mapping (compareFieldToPath)
//   - agent.BackResolveGitSource finds a matching document and field
//
// Returns nil when any precondition fails, so callers can fall back to the
// shared resource-level pointer.
func backResolveFieldGitSource(live compareSideSummary, fieldName string) *agent.GitSourceAnchor {
	return backResolveFieldGitSourceFromPath(live, fieldName, compareSourcePath)
}

func backResolveFieldGitSourceFromPath(live compareSideSummary, fieldName, sourcePath string) *agent.GitSourceAnchor {
	if strings.TrimSpace(sourcePath) == "" || live.GitSource == nil {
		return nil
	}
	canonicalPath, ok := compareFieldToPath[fieldName]
	if !ok {
		return nil
	}
	root := strings.TrimSpace(sourcePath)
	if subdir := strings.TrimSpace(live.GitSource.Path); subdir != "" {
		root = filepath.Join(root, subdir)
	}
	hit, ok := agent.BackResolveGitSource(root, live.Kind, live.Name, live.Namespace, canonicalPath)
	if !ok {
		return nil
	}
	clone := *live.GitSource
	clone.File = hit.File
	clone.Line = hit.Line
	return &clone
}

func renderGitSourceASCII(gs *agent.GitSourceAnchor) string {
	parts := make([]string, 0, 5)
	if gs.RepoURL != "" {
		parts = append(parts, gs.RepoURL)
	}
	if gs.Revision != "" {
		parts = append(parts, "@"+gs.Revision)
	}
	if gs.Path != "" {
		parts = append(parts, "path="+gs.Path)
	}
	if gs.File != "" {
		fileRef := "file=" + gs.File
		if gs.Line > 0 {
			fileRef = fmt.Sprintf("file=%s:%d", gs.File, gs.Line)
		}
		parts = append(parts, fileRef)
	}
	if gs.SourceType != "" {
		marker := "source=" + gs.SourceType
		if gs.Resolution != "" {
			marker += " (" + gs.Resolution + ")"
		}
		parts = append(parts, marker)
	}
	return strings.Join(parts, " ")
}

func renderGitSourceMarkdown(gs *agent.GitSourceAnchor) string {
	parts := make([]string, 0, 5)
	if gs.RepoURL != "" {
		parts = append(parts, "`"+gs.RepoURL+"`")
	}
	if gs.Revision != "" {
		parts = append(parts, "@`"+gs.Revision+"`")
	}
	if gs.Path != "" {
		parts = append(parts, "path=`"+gs.Path+"`")
	}
	if gs.File != "" {
		fileRef := "file=`" + gs.File + "`"
		if gs.Line > 0 {
			fileRef = fmt.Sprintf("file=`%s:%d`", gs.File, gs.Line)
		}
		parts = append(parts, fileRef)
	}
	if gs.SourceType != "" {
		marker := "source=`" + gs.SourceType + "`"
		if gs.Resolution != "" {
			marker += " (" + gs.Resolution + ")"
		}
		parts = append(parts, marker)
	}
	return strings.Join(parts, " ")
}

// compareFieldToPath maps a compareFieldMismatch.Field name to the canonical
// metadata.managedFields path string used by sigs.k8s.io/structured-merge-diff/v4
// fieldpath.Path.String. Fields without a single canonical path (e.g., "images"
// which spreads across container list items) are intentionally absent from
// this map and fall back to resource-level attribution.
var compareFieldToPath = map[string]string{
	"apiVersion": ".apiVersion",
	"kind":       ".kind",
	"namespace":  ".metadata.namespace",
	"replicas":   ".spec.replicas",
}

func lookupFieldAttribution(fieldName string, live compareSideSummary) (agent.FieldMutationAttribution, bool) {
	if len(live.AttributionByPath) == 0 {
		return agent.FieldMutationAttribution{}, false
	}
	path, ok := compareFieldToPath[fieldName]
	if !ok {
		return agent.FieldMutationAttribution{}, false
	}
	attr, ok := live.AttributionByPath[path]
	if !ok || attr.Cause == "" {
		return agent.FieldMutationAttribution{}, false
	}
	return attr, true
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
		if attr := result.Live.Attribution; attr != nil && attr.Cause != "" {
			b.WriteString(fmt.Sprintf("\nDrift cause: %s", attr.Cause))
			if attr.ManagerHint != "" {
				b.WriteString(fmt.Sprintf(" (manager: %s)", attr.ManagerHint))
			}
			b.WriteString("\n")
		}
		if gs := result.Live.GitSource; gs != nil && !gs.IsEmpty() {
			b.WriteString(fmt.Sprintf("Git source: %s", renderGitSourceASCII(gs)))
			b.WriteString("\n")
		}
		b.WriteString("\nDiff Highlights\n")
		for _, mismatch := range result.Mismatches {
			b.WriteString(fmt.Sprintf("  - %s: DRY=%s | WET=%s | LIVE=%s\n",
				mismatch.Field, mismatch.Dry, mismatch.Wet, mismatch.Live))
			if bs := mismatch.BindingSource; bs != nil {
				bindLine := "      <- bound from"
				if bs.UpstreamUnitID != "" {
					bindLine += fmt.Sprintf(" unit:%s", bs.UpstreamUnitID)
				}
				if bs.UpstreamPath != "" {
					bindLine += fmt.Sprintf(" path:%s", bs.UpstreamPath)
				}
				if bs.LinkSlug != "" {
					bindLine += fmt.Sprintf(" via link:%s", bs.LinkSlug)
				}
				if bs.TransformExpr != "" {
					bindLine += fmt.Sprintf(" transform:%s", bs.TransformExpr)
				}
				b.WriteString(bindLine + "\n")
			}
		}
	}

	renderIncomingBindingsASCII(&b, result.IncomingBindings)

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
		if attr := result.Live.Attribution; attr != nil && attr.Cause != "" {
			b.WriteString(fmt.Sprintf("Drift cause: `%s`", attr.Cause))
			if attr.ManagerHint != "" {
				b.WriteString(fmt.Sprintf(" (manager: `%s`)", attr.ManagerHint))
			}
			b.WriteString("\n\n")
		}
		if gs := result.Live.GitSource; gs != nil && !gs.IsEmpty() {
			b.WriteString(fmt.Sprintf("Git source: %s\n\n", renderGitSourceMarkdown(gs)))
		}
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

	renderIncomingBindingsMarkdown(&b, result.IncomingBindings)

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
