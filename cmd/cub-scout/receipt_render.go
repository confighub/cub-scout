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

	// Source-truth evidence (when present — only the source-truth-pass
	// path attaches this surface).
	if pred.Evidence.SourceTruth != nil {
		st := pred.Evidence.SourceTruth
		b.WriteString("\nEvidence (source-truth)\n")
		fmt.Fprintf(&b, "  strategy:    %s\n", st.DeclaredStrategy)
		fmt.Fprintf(&b, "  status:      %s\n", st.Status)
		fmt.Fprintf(&b, "  verdict:     %s\n", st.SourceTruth)
		if st.Outlier != "" {
			fmt.Fprintf(&b, "  outlier:     %s\n", st.Outlier)
		}
		if len(st.ProofGaps) > 0 {
			fmt.Fprintf(&b, "  proof_gaps:  %s\n", strings.Join(st.ProofGaps, ", "))
		}
	}

	if pred.Evidence.ObjectSet != nil {
		oset := pred.Evidence.ObjectSet
		b.WriteString("\nEvidence (object set)\n")
		fmt.Fprintf(&b, "  source:      %s %s\n", oset.DesiredSource.Type, oset.DesiredSource.Ref)
		fmt.Fprintf(&b, "  matchMode:   %s\n", oset.MatchMode)
		fmt.Fprintf(&b, "  desired:     %d\n", oset.Summary.Desired)
		fmt.Fprintf(&b, "  matched:     %d\n", oset.Summary.Matched)
		fmt.Fprintf(&b, "  missing:     %d\n", oset.Summary.Missing)
		fmt.Fprintf(&b, "  mismatched:  %d\n", oset.Summary.Mismatched)
		fmt.Fprintf(&b, "  inconclusive:%d\n", oset.Summary.Inconclusive)
		for _, obj := range oset.Objects {
			if obj.Status == agent.ObjectSetObjectMatched {
				continue
			}
			fmt.Fprintf(&b, "  - %s: %s\n", obj.ID.String(), obj.Status)
			if obj.Error != "" {
				fmt.Fprintf(&b, "      error: %s\n", obj.Error)
			}
			for _, diff := range obj.Differences {
				fmt.Fprintf(&b, "      diff: %s (%s)\n", diff.Path, diff.Reason)
			}
		}
	}

	if pred.Evidence.Workloads != nil {
		w := pred.Evidence.Workloads
		b.WriteString("\nEvidence (workloads converged)\n")
		fmt.Fprintf(&b, "  source:      %s %s\n", w.DesiredSource.Type, w.DesiredSource.Ref)
		if w.GraceWindow != "" {
			fmt.Fprintf(&b, "  graceWindow: %s\n", w.GraceWindow)
		}
		fmt.Fprintf(&b, "  observedAt:  %s\n", w.ObservedAt)
		fmt.Fprintf(&b, "  desired:     %d\n", w.Summary.Desired)
		fmt.Fprintf(&b, "  converged:   %d\n", w.Summary.Converged)
		fmt.Fprintf(&b, "  progressing: %d\n", w.Summary.Progressing)
		fmt.Fprintf(&b, "  failed:      %d\n", w.Summary.Failed)
		fmt.Fprintf(&b, "  missing:     %d\n", w.Summary.Missing)
		fmt.Fprintf(&b, "  inconclusive:%d\n", w.Summary.Inconclusive)
		for _, obj := range w.Workloads {
			if obj.Status == agent.WorkloadConvergedConverged {
				continue
			}
			fmt.Fprintf(&b, "  - %s: %s", obj.ID.String(), obj.Status)
			if obj.KstatusStatus != "" {
				fmt.Fprintf(&b, " (kstatus: %s)", obj.KstatusStatus)
			}
			b.WriteString("\n")
			if obj.Error != "" {
				fmt.Fprintf(&b, "      error: %s\n", obj.Error)
			}
			for _, pr := range obj.PodReasons {
				fmt.Fprintf(&b, "      pod %s: %s\n", pr.Pod, pr.Reason)
			}
		}
	}

	if pred.Evidence.Prerequisites != nil {
		p := pred.Evidence.Prerequisites
		b.WriteString("\nEvidence (prerequisites)\n")
		fmt.Fprintf(&b, "  source:      %s %s\n", p.Source.Type, p.Source.Ref)
		fmt.Fprintf(&b, "  required:    %d\n", p.Summary.Required)
		fmt.Fprintf(&b, "  present:     %d\n", p.Summary.Present)
		fmt.Fprintf(&b, "  missing:     %d\n", p.Summary.Missing)
		fmt.Fprintf(&b, "  inconclusive:%d\n", p.Summary.Inconclusive)
		for _, f := range p.Facts {
			if f.Status == agent.PrerequisitePresent {
				continue
			}
			fmt.Fprintf(&b, "  - %s %s", f.Kind, f.Name)
			if f.Namespace != "" {
				fmt.Fprintf(&b, " (ns %s)", f.Namespace)
			}
			fmt.Fprintf(&b, ": %s", f.Status)
			if f.Detail != "" {
				fmt.Fprintf(&b, " — %s", f.Detail)
			}
			b.WriteString("\n")
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
