package patterns

import (
	"fmt"

	"github.com/confighub/cub-scout/internal/graph"
)

func init() {
	Register(Pattern{
		ID:          "k8s.ownership_chain_complete",
		Name:        "Kubernetes Ownership Chain Complete",
		Description: "Checks that Deployment → ReplicaSet → Pod ownership chains are complete. Incomplete chains may indicate orphaned resources or missing links.",
		Category:    "k8s",
		Detect:      detectOwnershipChainComplete,
	})
}

// detectOwnershipChainComplete checks for complete ownership chains.
// A complete chain has: Deployment owns ReplicaSet owns Pod.
func detectOwnershipChainComplete(g *graph.Graph) ([]Finding, Status) {
	var findings []Finding

	// Build node lookup maps by kind
	deployments := make(map[string]graph.Node)
	replicasets := make(map[string]graph.Node)
	pods := make(map[string]graph.Node)

	for _, node := range g.Nodes {
		switch node.Kind {
		case "Deployment":
			deployments[node.ID] = node
		case "ReplicaSet":
			replicasets[node.ID] = node
		case "Pod":
			pods[node.ID] = node
		}
	}

	// Skip pattern if no deployments exist
	if len(deployments) == 0 {
		return []Finding{{
			Pattern:  "k8s.ownership_chain_complete",
			Severity: SeverityInfo,
			Message:  "No Deployments found in graph",
		}}, StatusSkip
	}

	// Build edge maps: parent -> children
	deploymentToRS := make(map[string][]string)   // Deployment ID -> ReplicaSet IDs
	rsToPods := make(map[string][]string)         // ReplicaSet ID -> Pod IDs
	rsToDeployment := make(map[string]string)     // ReplicaSet ID -> Deployment ID
	podToRS := make(map[string]string)            // Pod ID -> ReplicaSet ID

	for _, edge := range g.Edges {
		if edge.Type != graph.EdgeTypeOwns {
			continue
		}

		// Check Deployment -> ReplicaSet
		if _, isDeployment := deployments[edge.From]; isDeployment {
			if _, isRS := replicasets[edge.To]; isRS {
				deploymentToRS[edge.From] = append(deploymentToRS[edge.From], edge.To)
				rsToDeployment[edge.To] = edge.From
			}
		}

		// Check ReplicaSet -> Pod
		if _, isRS := replicasets[edge.From]; isRS {
			if _, isPod := pods[edge.To]; isPod {
				rsToPods[edge.From] = append(rsToPods[edge.From], edge.To)
				podToRS[edge.To] = edge.From
			}
		}
	}

	// Check for orphaned ReplicaSets (RS without owning Deployment)
	confidenceOrphanedRS := 0.9
	for rsID, rs := range replicasets {
		if _, hasOwner := rsToDeployment[rsID]; !hasOwner {
			findings = append(findings, Finding{
				Pattern:    "k8s.ownership_chain_complete",
				Severity:   SeverityWarning,
				Message:    fmt.Sprintf("ReplicaSet %q has no owning Deployment in graph", rs.Name),
				Resource:   rsID,
				Confidence: &confidenceOrphanedRS,
				Refs:       []string{fmt.Sprintf("k8s:ReplicaSet/%s/%s", rs.Namespace, rs.Name)},
				Evidence: []string{
					"No 'owns' edge from any Deployment to this ReplicaSet",
				},
				Remediation: &Remediation{
					Summary: "Verify the ReplicaSet is owned by a Deployment, or confirm it is intentionally standalone.",
					Steps: []string{
						"Check for a Deployment in the namespace with a matching selector/labels.",
						"Inspect the ReplicaSet metadata.ownerReferences.",
						"If it's orphaned and not serving pods, consider cleaning it up.",
					},
					Links: []string{
						"https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/",
					},
				},
			})
		}
	}

	// Check for orphaned Pods (Pod without owning ReplicaSet, for RS-owned pods only)
	for podID, pod := range pods {
		if _, hasOwner := podToRS[podID]; !hasOwner {
			// This might be a standalone pod or owned by something else (DaemonSet, StatefulSet, Job)
			// Only report as info since we're focused on Deployment chains
			findings = append(findings, Finding{
				Pattern:  "k8s.ownership_chain_complete",
				Severity: SeverityInfo,
				Message:  fmt.Sprintf("Pod %q is not owned by any ReplicaSet in graph", pod.Name),
				Resource: podID,
				Evidence: []string{
					"No 'owns' edge from any ReplicaSet to this Pod",
					"This may be a standalone pod or owned by a different controller type",
				},
			})
		}
	}

	// Check for Deployments without ReplicaSets
	confidenceNoRS := 0.85
	for deployID, deploy := range deployments {
		if rsIDs, hasRS := deploymentToRS[deployID]; !hasRS || len(rsIDs) == 0 {
			findings = append(findings, Finding{
				Pattern:    "k8s.ownership_chain_complete",
				Severity:   SeverityWarning,
				Message:    fmt.Sprintf("Deployment %q has no ReplicaSets in graph", deploy.Name),
				Resource:   deployID,
				Confidence: &confidenceNoRS,
				Refs:       []string{fmt.Sprintf("k8s:Deployment/%s/%s", deploy.Namespace, deploy.Name)},
				Evidence: []string{
					"No 'owns' edge from this Deployment to any ReplicaSet",
					"This may indicate the Deployment has not created any replicas yet",
				},
				Remediation: &Remediation{
					Summary: "Check whether the Deployment controller is healthy and has created ReplicaSets.",
					Steps: []string{
						"Run `kubectl get rs -n <namespace>` and look for ReplicaSets owned by this Deployment.",
						"Check status/events with `kubectl describe deployment <name> -n <namespace>`.",
						"Verify the Deployment is not paused and is not scaled to zero.",
					},
					Links: []string{
						"https://kubernetes.io/docs/concepts/workloads/controllers/deployment/",
					},
				},
			})
		}
	}

	// Determine status
	hasErrors := false
	hasWarnings := false
	for _, f := range findings {
		if f.Severity == SeverityError {
			hasErrors = true
		}
		if f.Severity == SeverityWarning {
			hasWarnings = true
		}
	}

	if hasErrors || hasWarnings {
		return findings, StatusFail
	}

	return findings, StatusPass
}
