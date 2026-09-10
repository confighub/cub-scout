// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	botWebhookURLEnv          = "CUB_SCOUT_BOT_WEBHOOK_URL"
	botOutputFileEnv          = "CUB_SCOUT_BOT_OUTPUT_FILE"
	botIntervalEnv            = "CUB_SCOUT_BOT_INTERVAL"
	botNamespaceEnv           = "CUB_SCOUT_BOT_NAMESPACE"
	botOwnerEnv               = "CUB_SCOUT_BOT_OWNER"
	botSeverityEnv            = "CUB_SCOUT_BOT_SEVERITY"
	botMaxQueuedEventsEnv     = "CUB_SCOUT_BOT_MAX_QUEUED_EVENTS"
	botEmitReceiptOnEnv       = "CUB_SCOUT_BOT_EMIT_RECEIPT_ON"
	botEmitReceiptBatchCapEnv = "CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP"
)

var (
	botWebhookURL          string
	botOutputFile          string
	botInterval            time.Duration
	botNamespace           string
	botOwner               string
	botSeverity            string
	botOnce                bool
	botMaxQueuedEvents     int
	botEmitReceiptOn       string
	botEmitReceiptBatchCap int
)

var botCmd = &cobra.Command{
	Use:   "bot",
	Short: "Run cub-scout as an in-cluster observation bot",
	Long: `Run cub-scout as a long-running read-only observation bot.

bot mode is a packaged entrypoint for running the existing watch engine inside
Kubernetes. It uses in-cluster authentication when available, falls back to
KUBECONFIG outside the cluster, and streams the same JSON watch events to a
webhook or JSONL file sink.

Environment variables mirror the flags for Kubernetes manifests:
  CUB_SCOUT_BOT_WEBHOOK_URL
  CUB_SCOUT_BOT_OUTPUT_FILE
  CUB_SCOUT_BOT_INTERVAL
  CUB_SCOUT_BOT_NAMESPACE
  CUB_SCOUT_BOT_OWNER
  CUB_SCOUT_BOT_SEVERITY
  CUB_SCOUT_BOT_MAX_QUEUED_EVENTS
  CUB_SCOUT_BOT_EMIT_RECEIPT_ON
  CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP
`,
	RunE: runBot,
}

func init() {
	rootCmd.AddCommand(botCmd)
	botCmd.Flags().StringVar(&botWebhookURL, "webhook", "", "Webhook URL to receive events (or CUB_SCOUT_BOT_WEBHOOK_URL)")
	botCmd.Flags().StringVar(&botOutputFile, "output-file", "", "Append JSONL events to a file path (or CUB_SCOUT_BOT_OUTPUT_FILE)")
	botCmd.Flags().DurationVar(&botInterval, "interval", 30*time.Second, "Polling interval (or CUB_SCOUT_BOT_INTERVAL)")
	botCmd.Flags().StringVarP(&botNamespace, "namespace", "n", "", "Filter events to a namespace (or CUB_SCOUT_BOT_NAMESPACE)")
	botCmd.Flags().StringVar(&botOwner, "owner", "", "Filter events by owner display name (or CUB_SCOUT_BOT_OWNER)")
	botCmd.Flags().StringVar(&botSeverity, "severity", "", "Filter finding/drift events by severity (or CUB_SCOUT_BOT_SEVERITY)")
	botCmd.Flags().BoolVar(&botOnce, "once", false, "Run one collection cycle and exit")
	botCmd.Flags().IntVar(&botMaxQueuedEvents, "max-queued-events", 1000, "Maximum buffered events when webhook is unavailable (or CUB_SCOUT_BOT_MAX_QUEUED_EVENTS)")
	botCmd.Flags().StringVar(&botEmitReceiptOn, "emit-receipt-on", "", "Comma-separated watch event types to attach a receipt to (or CUB_SCOUT_BOT_EMIT_RECEIPT_ON)")
	botCmd.Flags().IntVar(&botEmitReceiptBatchCap, "emit-receipt-batch-cap", 10, "Per-poll cap on receipt-build attempts (or CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP)")
}

func runBot(cmd *cobra.Command, args []string) error {
	opts, err := botWatchOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.WebhookURL) == "" && strings.TrimSpace(opts.OutputFile) == "" {
		return fmt.Errorf("missing bot destination: provide --webhook, --output-file, %s, or %s", botWebhookURLEnv, botOutputFileEnv)
	}
	return runWatchWithOptions(cmd, opts)
}

func botWatchOptionsFromFlags(cmd *cobra.Command) (watchOptions, error) {
	interval, err := botDurationValue(cmd, "interval", botInterval, botIntervalEnv)
	if err != nil {
		return watchOptions{}, err
	}
	maxQueuedEvents, err := botIntValue(cmd, "max-queued-events", botMaxQueuedEvents, botMaxQueuedEventsEnv)
	if err != nil {
		return watchOptions{}, err
	}
	emitReceiptBatchCap, err := botIntValue(cmd, "emit-receipt-batch-cap", botEmitReceiptBatchCap, botEmitReceiptBatchCapEnv)
	if err != nil {
		return watchOptions{}, err
	}

	return watchOptions{
		WebhookURL:          botStringValue(cmd, "webhook", botWebhookURL, botWebhookURLEnv),
		OutputFile:          botStringValue(cmd, "output-file", botOutputFile, botOutputFileEnv),
		Interval:            interval,
		Namespace:           botStringValue(cmd, "namespace", botNamespace, botNamespaceEnv),
		Owner:               botStringValue(cmd, "owner", botOwner, botOwnerEnv),
		Severity:            botStringValue(cmd, "severity", botSeverity, botSeverityEnv),
		Once:                botOnce,
		MaxQueuedEvents:     maxQueuedEvents,
		EmitReceiptOn:       botStringValue(cmd, "emit-receipt-on", botEmitReceiptOn, botEmitReceiptOnEnv),
		EmitReceiptBatchCap: emitReceiptBatchCap,
		CommandName:         "cub-scout bot",
	}, nil
}

func botStringValue(cmd *cobra.Command, flagName, flagValue, envName string) string {
	if cmd != nil && cmd.Flags().Changed(flagName) {
		return strings.TrimSpace(flagValue)
	}
	return strings.TrimSpace(firstNonEmpty(flagValue, os.Getenv(envName)))
}

func botDurationValue(cmd *cobra.Command, flagName string, flagValue time.Duration, envName string) (time.Duration, error) {
	if cmd != nil && cmd.Flags().Changed(flagName) {
		return flagValue, nil
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return flagValue, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", envName, err)
	}
	return value, nil
}

func botIntValue(cmd *cobra.Command, flagName string, flagValue int, envName string) (int, error) {
	if cmd != nil && cmd.Flags().Changed(flagName) {
		return flagValue, nil
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return flagValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", envName, err)
	}
	return value, nil
}
