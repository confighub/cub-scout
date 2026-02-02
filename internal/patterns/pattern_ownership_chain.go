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
	for rsID, rs := range replicasets {
		if _, hasOwner := rsToDeployment[rsID]; !hasOwner {
			findings = append(findings, Finding{
				Pattern:  "k8s.ownership_chain_complete",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("ReplicaSet %q has no owning Deployment in graph", rs.Name),
				Resource: rsID,
				Evidence: []string{
					"No 'owns' edge from any Deployment to this ReplicaSet",
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
	for deployID, deploy := range deployments {
		if rsIDs, hasRS := deploymentToRS[deployID]; !hasRS || len(rsIDs) == 0 {
			findings = append(findings, Finding{
				Pattern:  "k8s.ownership_chain_complete",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Deployment %q has no ReplicaSets in graph", deploy.Name),
				Resource: deployID,
				Evidence: []string{
					"No 'owns' edge from this Deployment to any ReplicaSet",
					"This may indicate the Deployment has not created any replicas yet",
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
