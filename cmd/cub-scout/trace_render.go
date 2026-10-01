// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// traceHumanOptions are the presentation choices for the shared human renderer.
type traceHumanOptions struct {
	Explain   bool
	Artifacts bool
	History   bool
	Limit     int
}

type traceNoChainError struct{ message string }

func (e traceNoChainError) Error() string {
	if e.message == "" {
		return "trace returned no ownership chain"
	}
	return e.message
}

type traceHumanWriter struct {
	writer io.Writer
	err    error
}

func (w *traceHumanWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

// renderTraceHuman writes the full human trace projection to w. It is a pure
// projection: it reads no flags, process streams, or cluster state and never
// exits the process.
func renderTraceHuman(w io.Writer, result *agent.TraceResult, artifacts map[string]mapsvc.TraceArtifactRef, invCtx InvocationContext, options traceHumanOptions) error {
	if w == nil {
		return fmt.Errorf("trace output writer is nil")
	}
	if result == nil {
		return fmt.Errorf("trace result is nil")
	}
	trackedWriter := &traceHumanWriter{writer: w}
	w = trackedWriter
	mode := invCtx.Mode()
	explicitMode := invCtx.IsExplicit()

	// Header - varies by presentation mode
	fmt.Fprintf(w, "\n")
	if explicitMode {
		heading := TraceHeading(mode, result.Object.String())
		if mode == PresentationAI {
			fmt.Fprintf(w, "%s\n", heading)
		} else {
			fmt.Fprintf(w, "%s%s%s\n", colorBold, heading, colorReset)
		}
		// Intro - shows ownership tool for AI mode
		if intro := TraceIntro(mode, result.Tool); intro != "" {
			fmt.Fprintf(w, "%s\n", intro)
		}
	} else {
		// Legacy format
		fmt.Fprintf(w, "%s%sTRACE:%s %s%s%s\n", colorBold, colorCyan, colorReset, colorBold, result.Object.String(), colorReset)
	}
	fmt.Fprintf(w, "\n")

	if result.Context != "" {
		fmt.Fprintf(w, "Selected Kubernetes context: %s\n\n", result.Context)
	}

	// Explanatory content when --explain is used
	if options.Explain {
		fmt.Fprintf(w, "%s%sOWNERSHIP CHAIN EXPLAINED%s\n", colorBold, colorWhite, colorReset)
		fmt.Fprintf(w, "%s════════════════════════════════════════════════════════════════════%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "GitOps creates a chain from Git to running pods:\n\n")
		fmt.Fprintf(w, "  %sGit Repository%s (source of truth)\n", colorPurple, colorReset)
		fmt.Fprintf(w, "       %s↓%s GitOps controller watches for changes\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sKustomization/HelmRelease%s (applies manifests)\n", colorCyan, colorReset)
		fmt.Fprintf(w, "       %s↓%s Creates/updates\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sDeployment%s (desired state)\n", colorGreen, colorReset)
		fmt.Fprintf(w, "       %s↓%s K8s controller creates\n", colorDim, colorReset)
		fmt.Fprintf(w, "  %sReplicaSet → Pods%s (running containers)\n", colorYellow, colorReset)
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%sThe trace below shows this chain for your resource:%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "\n")
	}

	if result.Error != "" && len(result.Chain) == 0 {
		// Per cli-contract.md, an empty trace chain is rendered as a warning and
		// returned as a typed error so CLI callers can preserve exit code 1 while
		// in-process callers (such as the TUI) remain alive.
		fmt.Fprintf(w, "  %s[warning] %s%s\n\n", colorYellow, result.Error, colorReset)
		if trackedWriter.err != nil {
			return trackedWriter.err
		}
		return traceNoChainError{message: result.Error}
	}
	if result.Error != "" {
		// Degraded mode: still render available chain, but make missing context explicit.
		fmt.Fprintf(w, "  %s[warning] %s%s\n\n", colorYellow, result.Error, colorReset)
	}

	// Chain heading for explicit presentation modes
	if explicitMode {
		if heading := TraceChainHeading(mode); heading != "" {
			if mode == PresentationAI {
				fmt.Fprintf(w, "%s\n", heading)
			} else {
				fmt.Fprintf(w, "%s%s%s\n", colorBold, heading, colorReset)
			}
		}
	}

	// Print chain
	for i, link := range result.Chain {
		prefix := "  "
		if i > 0 {
			// Add tree connector with color
			prefix = strings.Repeat("    ", i-1) + fmt.Sprintf("    %s└─▶%s ", colorDim, colorReset)
		}

		// Status icon with color
		var icon, iconColor string
		if link.Ready {
			icon = SymOK
			iconColor = colorGreen
		} else {
			icon = SymError
			iconColor = colorRed
		}

		// Kind color based on type
		kindColor := colorWhite
		switch link.Kind {
		case "GitRepository", "OCIRepository", "HelmRepository", "Bucket", "Source", "SveltosReference", "EventSource", "ModelCache", "InferenceClass", "InferenceCluster":
			kindColor = colorPurple
		case "ConfigHub OCI":
			kindColor = colorBlue // ConfigHub gets blue
		case "Kustomization", "HelmRelease", "HelmChart", "ClusterProfile", "Profile", "EventTrigger", "ClusterHealthCheck", "ClusterPromotion", "ModelDeployment", "ModelService", "InferenceGateway":
			kindColor = colorCyan
		case "Application":
			kindColor = colorBlue
		case "Deployment", "StatefulSet", "DaemonSet", "ModelReplica", "ModelEndpoint":
			kindColor = colorGreen
		case "Service", "ConfigMap", "Secret":
			kindColor = colorYellow
		}

		// Main line with colors
		fmt.Fprintf(w, "%s%s%s%s %s%s%s/%s%s\n", prefix, iconColor, icon, colorReset, kindColor, link.Kind, colorReset, colorBold, link.Name+colorReset)

		// Details (indented) with dim colors
		detailPrefix := strings.Repeat("    ", i) + fmt.Sprintf("    %s│%s ", colorDim, colorReset)
		if i == len(result.Chain)-1 {
			detailPrefix = strings.Repeat("    ", i) + "      "
		}

		if link.Namespace != "" && link.Namespace != result.Object.Namespace {
			fmt.Fprintf(w, "%s%sNamespace:%s %s\n", detailPrefix, colorDim, colorReset, link.Namespace)
		}

		// Show OCI source details for ConfigHub OCI sources
		if link.OCISource != nil && link.OCISource.IsConfigHub {
			if link.OCISource.Space != "" {
				fmt.Fprintf(w, "%s%sSpace:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorCyan, link.OCISource.Space, colorReset)
			}
			if link.OCISource.Target != "" {
				fmt.Fprintf(w, "%s%sTarget:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorCyan, link.OCISource.Target, colorReset)
			}
			if link.OCISource.Instance != "" {
				fmt.Fprintf(w, "%s%sRegistry:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorBlue, link.OCISource.Registry, colorReset)
			}
		} else if link.URL != "" {
			// Show URL for non-ConfigHub sources
			fmt.Fprintf(w, "%s%sURL:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorBlue, link.URL, colorReset)
		}

		if link.Path != "" {
			fmt.Fprintf(w, "%s%sPath:%s %s\n", detailPrefix, colorDim, colorReset, link.Path)
		}
		if link.Revision != "" {
			fmt.Fprintf(w, "%s%sRevision:%s %s%s%s\n", detailPrefix, colorDim, colorReset, colorPurple, link.Revision, colorReset)
		}
		if options.Artifacts && isTraceSourceKind(link.Kind) {
			artifact := artifactForLink(link, artifacts)
			fmt.Fprintf(w, "%s%sArtifact URL:%s %s\n", detailPrefix, colorDim, colorReset, artifact.URL)
			fmt.Fprintf(w, "%s%sArtifact Revision:%s %s\n", detailPrefix, colorDim, colorReset, artifact.Revision)
			fmt.Fprintf(w, "%s%sArtifact Digest:%s %s\n", detailPrefix, colorDim, colorReset, artifact.Digest)
			fmt.Fprintf(w, "%s%sArtifact Updated:%s %s\n", detailPrefix, colorDim, colorReset, artifact.LastUpdateTime)
		}
		if link.Status != "" {
			statusColor := colorGreen
			if !link.Ready {
				statusColor = colorYellow
			}
			fmt.Fprintf(w, "%s%sStatus:%s %s%s%s\n", detailPrefix, colorDim, colorReset, statusColor, link.Status, colorReset)
		}
		// Show elapsed time if available
		if link.LastTransitionTime != nil {
			elapsed := time.Since(*link.LastTransitionTime)
			elapsedStr := formatElapsed(elapsed)
			elapsedColor := colorDim
			// Highlight if stuck (e.g., reconciling for more than 5 minutes)
			if !link.Ready && elapsed > 5*time.Minute {
				elapsedColor = colorYellow
				elapsedStr += " ⚠"
			}
			fmt.Fprintf(w, "%s%sElapsed:%s %s%s%s\n", detailPrefix, colorDim, colorReset, elapsedColor, elapsedStr, colorReset)
		}
		if link.Message != "" && !link.Ready {
			fmt.Fprintf(w, "%s%sError:%s %s%s%s\n", detailPrefix, colorRed, colorReset, colorRed, link.Message, colorReset)
		}
		// Add spacing line
		if i < len(result.Chain)-1 {
			fmt.Fprintf(w, "%s%s│%s\n", strings.Repeat("    ", i)+"    ", colorDim, colorReset)
		}
	}

	// Show cross-owner references if detected
	if len(result.CrossReferences) > 0 {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s%s⚠ Cross-owner references detected:%s\n", colorBold, colorYellow, colorReset)
		for _, ref := range result.CrossReferences {
			ownerColor := colorWhite
			ownerType := "unknown"
			if ref.Owner != nil {
				ownerType = ref.Owner.Type
				switch ownerType {
				case "flux":
					ownerColor = colorCyan
				case "argo":
					ownerColor = colorPurple
				case "crossplane":
					ownerColor = colorBlue
				case "helm":
					ownerColor = colorYellow
				}
			}

			statusIcon := SymOK
			statusColor := colorGreen
			if ref.Status == "missing" {
				statusIcon = SymError
				statusColor = colorRed
			} else if ref.Status == "pending" {
				statusIcon = "⏳"
				statusColor = colorYellow
			}

			fmt.Fprintf(w, "  %s%s%s %s%s/%s%s %s(owner: %s%s%s)%s\n",
				statusColor, statusIcon, colorReset,
				colorWhite, ref.Ref.Kind, ref.Ref.Name, colorReset,
				colorDim, ownerColor, ownerType, colorDim, colorReset)

			if ref.RefType != "" {
				fmt.Fprintf(w, "      %sreferenced via: %s%s\n", colorDim, ref.RefType, colorReset)
			}
			if ref.Owner != nil && ref.Owner.Name != "" {
				fmt.Fprintf(w, "      %smanaged by: %s%s\n", colorDim, ref.Owner.Name, colorReset)
			}
			if ref.Message != "" && ref.Status != "exists" {
				fmt.Fprintf(w, "      %s%s%s\n", colorRed, ref.Message, colorReset)
			}
		}
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s  💡 These resources are managed by different tools.%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "%s     Changes to one tool won't automatically update the other.%s\n", colorDim, colorReset)
	}

	// Show secret evidence if present
	if result.Secrets != nil && result.Secrets.Summary.Total > 0 {
		fmt.Fprintf(w, "\n")
		if result.Secrets.HasIssues() {
			fmt.Fprintf(w, "%s%s⚠ Secret evidence:%s\n", colorBold, colorYellow, colorReset)
		} else {
			fmt.Fprintf(w, "%s%s✓ Secret evidence:%s\n", colorBold, colorGreen, colorReset)
		}

		for _, s := range result.Secrets.Secrets {
			var statusIcon, statusColor string
			switch s.Status {
			case agent.SecretStatusPresent:
				statusIcon = SymOK
				statusColor = colorGreen
			case agent.SecretStatusMissing:
				statusIcon = SymError
				statusColor = colorRed
			case agent.SecretStatusUnreadable:
				statusIcon = "🔒"
				statusColor = colorYellow
			default:
				statusIcon = "?"
				statusColor = colorYellow
			}

			optionalTag := ""
			if s.Optional {
				optionalTag = fmt.Sprintf(" %s(optional)%s", colorDim, colorReset)
			}

			fmt.Fprintf(w, "  %s%s%s %sSecret/%s%s %s[%s]%s%s\n",
				statusColor, statusIcon, colorReset,
				colorWhite, s.Name, colorReset,
				statusColor, s.Status, colorReset,
				optionalTag)

			fmt.Fprintf(w, "      %sreferenced via: %s%s\n", colorDim, s.RefType, colorReset)

			if s.Status == agent.SecretStatusPresent && s.SecretType != "" {
				fmt.Fprintf(w, "      %stype: %s%s\n", colorDim, s.SecretType, colorReset)
			}
			if s.Owner != nil && s.Owner.Name != "" {
				fmt.Fprintf(w, "      %smanaged by: %s (%s)%s\n", colorDim, s.Owner.Name, s.Owner.Type, colorReset)
			}
			if s.StatusReason != "" && s.Status != agent.SecretStatusPresent {
				fmt.Fprintf(w, "      %s%s%s\n", colorRed, s.StatusReason, colorReset)
			}
		}

		// Summary line
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "  %s%d secrets (%d present, %d missing, %d unreadable)%s\n",
			colorDim,
			result.Secrets.Summary.Total,
			result.Secrets.Summary.Present,
			result.Secrets.Summary.Missing,
			result.Secrets.Summary.Unreadable,
			colorReset)
	}

	// Show recent events if present
	if result.Events != nil && len(result.Events.Events) > 0 {
		fmt.Fprintf(w, "\n")
		if result.Events.WarningCount > 0 || result.Events.ErrorCount > 0 {
			fmt.Fprintf(w, "%s%s⚠ Recent events:%s\n", colorBold, colorYellow, colorReset)
		} else {
			fmt.Fprintf(w, "%s%sRecent events:%s\n", colorBold, colorWhite, colorReset)
		}

		for _, ev := range result.Events.Events {
			var icon, iconColor string
			switch ev.Severity {
			case "error":
				icon = SymError
				iconColor = colorRed
			case "warning":
				icon = "⚠"
				iconColor = colorYellow
			default:
				icon = "○"
				iconColor = colorDim
			}

			countStr := ""
			if ev.Count > 1 {
				countStr = fmt.Sprintf(" (x%d)", ev.Count)
			}
			detail := formatEventActionDetail(ev.Action)
			if detail != "" {
				detail = " [" + detail + "]"
			}

			fmt.Fprintf(w, "  %s%s%s %s%s%s %s%s: %s%s\n",
				iconColor, icon, colorReset,
				colorDim, ev.Age, colorReset,
				ev.Reason,
				detail,
				ev.Message,
				countStr,
			)
		}

		// Summary line
		if result.Events.TotalCount > len(result.Events.Events) {
			fmt.Fprintf(w, "\n  %s%d of %d events shown (warnings/errors prioritized)%s\n",
				colorDim,
				len(result.Events.Events),
				result.Events.TotalCount,
				colorReset)
		}
	}

	renderTraceDeliveryEvidenceHumanTo(w, result.DeliveryEvidence)

	// Show history if requested and available
	if options.History && len(result.History) > 0 {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s%sHistory:%s\n", colorBold, colorWhite, colorReset)
		limit := options.Limit
		if limit < 0 {
			limit = 0
		}
		if limit > len(result.History) {
			limit = len(result.History)
		}
		for i := 0; i < limit; i++ {
			h := result.History[i]
			timeStr := h.Timestamp.Format("2006-01-02 15:04")
			statusColor := colorGreen
			if h.Status == "failed" || h.Status == "superseded" {
				statusColor = colorYellow
			}
			fmt.Fprintf(w, "  %s%-16s%s  %s%-20s%s  %s%s%s",
				colorDim, timeStr, colorReset,
				colorPurple, truncate(h.Revision, 20), colorReset,
				statusColor, h.Status, colorReset)
			if h.Source != "" {
				fmt.Fprintf(w, "  %s%s%s", colorDim, h.Source, colorReset)
			}
			fmt.Fprintf(w, "\n")
		}
		if len(result.History) > limit {
			fmt.Fprintf(w, "  %s... and %d more (use --limit to show more)%s\n", colorDim, len(result.History)-limit, colorReset)
		}
	} else if options.History {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%s%sHistory:%s %sNo history available%s\n", colorBold, colorWhite, colorReset, colorDim, colorReset)
	}

	// Summary
	fmt.Fprintf(w, "\n")
	if result.FullyManaged {
		fmt.Fprintf(w, "%s%s✓ All levels in sync.%s Managed by %s%s%s.\n", colorBold, colorGreen, colorReset, colorCyan, result.Tool, colorReset)
	} else {
		// Find the broken link
		for _, link := range result.Chain {
			if !link.Ready {
				fmt.Fprintf(w, "%s%s⚠ Chain broken at %s/%s%s\n", colorBold, colorYellow, link.Kind, link.Name, colorReset)
				if link.Message != "" {
					fmt.Fprintf(w, "  %s%s%s\n", colorRed, link.Message, colorReset)
				}
				break
			}
		}
	}

	// Next steps and diagram link when --explain is used
	if options.Explain {
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "%sNEXT STEPS:%s\n", colorBold, colorReset)
		if result.Context == "" {
			fmt.Fprintf(w, "→ See orphan resources:    cub-scout map orphans\n")
			fmt.Fprintf(w, "→ Show diff from Git:      cub-scout trace %s -n %s --diff\n", result.Object.String(), result.Object.Namespace)
		} else {
			fmt.Fprintf(w, "→ Orphan and delegated diff follow-ups are withheld: these paths cannot preserve this captured binding.\n")
		}
		fmt.Fprintf(w, "→ Visual guide:            docs/diagrams/ownership-detection.svg\n")
	}

	// Outro - only for explicit AI mode
	if explicitMode {
		if outro := TraceOutro(mode); outro != "" {
			fmt.Fprintf(w, "\n%s\n", outro)
		}
	}

	fmt.Fprintf(w, "\n")

	return trackedWriter.err

}
