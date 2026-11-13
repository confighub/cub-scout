// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import "net/http"

// KubernetesReadMeter counts the Kubernetes API requests a command makes and
// the response bytes it reads, so an answer can say what it cost. It counts
// what passes through the transports it wraps and nothing else: a subprocess
// (the flux CLI, cub) or a second client is not in it, and Coverage on the
// reported cost has to say which reads were measured.
type KubernetesReadMeter struct{ inner *kubernetesReadMeter }

func NewKubernetesReadMeter() *KubernetesReadMeter {
	return &KubernetesReadMeter{inner: &kubernetesReadMeter{}}
}

// WrapTransport has the shape rest.Config.Wrap takes.
func (m *KubernetesReadMeter) WrapTransport(base http.RoundTripper) http.RoundTripper {
	return meteredTransport{base: base, meter: m.inner}
}

// Snapshot returns the counts so far. Duration, Reused and Coverage are the
// caller's to fill: the meter does not know when the command began.
func (m *KubernetesReadMeter) Snapshot() KubernetesReadCost {
	if m == nil || m.inner == nil {
		return KubernetesReadCost{}
	}
	return m.inner.snapshot()
}
