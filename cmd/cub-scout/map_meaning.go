// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

var (
	mapMeaningFormat     string
	mapMeaningMaxGroups  int
	mapMeaningMaxMembers int
)

type meaningGroup struct {
	Key              string     `json:"key"`
	Label            string     `json:"label"`
	Evidence         []string   `json:"evidence,omitempty"`
	DistinctiveTerms []string   `json:"distinctiveTerms,omitempty"`
	Members          []MapEntry `json:"members"`
}

type meaningReport struct {
	Groups []meaningGroup `json:"groups"`
	Total  int            `json:"total"`
}

var mapMeaningCmd = &cobra.Command{
	Use:   "meaning",
	Short: "Experimental meaning-first browse groups",
	Long: `Experimental meaning-first browse mode.

This command groups resources using a deterministic hybrid model:
- semantic signals (name + labels)
- structural signals (owner + namespace + kind)

Each group includes comparative labels and evidence lines.
No cluster mutation is performed.`,
	RunE: runMapMeaning,
}

func init() {
	mapCmd.AddCommand(mapMeaningCmd)
	mapMeaningCmd.Flags().StringVar(&mapNamespace, "namespace", "", "Filter by namespace")
	mapMeaningCmd.Flags().StringVar(&mapOwner, "owner", "", "Filter by owner")
	mapMeaningCmd.Flags().StringVar(&mapKind, "kind", "", "Filter by resource kind")
	mapMeaningCmd.Flags().StringVar(&mapMeaningFormat, "format", "ascii", "Output format: ascii, json")
	mapMeaningCmd.Flags().IntVar(&mapMeaningMaxGroups, "max-groups", 12, "Maximum groups to display")
	mapMeaningCmd.Flags().IntVar(&mapMeaningMaxMembers, "max-members", 10, "Maximum members shown per group")
	_ = mapMeaningCmd.RegisterFlagCompletionFunc("namespace", completeNamespaces)
	_ = mapMeaningCmd.RegisterFlagCompletionFunc("owner", completeOwners)
	_ = mapMeaningCmd.RegisterFlagCompletionFunc("kind", completeKinds)
}

func runMapMeaning(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if mapOwner != "" {
		if err := ValidateOwner(mapOwner); err != nil {
			return err
		}
		mapOwner = NormalizeOwner(mapOwner)
	}

	var entries []MapEntry
	if entriesJSON := strings.TrimSpace(os.Getenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON")); entriesJSON != "" {
		loaded, err := loadMapEntriesFromJSON(entriesJSON)
		if err != nil {
			return err
		}
		entries = loaded
	} else {
		loaded, err := collectMapEntriesForMeaning(ctx)
		if err != nil {
			return err
		}
		entries = loaded
	}

	entries = filterMeaningEntries(entries, mapNamespace, mapOwner, mapKind)
	groups := buildMeaningGroups(entries)

	if len(groups) > mapMeaningMaxGroups && mapMeaningMaxGroups > 0 {
		groups = groups[:mapMeaningMaxGroups]
	}
	if mapMeaningMaxMembers > 0 {
		for i := range groups {
			if len(groups[i].Members) > mapMeaningMaxMembers {
				groups[i].Members = groups[i].Members[:mapMeaningMaxMembers]
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(mapMeaningFormat)) {
	case "json":
		return json.NewEncoder(cmd.OutOrStdout()).Encode(meaningReport{
			Groups: groups,
			Total:  len(entries),
		})
	case "ascii", "":
		renderMeaningASCII(cmd, groups, len(entries))
		return nil
	default:
		return fmt.Errorf("unsupported format %q (valid: ascii, json)", mapMeaningFormat)
	}
}

func collectMapEntriesForMeaning(ctx context.Context) ([]MapEntry, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, withKubeRecoveryHint(fmt.Errorf("build kubernetes config: %w", err), "cub-scout map meaning")
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, withKubeRecoveryHint(fmt.Errorf("create dynamic client: %w", err), "cub-scout map meaning")
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	entries := []MapEntry{}
	byOwner := map[string]int{}
	appSetLookup := loadMapApplicationSetLookup(ctx, dynClient, mapNamespace)
	resources := collectMapResourceList()

	for _, gvr := range resources {
		if mapNamespace != "" {
			l, err := dynClient.Resource(gvr).Namespace(mapNamespace).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				entries = processResourceWithLookup(&item, gvr, clusterName, entries, byOwner, appSetLookup)
			}
			continue
		}

		l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range l.Items {
			entries = processResourceWithLookup(&item, gvr, clusterName, entries, byOwner, appSetLookup)
		}
	}

	return entries, nil
}

func filterMeaningEntries(entries []MapEntry, namespace, owner, kind string) []MapEntry {
	out := make([]MapEntry, 0, len(entries))
	for _, e := range entries {
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		if owner != "" && !strings.EqualFold(e.Owner, owner) {
			continue
		}
		if kind != "" && !strings.EqualFold(e.Kind, kind) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func buildMeaningGroups(entries []MapEntry) []meaningGroup {
	type tokenBag struct {
		groupKey  string
		entry     MapEntry
		allTokens []string
		uniq      map[string]struct{}
	}

	bags := make([]tokenBag, 0, len(entries))
	globalTokenFreq := map[string]int{}
	groupTokenFreq := map[string]map[string]int{}

	for _, e := range entries {
		tokens := meaningTokens(e)
		key := meaningGroupKey(e, tokens)
		if key == "" {
			key = "native|unscoped|misc"
		}
		uniq := map[string]struct{}{}
		for _, token := range tokens {
			if token == "" {
				continue
			}
			uniq[token] = struct{}{}
		}
		for token := range uniq {
			globalTokenFreq[token]++
		}
		if _, ok := groupTokenFreq[key]; !ok {
			groupTokenFreq[key] = map[string]int{}
		}
		for token := range uniq {
			groupTokenFreq[key][token]++
		}
		bags = append(bags, tokenBag{
			groupKey:  key,
			entry:     e,
			allTokens: tokens,
			uniq:      uniq,
		})
	}

	groupMap := map[string]*meaningGroup{}
	for _, bag := range bags {
		group := groupMap[bag.groupKey]
		if group == nil {
			group = &meaningGroup{Key: bag.groupKey}
			groupMap[bag.groupKey] = group
		}
		group.Members = append(group.Members, bag.entry)
	}

	for _, group := range groupMap {
		sort.Slice(group.Members, func(i, j int) bool {
			if group.Members[i].Namespace != group.Members[j].Namespace {
				return group.Members[i].Namespace < group.Members[j].Namespace
			}
			if group.Members[i].Kind != group.Members[j].Kind {
				return group.Members[i].Kind < group.Members[j].Kind
			}
			return group.Members[i].Name < group.Members[j].Name
		})

		distinctive := pickDistinctiveTokens(group.Key, groupTokenFreq[group.Key], globalTokenFreq)
		group.DistinctiveTerms = distinctive
		group.Label = buildMeaningLabel(group, distinctive)
		group.Evidence = buildMeaningEvidence(group, distinctive)
	}

	groups := make([]meaningGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].Members) != len(groups[j].Members) {
			return len(groups[i].Members) > len(groups[j].Members)
		}
		return groups[i].Label < groups[j].Label
	})

	return groups
}

func meaningTokens(e MapEntry) []string {
	parts := []string{
		e.Kind,
		e.Namespace,
		e.Owner,
		e.Name,
		e.Labels["app"],
		e.Labels["component"],
		e.Labels["tier"],
		e.Labels["team"],
	}

	stop := map[string]struct{}{
		"deployment": {}, "statefulset": {}, "daemonset": {}, "service": {}, "configmap": {},
		"default": {}, "prod": {}, "staging": {}, "native": {},
	}

	out := []string{}
	for _, p := range parts {
		for _, token := range tokenizeLower(p) {
			if len(token) < 2 {
				continue
			}
			if _, skip := stop[token]; skip {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

func tokenizeLower(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ToLower(s)
	replacer := strings.NewReplacer("/", " ", "-", " ", "_", " ", ".", " ", ":", " ")
	s = replacer.Replace(s)
	return strings.Fields(s)
}

func meaningGroupKey(e MapEntry, tokens []string) string {
	owner := strings.ToLower(strings.TrimSpace(e.Owner))
	if owner == "" {
		owner = "native"
	}
	ns := strings.ToLower(strings.TrimSpace(e.Namespace))
	if ns == "" {
		ns = "cluster"
	}
	kind := strings.ToLower(strings.TrimSpace(e.Kind))
	if kind == "" {
		kind = "resource"
	}
	semantic := kind
	for _, token := range tokens {
		if token == owner || token == ns || token == kind {
			continue
		}
		semantic = token
		break
	}
	return owner + "|" + ns + "|" + semantic
}

func pickDistinctiveTokens(groupKey string, groupFreq, globalFreq map[string]int) []string {
	type scored struct {
		token string
		score int
	}
	scores := make([]scored, 0, len(groupFreq))
	for token, inGroup := range groupFreq {
		score := (inGroup * 2) - globalFreq[token]
		if score <= 0 {
			continue
		}
		scores = append(scores, scored{token: token, score: score})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return scores[i].token < scores[j].token
	})
	limit := 3
	if len(scores) < limit {
		limit = len(scores)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scores[i].token)
	}
	return out
}

func buildMeaningLabel(group *meaningGroup, distinctive []string) string {
	parts := strings.Split(group.Key, "|")
	owner, ns, semantic := "native", "cluster", "resource"
	if len(parts) >= 3 {
		owner, ns, semantic = parts[0], parts[1], parts[2]
	}

	if len(distinctive) > 0 {
		return strings.Join(distinctive, " · ")
	}
	return fmt.Sprintf("%s · %s · %s", semantic, owner, ns)
}

func buildMeaningEvidence(group *meaningGroup, distinctive []string) []string {
	parts := strings.Split(group.Key, "|")
	owner, ns := "native", "cluster"
	if len(parts) >= 2 {
		owner, ns = parts[0], parts[1]
	}

	evidence := []string{
		"owner=" + owner,
		"namespace=" + ns,
	}
	if len(distinctive) > 0 {
		evidence = append(evidence, "distinctive="+strings.Join(distinctive, ","))
	}
	return evidence
}

func renderMeaningASCII(cmd *cobra.Command, groups []meaningGroup, total int) {
	fmt.Fprintln(cmd.OutOrStdout(), "Meaning Groups (experimental)")
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 60))
	fmt.Fprintf(cmd.OutOrStdout(), "Resources: %d, Groups: %d\n\n", total, len(groups))

	for i, group := range groups {
		fmt.Fprintf(cmd.OutOrStdout(), "%d. %s (%d)\n", i+1, group.Label, len(group.Members))
		if len(group.Evidence) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "   evidence: %s\n", strings.Join(group.Evidence, " | "))
		}
		for _, m := range group.Members {
			fmt.Fprintf(cmd.OutOrStdout(), "   - %s/%s in %s  owner=%s\n", m.Kind, m.Name, m.Namespace, m.Owner)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
}
