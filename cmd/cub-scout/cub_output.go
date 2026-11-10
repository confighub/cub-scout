// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// commandStdout runs a command and returns its stdout alone. Whatever it
// prints on stderr goes into the error, never into the data. cub prints
// notices there, such as a flag deprecation, and output read with
// CombinedOutput carries them into values that are then parsed, compared or
// written back.
func commandStdout(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
	}
	return out, err
}
