// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	deliveryTreeViewAll     = "all"
	deliveryTreeViewSummary = "summary"

	// deliveryTreeEvidence is said in every format: it is the one thing a
	// reader must not get wrong about a leaf.
	deliveryTreeEvidence = "Each deployer's resources are what it reports about itself. They are not checked against the cluster."
)

var gitopsTreeCmd = &cobra.Command{
	Use:   "tree [<kind>/<name>]",
	Short: "Show what is under each deployer, to any depth: child deployers and their resources",
	Long: `Show what each GitOps deployer delivers, and what the deployers it delivers
deliver in turn.

Start from one deployer, or from every deployer that no other deployer
delivers. Under each are the deployers it reports as its own, walked the same
way, and the other resources it reports. An Argo CD app-of-apps is the usual
case: a root Application, the Applications it creates, and theirs.

Each deployer is shown with its own sync and health, and with the delivery
settings 'gitops settings' reports for it. That is the point of the tree: a
parent's settings apply to the child deployer object itself. Where a parent
has an ignore rule that names a child, the rule is shown on the child.

Evidence. The tree is built from what each deployer reports about itself in
the cluster: an Argo CD Application's status.resources. Nothing asks Argo CD's
own API. A resource in the tree is one the deployer says it manages; it is not
checked against the cluster. Argo CD 3 keeps resource health in its own tree
and not on the Application, so a resource shows its sync status and no health.

What a Flux Kustomization or HelmRelease delivers is not read yet. They appear
in the tree with their own state and are marked as such.

A deployer that could not be read, was not found, or says nothing about what
it applied is kept in the tree and marked. It is never dropped, and never
shown as having nothing under it.

Examples:
  # Every root deployer and what is under it
  cub-scout gitops tree

  # One app-of-apps and everything under it
  cub-scout gitops tree app/platform -n argocd

  # Only the first level under it
  cub-scout gitops tree app/platform --depth 1

  # Where its Deployments are, and which child Application has each
  cub-scout gitops tree app/platform --kind Deployment

  # What is out of sync under it, as JSON
  cub-scout gitops tree app/platform --sync OutOfSync --format json
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGitOpsTree,
}

func init() {
	gitopsCmd.AddCommand(gitopsTreeCmd)
	addGitOpsTreeFlags(gitopsTreeCmd.Flags())
}

func addGitOpsTreeFlags(flags *pflag.FlagSet) {
	flags.StringP("namespace", "n", "", "Only deployers in this namespace, and the namespace of the deployer named (default: all namespaces)")
	flags.Int("depth", 0, "How many levels below a root to walk (default 0: no limit)")
	flags.StringSlice("kind", nil, "Only entries of this kind, with the deployers above them (repeatable)")
	flags.StringSlice("sync", nil, "Only entries with this sync status, such as OutOfSync, with the deployers above them (repeatable)")
	flags.StringSlice("health", nil, "Only entries with this health, such as Degraded, with the deployers above them (repeatable)")
	flags.Int("max-resources", 25, "Text output: list at most this many resources under one deployer and count the rest (0: list all)")
	flags.String("format", "ascii", "Output format: ascii, json, md")
	flags.Bool("json", false, "Output as JSON (shorthand for --format json)")
	flags.String("view", deliveryTreeViewAll, "JSON only: all, or summary (compact: resources counted by kind, and listed only when not Synced)")
	flags.Bool("tui", false, "View this snapshot in a scrollable terminal viewport")
}

// deliveryTreeParams are the validated flags and argument of gitops tree.
type deliveryTreeParams struct {
	Namespace    string
	Root         *agent.DeliveryRef
	Depth        int
	Kinds        []string
	Syncs        []string
	Healths      []string
	MaxResources int
	View         string
}

func (p deliveryTreeParams) filtered() bool {
	return len(p.Kinds)+len(p.Syncs)+len(p.Healths) > 0
}

type deliveryTreeFilters struct {
	Kinds   []string `json:"kinds,omitempty"`
	Syncs   []string `json:"syncs,omitempty"`
	Healths []string `json:"healths,omitempty"`
}

type deliveryTreeShown struct {
	Deployers int `json:"deployers"`
	Resources int `json:"resources"`
}

// deliveryTreeReport is the gitops tree output model.
type deliveryTreeReport struct {
	Context   string `json:"context,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Root      string `json:"root,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	// Complete is false when a list or a child deployer could not be read:
	// the tree is then missing something it cannot name.
	Complete bool   `json:"complete"`
	Evidence string `json:"evidence"`
	// Summary counts the whole tree, before any filter.
	Summary agent.DeliveryTreeSummary `json:"summary"`
	Filters *deliveryTreeFilters      `json:"filters,omitempty"`
	// Shown counts what is left after the filters.
	Shown *deliveryTreeShown           `json:"shown,omitempty"`
	Roots []agent.DeliveryTreeNode     `json:"roots"`
	Reads []agent.DeliverySettingsRead `json:"reads"`
	Notes []string                     `json:"notes,omitempty"`
}

var deliveryTreeKindAliases = map[string]string{
	"app": "Application", "apps": "Application", "application": "Application", "applications": "Application",
	"ks": "Kustomization", "kustomization": "Kustomization", "kustomizations": "Kustomization",
	"hr": "HelmRelease", "helmrelease": "HelmRelease", "helmreleases": "HelmRelease",
}

// parseDeliveryRef reads `<kind>/<name>` or `<kind>/<namespace>/<name>`.
func parseDeliveryRef(raw, namespace string) (*agent.DeliveryRef, error) {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("invalid deployer %q (examples: app/platform, ks/flux-system/apps)", raw)
	}
	kind, known := deliveryTreeKindAliases[strings.ToLower(parts[0])]
	if !known {
		return nil, fmt.Errorf("unknown deployer kind %q (valid: app, ks, hr)", parts[0])
	}
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "" {
			return nil, fmt.Errorf("invalid deployer %q (examples: app/platform, ks/flux-system/apps)", raw)
		}
	}
	ref := &agent.DeliveryRef{Kind: kind, Namespace: namespace, Name: parts[len(parts)-1]}
	if len(parts) == 3 {
		if namespace != "" && namespace != parts[1] {
			return nil, fmt.Errorf("%q names namespace %q and --namespace names %q", raw, parts[1], namespace)
		}
		ref.Namespace = parts[1]
	}
	return ref, nil
}

func cleanDeliveryList(values []string) []string {
	var cleaned []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func deliveryTreeParamsFromFlags(cmd *cobra.Command, args []string) (deliveryTreeParams, error) {
	flags := cmd.Flags()
	params := deliveryTreeParams{}
	params.Namespace, _ = flags.GetString("namespace")
	params.Namespace = strings.TrimSpace(params.Namespace)
	params.Depth, _ = flags.GetInt("depth")
	params.MaxResources, _ = flags.GetInt("max-resources")
	kinds, _ := flags.GetStringSlice("kind")
	syncs, _ := flags.GetStringSlice("sync")
	healths, _ := flags.GetStringSlice("health")
	params.Kinds, params.Syncs, params.Healths = cleanDeliveryList(kinds), cleanDeliveryList(syncs), cleanDeliveryList(healths)
	params.View, _ = flags.GetString("view")
	params.View = strings.ToLower(strings.TrimSpace(params.View))
	switch {
	case params.Depth < 0:
		return params, fmt.Errorf("invalid --depth %d (0 or more)", params.Depth)
	case params.MaxResources < 0:
		return params, fmt.Errorf("invalid --max-resources %d (0 or more)", params.MaxResources)
	case params.View != deliveryTreeViewAll && params.View != deliveryTreeViewSummary:
		return params, fmt.Errorf("invalid --view %q (valid: all, summary)", params.View)
	}
	if len(args) == 1 {
		ref, err := parseDeliveryRef(args[0], params.Namespace)
		if err != nil {
			return params, err
		}
		params.Root = ref
	}
	return params, nil
}

func runGitOpsTree(cmd *cobra.Command, args []string) error {
	params, err := deliveryTreeParamsFromFlags(cmd, args)
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	legacyJSON, _ := cmd.Flags().GetBool("json")
	format, err = normalizeGitOpsStatusFormat(format, legacyJSON)
	if err != nil {
		return err
	}
	if format != "json" && cmd.Flags().Changed("view") {
		return fmt.Errorf("--view applies to --format json")
	}
	tui, _ := cmd.Flags().GetBool("tui")
	if err := validateGitOpsTUIFormat(tui, cmd.Flags().Changed("format"), cmd.Flags().Changed("json")); err != nil {
		return err
	}
	ctx, err := boundCommandContext(cmd)
	if err != nil {
		return err
	}
	client, err := newReceiptDeliveryDynamicClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to build kubernetes client: %w", err)
	}
	report, err := buildDeliveryTreeReport(ctx, client, params)
	if err != nil {
		return err
	}
	report.Context = boundContextLabel(ctx)
	if tui {
		return runGitOpsMarkdownTUI(ctx, renderDeliveryTreeMarkdown(report, params.MaxResources))
	}
	return writeDeliveryTree(cmd.OutOrStdout(), report, format, params)
}

// buildDeliveryTreeReport walks the tree and applies the filters.
func buildDeliveryTreeReport(ctx context.Context, client dynamic.Interface, params deliveryTreeParams) (deliveryTreeReport, error) {
	// A deployer named with its namespace is looked for there; the listing
	// stays as wide as --namespace says, so its children elsewhere are found.
	tree, err := agent.CollectDeliveryTree(ctx, client, agent.DeliveryTreeOptions{Namespace: params.Namespace, Root: params.Root, Depth: params.Depth})
	report := deliveryTreeReport{
		Namespace: params.Namespace, Depth: params.Depth, Evidence: deliveryTreeEvidence,
		Summary: tree.Summary, Roots: tree.Roots, Reads: tree.Reads, Complete: tree.Summary.NotRead == 0,
	}
	for _, read := range tree.Reads {
		if read.Status == agent.DeliveryReadNotRead {
			report.Complete = false
		}
	}
	if params.Root != nil {
		report.Root = params.Root.String()
	}
	if err != nil {
		var rootErr *agent.DeliveryRootError
		if errors.As(err, &rootErr) && len(rootErr.Candidates) == 0 && !report.Complete {
			// Not found is not known when a list could not be read.
			return report, fmt.Errorf("%w; some deployers could not be listed, so it may exist", err)
		}
		return report, err
	}
	if report.Roots == nil {
		report.Roots = []agent.DeliveryTreeNode{}
	}
	if tree.Summary.Resources > 0 && !deliveryTreeReportsHealth(report.Roots) {
		report.Notes = append(report.Notes, "No resource health is reported: Argo CD keeps it in its own tree, not on the Application. A resource with no health shown is not known to be healthy.")
	}
	if tree.Summary.NotSupported > 0 {
		report.Notes = append(report.Notes, "What a Flux Kustomization or HelmRelease delivers is not read yet (#856); those deployers are shown without their children.")
	}
	if params.filtered() {
		report.Filters = &deliveryTreeFilters{Kinds: params.Kinds, Syncs: params.Syncs, Healths: params.Healths}
		report.Roots = filterDeliveryTree(report.Roots, params)
		shown := deliveryTreeShown{}
		countDeliveryTree(report.Roots, &shown)
		report.Shown = &shown
	}
	return report, nil
}

func deliveryTreeReportsHealth(nodes []agent.DeliveryTreeNode) bool {
	for _, node := range nodes {
		for _, resource := range node.Children.Resources {
			if resource.Health != "" {
				return true
			}
		}
		if deliveryTreeReportsHealth(node.Children.Deployers) {
			return true
		}
	}
	return false
}

func countDeliveryTree(nodes []agent.DeliveryTreeNode, shown *deliveryTreeShown) {
	for _, node := range nodes {
		shown.Deployers++
		shown.Resources += len(node.Children.Resources)
		countDeliveryTree(node.Children.Deployers, shown)
	}
}

func deliveryInFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}

// deliveryEntryMatches reports whether an entry with this kind, sync and
// health passes every filter that was given.
func deliveryEntryMatches(params deliveryTreeParams, kind, sync, health string) bool {
	if len(params.Kinds) > 0 && !deliveryInFold(params.Kinds, kind) {
		return false
	}
	if len(params.Syncs) > 0 && !deliveryInFold(params.Syncs, sync) {
		return false
	}
	if len(params.Healths) > 0 && !deliveryInFold(params.Healths, health) {
		return false
	}
	return true
}

// filterDeliveryTree keeps the entries that pass the filters and every
// deployer above one. A deployer is matched on its own sync and health, as it
// reports them. What marks a deployer (not found, nothing reported) stays on
// it: a kept ancestor still says what the walk could not see beneath it.
func filterDeliveryTree(nodes []agent.DeliveryTreeNode, params deliveryTreeParams) []agent.DeliveryTreeNode {
	kept := []agent.DeliveryTreeNode{}
	for _, node := range nodes {
		var resources []agent.DeliveryReported
		for _, resource := range node.Children.Resources {
			if deliveryEntryMatches(params, resource.Kind, resource.Sync, resource.Health) {
				resources = append(resources, resource)
			}
		}
		deployers := filterDeliveryTree(node.Children.Deployers, params)
		sync, health := "", ""
		if node.State != nil {
			sync, health = node.State.Sync, node.State.Health
		}
		if len(resources) == 0 && len(deployers) == 0 && !deliveryEntryMatches(params, node.Kind, sync, health) {
			continue
		}
		node.Children.Resources = resources
		node.Children.Deployers = nil
		if len(deployers) > 0 {
			node.Children.Deployers = deployers
		}
		kept = append(kept, node)
	}
	return kept
}

// deliveryTreeSummaryNode is a deployer in the summary view: its resources
// counted by kind, and named only when they are not Synced.
type deliveryTreeSummaryNode struct {
	Deployer string   `json:"deployer"`
	Object   string   `json:"object,omitempty"`
	Sync     string   `json:"sync,omitempty"`
	Health   string   `json:"health,omitempty"`
	Policies string   `json:"policies,omitempty"`
	Marks    []string `json:"marks,omitempty"`
	// Resources counts what the deployer reports, by kind.
	Resources map[string]int `json:"resources,omitempty"`
	// NotSynced names each reported resource whose sync is not Synced.
	NotSynced []string                  `json:"notSynced,omitempty"`
	Deployers []deliveryTreeSummaryNode `json:"deployers,omitempty"`
}

func deliveryPoliciesLine(policies []agent.DeliveryPolicyValue) string {
	parts := make([]string, len(policies))
	for i, policy := range policies {
		parts[i] = policy.Name + " " + policy.Value
	}
	return strings.Join(parts, ", ")
}

// deliveryNodeMarks says in words what is unusual about a deployer.
func deliveryNodeMarks(node agent.DeliveryTreeNode) []string {
	var marks []string
	switch node.Object {
	case agent.DeliveryObjectNotFound:
		mark := "not found in the cluster"
		if node.ObjectReason != "" {
			mark += ": " + node.ObjectReason
		}
		marks = append(marks, mark)
	case agent.DeliveryObjectNotRead:
		marks = append(marks, "could not be read: "+node.ObjectReason)
	}
	if node.State != nil && !node.State.Reconciled {
		marks = append(marks, "not reconciled: the object has no status")
	}
	if node.ReportedBy > 1 {
		marks = append(marks, fmt.Sprintf("reported by %d deployers", node.ReportedBy))
	}
	if node.GeneratedBy != "" {
		marks = append(marks, "generated by "+node.GeneratedBy)
	}
	if node.ReportedByParent != nil {
		if rules := deliveryIgnoreRulesLine(node.ReportedByParent.IgnoredByParent); rules != "" {
			marks = append(marks, "parent ignores differences at "+rules)
		}
	}
	if node.State != nil {
		for _, condition := range node.State.Conditions {
			// Argo CD lists only problems as conditions; Flux lists Ready
			// and friends, which the health already says.
			if node.Controller != agent.DeliveryControllerArgoCD {
				continue
			}
			marks = append(marks, condition.Type+": "+deliveryShorten(condition.Message, 160))
		}
	}
	switch node.Children.Status {
	case agent.DeliveryChildrenNoneReported:
		marks = append(marks, "reports nothing about what it applied: "+node.Children.Reason)
	case agent.DeliveryChildrenNotSupported:
		marks = append(marks, node.Children.Reason)
	case agent.DeliveryChildrenDepthLimit:
		marks = append(marks, "not walked further: --depth")
	case agent.DeliveryChildrenCycle:
		marks = append(marks, "already above in this branch: not walked again")
	}
	return marks
}

func deliveryShorten(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if characters := []rune(text); len(characters) > limit {
		return string(characters[:limit]) + "…"
	}
	return text
}

// deliveryIgnoreRulesLine names what a set of ignore rules ignores.
func deliveryIgnoreRulesLine(rules []interface{}) string {
	var what []string
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for _, field := range []string{"jsonPointers", "jqPathExpressions", "managedFieldsManagers"} {
			values, _ := rule[field].([]interface{})
			for _, value := range values {
				text := fmt.Sprint(value)
				if field == "managedFieldsManagers" {
					text = "fields managed by " + text
				}
				what = append(what, text)
			}
		}
	}
	return strings.Join(what, ", ")
}

func deliveryRefLine(kind, namespace, name string) string {
	if namespace == "" {
		return kind + " " + name
	}
	return kind + " " + namespace + "/" + name
}

func summariseDeliveryTree(nodes []agent.DeliveryTreeNode) []deliveryTreeSummaryNode {
	var out []deliveryTreeSummaryNode
	for _, node := range nodes {
		summary := deliveryTreeSummaryNode{
			Deployer: deliveryRefLine(node.Kind, node.Namespace, node.Name),
			Policies: deliveryPoliciesLine(node.Policies), Marks: deliveryNodeMarks(node),
		}
		if node.Object != agent.DeliveryObjectFound {
			summary.Object = node.Object
		}
		if node.State != nil {
			summary.Sync, summary.Health = node.State.Sync, node.State.Health
		}
		for _, resource := range node.Children.Resources {
			if summary.Resources == nil {
				summary.Resources = map[string]int{}
			}
			summary.Resources[resource.Kind]++
			if resource.Sync != "" && resource.Sync != "Synced" {
				summary.NotSynced = append(summary.NotSynced, deliveryRefLine(resource.Kind, resource.Namespace, resource.Name)+" "+resource.Sync)
			}
		}
		summary.Deployers = summariseDeliveryTree(node.Children.Deployers)
		out = append(out, summary)
	}
	return out
}

func writeDeliveryTree(w io.Writer, report deliveryTreeReport, format string, params deliveryTreeParams) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		if params.View == deliveryTreeViewSummary {
			// The summary exists to be cheap to read: one line, with the
			// leaves counted and only the exceptions named.
			encoded, err := json.Marshal(report)
			if err != nil {
				return err
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &document); err != nil {
				return err
			}
			delete(document, "roots")
			document["view"], _ = json.Marshal(deliveryTreeViewSummary)
			if document["tree"], err = json.Marshal(summariseDeliveryTree(report.Roots)); err != nil {
				return err
			}
			return encoder.Encode(document)
		}
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "md":
		_, err := io.WriteString(w, renderDeliveryTreeMarkdown(report, params.MaxResources))
		return err
	default:
		_, err := io.WriteString(w, renderDeliveryTreeASCII(report, params.MaxResources))
		return err
	}
}

func deliveryTreeHeadline(report deliveryTreeReport) string {
	summary := report.Summary
	line := fmt.Sprintf("%s, %s, %s reported",
		deliveryCount(summary.Roots, "root"), deliveryCount(summary.Deployers, "deployer"), deliveryCount(summary.Resources, "resource"))
	if report.Shown != nil {
		line += fmt.Sprintf("; showing %s and %s", deliveryCount(report.Shown.Deployers, "deployer"), deliveryCount(report.Shown.Resources, "resource"))
	}
	return line
}

func deliveryCount(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// deliveryTreeCaveats are the summary counts a reader must not miss.
func deliveryTreeCaveats(report deliveryTreeReport) []string {
	summary := report.Summary
	var caveats []string
	add := func(n int, text string) {
		if n > 0 {
			caveats = append(caveats, fmt.Sprintf("%d %s", n, text))
		}
	}
	add(summary.NotFound, "reported but not found in the cluster")
	add(summary.NotRead, "could not be read")
	add(summary.NotReconciled, "with no status: not reconciled")
	add(summary.NoneReported, "report nothing about what they applied")
	add(summary.NotSupported, "of a kind whose children are not read yet")
	add(summary.Shared, "reported by more than one deployer")
	add(summary.Cycles, "already above in their own branch")
	add(summary.DepthLimited, "not walked further because of --depth")
	return caveats
}

func deliveryNodeLine(node agent.DeliveryTreeNode) string {
	parts := []string{deliveryText(deliveryRefLine(node.Kind, node.Namespace, node.Name))}
	if node.State != nil {
		var state []string
		for _, value := range []string{node.State.Sync, node.State.Health} {
			if value != "" {
				state = append(state, deliveryText(value))
			}
		}
		if len(state) > 0 {
			parts = append(parts, strings.Join(state, " / "))
		}
	}
	if policies := deliveryPoliciesLine(node.Policies); policies != "" {
		parts = append(parts, deliveryText(policies))
	}
	return strings.Join(parts, "  ·  ")
}

func deliveryResourceLine(resource agent.DeliveryReported) string {
	line := deliveryText(deliveryRefLine(resource.Kind, resource.Namespace, resource.Name))
	if resource.Sync != "" {
		line += "  ·  " + deliveryText(resource.Sync)
	}
	if resource.Health != "" {
		line += " / " + deliveryText(resource.Health)
	}
	if resource.RequiresPruning {
		line += "  ·  no longer in the source: would be pruned"
	}
	if rules := deliveryIgnoreRulesLine(resource.IgnoredByParent); rules != "" {
		line += "  ·  differences ignored at " + deliveryText(rules)
	}
	return line
}

// deliveryResourceOverflow counts the resources left out of a text listing,
// by kind.
func deliveryResourceOverflow(resources []agent.DeliveryReported) string {
	counts := map[string]int{}
	for _, resource := range resources {
		counts[resource.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	parts := make([]string, len(kinds))
	for i, kind := range kinds {
		parts[i] = fmt.Sprintf("%d %s", counts[kind], deliveryText(kind))
	}
	return fmt.Sprintf("… %s more (%s); --max-resources 0 lists all", deliveryCount(len(resources), "resource"), strings.Join(parts, ", "))
}

func renderDeliveryTreeASCII(report deliveryTreeReport, maxResources int) string {
	var b strings.Builder
	title := "DELIVERY TREE"
	if report.Root != "" {
		title += "  " + deliveryText(report.Root)
	}
	if report.Context != "" {
		title += "  (context " + deliveryText(report.Context) + ")"
	}
	b.WriteString(title + "\n")
	b.WriteString(deliveryTreeHeadline(report) + "\n")
	b.WriteString(deliveryTreeEvidence + "\n")
	if !report.Complete {
		b.WriteString("INCOMPLETE: something could not be read; see Reads.\n")
	}
	for _, caveat := range deliveryTreeCaveats(report) {
		b.WriteString("  ! " + caveat + "\n")
	}
	b.WriteString("\n")

	var walk func(node agent.DeliveryTreeNode, prefix string, root, last bool)
	walk = func(node agent.DeliveryTreeNode, prefix string, root, last bool) {
		branch, below := "├─ ", "│  "
		if last {
			branch, below = "└─ ", "   "
		}
		if root {
			branch, below = "", ""
		}
		b.WriteString(prefix + branch + deliveryNodeLine(node) + "\n")
		inner := prefix + below
		for _, mark := range deliveryNodeMarks(node) {
			b.WriteString(inner + "   ! " + deliveryText(mark) + "\n")
		}
		resources := node.Children.Resources
		var overflow []agent.DeliveryReported
		if maxResources > 0 && len(resources) > maxResources {
			resources, overflow = resources[:maxResources], resources[maxResources:]
		}
		lines := len(node.Children.Deployers) + len(resources)
		if len(overflow) > 0 {
			lines++
		}
		written := 0
		for _, child := range node.Children.Deployers {
			written++
			walk(child, inner, false, written == lines)
		}
		for _, resource := range resources {
			written++
			leaf := "├─ "
			if written == lines {
				leaf = "└─ "
			}
			b.WriteString(inner + leaf + deliveryResourceLine(resource) + "\n")
		}
		if len(overflow) > 0 {
			b.WriteString(inner + "└─ " + deliveryResourceOverflow(overflow) + "\n")
		}
	}
	for _, root := range report.Roots {
		walk(root, "", true, true)
		b.WriteString("\n")
	}
	if len(report.Roots) == 0 {
		if report.Filters != nil {
			b.WriteString("Nothing matches the filters.\n\n")
		} else {
			b.WriteString("No deployers were found.\n\n")
		}
	}
	for _, note := range report.Notes {
		b.WriteString("Note: " + note + "\n")
	}
	b.WriteString("Reads:\n")
	for _, read := range report.Reads {
		line := fmt.Sprintf("  %s %s: %s", deliveryControllerTitle(read.Controller), read.Kind, strings.ReplaceAll(read.Status, "_", " "))
		if read.Status == agent.DeliveryReadRead {
			line += fmt.Sprintf(" (%d)", read.Count)
		}
		if read.Reason != "" {
			line += " (" + deliveryText(read.Reason) + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func deliveryMarkdownText(text string) string {
	return strings.NewReplacer("|", "\\|", "`", "'", "*", "\\*", "_", "\\_", "<", "&lt;", ">", "&gt;").Replace(deliveryText(text))
}

func renderDeliveryTreeMarkdown(report deliveryTreeReport, maxResources int) string {
	var b strings.Builder
	title := "# Delivery tree"
	if report.Root != "" {
		title += ": " + deliveryMarkdownText(report.Root)
	}
	b.WriteString(title + "\n\n")
	if report.Context != "" {
		b.WriteString("Context: `" + strings.ReplaceAll(deliveryText(report.Context), "`", "'") + "`\n\n")
	}
	b.WriteString(deliveryTreeHeadline(report) + ".\n\n")
	b.WriteString("> " + deliveryTreeEvidence + "\n\n")
	if !report.Complete {
		b.WriteString("**Incomplete:** something could not be read; see Reads.\n\n")
	}
	for _, caveat := range deliveryTreeCaveats(report) {
		b.WriteString("- **" + caveat + "**\n")
	}
	if len(deliveryTreeCaveats(report)) > 0 {
		b.WriteString("\n")
	}
	var walk func(node agent.DeliveryTreeNode, depth int)
	walk = func(node agent.DeliveryTreeNode, depth int) {
		indent := strings.Repeat("  ", depth)
		b.WriteString(indent + "- **" + deliveryMarkdownText(deliveryRefLine(node.Kind, node.Namespace, node.Name)) + "**")
		if node.State != nil {
			for _, value := range []string{node.State.Sync, node.State.Health} {
				if value != "" {
					b.WriteString(" · " + deliveryMarkdownText(value))
				}
			}
		}
		if policies := deliveryPoliciesLine(node.Policies); policies != "" {
			b.WriteString(" · " + deliveryMarkdownText(policies))
		}
		b.WriteString("\n")
		for _, mark := range deliveryNodeMarks(node) {
			b.WriteString(indent + "  - _" + deliveryMarkdownText(mark) + "_\n")
		}
		for _, child := range node.Children.Deployers {
			walk(child, depth+1)
		}
		resources := node.Children.Resources
		var overflow []agent.DeliveryReported
		if maxResources > 0 && len(resources) > maxResources {
			resources, overflow = resources[:maxResources], resources[maxResources:]
		}
		for _, resource := range resources {
			b.WriteString(indent + "  - " + deliveryMarkdownText(deliveryResourceLine(resource)) + "\n")
		}
		if len(overflow) > 0 {
			b.WriteString(indent + "  - " + deliveryMarkdownText(deliveryResourceOverflow(overflow)) + "\n")
		}
	}
	for _, root := range report.Roots {
		walk(root, 0)
	}
	if len(report.Roots) == 0 {
		if report.Filters != nil {
			b.WriteString("Nothing matches the filters.\n")
		} else {
			b.WriteString("No deployers were found.\n")
		}
	}
	b.WriteString("\n")
	for _, note := range report.Notes {
		b.WriteString("Note: " + deliveryMarkdownText(note) + "\n\n")
	}
	b.WriteString("## Reads\n\n| Controller | Kind | Status | Count |\n|---|---|---|---|\n")
	for _, read := range report.Reads {
		count := ""
		if read.Status == agent.DeliveryReadRead {
			count = fmt.Sprint(read.Count)
		}
		status := strings.ReplaceAll(read.Status, "_", " ")
		if read.Reason != "" {
			status += " (" + deliveryMarkdownText(read.Reason) + ")"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", deliveryControllerTitle(read.Controller), read.Kind, status, count))
	}
	return b.String()
}
