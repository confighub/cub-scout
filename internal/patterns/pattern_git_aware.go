package patterns

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func init() {
	// ApplicationSet generators pattern (Hybrid)
	Register(Pattern{
		ID:          "gitops.argocd.applicationset_generators",
		Name:        "ApplicationSet Generator Summary",
		Description: "Summarizes ApplicationSet generators. Runs without --git-root with reduced evidence; enriched when --git-root is provided.",
		Category:    "gitops",
		PatternType: "hybrid",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"ApplicationSet"}},
		},
		DetectWithGit: detectApplicationSetGenerators,
	})

	// Flux Kustomization paths pattern (Hybrid)
	Register(Pattern{
		ID:          "gitops.flux.kustomization_paths",
		Name:        "Flux Kustomization Paths",
		Description: "Correlates Flux Kustomization paths with Git repository structure. Runs without --git-root with reduced evidence; enriched when --git-root is provided.",
		Category:    "gitops",
		PatternType: "hybrid",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Kustomization"}},
		},
		DetectWithGit: detectFluxKustomizationPaths,
	})
}

// detectApplicationSetGenerators analyzes ApplicationSet generators.
// When git context is available, scans repo for ApplicationSet YAMLs and extracts generator types.
func detectApplicationSetGenerators(ctx *DetectContext) ([]Finding, Status) {
	var findings []Finding

	// Count ApplicationSets in graph
	appSetCount := 0
	for _, node := range ctx.Graph.Nodes {
		if node.Kind == "ApplicationSet" && node.APIVersion == "argoproj.io/v1alpha1" {
			appSetCount++
		}
	}

	// Mode 1: Without git context (reduced evidence)
	if ctx.Git == nil || !ctx.Git.Valid {
		findings = append(findings, Finding{
			Pattern:  "gitops.argocd.applicationset_generators",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("ApplicationSets detected: %d (use --git-root for generator details)", appSetCount),
		})
		return findings, StatusPass
	}

	// Mode 2: With valid git context (enriched)
	generatorCounts := make(map[string]int)
	var refs []string

	files := ctx.Git.Files()
	for _, file := range files {
		if !isYAMLFile(file) {
			continue
		}

		content := ctx.Git.ReadFile(file)
		if content == nil {
			continue
		}

		// Parse YAML documents
		docs := parseYAMLDocuments(content)
		for _, doc := range docs {
			if !isApplicationSet(doc) {
				continue
			}

			// Extract generator types
			generators := extractGeneratorTypes(doc)
			for _, gen := range generators {
				generatorCounts[gen]++
			}

			// Add repo-relative ref (bounded to first 10)
			if len(refs) < 10 {
				refs = append(refs, fmt.Sprintf("git:path:%s", file))
			}
		}
	}

	// Build generator summary
	if len(generatorCounts) > 0 {
		var parts []string
		// Sort for determinism
		var genTypes []string
		for gen := range generatorCounts {
			genTypes = append(genTypes, gen)
		}
		sort.Strings(genTypes)

		for _, gen := range genTypes {
			parts = append(parts, fmt.Sprintf("%s=%d", gen, generatorCounts[gen]))
		}

		sort.Strings(refs)
		findings = append(findings, Finding{
			Pattern:  "gitops.argocd.applicationset_generators",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("ApplicationSet generators: %s", strings.Join(parts, " ")),
			Refs:     refs,
		})
	}

	// Also report graph count
	findings = append(findings, Finding{
		Pattern:  "gitops.argocd.applicationset_generators",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("ApplicationSets in cluster: %d", appSetCount),
	})

	return findings, StatusPass
}

// detectFluxKustomizationPaths analyzes Flux Kustomization spec.path values.
// When git context is available, validates paths exist in repo.
func detectFluxKustomizationPaths(ctx *DetectContext) ([]Finding, Status) {
	var findings []Finding

	// Count Kustomizations in graph
	kustomizationCount := 0
	for _, node := range ctx.Graph.Nodes {
		if node.Kind == "Kustomization" && node.APIVersion == "kustomize.toolkit.fluxcd.io/v1" {
			kustomizationCount++
		}
	}

	// Mode 1: Without git context (reduced evidence)
	if ctx.Git == nil || !ctx.Git.Valid {
		findings = append(findings, Finding{
			Pattern:  "gitops.flux.kustomization_paths",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Flux Kustomizations detected: %d (use --git-root for path validation)", kustomizationCount),
		})
		return findings, StatusPass
	}

	// Mode 2: With valid git context (enriched)
	pathCounts := make(map[string]int)
	var refs []string

	files := ctx.Git.Files()
	for _, file := range files {
		if !isYAMLFile(file) {
			continue
		}

		content := ctx.Git.ReadFile(file)
		if content == nil {
			continue
		}

		// Parse YAML documents
		docs := parseYAMLDocuments(content)
		for _, doc := range docs {
			if !isFluxKustomization(doc) {
				continue
			}

			// Extract spec.path
			path := extractSpecPath(doc)
			if path != "" {
				pathCounts[path]++
			}

			// Add repo-relative ref (bounded to first 10)
			if len(refs) < 10 {
				refs = append(refs, fmt.Sprintf("git:path:%s", file))
			}
		}
	}

	// Build path summary
	if len(pathCounts) > 0 {
		var paths []string
		for path := range pathCounts {
			paths = append(paths, path)
		}
		sort.Strings(paths)

		sort.Strings(refs)
		findings = append(findings, Finding{
			Pattern:  "gitops.flux.kustomization_paths",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Kustomization paths: %s", strings.Join(paths, ", ")),
			Refs:     refs,
		})
	}

	// Report cluster count
	findings = append(findings, Finding{
		Pattern:  "gitops.flux.kustomization_paths",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Flux Kustomizations in cluster: %d", kustomizationCount),
	})

	return findings, StatusPass
}

// isYAMLFile checks if a file has a YAML extension.
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// parseYAMLDocuments parses multi-document YAML.
func parseYAMLDocuments(content []byte) []map[string]interface{} {
	var docs []map[string]interface{}

	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			break
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}

	return docs
}

// isApplicationSet checks if a YAML document is an ApplicationSet.
func isApplicationSet(doc map[string]interface{}) bool {
	apiVersion, _ := doc["apiVersion"].(string)
	kind, _ := doc["kind"].(string)
	return apiVersion == "argoproj.io/v1alpha1" && kind == "ApplicationSet"
}

// isFluxKustomization checks if a YAML document is a Flux Kustomization.
func isFluxKustomization(doc map[string]interface{}) bool {
	apiVersion, _ := doc["apiVersion"].(string)
	kind, _ := doc["kind"].(string)
	return apiVersion == "kustomize.toolkit.fluxcd.io/v1" && kind == "Kustomization"
}

// extractGeneratorTypes extracts generator types from an ApplicationSet spec.
func extractGeneratorTypes(doc map[string]interface{}) []string {
	var types []string

	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return types
	}

	generators, ok := spec["generators"].([]interface{})
	if !ok {
		return types
	}

	for _, gen := range generators {
		genMap, ok := gen.(map[string]interface{})
		if !ok {
			continue
		}

		// Each generator is a map with one key being the type
		for key := range genMap {
			types = append(types, key)
		}
	}

	return types
}

// extractSpecPath extracts spec.path from a Kustomization document.
func extractSpecPath(doc map[string]interface{}) string {
	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return ""
	}

	path, _ := spec["path"].(string)
	return path
}
