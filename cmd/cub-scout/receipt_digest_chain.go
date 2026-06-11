// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
)

// receipt digest — the producer half of the cross-tool digest convention.
// Computes the canonical rendered-object-set digest for a manifest set,
// byte-identical to the desiredDigest an object-set-matches receipt records
// for the same inputs and normalization profile. External producers (e.g.
// helm-expt render receipts) call this instead of reimplementing the
// canonicalization, which is what makes digest-equality chaining possible.
var receiptDigestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Compute the canonical rendered-object-set digest for a manifest set",
	Long: `digest computes the canonical digest of a rendered object set: SHA-256 over
RFC 8785 canonical JSON of the sorted (id, comparable-object) entries, after
optional --normalization-profile normalization.

The value is byte-identical to the desiredDigest (and rendered-object-set://
subject digest) that 'receipt verify --file' records for the same inputs and
profile. Producers stamp it on their own artifacts; verifiers chain by digest
equality.`,
	Args: cobra.NoArgs,
	RunE: runReceiptDigest,
}

var receiptChainCmd = &cobra.Command{
	Use:   "chain",
	Short: "Walk and verify a receipt's attestation chain",
	Long: `chain loads a leaf receipt, verifies its fingerprint, then resolves every
inputAttestations[] entry against --evidence-dir:

  cub-scout-receipt://  resolved by recomputing fingerprints of candidate
                        receipts in the directory; resolved receipts are
                        verified and walked recursively
  external-evidence://  resolved by content SHA-256 over the files in the
                        directory; digest-asserted (the trust label is
                        printed, not upgraded)

Exit 0 only when the leaf and every reachable link verify. Any unresolved or
tampered link exits 2. Subject digests are printed so producers can check
rendered-object-set digest equality across tools.`,
	Args: cobra.NoArgs,
	RunE: runReceiptChain,
}

var (
	digestFile        string
	digestProfile     string
	chainFile         string
	chainEvidenceDir  string
	chainPrintMaxDeep = 16
)

func init() {
	receiptCmd.AddCommand(receiptDigestCmd)
	receiptDigestCmd.Flags().StringVar(&digestFile, "file", "", "YAML file or directory containing the rendered object set (required)")
	receiptDigestCmd.Flags().StringVar(&digestProfile, "normalization-profile", "", "Named server-normalization profile (e.g. k8s-zero-defaults/v1); must match the verifier's profile for digests to chain")

	receiptCmd.AddCommand(receiptChainCmd)
	receiptChainCmd.Flags().StringVar(&chainFile, "file", "", "Leaf receipt JSON to start from (required)")
	receiptChainCmd.Flags().StringVar(&chainEvidenceDir, "evidence-dir", "", "Directory holding the referenced receipts and external artifacts (required)")
}

func runReceiptDigest(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(digestFile) == "" {
		return fmt.Errorf("--file is required")
	}
	objs, source, err := loadObjectSetDesiredManifests(digestFile)
	if err != nil {
		return err
	}
	profile := strings.TrimSpace(digestProfile)
	digest, count, err := agent.ComputeRenderedObjectSetDigest(profile, objs)
	if err != nil {
		return err
	}
	out := map[string]interface{}{
		"convention":              "rendered-object-set/v1",
		"renderedObjectSetDigest": digest,
		"objectCount":             count,
		"normalizationProfile":    profile,
		"sourceDigest":            source.Digest,
		"sourceRef":               source.Ref,
	}
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

type chainResolver struct {
	statements map[string]agent.Statement // full fingerprint hex -> verified statement
	files      map[string][]string        // content sha256 -> file paths
	problems   []string
}

func newChainResolver(dir string) (*chainResolver, error) {
	resolver := &chainResolver{statements: map[string]agent.Statement{}, files: map[string][]string{}}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		key := hex.EncodeToString(sum[:])
		resolver.files[key] = append(resolver.files[key], path)
		if strings.HasSuffix(path, ".json") {
			var stmt agent.Statement
			if json.Unmarshal(data, &stmt) == nil && stmt.Type == agent.StatementType {
				fp := strings.TrimPrefix(strings.TrimSpace(stmt.Predicate.Fingerprint), "sha256:")
				if fp != "" && agent.VerifyStatementFingerprint(stmt) == nil {
					resolver.statements[fp] = stmt
				}
			}
		}
		return nil
	})
	return resolver, err
}

func (r *chainResolver) walk(stmt agent.Statement, depth int, out *strings.Builder) {
	indent := strings.Repeat("  ", depth)
	name := string(stmt.Predicate.PredicateName)
	if name == "" {
		name = "(no predicate)"
	}
	fmt.Fprintf(out, "%s- %s verdict=%s fingerprint=%s [verified]\n", indent, name, stmt.Predicate.Verdict, shortHex(stmt.Predicate.Fingerprint))
	for _, subject := range stmt.Subject {
		fmt.Fprintf(out, "%s    subject %s sha256=%s\n", indent, subject.Name, shortHex(subject.Digest["sha256"]))
	}
	if depth >= chainPrintMaxDeep {
		r.problems = append(r.problems, "chain deeper than the walk limit")
		return
	}
	for _, ref := range stmt.Predicate.InputAttestations {
		digest := ref.Digest["sha256"]
		switch {
		case strings.HasPrefix(ref.URI, agent.AttestationURIScheme):
			child, ok := r.statements[digest]
			if !ok {
				r.problems = append(r.problems, fmt.Sprintf("unresolved or tampered cub-scout receipt %s (digest %s)", ref.URI, shortHex(digest)))
				fmt.Fprintf(out, "%s  - %s [UNRESOLVED]\n", indent, ref.URI)
				continue
			}
			r.walk(child, depth+1, out)
		case strings.HasPrefix(ref.URI, agent.ExternalEvidenceURIScheme):
			paths, ok := r.files[digest]
			if !ok {
				r.problems = append(r.problems, fmt.Sprintf("unresolved external evidence %s (digest %s)", ref.URI, shortHex(digest)))
				fmt.Fprintf(out, "%s  - %s [UNRESOLVED]\n", indent, ref.URI)
				continue
			}
			sort.Strings(paths)
			fmt.Fprintf(out, "%s  - %s -> %s [digest-asserted, not fingerprint-verified]\n", indent, ref.URI, paths[0])
		default:
			r.problems = append(r.problems, fmt.Sprintf("unknown attestation scheme %s", ref.URI))
			fmt.Fprintf(out, "%s  - %s [UNKNOWN SCHEME]\n", indent, ref.URI)
		}
	}
}

func shortHex(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func runReceiptChain(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(chainFile) == "" || strings.TrimSpace(chainEvidenceDir) == "" {
		return fmt.Errorf("--file and --evidence-dir are required")
	}
	leaf, err := agent.LoadStatement(chainFile)
	if err != nil {
		return fmt.Errorf("load leaf receipt: %w", err)
	}
	if err := agent.VerifyStatementFingerprint(leaf); err != nil {
		return newExitCodeError(fmt.Errorf("leaf receipt fingerprint does not verify: %w", err), 2)
	}
	resolver, err := newChainResolver(chainEvidenceDir)
	if err != nil {
		return fmt.Errorf("scan evidence dir: %w", err)
	}
	var out strings.Builder
	resolver.walk(leaf, 0, &out)
	fmt.Print(out.String())
	if len(resolver.problems) > 0 {
		for _, problem := range resolver.problems {
			fmt.Fprintf(os.Stderr, "chain: %s\n", problem)
		}
		return newExitCodeError(fmt.Errorf("chain verification failed: %d problem(s)", len(resolver.problems)), 2)
	}
	fmt.Println("chain verified")
	return nil
}
