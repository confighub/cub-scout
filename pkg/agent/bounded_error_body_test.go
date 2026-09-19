// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"io"
	"net/http"
	"testing"

	"k8s.io/client-go/rest"
)

type boundedErrorTransport struct {
	body   *boundedCountingBody
	status int
}

func (t boundedErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Header: make(http.Header), Body: t.body, Request: req}, nil
}

type boundedCountingBody struct {
	read   int
	closed bool
}

func (b *boundedCountingBody) Read(p []byte) (int, error) {
	if b.read >= 8<<20 {
		return 0, io.EOF
	}
	for i := range p {
		p[i] = 'x'
	}
	b.read += len(p)
	return len(p), nil
}

func (b *boundedCountingBody) Close() error { b.closed = true; return nil }

func TestBoundedErrorResponseBody(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		for _, list := range []bool{false, true} {
			body := &boundedCountingBody{}
			r, err := NewBoundedResourceReader(&rest.Config{Host: "https://example.invalid", Transport: boundedErrorTransport{body, status}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if list {
				_, _, _, err = r.ListPods(context.Background(), "default", map[string]string{"app": "test"}, 1)
			} else {
				_, err = r.get(context.Background(), "/api/v1")
			}
			if err == nil {
				t.Fatalf("status %d list=%t: expected error", status, list)
			}
			if body.read > (2<<20)+1 || !body.closed {
				t.Errorf("status %d list=%t: read=%d closed=%t", status, list, body.read, body.closed)
			}
		}
	}
}
