// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package agent

import "syscall"

func boundedExecProcessExists(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
