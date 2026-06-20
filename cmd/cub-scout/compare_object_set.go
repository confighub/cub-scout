// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// compare object-set flags (#496, Issue A). Named distinctly from the receipt
// flags so the two surfaces never share state.
var (
	compareObjectSetDryFrom   string
	compareObjectSetNamespace string
	compareObjectSetScopeRaw  string
	compareObjectSetDiff      bool
	compareObjectSetFormat    string
	compareObjectSetOut       string
	compareObjectSetSave      bool
	compareObjectSetTTL       string
	compareObjectSetFailOn    string
	compareObjectSetNormProf  string
)

var compareObjectSetCmd = &cobra.Command{
	Use:   "object-set",
	Short: "Emit a set-level diff receipt between a rendered object set and live",
	Long: `object-set compares a rendered (desired) object set against live cluster
state and emits a single signed, chain-walkable ObjectSetDiffReceipt
(predicate object-set-diff). Unlike compare three-way (per resource) and
receipt verify --predicate object-set-matches (a boolean "does the set
match?"), this aggregates the per-object authored-field deltas into ONE
set-level receipt: changedObjects (with field deltas), and — with --diff —
addedObjects / removedObjects.

One receipt shape serves both day-1 and day-2 (#496):
  - dry-run (pre-apply): --dry-from <changed render>  -> what a proposed
    change would touch across the set.
  - drift (post-apply):  --dry-from <current render>  -> what differs now.

It is tool-agnostic by construction: it reads the cluster and the --dry-from
source, never the reconciler, so Argo-, Flux-, and cub-direct-delivered
objects diff identically. Read-only against the cluster.

Verdict:
  PASS   no authored-field, added, or removed deltas.
  WATCH  only closure deltas (added/removed objects), no shared-object field
         changes (requires --diff to compute closure).
  BLOCK  one or more objects have authored-field deltas.

Examples:
  cub-scout compare object-set --dry-from out/manifests -n redis --format json
  cub-scout compare object-set --dry-from out/changed.yaml -n redis --diff
  cub-scout compare object-set --dry-from out/manifests --scope namespace/redis --fail-on any-non-pass`,
	Args: cobra.NoArgs,
	RunE: runCompareObjectSet,
}

func init() {
	combinedCmd.AddCommand(compareObjectSetCmd)

	compareObjectSetCmd.Flags().StringVar(&compareObjectSetDryFrom, "dry-from", "", "Rendered desired object set: a YAML file or directory (e.g. a git checkout / helm template output) used as the DESIRED state to diff against live (required).")
	compareObjectSetCmd.Flags().StringVarP(&compareObjectSetNamespace, "namespace", "n", "", "Default namespace for namespaced objects without an explicit metadata.namespace")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetScopeRaw, "scope", "", "Scope selector: 'namespace/<ns>' or 'cluster'. Defaults to namespace/<-n> or mixed.")
	compareObjectSetCmd.Flags().BoolVar(&compareObjectSetDiff, "diff", false, "Closed-world diff: also report addedObjects (live objects of the rendered kinds in scope not in the desired set) and removedObjects (desired objects absent live). Closure deltas alone downgrade a clean PASS to WATCH.")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetFormat, "format", "json", "Output format: json | ascii")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetOut, "out", "", "Write the receipt to this file path (also printed to stdout in the chosen format)")
	compareObjectSetCmd.Flags().BoolVar(&compareObjectSetSave, "save", false, "Save the receipt to the local store ($CUB_SCOUT_RECEIPTS_DIR or $XDG_DATA_HOME/cub-scout/receipts) for later cub-scout receipt list / show / validate")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetTTL, "ttl", "", "Observation-freshness boundary, e.g. 1h. When set, the receipt records an immutable freshness{observedAt,expiresAt,ttl}. Empty means the receipt makes no freshness claim.")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetFailOn, "fail-on", "", "Exit non-zero (code 2) when the receipt verdict matches. Accepts a comma-separated list of verdicts (WATCH, BLOCK, INCONCLUSIVE) or the sugar 'any-non-pass'. The receipt is still printed / saved / written to --out regardless of exit code.")
	compareObjectSetCmd.Flags().StringVar(&compareObjectSetNormProf, "normalization-profile", "", "Named server-normalization profile applied symmetrically to desired and live objects before comparison and digesting (e.g. k8s-zero-defaults/v1). Recorded on the receipt. Empty means raw comparison.")
}

func runCompareObjectSet(cmd *cobra.Command, _ []string) error {
	format := strings.ToLower(strings.TrimSpace(compareObjectSetFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", compareObjectSetFormat)
	}

	if strings.TrimSpace(compareObjectSetDryFrom) == "" {
		return fmt.Errorf("--dry-from is required for compare object-set (a rendered YAML file or directory)")
	}

	ttlDur, ttlErr := parseCompareObjectSetTTL()
	if ttlErr != nil {
		return ttlErr
	}

	var failOnSet map[agent.ReceiptVerdict]bool
	failOnRaw := strings.TrimSpace(compareObjectSetFailOn)
	if failOnRaw != "" {
		parsed, parseErr := parseReceiptFailOn(failOnRaw)
		if parseErr != nil {
			return parseErr
		}
		failOnSet = parsed
	}

	scope, defaultNamespace, err := resolveObjectSetScope(compareObjectSetScopeRaw, compareObjectSetNamespace)
	if err != nil {
		return err
	}

	desired, source, err := loadObjectSetDesiredManifests(compareObjectSetDryFrom)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		return fmt.Errorf("--dry-from %q contained no Kubernetes objects", compareObjectSetDryFrom)
	}

	observed, err := loadObjectSetLiveObjectsFn(cmd.Context(), desired, defaultNamespace)
	if err != nil {
		return err
	}
	profile := strings.TrimSpace(compareObjectSetNormProf)
	observed, err = agent.NormalizeObservedObjects(profile, observed)
	if err != nil {
		return err
	}

	evidence, err := buildObjectSetDiffEvidence(source, scope, observed)
	if err != nil {
		return err
	}
	evidence.NormalizationProfile = profile

	// Removed objects (desired-but-absent-live) are a membership delta we get
	// for free from the observed desired/live pairs, so they are always
	// reported. The closed-world ADDED side (live objects of the rendered
	// kinds not in the desired set) requires a cluster-wide list, so it is
	// gated behind --diff.
	evidence.RemovedObjects = collectObjectSetRemoved(observed)
	if compareObjectSetDiff {
		extras, exErr := loadObjectSetExtrasFn(cmd.Context(), desired, scope, defaultNamespace)
		if exErr != nil {
			return fmt.Errorf("closed-world diff: %w", exErr)
		}
		evidence.AddedObjects = extras
	}

	stmt, err := agent.BuildObjectSetDiffReceipt(agent.BuildObjectSetDiffReceiptInput{
		Evidence: evidence,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("build object-set-diff receipt: %w", err)
	}
	if err := agent.ApplyFreshness(&stmt, ttlDur); err != nil {
		return fmt.Errorf("apply freshness: %w", err)
	}

	var out []byte
	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt: %w", mErr)
		}
		out = buf
	case "ascii":
		out = []byte(renderReceiptASCII(stmt))
	}
	fmt.Println(string(out))

	if outPath := strings.TrimSpace(compareObjectSetOut); outPath != "" {
		jsonBytes, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt for --out: %w", mErr)
		}
		if err := writeReceiptOutFile(outPath, jsonBytes); err != nil {
			return err
		}
	}

	if compareObjectSetSave {
		if err := saveOneReceipt(cmd, stmt); err != nil {
			return fmt.Errorf("save receipt: %w", err)
		}
	}

	if failOnSet != nil && failOnSet[stmt.Predicate.Verdict] {
		return newExitCodeError(
			fmt.Errorf("receipt verdict %q matches --fail-on %q", stmt.Predicate.Verdict, failOnRaw),
			2,
		)
	}
	return nil
}

// buildObjectSetDiffEvidence assembles the set-level diff evidence from the
// observed desired/live pairs. It reuses BuildObjectSetEvidence to do the
// authored-field projection + digesting (so the desired/live digests and the
// per-object field differences are computed identically to object-set-matches),
// then maps each mismatched object into the diff shape. Per-field
// cause/managerHint attribution is sourced from the live managedFields via the
// shared compare path (detectCompareFieldMismatches) when a live object exists.
//
// changedObjects are objects present on BOTH sides whose authored fields
// differ. A desired object that is absent live is a membership delta, recorded
// by the caller as a removedObject (not a changed object), so the verdict rule
// can separate field drift (BLOCK) from closure deltas (WATCH). Inconclusive
// lookups return an error so the caller maps them to a build failure rather
// than a false PASS.
func buildObjectSetDiffEvidence(source agent.ObjectSetSource, scope agent.ObjectSetScope, observed []agent.ObjectSetObservedObject) (agent.ObjectSetDiffEvidence, error) {
	for _, obs := range observed {
		if obs.Inconclusive {
			id := agent.NewObjectSetObjectID(obs.Desired)
			return agent.ObjectSetDiffEvidence{}, fmt.Errorf("object-set-diff: live lookup inconclusive for %s: %s", id.String(), obs.Error)
		}
	}

	base, err := agent.BuildObjectSetEvidence(source, scope, observed)
	if err != nil {
		return agent.ObjectSetDiffEvidence{}, err
	}

	// Index the observed live objects by identity so per-field attribution can
	// be pulled from the same compare path the three-way diff uses.
	liveByKey := map[string]*unstructured.Unstructured{}
	desiredByKey := map[string]*unstructured.Unstructured{}
	for i := range observed {
		if observed[i].Desired == nil {
			continue
		}
		key := agent.NewObjectSetObjectID(observed[i].Desired).Key()
		desiredByKey[key] = observed[i].Desired
		if observed[i].Live != nil {
			liveByKey[key] = observed[i].Live
		}
	}

	var changed []agent.ObjectSetDiffObject
	for _, summary := range base.Objects {
		if summary.Status != agent.ObjectSetObjectMismatched {
			// Matched objects carry no delta; missing objects are membership
			// deltas the caller records as removedObjects.
			continue
		}
		changed = append(changed, agent.ObjectSetDiffObject{
			ID:            summary.ID,
			DesiredDigest: summary.DesiredDigest,
			LiveDigest:    summary.LiveDigest,
			Differences:   mapObjectSetFieldDiffs(summary, desiredByKey[summary.ID.Key()], liveByKey[summary.ID.Key()]),
		})
	}

	sort.Slice(changed, func(i, j int) bool { return changed[i].ID.Key() < changed[j].ID.Key() })

	return agent.ObjectSetDiffEvidence{
		DesiredSource:  base.DesiredSource,
		Scope:          base.Scope,
		DesiredDigest:  base.DesiredDigest,
		LiveDigest:     base.LiveDigest,
		ChangedObjects: changed,
		Summary: agent.ObjectSetDiffSummary{
			Total: base.Summary.Desired,
		},
	}, nil
}

// mapObjectSetFieldDiffs converts the authored-field projection differences for
// one object into ObjectSetFieldDiff entries. It primarily uses the projection
// diffs (which cover EVERY authored field, nested) from BuildObjectSetEvidence,
// then overlays per-field cause/managerHint attribution for the small set of
// well-known fields the compare path classifies (replicas, images, …) when a
// live object is present.
func mapObjectSetFieldDiffs(summary agent.ObjectSetObjectSummary, desired, live *unstructured.Unstructured) []agent.ObjectSetFieldDiff {
	// Build an attribution lookup keyed by compare field name from the shared
	// three-way compare path, so drift on classified fields carries the
	// writing field-manager. Only meaningful when both sides exist.
	attrByField := map[string]compareFieldMismatch{}
	if desired != nil && live != nil {
		drySummary := summarizeCompareManifestObject("dry", desired.Object)
		liveSummary := summarizeCompareLiveObject(live)
		for _, m := range detectCompareFieldMismatches(drySummary, nil, liveSummary) {
			attrByField[m.Field] = m
		}
	}

	out := make([]agent.ObjectSetFieldDiff, 0, len(summary.Differences))
	for _, d := range summary.Differences {
		fd := agent.ObjectSetFieldDiff{
			Field:   d.Path,
			Desired: formatObjectSetDiffValue(d.Desired),
			Live:    formatObjectSetDiffValue(d.Live),
		}
		if attr, ok := attrByField[compareFieldForPath(d.Path)]; ok {
			// Only carry a meaningful attribution. A bare "unknown" cause with
			// no manager hint is the absence of a managedFields signal, not a
			// finding — leave Cause/ManagerHint empty so the receipt stays
			// honest (the field is documented as optional).
			if (attr.Cause != "" && attr.Cause != agent.CauseUnknown) || attr.ManagerHint != "" {
				fd.Cause = string(attr.Cause)
				fd.ManagerHint = attr.ManagerHint
			}
		}
		out = append(out, fd)
	}
	return out
}

// compareFieldForPath maps an authored-field projection path (e.g.
// "spec.replicas" or "spec.template.spec.containers[0].image") to the compare
// field name (e.g. "replicas", "images") used by detectCompareFieldMismatches,
// so we can overlay that path's field-manager attribution. Unmapped paths
// return "" (no attribution overlay).
func compareFieldForPath(path string) string {
	switch {
	case path == "spec.replicas":
		return "replicas"
	case strings.Contains(path, ".containers[") && strings.HasSuffix(path, "].image"):
		return "images"
	case path == "metadata.namespace":
		return "namespace"
	case path == "apiVersion":
		return "apiVersion"
	case path == "kind":
		return "kind"
	default:
		return ""
	}
}

func formatObjectSetDiffValue(v interface{}) string {
	if v == nil {
		return "-"
	}
	switch typed := v.(type) {
	case string:
		if typed == "" {
			return "-"
		}
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// collectObjectSetRemoved returns the desired objects that were absent live —
// the removed side of the closed-world diff (--diff). It mirrors the
// object-set-matches "missing" classification.
func collectObjectSetRemoved(observed []agent.ObjectSetObservedObject) []agent.ObjectSetObjectID {
	var removed []agent.ObjectSetObjectID
	for _, obs := range observed {
		if obs.Desired == nil || obs.Inconclusive {
			continue
		}
		if obs.Live == nil {
			removed = append(removed, agent.NewObjectSetObjectID(obs.Desired))
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Key() < removed[j].Key() })
	return removed
}

func parseCompareObjectSetTTL() (time.Duration, error) {
	raw := strings.TrimSpace(compareObjectSetTTL)
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --ttl %q (expected a Go duration like 1h or 30m): %w", raw, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid --ttl %q (must not be negative)", raw)
	}
	return d, nil
}
