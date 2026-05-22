// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	receiptNamespace string
	receiptPredicate string
	receiptAtCommit  string
	receiptStrategy  string
	receiptSince     string
	receiptFormat    string
	receiptOut       string
	receiptSave      bool
	receiptSaveDir   string
)

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

v1 ships fingerprint-only (SHA-256 over RFC 8785 canonical JSON of the full
in-toto Statement v1 envelope minus only predicate.fingerprint). v2 will
add DSSE signing wrapped in Sigstore Bundle v0.3.`,
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

Subject is a positional argument in the form <kind>/<name>.

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
`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptVerify,
}

func init() {
	rootCmd.AddCommand(receiptCmd)
	receiptCmd.AddCommand(receiptVerifyCmd)

	receiptVerifyCmd.Flags().StringVarP(&receiptNamespace, "namespace", "n", "", "Namespace of the resource (required for namespaced kinds)")
	receiptVerifyCmd.Flags().StringVar(&receiptPredicate, "predicate", "", "Predicate to evaluate (default: auto-detect). v1 supports applied-matches-spec, source-truth-pass, no-manual-edits-since.")
	receiptVerifyCmd.Flags().StringVar(&receiptAtCommit, "at-commit", "", "Override the spec anchor revision (Git SHA). When empty, the controller-resolved anchor is used as both the spec and the evidence.")
	receiptVerifyCmd.Flags().StringVar(&receiptStrategy, "strategy", "", "Source-truth strategy for source-truth-pass (e.g. git-argo). cub-scout does not infer the strategy.")
	receiptVerifyCmd.Flags().StringVar(&receiptSince, "since", "", "RFC 3339 cutoff for no-manual-edits-since (e.g. 2026-05-22T00:00:00Z).")
	receiptVerifyCmd.Flags().StringVar(&receiptFormat, "format", "ascii", "Output format: ascii | json")
	receiptVerifyCmd.Flags().StringVar(&receiptOut, "out", "", "Write the receipt to this file path (also printed to stdout in the chosen format)")
	receiptVerifyCmd.Flags().BoolVar(&receiptSave, "save", false, "Save the receipt to the local store ($CUB_SCOUT_RECEIPTS_DIR or $XDG_DATA_HOME/cub-scout/receipts) for later cub-scout receipt list / show / validate")
	receiptVerifyCmd.Flags().StringVar(&receiptSaveDir, "save-dir", "", "Override the store directory used when --save is set")
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

	// 5. Build the receipt.
	stmt, err := agent.BuildReceipt(agent.BuildReceiptInput{
		Live: live,
		Scope: agent.Scope{
			Kind:      kind,
			Name:      name,
			Namespace: ns,
		},
		Owner:         owner,
		PredicateName: predicateExplicit,
		Spec:          spec,
		Evidence:      evidence,
		Connected:     connected,
		Strategy:      strings.TrimSpace(receiptStrategy),
		Since:         since,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("build receipt: %w", err)
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
		// --out is the explicit-path path: the caller picks the filename
		// and gets exactly one write. It is NOT immutable — re-running
		// the same `verify --out foo.json` overwrites foo.json. That's
		// the documented contract for --out.
		//
		// Codex round 5: reject --out paths under the resolved store
		// directory so the immutable-store invariant can't be bypassed.
		// Concretely: a caller who points --out at
		// $XDG_DATA_HOME/cub-scout/receipts/foo.json could otherwise
		// stomp on an existing canonical receipt and the store's "files
		// never change once written" property would be broken.
		storeDir, sdErr := agent.DefaultStoreDir()
		if sdErr == nil {
			absOut, aErr := filepath.Abs(outPath)
			absStore, asErr := filepath.Abs(storeDir)
			if aErr == nil && asErr == nil {
				if rel, rErr := filepath.Rel(absStore, absOut); rErr == nil &&
					!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
					return fmt.Errorf(
						"refusing to write --out to %q because it is under the receipt store %q; "+
							"use --save (immutable canonical store path) for store writes, or pick a path outside the store for ad-hoc --out files",
						absOut, absStore,
					)
				}
			}
		}

		// Always write the JSON form to disk regardless of console format
		// — disk is the long-lived artifact; ASCII is for humans.
		jsonBytes, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt for --out: %w", mErr)
		}
		if writeErr := os.WriteFile(outPath, append(jsonBytes, '\n'), 0o644); writeErr != nil {
			return fmt.Errorf("write receipt to %s: %w", outPath, writeErr)
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

	return nil
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
