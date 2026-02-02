package patterns

import (
	"fmt"

	"github.com/confighub/cub-scout/internal/graph"
)

func init() {
	Register(Pattern{
		ID:          "gitops.controller_presence",
		Name:        "GitOps Controller Presence",
		Description: "Checks for the presence of GitOps controller CRDs (Argo CD Applications/ApplicationSets, Flux Kustomizations/HelmReleases). Detects which GitOps tools are in use.",
		Category:    "gitops",
		Detect:      detectGitOpsControllerPresence,
	})
}

// GitOps CRD kinds to check for
var argoCDKinds = []string{"Application", "ApplicationSet"}
var fluxKinds = []string{"Kustomization", "HelmRelease", "GitRepository"}

// detectGitOpsControllerPresence checks for GitOps controller CRDs in the graph.
func detectGitOpsControllerPresence(g *graph.Graph) ([]Finding, Status) {
	var findings []Finding

	// Count CRDs by kind
	argoApps := 0
	argoAppSets := 0
	fluxKustomizations := 0
	fluxHelmReleases := 0
	fluxGitRepos := 0

	for _, node := range g.Nodes {
		switch node.Kind {
		case "Application":
			if node.APIVersion == "argoproj.io/v1alpha1" {
				argoApps++
			}
		case "ApplicationSet":
			if node.APIVersion == "argoproj.io/v1alpha1" {
				argoAppSets++
			}
		case "Kustomization":
			if node.APIVersion == "kustomize.toolkit.fluxcd.io/v1" {
				fluxKustomizations++
			}
		case "HelmRelease":
			if node.APIVersion == "helm.toolkit.fluxcd.io/v2" {
				fluxHelmReleases++
			}
		case "GitRepository":
			if node.APIVersion == "source.toolkit.fluxcd.io/v1" {
				fluxGitRepos++
			}
		}
	}

	hasArgoCD := argoApps > 0 || argoAppSets > 0
	hasFlux := fluxKustomizations > 0 || fluxHelmReleases > 0 || fluxGitRepos > 0

	// Report findings
	if hasArgoCD {
		evidence := []string{}
		if argoApps > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Application(s)", argoApps))
		}
		if argoAppSets > 0 {
			evidence = append(evidence, fmt.Sprintf("%d ApplicationSet(s)", argoAppSets))
		}

		findings = append(findings, Finding{
			Pattern:  "gitops.controller_presence",
			Severity: SeverityInfo,
			Message:  "Argo CD controller detected",
			Evidence: evidence,
		})
	}

	if hasFlux {
		evidence := []string{}
		if fluxKustomizations > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Kustomization(s)", fluxKustomizations))
		}
		if fluxHelmReleases > 0 {
			evidence = append(evidence, fmt.Sprintf("%d HelmRelease(s)", fluxHelmReleases))
		}
		if fluxGitRepos > 0 {
			evidence = append(evidence, fmt.Sprintf("%d GitRepository(s)", fluxGitRepos))
		}

		findings = append(findings, Finding{
			Pattern:  "gitops.controller_presence",
			Severity: SeverityInfo,
			Message:  "Flux controller detected",
			Evidence: evidence,
		})
	}

	if !hasArgoCD && !hasFlux {
		confidenceNoGitOps := 0.8
		findings = append(findings, Finding{
			Pattern:    "gitops.controller_presence",
			Severity:   SeverityWarning,
			Message:    "No GitOps controllers detected",
			Confidence: &confidenceNoGitOps,
			Evidence: []string{
				"No Argo CD Applications or ApplicationSets found",
				"No Flux Kustomizations, HelmReleases, or GitRepositories found",
				"This may indicate GitOps is not deployed, or CRDs are in namespaces not collected",
			},
			Remediation: &Remediation{
				Summary: "Install a GitOps controller (Argo CD or Flux) to manage cluster resources declaratively.",
				Steps: []string{
					"Choose a GitOps tool: Argo CD for application-centric workflows, Flux for Git-native automation.",
					"Install the controller following the official documentation.",
					"Create Application or Kustomization resources to manage your workloads.",
				},
				Links: []string{
					"https://argo-cd.readthedocs.io/en/stable/getting_started/",
					"https://fluxcd.io/flux/get-started/",
				},
			},
		})
		return findings, StatusFail
	}

	return findings, StatusPass
}
