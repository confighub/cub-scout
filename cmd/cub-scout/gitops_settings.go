// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	deliverySettingsGroupByProject  = "project"
	deliverySettingsGroupBySetting  = "setting"
	deliverySettingsGroupByDeployer = "deployer"

	// JSON views. "all" is the full report; the others keep the scope, counts
	// and reads and drop the two views that were not asked for, so a caller
	// that pays per byte reads one inversion and not three.
	deliverySettingsViewAll       = "all"
	deliverySettingsViewSummary   = "summary"
	deliverySettingsViewGroups    = "groups"
	deliverySettingsViewSettings  = "settings"
	deliverySettingsViewDeployers = "deployers"
)

var gitopsSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Show how each Argo CD and Flux deployer is set to sync, correct and prune",
	Long: `Show the delivery settings declared by each GitOps deployer, and which
deployers share each setting.

For Argo CD Applications: auto-sync, self-heal, prune, every sync option with
its value, and ignoreDifferences. For Flux Kustomizations: suspend, prune,
force, wait and deletionPolicy. For Flux HelmReleases: suspend, drift
detection, and every declared install, upgrade, rollback, uninstall and test
field.

These are the settings written in each object's spec. The command does not
judge them, and does not report whether a controller acted on them.

A setting that is absent is shown as unset, with the controller's documented
default named beside it; it is not shown as a declared "off". A kind that
could not be listed is reported under Reads as not read, never as empty.

Argo CD Applications are grouped by project. Flux has no project, so Flux
objects are grouped by namespace.

Examples:
  # Every deployer, grouped by project (Argo CD) or namespace (Flux)
  cub-scout gitops settings

  # One list per setting across the whole cluster
  cub-scout gitops settings --group-by setting

  # Applications that sync automatically but do not self-heal
  cub-scout gitops settings --setting self-heal=off

  # Everything that skips schema validation, as Markdown for a ticket
  cub-scout gitops settings --setting Validate=false --format md

  # One Argo CD project, machine-readable
  cub-scout gitops settings --project payments --format json
`,
	Args: cobra.NoArgs,
	RunE: runGitOpsSettings,
}

func init() {
	gitopsCmd.AddCommand(gitopsSettingsCmd)
	addGitOpsSettingsFlags(gitopsSettingsCmd.Flags())
}

func addGitOpsSettingsFlags(flags *pflag.FlagSet) {
	flags.StringP("namespace", "n", "", "Only deployers in this namespace (default: all namespaces)")
	flags.StringSlice("project", nil, "Only Argo CD Applications in this project (repeatable; Flux objects have no project and are left out)")
	flags.StringArray("setting", nil, "Only deployers with this setting, as name or name=value (repeatable; all must match)")
	flags.String("group-by", deliverySettingsGroupByProject, "Group by: project, setting, deployer")
	flags.String("format", "ascii", "Output format: ascii, json, md")
	flags.Bool("json", false, "Output as JSON (shorthand for --format json)")
	flags.String("view", deliverySettingsViewAll, "JSON only: all, summary (compact, per project or namespace), groups, settings (per kind), or deployers (per object)")
	flags.Bool("tui", false, "View this snapshot in a scrollable terminal viewport")
}

// deliverySettingsParams are the validated flags of gitops settings.
type deliverySettingsParams struct {
	Namespace string
	Projects  []string
	Settings  []string
	GroupBy   string
	View      string
}

type deliverySettingsFilters struct {
	Projects []string `json:"projects,omitempty"`
	Settings []string `json:"settings,omitempty"`
}

type deliveryKindCount struct {
	Controller string `json:"controller"`
	Kind       string `json:"kind"`
	// Status is the kind's read status. Read and Shown are counts only when
	// it is "read": a kind that was not read has no known count, not zero.
	Status string `json:"status"`
	// Read is how many objects of the kind were read; Shown is how many are
	// left after --project and --setting.
	Read  int `json:"read"`
	Shown int `json:"shown"`
}

// deliverySettingsReport is the gitops settings output model. Groups and
// Settings are two views of Deployers and are always both present, whatever
// --group-by renders.
type deliverySettingsReport struct {
	Context     string                           `json:"context,omitempty"`
	Namespace   string                           `json:"namespace,omitempty"`
	Filters     *deliverySettingsFilters         `json:"filters,omitempty"`
	Complete    bool                             `json:"complete"`
	Counts      []deliveryKindCount              `json:"counts"`
	Deployers   []agent.DeliveryDeployerSettings `json:"deployers"`
	Groups      []agent.DeliverySettingsGroup    `json:"groups"`
	Settings    []agent.DeliverySettingsGroup    `json:"settings"`
	Reads       []agent.DeliverySettingsRead     `json:"reads"`
	LinkSources []agent.DeliveryLinkSource       `json:"linkSources,omitempty"`
	Notes       []string                         `json:"notes,omitempty"`
}

func parseDeliverySettingMatches(raw []string) ([]agent.DeliverySettingMatch, error) {
	matches := make([]agent.DeliverySettingMatch, 0, len(raw))
	for _, item := range raw {
		name, value, hasValue := strings.Cut(strings.TrimSpace(item), "=")
		if name == "" {
			return nil, fmt.Errorf("invalid --setting %q (want name or name=value)", item)
		}
		matches = append(matches, agent.DeliverySettingMatch{Name: name, Value: value, AnyValue: !hasValue})
	}
	return matches, nil
}

func deliverySettingsParamsFromFlags(cmd *cobra.Command) (deliverySettingsParams, error) {
	flags := cmd.Flags()
	params := deliverySettingsParams{}
	params.Namespace, _ = flags.GetString("namespace")
	params.Namespace = strings.TrimSpace(params.Namespace)
	params.Projects, _ = flags.GetStringSlice("project")
	params.Settings, _ = flags.GetStringArray("setting")
	groupBy, _ := flags.GetString("group-by")
	params.GroupBy = strings.ToLower(strings.TrimSpace(groupBy))
	switch params.GroupBy {
	case deliverySettingsGroupByProject, deliverySettingsGroupBySetting, deliverySettingsGroupByDeployer:
	default:
		return params, fmt.Errorf("invalid --group-by %q (valid: project, setting, deployer)", groupBy)
	}
	view, _ := flags.GetString("view")
	params.View = strings.ToLower(strings.TrimSpace(view))
	switch params.View {
	case deliverySettingsViewAll, deliverySettingsViewSummary, deliverySettingsViewGroups, deliverySettingsViewSettings, deliverySettingsViewDeployers:
	default:
		return params, fmt.Errorf("invalid --view %q (valid: all, summary, groups, settings, deployers)", view)
	}
	if params.View != deliverySettingsViewAll {
		format, _ := flags.GetString("format")
		legacyJSON, _ := flags.GetBool("json")
		if !legacyJSON && !strings.EqualFold(strings.TrimSpace(format), "json") {
			return params, fmt.Errorf("--view applies to JSON output; use --group-by for ascii and md")
		}
	}
	for _, project := range params.Projects {
		if strings.TrimSpace(project) == "" {
			return params, fmt.Errorf("--project must not be empty")
		}
	}
	if _, err := parseDeliverySettingMatches(params.Settings); err != nil {
		return params, err
	}
	return params, nil
}

func runGitOpsSettings(cmd *cobra.Command, args []string) error {
	params, err := deliverySettingsParamsFromFlags(cmd)
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	legacyJSON, _ := cmd.Flags().GetBool("json")
	format, err = normalizeGitOpsStatusFormat(format, legacyJSON)
	if err != nil {
		return err
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
	report, err := buildDeliverySettingsReport(ctx, client, params)
	if err != nil {
		return err
	}
	report.Context = boundContextLabel(ctx)
	if tui {
		return runGitOpsMarkdownTUI(ctx, renderDeliverySettingsMarkdown(report, params.GroupBy))
	}
	if format == "json" && params.View != deliverySettingsViewAll {
		return writeDeliverySettingsView(cmd.OutOrStdout(), report, params.View)
	}
	return writeDeliverySettings(cmd.OutOrStdout(), report, format, params.GroupBy)
}

// writeDeliverySettingsView writes the report with only one of its three
// views. The scope, counts, reads, link sources and notes are always kept:
// they are what says whether the view is complete.
func writeDeliverySettingsView(w io.Writer, report deliverySettingsReport, view string) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		return err
	}
	for _, key := range []string{deliverySettingsViewGroups, deliverySettingsViewSettings, deliverySettingsViewDeployers} {
		if key != view {
			delete(document, key)
		}
	}
	document["view"], _ = json.Marshal(view)
	encoder := json.NewEncoder(w)
	if view == deliverySettingsViewSummary {
		// The summary exists to be cheap to read, so it is written without
		// indentation: one line.
		if document[deliverySettingsViewSummary], err = json.Marshal(summariseDeliveryGroups(report.Groups)); err != nil {
			return err
		}
		return encoder.Encode(document)
	}
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

// deliverySummaryGroup is one group of the groups view with each deployer
// written once per setting as "namespace/name". It says the same thing as the
// groups view in a fraction of the bytes; links and per-setting spec paths are
// in the deployers view.
type deliverySummaryGroup struct {
	Controller string `json:"controller"`
	Kind       string `json:"kind"`
	GroupKind  string `json:"groupKind"`
	Group      string `json:"group,omitempty"`
	Deployers  int    `json:"deployers"`
	// Policies and Options each map a setting name to its values, and each
	// value to the deployers that have it. They are separate because a sync
	// option may be spelled like a policy ("prune" and "Prune=false").
	Policies map[string]map[string][]string `json:"policies"`
	Options  map[string]map[string][]string `json:"options,omitempty"`
	// Unset lists, per policy, the deployers that do not declare it and are
	// counted under the controller default in Policies.
	Unset map[string][]string `json:"unset,omitempty"`
	// PolicyDetails and OptionDetails carry what a value alone does not say,
	// such as how many ignore rules a deployer declares.
	PolicyDetails map[string]map[string]string `json:"policyDetails,omitempty"`
	OptionDetails map[string]map[string]string `json:"optionDetails,omitempty"`
}

func summariseDeliveryGroups(groups []agent.DeliverySettingsGroup) []deliverySummaryGroup {
	out := make([]deliverySummaryGroup, 0, len(groups))
	for _, group := range groups {
		summary := deliverySummaryGroup{
			Controller: group.Controller, Kind: group.Kind, GroupKind: group.GroupKind, Group: group.Group,
			Deployers: group.Deployers, Policies: map[string]map[string][]string{},
		}
		for _, setting := range group.Settings {
			policy := setting.Category == agent.DeliverySettingPolicy
			values := map[string][]string{}
			for _, value := range setting.Values {
				for _, ref := range value.Deployers {
					name := ref.Namespace + "/" + ref.Name
					values[value.Value] = append(values[value.Value], name)
					if ref.Unset {
						if summary.Unset == nil {
							summary.Unset = map[string][]string{}
						}
						summary.Unset[setting.Name] = append(summary.Unset[setting.Name], name)
					}
					// "auto-sync is off" only restates the n/a value.
					if ref.Detail == "" || value.Value == agent.DeliveryValueNotApplicable {
						continue
					}
					details := &summary.OptionDetails
					if policy {
						details = &summary.PolicyDetails
					}
					if *details == nil {
						*details = map[string]map[string]string{}
					}
					if (*details)[setting.Name] == nil {
						(*details)[setting.Name] = map[string]string{}
					}
					(*details)[setting.Name][name] = ref.Detail
				}
			}
			if policy {
				summary.Policies[setting.Name] = values
				continue
			}
			if summary.Options == nil {
				summary.Options = map[string]map[string][]string{}
			}
			summary.Options[setting.Name] = values
		}
		out = append(out, summary)
	}
	return out
}

func buildDeliverySettingsReport(ctx context.Context, client dynamic.Interface, params deliverySettingsParams) (deliverySettingsReport, error) {
	matches, err := parseDeliverySettingMatches(params.Settings)
	if err != nil {
		return deliverySettingsReport{}, err
	}
	inventory := agent.CollectDeliverySettings(ctx, client, agent.DeliverySettingsOptions{Namespace: params.Namespace})
	report := deliverySettingsReport{
		Namespace:   params.Namespace,
		Complete:    true,
		Deployers:   []agent.DeliveryDeployerSettings{},
		Reads:       inventory.Reads,
		LinkSources: inventory.LinkSources,
	}
	if len(params.Projects) > 0 || len(params.Settings) > 0 {
		report.Filters = &deliverySettingsFilters{Projects: params.Projects, Settings: params.Settings}
	}
	projects := map[string]bool{}
	for _, project := range params.Projects {
		projects[strings.TrimSpace(project)] = true
	}
	shown := map[string]int{}
	fluxLeftOutByProject := 0
	for _, deployer := range inventory.Deployers {
		if len(projects) > 0 {
			if deployer.GroupKind != agent.DeliveryGroupProject {
				fluxLeftOutByProject++
				continue
			}
			if !projects[deployer.Group] {
				continue
			}
		}
		matched := true
		for _, match := range matches {
			if !match.Matches(deployer) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		shown[deployer.Kind]++
		report.Deployers = append(report.Deployers, deployer)
	}
	for _, read := range inventory.Reads {
		report.Counts = append(report.Counts, deliveryKindCount{
			Controller: read.Controller, Kind: read.Kind, Status: read.Status, Read: read.Count, Shown: shown[read.Kind],
		})
		if read.Status == agent.DeliveryReadNotRead {
			report.Complete = false
		}
	}
	report.Groups = agent.GroupDeliverySettings(report.Deployers, true)
	report.Settings = agent.GroupDeliverySettings(report.Deployers, false)
	if fluxLeftOutByProject > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf(
			"--project selects Argo CD Applications only; %d Flux object(s) have no project and are left out.", fluxLeftOutByProject))
	}
	return report, nil
}

func writeDeliverySettings(w io.Writer, report deliverySettingsReport, format, groupBy string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "md":
		_, err := io.WriteString(w, renderDeliverySettingsMarkdown(report, groupBy))
		return err
	default:
		_, err := io.WriteString(w, renderDeliverySettingsASCII(report, groupBy))
		return err
	}
}

// deliveryText makes a string read from the cluster safe to print as one
// piece of a line. A project name, sync option or ApplicationSet name is
// free text: without this a newline in one could forge a "Reads" line, and an
// escape sequence could rewrite the terminal. JSON output is left verbatim.
func deliveryText(text string) string {
	return strings.Map(func(r rune) rune {
		// Control characters, and the characters that change how the rest
		// of a line is displayed without being seen: bidirectional
		// overrides and other format characters, and the line and paragraph
		// separators.
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, text)
}

// deliveryMarkdownURL keeps a URL inside the parentheses of a Markdown link.
func deliveryMarkdownURL(raw string) string {
	return strings.NewReplacer("(", "%28", ")", "%29", " ", "%20", "<", "%3C", ">", "%3E", "|", "%7C").Replace(deliveryText(raw))
}

func deliveryControllerTitle(controller string) string {
	if controller == agent.DeliveryControllerArgoCD {
		return "Argo CD"
	}
	return controller
}

func deliveryKindPlural(kind string, n int) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", kind)
	}
	return fmt.Sprintf("%d %ss", n, kind)
}

// deliveryGroupTitle names a group: "Argo CD Application, project payments (7)".
func deliveryGroupTitle(group agent.DeliverySettingsGroup) string {
	scope := ""
	switch group.GroupKind {
	case agent.DeliveryGroupProject:
		scope = ", project " + deliveryText(group.Group)
		if group.Group == "" {
			scope = ", project (unset)"
		}
	case agent.DeliveryGroupNamespace:
		scope = ", namespace " + deliveryText(group.Group)
	}
	return fmt.Sprintf("%s %s%s (%d)", deliveryControllerTitle(group.Controller), group.Kind, scope, group.Deployers)
}

func deliverySettingLabel(name, value string) string {
	name, value = deliveryText(name), deliveryText(value)
	switch {
	case value == "" || value == agent.DeliveryValueSet:
		return name
	default:
		return name + "=" + value
	}
}

func deliveryRefLabel(ref agent.DeliveryDeployerRef) string {
	var notes []string
	if ref.Unset {
		notes = append(notes, "unset")
	}
	if ref.Detail != "" {
		notes = append(notes, deliveryText(ref.Detail))
	}
	label := deliveryText(ref.Namespace + "/" + ref.Name)
	if len(notes) > 0 {
		label += " (" + strings.Join(notes, "; ") + ")"
	}
	return label
}

func deliveryRefLabels(refs []agent.DeliveryDeployerRef, markdown bool) string {
	labels := make([]string, 0, len(refs))
	for _, ref := range refs {
		label := deliveryRefLabel(ref)
		if markdown {
			label = escapeDeliveryMarkdown(label)
			if ref.URL != "" {
				name := escapeDeliveryMarkdown(deliveryText(ref.Namespace + "/" + ref.Name))
				label = "[" + name + "](" + deliveryMarkdownURL(ref.URL) + ")" + strings.TrimPrefix(label, name)
			}
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func escapeDeliveryMarkdown(text string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(text)
}

// deliveryValueCount is "6" or "3 (2 unset, default)".
func deliveryValueCount(value agent.DeliverySettingValue) string {
	if value.Unset > 0 {
		return fmt.Sprintf("%d (%d unset, controller default)", value.Count, value.Unset)
	}
	return fmt.Sprintf("%d", value.Count)
}

// deliveryLinkPatterns lists the Argo CD URL each namespace's Applications
// link under, for views that name deployers without a link each.
func deliveryLinkPatterns(report deliverySettingsReport) []string {
	var lines []string
	for _, link := range report.LinkSources {
		link.Namespace, link.URL = deliveryText(link.Namespace), deliveryText(link.URL)
		switch link.Status {
		case agent.DeliveryLinkFound:
			lines = append(lines, fmt.Sprintf("%s: %s/applications/%s/<name>", link.Namespace, link.URL, link.Namespace))
		case agent.DeliveryLinkURLUnset:
			lines = append(lines, fmt.Sprintf("%s: argocd-cm declares no url; no links", link.Namespace))
		case agent.DeliveryLinkInvalid:
			lines = append(lines, fmt.Sprintf("%s: the url in argocd-cm is not an http(s) URL; no links", link.Namespace))
		case agent.DeliveryLinkNotFound:
			lines = append(lines, fmt.Sprintf("%s: no argocd-cm in this namespace; no links", link.Namespace))
		default:
			lines = append(lines, fmt.Sprintf("%s: argocd-cm not read (%s); no links", link.Namespace, link.Reason))
		}
	}
	return lines
}

func deliveryReadLine(read agent.DeliverySettingsRead) string {
	switch read.Status {
	case agent.DeliveryReadRead:
		return fmt.Sprintf("%s: read, %d", read.Resource, read.Count)
	case agent.DeliveryReadNotInstalled:
		return fmt.Sprintf("%s: not installed", read.Resource)
	default:
		return fmt.Sprintf("%s: NOT READ (%s); %ss are not known to be absent", read.Resource, read.Reason, read.Kind)
	}
}

func deliveryCountsLine(report deliverySettingsReport) string {
	parts := make([]string, 0, len(report.Counts))
	filtered := report.Filters != nil
	for _, count := range report.Counts {
		// A kind that was not read has no count. "0 Applications" would say
		// there are none.
		switch count.Status {
		case agent.DeliveryReadNotRead:
			parts = append(parts, count.Kind+"s NOT READ")
			continue
		case agent.DeliveryReadNotInstalled:
			parts = append(parts, count.Kind+"s not installed")
			continue
		}
		if filtered {
			parts = append(parts, fmt.Sprintf("%d of %s", count.Shown, deliveryKindPlural(count.Kind, count.Read)))
			continue
		}
		parts = append(parts, deliveryKindPlural(count.Kind, count.Read))
	}
	return strings.Join(parts, ", ")
}

func deliveryScopeLine(report deliverySettingsReport) string {
	namespace := report.Namespace
	if namespace == "" {
		namespace = "all"
	}
	line := "Namespace: " + namespace
	if report.Context != "" {
		line = "Context: " + report.Context + "   " + line
	}
	return line
}

func deliveryFilterLine(report deliverySettingsReport) string {
	if report.Filters == nil {
		return ""
	}
	var parts []string
	if len(report.Filters.Projects) > 0 {
		parts = append(parts, "project "+strings.Join(report.Filters.Projects, ", "))
	}
	if len(report.Filters.Settings) > 0 {
		parts = append(parts, "setting "+strings.Join(report.Filters.Settings, ", "))
	}
	return "Filter: " + strings.Join(parts, "; ")
}

func deliveryDeployerSummary(deployer agent.DeliveryDeployerSettings) (policies, options []string) {
	for _, setting := range deployer.Settings {
		if setting.Category == agent.DeliverySettingPolicy {
			text := deliveryText(setting.Name + " " + setting.Value)
			if setting.Value == agent.DeliveryValueUnset && setting.Default != "" {
				text += " (controller default " + setting.Default + ")"
			}
			policies = append(policies, text)
			continue
		}
		text := deliverySettingLabel(setting.Name, setting.Value)
		if setting.Detail != "" {
			text += " (" + deliveryText(setting.Detail) + ")"
		}
		options = append(options, text)
	}
	return policies, options
}

func deliveryGroupsFor(report deliverySettingsReport, groupBy string) []agent.DeliverySettingsGroup {
	if groupBy == deliverySettingsGroupBySetting {
		return report.Settings
	}
	return report.Groups
}

func renderDeliverySettingsASCII(report deliverySettingsReport, groupBy string) string {
	var b strings.Builder
	b.WriteString("DELIVERY SETTINGS\n")
	b.WriteString(strings.Repeat("═", 68) + "\n")
	b.WriteString(deliveryScopeLine(report) + "\n")
	b.WriteString("Read: " + deliveryCountsLine(report) + "\n")
	if line := deliveryFilterLine(report); line != "" {
		b.WriteString(line + "\n")
	}
	if !report.Complete {
		b.WriteString("INCOMPLETE: at least one kind could not be read; see Reads.\n")
	}
	if len(report.Deployers) == 0 {
		b.WriteString("\nNo deployers to show.\n")
	}

	if groupBy == deliverySettingsGroupByDeployer {
		for _, deployer := range report.Deployers {
			scope := "namespace " + deliveryText(deployer.Group)
			if deployer.GroupKind == agent.DeliveryGroupProject {
				scope = "project " + deliveryText(deployer.Group)
				if deployer.Group == "" {
					scope = "project (unset)"
				}
			}
			fmt.Fprintf(&b, "\n%s %s %s, %s\n", deliveryControllerTitle(deployer.Controller), deployer.Kind,
				deliveryText(deployer.Namespace+"/"+deployer.Name), scope)
			if deployer.URL != "" {
				fmt.Fprintf(&b, "  %s\n", deliveryText(deployer.URL))
			}
			if deployer.GeneratedBy != "" {
				fmt.Fprintf(&b, "  generated by %s\n", deliveryText(deployer.GeneratedBy))
			}
			policies, options := deliveryDeployerSummary(deployer)
			fmt.Fprintf(&b, "  %s\n", strings.Join(policies, "; "))
			if len(options) > 0 {
				fmt.Fprintf(&b, "  options: %s\n", strings.Join(options, ", "))
			}
		}
	} else {
		for _, group := range deliveryGroupsFor(report, groupBy) {
			fmt.Fprintf(&b, "\n%s\n", deliveryGroupTitle(group))
			optionsHeader := false
			for _, setting := range group.Settings {
				if setting.Category == agent.DeliverySettingPolicy {
					fmt.Fprintf(&b, "  %s\n", deliveryText(setting.Name))
					for _, value := range setting.Values {
						fmt.Fprintf(&b, "    %-5s %s  %s\n", deliveryText(value.Value), deliveryValueCount(value), deliveryRefLabels(value.Deployers, false))
					}
					continue
				}
				if !optionsHeader {
					b.WriteString("  options\n")
					optionsHeader = true
				}
				for _, value := range setting.Values {
					fmt.Fprintf(&b, "    %s  %s  %s\n", deliverySettingLabel(setting.Name, value.Value),
						deliveryValueCount(value), deliveryRefLabels(value.Deployers, false))
				}
			}
		}
	}

	if links := deliveryLinkPatterns(report); len(links) > 0 {
		b.WriteString("\nArgo CD UI links, by Application namespace\n")
		for _, line := range links {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\nReads\n")
	for _, read := range report.Reads {
		b.WriteString("  " + deliveryReadLine(read) + "\n")
	}
	for _, note := range report.Notes {
		b.WriteString("\nNote: " + note + "\n")
	}
	return b.String()
}

func renderDeliverySettingsMarkdown(report deliverySettingsReport, groupBy string) string {
	var b strings.Builder
	b.WriteString("# Delivery settings\n\n")
	b.WriteString("- " + escapeDeliveryMarkdown(deliveryScopeLine(report)) + "\n")
	b.WriteString("- Read: " + deliveryCountsLine(report) + "\n")
	if line := deliveryFilterLine(report); line != "" {
		b.WriteString("- " + escapeDeliveryMarkdown(line) + "\n")
	}
	if !report.Complete {
		b.WriteString("- **Incomplete:** at least one kind could not be read; see Reads.\n")
	}
	if len(report.Deployers) == 0 {
		b.WriteString("\nNo deployers to show.\n")
	}

	if groupBy == deliverySettingsGroupByDeployer {
		if len(report.Deployers) > 0 {
			b.WriteString("\n| Deployer | Group | Settings | Options |\n|---|---|---|---|\n")
		}
		for _, deployer := range report.Deployers {
			name := escapeDeliveryMarkdown(deliveryText(deployer.Namespace + "/" + deployer.Name))
			if deployer.URL != "" {
				name = "[" + name + "](" + deliveryMarkdownURL(deployer.URL) + ")"
			}
			policies, options := deliveryDeployerSummary(deployer)
			group := deliveryText(deployer.GroupKind + " " + deployer.Group)
			if deployer.GeneratedBy != "" {
				group += "; generated by " + deliveryText(deployer.GeneratedBy)
			}
			fmt.Fprintf(&b, "| %s %s %s | %s | %s | %s |\n", deliveryControllerTitle(deployer.Controller), deployer.Kind, name,
				escapeDeliveryMarkdown(group), escapeDeliveryMarkdown(strings.Join(policies, "; ")),
				escapeDeliveryMarkdown(strings.Join(options, ", ")))
		}
	} else {
		for _, group := range deliveryGroupsFor(report, groupBy) {
			fmt.Fprintf(&b, "\n## %s\n\n| Setting | Count | Deployers |\n|---|---|---|\n", escapeDeliveryMarkdown(deliveryGroupTitle(group)))
			for _, setting := range group.Settings {
				for _, value := range setting.Values {
					label := deliveryText(setting.Name + " " + value.Value)
					if setting.Category == agent.DeliverySettingOption {
						label = deliverySettingLabel(setting.Name, value.Value)
					}
					fmt.Fprintf(&b, "| %s | %s | %s |\n", escapeDeliveryMarkdown(label), deliveryValueCount(value),
						deliveryRefLabels(value.Deployers, true))
				}
			}
		}
	}

	if links := deliveryLinkPatterns(report); len(links) > 0 {
		b.WriteString("\n## Argo CD UI links, by Application namespace\n\n")
		for _, line := range links {
			b.WriteString("- " + escapeDeliveryMarkdown(line) + "\n")
		}
	}
	b.WriteString("\n## Reads\n\n")
	for _, read := range report.Reads {
		b.WriteString("- " + escapeDeliveryMarkdown(deliveryReadLine(read)) + "\n")
	}
	if len(report.Notes) > 0 {
		notes := append([]string{}, report.Notes...)
		sort.Strings(notes)
		b.WriteString("\n## Notes\n\n")
		for _, note := range notes {
			b.WriteString("- " + escapeDeliveryMarkdown(note) + "\n")
		}
	}
	return b.String()
}
