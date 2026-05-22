// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// exit_code.go — typed exit-code signaling for cobra RunE.
//
// Cobra's default behavior on RunE error is to print + exit non-zero,
// but it uses a single hard-coded exit code (1 in our main.go
// dispatcher). Some commands have richer exit semantics — notably
// `receipt validate`, which the docs promise will exit:
//
//   0 — fingerprint matches
//   1 — fingerprint mismatch (tampering or corruption)
//   2 — I/O or parse error
//
// To honor that contract, a RunE can return an exitCodeError wrapping
// the underlying error and the desired exit code. main.go unwraps
// these via errors.As and exits with the right code.
//
// Other commands that need richer exit semantics (compare three-way
// --fail-on, for instance) can adopt the same pattern.

// exitCodeError signals a non-default process exit code from a cobra
// RunE. The wrapped err is what cobra / main.go print to stderr; the
// code is what os.Exit gets called with.
type exitCodeError struct {
	err  error
	code int
}

func (e *exitCodeError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// ExitCode returns the desired process exit code. main.go matches on
// this via the unexported interface `{ ExitCode() int }`.
func (e *exitCodeError) ExitCode() int {
	if e == nil {
		return 0
	}
	return e.code
}

// newExitCodeError wraps err with a desired exit code. Returns nil if
// err is nil (so callers can compose without nil-checks).
func newExitCodeError(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{err: err, code: code}
}
