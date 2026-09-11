// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Generates a local unsigned test bundle; does not contact a registry or cluster.
package main

import (
	"fmt"
	"os"

	"github.com/confighub/cub-scout/internal/releasetest"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: create-layout <literal.yaml> <new-layout-dir>")
		os.Exit(1)
	}
	if _, err := os.Stat(os.Args[2]); !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "layout directory must not exist")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ref, err := releasetest.WriteLayout(os.Args[2], map[string][]byte{"objects.yaml": data})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(ref)
}
