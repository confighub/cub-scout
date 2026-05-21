// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
)

// renderReceiptASCII produces a concise human-readable summary of a receipt
// Statement. Designed for one-screen review during ad-hoc verification;
// for the full structured artifact, use --format json.
func renderReceiptASCII(stmt agent.Statement) string {
	var b strings.Builder

	pred := stmt.Predicate
	fmt.Fprintf(&b, "Receipt: %s\n", pred.PredicateName)
	fmt.Fprintf(&b, "  Verdict: %s\n", pred.Verdict)
	if pred.Claim != "" {
		fmt.Fprintf(&b, "  Claim:   %s\n", pred.Claim)
	}

	// Scope.
	fmt.Fprintf(&b, "  Scope:   %s/%s", pred.Scope.Kind, pred.Scope.Name)
	if pred.Scope.Namespace != "" {
		fmt.Fprintf(&b, " in %s", pred.Scope.Namespace)
	}
	if pred.Scope.Cluster != "" {
		fmt.Fprintf(&b, " (cluster: %s)", pred.Scope.Cluster)
	}
	b.WriteString("\n")

	// Verifier + timestamp.
	fmt.Fprintf(&b, "  By:      %s %s at %s\n", pred.Verifier.Tool, pred.Verifier.Version, pred.VerifiedAt)

	// Spec anchor (when present).
	if pred.Spec != nil {
		a := pred.Spec.Anchor
		parts := []string{}
		if a.RepoURL != "" {
			parts = append(parts, a.RepoURL)
		}
		if a.Revision != "" {
			parts = append(parts, "@"+a.Revision)
		}
		if a.Path != "" {
			parts = append(parts, "path="+a.Path)
		}
		if a.File != "" {
			if a.Line > 0 {
				parts = append(parts, fmt.Sprintf("file=%s:%d", a.File, a.Line))
			} else {
				parts = append(parts, "file="+a.File)
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(&b, "  Spec:    %s\n", strings.Join(parts, " "))
		}
	}

	// Subjects.
	if len(stmt.Subject) > 0 {
		b.WriteString("\nSubjects\n")
		for _, s := range stmt.Subject {
			digest := s.Digest["sha256"]
			if len(digest) > 12 {
				digest = digest[:12] + "…"
			}
			fmt.Fprintf(&b, "  - %s  sha256:%s\n", s.Name, digest)
		}
	}

	// Evidence summary (just the highlights — full body is in --format json).
	if pred.Evidence.Attribution != nil {
		attr := pred.Evidence.Attribution
		b.WriteString("\nEvidence (attribution)\n")
		fmt.Fprintf(&b, "  cause:       %s\n", attr.Cause)
		if attr.ManagerHint != "" {
			fmt.Fprintf(&b, "  managerHint: %s\n", attr.ManagerHint)
		}
		if pred.Evidence.GitSource != nil {
			gs := pred.Evidence.GitSource
			fmt.Fprintf(&b, "  gitSource:   %s @%s path=%s\n", gs.RepoURL, gs.Revision, gs.Path)
		}
	}

	// Omissions are critical — they convert silent PASS into honest PASS.
	if len(pred.Omissions) > 0 {
		b.WriteString("\nOmissions (deliberate non-claims)\n")
		for _, o := range pred.Omissions {
			line := fmt.Sprintf("  - %s", o.Missing)
			if o.Reason != "" {
				line += "  — " + o.Reason
			}
			if o.Severity != "" {
				line += "  [" + o.Severity + "]"
			}
			b.WriteString(line + "\n")
		}
	}

	// Next-step hints (filtered to read-only / waiting / human-decision).
	if len(pred.NextSteps) > 0 {
		b.WriteString("\nNext steps (read-only)\n")
		for _, s := range pred.NextSteps {
			fmt.Fprintf(&b, "  - [%s] %s\n", s.ActionType, s.Reason)
			if s.NextCommand != "" {
				fmt.Fprintf(&b, "      → %s\n", s.NextCommand)
			}
		}
	}

	// Fingerprint.
	fmt.Fprintf(&b, "\nFingerprint: %s\n", pred.Fingerprint)

	return b.String()
}
