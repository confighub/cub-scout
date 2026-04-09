// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"error", 2},
		{"warning", 1},
		{"info", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityRank(tt.severity)
			if got != tt.want {
				t.Errorf("severityRank(%q) = %d, want %d", tt.severity, got, tt.want)
			}
		})
	}
}

func TestGetSeverity(t *testing.T) {
	tests := []struct {
		eventType string
		reason    string
		want      string
	}{
		{"Warning", "CrashLoopBackOff", "error"},
		{"Warning", "ImagePullBackOff", "error"},
		{"Warning", "FailedScheduling", "error"},
		{"Warning", "SomethingElse", "warning"},
		{"Normal", "Pulled", "info"},
		{"Normal", "Created", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType+"/"+tt.reason, func(t *testing.T) {
			got := getSeverity(tt.eventType, tt.reason)
			if got != tt.want {
				t.Errorf("getSeverity(%q, %q) = %q, want %q", tt.eventType, tt.reason, got, tt.want)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatAge(tt.duration)
			if got != tt.want {
				t.Errorf("formatAge(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestTruncateEventMessage(t *testing.T) {
	tests := []struct {
		msg    string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly ten", 10, "exactly..."},
		{"hello world this is a long message", 20, "hello world this ..."},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := truncateEventMessage(tt.msg, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateEventMessage(%q, %d) = %q, want %q", tt.msg, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestResourceEventSortOrder(t *testing.T) {
	now := time.Now()
	older := now.Add(-1 * time.Hour)
	oldest := now.Add(-2 * time.Hour)

	events := []ResourceEvent{
		{Severity: "info", LastSeen: &oldest, Reason: "Pulled"},
		{Severity: "error", LastSeen: &older, Reason: "CrashLoopBackOff"},
		{Severity: "warning", LastSeen: &now, Reason: "BackOff"},
		{Severity: "info", LastSeen: &now, Reason: "Created"},
	}

	// Sort them
	for i := 0; i < len(events)-1; i++ {
		for j := i + 1; j < len(events); j++ {
			si := severityRank(events[i].Severity)
			sj := severityRank(events[j].Severity)
			if si < sj {
				events[i], events[j] = events[j], events[i]
			} else if si == sj {
				ti := getResourceEventTime(events[i])
				tj := getResourceEventTime(events[j])
				if ti.Before(tj) {
					events[i], events[j] = events[j], events[i]
				}
			}
		}
	}

	// Check order: error first, then warning, then info
	if events[0].Severity != "error" {
		t.Errorf("expected error first, got %s", events[0].Severity)
	}
	if events[1].Severity != "warning" {
		t.Errorf("expected warning second, got %s", events[1].Severity)
	}
	if events[2].Severity != "info" || events[2].Reason != "Created" {
		t.Errorf("expected most recent info third, got %s/%s", events[2].Severity, events[2].Reason)
	}
}
