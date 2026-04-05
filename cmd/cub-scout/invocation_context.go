// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// InvocationContext captures metadata about how a command was invoked.
// This model enables auditability and UX evaluation while keeping
// detection advisory-only and preserving the read-only boundary.
//
// Key design principles:
// - Explicit requests always win over detected context
// - Detection is advisory only and does not change output behavior
// - JSON/MCP outputs remain unchanged regardless of context
// - Presentation framing is opt-in via explicit --presentation flag
type InvocationContext struct {
	// RequestedMode is what the caller explicitly asked for via --presentation.
	// Empty string means no explicit request was made.
	RequestedMode PresentationMode

	// DetectedContext is an advisory hint about the execution environment.
	// This does not affect output behavior in the current implementation.
	DetectedContext DetectedContext

	// EffectiveMode is the actual presentation mode used after resolution.
	// This accurately reflects what the renderer will do:
	// - PresentationLegacy when no --presentation flag was provided
	// - The requested mode (human/ai/paired) when explicitly requested
	// This is derived from RequestedMode and defaults, not from detection.
	EffectiveMode PresentationMode

	// Transport indicates how the command was invoked.
	Transport Transport

	// ExplicitRequest indicates whether presentation mode was explicitly
	// requested. When false, legacy/default formatting is used.
	ExplicitRequest bool
}

// DetectedContext represents an advisory hint about the execution environment.
// Detection is intentionally modest - most invocations resolve to Unknown.
type DetectedContext string

const (
	// DetectedUnknown is the default when no detection signal is available.
	DetectedUnknown DetectedContext = "unknown"

	// DetectedTerminal indicates a human terminal session.
	DetectedTerminal DetectedContext = "terminal"

	// DetectedAIHost indicates an AI assistant environment.
	DetectedAIHost DetectedContext = "ai-host"

	// DetectedIDE indicates an IDE or editor assistant context.
	DetectedIDE DetectedContext = "ide"

	// DetectedPaired indicates a human + AI paired workflow.
	DetectedPaired DetectedContext = "paired"
)

// String returns the string representation of the detected context.
func (d DetectedContext) String() string {
	return string(d)
}

// Transport indicates how the command was invoked.
type Transport string

const (
	// TransportCLI indicates command-line invocation.
	TransportCLI Transport = "cli"

	// TransportTUI indicates terminal UI invocation.
	TransportTUI Transport = "tui"

	// TransportMCP indicates MCP protocol invocation.
	TransportMCP Transport = "mcp"

	// TransportOther indicates unknown or other transport.
	TransportOther Transport = "other"
)

// String returns the string representation of the transport.
func (t Transport) String() string {
	return string(t)
}

// NewInvocationContext creates an InvocationContext with resolution applied.
// The presentationFlag is the raw value from --presentation (empty if not provided).
// The transport indicates how the command was invoked.
//
// Resolution rules:
//  1. If presentationFlag is non-empty, parse it as RequestedMode
//  2. ExplicitRequest is true only when presentationFlag was provided
//  3. EffectiveMode is RequestedMode if explicit, otherwise DefaultPresentationMode
//  4. DetectedContext is set via separate detection (advisory only)
func NewInvocationContext(presentationFlag string, transport Transport) (InvocationContext, error) {
	ctx := InvocationContext{
		Transport:       transport,
		DetectedContext: DetectedUnknown, // Detection is minimal in this slice
	}

	// Parse explicit request if provided
	if presentationFlag != "" {
		mode, err := ParsePresentationMode(presentationFlag)
		if err != nil {
			return InvocationContext{}, err
		}
		ctx.RequestedMode = mode
		ctx.ExplicitRequest = true
		ctx.EffectiveMode = mode
	} else {
		// No explicit request - use default, ExplicitRequest remains false
		ctx.EffectiveMode = DefaultPresentationMode
	}

	return ctx, nil
}

// NewInvocationContextWithDetection creates an InvocationContext with both
// explicit request parsing and advisory detection applied.
// Detection does not affect EffectiveMode - explicit requests and defaults win.
func NewInvocationContextWithDetection(presentationFlag string, transport Transport, detected DetectedContext) (InvocationContext, error) {
	ctx, err := NewInvocationContext(presentationFlag, transport)
	if err != nil {
		return InvocationContext{}, err
	}
	ctx.DetectedContext = detected
	// Note: detection does not change EffectiveMode in this implementation
	return ctx, nil
}

// IsExplicit returns true if presentation mode was explicitly requested.
// This is the primary signal for whether to apply presentation-specific framing.
func (c InvocationContext) IsExplicit() bool {
	return c.ExplicitRequest
}

// Mode returns the effective presentation mode to use for rendering.
func (c InvocationContext) Mode() PresentationMode {
	return c.EffectiveMode
}

// detectContextFromEnvironment performs minimal advisory detection.
// This is intentionally conservative - returns DetectedUnknown unless
// there is a clear local signal.
//
// Note: This function is provided for future use but detection is
// intentionally modest in this slice. Most call sites should use
// DetectedUnknown directly.
func detectContextFromEnvironment() DetectedContext {
	// Intentionally minimal detection in this slice.
	// Future implementations may check:
	// - TERM_PROGRAM for terminal identification
	// - CI environment variables
	// - MCP transport detection
	// - IDE-specific signals
	//
	// For now, return unknown to keep detection advisory and modest.
	return DetectedUnknown
}
