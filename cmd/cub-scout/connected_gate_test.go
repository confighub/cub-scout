// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
)

// stubConnectedGate fixes the gate's answer for one test.
//
// This and answerGateWith mutate package state without a lock, so a test that
// uses either must not call t.Parallel(). Production callers all go through
// configHubReadsMu; the seam itself is deliberately unguarded.
func stubConnectedGate(t *testing.T, err error) {
	t.Helper()
	old := requireCubConnectedFn
	t.Cleanup(func() {
		requireCubConnectedFn = old
		resetConfigHubReads()
	})
	requireCubConnectedFn = func() error { return err }
	resetConfigHubReads()
}

// resetConfigHubReads discards the kept gate answer, which production keeps for
// the life of a command.
func resetConfigHubReads() {
	configHubReadsMu.Lock()
	defer configHubReadsMu.Unlock()
	configHubReadsAnswered = false
	configHubReadsErr = nil
}

// answerGateWith installs a gate answer that the test can change, plus a call
// counter, for the difference between the kept answer and a refresh.
func answerGateWith(t *testing.T, answer *error) *int {
	t.Helper()
	calls := 0
	old := requireCubConnectedFn
	t.Cleanup(func() { requireCubConnectedFn = old; resetConfigHubReads() })
	requireCubConnectedFn = func() error { calls++; return *answer }
	resetConfigHubReads()
	return &calls
}

func TestRequireConfigHubFor_NamesTheCommandAndKeepsTheCause(t *testing.T) {
	stubConnectedGate(t, hub.ErrCubNotInstalled)
	err := requireConfigHubFor("compare source-truth")
	if !errors.Is(err, hub.ErrCubNotInstalled) {
		t.Fatalf("error = %v, want it to wrap %v", err, hub.ErrCubNotInstalled)
	}
	for _, want := range []string{"compare source-truth", "cub auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}

	stubConnectedGate(t, nil)
	if err := requireConfigHubFor("views resolve"); err != nil {
		t.Fatalf("error = %v, want nil when cub reports an authenticated session", err)
	}
}

// Each connected-only command must refuse with the gate's own error, and must
// do so before it reads anything from ConfigHub.
func TestConnectedOnlyCommandsRefuseBeforeReadingConfigHub(t *testing.T) {
	const viewID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	refusal := errors.New("gate refused")

	oldRunner := viewCubRunner
	t.Cleanup(func() { viewCubRunner = oldRunner })
	viewCubRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		t.Fatalf("ConfigHub was read (cub %s) although the gate refused", strings.Join(args, " "))
		return nil, nil
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "compare source-truth", run: func() error {
			oldFormat, oldStrategy := sourceTruthFormat, sourceTruthStrategy
			defer func() { sourceTruthFormat, sourceTruthStrategy = oldFormat, oldStrategy }()
			sourceTruthFormat, sourceTruthStrategy = "json", "git-argo"
			return runSourceTruth(&cobra.Command{}, []string{"deployment/api"})
		}},
		{name: "views resolve", run: func() error {
			return runViewsResolve(&cobra.Command{}, []string{viewID})
		}},
		{name: "views project", run: func() error {
			oldFormat := viewsProjectFormat
			defer func() { viewsProjectFormat = oldFormat }()
			viewsProjectFormat = "json"
			return runViewsProject(&cobra.Command{}, []string{viewID})
		}},
		{name: "compare three-way --view", run: func() error {
			oldScope, oldView := compareThreeWayScopeRaw, compareThreeWayView
			defer func() { compareThreeWayScopeRaw, compareThreeWayView = oldScope, oldView }()
			compareThreeWayScopeRaw, compareThreeWayView = "", viewID
			return runCompareThreeWay(&cobra.Command{}, nil)
		}},
		{name: "import argocd", run: checkCubAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubConnectedGate(t, refusal)
			err := tt.run()
			if !errors.Is(err, refusal) {
				t.Fatalf("error = %v, want the gate's refusal", err)
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Fatalf("error = %q, want it to name %q", err, tt.name)
			}
		})
	}
}

// The MCP gateway offers its connected tools exactly when the CLI would run
// them, so `compare source-truth` cannot work on the CLI while the MCP tool of
// the same name is never listed.
func TestMCPConnectedModeFollowsTheSameGate(t *testing.T) {
	stubConnectedGate(t, nil)
	if !detectMCPConnectedMode() {
		t.Fatal("detectMCPConnectedMode() = false although the gate passes")
	}
	stubConnectedGate(t, hub.ErrCubNotAuthenticated)
	if detectMCPConnectedMode() {
		t.Fatal("detectMCPConnectedMode() = true although the gate refuses")
	}
}

// parseRepoGoFiles parses every non-test Go file under the given repo-relative
// directories, skipping testdata.
func parseRepoGoFiles(t *testing.T, dirs ...string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, dir := range dirs {
		root := filepath.Join("..", "..", dir)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			files[path] = file
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return fset, files
}

// hub.QuickMode and hub.IsAuthenticated never consult the cub CLI, so neither
// may decide whether a command runs. Rather than chase every way of writing
// such a test (a comparison, a switch, a stored value), allow the names only
// where they are known to be display-only.
func TestDisplayOnlyHubChecksStayOutOfCommandGates(t *testing.T) {
	allowed := map[string]string{
		"localcluster.go": "QuickMode", // TUI header, refined asynchronously
	}
	fset, files := parseRepoGoFiles(t, "cmd")
	for path, file := range files {
		base := filepath.Base(path)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "hub" || (sel.Sel.Name != "QuickMode" && sel.Sel.Name != "IsAuthenticated") {
				return true
			}
			if allowed[base] != sel.Sel.Name {
				t.Errorf("%s uses hub.%s, which never consults the cub CLI; gate with requireConfigHubFor", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}

var (
	envVarName       = regexp.MustCompile(`\b(?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9]+(?:_[A-Z0-9]+)*\b`)
	envVarAssignment = regexp.MustCompile(`^(?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9_]+=\S*$`)
)

// envVarsReadOutsideThisRepo are read by the cub CLI, not by cub-scout, so text
// may name them. Each was checked against cub v0.5.1: CUB_CONTEXT selects the
// context, CUB_CONFIG names the config directory.
var envVarsReadOutsideThisRepo = map[string]bool{
	"CUB_CONFIG":  true,
	"CUB_CONTEXT": true,
}

// A message must never tell someone to set an environment variable that
// nothing reads. Every CONFIGHUB_*, CUB_SCOUT_* or CUB_* name appearing in a
// string in cmd/, pkg/ or internal/ must reach os.Getenv or os.LookupEnv:
// directly, through a constant, or through a function that passes its
// parameter on to one of them. Declaring a constant is not enough.
func TestEveryEnvVarNamedInTextIsRead(t *testing.T) {
	fset, files := parseRepoGoFiles(t, "cmd", "pkg", "internal")
	if unread := unreadEnvVarMentions(fset, files); len(unread) > 0 {
		t.Fatalf("text names environment variables that nothing reads:\n  %s", strings.Join(unread, "\n  "))
	}
}

// The guard must catch the defect it was written for, in each shape it could
// return in, and must not be silenced by a constant nobody reads.
func TestEnvVarGuardCatchesPhantoms(t *testing.T) {
	scan := func(t *testing.T, src string) []string {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "sample.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return unreadEnvVarMentions(fset, map[string]*ast.File{"sample.go": file})
	}

	phantoms := map[string]string{
		"error string":            "package p\nimport \"fmt\"\nfunc f() error { return fmt.Errorf(\"run cub auth login or set CONFIGHUB_API_KEY\") }",
		"message held in a const": "package p\nconst authHint = \"run cub auth login or set CONFIGHUB_API_KEY\"",
		"unread constant":         "package p\nimport \"fmt\"\nconst envAPIKey = \"CONFIGHUB_API_KEY\"\nfunc f() error { return fmt.Errorf(\"set CONFIGHUB_API_KEY\") }",
		"shell example":           "package p\nvar example = []string{\"CONFIGHUB_API_KEY=<key> cub-scout compare source-truth\"}",
		"only ever set":           "package p\nimport \"os\"\nfunc f() { os.Setenv(\"CONFIGHUB_API_KEY\", \"x\") }\nvar help = \"set CONFIGHUB_API_KEY\"",
	}
	for name, src := range phantoms {
		t.Run("phantom/"+name, func(t *testing.T) {
			unread := scan(t, src)
			if len(unread) != 1 || !strings.HasPrefix(unread[0], "CONFIGHUB_API_KEY") {
				t.Fatalf("unread = %v, want the phantom CONFIGHUB_API_KEY reported", unread)
			}
		})
	}

	legitimate := map[string]string{
		"read directly":           "package p\nimport \"os\"\nvar v = os.Getenv(\"CUB_SCOUT_DEBUG\")\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"read through a constant": "package p\nimport \"os\"\nconst envDebug = \"CUB_SCOUT_DEBUG\"\nvar v = os.Getenv(envDebug)\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"read through a helper":   "package p\nimport \"os\"\nfunc envOr(name, d string) string { if v, ok := os.LookupEnv(name); ok { return v }; return d }\nvar v = envOr(\"CUB_SCOUT_DEBUG\", \"\")\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"exported to a child":     "package p\nvar env = []string{\"CUB_SCOUT_TUI=1\"}",
	}
	for name, src := range legitimate {
		t.Run("legitimate/"+name, func(t *testing.T) {
			if unread := scan(t, src); len(unread) != 0 {
				t.Fatalf("unread = %v, want none", unread)
			}
		})
	}
}

func stringLiteral(node ast.Node) (string, bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}

func isEnvReader(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os"
}

func calledName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

func unreadEnvVarMentions(fset *token.FileSet, files map[string]*ast.File) []string {
	// Pass 1: constants holding exactly one variable name, and helpers that
	// hand one of their parameters to os.Getenv or os.LookupEnv.
	constants := map[string]string{}
	helpers := map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, value := range vs.Values {
						if text, ok := stringLiteral(value); ok && i < len(vs.Names) && envVarName.FindString(text) == text {
							constants[vs.Names[i].Name] = text
						}
					}
				}
			case *ast.FuncDecl:
				if d.Body == nil || d.Type.Params == nil {
					continue
				}
				index := map[string]int{}
				n := 0
				for _, field := range d.Type.Params.List {
					for _, name := range field.Names {
						index[name.Name] = n
						n++
					}
				}
				ast.Inspect(d.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !isEnvReader(call) || len(call.Args) == 0 {
						return true
					}
					if id, ok := call.Args[0].(*ast.Ident); ok {
						if i, isParam := index[id.Name]; isParam {
							helpers[d.Name.Name] = i
						}
					}
					return true
				})
			}
		}
	}

	// Pass 2: every name that actually reaches a reader.
	read := map[string]bool{}
	nameOf := func(expr ast.Expr) (string, bool) {
		if text, ok := stringLiteral(expr); ok && envVarName.FindString(text) == text {
			return text, true
		}
		if id, ok := expr.(*ast.Ident); ok {
			text, ok := constants[id.Name]
			return text, ok
		}
		return "", false
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			arg := -1
			if isEnvReader(call) {
				arg = 0
			} else if i, ok := helpers[calledName(call)]; ok {
				arg = i
			}
			if arg >= 0 && arg < len(call.Args) {
				if name, ok := nameOf(call.Args[arg]); ok {
					read[name] = true
				}
			}
			return true
		})
	}

	// Pass 3: every name that text mentions.
	var unread []string
	seen := map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			text, ok := stringLiteral(node)
			if !ok {
				return true
			}
			// A bare name is a constant's value or a reader's argument, not
			// prose. "CUB_SCOUT_TUI=1" with no spaces builds a child process's
			// environment. Neither tells anyone to set anything.
			if envVarName.FindString(text) == text || envVarAssignment.MatchString(text) {
				return true
			}
			for _, name := range envVarName.FindAllString(text, -1) {
				if read[name] || envVarsReadOutsideThisRepo[name] || seen[name] {
					continue
				}
				seen[name] = true
				unread = append(unread, name+" (first named at "+fset.Position(node.Pos()).String()+")")
			}
			return true
		})
	}
	sort.Strings(unread)
	return unread
}

// Every surface that reads ConfigHub by running `cub` asks the same question,
// so a command, the facts it records and `status` cannot disagree about whether
// the session was usable. Before this, ten sites used a check that probed
// hub.confighub.com and then accepted `cub auth get-token`, which passes an
// expired session.
func TestConvergedGatesFollowTheCubSession(t *testing.T) {
	states := []struct {
		name string
		err  error
	}{
		{name: "authenticated"},
		{name: "expired or tokenless", err: hub.ErrCubNotAuthenticated},
		{name: "cub missing", err: hub.ErrCubNotInstalled},
		{name: "reads turned off", err: hub.ErrConfigHubReadsDisabled},
	}

	refusals := map[string]func() error{
		"audit list":                  requireAuditConnected,
		"history":                     requireHistoryConnected,
		"ConfigHub delivery evidence": requireGitOpsConfigHubConnected,
	}
	for feature, gate := range refusals {
		for _, state := range states {
			t.Run(feature+"/"+state.name, func(t *testing.T) {
				stubConnectedGate(t, state.err)
				err := gate()
				if state.err == nil {
					if err != nil {
						t.Fatalf("err = %v, want nil for an authenticated session", err)
					}
					return
				}
				if !errors.Is(err, state.err) {
					t.Fatalf("err = %v, want it to wrap %v", err, state.err)
				}
				if !strings.Contains(err.Error(), feature) {
					t.Fatalf("err = %q, want it to name %q", err, feature)
				}
			})
		}
	}

	facts := map[string]func() bool{
		"receipt":         detectConnectedForReceipt,
		"summary":         summaryConnectedFn,
		"compare":         isCompareConnected,
		"watch/aggregate": configHubReadsAvailable,
	}
	for name, fact := range facts {
		for _, state := range states {
			t.Run("connected fact "+name+"/"+state.name, func(t *testing.T) {
				stubConnectedGate(t, state.err)
				if got := fact(); got != (state.err == nil) {
					t.Fatalf("%s connected = %v for %s", name, got, state.name)
				}
			})
		}
	}
}

// The three-way disagreement path reports the gate's own reason rather than
// "connect to ConfigHub", which cannot fix a session that expired or reads
// that were turned off.
func TestThreeWayDisagreementReportsTheGateReason(t *testing.T) {
	stubConnectedGate(t, hub.ErrConfigHubReadsDisabled)
	got, err := buildThreeWayDisagreement(context.Background(), "Deployment", "api", "prod", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Pattern != PatternDisconnected {
		t.Fatalf("disagreement = %+v, want the disconnected pattern", got)
	}
	for _, want := range []string{"three-way comparison", "turned off"} {
		if !strings.Contains(got.Meaning, want) {
			t.Fatalf("meaning = %q, want it to contain %q", got.Meaning, want)
		}
	}
}

// doctor offers the three-way hint exactly when the command it suggests would
// run, so the hint cannot point at a command that refuses.
func TestDoctorThreeWayHintFollowsTheGate(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		hint bool
	}{
		{name: "authenticated", hint: true},
		{name: "expired", err: hub.ErrCubNotAuthenticated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubConnectedGate(t, tt.err)
			summary := buildDoctorSummary(nil, nil, "kind-dev", "prod", 3)
			if (summary.ThreeWay != nil) != tt.hint {
				t.Fatalf("threeWay = %+v, want hint=%v", summary.ThreeWay, tt.hint)
			}
		})
	}
}

// The gate costs one `cub` process, and callers ask it per resource, per
// receipt and per poll cycle.
func TestConfigHubReadsIsAnsweredOnce(t *testing.T) {
	calls := 0
	old := requireCubConnectedFn
	t.Cleanup(func() { requireCubConnectedFn = old; resetConfigHubReads() })
	requireCubConnectedFn = func() error { calls++; return nil }
	resetConfigHubReads()

	for i := 0; i < 5; i++ {
		if err := configHubReads(); err != nil {
			t.Fatal(err)
		}
		_ = configHubReadsAvailable()
		_ = isCompareConnected()
	}
	if calls != 1 {
		t.Fatalf("gate ran %d times, want 1", calls)
	}
}

// A kept answer is right for a command that ends, and wrong for one that does
// not. `cub auth login` in another terminal must take effect in a running TUI
// or watch, so those ask again through refreshConfigHubReads.
func TestRefreshAsksTheGateAgainAndReplacesTheKeptAnswer(t *testing.T) {
	session := error(hub.ErrCubNotAuthenticated)
	calls := answerGateWith(t, &session)

	if err := configHubReads(); !errors.Is(err, hub.ErrCubNotAuthenticated) {
		t.Fatalf("err = %v, want the expired-session refusal", err)
	}
	if err := configHubReads(); !errors.Is(err, hub.ErrCubNotAuthenticated) {
		t.Fatalf("err = %v, want the kept refusal", err)
	}
	if *calls != 1 {
		t.Fatalf("gate ran %d times, want 1 while the answer is kept", *calls)
	}

	// The user logs in elsewhere.
	session = nil

	if err := configHubReads(); !errors.Is(err, hub.ErrCubNotAuthenticated) {
		t.Fatalf("err = %v, want the kept answer until something refreshes it", err)
	}
	if err := refreshConfigHubReads(); err != nil {
		t.Fatalf("refresh = %v, want nil once the session works", err)
	}
	if *calls != 2 {
		t.Fatalf("gate ran %d times, want 2 after a refresh", *calls)
	}
	if err := configHubReads(); err != nil {
		t.Fatalf("kept answer = %v, want the refreshed one", err)
	}
	if !configHubReadsAvailable() || !refreshConfigHubReadsAvailable() {
		t.Fatal("boolean forms disagree with the refreshed answer")
	}
}

// The TUI runs for as long as its user leaves it open. Before the refresh, a
// history panel refused once stayed refused for the life of the process, while
// telling the user to press h again once it was fixed.
func TestHistoryPanelNoticesASessionFixedWhileTheTUIIsUp(t *testing.T) {
	t.Setenv("CUB_SPACE", "demo")               // the TUI has no --space flag
	t.Setenv("CUB_SCOUT_TEST_HISTORY_JSON", "") // take the ConfigHub path

	prevRequire, prevFetch := requireHistoryConnectedFn, fetchHistoryEntriesFn
	t.Cleanup(func() { requireHistoryConnectedFn, fetchHistoryEntriesFn = prevRequire, prevFetch })
	requireHistoryConnectedFn = requireHistoryConnected
	fetchHistoryEntriesFn = func(context.Context, historyQuery) ([]historyEntry, error) {
		return []historyEntry{{Actor: "release-bot", Change: "replicas: 2 -> 3", ChangeSet: "CS-1"}}, nil
	}

	session := error(hub.ErrCubNotAuthenticated)
	answerGateWith(t, &session)

	model := testLocalModel()
	first, ok := model.runHistoryPanel()().(localHistoryLoadedMsg)
	if !ok {
		t.Fatal("history panel returned an unexpected message type")
	}
	if !errors.Is(first.err, errConfigHubUnavailable) {
		t.Fatalf("first press: err = %v, want the gate's refusal", first.err)
	}

	// The user does what the panel told them to do, in another terminal.
	session = nil

	second, ok := model.runHistoryPanel()().(localHistoryLoadedMsg)
	if !ok {
		t.Fatal("history panel returned an unexpected message type")
	}
	if second.err != nil {
		t.Fatalf("second press: err = %v, want the panel to load once the session works", second.err)
	}
	if len(second.result.Entries) != 1 {
		t.Fatalf("entries = %d, want the loaded history", len(second.result.Entries))
	}
}

// The panel explains any refusal the gate can produce, including a cause added
// after this test was written, and keeps "History load failed" for the rest.
func TestHistoryPanelExplainsEveryGateRefusal(t *testing.T) {
	stubConnectedGate(t, fmt.Errorf("%w: a cause nobody has listed yet", errors.New("cub session unusable")))

	model := testLocalModel()
	model.panelMode, model.panelView = true, viewHistory
	model.historyPanelError = requireConfigHubFor("history")

	panel := model.getPanelHistory()
	for _, want := range []string{"history needs ConfigHub", "a cause nobody has listed yet", "Press h again once it is fixed."} {
		if !strings.Contains(panel, want) {
			t.Fatalf("panel = %q, want it to contain %q", panel, want)
		}
	}

	model.historyPanelError = errors.New("read changesets: connection reset")
	if panel := model.getPanelHistory(); !strings.Contains(panel, "History load failed") {
		t.Fatalf("panel = %q, want a load failure for an error the gate did not produce", panel)
	}
}

// status is read as a pre-flight, so its verdict must be the gate's own answer
// rather than a second opinion. It reports the reason it was given.
func TestStatusRecordsTheGateVerdict(t *testing.T) {
	t.Setenv("CUB_SCOUT_OFFLINE", "true") // no network probe from hub.CurrentMode
	t.Setenv("PATH", t.TempDir())         // no cub on PATH, so no context upgrade

	for _, tt := range []struct {
		name   string
		err    error
		reads  bool
		reason string
	}{
		{name: "authenticated", reads: true},
		{name: "expired", err: hub.ErrCubNotAuthenticated, reason: "did not report an authenticated session"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubConnectedGate(t, tt.err)

			cmd := &cobra.Command{}
			cmd.Flags().Bool("json", true, "")
			out := captureStdout(t, func() {
				if err := runStatus(cmd); err != nil {
					t.Fatalf("runStatus: %v", err)
				}
			})

			var got StatusInfo
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal status %q: %v", out, err)
			}
			if got.ConfigHubReads != tt.reads {
				t.Fatalf("confighub_reads = %v, want %v", got.ConfigHubReads, tt.reads)
			}
			if tt.reason == "" {
				if got.ConfigHubReadsReason != "" {
					t.Fatalf("reason = %q, want none when reads work", got.ConfigHubReadsReason)
				}
				return
			}
			if !strings.Contains(got.ConfigHubReadsReason, tt.reason) {
				t.Fatalf("reason = %q, want it to name %q", got.ConfigHubReadsReason, tt.reason)
			}
		})
	}
}

// status asked `cub auth status` twice: once through the gate, once through
// validateAuthToken. In plugin mode with no CUB_TOKEN the two disagreed, and
// status printed "Connected (auth expired)", "Run: cub auth login" and
// "ConfigHub reads available" together. The session verdict now follows the
// gate wherever the gate looked at the session.
func TestStatusSessionVerdictFollowsTheGate(t *testing.T) {
	if !statusSessionValid(nil) {
		t.Fatal("session invalid although the gate accepted it; status would say 'auth expired' and 'reads available' together")
	}
	if statusSessionValid(fmt.Errorf("%w: token expired", hub.ErrCubNotAuthenticated)) {
		t.Fatal("session valid although the gate refused it for not being authenticated")
	}

	// A refusal that is not about authentication says nothing about the
	// session, so it is asked. In plugin mode that is the host's token.
	t.Setenv("CUB_PLUGIN", "1")
	t.Setenv("CUB_TOKEN", "a-token")
	if !statusSessionValid(hub.ErrConfigHubReadsDisabled) {
		t.Fatal("session invalid although the plugin host passed a token")
	}
	t.Setenv("CUB_TOKEN", "")
	if statusSessionValid(hub.ErrConfigHubReadsDisabled) {
		t.Fatal("session valid although the plugin host passed no token")
	}
}

// Whichever way the mode line and the gate disagree, the text says so: a mode
// that promises reads the commands refuse, and a mode that denies reads they
// would make.
func TestStatusCorrectsTheModeInBothDirections(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status StatusInfo
		want   string
		absent string
	}{
		{
			name:   "connected but the gate refuses",
			status: StatusInfo{Mode: "connected", ConfigHubReadsReason: "cub auth status did not report an authenticated session"},
			want:   "ConfigHub reads unavailable: cub auth status did not report an authenticated session",
		},
		{
			// Reached when cub has a session but `cub context get` failed, so
			// the mode line describes only hub.confighub.com.
			name:   "offline but the gate passes",
			status: StatusInfo{Mode: "offline", ConfigHubReads: true},
			want:   "ConfigHub reads available: cub has a session",
		},
		{
			name:   "connected and the gate agrees",
			status: StatusInfo{Mode: "connected", ConfigHubReads: true},
			absent: "ConfigHub reads",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() { printStatus(tt.status) })
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Fatalf("status text = %q, want it to contain %q", out, tt.want)
			}
			if tt.absent != "" && strings.Contains(out, tt.absent) {
				t.Fatalf("status text = %q, want no correction when the two agree", out)
			}
		})
	}
}

// A scan needs no ConfigHub session: it must not spend a `cub` process to find
// that out, and only --verbose explains which pattern set it used.
func TestScanAsksTheGateOnlyWhenItWouldExplain(t *testing.T) {
	session := error(hub.ErrConfigHubReadsDisabled)
	calls := answerGateWith(t, &session)

	if note := scanConfigHubNote(false); note != "" {
		t.Fatalf("note = %q, want none without --verbose", note)
	}
	if *calls != 0 {
		t.Fatalf("gate ran %d times without --verbose, want 0", *calls)
	}

	note := scanConfigHubNote(true)
	if !strings.Contains(note, "embedded patterns") || !strings.Contains(note, "turned off") {
		t.Fatalf("note = %q, want it to name the cause and the pattern set used", note)
	}

	session = nil
	resetConfigHubReads()
	if note := scanConfigHubNote(true); note != "" {
		t.Fatalf("note = %q, want none when ConfigHub reads work", note)
	}
}

// A watch runs for days. The fact its receipts record is read per cycle, so a
// session that expires mid-run stops being asserted, and one that is restored
// starts being asserted, without restarting the watch.
func TestWatchAsksTheGateEachCycle(t *testing.T) {
	session := error(nil)
	calls := answerGateWith(t, &session)

	events := []watchEvent{{Type: "drift.detected"}}
	emitOn := map[string]bool{"drift.detected": true}

	if watchCycleConnected(nil, events) {
		t.Fatal("connected = true with no receipts requested")
	}
	if watchCycleConnected(emitOn, nil) {
		t.Fatal("connected = true on an idle cycle")
	}
	// resource.deleted has no receipt support, so asking for it builds nothing
	// and must cost nothing, even though the flag names the type.
	if watchCycleConnected(map[string]bool{"resource.deleted": true}, []watchEvent{{Type: "resource.deleted"}}) {
		t.Fatal("connected = true for a cycle whose events build no receipt")
	}
	// --emit-receipt-batch-cap 0 disables receipt-build while keeping the flag
	// explicit, so there is nothing to record the fact on then either.
	prevCap := watchReceiptBatchCap
	t.Cleanup(func() { watchReceiptBatchCap = prevCap })
	watchReceiptBatchCap = 0
	if watchCycleConnected(emitOn, events) {
		t.Fatal("connected = true although the batch cap builds no receipts")
	}
	watchReceiptBatchCap = prevCap

	if *calls != 0 {
		t.Fatalf("gate ran %d times with nothing to record it on, want 0", *calls)
	}

	if !watchCycleConnected(emitOn, events) {
		t.Fatal("connected = false although the session works")
	}

	// The session expires while the watch is up.
	session = hub.ErrCubNotAuthenticated
	if watchCycleConnected(emitOn, events) {
		t.Fatal("connected = true after the session expired; receipts would assert a session that is gone")
	}
	if *calls != 2 {
		t.Fatalf("gate ran %d times over two cycles with events, want 2", *calls)
	}
}

// `mcp serve` runs for as long as the agent holds it, so it has the TUI's
// problem one process up: the tool set was decided once at start-up. A session
// that expired left the connected tools advertised, and their calls failed with
// a raw error; `cub auth login` in another terminal never made them appear.
func TestMCPGatewayFollowsTheSession(t *testing.T) {
	session := error(hub.ErrCubNotAuthenticated)
	answerGateWith(t, &session)

	runner := func(context.Context, []string) (string, error) { return `{"ok":true}`, nil }
	gateway := newMCPGatewayWithMode(runner, runner, false)
	gateway.connectedOnly = connectedOnlyToolNames(gateway.runTool, gateway.connectedRunner)
	gateway.sessionFn = refreshConfigHubReadsAvailable

	offers := func() bool {
		resp := gateway.handleRequest(context.Background(), mcpRequest{
			JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list",
		})
		var result struct {
			Tools []mcpToolDescriptor `json:"tools"`
		}
		if err := marshalInto(resp.Result, &result); err != nil {
			t.Fatalf("decode tools/list: %v", err)
		}
		for _, tool := range result.Tools {
			if tool.Name == "compare_three_way" {
				return true
			}
		}
		return false
	}
	call := func() (bool, string) {
		resp := gateway.handleRequest(context.Background(), mcpRequest{
			JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "tools/call",
			Params: json.RawMessage(`{"name":"compare_three_way","arguments":{"scope":"cluster"}}`),
		})
		var result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := marshalInto(resp.Result, &result); err != nil {
			t.Fatalf("decode tools/call: %v", err)
		}
		text := ""
		if len(result.Content) > 0 {
			text = result.Content[0].Text
		}
		return result.IsError, text
	}

	if offers() {
		t.Fatal("a connected tool is offered although cub has no session")
	}
	// Calling it anyway is answered with the gate's reason. "unknown tool"
	// would read as a cub-scout defect.
	isErr, text := call()
	if !isErr || !strings.Contains(text, "needs ConfigHub") || strings.Contains(text, "unknown tool") {
		t.Fatalf("tools/call error = %q, want the gate's refusal", text)
	}

	// The user logs in elsewhere.
	session = nil
	if !offers() {
		t.Fatal("a connected tool is still not offered although the session works")
	}
	if isErr, text := call(); isErr {
		t.Fatalf("tools/call failed after login: %q", text)
	}

	// And the session expires while the server is still up.
	session = hub.ErrCubNotAuthenticated
	if offers() {
		t.Fatal("a connected tool is still offered although the session expired")
	}
}

// A receipt asserting a ConfigHub session that never existed is a false
// evidence claim, so the recorded fact must come from the gate. Neither site
// can be driven from a test without a cluster, so the wiring is the assertion.
func TestRecordedConnectedFactComesFromTheGate(t *testing.T) {
	// Every way the fact legitimately reaches a receipt.
	fromGate := map[string]bool{
		"watchCycleConnected":            true,
		"detectConnectedForReceipt":      true,
		"scopedConnectedMode":            true,
		"configHubReadsAvailable":        true,
		"refreshConfigHubReadsAvailable": true,
	}
	// Every file that records it. watch_receipt.go is not here: it takes the
	// value as a parameter, and the watch.go check below covers the caller.
	recordingFiles := []string{"watch.go", "receipt.go", "receipt_aggregate.go"}

	fset, files := parseRepoGoFiles(t, "cmd")
	byBase := map[string]*ast.File{}
	for path, file := range files {
		byBase[filepath.Base(path)] = file
	}

	checked := 0
	for _, base := range recordingFiles {
		file := byBase[base]
		if file == nil {
			t.Fatalf("%s is gone; this guard has stopped guarding it", base)
		}

		// What every local name in this file is assigned, so the check follows
		// a value hoisted into a variable instead of demanding a literal call.
		assigned := map[string][]ast.Expr{}
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			if name, ok := assign.Lhs[0].(*ast.Ident); ok {
				assigned[name.Name] = append(assigned[name.Name], assign.Rhs[0])
			}
			return true
		})

		var derivesFromGate func(ast.Expr, map[string]bool) bool
		derivesFromGate = func(expr ast.Expr, seen map[string]bool) bool {
			switch node := expr.(type) {
			case *ast.CallExpr:
				return fromGate[calledName(node)]
			case *ast.Ident:
				if seen[node.Name] {
					return false
				}
				seen[node.Name] = true
				values := assigned[node.Name]
				if len(values) == 0 {
					return false
				}
				// A `false` default is deliberate: a standalone read records
				// standalone. Every other assignment must reach the gate.
				sawGate := false
				for _, value := range values {
					if ident, ok := value.(*ast.Ident); ok && ident.Name == "false" {
						continue
					}
					if !derivesFromGate(value, seen) {
						return false
					}
					sawGate = true
				}
				return sawGate
			}
			return false
		}

		report := func(expr ast.Expr, what string) {
			checked++
			if !derivesFromGate(expr, map[string]bool{}) {
				t.Errorf("%s: %s does not come from the gate; a receipt would assert a ConfigHub session that was never checked", fset.Position(expr.Pos()), what)
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				// The connected argument of every receipt-attaching call.
				if calledName(node) == "attachReceiptsIfRequested" && len(node.Args) >= 5 {
					report(node.Args[4], "the connected fact this watch cycle records")
				}
			case *ast.KeyValueExpr:
				// The Connected field of every receipt built here.
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Connected" {
					report(node.Value, "the receipt's Connected field")
				}
			}
			return true
		})
	}

	if checked < 4 {
		t.Fatalf("only %d recorded connected facts found across %v; this guard has stopped guarding anything", checked, recordingFiles)
	}
}

// No command may use the older check: it probes hub.confighub.com, which says
// nothing about whether `cub` can reach its own server, and then accepts a
// token cub itself has expired.
//
// Nor may one call the gate's own implementation directly. That is how `status`
// came to report a verdict no test could reach: hub.RequireCubConnected bypasses
// the seam, so stubConnectedGate does not reach it and every mutation survives.
func TestNoCommandUsesTheOlderConnectedCheck(t *testing.T) {
	banned := map[string]string{
		"RequireConnected(":        "probes hub.confighub.com and then accepts an expired token; use requireConfigHubFor or configHubReads",
		"hub.RequireCubConnected(": "bypasses the gate's seam, so no test can reach it; call configHubReads",
	}
	var problems []string
	err := filepath.Walk(filepath.Join("..", "..", "cmd"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// connected_gate.go is where the gate is implemented, so it is the one
		// file that names the implementation.
		isGate := filepath.Base(path) == "connected_gate.go"
		for number, line := range strings.Split(string(data), "\n") {
			for call, why := range banned {
				if !strings.Contains(line, call) {
					continue
				}
				if isGate && call == "hub.RequireCubConnected(" {
					continue
				}
				problems = append(problems, fmt.Sprintf("%s:%d: %s (%s)", path, number+1, strings.TrimSpace(line), why))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d command sites reach past the gate:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}
