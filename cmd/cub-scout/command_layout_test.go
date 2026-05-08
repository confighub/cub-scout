package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommandLayout_VisibleTopLevelCount(t *testing.T) {
	count := 0
	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden {
			continue
		}
		count++
	}

	// Cap is a soft governance signal. Bumped 30 -> 31 in #391 to admit
	// `views` as a real new top-level capability (View Explorer
	// integration per the council verdict on #393). Deliberate; do not
	// bump again without a paired discussion.
	if count > 31 {
		t.Fatalf("visible top-level command count = %d, want <= 31", count)
	}
}

func TestRootCommandLayout_HelpReflectsNewLayout(t *testing.T) {
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("--help returned error: %v", err)
		}
	})

	required := []string{
		"compare",
		"import",
		"quickstart",
		"setup",
	}
	for _, want := range required {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in root help output:\n%s", want, out)
		}
	}

	unexpected := []string{
		"discover",
		"health",
		"connect",
		"completion",
		"parse-repo",
		"import-argocd",
		"import-cluster-aggregator",
	}
	for _, bad := range unexpected {
		if strings.Contains(out, "\n  "+bad+" ") {
			t.Fatalf("did not expect deprecated top-level command %q in root help output:\n%s", bad, out)
		}
	}
}

func TestRootCommandLayout_ReparentedSubcommandsRegistered(t *testing.T) {
	cases := []struct {
		parent string
		child  string
	}{
		{parent: "setup", child: "connect"},
		{parent: "setup", child: "completion"},
		{parent: "import", child: "apply"},
		{parent: "import", child: "parse-repo"},
		{parent: "import", child: "argocd"},
		{parent: "import", child: "cluster-aggregator"},
		{parent: "quickstart", child: "demo"},
		{parent: "compare", child: "drift"},
	}

	for _, tc := range cases {
		var parent *cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == tc.parent {
				parent = cmd
				break
			}
		}
		if parent == nil {
			t.Fatalf("parent command %q not found", tc.parent)
		}
		if parent.CommandPath() == "" {
			t.Fatalf("parent command %q has empty path", tc.parent)
		}

		found := false
		for _, sub := range parent.Commands() {
			if sub.Name() == tc.child {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected subcommand %q under %q", tc.child, tc.parent)
		}
	}
}

func TestRootCommandLayout_DeprecatedAliasesRemainAvailable(t *testing.T) {
	cases := []string{
		"discover",
		"health",
		"connect",
		"completion",
		"apply",
		"drift",
		"demo",
		"parse-repo",
		"import-argocd",
		"import-cluster-aggregator",
	}

	for _, name := range cases {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		if cmd == nil {
			t.Fatalf("command %q not found", name)
		}
		if !cmd.Hidden {
			t.Fatalf("expected %q to be hidden from root help", name)
		}
		if cmd.Deprecated == "" {
			t.Fatalf("expected %q to be marked deprecated", name)
		}
	}
}
