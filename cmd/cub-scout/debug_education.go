// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// Education contains explanation for common issues
type Education struct {
	Title           string
	ShortTip        string
	FullExplanation string
	LearnMoreURL    string
	Commands        []string
}

// educationDatabase contains explanations for common GitOps and Kubernetes issues
var educationDatabase = map[string]Education{
	// Pod container states
	"CrashLoopBackOff": {
		Title:    "CrashLoopBackOff",
		ShortTip: "Container keeps crashing. Kubernetes backs off between restarts.",
		FullExplanation: `CrashLoopBackOff means the container crashed and Kubernetes is
waiting longer between restart attempts. This is normal behavior to prevent
overwhelming the system.

Common causes:
- Application error (check logs with --previous)
- Missing configuration (ConfigMap, Secret)
- Database/dependency unreachable
- OOMKilled (out of memory)
- Health check failing immediately`,
		LearnMoreURL: "https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy",
		Commands: []string{
			"kubectl logs <pod> --previous",
			"kubectl describe pod <pod>",
		},
	},
	"ImagePullBackOff": {
		Title:    "ImagePullBackOff",
		ShortTip: "Cannot pull container image. Check image name and registry credentials.",
		FullExplanation: `ImagePullBackOff means Kubernetes cannot pull the container image.
This is often a configuration issue.

Common causes:
- Image name or tag is incorrect
- Image does not exist in the registry
- Registry credentials are missing or invalid
- Network cannot reach the registry
- Rate limiting on public registries (Docker Hub)`,
		LearnMoreURL: "https://kubernetes.io/docs/concepts/containers/images/",
		Commands: []string{
			"kubectl describe pod <pod>",
			"kubectl get secret -n <namespace> -o yaml",
		},
	},
	"ErrImagePull": {
		Title:    "ErrImagePull",
		ShortTip: "Failed to pull image. Often same causes as ImagePullBackOff.",
		FullExplanation: `ErrImagePull is the initial error when pulling fails.
After retries, it becomes ImagePullBackOff.

Check the Events section of kubectl describe for the specific error message.`,
		LearnMoreURL: "https://kubernetes.io/docs/concepts/containers/images/",
		Commands: []string{
			"kubectl describe pod <pod>",
		},
	},
	"Pending": {
		Title:    "Pending",
		ShortTip: "Pod waiting for resources or scheduling.",
		FullExplanation: `Pending means the pod cannot be scheduled to a node.

Common causes:
- Insufficient CPU or memory on nodes
- Node selector or affinity rules cannot be satisfied
- PersistentVolumeClaim is pending
- Taint/toleration mismatch
- Too many pods on cluster`,
		LearnMoreURL: "https://kubernetes.io/docs/concepts/scheduling-eviction/",
		Commands: []string{
			"kubectl describe pod <pod>",
			"kubectl get nodes -o wide",
			"kubectl describe node <node>",
		},
	},
	"OOMKilled": {
		Title:    "OOMKilled",
		ShortTip: "Container ran out of memory. Increase limits or optimize the app.",
		FullExplanation: `OOMKilled means the container exceeded its memory limit and was
terminated by the kernel.

Solutions:
- Increase memory limits in the pod spec
- Profile the application for memory leaks
- Optimize application memory usage
- Check for runaway processes`,
		LearnMoreURL: "https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/",
		Commands: []string{
			"kubectl top pod <pod>",
			"kubectl describe pod <pod>",
		},
	},
	"CreateContainerConfigError": {
		Title:    "CreateContainerConfigError",
		ShortTip: "Container configuration is invalid. Check ConfigMaps and Secrets.",
		FullExplanation: `CreateContainerConfigError means the container cannot be created
due to configuration issues.

Common causes:
- Referenced ConfigMap does not exist
- Referenced Secret does not exist
- Environment variable references are broken
- Volume mount paths are invalid`,
		LearnMoreURL: "https://kubernetes.io/docs/tasks/configure-pod-container/",
		Commands: []string{
			"kubectl describe pod <pod>",
			"kubectl get configmap -n <namespace>",
			"kubectl get secret -n <namespace>",
		},
	},

	// Flux/Argo failure stages
	"source": {
		Title:    "Source Stage Failure",
		ShortTip: "Source fetch failed. Check URL, credentials, and network.",
		FullExplanation: `Source stage failures occur when Flux or Argo cannot fetch
manifests from the source repository.

Common causes:
- Invalid URL or artifact reference
- Authentication credentials missing or expired
- Network connectivity issues
- Source server unavailable
- Rate limiting`,
		LearnMoreURL: "https://fluxcd.io/flux/components/source/",
		Commands: []string{
			"kubectl describe gitrepository <name> -n flux-system",
			"kubectl describe ocirepository <name> -n flux-system",
		},
	},
	"build": {
		Title:    "Build Stage Failure",
		ShortTip: "Build/render failed. Check kustomize/helm syntax and dependencies.",
		FullExplanation: `Build stage failures occur when Flux cannot render the manifests
using Kustomize or Helm.

Common causes:
- Invalid YAML syntax in source
- Missing Kustomize resources
- Helm values file not found
- Variable substitution errors`,
		LearnMoreURL: "https://fluxcd.io/flux/components/kustomize/",
		Commands: []string{
			"flux build kustomization <name> -n <namespace>",
			"kustomize build <path>",
		},
	},
	"apply": {
		Title:    "Apply Stage Failure",
		ShortTip: "Apply to cluster failed. Check RBAC and resource validation.",
		FullExplanation: `Apply stage failures occur when rendered manifests cannot be
applied to the cluster.

Common causes:
- RBAC permissions insufficient
- Resource validation failed
- Namespace does not exist
- Resource conflicts
- Immutable field changes`,
		LearnMoreURL: "https://fluxcd.io/flux/components/kustomize/kustomization/#reconciliation",
		Commands: []string{
			"kubectl auth can-i create deployment -n <namespace>",
			"kubectl describe kustomization <name> -n <namespace>",
		},
	},
	"sync": {
		Title:    "Sync Stage Failure",
		ShortTip: "Sync failed. Check ArgoCD application status and destination cluster.",
		FullExplanation: `Sync stage failures are specific to ArgoCD and indicate the
application cannot be synchronized.

Common causes:
- Destination cluster unreachable
- Application in auto-sync disabled
- Sync hooks failing
- Resource out of sync`,
		LearnMoreURL: "https://argo-cd.readthedocs.io/en/stable/user-guide/sync-options/",
		Commands: []string{
			"argocd app get <app-name>",
			"argocd app sync <app-name>",
		},
	},

	// GitOps concepts
	"Reconciling": {
		Title:    "Reconciling",
		ShortTip: "GitOps controller is actively syncing. This is normal.",
		FullExplanation: `Reconciling means the GitOps controller (Flux or Argo) is
actively working to bring the cluster state in line with the desired state.

This is normal and should complete within a few minutes.
If it stays in Reconciling for a long time, there may be an issue.`,
		LearnMoreURL: "https://fluxcd.io/flux/concepts/",
		Commands: []string{
			"flux get all -A",
		},
	},
	"Suspended": {
		Title:    "Suspended",
		ShortTip: "Reconciliation is paused. Resume when ready.",
		FullExplanation: `Suspended means the GitOps controller has been told to stop
reconciling this resource.

This is often done intentionally during maintenance or debugging.
To resume, remove the suspend annotation or flag.`,
		LearnMoreURL: "https://fluxcd.io/flux/components/kustomize/kustomization/#suspend",
		Commands: []string{
			"flux resume kustomization <name> -n <namespace>",
			"flux resume helmrelease <name> -n <namespace>",
		},
	},
	"Stalled": {
		Title:    "Stalled",
		ShortTip: "Reconciliation is not making progress. Check for blocking issues.",
		FullExplanation: `Stalled means the reconciliation has stopped making progress.
This is different from a failure - it means the controller is stuck.

Common causes:
- Dependent resource not ready
- Finalizer blocking deletion
- Webhook timeout`,
		LearnMoreURL: "https://fluxcd.io/flux/faq/",
		Commands: []string{
			"kubectl describe kustomization <name> -n <namespace>",
			"flux logs --level=error",
		},
	},

	// Source reasons
	"AuthenticationFailed": {
		Title:    "AuthenticationFailed",
		ShortTip: "Credentials are invalid or missing. Check your secret.",
		FullExplanation: `AuthenticationFailed means the GitOps controller cannot
authenticate to the source.

For Git:
- SSH key may be invalid or not in known_hosts
- Token may have expired
- Token may lack required scopes

For OCI:
- Registry credentials may be wrong
- Image pull secret may be misconfigured`,
		LearnMoreURL: "https://fluxcd.io/flux/components/source/gitrepositories/#secret-reference",
		Commands: []string{
			"kubectl get secret <secretname> -n <namespace> -o yaml",
			"kubectl create secret generic <name> --from-file=...",
		},
	},
}

// GetEducation returns education content for a given key
func GetEducation(key string) *Education {
	if edu, ok := educationDatabase[key]; ok {
		return &edu
	}
	return nil
}

// GetEducationTip returns a short tip for a given key
func GetEducationTip(key string) string {
	if edu, ok := educationDatabase[key]; ok {
		return edu.ShortTip
	}
	return ""
}

// GetEducationCommands returns diagnostic commands for a given key
func GetEducationCommands(key string) []string {
	if edu, ok := educationDatabase[key]; ok {
		return edu.Commands
	}
	return nil
}
