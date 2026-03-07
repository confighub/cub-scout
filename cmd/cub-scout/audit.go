// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
)

var (
	auditListNamespace        string
	auditListFormat           string
	auditListSince            string
	auditListIncludeSynthetic bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Break-glass audit trail tools",
}

var auditListCmd = &cobra.Command{
	Use:   "list",
	Short: "List break-glass accept/reject decisions",
	Long: `List break-glass decisions from connected ConfigHub ChangeSet history.

Examples:
  cub-scout audit list
  cub-scout audit list -n prod --since 7d
  cub-scout audit list --format md
  cub-scout audit list --json`,
	RunE: runAuditList,
}

func init() {
	auditCmd.AddCommand(auditListCmd)
	rootCmd.AddCommand(auditCmd)

	auditListCmd.Flags().StringVarP(&auditListNamespace, "namespace", "n", "", "Namespace scope (optional)")
	auditListCmd.Flags().StringVar(&auditListFormat, "format", "ascii", "Output format: ascii, json, md")
	auditListCmd.Flags().StringVar(&auditListSince, "since", "7d", "Lookback window (examples: 24h, 7d, 2w)")
	auditListCmd.Flags().BoolVar(&auditListIncludeSynthetic, "include-synthetic", false, "Include synthetic/demo seeded ChangeSets")
	auditListCmd.Flags().Bool("json", false, "Output as JSON (shorthand for --format json)")
}

var errAuditDisconnected = errors.New("audit requires ConfigHub connection. Run: cub auth login")

type auditListQuery struct {
	Namespace        string
	Since            string
	Window           time.Duration
	Now              time.Time
	IncludeSynthetic bool
}

type auditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	What      string    `json:"what"`
	ChangeSet string    `json:"changeset,omitempty"`
}

type auditListResult struct {
	Namespace string       `json:"namespace,omitempty"`
	Since     string       `json:"since"`
	Entries   []auditEntry `json:"entries"`
}

var (
	requireAuditConnectedFn = requireAuditConnected
	fetchAuditEntriesFn     = fetchAuditEntries
	auditNowFn              = time.Now
	runAuditCubCommand      = runHistoryCubCommandImpl
)

func runAuditList(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(auditListFormat))
	if cmd != nil {
		if jsonFlag, err := cmd.Flags().GetBool("json"); err == nil && jsonFlag {
			format = "json"
		}
	}
	if format == "" {
		format = "ascii"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", auditListFormat)
	}

	window, err := parseHistorySince(auditListSince)
	if err != nil {
		return err
	}

	query := auditListQuery{
		Namespace:        strings.TrimSpace(auditListNamespace),
		Since:            strings.TrimSpace(auditListSince),
		Window:           window,
		Now:              auditNowFn().UTC(),
		IncludeSynthetic: auditListIncludeSynthetic,
	}

	entries, err := resolveAuditEntries(cmd.Context(), query)
	if err != nil {
		return err
	}

	result := auditListResult{
		Namespace: query.Namespace,
		Since:     query.Since,
		Entries:   entries,
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "md":
		fmt.Print(renderAuditMarkdown(result))
		return nil
	default:
		fmt.Print(renderAuditASCII(result))
		return nil
	}
}

func resolveAuditEntries(ctx context.Context, q auditListQuery) ([]auditEntry, error) {
	if fixture := strings.TrimSpace(os.Getenv("CUB_SCOUT_TEST_AUDIT_JSON")); fixture != "" {
		raw, err := os.ReadFile(fixture)
		if err != nil {
			return nil, fmt.Errorf("read audit fixture %q: %w", fixture, err)
		}
		entries, err := buildAuditEntriesFromChangeSetsWithOptions(string(raw), q.Namespace, q.Now.Add(-q.Window), q.IncludeSynthetic)
		if err != nil {
			return nil, fmt.Errorf("parse audit fixture %q: %w", fixture, err)
		}
		return entries, nil
	}

	if err := requireAuditConnectedFn(); err != nil {
		return nil, err
	}

	return fetchAuditEntriesFn(ctx, q)
}

func requireAuditConnected() error {
	if err := hub.NewClient().RequireConnected(); err != nil {
		return errAuditDisconnected
	}
	if _, err := exec.LookPath("cub"); err != nil {
		return fmt.Errorf("audit requires cub CLI for connected queries: %w", err)
	}
	return nil
}

func fetchAuditEntries(ctx context.Context, q auditListQuery) ([]auditEntry, error) {
	args := []string{"changeset", "list", "--json", "--contains", "break-glass"}
	if space := detectHistorySpace(ctx); space != "" {
		args = append(args, "--space", space)
	}

	raw, err := runAuditCubCommand(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("fetch break-glass audit history: %w", err)
	}

	return buildAuditEntriesFromChangeSetsWithOptions(raw, q.Namespace, q.Now.Add(-q.Window), q.IncludeSynthetic)
}

func buildAuditEntriesFromChangeSets(raw, namespace string, cutoff time.Time) ([]auditEntry, error) {
	return buildAuditEntriesFromChangeSetsWithOptions(raw, namespace, cutoff, false)
}

func buildAuditEntriesFromChangeSetsWithOptions(raw, namespace string, cutoff time.Time, includeSynthetic bool) ([]auditEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("parse changeset list json: %w", err)
	}

	namespace = strings.TrimSpace(namespace)
	items := historyExtractItems(payload)
	entries := make([]auditEntry, 0, len(items))
	for _, item := range items {
		if !includeSynthetic && isSyntheticChangeSet(item) {
			continue
		}
		ts, ok := historyExtractTime(item, "CreatedAt", "createdAt", "Timestamp", "timestamp", "UpdatedAt", "updatedAt")
		if !ok {
			continue
		}
		ts = ts.UTC()
		if ts.Before(cutoff) {
			continue
		}

		if !auditItemIsBreakGlass(item) {
			continue
		}
		if namespace != "" && !auditItemMatchesNamespace(item, namespace) {
			continue
		}

		actor := historyActor(item)
		if actor == "" {
			actor = "unknown"
		}

		reason := auditItemReason(item)
		if reason == "" {
			reason = "break-glass decision recorded"
		}
		what := auditItemWhat(item)
		if what == "" {
			what = "decision details unavailable"
		}

		entries = append(entries, auditEntry{
			Timestamp: ts,
			Actor:     actor,
			Reason:    reason,
			What:      what,
			ChangeSet: historyFirstString(item, "Slug", "slug", "Name", "name", "ID", "id"),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	return entries, nil
}

func auditItemIsBreakGlass(item map[string]interface{}) bool {
	allText := strings.ToLower(strings.Join([]string{
		historyFirstString(item, "Slug", "slug", "Name", "name"),
		historyFirstString(item, "Description", "description", "Summary", "summary", "Message", "message", "What", "what"),
	}, " "))
	if strings.Contains(allText, "break-glass") || strings.Contains(allText, "break glass") {
		return true
	}

	for _, key := range []string{"Labels", "labels", "Annotations", "annotations"} {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		labels, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range labels {
			lk := strings.ToLower(strings.TrimSpace(k))
			lv := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
			if strings.Contains(lk, "break-glass") || strings.Contains(lk, "break_glass") {
				if lv == "" || lv == "true" || lv == "1" || lv == "yes" {
					return true
				}
			}
			if strings.Contains(lv, "break-glass") || strings.Contains(lv, "break glass") {
				return true
			}
		}
	}

	return false
}

func auditItemMatchesNamespace(item map[string]interface{}, namespace string) bool {
	namespace = strings.ToLower(strings.TrimSpace(namespace))
	if namespace == "" {
		return true
	}

	if strings.EqualFold(historyFirstString(item, "Namespace", "namespace"), namespace) {
		return true
	}

	text := strings.ToLower(strings.Join([]string{
		historyFirstString(item, "Description", "description", "Summary", "summary", "Message", "message", "What", "what"),
	}, " "))
	if strings.Contains(text, namespace) {
		return true
	}

	for _, key := range []string{"Labels", "labels", "Annotations", "annotations"} {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		labels, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range labels {
			lk := strings.ToLower(strings.TrimSpace(k))
			lv := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
			if lk == "namespace" || lk == "k8s-namespace" || lk == "resource-namespace" {
				if lv == namespace {
					return true
				}
			}
			if strings.Contains(lv, namespace) {
				return true
			}
		}
	}

	return false
}

func auditItemReason(item map[string]interface{}) string {
	reason := historyFirstString(item, "Reason", "reason")
	if reason != "" {
		return reason
	}

	desc := historyFirstString(item, "Description", "description", "Summary", "summary", "Message", "message")
	if desc == "" {
		return ""
	}

	clean := strings.TrimSpace(desc)
	lower := strings.ToLower(clean)
	for _, prefix := range []string{"break-glass accept:", "break glass accept:", "break-glass decision:", "break glass decision:", "break-glass:"} {
		if strings.HasPrefix(lower, prefix) {
			clean = strings.TrimSpace(clean[len(prefix):])
			break
		}
	}
	return clean
}

func auditItemWhat(item map[string]interface{}) string {
	return strings.TrimSpace(historyFirstString(item, "What", "what", "Change", "change", "Summary", "summary", "Description", "description", "Message", "message"))
}

func renderAuditASCII(result auditListResult) string {
	var b strings.Builder
	if result.Namespace != "" {
		b.WriteString(fmt.Sprintf("Break-Glass Audit (last %s, namespace: %s)\n", result.Since, result.Namespace))
	} else {
		b.WriteString(fmt.Sprintf("Break-Glass Audit (last %s)\n", result.Since))
	}

	if len(result.Entries) == 0 {
		b.WriteString("No break-glass decisions recorded for this scope\n")
		return b.String()
	}

	for _, entry := range result.Entries {
		changeSet := strings.TrimSpace(entry.ChangeSet)
		if changeSet == "" {
			changeSet = "-"
		}
		actor := strings.TrimSpace(entry.Actor)
		if actor == "" {
			actor = "unknown"
		}
		reason := strings.TrimSpace(entry.Reason)
		if reason == "" {
			reason = "break-glass decision recorded"
		}
		what := strings.TrimSpace(entry.What)
		if what == "" {
			what = "decision details unavailable"
		}
		b.WriteString(fmt.Sprintf("  %s  reason: %s\n", entry.Timestamp.Format("2006-01-02 15:04"), reason))
		b.WriteString(fmt.Sprintf("              what: %s\n", what))
		b.WriteString(fmt.Sprintf("              by: %s  changeset: %s\n", actor, changeSet))
	}

	return b.String()
}

func renderAuditMarkdown(result auditListResult) string {
	var b strings.Builder
	b.WriteString("# Break-Glass Audit\n\n")
	if result.Namespace != "" {
		b.WriteString(fmt.Sprintf("- Namespace: `%s`\n", result.Namespace))
	}
	b.WriteString(fmt.Sprintf("- Since: `%s`\n\n", result.Since))

	if len(result.Entries) == 0 {
		b.WriteString("No break-glass decisions recorded for this scope.\n")
		return b.String()
	}

	b.WriteString("| Time (UTC) | Reason | What | Actor | ChangeSet |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, entry := range result.Entries {
		reason := strings.TrimSpace(entry.Reason)
		if reason == "" {
			reason = "break-glass decision recorded"
		}
		what := strings.TrimSpace(entry.What)
		if what == "" {
			what = "decision details unavailable"
		}
		actor := strings.TrimSpace(entry.Actor)
		if actor == "" {
			actor = "unknown"
		}
		changeSet := strings.TrimSpace(entry.ChangeSet)
		if changeSet == "" {
			changeSet = "-"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			entry.Timestamp.Format(time.RFC3339),
			historyEscapeMarkdown(reason),
			historyEscapeMarkdown(what),
			historyEscapeMarkdown(actor),
			historyEscapeMarkdown(changeSet),
		))
	}

	return b.String()
}
