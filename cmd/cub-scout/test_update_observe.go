// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// The import test-update flows exist to prove the ConfigHub pipeline end to
// end: change a unit, and see the change arrive in the cluster.
//
// They used to prove it with cub: read `cub unit livedata`, write the change
// back, then `cub unit apply --wait`. cub removed both commands (#571), and
// ConfigHub's unit JSON carries no applied or live revision to read instead —
// checked across every unit with a target on a live server. So the flow lost
// its proof and, until #571, claimed success anyway.
//
// cub-scout does have a way to answer the question: look at the cluster. That
// is what it is for. These flows now write the unit and then watch the live
// object for the annotation they wrote, bounded, and report what they saw
// rather than what they assume.

// annotatedTarget is the live object a test-update expects its annotation to
// reach: the workload the unit's data describes.
type annotatedTarget struct {
	Kind      string
	Name      string
	Namespace string
}

// annotationObservation is what watching the cluster established.
type annotationObservation struct {
	Seen    bool
	Elapsed time.Duration
	// Reason is why nothing was seen: the last read error, or that the
	// annotation never arrived before the deadline.
	Reason string
}

// Summary is one sentence about what was observed, for the flow's message.
func (o annotationObservation) Summary(kind, name, namespace string) string {
	where := strings.ToLower(kind) + "/" + name
	if namespace != "" {
		where += " in " + namespace
	}
	if o.Seen {
		return fmt.Sprintf("observed on %s after %s", where, o.Elapsed.Round(time.Second))
	}
	return fmt.Sprintf("not observed on %s after %s: %s", where, o.Elapsed.Round(time.Second), o.Reason)
}

// testUpdateTimeoutFlag bounds how long a test-update waits for the annotation
// to reach the cluster. It replaces the two minutes the old flow spent waiting
// for `cub unit livedata` to report data.
var testUpdateTimeoutFlag time.Duration

func testUpdateTimeout() time.Duration {
	if testUpdateTimeoutFlag > 0 {
		return testUpdateTimeoutFlag
	}
	return 2 * time.Minute
}

// testUpdateInterval is how often the live object is read while waiting.
func testUpdateInterval() time.Duration {
	if testUpdateTimeoutFlag > 0 && testUpdateTimeoutFlag < 10*time.Second {
		return time.Second
	}
	return 3 * time.Second
}

// readLiveAnnotationsFn reads a live object's annotations. A variable so the
// flows can be driven without a cluster.
var readLiveAnnotationsFn = readLiveAnnotations

// observeSleepFn is the wait between polls, replaced in tests.
var observeSleepFn = time.Sleep

func readLiveAnnotations(ctx context.Context, kind, name, namespace string) (map[string]string, error) {
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unsupported resource kind %q", kind)
	}
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return obj.GetAnnotations(), nil
}

// waitForAnnotationInCluster watches one live object until it carries the
// annotation, or until the deadline. It reports what it saw; it never reports
// that a change was applied, because it did not apply anything.
func waitForAnnotationInCluster(ctx context.Context, kind, name, namespace, key, value string, timeout, interval time.Duration) annotationObservation {
	started := time.Now()
	if timeout <= 0 {
		return annotationObservation{Reason: "not waited for: the timeout is zero"}
	}
	if interval <= 0 {
		interval = time.Second
	}

	var lastErr error
	for {
		annotations, err := readLiveAnnotationsFn(ctx, kind, name, namespace)
		switch {
		case err != nil:
			lastErr = err
		case annotations[key] == value:
			return annotationObservation{Seen: true, Elapsed: time.Since(started)}
		default:
			lastErr = nil
		}

		if elapsed := time.Since(started); elapsed >= timeout {
			return annotationObservation{Elapsed: elapsed, Reason: observationReason(lastErr)}
		}
		if err := ctx.Err(); err != nil {
			return annotationObservation{Elapsed: time.Since(started), Reason: err.Error()}
		}
		observeSleepFn(interval)
	}
}

// observationReason says why the annotation was not seen, keeping a read error
// apart from a cluster that simply has not converged.
func observationReason(lastErr error) string {
	if lastErr != nil {
		return "the live object could not be read: " + lastErr.Error()
	}
	return "the unit carries it, but the target has not applied it — check that a worker is running for this target"
}
