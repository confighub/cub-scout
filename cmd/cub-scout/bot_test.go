// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestBotWatchOptionsFromFlags_UsesEnvFallbacks(t *testing.T) {
	restore := overrideBotGlobals(t)
	defer restore()

	t.Setenv(botWebhookURLEnv, "https://events.example.com/cub-scout")
	t.Setenv(botIntervalEnv, "45s")
	t.Setenv(botNamespaceEnv, "prod")
	t.Setenv(botSeverityEnv, "warning,critical")
	t.Setenv(botMaxQueuedEventsEnv, "25")
	t.Setenv(botEmitReceiptOnEnv, "all")
	t.Setenv(botEmitReceiptBatchCapEnv, "3")

	opts, err := botWatchOptionsFromFlags(&cobra.Command{})
	if err != nil {
		t.Fatalf("botWatchOptionsFromFlags() error = %v", err)
	}
	if opts.WebhookURL != "https://events.example.com/cub-scout" {
		t.Fatalf("webhook = %q", opts.WebhookURL)
	}
	if opts.Interval != 45*time.Second {
		t.Fatalf("interval = %s, want 45s", opts.Interval)
	}
	if opts.Namespace != "prod" || opts.Severity != "warning,critical" {
		t.Fatalf("scope/filter = namespace %q severity %q", opts.Namespace, opts.Severity)
	}
	if opts.MaxQueuedEvents != 25 || opts.EmitReceiptBatchCap != 3 {
		t.Fatalf("caps = queue %d receipt %d", opts.MaxQueuedEvents, opts.EmitReceiptBatchCap)
	}
	if opts.EmitReceiptOn != "all" || opts.CommandName != "cub-scout bot" {
		t.Fatalf("receipt/command = %q/%q", opts.EmitReceiptOn, opts.CommandName)
	}
}

func TestBotWatchOptionsFromFlags_InvalidEnvIsError(t *testing.T) {
	restore := overrideBotGlobals(t)
	defer restore()

	t.Setenv(botIntervalEnv, "not-a-duration")

	_, err := botWatchOptionsFromFlags(&cobra.Command{})
	if err == nil {
		t.Fatal("expected invalid env error")
	}
	if !strings.Contains(err.Error(), botIntervalEnv) {
		t.Fatalf("error = %v, want env name", err)
	}
}

func TestRunBotRequiresDestinationBeforeClusterAccess(t *testing.T) {
	restore := overrideBotGlobals(t)
	defer restore()

	err := runBot(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected destination error")
	}
	if !strings.Contains(err.Error(), "missing bot destination") {
		t.Fatalf("error = %v, want missing destination", err)
	}
}

func overrideBotGlobals(t *testing.T) func() {
	t.Helper()
	oldWebhookURL := botWebhookURL
	oldOutputFile := botOutputFile
	oldInterval := botInterval
	oldNamespace := botNamespace
	oldOwner := botOwner
	oldSeverity := botSeverity
	oldOnce := botOnce
	oldMaxQueuedEvents := botMaxQueuedEvents
	oldEmitReceiptOn := botEmitReceiptOn
	oldEmitReceiptBatchCap := botEmitReceiptBatchCap

	botWebhookURL = ""
	botOutputFile = ""
	botInterval = 30 * time.Second
	botNamespace = ""
	botOwner = ""
	botSeverity = ""
	botOnce = false
	botMaxQueuedEvents = 1000
	botEmitReceiptOn = ""
	botEmitReceiptBatchCap = 10

	return func() {
		botWebhookURL = oldWebhookURL
		botOutputFile = oldOutputFile
		botInterval = oldInterval
		botNamespace = oldNamespace
		botOwner = oldOwner
		botSeverity = oldSeverity
		botOnce = oldOnce
		botMaxQueuedEvents = oldMaxQueuedEvents
		botEmitReceiptOn = oldEmitReceiptOn
		botEmitReceiptBatchCap = oldEmitReceiptBatchCap
	}
}
