// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/spf13/cobra"
)

const fleetReceiptVerificationSchemaVersion = "fleet-receipt-verification/v1"

var (
	receiptVerifyFleetInputs          []string
	receiptVerifyFleetPubKey          string
	receiptVerifyFleetRevision        string
	receiptVerifyFleetRequireClusters []string
	receiptVerifyFleetMaxAge          time.Duration
	receiptVerifyFleetFormat          string

	// receiptVerifyFleetNow is overridden in tests for deterministic timestamps and max-age checks.
	receiptVerifyFleetNow = func() time.Time { return time.Now().UTC() }
)

var receiptVerifyFleetCmd = &cobra.Command{
	Use:   "verify-fleet",
	Short: "Verify and aggregate signed receipts across multiple clusters offline",
	Long: `verify-fleet verifies a set of local signed-receipt/v1 JSON files against an
Ed25519 public key and aggregates per-cluster landing state into one compact
fleet answer envelope.

verify-fleet is strictly offline and read-only: zero network calls, zero
Kubernetes calls, and zero fleet database. It fails closed on any invalid
signature, tampered payload, wrong key, malformed JSON, revision mismatch, or
missing required cluster.

Examples:
  cub-scout receipt verify-fleet --inputs ./fleet-receipts --pubkey pub.pem --format json
  cub-scout receipt verify-fleet --inputs dev.signed.json,prod.signed.json --pubkey pub.pem --revision a1b2c3d --require-clusters dev,staging,prod
`,
	Args: cobra.NoArgs,
	RunE: runReceiptVerifyFleet,
}

func init() {
	receiptCmd.AddCommand(receiptVerifyFleetCmd)

	receiptVerifyFleetCmd.Flags().StringSliceVar(&receiptVerifyFleetInputs, "inputs", nil, "Paths to signed-receipt/v1 JSON files or directories (repeatable or comma-separated, required)")
	receiptVerifyFleetCmd.Flags().StringVar(&receiptVerifyFleetPubKey, "pubkey", "", "Path to Ed25519 PKIX public key PEM file (required)")
	receiptVerifyFleetCmd.Flags().StringVar(&receiptVerifyFleetRevision, "revision", "", "Expected Git SHA, OCI digest, or revision anchor each receipt must match")
	receiptVerifyFleetCmd.Flags().StringSliceVar(&receiptVerifyFleetRequireClusters, "require-clusters", nil, "Required cluster names (repeatable or comma-separated); missing clusters are reported as not-observable")
	receiptVerifyFleetCmd.Flags().DurationVar(&receiptVerifyFleetMaxAge, "max-age", 0, "Maximum allowed receipt age relative to evaluation time; stale receipts add a stale unknown")
	receiptVerifyFleetCmd.Flags().StringVar(&receiptVerifyFleetFormat, "format", "ascii", "Output format: ascii or json")
}

type fleetReceiptVerificationOutput struct {
	SchemaVersion    string                      `json:"schemaVersion"`
	ExpectedRevision string                      `json:"expectedRevision,omitempty"`
	RequiredClusters []string                    `json:"requiredClusters,omitempty"`
	MaxAge           string                      `json:"maxAge,omitempty"`
	Summary          fleetReceiptSummaryCounts   `json:"summary"`
	Clusters         []fleetClusterReceiptResult `json:"clusters"`
	Answer           *mapsvc.Answer              `json:"answer"`
}

type fleetReceiptSummaryCounts struct {
	Total            int `json:"total"`
	ValidSignatures  int `json:"validSignatures"`
	InvalidReceipts  int `json:"invalidReceipts"`
	Landed           int `json:"landed"`
	Converging       int `json:"converging"`
	NotLanded        int `json:"notLanded"`
	NotObservable    int `json:"notObservable"`
	MissingClusters  int `json:"missingClusters,omitempty"`
	StaleReceipts    int `json:"staleReceipts,omitempty"`
	RevisionMismatch int `json:"revisionMismatch,omitempty"`
}

type fleetClusterReceiptResult struct {
	Cluster       string `json:"cluster"`
	File          string `json:"file,omitempty"`
	Valid         bool   `json:"valid"`
	Missing       bool   `json:"missing,omitempty"`
	Stale         bool   `json:"stale,omitempty"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
	Target        string `json:"target,omitempty"`
	Revision      string `json:"revision,omitempty"`
	RevisionMatch *bool  `json:"revisionMatch,omitempty"`
	ObservedAt    string `json:"observedAt,omitempty"`
	SignedAt      string `json:"signedAt,omitempty"`
	KeyID         string `json:"keyId,omitempty"`
	PayloadDigest string `json:"payloadDigest,omitempty"`
	Error         string `json:"error,omitempty"`
}

type fleetVerifyOptions struct {
	Inputs           []string
	PubKeyPEM        []byte
	ExpectedRevision string
	RequireClusters  []string
	MaxAge           time.Duration
	Now              time.Time
}

func runReceiptVerifyFleet(cmd *cobra.Command, _ []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptVerifyFleetFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptVerifyFleetFormat)
	}
	if len(receiptVerifyFleetInputs) == 0 {
		return fmt.Errorf("--inputs is required (file or directory of signed-receipt/v1 JSON files)")
	}
	pubKeyPath := strings.TrimSpace(receiptVerifyFleetPubKey)
	if pubKeyPath == "" {
		return fmt.Errorf("--pubkey is required (path to Ed25519 PKIX public key PEM)")
	}
	if receiptVerifyFleetMaxAge < 0 {
		return fmt.Errorf("--max-age must be non-negative")
	}

	files, err := resolveFleetReceiptInputFiles(receiptVerifyFleetInputs)
	if err != nil {
		return newExitCodeError(err, 2)
	}
	pubBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return newExitCodeError(fmt.Errorf("read public key %s: %w", pubKeyPath, err), 2)
	}

	out := evaluateFleetReceipts(fleetVerifyOptions{
		Inputs:           files,
		PubKeyPEM:        pubBytes,
		ExpectedRevision: strings.TrimSpace(receiptVerifyFleetRevision),
		RequireClusters:  normalizeClusterList(receiptVerifyFleetRequireClusters),
		MaxAge:           receiptVerifyFleetMaxAge,
		Now:              receiptVerifyFleetNow().UTC(),
	})

	if err := renderFleetReceiptOutput(cmd.OutOrStdout(), out, format); err != nil {
		return err
	}
	if out.Answer.State != agent.LandingStateLanded {
		return newExitCodeError(fmt.Errorf("fleet verification %s (%s)", out.Answer.State, out.Answer.Reason), 1)
	}
	return nil
}

func resolveFleetReceiptInputFiles(inputs []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string

	for _, raw := range inputs {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat input %s: %w", p, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, fmt.Errorf("read input directory %s: %w", p, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
					continue
				}
				full := filepath.Clean(filepath.Join(p, entry.Name()))
				if !seen[full] {
					seen[full] = true
					files = append(files, full)
				}
			}
			continue
		}
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			files = append(files, clean)
		}
	}

	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no signed receipt JSON files found in --inputs")
	}
	return files, nil
}

func normalizeClusterList(raw []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			name := strings.TrimSpace(part)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func evaluateFleetReceipts(opts fleetVerifyOptions) fleetReceiptVerificationOutput {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	results := make([]fleetClusterReceiptResult, 0, len(opts.Inputs)+len(opts.RequireClusters))
	unknowns := make([]mapsvc.AnswerUnknown, 0)
	seenClusters := make(map[string]bool)
	var counts fleetReceiptSummaryCounts

	for _, file := range opts.Inputs {
		entry, staleUnknown := verifySingleFleetReceiptFile(file, opts.PubKeyPEM, opts.ExpectedRevision, opts.MaxAge, now)
		results = append(results, entry)
		seenClusters[entry.Cluster] = true
		if staleUnknown != nil {
			unknowns = append(unknowns, *staleUnknown)
		}
	}

	for _, reqCluster := range opts.RequireClusters {
		if seenClusters[reqCluster] {
			continue
		}
		results = append(results, fleetClusterReceiptResult{
			Cluster: reqCluster,
			Valid:   false,
			Missing: true,
			State:   agent.LandingStateNotObservable,
			Reason:  "missing-required-cluster",
		})
		unknowns = append(unknowns, mapsvc.AnswerUnknown{
			What:   "cluster " + reqCluster,
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "required cluster receipt missing",
		})
	}

	for _, r := range results {
		counts.Total++
		if r.Valid {
			counts.ValidSignatures++
		} else if !r.Missing {
			counts.InvalidReceipts++
		}
		if r.Missing {
			counts.MissingClusters++
		}
		if r.Stale {
			counts.StaleReceipts++
		}
		if r.RevisionMatch != nil && !*r.RevisionMatch {
			counts.RevisionMismatch++
		}
		switch r.State {
		case agent.LandingStateLanded:
			counts.Landed++
		case agent.LandingStateConverging:
			counts.Converging++
		case agent.LandingStateNotLanded:
			counts.NotLanded++
		default:
			counts.NotObservable++
		}
	}

	ans := buildFleetReceiptAnswer(results, counts, unknowns, now)
	out := fleetReceiptVerificationOutput{
		SchemaVersion:    fleetReceiptVerificationSchemaVersion,
		ExpectedRevision: opts.ExpectedRevision,
		RequiredClusters: opts.RequireClusters,
		Summary:          counts,
		Clusters:         results,
		Answer:           ans,
	}
	if opts.MaxAge > 0 {
		out.MaxAge = opts.MaxAge.String()
	}
	return out
}

func verifySingleFleetReceiptFile(file string, pubKeyPEM []byte, expectedRevision string, maxAge time.Duration, now time.Time) (fleetClusterReceiptResult, *mapsvc.AnswerUnknown) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return fleetClusterReceiptResult{
			Cluster: clusterNameFromFilename(file),
			File:    file,
			Valid:   false,
			State:   agent.LandingStateNotLanded,
			Reason:  "read-error",
			Error:   err.Error(),
		}, nil
	}

	verRes, verifyErr := agent.VerifySignedReceipt(raw, pubKeyPEM)
	if verifyErr != nil || verRes == nil || !verRes.Valid {
		res := fleetClusterReceiptResult{
			Cluster: clusterNameFromFilename(file),
			File:    file,
			Valid:   false,
			State:   agent.LandingStateNotLanded,
			Reason:  "signature-invalid",
		}
		if verRes != nil {
			res.SignedAt = verRes.SignedAt
			res.KeyID = verRes.KeyID
			res.PayloadDigest = verRes.PayloadDigest
			res.Error = verRes.Error
		}
		if res.Error == "" && verifyErr != nil {
			res.Error = verifyErr.Error()
		}
		return res, nil
	}

	var env agent.SignedReceiptEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fleetClusterReceiptResult{
			Cluster: clusterNameFromFilename(file),
			File:    file,
			Valid:   false,
			State:   agent.LandingStateNotLanded,
			Reason:  "envelope-parse-error",
			Error:   err.Error(),
		}, nil
	}

	extracted := extractFleetReceiptPayload(env.Payload, file, verRes.SignedAt)
	res := fleetClusterReceiptResult{
		Cluster:       extracted.Cluster,
		File:          file,
		Valid:         true,
		State:         extracted.State,
		Reason:        extracted.Reason,
		Target:        extracted.Target,
		Revision:      extracted.Revision,
		ObservedAt:    extracted.ObservedAt,
		SignedAt:      verRes.SignedAt,
		KeyID:         verRes.KeyID,
		PayloadDigest: verRes.PayloadDigest,
	}

	if expectedRevision != "" {
		matched := fleetRevisionMatches(res.Revision, expectedRevision)
		res.RevisionMatch = &matched
		if !matched {
			res.State = agent.LandingStateNotLanded
			res.Reason = "revision-mismatch"
		}
	}

	var staleUnknown *mapsvc.AnswerUnknown
	if maxAge > 0 {
		ts, ok := parseFleetTimestamp(res.ObservedAt)
		if !ok {
			res.Stale = true
			staleUnknown = &mapsvc.AnswerUnknown{
				What:   "cluster " + res.Cluster,
				Kind:   mapsvc.UnknownStale,
				Reason: "receipt missing valid observedAt timestamp",
			}
		} else if age := now.Sub(ts); age > maxAge {
			res.Stale = true
			staleUnknown = &mapsvc.AnswerUnknown{
				What:   "cluster " + res.Cluster,
				Kind:   mapsvc.UnknownStale,
				Reason: fmt.Sprintf("receipt age %s exceeds max-age %s", age.Truncate(time.Second), maxAge),
			}
		}
	}

	return res, staleUnknown
}

type extractedReceiptMetadata struct {
	Cluster    string
	ObservedAt string
	Target     string
	Revision   string
	State      string
	Reason     string
}

func extractFleetReceiptPayload(payload json.RawMessage, file, signedAt string) extractedReceiptMetadata {
	meta := extractedReceiptMetadata{
		Cluster:    clusterNameFromFilename(file),
		ObservedAt: strings.TrimSpace(signedAt),
		State:      agent.LandingStateNotObservable,
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		meta.Reason = "payload-not-object"
		return meta
	}

	if c := extractClusterIdentity(doc); c != "" {
		meta.Cluster = c
	}
	if ts := extractObservedAt(doc); ts != "" {
		meta.ObservedAt = ts
	}
	meta.Target = extractTargetRef(doc)
	meta.Revision = extractObservedRevision(doc)
	meta.State, meta.Reason = extractLandingState(doc)
	return meta
}

func extractClusterIdentity(doc map[string]interface{}) string {
	// 1. context.cluster or context (string)
	if ctxObj, ok := doc["context"].(map[string]interface{}); ok {
		if c := fleetStringField(ctxObj, "cluster"); c != "" {
			return c
		}
		if c := fleetStringField(ctxObj, "name"); c != "" {
			return c
		}
	}
	if c := fleetStringField(doc, "context"); c != "" {
		return c
	}

	// 2. answer.cluster.context
	if ansObj, ok := doc["answer"].(map[string]interface{}); ok {
		if clObj, ok := ansObj["cluster"].(map[string]interface{}); ok {
			if c := fleetStringField(clObj, "context"); c != "" {
				return c
			}
			if c := fleetStringField(clObj, "name"); c != "" {
				return c
			}
		}
	}

	// 3. top-level cluster (string or object)
	if c := fleetStringField(doc, "cluster"); c != "" {
		return c
	}
	if clObj, ok := doc["cluster"].(map[string]interface{}); ok {
		if c := fleetStringField(clObj, "name"); c != "" {
			return c
		}
		if c := fleetStringField(clObj, "context"); c != "" {
			return c
		}
	}

	// 4. receipt/v1 predicate.scope.cluster
	if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
		if scopeObj, ok := predObj["scope"].(map[string]interface{}); ok {
			if c := fleetStringField(scopeObj, "cluster"); c != "" {
				return c
			}
		}
	}

	// 5. subject (string or object with cluster)
	if s := fleetStringField(doc, "subject"); s != "" {
		return s
	}
	if subjObj, ok := doc["subject"].(map[string]interface{}); ok {
		if c := fleetStringField(subjObj, "cluster"); c != "" {
			return c
		}
	}

	return ""
}

func extractObservedAt(doc map[string]interface{}) string {
	if ansObj, ok := doc["answer"].(map[string]interface{}); ok {
		if ts := fleetStringField(ansObj, "observedAt"); ts != "" {
			return ts
		}
	}
	if ts := fleetStringField(doc, "observedAt"); ts != "" {
		return ts
	}
	if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
		if ts := fleetStringField(predObj, "verifiedAt"); ts != "" {
			return ts
		}
	}
	return ""
}

func extractTargetRef(doc map[string]interface{}) string {
	if t := fleetStringField(doc, "target"); t != "" {
		return t
	}
	if tObj, ok := doc["target"].(map[string]interface{}); ok {
		if ref := formatKindNamespaceName(fleetStringField(tObj, "kind"), fleetStringField(tObj, "namespace"), fleetStringField(tObj, "name")); ref != "" {
			return ref
		}
	}
	if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
		if scopeObj, ok := predObj["scope"].(map[string]interface{}); ok {
			if ref := formatKindNamespaceName(fleetStringField(scopeObj, "kind"), fleetStringField(scopeObj, "namespace"), fleetStringField(scopeObj, "name")); ref != "" {
				return ref
			}
		}
	}
	if subjs, ok := doc["subject"].([]interface{}); ok && len(subjs) > 0 {
		if first, ok := subjs[0].(map[string]interface{}); ok {
			if name := fleetStringField(first, "name"); name != "" {
				return name
			}
		}
	}
	return ""
}

func extractObservedRevision(doc map[string]interface{}) string {
	if anchorObj, ok := doc["anchor"].(map[string]interface{}); ok {
		if v := fleetStringField(anchorObj, "value"); v != "" {
			return v
		}
	}
	if rev := fleetStringField(doc, "revision"); rev != "" {
		return rev
	}
	if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
		if specObj, ok := predObj["spec"].(map[string]interface{}); ok {
			if anchorObj, ok := specObj["anchor"].(map[string]interface{}); ok {
				if rev := fleetStringField(anchorObj, "revision"); rev != "" {
					return rev
				}
			}
			if rev := fleetStringField(specObj, "revision"); rev != "" {
				return rev
			}
		}
	}
	return ""
}

func extractLandingState(doc map[string]interface{}) (string, string) {
	if ansObj, ok := doc["answer"].(map[string]interface{}); ok {
		if st := normalizeLandingState(fleetStringField(ansObj, "state")); st != "" {
			return st, fleetStringField(ansObj, "reason")
		}
	}
	if st := normalizeLandingState(fleetStringField(doc, "state")); st != "" {
		return st, fleetStringField(doc, "reason")
	}
	if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
		switch strings.ToUpper(fleetStringField(predObj, "verdict")) {
		case string(agent.VerdictPASS):
			return agent.LandingStateLanded, "receipt-verdict-pass"
		case string(agent.VerdictWATCH):
			return agent.LandingStateConverging, "receipt-verdict-watch"
		case string(agent.VerdictBLOCK):
			return agent.LandingStateNotLanded, "receipt-verdict-block"
		case string(agent.VerdictINCONCLUSIVE):
			return agent.LandingStateNotObservable, "receipt-verdict-inconclusive"
		}
	}
	return agent.LandingStateNotObservable, "state-missing"
}

func normalizeLandingState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case agent.LandingStateLanded:
		return agent.LandingStateLanded
	case agent.LandingStateConverging:
		return agent.LandingStateConverging
	case agent.LandingStateNotLanded:
		return agent.LandingStateNotLanded
	case agent.LandingStateNotObservable:
		return agent.LandingStateNotObservable
	default:
		return ""
	}
}

func fleetRevisionMatches(observed, expected string) bool {
	obs := strings.TrimSpace(observed)
	exp := strings.TrimSpace(expected)
	if obs == "" || exp == "" {
		return false
	}
	if obs == exp {
		return true
	}
	if strings.HasSuffix(obs, "@"+exp) || strings.HasSuffix(obs, "@sha1:"+exp) {
		return true
	}
	return false
}

func buildFleetReceiptAnswer(results []fleetClusterReceiptResult, counts fleetReceiptSummaryCounts, unknowns []mapsvc.AnswerUnknown, now time.Time) *mapsvc.Answer {
	var state, reason, summary string
	switch {
	case counts.InvalidReceipts > 0:
		state = agent.LandingStateNotLanded
		reason = "fleet-signature-invalid"
		summary = fmt.Sprintf("%d/%d receipts failed signature verification", counts.InvalidReceipts, counts.Total)
	case counts.RevisionMismatch > 0:
		state = agent.LandingStateNotLanded
		reason = "fleet-revision-mismatch"
		summary = fmt.Sprintf("%d/%d clusters did not match expected revision", counts.RevisionMismatch, counts.Total)
	case counts.NotLanded > 0:
		state = agent.LandingStateNotLanded
		reason = "fleet-not-landed"
		summary = fmt.Sprintf("%d/%d clusters landed; %d not-landed", counts.Landed, counts.Total, counts.NotLanded)
	case counts.MissingClusters > 0:
		state = agent.LandingStateNotObservable
		reason = "fleet-missing-clusters"
		summary = fmt.Sprintf("%d/%d clusters landed; %d required missing", counts.Landed, counts.Total, counts.MissingClusters)
	case counts.NotObservable > 0:
		state = agent.LandingStateNotObservable
		reason = "fleet-not-observable"
		summary = fmt.Sprintf("%d/%d clusters landed; %d not-observable", counts.Landed, counts.Total, counts.NotObservable)
	case counts.Converging > 0:
		state = agent.LandingStateConverging
		reason = "fleet-converging"
		summary = fmt.Sprintf("%d/%d clusters landed; %d converging", counts.Landed, counts.Total, counts.Converging)
	default:
		state = agent.LandingStateLanded
		reason = "fleet-landed"
		summary = fmt.Sprintf("%d/%d clusters verified and landed", counts.Landed, counts.Total)
	}

	if unknowns == nil {
		unknowns = []mapsvc.AnswerUnknown{}
	}
	// If any non-missing cluster reported not-observable without already being listed in unknowns,
	// add one compact unknown so the answer never hides unobservable clusters.
	if counts.NotObservable > counts.MissingClusters {
		var unobs []string
		for _, r := range results {
			if !r.Missing && r.State == agent.LandingStateNotObservable {
				unobs = append(unobs, r.Cluster)
			}
		}
		if len(unobs) > 0 {
			unknowns = append(unknowns, mapsvc.AnswerUnknown{
				What:   "clusters " + strings.Join(unobs, ","),
				Kind:   mapsvc.UnknownNotObservable,
				Reason: "cluster receipt reported not-observable",
			})
		}
	}

	next := []mapsvc.AnswerNext{}
	if state != agent.LandingStateLanded {
		next = append(next, mapsvc.AnswerNext{
			Command: preferInvocationForm("cub-scout receipt verify --input <file> --pubkey <pub.pem> --format json"),
			Why:     "inspect the failing cluster receipt offline",
		})
	}

	return &mapsvc.Answer{
		State:      state,
		Reason:     reason,
		Summary:    summary,
		ObservedAt: now.UTC().Format(time.RFC3339),
		Cluster:    mapsvc.AnswerCluster{Context: "fleet", IDSource: "not-read"},
		Basis: []mapsvc.AnswerBasis{
			{
				Claim:   fmt.Sprintf("%d/%d signed receipts verified", counts.ValidSignatures, counts.Total),
				RestsOn: mapsvc.RestsOnCubScoutComparison,
			},
		},
		Unknowns: unknowns,
		Next:     next,
		Cost:     mapsvc.AnswerCost{Coverage: "offline-signed-receipts"},
	}
}

func renderFleetReceiptOutput(w io.Writer, out fleetReceiptVerificationOutput, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	}

	if _, err := fmt.Fprintf(w, "FLEET VERIFY: %s (%s)\n  Summary: %s\n", out.Answer.State, out.Answer.Reason, out.Answer.Summary); err != nil {
		return err
	}
	if out.ExpectedRevision != "" {
		if _, err := fmt.Fprintf(w, "  Revision: %s\n", out.ExpectedRevision); err != nil {
			return err
		}
	}
	for _, c := range out.Clusters {
		sigStatus := "sig=ok"
		if c.Missing {
			sigStatus = "missing"
		} else if !c.Valid {
			sigStatus = "sig=INVALID"
		}
		extra := ""
		if c.Revision != "" {
			extra += " rev=" + c.Revision
		}
		if c.Stale {
			extra += " [STALE]"
		}
		if c.Error != "" {
			extra += " err=" + c.Error
		}
		if _, err := fmt.Fprintf(w, "  - %-16s state=%-14s %s%s\n", c.Cluster, c.State, sigStatus, extra); err != nil {
			return err
		}
	}
	return nil
}

func clusterNameFromFilename(path string) string {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	for _, suffix := range []string{".signed.json", ".receipt.json", ".json"} {
		if strings.HasSuffix(lower, suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

func fleetStringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func formatKindNamespaceName(kind, namespace, name string) string {
	if kind == "" && name == "" {
		return ""
	}
	if namespace != "" {
		return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
	}
	return fmt.Sprintf("%s/%s", kind, name)
}

func parseFleetTimestamp(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return ts.UTC(), true
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.UTC(), true
	}
	return time.Time{}, false
}
