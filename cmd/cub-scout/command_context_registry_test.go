package main

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

// #599: every command that reads a cluster takes an explicit context
// (--kube-context) and never falls back to the ambient one. A command without
// the flag must be listed here, with why: either it reads no cluster, or it
// does and the explicit context is still owed. An owed entry must still lack
// the flag, so it is removed from this list when the flag lands; a new
// command that is in neither list fails.

// contextExempt read no cluster through the Kubernetes API.
var contextExempt = map[string]string{
	"cub-scout":                           "prints the landing page",
	"cub-scout app create":                "writes a ConfigHub App",
	"cub-scout app list":                  "reads ConfigHub",
	"cub-scout audit list":                "reads ConfigHub ChangeSets",
	"cub-scout bundle diff":               "reads bundle files",
	"cub-scout bundle inspect":            "reads a bundle file",
	"cub-scout bundle replay":             "reads a bundle file",
	"cub-scout bundle summarize":          "reads a bundle file",
	"cub-scout bundle timeline":           "reads a bundle file",
	"cub-scout catalog add":               "writes a local catalog",
	"cub-scout catalog init":              "writes a local catalog",
	"cub-scout catalog list":              "reads a local catalog",
	"cub-scout catalog validate":          "reads a local catalog",
	"cub-scout demo list":                 "lists built-in demos",
	"cub-scout history":                   "reads ConfigHub ChangeSets",
	"cub-scout history changeorder":       "reads a ConfigHub ChangeOrder",
	"cub-scout impact":                    "reads ConfigHub",
	"cub-scout import apply":              "writes a proposal to ConfigHub",
	"cub-scout import cluster-aggregator": "reads files or stdin",
	"cub-scout import parse-repo":         "reads a local Git checkout",
	"cub-scout map fleet":                 "reads ConfigHub",
	"cub-scout map queries":               "reads local saved queries",
	"cub-scout map queries connect":       "writes local saved queries",
	"cub-scout map queries delete":        "writes local saved queries",
	"cub-scout map queries save":          "writes local saved queries",
	"cub-scout mcp serve":                 "each tool that reads a cluster takes its own context argument",
	"cub-scout patterns list":             "lists built-in patterns",
	"cub-scout quickstart demo list":      "lists built-in demos",
	"cub-scout receipt chain":              "reads local receipts",
	"cub-scout receipt digest":             "reads local receipts",
	"cub-scout receipt export-attestation": "exports an in-toto v1 attestation from a local signed receipt offline",
	"cub-scout receipt keygen":             "generates local Ed25519 key files offline",
	"cub-scout receipt list":              "reads the local receipt store",
	"cub-scout receipt show":              "reads the local receipt store",
	"cub-scout receipt sign":              "signs a local receipt or landed payload offline",
	"cub-scout receipt validate":          "reads a local receipt",
	"cub-scout receipt verify-fleet":      "verifies and aggregates local signed receipts offline",
	"cub-scout setup":                     "local setup",
	"cub-scout setup completion":          "prints shell completion",
	"cub-scout setup connect":             "merges kubeconfigs, choosing its source context by its own flags",
	"cub-scout status":                    "reports the kubeconfig's current context name; reads no cluster",
	"cub-scout summary list":              "reads ConfigHub summary storage",
	"cub-scout summary slack":             "reads ConfigHub summary storage",
	"cub-scout version":                   "prints the version",
	"cub-scout views open":                "opens a ConfigHub View in the browser",
	"cub-scout views resolve":             "reads a ConfigHub View",
}

// contextOwed read the cluster through the ambient context today (#599).
var contextOwed = map[string]string{}

func hasKubeContextFlag(cmd *cobra.Command) bool {
	return cmd.Flags().Lookup("kube-context") != nil || cmd.InheritedFlags().Lookup("kube-context") != nil
}

func TestEveryCommandTakesAnExplicitContextOrSaysWhyNot(t *testing.T) {
	seen := map[string]bool{}
	var unclassified []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && !cmd.Hidden && cmd.Deprecated == "" && cmd.Name() != "help" {
			path := cmd.CommandPath()
			seen[path] = true
			_, exempt := contextExempt[path]
			_, owed := contextOwed[path]
			switch {
			case hasKubeContextFlag(cmd) && (exempt || owed):
				t.Errorf("%s takes --kube-context; remove it from %s", path, map[bool]string{true: "contextExempt", false: "contextOwed"}[exempt])
			case !hasKubeContextFlag(cmd) && !exempt && !owed:
				unclassified = append(unclassified, path)
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
	sort.Strings(unclassified)
	for _, path := range unclassified {
		t.Errorf("%s takes no --kube-context and is in neither contextExempt nor contextOwed: give it the flag if it reads a cluster", path)
	}
	for _, list := range []map[string]string{contextExempt, contextOwed} {
		for path := range list {
			if !seen[path] {
				t.Errorf("%s is listed but is not a runnable command; remove it", path)
			}
		}
	}
}
