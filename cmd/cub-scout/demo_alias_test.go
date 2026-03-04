package main

import "testing"

func TestCanonicalDemoName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "alias_risk", in: "risk", want: "ccve"},
		{name: "canonical", in: "quick", want: "quick"},
		{name: "unknown_passthrough", in: "does-not-exist", want: "does-not-exist"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := canonicalDemoName(tc.in)
			if got != tc.want {
				t.Fatalf("canonicalDemoName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCanonicalScenarioName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "alias_bigbank", in: "bigbank", want: "bigbank-incident"},
		{name: "canonical", in: "break-glass", want: "break-glass"},
		{name: "unknown_passthrough", in: "does-not-exist", want: "does-not-exist"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := canonicalScenarioName(tc.in)
			if got != tc.want {
				t.Fatalf("canonicalScenarioName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
