// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/confighub/cub-scout/v2/internal/hubread"
)

// This file is where a `cub` read is answered through the ConfigHub SDK
// instead of by starting `cub` (#758).
//
// It works at the level of the command cub-scout was about to run. Every read
// already goes through cubStdout with cub's own arguments, and its callers
// already parse cub's JSON. So the adapter recognises a command it can
// reproduce exactly, reads the same thing the way cub does, and returns the
// bytes cub would have printed. A caller cannot tell which route answered,
// which is the point: nothing downstream changes.
//
// A command is only taken when every argument is understood. Anything else,
// a flag this file does not know, a second positional, a selector, runs cub
// as before. Recognition is narrow on purpose; a near miss must not become a
// different question answered confidently.

// configHubReaderEnv selects how the reads this file reproduces reach
// ConfigHub. Unset or "cub" runs the cub CLI, as every release has. "sdk" uses
// the SDK's typed client through internal/hubread, with no cub process.
// Experimental.
const configHubReaderEnv = "CUB_SCOUT_CONFIGHUB_READER"

// configHubReaderRoute returns "cub" or "sdk". Any other value is an error:
// a misspelt route must not quietly become the default one.
func configHubReaderRoute() (string, error) {
	switch value := strings.ToLower(strings.TrimSpace(os.Getenv(configHubReaderEnv))); value {
	case "", "cub":
		return "cub", nil
	case "sdk":
		return "sdk", nil
	default:
		return "", fmt.Errorf("%s=%q is not a reader (valid: cub, sdk)", configHubReaderEnv, value)
	}
}

// sdkReader builds the reader for one read. It resolves the same credential
// cub would use, and reads; it does not log in or write.
var sdkReader = func(ctx context.Context) (*hubread.Reader, error) {
	return hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout"})
}

// sdkUnitGetArgs recognises `cub unit get <unit> -o json [--quiet] --space
// <space>` and `cub unit get <space>/<unit> -o json [--quiet]`, with the flags
// in any order, and nothing else.
func sdkUnitGetArgs(args []string) (space, unit string, ok bool) {
	if len(args) < 3 || args[0] != "unit" || args[1] != "get" {
		return "", "", false
	}
	var positionals []string
	jsonOutput, flagSpace, hasFlagSpace := false, "", false
	for i := 2; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) || args[i+1] != "json" {
				return "", "", false
			}
			jsonOutput = true
			i++
		case arg == "--space":
			// cub takes the next argument as the value even when it looks
			// like a flag. That is more likely a mistake than a space.
			if i+1 >= len(args) || hasFlagSpace || strings.HasPrefix(args[i+1], "-") {
				return "", "", false
			}
			flagSpace, hasFlagSpace = strings.TrimSpace(args[i+1]), true
			i++
		case arg == "--quiet":
			// With -o json, cub prints the payload alone either way.
		case strings.HasPrefix(arg, "-"):
			return "", "", false
		default:
			positionals = append(positionals, arg)
		}
	}
	if !jsonOutput || len(positionals) != 1 {
		return "", "", false
	}
	ref := strings.TrimSpace(positionals[0])
	refSpace, refUnit, qualified := strings.Cut(ref, "/")
	switch {
	case qualified && hasFlagSpace:
		// Two spaces named. cub takes the reference's; rather than
		// encode that precedence here, leave the command to cub.
		return "", "", false
	case qualified:
		space, unit = strings.TrimSpace(refSpace), strings.TrimSpace(refUnit)
	default:
		space, unit = flagSpace, ref
	}
	if space == "" || unit == "" || strings.Contains(unit, "/") || strings.Contains(space, "/") {
		return "", "", false
	}
	// "*" asks cub for every space, and is no Unit's slug.
	if space == allConfigHubSpaces || unit == allConfigHubSpaces {
		return "", "", false
	}
	// cub reads a name as an ID in every spelling uuid.Parse accepts: the
	// canonical form, 32 bare hex digits, braces, a urn: prefix. A Unit
	// named by ID is found whatever the space, so it stays with cub. The
	// reader takes a space by ID in the canonical form only and would look
	// the other spellings up as slugs, so those stay with cub too.
	if spelledAsID(unit) || (spelledAsID(space) && !canonicalID(space)) {
		return "", "", false
	}
	return space, unit, true
}

func spelledAsID(name string) bool {
	_, err := uuid.Parse(name)
	return err == nil
}

func canonicalID(name string) bool {
	id, err := uuid.Parse(name)
	return err == nil && strings.EqualFold(id.String(), name)
}

// sdkRouteError marks a failure of a read that the SDK route took: a failed
// read, a credential that could not be resolved, a refusal, or a route setting
// that names no route. In every case no cub process ran, so a caller that
// words its failure as "cub ... failed" checks for this first.
type sdkRouteError struct{ err error }

func (e *sdkRouteError) Error() string { return e.err.Error() }
func (e *sdkRouteError) Unwrap() error { return e.err }

// failedOnSDKRoute reports whether err came from a read the SDK route answered.
func failedOnSDKRoute(err error) bool {
	var routed *sdkRouteError
	return errors.As(err, &routed)
}

// cubReadViaSDK answers a cub read through the SDK when the SDK route is
// selected and the command is one this file reproduces. handled is false when
// cub should run. When handled is true the result is final: a failed SDK read
// is returned as its error and cub is not tried, because a silent retry would
// hide that the route asked for did not work.
func cubReadViaSDK(ctx context.Context, args []string) (out []byte, handled bool, err error) {
	space, unit, ok := sdkUnitGetArgs(args)
	if !ok {
		return nil, false, nil
	}
	route, err := configHubReaderRoute()
	if err != nil {
		return nil, true, &sdkRouteError{err}
	}
	if route != "sdk" {
		return nil, false, nil
	}
	// The same refusals as the cub route: a call that forgot its space is
	// refused here too, before anything is resolved or sent.
	if err := checkCubArgs(args); err != nil {
		return nil, true, &sdkRouteError{err}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reader, err := sdkReader(ctx)
	if err != nil {
		return nil, true, &sdkRouteError{err}
	}
	out, err = reader.UnitJSON(ctx, space, unit)
	if err != nil {
		return nil, true, &sdkRouteError{err}
	}
	return out, true, nil
}
