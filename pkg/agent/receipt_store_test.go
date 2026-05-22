// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeStubStatement(verifiedAt, predicate, kind, name, fingerprint string) Statement {
	return Statement{
		Type:          StatementType,
		Subject:       []Subject{{Name: "k8s-live://apps/v1/Deployment/prod/api", Digest: map[string]string{"sha256": "deadbeef"}}},
		PredicateType: PredicateTypeReceiptV1,
		Predicate: Predicate{
			Version:           PredicateVersion,
			Claim:             "test",
			Scope:             Scope{Kind: kind, Name: name, Namespace: "prod"},
			Verifier:          Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
			VerifiedAt:        verifiedAt,
			PredicateName:     predicate,
			Verdict:           VerdictPASS,
			Omissions:         []Omission{},
			InputAttestations: []AttestationRef{},
			Fingerprint:       fingerprint,
		},
	}
}

// --- DefaultStoreDir ---------------------------------------------------

func TestDefaultStoreDir_HonorsExplicitOverride(t *testing.T) {
	t.Setenv("CUB_SCOUT_RECEIPTS_DIR", "/tmp/cub-scout-override")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")
	dir, err := DefaultStoreDir()
	if err != nil {
		t.Fatalf("DefaultStoreDir: %v", err)
	}
	if dir != "/tmp/cub-scout-override" {
		t.Errorf("CUB_SCOUT_RECEIPTS_DIR must win; got %q", dir)
	}
}

func TestDefaultStoreDir_FallsBackToXDG(t *testing.T) {
	t.Setenv("CUB_SCOUT_RECEIPTS_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	dir, err := DefaultStoreDir()
	if err != nil {
		t.Fatalf("DefaultStoreDir: %v", err)
	}
	if dir != filepath.Join("/tmp/xdg-data", StoreSubdir) {
		t.Errorf("XDG path mismatch; got %q", dir)
	}
}

func TestDefaultStoreDir_FallsBackToHomeShare(t *testing.T) {
	t.Setenv("CUB_SCOUT_RECEIPTS_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	dir, err := DefaultStoreDir()
	if err != nil {
		t.Skipf("home dir unresolvable in this environment: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join(".local", "share", StoreSubdir)) {
		t.Errorf("home default mismatch; got %q", dir)
	}
}

// --- DeriveFilename ---------------------------------------------------

func TestDeriveFilename_Shape(t *testing.T) {
	stmt := makeStubStatement(
		"2026-05-22T10:30:00Z",
		"applied-matches-spec",
		"Deployment",
		"api",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	name := DeriveFilename(stmt)

	// Sortable timestamp prefix (colons replaced by hyphens).
	if !strings.HasPrefix(name, "2026-05-22T10-30-00Z__") {
		t.Errorf("filename must start with filename-safe RFC3339; got %q", name)
	}
	if !strings.Contains(name, "applied-matches-spec") {
		t.Errorf("filename must contain predicate; got %q", name)
	}
	if !strings.Contains(name, "Deployment-api") {
		t.Errorf("filename must contain kind-name; got %q", name)
	}
	if !strings.Contains(name, "0123456789ab") {
		t.Errorf("filename must contain 12-char fingerprint prefix; got %q", name)
	}
	if !strings.HasSuffix(name, ReceiptFileExt) {
		t.Errorf("filename must end with %s; got %q", ReceiptFileExt, name)
	}
}

func TestDeriveFilename_SortsChronologically(t *testing.T) {
	older := DeriveFilename(makeStubStatement("2026-05-22T08:00:00Z", "applied-matches-spec", "Deployment", "api", "sha256:aaaaaaaaaaaa"))
	newer := DeriveFilename(makeStubStatement("2026-05-22T10:00:00Z", "applied-matches-spec", "Deployment", "api", "sha256:bbbbbbbbbbbb"))
	if !(older < newer) {
		t.Errorf("filename lex order must match chronological: older=%q newer=%q", older, newer)
	}
}

func TestDeriveFilename_UnstampedReceipt(t *testing.T) {
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "")
	name := DeriveFilename(stmt)
	if !strings.Contains(name, "unstamped") {
		t.Errorf("unstamped receipt filename must signal that; got %q", name)
	}
}

func TestDeriveFilename_SanitizesPathSeparators(t *testing.T) {
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "weird/Kind", "weird/name", "sha256:cccccccccccc")
	name := DeriveFilename(stmt)
	if strings.Contains(name, "weird/Kind") || strings.Contains(name, "weird/name") {
		t.Errorf("path separators must be sanitized; got %q", name)
	}
}

// --- SaveStatement / LoadStatement ------------------------------------

func TestSaveStatement_WritesAndCanLoad(t *testing.T) {
	dir := t.TempDir()
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")

	path, err := SaveStatement(stmt, dir)
	if err != nil {
		t.Fatalf("SaveStatement: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("path must live under store dir; got %q", path)
	}

	loaded, err := LoadStatement(path)
	if err != nil {
		t.Fatalf("LoadStatement: %v", err)
	}
	if loaded.Predicate.Fingerprint != stmt.Predicate.Fingerprint {
		t.Errorf("round-trip fingerprint mismatch: %q vs %q", loaded.Predicate.Fingerprint, stmt.Predicate.Fingerprint)
	}
}

func TestSaveStatement_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")
	if _, err := SaveStatement(stmt, dir); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Same VerifiedAt + predicate + scope + fingerprint → same filename
	// → SaveStatement must refuse.
	_, err := SaveStatement(stmt, dir)
	if err == nil {
		t.Fatal("expected ErrExist on duplicate save; got nil")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("expected os.ErrExist sentinel; got %v", err)
	}
}

// TestSaveStatement_OriginalContentPreservedOnCollision is the Codex
// round-5 immutability invariant: a second SaveStatement on the same
// canonical filename MUST leave the original file content untouched.
// The pre-fix code used Stat-then-WriteFile which had a TOCTOU race;
// the fixed code uses O_EXCL atomic create.
func TestSaveStatement_OriginalContentPreservedOnCollision(t *testing.T) {
	dir := t.TempDir()
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")

	path, err := SaveStatement(stmt, dir)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	// Mutate the stmt — different claim, but the SAME canonical filename
	// (because VerifiedAt + predicate + scope + fingerprint are
	// unchanged). The second save must refuse AND must not stomp on the
	// original content.
	stmt.Predicate.Claim = "tampered claim"
	_, err = SaveStatement(stmt, dir)
	if err == nil {
		t.Fatal("expected ErrExist on collision")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("expected os.ErrExist; got %v", err)
	}

	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after collision: %v", err)
	}
	if !bytes.Equal(originalBytes, afterBytes) {
		t.Errorf("ON-DISK CONTENT CHANGED across a collision-rejected save — immutability invariant broken.\nbefore: %s\nafter:  %s",
			string(originalBytes), string(afterBytes))
	}
}

// TestSaveStatement_ConcurrentSavesOneWinner exercises the O_EXCL
// atomic-create path. Without it (Stat-then-WriteFile), both goroutines
// could pass the Stat check and both could WriteFile, with the second
// stomping the first. With O_EXCL, exactly one goroutine creates the
// file; the other gets ErrExist.
func TestSaveStatement_ConcurrentSavesOneWinner(t *testing.T) {
	dir := t.TempDir()
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")

	const N = 8
	var (
		wg       sync.WaitGroup
		successC int32
		existC   int32
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := SaveStatement(stmt, dir)
			if err == nil {
				atomic.AddInt32(&successC, 1)
			} else if errors.Is(err, os.ErrExist) {
				atomic.AddInt32(&existC, 1)
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if successC != 1 {
		t.Errorf("expected exactly 1 winner among %d concurrent SaveStatement calls; got %d successes + %d ErrExist", N, successC, existC)
	}
	if successC+existC != N {
		t.Errorf("expected all %d concurrent SaveStatements to either win or ErrExist; got %d successes + %d ErrExist", N, successC, existC)
	}
}

func TestSaveStatement_CreatesMissingDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "store")
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")
	if _, err := SaveStatement(stmt, dir); err != nil {
		t.Fatalf("SaveStatement must mkdir; %v", err)
	}
}

func TestSaveStatement_RejectsEmptyDir(t *testing.T) {
	stmt := makeStubStatement("2026-05-22T10:30:00Z", "applied-matches-spec", "Deployment", "api", "sha256:0123456789ab0123")
	_, err := SaveStatement(stmt, "")
	if err == nil {
		t.Error("SaveStatement with empty dir must error")
	}
}

func TestLoadStatement_RejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad"+ReceiptFileExt)
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("seed bad file: %v", err)
	}
	_, err := LoadStatement(path)
	if err == nil {
		t.Error("LoadStatement on malformed JSON must error")
	}
}

// --- ListStatements --------------------------------------------------

func TestListStatements_ReturnsSortedDescending(t *testing.T) {
	dir := t.TempDir()
	stmts := []Statement{
		makeStubStatement("2026-05-22T08:00:00Z", "applied-matches-spec", "Deployment", "api", "sha256:aaaa000000000000aaaa"),
		makeStubStatement("2026-05-22T12:00:00Z", "no-manual-edits-since", "Deployment", "api", "sha256:bbbb000000000000bbbb"),
		makeStubStatement("2026-05-22T10:00:00Z", "source-truth-pass", "Deployment", "api", "sha256:cccc000000000000cccc"),
	}
	for _, s := range stmts {
		if _, err := SaveStatement(s, dir); err != nil {
			t.Fatalf("seed save: %v", err)
		}
	}

	entries, err := ListStatements(dir)
	if err != nil {
		t.Fatalf("ListStatements: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries; got %d", len(entries))
	}
	// Sorted by VerifiedAt descending.
	if entries[0].VerifiedAt != "2026-05-22T12:00:00Z" {
		t.Errorf("first entry must be newest; got VerifiedAt=%q", entries[0].VerifiedAt)
	}
	if entries[2].VerifiedAt != "2026-05-22T08:00:00Z" {
		t.Errorf("last entry must be oldest; got VerifiedAt=%q", entries[2].VerifiedAt)
	}
}

func TestListStatements_TolerataesNonReceiptFiles(t *testing.T) {
	dir := t.TempDir()
	// Drop a non-receipt file in the dir. List should skip it.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed text file: %v", err)
	}
	stmt := makeStubStatement("2026-05-22T10:00:00Z", "applied-matches-spec", "Deployment", "api", "sha256:aaaa000000000000aaaa")
	if _, err := SaveStatement(stmt, dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := ListStatements(dir)
	if err != nil {
		t.Fatalf("ListStatements: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("non-receipt files must be skipped; got %d entries", len(entries))
	}
}

func TestListStatements_PartialOnParseFailure(t *testing.T) {
	dir := t.TempDir()
	stmt := makeStubStatement("2026-05-22T10:00:00Z", "applied-matches-spec", "Deployment", "api", "sha256:aaaa000000000000aaaa")
	if _, err := SaveStatement(stmt, dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Drop a malformed receipt file alongside the good one.
	if err := os.WriteFile(filepath.Join(dir, "malformed"+ReceiptFileExt), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("seed malformed: %v", err)
	}
	entries, err := ListStatements(dir)
	if err == nil {
		t.Error("List must return non-nil error when a file failed to parse")
	}
	// Partial list: the good entry is still there.
	if len(entries) != 1 {
		t.Errorf("partial list must include parseable entry; got %d", len(entries))
	}
}

func TestListStatements_MissingDirReturnsEmpty(t *testing.T) {
	entries, err := ListStatements("/tmp/this/does/not/exist/cub-scout-test")
	if err != nil {
		t.Errorf("missing dir must be a non-error empty list; got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty list; got %d entries", len(entries))
	}
}

func TestListStatements_VerifyFingerprintRoundTrip(t *testing.T) {
	// Save a *real* receipt (built via BuildReceipt) and verify the
	// loaded copy still passes the fingerprint integrity check. Catches
	// any subtle JSON serialization drift in Save/Load.
	dir := t.TempDir()
	live := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{"replicas": int64(1)},
		},
	}
	stmt, err := BuildReceipt(BuildReceiptInput{
		Live: live,
		Scope: Scope{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "prod",
		},
		Owner:         Ownership{Type: OwnerUnknown},
		PredicateName: "",
		Verifier:      Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt:    time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}

	path, err := SaveStatement(stmt, dir)
	if err != nil {
		t.Fatalf("SaveStatement: %v", err)
	}
	loaded, err := LoadStatement(path)
	if err != nil {
		t.Fatalf("LoadStatement: %v", err)
	}
	if err := VerifyStatementFingerprint(loaded); err != nil {
		t.Errorf("round-tripped receipt fails fingerprint check: %v", err)
	}
}
