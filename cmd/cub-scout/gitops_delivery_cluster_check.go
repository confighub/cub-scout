package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The states of a cluster check. They compare two reports; none of them is a
// verdict on the delivery.
const (
	liveStatusClusterAgrees        = "agrees"
	liveStatusClusterDisagrees     = "disagrees"
	liveStatusClusterNotObservable = "not-observable"

	liveStatusClusterLimitation = "Compares one thing: whether a controller in this cluster reports this Release's manifest digest as applied, against the sync state the deploying tool recorded. The two reports were made at different moments and can both have been true in turn. Health is not compared. A controller's report is not proof that workloads converged. Controllers are matched by ConfigHub registry and space slug only: not by organization, Target or path, and this cluster may not be the Release's Target."
)

// ConfigHubLiveStatusClusterCheck is cub-scout's own reading of the cluster
// beside the status a deploying tool recorded on a Release (#887). ConfigHub
// gates on that status; this says whether the cluster in the current context
// agrees with it.
type ConfigHubLiveStatusClusterCheck struct {
	State       string                          `json:"state"`
	Reason      string                          `json:"reason"`
	Controllers []ConfigHubLiveStatusController `json:"controllers,omitempty"`
	ObservedAt  string                          `json:"observedAt,omitempty"`
	Limitation  string                          `json:"limitation"`
}

// ConfigHubLiveStatusController is one controller object that pulls the
// Release's space, with what it reports about the Release's digest.
type ConfigHubLiveStatusController struct {
	Kind      string                           `json:"kind"`
	Namespace string                           `json:"namespace"`
	Name      string                           `json:"name"`
	State     string                           `json:"state"`
	Revision  agent.ControllerRevisionEvidence `json:"revision"`
}

var (
	liveStatusApplicationGVR   = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	liveStatusOCIRepositoryGVR = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}
	liveStatusKustomizationGVR = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
)

// liveStatusClusterControllers is what one read of the cluster found: the
// controller objects that pull a ConfigHub space, by space slug, and why a
// kind could not be read.
type liveStatusClusterControllers struct {
	bySpace map[string][]*unstructured.Unstructured
	unread  []string
}

// readLiveStatusClusterControllers lists Argo CD Applications and Flux
// Kustomizations once and keeps those whose source is a ConfigHub space.
func readLiveStatusClusterControllers(ctx context.Context, client dynamic.Interface) liveStatusClusterControllers {
	found := liveStatusClusterControllers{bySpace: map[string][]*unstructured.Unstructured{}}
	list := func(gvr schema.GroupVersionResource, kind string) []unstructured.Unstructured {
		items, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			// The kind is not installed here: nothing of it to read.
			return nil
		}
		if err != nil {
			found.unread = append(found.unread, fmt.Sprintf("%s could not be listed: %v", kind, err))
			return nil
		}
		return items.Items
	}
	space := func(url string) string {
		info := agent.ParseOCISource(url)
		if !info.IsConfigHub || info.Layout != agent.OCILayoutSpace {
			return ""
		}
		return info.Space
	}
	applications := list(liveStatusApplicationGVR, "Argo CD Applications")
	for i := range applications {
		repos := []string{}
		if repo, _, _ := unstructured.NestedString(applications[i].Object, "spec", "source", "repoURL"); repo != "" {
			repos = append(repos, repo)
		}
		// A multi-source Application is counted, so that it is reported as
		// not comparable and not silently left out of "all controllers".
		sources, _, _ := unstructured.NestedSlice(applications[i].Object, "spec", "sources")
		for _, source := range sources {
			if entry, ok := source.(map[string]interface{}); ok {
				if repo, _ := entry["repoURL"].(string); repo != "" {
					repos = append(repos, repo)
				}
			}
		}
		matched := map[string]bool{}
		for _, repo := range repos {
			if slug := space(repo); slug != "" && !matched[slug] {
				matched[slug] = true
				found.bySpace[slug] = append(found.bySpace[slug], &applications[i])
			}
		}
	}
	sources := map[string]string{}
	repositories := list(liveStatusOCIRepositoryGVR, "Flux OCIRepositories")
	for i := range repositories {
		url, _, _ := unstructured.NestedString(repositories[i].Object, "spec", "url")
		if slug := space(url); slug != "" {
			sources[repositories[i].GetNamespace()+"/"+repositories[i].GetName()] = slug
		}
	}
	if len(sources) > 0 {
		kustomizations := list(liveStatusKustomizationGVR, "Flux Kustomizations")
		for i := range kustomizations {
			kind, _, _ := unstructured.NestedString(kustomizations[i].Object, "spec", "sourceRef", "kind")
			name, _, _ := unstructured.NestedString(kustomizations[i].Object, "spec", "sourceRef", "name")
			namespace, _, _ := unstructured.NestedString(kustomizations[i].Object, "spec", "sourceRef", "namespace")
			if namespace == "" {
				namespace = kustomizations[i].GetNamespace()
			}
			if slug := sources[namespace+"/"+name]; kind == "OCIRepository" && slug != "" {
				found.bySpace[slug] = append(found.bySpace[slug], &kustomizations[i])
			}
		}
	}
	return found
}

// checkLiveStatusesAgainstCluster adds a cluster check to every status that is
// recorded on a Release. It makes at most three list calls, whatever the
// number of statuses, and none when no status is on a Release.
func checkLiveStatusesAgainstCluster(ctx context.Context, client dynamic.Interface, statuses []ConfigHubLiveStatusEvidence, now time.Time) {
	var found *liveStatusClusterControllers
	for i := range statuses {
		status := &statuses[i]
		if status.RecordedOn != "release" {
			continue
		}
		check := &ConfigHubLiveStatusClusterCheck{State: liveStatusClusterNotObservable, Limitation: liveStatusClusterLimitation}
		status.ClusterCheck = check
		reportedSynced := strings.EqualFold(strings.TrimSpace(status.SyncStatus), "Synced")
		switch sync := strings.ToLower(strings.TrimSpace(status.SyncStatus)); {
		case client == nil:
			check.Reason = "no cluster was read"
			continue
		case sync == "" || sync == "unknown":
			check.Reason = "the recorded status reports no sync state to compare"
			continue
		case sync != "synced" && sync != "outofsync":
			check.Reason = fmt.Sprintf("the recorded sync state %q is neither Synced nor OutOfSync, so it says nothing a digest can be compared with", status.SyncStatus)
			continue
		case status.Freshness != "fresh":
			check.Reason = fmt.Sprintf("the recorded status is %s (observed %s); comparing it with the cluster now would compare different moments", firstNonEmpty(status.Freshness, "undated"), firstNonEmpty(status.ObservedAt, "at no stated time"))
			continue
		case status.SupersededByRelease != "":
			check.Reason = fmt.Sprintf("a newer Release, %s, has no recorded status; a controller may already be on it, so this Release's status is not compared", status.SupersededByRelease)
			continue
		case !completeSHA256Digest.MatchString(status.manifestDigest):
			check.Reason = "the Release has no complete manifest digest to compare"
			continue
		case strings.TrimSpace(status.Space) == "":
			check.Reason = "the Release's space slug is unknown, so no controller source can be matched to it"
			continue
		}
		if found == nil {
			read := readLiveStatusClusterControllers(ctx, client)
			found = &read
		}
		check.ObservedAt = now.UTC().Format(time.RFC3339)
		controllers := found.bySpace[status.Space]
		if len(controllers) == 0 {
			check.Reason = fmt.Sprintf("no Argo CD Application or Flux Kustomization in this cluster was recognised as pulling ConfigHub space %q", status.Space)
			if len(found.unread) > 0 {
				check.Reason += "; " + strings.Join(found.unread, "; ")
			}
			continue
		}
		recordedAt, _ := parseConfigHubObservedAt(status.ObservedAt)
		agree, disagree, unknown, later := 0, 0, 0, 0
		for _, object := range controllers {
			revision := agent.BuildControllerRevisionEvidence(object, status.manifestDigest, now)
			state := liveStatusClusterNotObservable
			switch revision.Comparison {
			case "match":
				state = liveStatusClusterDisagrees
				if reportedSynced {
					state = liveStatusClusterAgrees
				}
			case "mismatch":
				state = liveStatusClusterAgrees
				if reportedSynced {
					state = liveStatusClusterDisagrees
				}
			}
			// A controller that reconciled after the status was recorded can
			// differ from it without either being wrong.
			if reported, err := time.Parse(time.RFC3339Nano, revision.ReportedAt); state == liveStatusClusterDisagrees && err == nil && reported.After(recordedAt) {
				state = liveStatusClusterNotObservable
				later++
			}
			switch state {
			case liveStatusClusterAgrees:
				agree++
			case liveStatusClusterDisagrees:
				disagree++
			default:
				unknown++
			}
			check.Controllers = append(check.Controllers, ConfigHubLiveStatusController{Kind: object.GetKind(), Namespace: object.GetNamespace(), Name: object.GetName(), State: state, Revision: revision})
		}
		sort.Slice(check.Controllers, func(a, b int) bool {
			left, right := check.Controllers[a], check.Controllers[b]
			return left.Kind+"/"+left.Namespace+"/"+left.Name < right.Kind+"/"+right.Namespace+"/"+right.Name
		})
		switch {
		case len(found.unread) > 0 && disagree == 0:
			// Agreement must not be claimed over controllers that were not read.
			check.Reason = "not every controller kind could be read"
		case disagree > 0:
			check.State = liveStatusClusterDisagrees
			check.Reason = fmt.Sprintf("the recorded sync state is %q (observed %s), and %d of %d controllers pulling this space report otherwise about the Release's digest; where a controller gives no time for its report, the two may describe different moments", status.SyncStatus, status.ObservedAt, disagree, len(controllers))
		case later > 0:
			check.Reason = fmt.Sprintf("%d of %d controllers pulling this space reconciled after the status was recorded (%s) and now report otherwise; both can have been true in turn", later, len(controllers), status.ObservedAt)
		case unknown > 0:
			check.Reason = fmt.Sprintf("%d of %d controllers pulling this space gave no settled revision to compare", unknown, len(controllers))
		default:
			check.State = liveStatusClusterAgrees
			check.Reason = fmt.Sprintf("the recorded sync state is %q, and all %d controllers pulling this space report the same about the Release's digest", status.SyncStatus, agree)
		}
		if len(found.unread) > 0 {
			check.Reason += "; " + strings.Join(found.unread, "; ")
		}
	}
}

// liveStatusClusterCheckText is the one-line form for text and Markdown.
func liveStatusClusterCheckText(check *ConfigHubLiveStatusClusterCheck) string {
	if check == nil {
		return "-"
	}
	return check.State + ": " + check.Reason
}
