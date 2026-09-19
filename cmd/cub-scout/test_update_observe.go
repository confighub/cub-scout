// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// The import test-update flows exist to prove the ConfigHub pipeline end to
// end: change a unit, and see the change arrive in the cluster.
//
// They used to prove it with cub: read `cub unit livedata`, write the change
// back, then `cub unit apply --wait`. cub removed both commands (#571), and
// ConfigHub's unit JSON carries no applied or live revision to read instead. So
// the flow lost its proof and, until #571, claimed success anyway.
//
// cub-scout does have a way to answer the question: look at the cluster. That is
// what it is for. These flows write the unit and then watch the live object for
// the annotation they wrote, bounded, and report what they saw.

// annotationLocation is where in the live object an annotation appears.
//
// This matters more than it looks. A rollout annotation is written to the pod
// template, and a Deployment's own metadata never gains it, so reading the wrong
// one means never seeing a change that did arrive — and then blaming the worker
// for it. The location is carried from the write to the read.
type annotationLocation int

const (
	annotationOnObject annotationLocation = iota
	annotationOnPodTemplate
)

func (l annotationLocation) String() string {
	if l == annotationOnPodTemplate {
		return "pod template"
	}
	return "metadata"
}

// annotatedTarget is the live object a test-update expects its annotation to
// reach: the workload the unit's data describes, and where in it to look.
type annotatedTarget struct {
	Kind      string
	Name      string
	Namespace string
	Where     annotationLocation
}

// annotationObservation is what watching the cluster established.
type annotationObservation struct {
	Seen    bool
	Elapsed time.Duration
	// Reason is why nothing was seen: a read that failed, a cluster that has
	// not converged, or that cub-scout could not look at all.
	Reason string
}

// Summary is one sentence about what was observed, for the flow's message.
func (o annotationObservation) Summary(target annotatedTarget) string {
	where := strings.ToLower(target.Kind) + "/" + target.Name
	if target.Namespace != "" {
		where += " in " + target.Namespace
	}
	if o.Seen {
		return fmt.Sprintf("observed on %s (%s) after %s", where, target.Where, o.Elapsed.Round(time.Second))
	}
	return fmt.Sprintf("not observed on %s after %s: %s", where, o.Elapsed.Round(time.Second), o.Reason)
}

// testUpdateTimeoutFlag bounds how long a test-update waits for the annotation
// to reach the cluster. It replaces the two minutes the old flow spent waiting
// for `cub unit livedata` to report data.
var testUpdateTimeoutFlag time.Duration

// testUpdateTimeout is what --test-timeout says. cobra writes the 2m default at
// registration, so a zero here is one the user asked for, and it means "write
// the unit and do not wait" rather than silently waiting two minutes.
func testUpdateTimeout() time.Duration {
	return testUpdateTimeoutFlag
}

// testUpdateInterval is how often the live object is read while waiting. It is
// never longer than the wait itself, so a short timeout is not spent inside a
// single sleep.
func testUpdateInterval() time.Duration {
	if testUpdateTimeoutFlag < 10*time.Second {
		if third := testUpdateTimeoutFlag / 3; third > 0 {
			return third
		}
		return testUpdateTimeoutFlag
	}
	return 3 * time.Second
}

// readLiveAnnotationsFn reads a live object's annotations, from wherever the
// target says they were written. A variable so the flows can be driven without
// a cluster.
var readLiveAnnotationsFn = readLiveAnnotations

// observeSleepFn is the wait between reads, and observeNowFn is the clock the
// deadline is measured with. Tests replace both, so they neither sleep nor spin.
var (
	observeSleepFn = time.Sleep
	observeNowFn   = time.Now
)

// permanentObservationError is a failure that waiting cannot fix: no kubeconfig,
// or a kind cub-scout cannot map. Retrying it for the whole timeout would leave
// someone with no cluster access watching a two-minute hang.
type permanentObservationError struct{ error }

func readLiveAnnotations(ctx context.Context, target annotatedTarget) (map[string]string, error) {
	gvr := kindToGVR(target.Kind)
	if gvr.Resource == "" {
		return nil, permanentObservationError{fmt.Errorf("cub-scout has no mapping for kind %q, so it cannot read the live object", target.Kind)}
	}
	cfg, err := buildConfig()
	if err != nil {
		return nil, permanentObservationError{fmt.Errorf("build kubernetes config: %w", err)}
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, permanentObservationError{fmt.Errorf("build dynamic client: %w", err)}
	}
	obj, err := dynClient.Resource(gvr).Namespace(target.Namespace).Get(ctx, target.Name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if target.Where == annotationOnPodTemplate {
		annotations, _, err := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
		if err != nil {
			return nil, fmt.Errorf("read the pod template's annotations: %w", err)
		}
		return annotations, nil
	}
	return obj.GetAnnotations(), nil
}

// waitForAnnotationInCluster watches one live object until it carries the
// annotation, or until the deadline. It reports what it saw; it never reports
// that a change was applied, because it applied nothing.
//
// The deadline bounds the whole wait, not the gap between reads: the read is
// given the remaining time, so a cluster that never answers cannot hold the flow
// open past the timeout.
func waitForAnnotationInCluster(parent context.Context, target annotatedTarget, key, value string, timeout, interval time.Duration) annotationObservation {
	started := observeNowFn()
	elapsed := func() time.Duration { return observeNowFn().Sub(started) }

	switch {
	case timeout <= 0:
		return annotationObservation{Reason: "cub-scout did not wait for it: --test-timeout is " + timeout.String()}
	case key == "" || value == "":
		return annotationObservation{Reason: "there is no annotation value to look for"}
	case target.Name == "":
		return annotationObservation{Reason: "the unit's data names no workload to watch"}
	case target.Namespace == "":
		// A read with no namespace asks for a cluster-scoped resource and gets
		// "the server could not find the requested resource", which says
		// nothing about the real problem.
		return annotationObservation{Reason: fmt.Sprintf("the unit's data names no namespace, so cub-scout does not know where to look for %s/%s",
			strings.ToLower(target.Kind), target.Name)}
	}
	if interval <= 0 {
		interval = time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var lastErr error
	for {
		annotations, err := readLiveAnnotationsFn(ctx, target)
		var permanent permanentObservationError
		switch {
		case errors.As(err, &permanent):
			return annotationObservation{Elapsed: elapsed(), Reason: permanent.Error()}
		case err != nil:
			lastErr = err
		case annotations[key] == value:
			return annotationObservation{Seen: true, Elapsed: elapsed()}
		default:
			lastErr = nil
		}

		if elapsed() >= timeout || ctx.Err() != nil {
			return annotationObservation{Elapsed: elapsed(), Reason: observationReason(lastErr, target)}
		}
		observeSleepFn(interval)
	}
}

// observationReason says why the annotation was not seen, keeping a read failure
// apart from a cluster that simply has not converged, and naming where cub-scout
// looked.
func observationReason(lastErr error, target annotatedTarget) string {
	if lastErr != nil {
		return "the live object could not be read: " + lastErr.Error()
	}
	return fmt.Sprintf("the unit carries it, but the %s's %s does not — check that a worker is running for this target",
		strings.ToLower(target.Kind), target.Where)
}
