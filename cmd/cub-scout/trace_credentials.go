// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/rest"
)

// captureTraceCredentialFiles resolves file-backed credentials once, before any
// session client is constructed. Later navigation cannot silently adopt edited
// CA, client identity or token files. A new invocation captures rotated files.
// Exec authentication retains its configured refresh behavior; this does not
// sandbox an exec plugin or snapshot files that the plugin itself chooses to read.
func captureTraceCredentialFiles(config *rest.Config) error {
	files := []struct {
		label string
		path  *string
		data  *[]byte
	}{
		{"CA", &config.CAFile, &config.CAData},
		{"client certificate", &config.CertFile, &config.CertData},
		{"client key", &config.KeyFile, &config.KeyData},
	}
	for _, file := range files {
		if len(*file.data) == 0 && *file.path != "" {
			data, err := os.ReadFile(*file.path)
			if err != nil {
				// File paths and contents may be sensitive. Do not include either.
				return fmt.Errorf("capture trace %s file: unable to read configured file", file.label)
			}
			*file.data = data
		}
		*file.path = ""
	}
	if config.BearerTokenFile != "" {
		data, err := os.ReadFile(config.BearerTokenFile)
		if err != nil {
			return fmt.Errorf("capture trace bearer token file: unable to read configured file")
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return fmt.Errorf("capture trace bearer token file: configured file is empty")
		}
		config.BearerToken = token
		config.BearerTokenFile = ""
	}
	return nil
}
