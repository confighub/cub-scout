// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// receipt flags
var (
	receiptNamespace         string
	receiptPredicate         string
	receiptAtCommit          string
	receiptStrategy          string
	receiptSince             string
	receiptFormat            string
	receiptOut               string
	receiptSave              bool
	receiptSaveDir           string
	receiptFailOn            string
	receiptInputAttestations []string
	receiptGraceWindow       string
	receiptPrerequisitesFile string
	receiptTTL               string
	receiptTTLDur            time.Duration
	receiptReferenceEvidence []string
	receiptNoExtras          bool
	receiptNormalizationProf string
)

// runReceiptVerifyDispatch is the shared entry point for the receipt
// verify command. It branches between the single-resource flow
// (existing runReceiptVerify) and the aggregate-with-discovery flow
// (#448, runReceiptVerifyScoped in receipt_aggregate.go) based on
// whether --scope is set or the positional argument is comma-shaped.
func runReceiptVerifyDispatch(cmd *cobra.Command, args []string) error {
	// Parse --ttl UPFRONT (before any routing or side effect) so a bad value
	// rejects cleanly; every receipt path reads receiptTTLDur.
	ttl, ttlErr := parseReceiptTTL()
	if ttlErr != nil {
		return ttlErr
	}
	receiptTTLDur = ttl

	// Detect aggregate mode by inspecting --scope OR the positional.
	scopeFlag := strings.TrimSpace(receiptScope)
	positional := ""
	if len(args) > 0 {
		positional = strings.TrimSpace(args[0])
	}

	if strings.TrimSpace(receiptPrerequisitesFile) != "" || agent.PredicateName(strings.TrimSpace(receiptPredicate)) == agent.PredicatePrerequisitesMet {
		return runReceiptVerifyPrerequisites(cmd, args)
	}

	if strings.TrimSpace(receiptObjectSetFile) != "" {
		if agent.PredicateName(strings.TrimSpace(receiptPredicate)) == agent.PredicateWorkloadsConverged {
			return runReceiptVerifyWorkloadsConverged(cmd, args)
		}
		return runReceiptVerifyObjectSet(cmd, args)
	}

	if scopeFlag != "" || strings.Contains(positional, ",") {
		spec, refs, isAgg, err := parseAggregateScope(scopeFlag, positional, receiptNamespace)
		if err != nil {
			return err
		}
		if isAgg {
			return runReceiptVerifyScoped(cmd, spec, refs)
		}
	}

	// Fall back to the single-resource flow. Requires exactly one
	// positional.
	if len(args) != 1 {
		return fmt.Errorf("receipt verify requires either a single <kind>/<name> positional, a comma-list positional, or --scope namespace/<ns>")
	}
	return runReceiptVerify(cmd, args)
}

// Function-variable seam for tests. Production reads from the cluster via
// the dynamic client; tests swap with a fake returning prefab objects.
var loadReceiptLiveFn = loadReceiptLive

// collectSourceTruthForReceiptFn is the function-variable seam used by the
// source-truth-pass predicate path. Production wires it to the live
// source-truth derivation; tests swap with a fake returning prefab
// evidence so the receipt CLI integration tests do not require a real
// ConfigHub or cluster.
var collectSourceTruthForReceiptFn = collectSourceTruthForReceipt

var receiptCmd = &cobra.Command{
	Use:   "receipt",
	Short: "Create and verify cub-scout evidence receipts (#446)",
	Long: `receipt produces typed, fingerprinted, immutable artifacts wrapping
cub-scout's existing field-level evidence (compareThreeWay, attribution,
sourceTruth, gitSource) into a verifiable record that downstream consumers
(CI/CD, audit, postmortem, acceptance-judge tooling) can attach to a decision
and later prove the inputs were what they claim to be.

Receipts are HISTORICAL, IMMUTABLE records of past events. Updates produce
new receipts, never mutate old ones. cub-scout never mutates the cluster
or ConfigHub.

Current shipping releases use fingerprint-only integrity (SHA-256 over RFC 8785
canonical JSON of the full in-toto Statement v1 envelope minus only
predicate.fingerprint). Cryptographic signing (e.g., DSSE wrapped in a Sigstore
Bundle, or a comparable scheme) is a future hardening direction — purely
additive to the wire format, no envelope change required.`,
}

var receiptVerifyCmd = &cobra.Command{
	Use:   "verify <subject>",
	Short: "Build a receipt verifying a property of a Kubernetes resource",
	Long: `verify produces an in-toto Statement v1 receipt asserting a predicate
about a live Kubernetes resource.

v1 predicates:
  applied-matches-spec    LIVE matches the controller-resolved git anchor
  source-truth-pass       compare source-truth returned PASS under --strategy
  no-manual-edits-since   no kubectl-* writer touched managedFields after --since
  object-set-matches      rendered manifest objects are present live and authored fields match
  workloads-converged     desired workloads reached a ready/converged runtime state (reads status)
  prerequisites-met       declared cluster facts (CRDs/Secrets/StorageClasses/namespaces) are present live

Subject is usually a positional argument in the form <kind>/<name>.
For install/object-set receipts, pass --file <manifest.yaml|dir> and scope
with --scope namespace/<ns>, --scope cluster, or -n <ns>.

Predicate auto-detection priority (when --predicate is not passed):
  1. Argo / Flux / ConfigHub-via-GitOps owner WITH a resolvable git anchor
     → applied-matches-spec
  2. --strategy provided → source-truth-pass
  3. --since provided    → no-manual-edits-since
  4. Otherwise → INCONCLUSIVE + omission (no auto-detected predicate)

Examples:
  cub-scout receipt verify deploy/api -n prod
  cub-scout receipt verify deploy/api -n prod --at-commit abc123
  cub-scout receipt verify deploy/api -n prod --strategy git-argo
  cub-scout receipt verify deploy/api -n prod --since 2026-05-22T00:00:00Z
  cub-scout receipt verify deploy/api -n prod --predicate applied-matches-spec --format json --out api.receipt.json
  cub-scout receipt verify --file out/manifests --scope namespace/redis --format json --out install.receipt.json
  cub-scout receipt verify --file out/manifests --scope namespace/redis --predicate workloads-converged --fail-on any-non-pass
  cub-scout receipt verify --prerequisites prereqs.yaml --scope namespace/redis --fail-on any-non-pass
  cub-scout receipt verify --file out/manifests --scope namespace/redis --reference-evidence upstream-package.receipt.json
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReceiptVerifyDispatch,
}

func init() {
	rootCmd.AddCommand(receiptCmd)
	receiptCmd.AddCommand(receiptVerifyCmd)

	receiptVerifyCmd.Flags().StringVarP(&receiptNamespace, "namespace", "n", "", "Namespace of the resource (required for namespaced kinds)")
	receiptVerifyCmd.Flags().StringVar(&receiptPredicate, "predicate", "", "Predicate to evaluate (default: auto-detect). v1 supports applied-matches-spec, source-truth-pass, no-manual-edits-since, object-set-matches, workloads-converged, prerequisites-met.")
	receiptVerifyCmd.Flags().StringVar(&receiptObjectSetFile, "file", "", "YAML file or directory containing rendered desired objects for object-set-matches / workloads-converged install receipts.")
	receiptVerifyCmd.Flags().StringVar(&receiptGraceWindow, "grace-window", "", "For --predicate workloads-converged: how long a workload may stay InProgress before it counts as failed (BLOCK) rather than progressing (WATCH), e.g. 5m. Empty means no deadline (InProgress is WATCH).")
	receiptVerifyCmd.Flags().StringVar(&receiptPrerequisitesFile, "prerequisites", "", "YAML/JSON file declaring required cluster facts (requiredCRDs, requiredSecrets, requiredNamespaces, requiredStorageClasses, requiredIngressClasses) for a prerequisites-met receipt.")
	receiptVerifyCmd.Flags().StringVar(&receiptTTL, "ttl", "", "Observation-freshness boundary, e.g. 1h. When set, the receipt records an immutable freshness{observedAt,expiresAt,ttl} so a consumer can tell a fresh receipt from a stale one. Empty means the receipt makes no freshness claim.")
	receiptVerifyCmd.Flags().BoolVar(&receiptNoExtras, "no-extras", false, "For --predicate object-set-matches: also run the closed-world check — flag live objects of the rendered kinds in scope that are not in the desired set (resolves the extra-live-object-coverage omission). Extras downgrade a clean PASS to WATCH. Skips owner-referenced children and system objects.")
	receiptVerifyCmd.Flags().StringVar(&receiptNormalizationProf, "normalization-profile", "", "Named server-normalization profile applied symmetrically to desired and live objects before object-set comparison and digesting (e.g. k8s-zero-defaults/v1). Recorded on the receipt. Empty means raw comparison.")
	receiptVerifyCmd.Flags().StringVar(&receiptAtCommit, "at-commit", "", "Override the spec anchor revision (Git SHA). When empty, the controller-resolved anchor is used as both the spec and the evidence.")
	receiptVerifyCmd.Flags().StringVar(&receiptStrategy, "strategy", "", "Source-truth strategy for source-truth-pass (e.g. git-argo). cub-scout does not infer the strategy.")
	receiptVerifyCmd.Flags().StringVar(&receiptSince, "since", "", "RFC 3339 cutoff for no-manual-edits-since (e.g. 2026-05-22T00:00:00Z).")
	receiptVerifyCmd.Flags().StringVar(&receiptFormat, "format", "ascii", "Output format: ascii | json")
	receiptVerifyCmd.Flags().StringVar(&receiptOut, "out", "", "Write the receipt to this file path (also printed to stdout in the chosen format)")
	receiptVerifyCmd.Flags().BoolVar(&receiptSave, "save", false, "Save the receipt to the local store ($CUB_SCOUT_RECEIPTS_DIR or $XDG_DATA_HOME/cub-scout/receipts) for later cub-scout receipt list / show / validate")
	receiptVerifyCmd.Flags().StringVar(&receiptSaveDir, "save-dir", "", "Override the store directory used when --save is set")
	receiptVerifyCmd.Flags().StringVar(&receiptFailOn, "fail-on", "", "Exit non-zero (code 2) when the receipt verdict matches. Accepts a comma-separated list of verdicts (WATCH, BLOCK, INCONCLUSIVE) or the sugar 'any-non-pass' (= WATCH,BLOCK,INCONCLUSIVE). The receipt is still printed / saved / written to --out regardless of exit code.")
	receiptVerifyCmd.Flags().StringArrayVar(&receiptInputAttestations, "input-attestation", nil, "Path to a prior receipt to reference via inputAttestations[] (repeatable). Each referenced receipt's fingerprint is verified at chain-construction time; tampered receipts are refused. The new receipt's fingerprint covers the inputAttestations[] field by construction.")
	receiptVerifyCmd.Flags().StringArrayVar(&receiptReferenceEvidence, "reference-evidence", nil, "Path to an EXTERNAL (non-cub-scout) evidence artifact to reference via inputAttestations[] by content digest (repeatable). Recorded under the external-evidence:// scheme with digest-asserted (not fingerprint-verified) trust — lets an upstream producer's artifact (e.g. a render/package receipt) enter the chain without cub-scout understanding its format (#482).")
	receiptVerifyCmd.Flags().StringVar(&receiptScope, "scope", "", "Scope selector. Without --file: aggregate-receipt scope (#448), accepting 'namespace/<ns>' or a comma-list positional. With --file: object-set scope, accepting 'namespace/<ns>' or 'cluster'.")
	receiptVerifyCmd.Flags().StringVar(&receiptAggregatePolicy, "aggregate-policy", "", "Verdict-synthesis policy for the aggregate receipt (#448). v1 supports only 'max-severity' (the default; BLOCK > INCONCLUSIVE > WATCH > PASS).")

	// Wire the production connected-mode detector for the aggregate
	// flow. (Lives in receipt_aggregate.go as a function-variable seam
	// so tests can swap to a fixed standalone/connected value without
	// importing the hub package.)
	detectConnectedForReceipt = func() bool {
		return hub.NewClient().RequireConnected() == nil
	}
}

func runReceiptVerify(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
	}

	// Parse --since (RFC 3339). Empty means "not provided".
	var since time.Time
	if raw := strings.TrimSpace(receiptSince); raw != "" {
		parsed, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			return fmt.Errorf("invalid --since %q (expected RFC 3339, e.g. 2026-05-22T00:00:00Z): %w", raw, perr)
		}
		since = parsed
	}

	// Reject contradictory predicate+signal pairs upfront. cub-scout
	// won't silently demote an explicit --predicate to "INCONCLUSIVE
	// because you forgot the matching signal" — the CLI tells the user
	// to fix it.
	predicateExplicit := agent.PredicateName(strings.TrimSpace(receiptPredicate))
	if predicateExplicit == agent.PredicateSourceTruthPass && strings.TrimSpace(receiptStrategy) == "" {
		return fmt.Errorf("--predicate source-truth-pass requires --strategy (cub-scout does not infer the delivery strategy)")
	}
	if predicateExplicit == agent.PredicateNoManualEditsSince && since.IsZero() {
		return fmt.Errorf("--predicate no-manual-edits-since requires --since <RFC3339 timestamp>")
	}

	// Parse --fail-on UPFRONT so a typo or invalid value rejects the
	// command before any side effect (stdout print, --out write, --save
	// to store). The fail-on check itself fires later (after the
	// receipt is built and the artifact is persisted), but the parse
	// must not let the artifact escape on a misconfigured gate.
	//
	// Codex round-6 P2 fix (#463): the prior order let an invalid
	// --fail-on value print the receipt, write --out, and save before
	// returning the parse error — which is a CI footgun (the gate
	// "fires" with the wrong exit code because the typo error wraps to
	// 1, not 2, and the artifact is already on disk).
	var failOnSet map[agent.ReceiptVerdict]bool
	failOnRaw := strings.TrimSpace(receiptFailOn)
	if failOnRaw != "" {
		parsed, parseErr := parseReceiptFailOn(failOnRaw)
		if parseErr != nil {
			return parseErr
		}
		failOnSet = parsed
	}

	kindRaw, name, ok := parseKindName(args[0])
	if !ok {
		return fmt.Errorf("invalid subject %q (expected <kind>/<name>, e.g. deploy/api)", args[0])
	}
	kind := normalizeKind(kindRaw)
	if kind == "" {
		return fmt.Errorf("invalid subject %q (unsupported kind)", args[0])
	}
	ns := strings.TrimSpace(receiptNamespace)
	if ns == "" {
		ns = "default"
	}

	ctx := cmd.Context()

	// 1. Load the live object (read-only).
	live, err := loadReceiptLiveFn(ctx, kind, name, ns)
	if err != nil {
		return fmt.Errorf("load live snapshot for %s/%s in %s: %w", kind, name, ns, err)
	}

	// 2. Build the evidence body from existing cub-scout helpers. All
	// read-only.
	//
	// Note (Codex #446 round-4 fix): we use CollectGitSourceAnchorForOwner
	// so the Argo tracer is invoked against the resolved Application
	// rather than the workload kind. CollectGitSourceAnchor's non-
	// owner-aware path would call ArgoTracer.Trace(kind=Deployment),
	// which the tracer rejects, leaving gitSource=nil for every
	// Argo-owned Deployment receipt.
	owner := agent.DetectOwnership(live)
	attribution := agent.AttributeFieldMutation(live, owner)
	gitSource := agent.CollectGitSourceAnchorForOwner(ctx, live, owner)

	evidence := agent.Evidence{
		Attribution: &attribution,
		GitSource:   gitSource,
	}

	// source-truth evidence is populated only when the caller signaled
	// the source-truth-pass predicate (explicitly or via --strategy
	// auto-detect). The runner refuses to silently derive source-truth
	// for any receipt — that would attach evidence the consumer may not
	// have asked for. Source-truth requires connected mode, so this
	// path is connected-only by definition.
	if strategy := strings.TrimSpace(receiptStrategy); strategy != "" {
		stEvidence, stErr := collectSourceTruthForReceiptFn(ctx, kind, name, ns, strategy)
		if stErr != nil {
			return fmt.Errorf("collect source-truth evidence: %w", stErr)
		}
		evidence.SourceTruth = stEvidence
	}

	// 3. Build the spec anchor. If --at-commit is set, override the
	// revision; otherwise use the controller-resolved anchor as both
	// the spec and the evidence (the anchor-match check is then trivially
	// true and the predicate's cause check decides the verdict).
	var spec *agent.SpecAnchor
	if gitSource != nil {
		spec = &agent.SpecAnchor{
			Anchor: agent.SpecAnchorBody{
				Type:     "git",
				RepoURL:  gitSource.RepoURL,
				Revision: gitSource.Revision,
				Path:     gitSource.Path,
			},
		}
		if strings.TrimSpace(receiptAtCommit) != "" {
			spec.Anchor.Revision = strings.TrimSpace(receiptAtCommit)
		}
	}

	// 4. Detect connected mode (for the second subject + omission logic).
	connected := hub.NewClient().RequireConnected() == nil

	// 4b. Build the inputAttestations[] from --input-attestation
	// (repeatable). Each path is loaded + fingerprint-verified before
	// being chained — a tampered referenced receipt fails the chain
	// construction upfront rather than silently propagating bad
	// evidence. The returned wrappers are the API-boundary-checked type
	// (`VerifiedAttestationRef`); BuildReceipt unwraps them when it
	// populates the wire shape. See pkg/agent/receipt_inputattestations.go.
	inputAttestations, iaErr := collectReceiptInputAttestations()
	if iaErr != nil {
		return iaErr
	}

	// 5. Build the receipt.
	stmt, err := agent.BuildReceipt(agent.BuildReceiptInput{
		Live: live,
		Scope: agent.Scope{
			Kind:      kind,
			Name:      name,
			Namespace: ns,
		},
		Owner:             owner,
		PredicateName:     predicateExplicit,
		Spec:              spec,
		Evidence:          evidence,
		Connected:         connected,
		Strategy:          strings.TrimSpace(receiptStrategy),
		Since:             since,
		InputAttestations: inputAttestations,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("build receipt: %w", err)
	}

	// 5b. Stamp observation freshness (#478) when --ttl was passed.
	if err := agent.ApplyFreshness(&stmt, receiptTTLDur); err != nil {
		return fmt.Errorf("apply freshness: %w", err)
	}

	// 6. Serialize to bytes.
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

	// 7. Print to stdout AND optionally write to file.
	fmt.Println(string(out))
	if outPath := strings.TrimSpace(receiptOut); outPath != "" {
		// Always write the JSON form to disk regardless of console format
		// — disk is the long-lived artifact; ASCII is for humans.
		jsonBytes, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt for --out: %w", mErr)
		}
		if writeErr := writeReceiptOutFile(outPath, jsonBytes); writeErr != nil {
			return writeErr
		}
	}

	if receiptSave {
		// Save into the local store. Resolve dir from --save-dir or the
		// default (XDG / HOME). agent.SaveStatement is immutable —
		// duplicate filenames produce ErrExist. We catch that and emit
		// an informational message so a repeated verify on the same
		// resource at the same instant doesn't fail the command; the
		// receipt is already on disk and identical by construction.
		dir := strings.TrimSpace(receiptSaveDir)
		if dir == "" {
			resolved, dErr := agent.DefaultStoreDir()
			if dErr != nil {
				return fmt.Errorf("resolve receipt store: %w", dErr)
			}
			dir = resolved
		}
		savedPath, sErr := agent.SaveStatement(stmt, dir)
		if sErr != nil {
			if errors.Is(sErr, os.ErrExist) {
				fmt.Fprintf(cmd.ErrOrStderr(), "already saved: %s\n", savedPath)
			} else {
				return fmt.Errorf("save receipt: %w", sErr)
			}
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "saved: %s\n", savedPath)
		}
	}

	// --fail-on: exit code 2 if the receipt verdict matches one of the
	// listed verdicts (or the 'any-non-pass' sugar). This is the
	// CI-gate hook; the receipt has ALREADY been printed / saved /
	// written to --out above, so a failing gate still leaves the
	// audit artifact in place. The exitCodeError wrapping (per
	// cmd/cub-scout/exit_code.go + main.go's errors.As dispatcher)
	// drives the actual os.Exit(2).
	//
	// The fail-on set was parsed upfront (above) so an invalid value
	// could never produce side effects. This block only consults the
	// already-validated set.
	if failOnSet != nil && failOnSet[stmt.Predicate.Verdict] {
		return newExitCodeError(
			fmt.Errorf(
				"receipt verdict %q matches --fail-on %q",
				stmt.Predicate.Verdict, failOnRaw,
			),
			2,
		)
	}

	return nil
}

// parseReceiptFailOn parses the --fail-on flag value into the set of
// verdicts that should trigger exit code 2. Accepts:
//
//   - "any-non-pass" — sugar for WATCH,BLOCK,INCONCLUSIVE
//   - comma-separated list of WATCH / BLOCK / INCONCLUSIVE (case-insensitive)
//
// PASS is not a valid --fail-on value (gating on success doesn't make
// sense). The function rejects unknown verdicts upfront with a clear
// error rather than silently accepting them — this is a CI gate, the
// CI shouldn't see a typo become a "never fires" gate.
func parseReceiptFailOn(raw string) (map[agent.ReceiptVerdict]bool, error) {
	out := map[agent.ReceiptVerdict]bool{}
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "any-non-pass" {
		out[agent.VerdictWATCH] = true
		out[agent.VerdictBLOCK] = true
		out[agent.VerdictINCONCLUSIVE] = true
		return out, nil
	}

	for _, part := range strings.Split(raw, ",") {
		v := strings.ToUpper(strings.TrimSpace(part))
		if v == "" {
			continue
		}
		switch agent.ReceiptVerdict(v) {
		case agent.VerdictWATCH, agent.VerdictBLOCK, agent.VerdictINCONCLUSIVE:
			out[agent.ReceiptVerdict(v)] = true
		case agent.VerdictPASS:
			return nil, fmt.Errorf(
				"--fail-on PASS is not allowed (gating on a passing receipt is a no-op; if you want every receipt to fail, that's a workflow bug)",
			)
		default:
			return nil, fmt.Errorf(
				"--fail-on %q: unknown verdict %q (valid: WATCH, BLOCK, INCONCLUSIVE, or the sugar 'any-non-pass')",
				raw, v,
			)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("--fail-on %q resolved to empty verdict set", raw)
	}
	return out, nil
}

// parseReceiptTTL parses the --ttl flag into a duration. Empty means "no
// freshness boundary" (0). Negative or unparseable values are rejected.
func parseReceiptTTL() (time.Duration, error) {
	raw := strings.TrimSpace(receiptTTL)
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

// collectReceiptInputAttestations builds the combined inputAttestations[] from
// both --input-attestation (cub-scout receipts, fingerprint-verified) and
// --reference-evidence (external artifacts, digest-asserted, #482). The two
// reference kinds share the field but are distinguished by URI scheme.
func collectReceiptInputAttestations() ([]agent.VerifiedAttestationRef, error) {
	var refs []agent.VerifiedAttestationRef
	if len(receiptInputAttestations) > 0 {
		r, err := agent.BuildAttestationRefsFromPaths(receiptInputAttestations, nil)
		if err != nil {
			return nil, fmt.Errorf("build input-attestations: %w", err)
		}
		refs = append(refs, r...)
	}
	if len(receiptReferenceEvidence) > 0 {
		r, err := agent.BuildExternalAttestationRefsFromPaths(receiptReferenceEvidence, nil)
		if err != nil {
			return nil, fmt.Errorf("build reference-evidence: %w", err)
		}
		refs = append(refs, r...)
	}
	return refs, nil
}

// loadReceiptLive fetches the live K8s object via the dynamic client. The
// function-variable seam (loadReceiptLiveFn) above lets tests inject fakes.
func loadReceiptLive(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unsupported resource kind %q for receipt verify", kind)
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// parseKindName lives in navigation_hints.go; we reuse its (kind, name, ok)
// signature here.

// collectSourceTruthForReceipt runs the same three-surface derivation
// that `cub-scout compare source-truth` runs and returns the structured
// evidence body. Read-only by construction: every helper here is a Get
// against the cluster / ConfigHub / controller tracers; nothing writes.
//
// Errors are returned only when the caller's input is bad (unknown
// strategy, runtime fetch failure with no graceful fallback). Soft
// failures — missing ConfigHub linkage, controller CLI absent — are
// converted into nil surfaces and surface through Derive as BLOCK /
// WATCH / proof_gaps. The receipt orchestrator then mirrors those
// proof_gaps into omissions[].
func collectSourceTruthForReceipt(ctx context.Context, kind, name, namespace, strategyStr string) (*agent.SourceTruthEvidence, error) {
	strategy, ok := agent.ParseStrategy(strategyStr)
	if !ok {
		return nil, fmt.Errorf("unknown source-truth strategy %q (valid: %v)", strategyStr, agent.AllStrategies())
	}

	runtime, runtimeObj, runtimeErr := collectRuntimeSurface(ctx, kind, name, namespace)
	if runtimeErr != nil {
		// Runtime unfetchable → BLOCKED evidence (the same shape
		// `compare source-truth` produces in this case).
		ev := agent.Derive(strategy, agent.SourceTruthSurfaces{})
		return &ev, nil
	}

	chSurface, _ := collectConfigHubSurface(ctx, runtimeObj)
	ctrlSurface := collectControllerSurface(ctx, strategy, kind, name, namespace)

	ev := agent.Derive(strategy, agent.SourceTruthSurfaces{
		ConfigHub:  chSurface,
		Controller: ctrlSurface,
		Runtime:    runtime,
	})
	return &ev, nil
}
