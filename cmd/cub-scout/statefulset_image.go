// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package main

import (
	"context"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func observeStatefulSetImage(ctx context.Context, r *agent.ReleaseCheckReport, reader *agent.BoundedResourceReader, wl agent.WorkloadConvergedObservedObject, maxPods int) agent.RunningImageWorkload {
	desired, live := wl.Desired, wl.Live
	selector, err := agent.WorkloadPodSelector(live)
	if err != nil {
		return agent.BuildRunningImageWorkload(desired, nil, false, "selector-unsupported")
	}
	pods, capped, e, err := reader.ListPodsMatching(ctx, live.GetNamespace(), selector, maxPods)
	r.AddRead(e)
	readErr := ""
	if err != nil {
		readErr = classifyPodReadError(err)
	}
	w := agent.BuildRunningImageWorkload(desired, pods, capped, readErr)
	if e.Available {
		w.ObservedAt = &e.ObservedAt
	}
	if readErr != "" {
		return w
	}
	if capped {
		w.Verdict, w.Reason = "unknown", "coverage-capped"
		return w
	}
	revisionName, _, _ := unstructured.NestedString(live.Object, "status", "currentRevision")
	var revision *unstructured.Unstructured
	if revisionName != "" {
		var re agent.BoundedReadEvidence
		revision, re, err = reader.Read(ctx, agent.BoundedResourceRef{APIVersion: "apps/v1", Kind: "ControllerRevision", Name: revisionName, Namespace: live.GetNamespace()}, true)
		r.AddRead(re)
		if err != nil {
			readErr = classifyPodReadError(err)
		}
	}
	coverage := agent.BuildStatefulSetImageCoverage(live, pods, revision)
	w.StatefulSet = &coverage
	if !coverage.Complete {
		w.Verdict, w.Reason = "unknown", coverage.Reason
	}
	if readErr != "" {
		w.Verdict, w.Reason = "unknown", readErr
		coverage.Complete, coverage.Reason = false, readErr
	}
	after, re, err := reader.Read(ctx, w.ID, true)
	r.AddRead(re)
	if recheckReason := workloadRecheckReason(live, after, err); recheckReason != "" {
		w.Verdict, w.Reason = "unknown", recheckReason
		coverage.Complete, coverage.Reason = false, w.Reason
	}
	return w
}
