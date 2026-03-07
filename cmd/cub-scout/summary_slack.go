// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/internal/summarystore"
	"github.com/spf13/cobra"
)

var (
	summarySlackWebhookURL   string
	summarySlackSince        string
	summarySlackType         string
	summarySlackCluster      string
	summarySlackNamespace    string
	summarySlackBatchSize    int
	summarySlackDedupeWindow string
	summarySlackForce        bool
	summarySlackDryRun       bool

	summarySlackHTTPClient = &http.Client{Timeout: 10 * time.Second}
)

type summarySlackDigestOptions struct {
	Since     string
	Type      string
	Cluster   string
	Namespace string
	BatchSize int
}

type summarySlackRiskTotals struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

type summarySlackSyncTotals struct {
	Total     int `json:"total"`
	Failed    int `json:"failed"`
	OutOfSync int `json:"outOfSync"`
}

type summarySlackDriftTotals struct {
	Total int `json:"total"`
}

type summarySlackTopEntry struct {
	When      string `json:"when"`
	Type      string `json:"type"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace,omitempty"`
	Summary   string `json:"summary"`
}

type summarySlackDigest struct {
	GeneratedAt time.Time               `json:"generatedAt"`
	Since       string                  `json:"since"`
	Type        string                  `json:"type,omitempty"`
	Cluster     string                  `json:"cluster,omitempty"`
	Namespace   string                  `json:"namespace,omitempty"`
	Count       int                     `json:"count"`
	Risk        summarySlackRiskTotals  `json:"risk"`
	Sync        summarySlackSyncTotals  `json:"sync"`
	Drift       summarySlackDriftTotals `json:"drift"`
	Clusters    []string                `json:"clusters"`
	Scopes      []string                `json:"scopes"`
	Top         []summarySlackTopEntry  `json:"top"`
	NextAction  string                  `json:"nextAction"`
	Signature   string                  `json:"signature"`
}

type slackTextObject struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackBlock struct {
	Type   string            `json:"type"`
	Text   *slackTextObject  `json:"text,omitempty"`
	Fields []slackTextObject `json:"fields,omitempty"`
}

type slackWebhookPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

type summarySlackDedupeState struct {
	LastSignature string    `json:"lastSignature"`
	LastSentAt    time.Time `json:"lastSentAt"`
}

var summarySlackCmd = &cobra.Command{
	Use:   "slack",
	Short: "Publish connected summary digest to a Slack webhook",
	Long: `Build a connected drift/sync/risk digest from persisted summary records
and post it to a Slack Incoming Webhook.

Examples:
  cub-scout summary slack --webhook-url https://hooks.slack.com/services/...
  cub-scout summary slack --since 7d --cluster kind-dev --batch-size 5
  cub-scout summary slack --dry-run --since 24h`,
	RunE: runSummarySlack,
}

func init() {
	summaryCmd.AddCommand(summarySlackCmd)

	summarySlackCmd.Flags().StringVar(&summarySlackWebhookURL, "webhook-url", "", "Slack incoming webhook URL (or CUB_SCOUT_SLACK_WEBHOOK_URL)")
	summarySlackCmd.Flags().StringVar(&summarySlackSince, "since", "24h", "Digest lookback window (examples: 24h, 7d, 2w)")
	summarySlackCmd.Flags().StringVar(&summarySlackType, "type", "", "Filter by summary type (scan, gitops-status)")
	summarySlackCmd.Flags().StringVar(&summarySlackCluster, "cluster", "", "Filter by cluster/context")
	summarySlackCmd.Flags().StringVarP(&summarySlackNamespace, "namespace", "n", "", "Filter by namespace")
	summarySlackCmd.Flags().IntVar(&summarySlackBatchSize, "batch-size", 10, "Maximum entries to include in digest body")
	summarySlackCmd.Flags().StringVar(&summarySlackDedupeWindow, "dedupe-window", "30m", "Skip duplicate digest signatures within this window")
	summarySlackCmd.Flags().BoolVar(&summarySlackForce, "force", false, "Bypass dedupe and always post")
	summarySlackCmd.Flags().BoolVar(&summarySlackDryRun, "dry-run", false, "Print payload JSON without posting")
}

func runSummarySlack(cmd *cobra.Command, args []string) error {
	window, err := parseHistorySince(summarySlackSince)
	if err != nil {
		return err
	}
	if summarySlackBatchSize <= 0 {
		return fmt.Errorf("--batch-size must be > 0")
	}
	dedupeWindow, err := parseHistorySince(summarySlackDedupeWindow)
	if err != nil {
		return fmt.Errorf("invalid --dedupe-window: %w", err)
	}

	store, err := newSummaryStore()
	if err != nil {
		return fmt.Errorf("open summary store: %w", err)
	}

	entries, err := store.List(summarystore.Query{
		Since:     summaryNowFn().UTC().Add(-window),
		Type:      strings.TrimSpace(summarySlackType),
		Cluster:   strings.TrimSpace(summarySlackCluster),
		Namespace: strings.TrimSpace(summarySlackNamespace),
	})
	if err != nil {
		return fmt.Errorf("query summary store: %w", err)
	}

	digest := buildSlackDigest(entries, summarySlackDigestOptions{
		Since:     strings.TrimSpace(summarySlackSince),
		Type:      strings.TrimSpace(summarySlackType),
		Cluster:   strings.TrimSpace(summarySlackCluster),
		Namespace: strings.TrimSpace(summarySlackNamespace),
		BatchSize: summarySlackBatchSize,
	})
	payload := buildSlackPayload(digest)

	if summarySlackDryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if digest.Count == 0 {
		fmt.Println("No connected summary records in selected window; skipping Slack post.")
		return nil
	}

	statePath, err := summarySlackStatePath()
	if err != nil {
		return err
	}
	skip, err := shouldSkipSlackPost(statePath, digest.Signature, summaryNowFn().UTC(), dedupeWindow, summarySlackForce)
	if err != nil {
		return err
	}
	if skip {
		fmt.Println("Skipping Slack post (duplicate digest within dedupe window).")
		return nil
	}

	webhookURL := strings.TrimSpace(summarySlackWebhookURL)
	if webhookURL == "" {
		webhookURL = strings.TrimSpace(os.Getenv("CUB_SCOUT_SLACK_WEBHOOK_URL"))
	}
	if webhookURL == "" {
		return fmt.Errorf("missing webhook URL (set --webhook-url or CUB_SCOUT_SLACK_WEBHOOK_URL)")
	}

	if err := postSlackPayload(cmd.Context(), webhookURL, payload); err != nil {
		return err
	}

	if err := writeSlackDedupeState(statePath, summarySlackDedupeState{
		LastSignature: digest.Signature,
		LastSentAt:    summaryNowFn().UTC(),
	}); err != nil {
		return err
	}

	fmt.Printf("Posted Slack digest: %d record(s), risk C/W/I=%d/%d/%d, sync failed/out=%d/%d\n",
		digest.Count, digest.Risk.Critical, digest.Risk.Warning, digest.Risk.Info, digest.Sync.Failed, digest.Sync.OutOfSync)
	return nil
}

func buildSlackDigest(entries []summarystore.Record, opts summarySlackDigestOptions) summarySlackDigest {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	clusterSet := map[string]struct{}{}
	scopeSet := map[string]struct{}{}
	top := make([]summarySlackTopEntry, 0)

	digest := summarySlackDigest{
		GeneratedAt: summaryNowFn().UTC(),
		Since:       strings.TrimSpace(opts.Since),
		Type:        strings.TrimSpace(opts.Type),
		Cluster:     strings.TrimSpace(opts.Cluster),
		Namespace:   strings.TrimSpace(opts.Namespace),
		Count:       len(entries),
		NextAction:  buildSlackNextAction(opts),
	}

	for i, entry := range entries {
		cluster := strings.TrimSpace(entry.Cluster)
		if cluster != "" {
			clusterSet[cluster] = struct{}{}
		}
		namespace := strings.TrimSpace(entry.Scope.Namespace)
		if namespace != "" {
			scopeSet[namespace] = struct{}{}
		}

		digest.Risk.Total += entry.Metrics.RiskTotal
		digest.Risk.Critical += entry.Metrics.RiskCritical
		digest.Risk.Warning += entry.Metrics.RiskWarning
		digest.Risk.Info += entry.Metrics.RiskInfo
		digest.Sync.Total += entry.Metrics.SyncTotal
		digest.Sync.Failed += entry.Metrics.SyncFailed
		digest.Sync.OutOfSync += entry.Metrics.SyncOutOfSync
		digest.Drift.Total += entry.Metrics.DriftTotal

		if i < opts.BatchSize {
			top = append(top, summarySlackTopEntry{
				When:      entry.Timestamp.UTC().Format(time.RFC3339),
				Type:      entry.Type,
				Cluster:   entry.Cluster,
				Namespace: namespace,
				Summary:   summarizeSlackEntry(entry),
			})
		}
	}

	digest.Clusters = sortedKeys(clusterSet)
	digest.Scopes = sortedKeys(scopeSet)
	digest.Top = top
	digest.Signature = signSlackDigest(digest)

	return digest
}

func summarizeSlackEntry(entry summarystore.Record) string {
	parts := make([]string, 0, 3)
	if entry.Metrics.RiskTotal > 0 {
		parts = append(parts, fmt.Sprintf("risk C/W/I %d/%d/%d", entry.Metrics.RiskCritical, entry.Metrics.RiskWarning, entry.Metrics.RiskInfo))
	}
	if entry.Metrics.SyncTotal > 0 || entry.Metrics.SyncFailed > 0 || entry.Metrics.SyncOutOfSync > 0 {
		parts = append(parts, fmt.Sprintf("sync failed/out %d/%d", entry.Metrics.SyncFailed, entry.Metrics.SyncOutOfSync))
	}
	if entry.Metrics.DriftTotal > 0 {
		parts = append(parts, fmt.Sprintf("drift %d", entry.Metrics.DriftTotal))
	}
	if len(parts) == 0 {
		return "no drift/sync/risk signals"
	}
	return strings.Join(parts, "; ")
}

func buildSlackNextAction(opts summarySlackDigestOptions) string {
	args := []string{"cub-scout", "summary", "list", "--since", strings.TrimSpace(opts.Since), "--format", "md"}
	if value := strings.TrimSpace(opts.Type); value != "" {
		args = append(args, "--type", value)
	}
	if value := strings.TrimSpace(opts.Cluster); value != "" {
		args = append(args, "--cluster", value)
	}
	if value := strings.TrimSpace(opts.Namespace); value != "" {
		args = append(args, "--namespace", value)
	}
	return strings.Join(args, " ")
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func signSlackDigest(digest summarySlackDigest) string {
	payload := struct {
		Count   int
		Risk    summarySlackRiskTotals
		Sync    summarySlackSyncTotals
		Drift   summarySlackDriftTotals
		Since   string
		Type    string
		Cluster string
		Scope   string
		Top     []summarySlackTopEntry
	}{
		Count:   digest.Count,
		Risk:    digest.Risk,
		Sync:    digest.Sync,
		Drift:   digest.Drift,
		Since:   digest.Since,
		Type:    digest.Type,
		Cluster: digest.Cluster,
		Scope:   digest.Namespace,
		Top:     digest.Top,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func buildSlackPayload(digest summarySlackDigest) slackWebhookPayload {
	clusters := "-"
	if len(digest.Clusters) > 0 {
		clusters = strings.Join(digest.Clusters, ", ")
	}
	scopes := "-"
	if len(digest.Scopes) > 0 {
		scopes = strings.Join(digest.Scopes, ", ")
	}

	topLines := make([]string, 0, len(digest.Top))
	for _, entry := range digest.Top {
		namespace := entry.Namespace
		if namespace == "" {
			namespace = "-"
		}
		topLines = append(topLines, fmt.Sprintf("• `%s` `%s` `%s/%s` — %s", entry.When, entry.Type, entry.Cluster, namespace, entry.Summary))
	}
	if len(topLines) == 0 {
		topLines = append(topLines, "• No records in selected window")
	}

	text := fmt.Sprintf("Connected digest (%s): %d record(s), risk C/W/I=%d/%d/%d, sync failed/out=%d/%d, drift=%d",
		digest.Since,
		digest.Count,
		digest.Risk.Critical,
		digest.Risk.Warning,
		digest.Risk.Info,
		digest.Sync.Failed,
		digest.Sync.OutOfSync,
		digest.Drift.Total,
	)

	return slackWebhookPayload{
		Text: text,
		Blocks: []slackBlock{
			{
				Type: "header",
				Text: &slackTextObject{Type: "plain_text", Text: "cub-scout Connected Digest"},
			},
			{
				Type: "section",
				Text: &slackTextObject{Type: "mrkdwn", Text: fmt.Sprintf("*Window:* `%s`\n*Records:* `%d`\n*Clusters:* `%s`\n*Scopes:* `%s`", digest.Since, digest.Count, clusters, scopes)},
			},
			{
				Type: "section",
				Fields: []slackTextObject{
					{Type: "mrkdwn", Text: fmt.Sprintf("*Risk (C/W/I)*\n`%d / %d / %d`", digest.Risk.Critical, digest.Risk.Warning, digest.Risk.Info)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Sync (failed/out)*\n`%d / %d`", digest.Sync.Failed, digest.Sync.OutOfSync)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Drift*\n`%d`", digest.Drift.Total)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Next Action*\n`%s`", digest.NextAction)},
				},
			},
			{
				Type: "section",
				Text: &slackTextObject{Type: "mrkdwn", Text: "*Top Signals*\n" + strings.Join(topLines, "\n")},
			},
		},
	}
}

func postSlackPayload(ctx context.Context, webhookURL string, payload slackWebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal Slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := summarySlackHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post Slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("Slack webhook returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func summarySlackStatePath() (string, error) {
	dir, err := defaultSummaryStoreDir()
	if err != nil {
		return "", fmt.Errorf("resolve summary store directory: %w", err)
	}
	return filepath.Join(dir, ".slack-dedupe-v1.json"), nil
}

func shouldSkipSlackPost(path, signature string, now time.Time, window time.Duration, force bool) (bool, error) {
	if force {
		return false, nil
	}
	state, err := readSlackDedupeState(path)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(state.LastSignature) == "" {
		return false, nil
	}
	if state.LastSignature != signature {
		return false, nil
	}
	if state.LastSentAt.IsZero() {
		return false, nil
	}
	if now.UTC().Sub(state.LastSentAt.UTC()) < window {
		return true, nil
	}
	return false, nil
}

func readSlackDedupeState(path string) (summarySlackDedupeState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return summarySlackDedupeState{}, nil
		}
		return summarySlackDedupeState{}, fmt.Errorf("read dedupe state: %w", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return summarySlackDedupeState{}, nil
	}
	var state summarySlackDedupeState
	if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
		return summarySlackDedupeState{}, fmt.Errorf("parse dedupe state: %w", err)
	}
	return state, nil
}

func writeSlackDedupeState(path string, state summarySlackDedupeState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dedupe state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dedupe state: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write dedupe state: %w", err)
	}
	return nil
}
