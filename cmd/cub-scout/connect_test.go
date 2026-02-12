package main

import (
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNormalizeServerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "api.example.com:6443", want: "https://api.example.com:6443"},
		{in: "https://api.example.com:6443/", want: "https://api.example.com:6443"},
		{in: "http://10.0.0.1:8080", want: "http://10.0.0.1:8080"},
		{in: "", wantErr: true},
		{in: "://broken", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeServerURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeServerURL(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeServerURL(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeServerURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildAuthInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		token      string
		user       string
		pass       string
		clientCert string
		clientKey  string
		wantLabel  string
		wantErr    bool
	}{
		{name: "token", token: "abc", wantLabel: "bearer token"},
		{name: "basic-auth", user: "u", pass: "p", wantLabel: "basic auth"},
		{name: "client-cert", clientCert: "/tmp/a.crt", clientKey: "/tmp/a.key", wantLabel: "client cert"},
		{name: "no-auth", wantLabel: "no auth"},
		{name: "mixed-token-basic", token: "abc", user: "u", pass: "p", wantErr: true},
		{name: "missing-password", user: "u", wantErr: true},
		{name: "missing-client-key", clientCert: "/tmp/a.crt", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			auth, label, err := buildAuthInfo(tt.token, tt.user, tt.pass, tt.clientCert, tt.clientKey)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildAuthInfo expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildAuthInfo error: %v", err)
			}
			if auth == nil {
				t.Fatalf("buildAuthInfo returned nil auth")
			}
			if label != tt.wantLabel {
				t.Fatalf("buildAuthInfo label = %q, want %q", label, tt.wantLabel)
			}
		})
	}
}

func TestSelectSourceContext(t *testing.T) {
	t.Parallel()

	cfg := clientcmdapi.NewConfig()
	cfg.Contexts = map[string]*clientcmdapi.Context{
		"one": {},
		"two": {},
	}
	cfg.CurrentContext = "one"

	got, err := selectSourceContext(cfg, "")
	if err != nil {
		t.Fatalf("selectSourceContext current context error: %v", err)
	}
	if got != "one" {
		t.Fatalf("selectSourceContext current = %q, want one", got)
	}

	got, err = selectSourceContext(cfg, "two")
	if err != nil {
		t.Fatalf("selectSourceContext explicit error: %v", err)
	}
	if got != "two" {
		t.Fatalf("selectSourceContext explicit = %q, want two", got)
	}

	_, err = selectSourceContext(cfg, "missing")
	if err == nil {
		t.Fatalf("selectSourceContext expected error for missing context")
	}
}

func TestDefaultContextName(t *testing.T) {
	t.Parallel()

	got := defaultContextName("https://api.ske-vcl-pro.example.com:6443")
	if got != "api-ske-vcl-pro-example-com" {
		t.Fatalf("defaultContextName unexpected: %q", got)
	}
}
