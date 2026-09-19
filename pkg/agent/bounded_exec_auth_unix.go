// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package agent

import (
	"os"
	osexec "os/exec"
	"syscall"
)

func prepareBoundedExecCommand(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func cancelBoundedExecCommand(cmd *osexec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	// The helper and its descendants share a process group. Killing the group
	// prevents a descendant that inherited stdout from outliving Cmd.Wait.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return cmd.Process.Kill()
	}
	return nil
}
