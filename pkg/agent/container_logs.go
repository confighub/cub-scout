// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ContainerLogResult holds the result of fetching container logs
type ContainerLogResult struct {
	PodName       string          `json:"podName"`
	ContainerName string          `json:"containerName"`
	Namespace     string          `json:"namespace"`
	Lines         []string        `json:"lines"`
	TotalLines    int             `json:"totalLines"`
	Truncated     bool            `json:"truncated"`
	Previous      bool            `json:"previous"` // Whether these are logs from previous container
	Patterns      []LogPattern    `json:"patterns,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// LogPattern represents a detected pattern in logs
type LogPattern struct {
	Type        string `json:"type"`        // error_type, config_missing, connection_refused, etc.
	Line        int    `json:"line"`        // Line number where pattern was found
	Match       string `json:"match"`       // The matching text
	Explanation string `json:"explanation"` // Human-readable explanation
	Suggestion  string `json:"suggestion"`  // Suggested action
}

// ContainerLogFetcher fetches and analyzes container logs
type ContainerLogFetcher struct {
	clientset *kubernetes.Clientset
}

// NewContainerLogFetcher creates a new log fetcher
func NewContainerLogFetcher(clientset *kubernetes.Clientset) *ContainerLogFetcher {
	return &ContainerLogFetcher{
		clientset: clientset,
	}
}

// FetchLogs fetches logs for a container
func (f *ContainerLogFetcher) FetchLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64, previous bool) (*ContainerLogResult, error) {
	result := &ContainerLogResult{
		PodName:       podName,
		ContainerName: containerName,
		Namespace:     namespace,
		Previous:      previous,
	}

	opts := &corev1.PodLogOptions{
		Container: containerName,
		TailLines: &tailLines,
		Previous:  previous,
	}

	req := f.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil // Return result with error, don't fail
	}
	defer stream.Close()

	// Read logs
	lines, err := readLogLines(stream, int(tailLines))
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Lines = lines
	result.TotalLines = len(lines)
	result.Truncated = len(lines) >= int(tailLines)

	// Detect patterns
	result.Patterns = detectLogPatterns(lines)

	return result, nil
}

// FetchLogsForPod fetches logs for all containers in a pod
func (f *ContainerLogFetcher) FetchLogsForPod(ctx context.Context, namespace, podName string, tailLines int64, previous bool) ([]*ContainerLogResult, error) {
	// Get pod to find container names
	pod, err := f.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	var results []*ContainerLogResult

	// Fetch logs for each container
	for _, container := range pod.Spec.Containers {
		result, err := f.FetchLogs(ctx, namespace, podName, container.Name, tailLines, previous)
		if err != nil {
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// readLogLines reads log lines from a stream
func readLogLines(stream io.Reader, maxLines int) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(stream)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= maxLines {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return lines, err
	}

	return lines, nil
}

// LogPatternMatcher defines a pattern to look for in logs
type LogPatternMatcher struct {
	Name        string
	Regex       *regexp.Regexp
	Type        string
	Explanation string
	Suggestion  string
}

// Common log patterns to detect
var logPatternMatchers = []LogPatternMatcher{
	{
		Name:        "connection_refused",
		Regex:       regexp.MustCompile(`(?i)(connection refused|ECONNREFUSED|connect: connection refused)`),
		Type:        "connection_refused",
		Explanation: "The application cannot connect to a dependent service",
		Suggestion:  "Check that the target service is running and the endpoint is correct",
	},
	{
		Name:        "file_not_found",
		Regex:       regexp.MustCompile(`(?i)(no such file|file not found|ENOENT|FileNotFoundError)`),
		Type:        "file_not_found",
		Explanation: "A required file or path does not exist",
		Suggestion:  "Check that ConfigMaps/Secrets are mounted correctly and paths are valid",
	},
	{
		Name:        "permission_denied",
		Regex:       regexp.MustCompile(`(?i)(permission denied|EACCES|access denied|forbidden)`),
		Type:        "permission_denied",
		Explanation: "The application lacks permission to access a resource",
		Suggestion:  "Check file permissions, security context, and RBAC settings",
	},
	{
		Name:        "out_of_memory",
		Regex:       regexp.MustCompile(`(?i)(out of memory|OOM|cannot allocate memory|MemoryError|heap out of memory)`),
		Type:        "out_of_memory",
		Explanation: "The application ran out of memory",
		Suggestion:  "Increase memory limits or optimize application memory usage",
	},
	{
		Name:        "database_error",
		Regex:       regexp.MustCompile(`(?i)(database.*error|db.*error|sql.*error|connection.*database|SQLSTATE)`),
		Type:        "database_error",
		Explanation: "Database connection or query error",
		Suggestion:  "Check database credentials, connectivity, and query syntax",
	},
	{
		Name:        "authentication_failed",
		Regex:       regexp.MustCompile(`(?i)(authentication failed|unauthorized|401|invalid.*token|invalid.*credential)`),
		Type:        "authentication_failed",
		Explanation: "Authentication to an external service failed",
		Suggestion:  "Check that credentials/tokens are correct and not expired",
	},
	{
		Name:        "timeout",
		Regex:       regexp.MustCompile(`(?i)(timeout|timed out|deadline exceeded|ETIMEDOUT)`),
		Type:        "timeout",
		Explanation: "An operation timed out waiting for a response",
		Suggestion:  "Check network connectivity and increase timeout if needed",
	},
	{
		Name:        "config_missing",
		Regex:       regexp.MustCompile(`(?i)(config.*not found|missing.*config|cannot.*load.*config|configuration.*error)`),
		Type:        "config_missing",
		Explanation: "Required configuration is missing or invalid",
		Suggestion:  "Check that ConfigMaps are created and mounted at the expected path",
	},
	{
		Name:        "secret_missing",
		Regex:       regexp.MustCompile(`(?i)(secret.*not found|missing.*secret|cannot.*load.*secret)`),
		Type:        "secret_missing",
		Explanation: "Required secret is missing",
		Suggestion:  "Check that Secrets are created and mounted correctly",
	},
	{
		Name:        "dns_error",
		Regex:       regexp.MustCompile(`(?i)(DNS.*error|no such host|NXDOMAIN|name.*resolution|getaddrinfo)`),
		Type:        "dns_error",
		Explanation: "DNS resolution failed",
		Suggestion:  "Check that the hostname is correct and DNS is working",
	},
	{
		Name:        "port_in_use",
		Regex:       regexp.MustCompile(`(?i)(address already in use|EADDRINUSE|port.*already.*bound)`),
		Type:        "port_in_use",
		Explanation: "The port the application wants to use is already taken",
		Suggestion:  "Check for duplicate pods or conflicting services",
	},
	{
		Name:        "panic",
		Regex:       regexp.MustCompile(`(?i)(panic:|runtime error:|SIGSEGV|segmentation fault)`),
		Type:        "panic",
		Explanation: "The application crashed with a panic or runtime error",
		Suggestion:  "Check the stack trace above this line for the root cause",
	},
	{
		Name:        "fatal_error",
		Regex:       regexp.MustCompile(`(?i)(FATAL|CRITICAL|fatal error)`),
		Type:        "fatal_error",
		Explanation: "A fatal error occurred causing the application to exit",
		Suggestion:  "Check the error message for details on what failed",
	},
}

// detectLogPatterns scans log lines for known patterns
func detectLogPatterns(lines []string) []LogPattern {
	var patterns []LogPattern
	seen := make(map[string]bool) // Deduplicate patterns

	for lineNum, line := range lines {
		for _, matcher := range logPatternMatchers {
			if seen[matcher.Type] {
				continue // Only report first occurrence of each pattern type
			}

			if match := matcher.Regex.FindString(line); match != "" {
				patterns = append(patterns, LogPattern{
					Type:        matcher.Type,
					Line:        lineNum + 1, // 1-indexed
					Match:       truncateMatch(match, 60),
					Explanation: matcher.Explanation,
					Suggestion:  matcher.Suggestion,
				})
				seen[matcher.Type] = true
			}
		}
	}

	return patterns
}

// truncateMatch truncates a match string to maxLen
func truncateMatch(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// GetPatternExplanation returns the explanation for a pattern type
func GetPatternExplanation(patternType string) string {
	for _, m := range logPatternMatchers {
		if m.Type == patternType {
			return m.Explanation
		}
	}
	return ""
}

// GetPatternSuggestion returns the suggestion for a pattern type
func GetPatternSuggestion(patternType string) string {
	for _, m := range logPatternMatchers {
		if m.Type == patternType {
			return m.Suggestion
		}
	}
	return ""
}
