package main

import (
	"fmt"
	"strings"
)

func withKubeRecoveryHint(err error, command string) error {
	if err == nil {
		return nil
	}
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		cmd = "cub-scout"
	}
	return fmt.Errorf("%w\n\nRecovery:\n  1) kubectl config current-context\n  2) kubectl get ns\n  3) %s --help\n  4) cub-scout quickstart", err, cmd)
}

func mapListTryNextHints(entries []MapEntry, byOwner map[string]int, namespace string) []string {
	hints := make([]string, 0, 3)
	nsFlag := commandNamespaceFlag(namespace)

	if byOwner["Native"] > 0 {
		hints = append(hints, fmt.Sprintf("Found %d unmanaged resources: cub-scout map orphans%s", byOwner["Native"], nsFlag))
	}

	if kind, name, ns, ok := pickExplainResource(entries); ok {
		useNS := chooseNamespace(namespace, ns)
		hints = append(hints, fmt.Sprintf("Explain one resource end-to-end: cub-scout explain %s/%s%s", strings.ToLower(kind), name, commandNamespaceFlag(useNS)))
	}

	hints = append(hints, fmt.Sprintf("Get a one-command health summary: cub-scout doctor%s", nsFlag))

	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hints
}

func doctorTryNextHints(summary DoctorSummary) []string {
	hints := make([]string, 0, 3)
	nsFlag := commandNamespaceFlag(summary.Namespace)

	if summary.Ownership.Unmanaged > 0 {
		hints = append(hints, fmt.Sprintf("Review unmanaged resources: cub-scout map orphans%s", nsFlag))
	}

	if len(summary.TopIssues) > 0 {
		issue := summary.TopIssues[0]
		if kind, name, ok := parseKindName(issue.Resource); ok {
			useNS := chooseNamespace(summary.Namespace, issue.Namespace)
			hints = append(hints, fmt.Sprintf("Explain your top issue: cub-scout explain %s/%s%s", strings.ToLower(kind), name, commandNamespaceFlag(useNS)))
		}
	}

	hints = append(hints, fmt.Sprintf("Run the guided path: cub-scout quickstart%s --yes", nsFlag))

	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hints
}

func explainTryNextHints(summary ExplainSummary) []string {
	hints := make([]string, 0, 3)
	ns := strings.TrimSpace(summary.Namespace)
	nsFlag := commandNamespaceFlag(ns)

	owner := strings.TrimSpace(summary.Owner)
	unknownOwner := strings.HasPrefix(strings.ToLower(owner), "unknown")

	if unknownOwner {
		hints = append(hints,
			fmt.Sprintf("Find unmanaged resources: cub-scout map orphans%s", nsFlag),
			fmt.Sprintf("Find unhealthy resources: cub-scout map issues%s", nsFlag),
		)
	} else {
		if kind, name, ok := parseKindName(summary.Resource); ok {
			hints = append(hints, fmt.Sprintf("Follow the chain in detail: cub-scout trace %s/%s%s --explain", strings.ToLower(kind), name, nsFlag))
		}
		hints = append(hints, fmt.Sprintf("See all %s-owned resources: cub-scout map list%s -q \"owner=%s\"", owner, nsFlag, owner))
	}

	hints = append(hints, fmt.Sprintf("Check overall health: cub-scout doctor%s", nsFlag))

	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hints
}

func renderTryNextSection(hints []string) string {
	if len(hints) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nTRY NEXT:\n")
	for _, hint := range hints {
		b.WriteString("  - ")
		b.WriteString(hint)
		b.WriteString("\n")
	}
	return b.String()
}

func renderTryNextMarkdown(hints []string) string {
	if len(hints) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n### Try Next\n\n")
	for _, hint := range hints {
		b.WriteString("- `")
		b.WriteString(extractCommand(hint))
		b.WriteString("`")
		b.WriteString("\n")
	}
	return b.String()
}

func extractCommand(hint string) string {
	idx := strings.Index(hint, "cub-scout ")
	if idx < 0 {
		return hint
	}
	return strings.TrimSpace(hint[idx:])
}

func commandNamespaceFlag(namespace string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" || strings.EqualFold(ns, "all") || ns == "-" {
		return ""
	}
	return " -n " + ns
}

func chooseNamespace(preferred, fallback string) string {
	p := strings.TrimSpace(preferred)
	if p != "" && !strings.EqualFold(p, "all") {
		return p
	}
	return strings.TrimSpace(fallback)
}

func parseKindName(resource string) (kind, name string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(resource), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func pickExplainResource(entries []MapEntry) (kind, name, namespace string, ok bool) {
	workloadKinds := map[string]struct{}{
		"Deployment":  {},
		"StatefulSet": {},
		"DaemonSet":   {},
		"Application": {},
		"Pod":         {},
	}

	for _, e := range entries {
		if e.Namespace == "" {
			continue
		}
		if _, preferred := workloadKinds[e.Kind]; preferred {
			return e.Kind, e.Name, e.Namespace, true
		}
	}
	for _, e := range entries {
		if e.Namespace == "" {
			continue
		}
		if e.Kind != "" && e.Name != "" {
			return e.Kind, e.Name, e.Namespace, true
		}
	}
	return "", "", "", false
}
