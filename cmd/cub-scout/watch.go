// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	watchWebhookURL      string
	watchInterval        time.Duration
	watchNamespace       string
	watchOwner           string
	watchSeverity        string
	watchOnce            bool
	watchMaxQueuedEvents int
)

type watchEvent struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Resource  watchEventResource     `json:"resource"`
	Owner     *watchEventOwner       `json:"owner,omitempty"`
	Severity  string                 `json:"severity,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type watchEventResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type watchEventOwner struct {
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
}

type watchFinding struct {
	Key       string
	Category  string
	Severity  string
	Kind      string
	Name      string
	Namespace string
	Message   string
}

type watchState struct {
	entriesByID map[string]MapEntry
	findings    map[string]watchFinding
}

var (
	watchBuildConfig   = buildConfig
	watchCollectState  = collectWatchState
	watchPostEvent     = postWatchEvent
	watchEventNow      = time.Now
	watchSleepWithCtx  = sleepWithContext
	watchDefaultClient = &http.Client{Timeout: 10 * time.Second}
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stream observation events to a webhook",
	Long: `Watch cluster observation changes and stream JSON events to a webhook.

Event types:
  - resource.discovered
  - ownership.changed
  - drift.detected
  - scan.finding
`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().StringVar(&watchWebhookURL, "webhook", "", "Webhook URL to receive events")
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 20*time.Second, "Polling interval (for example 20s, 1m)")
	watchCmd.Flags().StringVarP(&watchNamespace, "namespace", "n", "", "Filter events to a namespace")
	watchCmd.Flags().StringVar(&watchOwner, "owner", "", "Filter events by owner display name (for example Flux, ArgoCD, Internal Platform)")
	watchCmd.Flags().StringVar(&watchSeverity, "severity", "", "Filter finding/drift events by severity (comma-separated: critical,warning,info)")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "Run one collection cycle and exit")
	watchCmd.Flags().IntVar(&watchMaxQueuedEvents, "max-queued-events", 1000, "Maximum buffered events when webhook is unavailable")
}

func runWatch(cmd *cobra.Command, args []string) error {
	webhookURL := strings.TrimSpace(watchWebhookURL)
	if webhookURL == "" {
		return fmt.Errorf("missing --webhook URL")
	}
	if watchInterval <= 0 {
		return fmt.Errorf("--interval must be > 0")
	}
	if watchMaxQueuedEvents <= 0 {
		return fmt.Errorf("--max-queued-events must be > 0")
	}

	severityFilter := parseWatchSeverityFilter(watchSeverity)
	ownerFilter := strings.TrimSpace(watchOwner)

	cfg, err := watchBuildConfig()
	if err != nil {
		return withKubeRecoveryHint(fmt.Errorf("build kubernetes config: %w", err), "cub-scout watch")
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return withKubeRecoveryHint(fmt.Errorf("create dynamic client: %w", err), "cub-scout watch")
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		prevState watchState
		queue     []watchEvent
	)

	if watchOnce {
		curr, err := watchCollectState(ctx, dynClient, watchNamespace)
		if err != nil {
			return err
		}
		events := buildWatchEvents(prevState, curr, severityFilter, ownerFilter, watchEventNow)
		queue = appendWatchQueue(queue, events, watchMaxQueuedEvents)
		_, err = flushWatchQueue(ctx, webhookURL, queue)
		return err
	}

	// Prime baseline to avoid spamming initial full-state events in long-running mode.
	initial, err := watchCollectState(ctx, dynClient, watchNamespace)
	if err != nil {
		return err
	}
	prevState = initial

	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			curr, err := watchCollectState(ctx, dynClient, watchNamespace)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: watch collection failed: %v\n", err)
				continue
			}
			events := buildWatchEvents(prevState, curr, severityFilter, ownerFilter, watchEventNow)
			queue = appendWatchQueue(queue, events, watchMaxQueuedEvents)
			remaining, err := flushWatchQueue(ctx, webhookURL, queue)
			queue = remaining
			prevState = curr
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: webhook delivery failed (buffered %d events): %v\n", len(queue), err)
				continue
			}
		}
	}
}

func collectWatchState(ctx context.Context, dynClient dynamic.Interface, namespace string) (watchState, error) {
	entries, err := collectWatchEntries(ctx, dynClient, namespace)
	if err != nil {
		return watchState{}, err
	}
	findings, err := collectWatchFindings(ctx, dynClient, namespace)
	if err != nil {
		return watchState{}, err
	}

	state := watchState{
		entriesByID: make(map[string]MapEntry, len(entries)),
		findings:    make(map[string]watchFinding, len(findings)),
	}
	for _, entry := range entries {
		state.entriesByID[entry.ID] = entry
	}
	for _, finding := range findings {
		state.findings[finding.Key] = finding
	}
	return state, nil
}

func collectWatchEntries(ctx context.Context, dynClient dynamic.Interface, namespace string) ([]MapEntry, error) {
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}

	appSetLookup := loadMapApplicationSetLookup(ctx, dynClient, namespace)

	entries := make([]MapEntry, 0, 256)
	byOwner := map[string]int{}
	for _, gvr := range resources {
		if namespace != "" {
			l, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				entries = processResourceWithLookup(&item, gvr, clusterName, entries, byOwner, appSetLookup)
			}
			continue
		}

		l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range l.Items {
			entries = processResourceWithLookup(&item, gvr, clusterName, entries, byOwner, appSetLookup)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func collectWatchFindings(ctx context.Context, dynClient dynamic.Interface, namespace string) ([]watchFinding, error) {
	scanner := agent.NewStateScannerWithClient(dynClient)

	var (
		res *agent.StateScanResult
		err error
	)
	if namespace != "" {
		res, err = scanner.ScanNamespace(ctx, namespace)
	} else {
		res, err = scanner.Scan(ctx)
	}
	if err != nil {
		return nil, err
	}

	out := make([]watchFinding, 0, len(res.Findings)+len(res.RuntimeFindings))
	for _, finding := range res.Findings {
		key := fmt.Sprintf("%s|%s|%s|%s|%s", finding.CCVEID, finding.Kind, finding.Namespace, finding.Name, finding.Reason)
		out = append(out, watchFinding{
			Key:       key,
			Category:  finding.Category,
			Severity:  normalizeFindingSeverity(finding.Severity),
			Kind:      finding.Kind,
			Name:      finding.Name,
			Namespace: finding.Namespace,
			Message:   finding.Message,
		})
	}
	for _, finding := range res.RuntimeFindings {
		key := fmt.Sprintf("%s|%s|%s|%s|%s", finding.CCVEID, finding.Kind, finding.Namespace, finding.Name, finding.FailureType)
		out = append(out, watchFinding{
			Key:       key,
			Category:  finding.Category,
			Severity:  normalizeFindingSeverity(finding.Severity),
			Kind:      finding.Kind,
			Name:      finding.Name,
			Namespace: finding.Namespace,
			Message:   finding.Message,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func buildWatchEvents(prev, curr watchState, severityFilter map[string]struct{}, ownerFilter string, nowFn func() time.Time) []watchEvent {
	ts := nowFn().UTC()
	events := make([]watchEvent, 0)

	for id, entry := range curr.entriesByID {
		prevEntry, existed := prev.entriesByID[id]
		if !existed {
			event := watchEvent{
				Type:      "resource.discovered",
				Timestamp: ts,
				Resource: watchEventResource{
					Kind:      entry.Kind,
					Name:      entry.Name,
					Namespace: entry.Namespace,
				},
				Owner: watchOwnerFromEntry(entry),
				Details: map[string]interface{}{
					"status": entry.Status,
				},
			}
			if shouldEmitWatchEvent(event, severityFilter, ownerFilter) {
				events = append(events, event)
			}
			continue
		}

		if prevEntry.Owner != entry.Owner {
			event := watchEvent{
				Type:      "ownership.changed",
				Timestamp: ts,
				Resource: watchEventResource{
					Kind:      entry.Kind,
					Name:      entry.Name,
					Namespace: entry.Namespace,
				},
				Owner: watchOwnerFromEntry(entry),
				Details: map[string]interface{}{
					"before": prevEntry.Owner,
					"after":  entry.Owner,
				},
			}
			if shouldEmitWatchEvent(event, severityFilter, ownerFilter) {
				events = append(events, event)
			}
		}
	}

	for key, finding := range curr.findings {
		if _, alreadySeen := prev.findings[key]; alreadySeen {
			continue
		}
		entry := findWatchEntryForResource(curr.entriesByID, finding.Namespace, finding.Kind, finding.Name)
		owner := watchOwnerFromEntry(entry)

		scanEvent := watchEvent{
			Type:      "scan.finding",
			Timestamp: ts,
			Resource:  watchEventResource{Kind: finding.Kind, Name: finding.Name, Namespace: finding.Namespace},
			Owner:     owner,
			Severity:  finding.Severity,
			Details: map[string]interface{}{
				"category": finding.Category,
				"message":  finding.Message,
			},
		}
		if shouldEmitWatchEvent(scanEvent, severityFilter, ownerFilter) {
			events = append(events, scanEvent)
		}

		if strings.EqualFold(finding.Category, "STATE") || strings.EqualFold(finding.Category, "DRIFT") {
			driftEvent := watchEvent{
				Type:      "drift.detected",
				Timestamp: ts,
				Resource:  watchEventResource{Kind: finding.Kind, Name: finding.Name, Namespace: finding.Namespace},
				Owner:     owner,
				Severity:  finding.Severity,
				Details: map[string]interface{}{
					"category": finding.Category,
					"message":  finding.Message,
				},
			}
			if shouldEmitWatchEvent(driftEvent, severityFilter, ownerFilter) {
				events = append(events, driftEvent)
			}
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Type != events[j].Type {
			return events[i].Type < events[j].Type
		}
		if events[i].Resource.Namespace != events[j].Resource.Namespace {
			return events[i].Resource.Namespace < events[j].Resource.Namespace
		}
		if events[i].Resource.Kind != events[j].Resource.Kind {
			return events[i].Resource.Kind < events[j].Resource.Kind
		}
		return events[i].Resource.Name < events[j].Resource.Name
	})

	return events
}

func watchOwnerFromEntry(entry MapEntry) *watchEventOwner {
	if entry.Owner == "" {
		return nil
	}
	owner := &watchEventOwner{Type: entry.Owner}
	if entry.OwnerDetails != nil {
		owner.Name = strings.TrimSpace(entry.OwnerDetails["name"])
	}
	return owner
}

func findWatchEntryForResource(entriesByID map[string]MapEntry, namespace, kind, name string) MapEntry {
	for _, entry := range entriesByID {
		if entry.Namespace == namespace && entry.Kind == kind && entry.Name == name {
			return entry
		}
	}
	return MapEntry{}
}

func normalizeFindingSeverity(severity string) string {
	s := strings.ToLower(strings.TrimSpace(severity))
	if s == "" {
		return "warning"
	}
	return s
}

func parseWatchSeverityFilter(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range strings.Split(raw, ",") {
		v := strings.ToLower(strings.TrimSpace(token))
		if v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	return out
}

func shouldEmitWatchEvent(event watchEvent, severityFilter map[string]struct{}, ownerFilter string) bool {
	if ownerFilter != "" {
		if event.Owner == nil || !strings.EqualFold(strings.TrimSpace(event.Owner.Type), ownerFilter) {
			return false
		}
	}
	if len(severityFilter) == 0 || event.Severity == "" {
		return true
	}
	_, ok := severityFilter[strings.ToLower(event.Severity)]
	return ok
}

func appendWatchQueue(queue, events []watchEvent, maxQueued int) []watchEvent {
	if len(events) == 0 {
		return queue
	}
	queue = append(queue, events...)
	if len(queue) <= maxQueued {
		return queue
	}
	return append([]watchEvent(nil), queue[len(queue)-maxQueued:]...)
}

func flushWatchQueue(ctx context.Context, webhookURL string, queue []watchEvent) ([]watchEvent, error) {
	for i, event := range queue {
		if err := watchPostEvent(ctx, webhookURL, event); err != nil {
			return append([]watchEvent(nil), queue[i:]...), err
		}
	}
	return queue[:0], nil
}

func postWatchEvent(ctx context.Context, webhookURL string, event watchEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("build webhook request: %w", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := watchDefaultClient.Do(req)
		if doErr == nil {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("webhook returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
		} else {
			lastErr = doErr
		}

		if attempt == 3 {
			break
		}
		if err := watchSleepWithCtx(ctx, backoff); err != nil {
			return err
		}
		backoff *= 2
	}
	return fmt.Errorf("post webhook event failed after retries: %w", lastErr)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
