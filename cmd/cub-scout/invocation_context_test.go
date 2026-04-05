// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestNewInvocationContext_ExplicitModeWins(t *testing.T) {
	tests := []struct {
		name           string
		flag           string
		wantMode       PresentationMode
		wantExplicit   bool
		wantErr        bool
	}{
		{
			name:         "explicit human",
			flag:         "human",
			wantMode:     PresentationHuman,
			wantExplicit: true,
		},
		{
			name:         "explicit ai",
			flag:         "ai",
			wantMode:     PresentationAI,
			wantExplicit: true,
		},
		{
			name:         "explicit paired",
			flag:         "paired",
			wantMode:     PresentationPaired,
			wantExplicit: true,
		},
		{
			name:         "no flag - legacy",
			flag:         "",
			wantMode:     PresentationLegacy,
			wantExplicit: false,
		},
		{
			name:    "invalid mode",
			flag:    "invalid",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := NewInvocationContext(tc.flag, TransportCLI)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for flag %q, got nil", tc.flag)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if ctx.EffectiveMode != tc.wantMode {
				t.Errorf("EffectiveMode = %q, want %q", ctx.EffectiveMode, tc.wantMode)
			}
			if ctx.ExplicitRequest != tc.wantExplicit {
				t.Errorf("ExplicitRequest = %v, want %v", ctx.ExplicitRequest, tc.wantExplicit)
			}
			if ctx.IsExplicit() != tc.wantExplicit {
				t.Errorf("IsExplicit() = %v, want %v", ctx.IsExplicit(), tc.wantExplicit)
			}
		})
	}
}

func TestNewInvocationContext_RequestedModePreserved(t *testing.T) {
	ctx, err := NewInvocationContext("ai", TransportCLI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.RequestedMode != PresentationAI {
		t.Errorf("RequestedMode = %q, want %q", ctx.RequestedMode, PresentationAI)
	}

	// Empty flag should leave RequestedMode empty
	ctx2, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx2.RequestedMode != "" {
		t.Errorf("RequestedMode = %q, want empty", ctx2.RequestedMode)
	}
}

func TestNewInvocationContext_TransportPreserved(t *testing.T) {
	transports := []Transport{TransportCLI, TransportTUI, TransportMCP, TransportOther}

	for _, transport := range transports {
		ctx, err := NewInvocationContext("", transport)
		if err != nil {
			t.Fatalf("unexpected error for transport %q: %v", transport, err)
		}
		if ctx.Transport != transport {
			t.Errorf("Transport = %q, want %q", ctx.Transport, transport)
		}
	}
}

func TestNewInvocationContextWithDetection_DetectionDoesNotChangeEffectiveMode(t *testing.T) {
	// Key test: detection is advisory only and does not change EffectiveMode
	tests := []struct {
		name         string
		flag         string
		detected     DetectedContext
		wantMode     PresentationMode
		wantExplicit bool
	}{
		{
			name:         "no flag, AI detected - still uses legacy",
			flag:         "",
			detected:     DetectedAIHost,
			wantMode:     PresentationLegacy, // legacy, NOT ai
			wantExplicit: false,
		},
		{
			name:         "explicit human, AI detected - explicit wins",
			flag:         "human",
			detected:     DetectedAIHost,
			wantMode:     PresentationHuman,
			wantExplicit: true,
		},
		{
			name:         "explicit ai, terminal detected - explicit wins",
			flag:         "ai",
			detected:     DetectedTerminal,
			wantMode:     PresentationAI,
			wantExplicit: true,
		},
		{
			name:         "no flag, paired detected - still uses legacy",
			flag:         "",
			detected:     DetectedPaired,
			wantMode:     PresentationLegacy, // legacy, NOT paired
			wantExplicit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := NewInvocationContextWithDetection(tc.flag, TransportCLI, tc.detected)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ctx.EffectiveMode != tc.wantMode {
				t.Errorf("EffectiveMode = %q, want %q (detection should not change this)", ctx.EffectiveMode, tc.wantMode)
			}
			if ctx.ExplicitRequest != tc.wantExplicit {
				t.Errorf("ExplicitRequest = %v, want %v", ctx.ExplicitRequest, tc.wantExplicit)
			}
			if ctx.DetectedContext != tc.detected {
				t.Errorf("DetectedContext = %q, want %q", ctx.DetectedContext, tc.detected)
			}
		})
	}
}

func TestInvocationContext_Mode(t *testing.T) {
	ctx, _ := NewInvocationContext("paired", TransportCLI)
	if ctx.Mode() != PresentationPaired {
		t.Errorf("Mode() = %q, want %q", ctx.Mode(), PresentationPaired)
	}
}

func TestDetectedContext_String(t *testing.T) {
	tests := []struct {
		ctx  DetectedContext
		want string
	}{
		{DetectedUnknown, "unknown"},
		{DetectedTerminal, "terminal"},
		{DetectedAIHost, "ai-host"},
		{DetectedIDE, "ide"},
		{DetectedPaired, "paired"},
	}

	for _, tc := range tests {
		if got := tc.ctx.String(); got != tc.want {
			t.Errorf("DetectedContext(%q).String() = %q, want %q", tc.ctx, got, tc.want)
		}
	}
}

func TestTransport_String(t *testing.T) {
	tests := []struct {
		transport Transport
		want      string
	}{
		{TransportCLI, "cli"},
		{TransportTUI, "tui"},
		{TransportMCP, "mcp"},
		{TransportOther, "other"},
	}

	for _, tc := range tests {
		if got := tc.transport.String(); got != tc.want {
			t.Errorf("Transport(%q).String() = %q, want %q", tc.transport, got, tc.want)
		}
	}
}

func TestDetectContextFromEnvironment_ReturnsUnknown(t *testing.T) {
	// Detection is intentionally minimal - should return unknown
	detected := detectContextFromEnvironment()
	if detected != DetectedUnknown {
		t.Errorf("detectContextFromEnvironment() = %q, want %q (detection should be minimal)", detected, DetectedUnknown)
	}
}

func TestDefaultBehaviorPreserved_NoFlagMeansLegacy(t *testing.T) {
	// Critical test: no --presentation flag should preserve legacy behavior
	ctx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ExplicitRequest must be false when no flag provided
	if ctx.ExplicitRequest {
		t.Error("ExplicitRequest should be false when no flag provided")
	}

	// IsExplicit must return false
	if ctx.IsExplicit() {
		t.Error("IsExplicit() should return false when no flag provided")
	}

	// EffectiveMode should be default
	if ctx.EffectiveMode != DefaultPresentationMode {
		t.Errorf("EffectiveMode = %q, want %q", ctx.EffectiveMode, DefaultPresentationMode)
	}
}
