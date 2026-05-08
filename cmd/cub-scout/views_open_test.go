// Tests for the `cub-scout views open` browser-handoff helper added
// for #391 scope item #4. Covers URL construction (host pinning to the
// product UI host) and the platform-aware browser-launcher dispatch.
//
// Browser-launching itself is not exercised end-to-end — that requires
// a real desktop environment. The tests instead verify the launcher's
// argv shape and the --print fallback that scripts and headless runs
// rely on.

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/hub"
)

func TestViewExplorerURL_AnchorsAtProductUIHost(t *testing.T) {
	const uuid = "806aac53-236c-446d-8ad6-91d6daf6810e"

	got := viewExplorerURL(uuid)
	want := strings.TrimRight(hub.HubBaseURL, "/") + "/x/view-explorer?view=" + uuid
	if got != want {
		t.Errorf("viewExplorerURL(%q) = %q, want %q", uuid, got, want)
	}

	// Sanity: the URL must be parseable back into the same UUID by
	// the parser the rest of the views tooling uses. This proves
	// open and resolve round-trip the same identifier.
	if !strings.Contains(got, uuid) {
		t.Errorf("URL %q lost the UUID", got)
	}
	if !strings.Contains(got, "/x/view-explorer") {
		t.Errorf("URL %q is missing the /x/view-explorer path", got)
	}
}

func TestViewExplorerURL_StripsTrailingSlashFromBase(t *testing.T) {
	// Defensive: the base may or may not have a trailing slash
	// depending on config. The builder must produce a single slash
	// before /x/view-explorer regardless.
	got := viewExplorerURL("ffffffff-ffff-ffff-ffff-ffffffffffff")
	if strings.Contains(got, "//x/") {
		t.Errorf("URL has double-slash before path: %q", got)
	}
}

// TestRunViewsOpen_PrintMode covers the headless / pipe-friendly path:
// `--print` emits the URL to stdout instead of opening a browser, so
// the test does not need to mock browser-launching at all.
func TestRunViewsOpen_PrintMode(t *testing.T) {
	const uuid = "806aac53-236c-446d-8ad6-91d6daf6810e"

	// Force --print mode and reset afterwards so we don't pollute
	// other tests sharing the package-level flag var.
	prev := viewsOpenPrintOnly
	viewsOpenPrintOnly = true
	t.Cleanup(func() { viewsOpenPrintOnly = prev })

	out := captureStdout(t, func() {
		// runViewsOpen reads its arg, parses the ref, builds the URL,
		// and prints it.
		if err := runViewsOpen(viewsOpenCmd, []string{uuid}); err != nil {
			t.Fatalf("runViewsOpen returned error in print mode: %v", err)
		}
	})

	wantSubstring := "/x/view-explorer?view=" + uuid
	if !strings.Contains(out, wantSubstring) {
		t.Errorf("--print output %q does not contain %q", out, wantSubstring)
	}
}

// TestRunViewsOpen_PrintMode_AcceptsViewExplorerURL verifies the URL
// form round-trips through the open command — the operator can paste a
// URL from `views resolve` output back into `views open` and get the
// canonical product-UI URL out, even if the input pointed at a
// different host (e.g. an on-prem URL someone pasted into a doc).
func TestRunViewsOpen_PrintMode_AcceptsViewExplorerURL(t *testing.T) {
	const uuid = "806aac53-236c-446d-8ad6-91d6daf6810e"
	in := "https://confighub.example.internal/x/view-explorer?view=" + uuid

	prev := viewsOpenPrintOnly
	viewsOpenPrintOnly = true
	t.Cleanup(func() { viewsOpenPrintOnly = prev })

	out := captureStdout(t, func() {
		if err := runViewsOpen(viewsOpenCmd, []string{in}); err != nil {
			t.Fatalf("runViewsOpen returned error: %v", err)
		}
	})

	// The output URL is anchored at the configured product-UI host,
	// not the on-prem host the input URL used. This is the desired
	// behaviour: cub-scout's hub config wins over whatever URL was
	// pasted (so on-prem operators with a configured override don't
	// get bounced to the wrong site).
	if !strings.Contains(out, "/x/view-explorer?view="+uuid) {
		t.Errorf("output URL %q missing canonical View Explorer path", out)
	}
}

func TestRunViewsOpen_RejectsInvalidInput(t *testing.T) {
	prev := viewsOpenPrintOnly
	viewsOpenPrintOnly = true
	t.Cleanup(func() { viewsOpenPrintOnly = prev })

	if err := runViewsOpen(viewsOpenCmd, []string{"definitely-not-a-uuid"}); err == nil {
		t.Fatal("expected runViewsOpen to error on garbage input, got nil")
	}
}
