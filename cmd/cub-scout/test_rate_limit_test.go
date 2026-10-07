// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "k8s.io/client-go/rest"

// client-go limits each client to 5 requests a second with a burst of 10. An
// inventory read makes about forty, so every in-process test that ran one
// against a local fake API server spent six seconds waiting on a limit that
// protects real API servers and tests nothing here (#800). Request counts and
// ordering are unchanged; only the waiting goes.
func init() {
	clusterConfigTestHook = func(cfg *rest.Config) {
		cfg.QPS = 1000
		cfg.Burst = 1000
	}
}
