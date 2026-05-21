// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
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
	receiptNamespace string
	receiptPredicate string
	receiptAtCommit  string
	receiptFormat    string
	receiptOut       string
)

// Function-variable seam for tests. Production reads from the cluster via
// the dynamic client; tests swap with a fake returning prefab objects.
var loadReceiptLiveFn = loadReceiptLive

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
about a live Kubernetes resource. v1 batch 1 supports the applied-matches-spec
predicate (LIVE matches the controller-resolved git anchor).

Subject is a positional argument in the form <kind>/<name>.

Predicate auto-detection priority (when --predicate is not passed):
  1. Argo / Flux / ConfigHub-via-GitOps owner → applied-matches-spec
  2. Otherwise → INCONCLUSIVE + omission (no auto-detected predicate)

Examples:
  cub-scout receipt verify deploy/api -n prod
  cub-scout receipt verify deploy/api -n prod --at-commit abc123
  cub-scout receipt verify deploy/api -n prod --predicate applied-matches-spec --format json --out api.receipt.json
`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptVerify,
}

func init() {
	rootCmd.AddCommand(receiptCmd)
	receiptCmd.AddCommand(receiptVerifyCmd)

	receiptVerifyCmd.Flags().StringVarP(&receiptNamespace, "namespace", "n", "", "Namespace of the resource (required for namespaced kinds)")
	receiptVerifyCmd.Flags().StringVar(&receiptPredicate, "predicate", "", "Predicate to evaluate (default: auto-detect). v1 batch 1 supports applied-matches-spec only.")
	receiptVerifyCmd.Flags().StringVar(&receiptAtCommit, "at-commit", "", "Override the spec anchor revision (Git SHA). When empty, the controller-resolved anchor is used as both the spec and the evidence.")
	receiptVerifyCmd.Flags().StringVar(&receiptFormat, "format", "ascii", "Output format: ascii | json")
	receiptVerifyCmd.Flags().StringVar(&receiptOut, "out", "", "Write the receipt to this file path (also printed to stdout in the chosen format)")
}

func runReceiptVerify(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
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
	owner := agent.DetectOwnership(live)
	attribution := agent.AttributeFieldMutation(live, owner)
	gitSource := agent.CollectGitSourceAnchor(ctx, live)

	evidence := agent.Evidence{
		Attribution: &attribution,
		GitSource:   gitSource,
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
		PredicateName: agent.PredicateName(strings.TrimSpace(receiptPredicate)),
		Spec:          spec,
		Evidence:      evidence,
		Connected:     connected,
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
	if strings.TrimSpace(receiptOut) != "" {
		// Always write the JSON form to disk regardless of console format
		// — disk is the long-lived artifact; ASCII is for humans.
		jsonBytes, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt for --out: %w", mErr)
		}
		if writeErr := os.WriteFile(receiptOut, append(jsonBytes, '\n'), 0o644); writeErr != nil {
			return fmt.Errorf("write receipt to %s: %w", receiptOut, writeErr)
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
