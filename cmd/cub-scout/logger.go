// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImportLogger logs import operations to a file
type ImportLogger struct {
	file      *os.File
	startTime time.Time
	command   string
}

// NewImportLogger creates a new logger for an import operation
func NewImportLogger(command string) (*ImportLogger, error) {
	// Create .confighub/logs directory
	logDir := ".confighub/logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", command, timestamp))

	file, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	logger := &ImportLogger{
		file:      file,
		startTime: time.Now(),
		command:   command,
	}

	// Write header
	logger.writeHeader()

	return logger, nil
}

func (l *ImportLogger) writeHeader() {
	l.file.WriteString("=" + strings.Repeat("=", 79) + "\n")
	l.file.WriteString(fmt.Sprintf("ConfigHub Agent: %s\n", l.command))
	l.file.WriteString(fmt.Sprintf("Started: %s\n", l.startTime.Format(time.RFC3339)))
	l.file.WriteString("=" + strings.Repeat("=", 79) + "\n\n")
}

// Log writes a message to the log file
func (l *ImportLogger) Log(format string, args ...interface{}) {
	if l == nil || l.file == nil {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	l.file.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, msg))
}

// Section writes a section header
func (l *ImportLogger) Section(title string) {
	if l == nil || l.file == nil {
		return
	}
	l.file.WriteString(fmt.Sprintf("\n--- %s ---\n", title))
}

// LogWorkloads writes discovered workloads
func (l *ImportLogger) LogWorkloads(workloads []WorkloadInfo) {
	if l == nil || l.file == nil {
		return
	}
	l.Section("DISCOVERED WORKLOADS")
	l.Log("Found %d workloads", len(workloads))
	for _, w := range workloads {
		l.Log("  %s/%s (%s) owner=%s ready=%v", w.Namespace, w.Name, w.Kind, w.Owner, w.Ready)
	}
}

// LogProposal writes the proposal details
func (l *ImportLogger) LogProposal(proposal *FullProposal) {
	if l == nil || l.file == nil || proposal == nil {
		return
	}
	l.Section("PROPOSAL")
	l.Log("App Space: %s", proposal.AppSpace)
	l.Log("Units: %d", len(proposal.Units))
	for _, u := range proposal.Units {
		l.Log("  %s (app=%s, variant=%s)", u.Slug, u.App, u.Variant)
		for k, v := range u.Labels {
			l.Log("    label: %s=%s", k, v)
		}
		for _, w := range u.Workloads {
			l.Log("    workload: %s", w)
		}
	}
}

// LogResult writes the operation result
func (l *ImportLogger) LogResult(created, failed int, err error) {
	if l == nil || l.file == nil {
		return
	}
	l.Section("RESULT")
	if err != nil {
		l.Log("ERROR: %v", err)
	}
	l.Log("Created: %d", created)
	l.Log("Failed: %d", failed)
	l.Log("Duration: %s", time.Since(l.startTime).Round(time.Millisecond))
}

// Close closes the log file and prints path
func (l *ImportLogger) Close() string {
	if l == nil || l.file == nil {
		return ""
	}

	// Write footer
	l.file.WriteString(fmt.Sprintf("\n\nCompleted: %s\n", time.Now().Format(time.RFC3339)))
	l.file.WriteString(fmt.Sprintf("Duration: %s\n", time.Since(l.startTime).Round(time.Millisecond)))

	path := l.file.Name()
	l.file.Close()
	return path
}
