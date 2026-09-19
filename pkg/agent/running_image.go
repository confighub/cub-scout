// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"sort"
	"strings"
	"time"

	ocidigest "github.com/opencontainers/go-digest"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Running-image identity is a distinct evidence tier from the configuration
// bundle check. It answers "is the image actually running the one the intended
// configuration declares?" by comparing the container image reference in the
// desired workload spec against the digest actually executing in live pods
// (.status.containerStatuses[].imageID).
//
// Three identities are kept strictly separate and never compared to one another:
//   1. configuration-bundle digest  (the OCI config artifact; see release_check)
//   2. intended container-image ref (spec.template.spec.containers[].image)
//   3. running container-image digest (pod containerStatuses[].imageID)
//
// This tier compares (2) against (3). It is deliberately conservative: a mutable
// tag is UNKNOWN, never an assumed match; an unresolved index/platform digest
// difference is UNKNOWN too, not proof that the wrong image is running.

// runningImageCaveat is attached to unresolved differences: a manifest-list (index)
// digest and its resolved per-architecture manifest digest can legitimately
// differ, and we cannot distinguish that from a genuine mismatch without
// registry-side resolution (later work, #505).
const runningImageCaveat = "Multi-architecture images can differ between the index digest and the resolved per-architecture manifest digest; confirm against the registry."

// RunningImageContainer is the per-container comparison result.
type RunningImageContainer struct {
	Name           string   `json:"name"`
	Verdict        string   `json:"verdict"` // match | mismatch | unknown
	Reason         string   `json:"reason,omitempty"`
	IntendedImage  string   `json:"intendedImage,omitempty"`
	IntendedDigest string   `json:"intendedDigest,omitempty"`
	RunningDigests []string `json:"runningDigests,omitempty"`
	Caveat         string   `json:"caveat,omitempty"`
}

// RunningImageWorkload aggregates the containers of one workload across the pods
// that were read for it.
type RunningImageWorkload struct {
	ID             BoundedResourceRef       `json:"id"`
	Verdict        string                   `json:"verdict"` // match | mismatch | unknown
	Reason         string                   `json:"reason,omitempty"`
	PodsRead       int                      `json:"podsRead"`
	CoverageCapped bool                     `json:"coverageCapped,omitempty"`
	Containers     []RunningImageContainer  `json:"containers,omitempty"`
	Pods           []RunningImagePod        `json:"pods,omitempty"`
	Deployment     *DeploymentImageCoverage `json:"deployment,omitempty"`
	ObservedAt     *time.Time               `json:"observedAt,omitempty"`
}

// RunningImagePod retains per-pod evidence so missing statuses cannot be hidden
// by a matching container in another pod. Ownership is checked separately.
type RunningImagePod struct {
	Name           string                  `json:"name"`
	UID            string                  `json:"uid"`
	ReplicaSetName string                  `json:"replicaSetName,omitempty"`
	ReplicaSetUID  string                  `json:"replicaSetUID,omitempty"`
	Containers     []RunningImageContainer `json:"containers"`
}

// RunningImageEvidence is the tier-level result folded into the release report.
type RunningImageEvidence struct {
	Verdict   string                 `json:"verdict"` // match | mismatch | unknown
	Reason    string                 `json:"reason,omitempty"`
	PodReads  int                    `json:"podReads"`
	Workloads []RunningImageWorkload `json:"workloads"`
}

type nameImage struct {
	name  string
	image string
}

type runObserved struct {
	repo   string
	digest string
	reason string
}

// ParseImageReference splits a container image reference into repository, tag and
// digest. Any of the three may be empty. The digest is validated as algo:hex.
func ParseImageReference(ref string) (repo, tag, digest string) {
	s := strings.TrimSpace(ref)
	if i := strings.Index(s, "@"); i >= 0 {
		digest = s[i+1:]
		s = s[:i]
	}
	slash := strings.LastIndex(s, "/")
	if c := strings.LastIndex(s, ":"); c > slash {
		tag = s[c+1:]
		s = s[:c]
	}
	repo = s
	if !validDigest(digest) {
		digest = ""
	}
	return repo, tag, digest
}

// DigestFromImageID extracts the repository and digest from a pod container
// status imageID. It tolerates the docker-pullable:// scheme, registry-qualified
// (repo@sha256:...) and bare (sha256:...) forms.
func DigestFromImageID(imageID string) (repo, digest string) {
	s := strings.TrimSpace(imageID)
	s = strings.TrimPrefix(s, "docker-pullable://")
	if i := strings.LastIndex(s, "@"); i >= 0 {
		repo = s[:i]
		digest = s[i+1:]
	} else if validDigest(s) {
		digest = s
	} else {
		repo = s
	}
	if !validDigest(digest) {
		digest = ""
	}
	return repo, digest
}

func validDigest(d string) bool {
	return ocidigest.Digest(d).Validate() == nil
}

func normalizeRepo(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	r = strings.TrimPrefix(r, "index.docker.io/")
	r = strings.TrimPrefix(r, "docker.io/")
	return r
}

// runningImageRank orders the tier outcomes worst-first for weakest-link
// aggregation: mismatch is the strongest negative, unknown is uncertainty, match
// is the only positive.
func runningImageRank(v string) int {
	switch v {
	case "mismatch":
		return 2
	case "unknown":
		return 1
	default:
		return 0
	}
}

func weakerRunningImage(a, b string) string {
	if runningImageRank(b) > runningImageRank(a) {
		return b
	}
	return a
}

func intendedContainers(obj *unstructured.Unstructured) []nameImage {
	if obj == nil {
		return nil
	}
	fields := []string{"spec", "template", "spec", "containers"}
	if obj.GetKind() == "Pod" {
		fields = []string{"spec", "containers"}
	}
	items, found, err := unstructured.NestedSlice(obj.Object, fields...)
	if err != nil || !found {
		return nil
	}
	var out []nameImage
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		image, _ := m["image"].(string)
		out = append(out, nameImage{name: name, image: image})
	}
	return out
}

// runningByContainer emits one observation per intended container per pod,
// including a reason when its status is missing or cannot establish execution.
func runningByContainer(pods []*unstructured.Unstructured, containers []nameImage) map[string][]runObserved {
	out := map[string][]runObserved{}
	for _, pod := range pods {
		var statuses []interface{}
		if pod != nil {
			statuses, _, _ = unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
		}
		for _, ci := range containers {
			ro := runObserved{reason: "container-not-found"}
			count := 0
			for _, item := range statuses {
				m, ok := item.(map[string]interface{})
				if !ok || m["name"] != ci.name {
					continue
				}
				count++
				imageID, _ := m["imageID"].(string)
				ro.repo, ro.digest = DigestFromImageID(imageID)
				ro.reason = ""
				state, _, _ := unstructured.NestedMap(m, "state")
				running, ok := state["running"].(map[string]interface{})
				ready, _ := m["ready"].(bool)
				if !ok || running == nil || len(state) != 1 {
					ro.reason = "container-not-running"
				} else if !ready {
					ro.reason = "container-not-ready"
				}
			}
			if count > 1 {
				ro.reason = "container-status-ambiguous"
			}
			if reason := runningImagePodReason(pod); reason != "" {
				ro.reason = reason
			}
			out[ci.name] = append(out[ci.name], ro)
		}
	}
	return out
}

// IntendedImageDigestPinned reports whether any container in the workload pins a
// digest. When nothing is digest-pinned there is nothing a pod read could
// confirm, so callers skip the read entirely.
func IntendedImageDigestPinned(obj *unstructured.Unstructured) bool {
	for _, ci := range intendedContainers(obj) {
		if _, _, d := ParseImageReference(ci.image); d != "" {
			return true
		}
	}
	return false
}

func compareRunningContainer(ci nameImage, running []runObserved, podsRead int) RunningImageContainer {
	running = append([]runObserved(nil), running...)
	sort.Slice(running, func(i, j int) bool {
		a, b := running[i], running[j]
		if a.reason != b.reason {
			return a.reason < b.reason
		}
		if a.digest != b.digest {
			return a.digest < b.digest
		}
		return a.repo < b.repo
	})
	c := RunningImageContainer{Name: ci.name, IntendedImage: ci.image}
	irepo, tag, idigest := ParseImageReference(ci.image)
	c.IntendedDigest = idigest
	if idigest == "" {
		c.Verdict = "unknown"
		if tag == "" {
			c.Reason = "intended image has no digest or tag"
		} else {
			c.Reason = "mutable-tag"
		}
		return c
	}
	if len(running) == 0 {
		c.Verdict = "unknown"
		if podsRead == 0 {
			c.Reason = "no-running-pods"
		} else {
			c.Reason = "container-not-found"
		}
		return c
	}
	verdict := "match"
	reason := ""
	seen := map[string]bool{}
	for _, ro := range running {
		if ro.reason != "" {
			verdict = weakerRunningImage(verdict, "unknown")
			if reason == "" || ro.reason < reason {
				reason = ro.reason
			}
		}
		if ro.digest == "" {
			verdict = weakerRunningImage(verdict, "unknown")
			if reason == "" {
				reason = "unreadable"
			}
			continue
		}
		if !seen[ro.digest] {
			seen[ro.digest] = true
			c.RunningDigests = append(c.RunningDigests, ro.digest)
		}
		if ro.digest == idigest {
			continue
		}
		// Digest differs. A bare-digest imageID carries no repository; the
		// container was matched by name inside the workload's own pods, so treat
		// it as the same repository. A differing named repository is not
		// comparable and stays UNKNOWN rather than a false alarm.
		if ro.repo == "" || normalizeRepo(ro.repo) == normalizeRepo(irepo) {
			verdict = weakerRunningImage(verdict, "unknown")
			if reason == "" {
				reason = "digest-form-unresolved"
			}
			c.Caveat = runningImageCaveat
		} else {
			verdict = weakerRunningImage(verdict, "unknown")
			if reason == "" {
				reason = "different-repository"
			}
		}
	}
	sort.Strings(c.RunningDigests)
	if verdict == "match" && len(c.RunningDigests) == 0 {
		verdict = "unknown"
		reason = "unreadable"
	}
	c.Verdict = verdict
	c.Reason = reason
	return c
}

// BuildRunningImageWorkload compares the intended images of one workload against
// the images running in the pods read for it. readErr, when non-empty, records a
// pod-read failure (e.g. RBAC denial) and yields UNKNOWN without a false claim.
func BuildRunningImageWorkload(desired *unstructured.Unstructured, pods []*unstructured.Unstructured, capped bool, readErr string) RunningImageWorkload {
	w := RunningImageWorkload{PodsRead: len(pods), CoverageCapped: capped}
	if desired != nil {
		gv := desired.GetObjectKind().GroupVersionKind()
		w.ID = BoundedResourceRef{APIVersion: desired.GetAPIVersion(), Kind: gv.Kind, Namespace: desired.GetNamespace(), Name: desired.GetName()}
	}
	containers := intendedContainers(desired)
	if len(containers) == 0 {
		w.Verdict = "unknown"
		w.Reason = "no intended containers found"
		return w
	}
	if readErr != "" {
		w.Verdict = "unknown"
		w.Reason = readErr
		for _, ci := range containers {
			w.Containers = append(w.Containers, RunningImageContainer{Name: ci.name, Verdict: "unknown", Reason: readErr, IntendedImage: ci.image})
		}
		return w
	}
	observed := runningByContainer(pods, containers)
	for _, pod := range pods {
		p := RunningImagePod{}
		if pod != nil {
			p.Name, p.UID = pod.GetName(), string(pod.GetUID())
			if owner := ControllerOwner(pod, "ReplicaSet"); owner != nil {
				p.ReplicaSetName, p.ReplicaSetUID = owner.Name, string(owner.UID)
			}
		}
		perPod := runningByContainer([]*unstructured.Unstructured{pod}, containers)
		for _, ci := range containers {
			p.Containers = append(p.Containers, compareRunningContainer(ci, perPod[ci.name], 1))
		}
		w.Pods = append(w.Pods, p)
	}
	sort.Slice(w.Pods, func(i, j int) bool {
		if w.Pods[i].Name != w.Pods[j].Name {
			return w.Pods[i].Name < w.Pods[j].Name
		}
		return w.Pods[i].UID < w.Pods[j].UID
	})
	verdict := "match"
	for _, ci := range containers {
		rc := compareRunningContainer(ci, observed[ci.name], len(pods))
		w.Containers = append(w.Containers, rc)
		verdict = weakerRunningImage(verdict, rc.Verdict)
	}
	if capped && verdict != "mismatch" {
		verdict = weakerRunningImage(verdict, "unknown")
	}
	w.Verdict = verdict
	w.Reason = reasonForContainers(w.Containers, verdict)
	if capped && w.Reason == "" {
		w.Reason = "coverage-capped"
	}
	return w
}

func reasonForContainers(containers []RunningImageContainer, verdict string) string {
	for _, c := range containers {
		if c.Verdict == verdict && c.Reason != "" {
			return c.Reason
		}
	}
	return ""
}

// AggregateRunningImage folds the per-workload results into one tier verdict and
// a representative reason (the reason of a workload at the weakest verdict).
func AggregateRunningImage(workloads []RunningImageWorkload) (string, string) {
	if len(workloads) == 0 {
		return "unknown", "no supported workloads to inspect"
	}
	verdict := "match"
	for _, w := range workloads {
		verdict = weakerRunningImage(verdict, w.Verdict)
	}
	reason := ""
	for _, w := range workloads {
		if w.Verdict == verdict && w.Reason != "" {
			reason = w.Reason
			break
		}
	}
	return verdict, reason
}

// WorkloadSelectorLabels is the legacy labels-only helper. Release verification
// uses WorkloadPodSelector instead, preserving matchExpressions as well.
func WorkloadSelectorLabels(live *unstructured.Unstructured) (map[string]string, bool) {
	if live == nil {
		return nil, false
	}
	labels, found, err := unstructured.NestedStringMap(live.Object, "spec", "selector", "matchLabels")
	if err != nil || !found || len(labels) == 0 {
		return nil, false
	}
	return labels, true
}
