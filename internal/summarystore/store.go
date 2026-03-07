package summarystore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// SchemaVersion is the versioned schema for connected summary records.
	SchemaVersion = "connected.summary.v1"

	defaultRetentionDays = 30
)

// Scope identifies the namespace/resource scope of a summary record.
type Scope struct {
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource,omitempty"`
}

// Metrics stores normalized drift/sync/risk counters.
type Metrics struct {
	RiskTotal    int `json:"riskTotal,omitempty"`
	RiskCritical int `json:"riskCritical,omitempty"`
	RiskWarning  int `json:"riskWarning,omitempty"`
	RiskInfo     int `json:"riskInfo,omitempty"`

	SyncTotal     int `json:"syncTotal,omitempty"`
	SyncFailed    int `json:"syncFailed,omitempty"`
	SyncOutOfSync int `json:"syncOutOfSync,omitempty"`

	DriftTotal int `json:"driftTotal,omitempty"`
}

// Record is one persisted connected summary artifact.
type Record struct {
	SchemaVersion string    `json:"schemaVersion"`
	Timestamp     time.Time `json:"timestamp"`
	Type          string    `json:"type"`
	Cluster       string    `json:"cluster"`
	Scope         Scope     `json:"scope,omitempty"`
	Metrics       Metrics   `json:"metrics"`
	Source        string    `json:"source,omitempty"`
}

// Query filters persisted records.
type Query struct {
	Since     time.Time
	Until     time.Time
	Type      string
	Cluster   string
	Namespace string
	Limit     int
}

// Options configures store creation.
type Options struct {
	RootDir       string
	RetentionDays int
	Now           func() time.Time
}

// Store persists and queries connected summary records.
type Store struct {
	rootDir       string
	retentionDays int
	now           func() time.Time
}

// DefaultRootDir returns the default persistent directory.
func DefaultRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".confighub", "cub-scout", "summaries"), nil
}

// NewDefault creates a store with default root and retention settings.
func NewDefault() (*Store, error) {
	root, err := DefaultRootDir()
	if err != nil {
		return nil, err
	}
	return New(Options{RootDir: root})
}

// New creates a configured store.
func New(opts Options) (*Store, error) {
	root := strings.TrimSpace(opts.RootDir)
	if root == "" {
		return nil, fmt.Errorf("summary store root directory is required")
	}

	retention := opts.RetentionDays
	if retention <= 0 {
		retention = defaultRetentionDays
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &Store{
		rootDir:       root,
		retentionDays: retention,
		now:           nowFn,
	}, nil
}

// Write appends a new summary record and prunes expired files.
func (s *Store) Write(record Record) error {
	normalized, err := normalizeRecord(record)
	if err != nil {
		return err
	}

	path := s.recordFilePath(normalized)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create summary directory: %w", err)
	}

	line, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("marshal summary record: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open summary file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append summary record: %w", err)
	}

	cutoff := s.now().UTC().AddDate(0, 0, -s.retentionDays)
	if err := s.prune(cutoff); err != nil {
		return fmt.Errorf("prune summary records: %w", err)
	}

	return nil
}

// List returns records matching query filters, sorted newest-first.
func (s *Store) List(q Query) ([]Record, error) {
	typeFilter := strings.TrimSpace(q.Type)
	clusterFilter := strings.TrimSpace(q.Cluster)
	namespaceFilter := strings.TrimSpace(q.Namespace)

	records := make([]Record, 0)
	walkErr := filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		rel, relErr := filepath.Rel(s.rootDir, path)
		if relErr == nil {
			parts := strings.Split(rel, string(os.PathSeparator))
			if len(parts) >= 3 {
				if clusterFilter != "" && sanitizeSegment(clusterFilter) != parts[0] {
					return nil
				}
				if typeFilter != "" && sanitizeSegment(typeFilter) != parts[1] {
					return nil
				}
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var record Record
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return fmt.Errorf("parse summary record %s: %w", path, err)
			}
			record.Timestamp = record.Timestamp.UTC()
			if !matchesQuery(record, q, typeFilter, clusterFilter, namespaceFilter) {
				continue
			}
			records = append(records, record)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if walkErr != nil {
		if os.IsNotExist(walkErr) {
			return []Record{}, nil
		}
		return nil, walkErr
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.After(records[j].Timestamp)
	})

	if q.Limit > 0 && len(records) > q.Limit {
		records = records[:q.Limit]
	}

	return records, nil
}

func matchesQuery(record Record, q Query, typeFilter, clusterFilter, namespaceFilter string) bool {
	if !q.Since.IsZero() && record.Timestamp.Before(q.Since.UTC()) {
		return false
	}
	if !q.Until.IsZero() && record.Timestamp.After(q.Until.UTC()) {
		return false
	}
	if typeFilter != "" && !strings.EqualFold(record.Type, typeFilter) {
		return false
	}
	if clusterFilter != "" && !strings.EqualFold(record.Cluster, clusterFilter) {
		return false
	}
	if namespaceFilter != "" && !strings.EqualFold(record.Scope.Namespace, namespaceFilter) {
		return false
	}
	return true
}

func normalizeRecord(record Record) (Record, error) {
	record.Type = strings.TrimSpace(record.Type)
	record.Cluster = strings.TrimSpace(record.Cluster)
	record.Scope.Namespace = strings.TrimSpace(record.Scope.Namespace)
	record.Scope.Resource = strings.TrimSpace(record.Scope.Resource)
	record.Source = strings.TrimSpace(record.Source)

	if record.Timestamp.IsZero() {
		return Record{}, fmt.Errorf("summary record timestamp is required")
	}
	record.Timestamp = record.Timestamp.UTC()
	if record.Type == "" {
		return Record{}, fmt.Errorf("summary record type is required")
	}
	if record.Cluster == "" {
		return Record{}, fmt.Errorf("summary record cluster is required")
	}

	record.SchemaVersion = SchemaVersion
	return record, nil
}

func (s *Store) recordFilePath(record Record) string {
	day := record.Timestamp.UTC().Format("2006-01-02") + ".jsonl"
	return filepath.Join(s.rootDir, sanitizeSegment(record.Cluster), sanitizeSegment(record.Type), day)
}

func sanitizeSegment(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}

func (s *Store) prune(cutoff time.Time) error {
	cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, time.UTC)

	return filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		day, err := time.Parse("2006-01-02", base)
		if err != nil {
			return nil // Ignore unknown filenames.
		}
		day = day.UTC()
		if !day.Before(cutoffDay) {
			return nil
		}

		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		cleanupEmptyParentDirs(filepath.Dir(path), s.rootDir)
		return nil
	})
}

func cleanupEmptyParentDirs(dir, root string) {
	root = filepath.Clean(root)
	for {
		dir = filepath.Clean(dir)
		if dir == root {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}
