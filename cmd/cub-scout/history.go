// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
)

var (
	historyNamespace string
	historyFormat    string
	historySince     string
)

var historyCmd = &cobra.Command{
	Use:   "history <resource>",
	Short: "Show connected change history from ConfigHub ChangeSets",
	Long: `Show change history for a resource using ConfigHub ChangeSets.

Examples:
  cub-scout history deploy/my-app -n prod
  cub-scout history deploy/my-app -n prod --since 24h --format json
  cub-scout history deploy/my-app --format md
`,
	Args: cobra.ExactArgs(1),
	RunE: runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().StringVarP(&historyNamespace, "namespace", "n", "", "Namespace scope (optional)")
	historyCmd.Flags().StringVar(&historyFormat, "format", "ascii", "Output format: ascii, json, md")
	historyCmd.Flags().StringVar(&historySince, "since", "7d", "Lookback window (examples: 24h, 7d, 2w)")
}

var errHistoryDisconnected = errors.New("history requires ConfigHub connection. Run: cub auth login")

type historyQuery struct {
	Resource  string
	Namespace string
	Since     string
	Window    time.Duration
	Now       time.Time
}

type historyEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Change    string    `json:"change"`
	ChangeSet string    `json:"changeset,omitempty"`
}

type historyResult struct {
	Resource  string         `json:"resource"`
	Namespace string         `json:"namespace,omitempty"`
	Since     string         `json:"since"`
	Entries   []historyEntry `json:"entries"`
}

var (
	requireHistoryConnectedFn = requireHistoryConnected
	fetchHistoryEntriesFn     = fetchHistoryEntries
	historyNowFn              = time.Now
	runHistoryCubCommand      = runHistoryCubCommandImpl
)

func runHistory(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(historyFormat))
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", historyFormat)
	}

	window, err := parseHistorySince(historySince)
	if err != nil {
		return err
	}

	if err := requireHistoryConnectedFn(); err != nil {
		return err
	}

	query := historyQuery{
		Resource:  strings.TrimSpace(args[0]),
		Namespace: strings.TrimSpace(historyNamespace),
		Since:     strings.TrimSpace(historySince),
		Window:    window,
		Now:       historyNowFn().UTC(),
	}

	entries, err := fetchHistoryEntriesFn(cmd.Context(), query)
	if err != nil {
		return err
	}

	result := historyResult{
		Resource:  query.Resource,
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
		fmt.Print(renderHistoryMarkdown(result))
		return nil
	default:
		fmt.Print(renderHistoryASCII(result))
		return nil
	}
}

func requireHistoryConnected() error {
	if err := hub.NewClient().RequireConnected(); err != nil {
		return errHistoryDisconnected
	}
	if _, err := exec.LookPath("cub"); err != nil {
		return fmt.Errorf("history requires cub CLI for connected queries: %w", err)
	}
	return nil
}

func parseHistorySince(raw string) (time.Duration, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return 7 * 24 * time.Hour, nil
	}

	if d, err := time.ParseDuration(value); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("--since must be > 0")
		}
		return d, nil
	}

	if strings.HasSuffix(value, "d") || strings.HasSuffix(value, "w") {
		unit := value[len(value)-1]
		numRaw := strings.TrimSpace(value[:len(value)-1])
		n, err := strconv.Atoi(numRaw)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid --since %q (examples: 24h, 7d, 2w)", raw)
		}
		switch unit {
		case 'd':
			return time.Duration(n) * 24 * time.Hour, nil
		case 'w':
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("invalid --since %q (examples: 24h, 7d, 2w)", raw)
}

func fetchHistoryEntries(ctx context.Context, q historyQuery) ([]historyEntry, error) {
	contains := q.Resource
	if q.Namespace != "" {
		contains = q.Namespace + " " + q.Resource
	}

	args := []string{"changeset", "list", "--json", "--contains", contains}
	if space := detectHistorySpace(ctx); space != "" {
		args = append(args, "--space", space)
	}

	raw, err := runHistoryCubCommand(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("fetch changeset history: %w", err)
	}

	cutoff := q.Now.Add(-q.Window)
	entries, err := buildHistoryEntriesFromChangeSets(raw, cutoff)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func detectHistorySpace(ctx context.Context) string {
	cubCtx, _, err := getStatusCubContext()
	if err != nil || cubCtx == nil {
		return ""
	}
	return strings.TrimSpace(cubCtx.Settings.DefaultSpace)
}

func runHistoryCubCommandImpl(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "cub", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("cub %s failed: %s", strings.Join(args, " "), msg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func buildHistoryEntriesFromChangeSets(raw string, cutoff time.Time) ([]historyEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("parse changeset list json: %w", err)
	}

	items := historyExtractItems(payload)
	entries := make([]historyEntry, 0, len(items))
	for _, item := range items {
		ts, ok := historyExtractTime(item, "CreatedAt", "createdAt", "Timestamp", "timestamp", "UpdatedAt", "updatedAt")
		if !ok {
			continue
		}
		ts = ts.UTC()
		if ts.Before(cutoff) {
			continue
		}

		changeSet := historyFirstString(item, "Slug", "slug", "Name", "name", "ID", "id")
		change := historyFirstString(item, "Description", "description", "DisplayName", "displayName", "Summary", "summary", "Message", "message")
		if change == "" {
			change = changeSet
		}
		if change == "" {
			change = "change recorded"
		}

		actor := historyActor(item)
		if actor == "" {
			actor = "unknown"
		}

		entries = append(entries, historyEntry{
			Timestamp: ts,
			Actor:     actor,
			Change:    change,
			ChangeSet: changeSet,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, nil
}

func historyExtractItems(payload interface{}) []map[string]interface{} {
	switch typed := payload.(type) {
	case []interface{}:
		return historyArrayToObjects(typed)
	case map[string]interface{}:
		for _, key := range []string{"items", "data", "results", "changesets", "ChangeSets"} {
			if arr, ok := typed[key].([]interface{}); ok {
				return historyArrayToObjects(arr)
			}
		}
		return []map[string]interface{}{typed}
	default:
		return nil
	}
}

func historyArrayToObjects(arr []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(arr))
	for _, raw := range arr {
		m, ok := raw.(map[string]interface{})
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func historyExtractTime(item map[string]interface{}, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				continue
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t, true
			}
			if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
				return t, true
			}
		case float64:
			if v > 0 {
				return time.Unix(int64(v), 0), true
			}
		}
	}
	return time.Time{}, false
}

func historyFirstString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		if s, ok := raw.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func historyActor(item map[string]interface{}) string {
	for _, key := range []string{"CreatedBy", "createdBy", "Actor", "actor", "User", "user"} {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			s := strings.TrimSpace(typed)
			if s != "" {
				return s
			}
		case map[string]interface{}:
			s := historyFirstString(typed, "Slug", "slug", "Email", "email", "Name", "name", "DisplayName", "displayName", "ID", "id")
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func renderHistoryASCII(result historyResult) string {
	var b strings.Builder
	target := result.Resource
	if result.Namespace != "" {
		target = fmt.Sprintf("%s (namespace: %s)", result.Resource, result.Namespace)
	}

	b.WriteString(fmt.Sprintf("Change History (last %s): %s\n", result.Since, target))
	if len(result.Entries) == 0 {
		b.WriteString("No history available — resource not yet imported to ConfigHub\n")
		return b.String()
	}

	for _, entry := range result.Entries {
		changeSet := entry.ChangeSet
		if strings.TrimSpace(changeSet) == "" {
			changeSet = "-"
		}
		actor := entry.Actor
		if strings.TrimSpace(actor) == "" {
			actor = "unknown"
		}
		b.WriteString(fmt.Sprintf("  %s  %s  by: %s  changeset: %s\n",
			entry.Timestamp.Format("2006-01-02 15:04"),
			entry.Change,
			actor,
			changeSet,
		))
	}

	return b.String()
}

func renderHistoryMarkdown(result historyResult) string {
	var b strings.Builder
	b.WriteString("# Change History\n\n")
	b.WriteString(fmt.Sprintf("- Resource: `%s`\n", result.Resource))
	if result.Namespace != "" {
		b.WriteString(fmt.Sprintf("- Namespace: `%s`\n", result.Namespace))
	}
	b.WriteString(fmt.Sprintf("- Since: `%s`\n\n", result.Since))

	if len(result.Entries) == 0 {
		b.WriteString("No history available — resource not yet imported to ConfigHub.\n")
		return b.String()
	}

	b.WriteString("| Time (UTC) | Change | Actor | ChangeSet |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, entry := range result.Entries {
		changeSet := strings.TrimSpace(entry.ChangeSet)
		if changeSet == "" {
			changeSet = "-"
		}
		actor := strings.TrimSpace(entry.Actor)
		if actor == "" {
			actor = "unknown"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			entry.Timestamp.Format(time.RFC3339),
			historyEscapeMarkdown(entry.Change),
			historyEscapeMarkdown(actor),
			historyEscapeMarkdown(changeSet),
		))
	}

	return b.String()
}

func historyEscapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}
