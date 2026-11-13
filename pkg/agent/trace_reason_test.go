// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

// The words say what was observed: no marker was recognised. They do not say
// the resource is not managed, which the evidence does not show.
func TestNoOwnerTraceWordsDoNotClaimTheResourceIsUnmanaged(t *testing.T) {
	for _, message := range []string{NoOwnerTraceMessage, NoFluxOwnerTraceMessage} {
		if strings.Contains(strings.ToLower(message), "not managed") {
			t.Errorf("%q claims the resource is not managed", message)
		}
		if !IsNoOwnerTrace("", message) {
			t.Errorf("%q is not recognised as a no-owner result", message)
		}
	}
}

func TestIsNoOwnerTrace(t *testing.T) {
	cases := []struct {
		name, reason, text string
		want               bool
	}{
		{"typed reason decides, whatever the words", TraceReasonNoOwnerMarker, "anything at all", true},
		{"the flux CLI's wording", "", "✗ object not managed by Flux", true},
		{"results written before the reason existed", "", "resource not managed by any detected GitOps tool", true},
		{"helm found no release", "", "no Helm release found for deployment/api", true},
		{"a read failure is not a no-owner result", "", "kustomizations.kustomize.toolkit.fluxcd.io is forbidden", false},
		{"another reason is not a no-owner result", "timeout", "context deadline exceeded", false},
		{"nothing said", "", "", false},
	}
	for _, tc := range cases {
		if got := IsNoOwnerTrace(tc.reason, tc.text); got != tc.want {
			t.Errorf("%s: IsNoOwnerTrace(%q, %q) = %v, want %v", tc.name, tc.reason, tc.text, got, tc.want)
		}
	}
}
