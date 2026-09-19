// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build windows

package agent

func boundedExecProcessExists(int) bool { return false }
