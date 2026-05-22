// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// receipt_store.go — local file-system store for receipt artifacts.
//
// Receipts are HISTORICAL, IMMUTABLE records (per the #446 design).
// The store enforces that: once a receipt is saved, the file content
// (including fingerprint) is the artifact. The store never modifies an
// existing file — Save() refuses to overwrite, returning the existing
// path. Tests assert this invariant.
//
// Layout: one JSON file per receipt at `<dir>/<filename>` where dir is
// resolved from $CUB_SCOUT_RECEIPTS_DIR (override), then
// $XDG_DATA_HOME/cub-scout/receipts (XDG basedir convention), then
// $HOME/.local/share/cub-scout/receipts (XDG default). No subdirectories,
// no DB, no index — flat files keyed by a sortable filename.

const (
	// StoreSubdir is the path under XDG_DATA_HOME (or HOME) where
	// receipts live.
	StoreSubdir = "cub-scout/receipts"

	// ReceiptFileExt is the conventional extension. Used by List() to
	// filter the directory.
	ReceiptFileExt = ".receipt.json"
)

// DefaultStoreDir returns the canonical receipt store directory,
// resolved in priority order:
//
//  1. $CUB_SCOUT_RECEIPTS_DIR (explicit override; useful for tests
//     and CI sandboxes)
//  2. $XDG_DATA_HOME/cub-scout/receipts
//  3. $HOME/.local/share/cub-scout/receipts (XDG default)
//
// Returns an empty string and an error if HOME cannot be resolved. The
// directory may not exist on disk — callers create on demand. Pure
// (no filesystem reads); test-friendly.
func DefaultStoreDir() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("CUB_SCOUT_RECEIPTS_DIR")); explicit != "" {
		return explicit, nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, StoreSubdir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("cannot resolve home directory for receipt store; set $CUB_SCOUT_RECEIPTS_DIR or $XDG_DATA_HOME")
	}
	return filepath.Join(home, ".local", "share", StoreSubdir), nil
}

// DeriveFilename builds the canonical filename for a receipt. The shape
// is deterministic and sortable:
//
//	<verifiedAt-rfc3339-safe>__<predicate>__<kind>-<name>__<short-fingerprint>.receipt.json
//
// "rfc3339-safe" replaces ":" with "-" because POSIX-portable filenames
// cannot contain colons. The short fingerprint is the first 12 hex
// chars after the "sha256:" prefix and makes co-existing receipts for
// the same scope distinguishable in `ls`.
//
// Empty fingerprint produces a filename ending in "__unstamped".
func DeriveFilename(stmt Statement) string {
	predicate := stmt.Predicate.PredicateName
	if predicate == "" {
		predicate = "noop"
	}
	kind := stmt.Predicate.Scope.Kind
	if kind == "" {
		kind = "unknown"
	}
	name := stmt.Predicate.Scope.Name
	if name == "" {
		name = "unknown"
	}

	verifiedAt := stmt.Predicate.VerifiedAt
	if verifiedAt == "" {
		verifiedAt = time.Now().UTC().Format(time.RFC3339)
	}
	// Filename-safe: replace ":" with "-".
	verifiedAt = strings.ReplaceAll(verifiedAt, ":", "-")

	short := "unstamped"
	if fp := stmt.Predicate.Fingerprint; strings.HasPrefix(fp, "sha256:") {
		hex := strings.TrimPrefix(fp, "sha256:")
		if len(hex) >= 12 {
			short = hex[:12]
		} else {
			short = hex
		}
	}

	// Sanitize any path separator from the parts. Receipt scopes shouldn't
	// contain "/" in kind or name, but be defensive.
	safe := func(s string) string {
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
		return s
	}

	return fmt.Sprintf("%s__%s__%s-%s__%s%s",
		verifiedAt,
		safe(string(predicate)),
		safe(kind),
		safe(name),
		short,
		ReceiptFileExt,
	)
}

// SaveStatement writes stmt to the store directory under its canonical
// filename. Creates the directory if missing.
//
// Receipts are immutable: SaveStatement REFUSES to overwrite an existing
// file and returns the existing path with a wrapped os.ErrExist. The
// rationale: a duplicate filename means the same VerifiedAt + same
// predicate + same scope + same fingerprint already exists on disk;
// re-writing serves no purpose and a different fingerprint at the same
// filename is impossible (the fingerprint is part of the filename).
//
// Atomicity: the create-and-write uses os.OpenFile with O_CREATE|O_EXCL|
// O_WRONLY, so the check-vs-create is a single syscall. Two concurrent
// SaveStatement calls for the same canonical filename can't both succeed
// — exactly one creates the file, the other races and gets ErrExist.
// The earlier Stat-then-WriteFile shape (pre-Codex-round-5 fix) was
// vulnerable to TOCTOU.
//
// Returns the absolute path written.
func SaveStatement(stmt Statement, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("save-receipt: empty store directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("save-receipt: mkdir %s: %w", dir, err)
	}
	filename := DeriveFilename(stmt)
	path := filepath.Join(dir, filename)

	buf, err := json.MarshalIndent(stmt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("save-receipt: marshal: %w", err)
	}

	// Atomic exclusive create. O_EXCL ensures the file did not previously
	// exist; if it did, the OS returns the platform-specific EEXIST which
	// os.IsExist recognizes and which wraps as os.ErrExist for callers
	// using errors.Is.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, fmt.Errorf("save-receipt: %s: %w", path, os.ErrExist)
		}
		return "", fmt.Errorf("save-receipt: open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(append(buf, '\n')); err != nil {
		// Clean up partial write on error — the file existed before
		// this call returned, so leaving it around would defeat
		// O_EXCL on a retry. Best-effort; if Remove fails the next
		// SaveStatement on the same canonical name will see ErrExist
		// on a corrupted file, which is loud rather than silent.
		_ = os.Remove(path)
		return "", fmt.Errorf("save-receipt: write %s: %w", path, err)
	}
	return path, nil
}

// LoadStatement reads a receipt from path and parses it. The fingerprint
// is NOT verified here — that's `cub-scout receipt validate`'s job —
// because a tampered receipt may still need to be loaded for forensic
// inspection. Callers that need verification call
// VerifyStatementFingerprint after Load.
func LoadStatement(path string) (Statement, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Statement{}, fmt.Errorf("load-receipt: read %s: %w", path, err)
	}
	var stmt Statement
	if err := json.Unmarshal(data, &stmt); err != nil {
		return Statement{}, fmt.Errorf("load-receipt: parse %s: %w", path, err)
	}
	return stmt, nil
}

// ReceiptListEntry summarizes a receipt for `cub-scout receipt list`
// output. The full Statement is left on disk; this is enough to render
// a one-line-per-receipt table.
type ReceiptListEntry struct {
	Path          string         `json:"path"`
	VerifiedAt    string         `json:"verifiedAt"`
	PredicateName string         `json:"predicateName"`
	Scope         Scope          `json:"scope"`
	Verdict       ReceiptVerdict `json:"verdict"`
	Fingerprint   string         `json:"fingerprint"`
}

// ListStatements walks dir, parses every *.receipt.json file, and
// returns one ReceiptListEntry per receipt sorted by VerifiedAt
// descending (most recent first — matches how operators scan logs).
//
// Files that fail to parse are skipped silently and a non-nil
// error is returned alongside the partial list. The store is meant
// to tolerate stray files (a partial write, an unrelated JSON) without
// failing the whole list. Callers that need strict semantics check
// the error.
func ListStatements(dir string) ([]ReceiptListEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list-receipts: read dir %s: %w", dir, err)
	}

	out := make([]ReceiptListEntry, 0, len(entries))
	var skipErrors []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ReceiptFileExt) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		stmt, err := LoadStatement(path)
		if err != nil {
			skipErrors = append(skipErrors, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		out = append(out, ReceiptListEntry{
			Path:          path,
			VerifiedAt:    stmt.Predicate.VerifiedAt,
			PredicateName: stmt.Predicate.PredicateName,
			Scope:         stmt.Predicate.Scope,
			Verdict:       stmt.Predicate.Verdict,
			Fingerprint:   stmt.Predicate.Fingerprint,
		})
	}

	// Sort by VerifiedAt descending. RFC 3339 strings sort
	// lexicographically in chronological order.
	sort.Slice(out, func(i, j int) bool {
		return out[i].VerifiedAt > out[j].VerifiedAt
	})

	if len(skipErrors) > 0 {
		return out, fmt.Errorf("list-receipts: skipped %d unparseable file(s): %s", len(skipErrors), strings.Join(skipErrors, "; "))
	}
	return out, nil
}
