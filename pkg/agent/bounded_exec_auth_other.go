// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package agent

import (
	"os"
	osexec "os/exec"
)

func prepareBoundedExecCommand(*osexec.Cmd) {}

func cancelBoundedExecCommand(cmd *osexec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
