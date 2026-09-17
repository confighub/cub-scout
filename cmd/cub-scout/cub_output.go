// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
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

// cubUnitHasTarget reports whether `cub unit get -o json` output shows a unit
// with a target.
func cubUnitHasTarget(raw []byte) bool {
	var unit struct {
		Target *struct {
			TargetID string `json:"TargetID"`
			Slug     string `json:"Slug"`
		} `json:"Target"`
	}
	if err := json.Unmarshal(raw, &unit); err != nil || unit.Target == nil {
		return false
	}
	return unit.Target.TargetID != "" || unit.Target.Slug != ""
}

// firstKubernetesTargetSlug returns the slug of the first Kubernetes target in
// `cub target list -o json` output, or "" when there is none or the output is
// not a target list.
func firstKubernetesTargetSlug(raw []byte) string {
	var targets []struct {
		Target struct {
			Slug         string `json:"Slug"`
			ProviderType string `json:"ProviderType"`
		} `json:"Target"`
	}
	if err := json.Unmarshal(raw, &targets); err != nil {
		return ""
	}
	for _, target := range targets {
		if target.Target.ProviderType == "Kubernetes" && target.Target.Slug != "" {
			return target.Target.Slug
		}
	}
	return ""
}
