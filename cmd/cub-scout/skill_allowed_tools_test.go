// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A skill's allowed-tools are granted without a prompt while the skill is
// active, so every grant must stay read-only however the command line is
// completed. Claude Code matches a Bash rule's "*" against any text, flags
// included, and a later flag overrides an earlier one ("--dry-run *" admits
// "--dry-run --dry-run=false"). So a grant is a command path, optionally
// followed by " *", and the path must be one where nothing reachable writes to
// the cluster or ConfigHub, sends data to another endpoint, or deletes
// anything. Writing a local output file the user named (snapshot -o, receipt
// verify --save) is allowed.

// scoutInvocations are the three ways a skill may name cub-scout.
var scoutInvocations = []string{"./cub-scout", "cub-scout", "cub scout"}

// readOnlyScoutPaths are the cub-scout command paths a skill may grant. A
// grant ending in " *" also admits every subcommand under the path, so each
// subcommand must be listed too: adding one under a granted path fails this
// test until someone decides whether it is read-only.
var readOnlyScoutPaths = map[string]bool{
	"app list":                  true,
	"audit":                     true,
	"audit list":                true,
	"bundle diff":               true,
	"bundle inspect":            true,
	"bundle summarize":          true,
	"bundle timeline":           true,
	"catalog list":              true,
	"catalog validate":          true,
	"compare drift":             true,
	"compare source-truth":      true,
	"compare three-way":         true,
	"context-pack":              true,
	"debug":                     true,
	"doctor":                    true,
	"explain":                   true,
	"fleet":                     true,
	"fleet outliers":            true,
	"gitops status":             true,
	"graph":                     true,
	"graph explain":             true,
	"graph export":              true,
	"help":                      true,
	"history":                   true,
	"impact":                    true,
	"import cluster-aggregator": true,
	"import parse-repo":         true,
	"map activity":              true,
	"map hooks":                 true,
	"map list":                  true,
	"map meaning":               true,
	"map orphans":               true,
	"map workloads":             true,
	"mcp":                       true,
	"mcp serve":                 true,
	"patterns":                  true,
	"patterns detect":           true,
	"patterns explain":          true,
	"patterns list":             true,
	"receipt list":              true,
	"receipt show":              true,
	"receipt validate":          true,
	"receipt verify":            true,
	"scan":                      true,
	"snapshot":                  true,
	"status":                    true,
	"suggest-remedy":            true,
	"summary list":              true,
	"trace":                     true,
	"tree":                      true,
	"version":                   true,
	"views project":             true,
	"views resolve":             true,
}

// deniedScoutPaths are never grantable: each writes, sends or deletes through
// some flag or subcommand. The list documents the known cases; the allowlist
// above is what the test enforces.
var deniedScoutPaths = map[string]string{
	"bot":                "--webhook sends cluster events to any URL",
	"watch":              "--webhook sends cluster events to any URL",
	"summary slack":      "posts to a Slack webhook, from a flag or CUB_SCOUT_SLACK_WEBHOOK_URL",
	"import":             "imports into ConfigHub; --dry-run and --git-path can be overridden by a later flag",
	"import apply":       "writes ConfigHub units, spaces and targets",
	"import argocd":      "creates units and can disable sync or delete the Application",
	"app create":         "creates a ConfigHub App",
	"compare":            "--suggest --apply creates ConfigHub Apps and Deployments",
	"map queries save":   "writes the user's saved queries",
	"map queries delete": "deletes a saved query",
	"catalog init":       "creates a catalog",
	"catalog add":        "modifies a catalog",
	"bundle replay":      "runs renderers over bundle contents",
	"views open":         "opens a browser",
	"demo":               "applies demo state to the cluster",
}

// readOnlyOtherGrants are the non-cub-scout grants a skill may carry, matched
// exactly.
var readOnlyOtherGrants = map[string]bool{
	"argocd app get *":                    true,
	"argocd app list *":                   true,
	"argocd appset get *":                 true,
	"argocd appset list *":                true,
	"crossplane beta trace *":             true,
	"crossplane describe *":               true,
	"crossplane trace *":                  true,
	"cub auth status":                     true,
	"cub changeset list *":                true,
	"cub link get *":                      true,
	"cub link list *":                     true,
	"cub release list *":                  true,
	"cub resource list *":                 true,
	"cub space list *":                    true,
	"cub unit get *":                      true,
	"cub unit list *":                     true,
	"cub unit-event list *":               true,
	"cub view get *":                      true,
	"cub view list":                       true,
	"cub view list *":                     true,
	"flux events *":                       true,
	"flux get *":                          true,
	"flux get hr *":                       true,
	"flux trace *":                        true,
	"helm get *":                          true,
	"helm history *":                      true,
	"helm list *":                         true,
	"helm status *":                       true,
	"kubectl config current-context":      true,
	"kubectl config get-contexts *":       true,
	"kubectl describe *":                  true,
	"kubectl get *":                       true,
	"kubectl get --show-managed-fields *": true,
	"kubectl get events *":                true,
	"kubectl logs *":                      true,
	"kubectl version":                     true,
}

var allowedToolsTokenRE = regexp.MustCompile(`Bash\([^)]*\)|\S+`)

// skillAllowedTools returns each skill's allowed-tools grants, keyed by path.
func skillAllowedTools(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no skills/*/SKILL.md found")
	}
	grants := map[string][]string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) == 0 || lines[0] != "---" {
			t.Errorf("%s: no frontmatter", file)
			continue
		}
		for _, line := range lines[1:] {
			if line == "---" {
				break
			}
			if rest, ok := strings.CutPrefix(line, "allowed-tools:"); ok {
				grants[file] = allowedToolsTokenRE.FindAllString(strings.TrimSpace(rest), -1)
			}
		}
	}
	return grants
}

func TestSkillAllowedToolsAreReadOnly(t *testing.T) {
	rootCmd.InitDefaultHelpCmd()
	for file, tokens := range skillAllowedTools(t) {
		for _, token := range tokens {
			if err := checkSkillGrant(token); err != "" {
				t.Errorf("%s: %s: %s", file, token, err)
			}
		}
	}
}

func checkSkillGrant(token string) string {
	inner, ok := strings.CutPrefix(token, "Bash(")
	if !ok || !strings.HasSuffix(inner, ")") {
		return "only Bash(...) grants are allowed"
	}
	inner = strings.TrimSuffix(inner, ")")
	body, wildcard := strings.CutSuffix(inner, " *")
	if strings.Contains(body, "*") {
		return `"*" may appear only as a final " *": a wildcard elsewhere matches any text, including mutating flags`
	}
	for _, inv := range scoutInvocations {
		if body == inv+" --help" && !wildcard {
			return ""
		}
		rest, ok := strings.CutPrefix(body, inv+" ")
		if !ok {
			continue
		}
		if strings.HasPrefix(rest, "-") || strings.Contains(rest, " -") {
			return "a flag in a cub-scout grant can be overridden by a later flag; grant the command path only"
		}
		if reason, denied := deniedScoutPaths[rest]; denied {
			return "not read-only: " + reason
		}
		if !readOnlyScoutPaths[rest] {
			return "cub-scout " + rest + " is not in readOnlyScoutPaths"
		}
		cmd, _, err := rootCmd.Find(strings.Fields(rest))
		if err != nil || cmd.CommandPath() != "cub-scout "+rest {
			return "cub-scout " + rest + " is not a cub-scout command"
		}
		if wildcard {
			if unlisted := unlistedSubcommands(cmd); len(unlisted) > 0 {
				return "the wildcard also admits subcommands not in readOnlyScoutPaths: " + strings.Join(unlisted, ", ")
			}
		}
		return ""
	}
	if !readOnlyOtherGrants[inner] {
		return "not in readOnlyOtherGrants"
	}
	return ""
}

// unlistedSubcommands returns the paths under cmd missing from
// readOnlyScoutPaths.
func unlistedSubcommands(cmd *cobra.Command) []string {
	var out []string
	for _, sub := range cmd.Commands() {
		path := strings.TrimPrefix(sub.CommandPath(), "cub-scout ")
		if !readOnlyScoutPaths[path] {
			out = append(out, path)
		}
		out = append(out, unlistedSubcommands(sub)...)
	}
	sort.Strings(out)
	return out
}

// A skill that grants a cub-scout command grants it in all three invocation
// forms, so the skill behaves the same standalone and as a cub plugin.
func TestSkillAllowedToolsCoverEveryInvocation(t *testing.T) {
	for file, tokens := range skillAllowedTools(t) {
		have := map[string]bool{}
		for _, token := range tokens {
			have[token] = true
		}
		for _, token := range tokens {
			for _, inv := range scoutInvocations {
				rest, ok := strings.CutPrefix(token, "Bash("+inv+" ")
				if !ok {
					continue
				}
				for _, other := range scoutInvocations {
					if want := "Bash(" + other + " " + rest; !have[want] {
						t.Errorf("%s: grants %s but not %s", file, token, want)
					}
				}
				break
			}
		}
	}
}

// The checker itself must reject the grants this test exists to keep out.
func TestCheckSkillGrantRejectsOverridableGrants(t *testing.T) {
	rootCmd.InitDefaultHelpCmd()
	for _, token := range []string{
		"Bash(cub-scout import --dry-run *)",
		"Bash(cub-scout import --git-path *)",
		"Bash(cub-scout * --help)",
		"Bash(cub-scout bot *)",
		"Bash(cub-scout watch *)",
		"Bash(cub-scout summary *)",
		"Bash(cub-scout map *)",
		"Bash(cub-scout views *)",
		"Bash(cub-scout compare *)",
		"Bash(cub-scout *)",
		"Bash(cub * get)",
		"Bash(cub * list)",
		"Bash(cub history *)",
		"Bash(kubectl config view *)",
		"Bash(kubectl apply *)",
		"Write",
	} {
		if checkSkillGrant(token) == "" {
			t.Errorf("checkSkillGrant accepted %s", token)
		}
	}
	for _, token := range []string{
		"Bash(cub-scout --help)",
		"Bash(cub scout trace *)",
		"Bash(./cub-scout receipt list)",
		"Bash(cub unit get *)",
	} {
		if err := checkSkillGrant(token); err != "" {
			t.Errorf("checkSkillGrant rejected %s: %s", token, err)
		}
	}
}

// A subcommand added under a wildcard-granted path is reported until it is
// reviewed and listed.
func TestUnlistedSubcommandsReportsNewSubcommand(t *testing.T) {
	root := &cobra.Command{Use: "cub-scout"}
	group := &cobra.Command{Use: "graph"}
	group.AddCommand(&cobra.Command{Use: "export", Run: func(*cobra.Command, []string) {}})
	group.AddCommand(&cobra.Command{Use: "publish", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	got := unlistedSubcommands(group)
	if len(got) != 1 || got[0] != "graph publish" {
		t.Fatalf("unlistedSubcommands = %v, want [graph publish]", got)
	}
}
