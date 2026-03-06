// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestResolveImportConnectionMode(t *testing.T) {
	tests := []struct {
		name              string
		importYes         bool
		importConnect     bool
		importNoConnect   bool
		wantShouldConnect bool
		wantShowHint      bool
	}{
		{
			name:              "interactive defaults to connect",
			importYes:         false,
			importConnect:     false,
			importNoConnect:   false,
			wantShouldConnect: true,
			wantShowHint:      false,
		},
		{
			name:              "interactive explicit no-connect",
			importYes:         false,
			importConnect:     false,
			importNoConnect:   true,
			wantShouldConnect: false,
			wantShowHint:      false,
		},
		{
			name:              "non-interactive keeps legacy no-connect and shows hint",
			importYes:         true,
			importConnect:     false,
			importNoConnect:   false,
			wantShouldConnect: false,
			wantShowHint:      true,
		},
		{
			name:              "non-interactive with explicit connect",
			importYes:         true,
			importConnect:     true,
			importNoConnect:   false,
			wantShouldConnect: true,
			wantShowHint:      false,
		},
		{
			name:              "non-interactive explicit no-connect",
			importYes:         true,
			importConnect:     false,
			importNoConnect:   true,
			wantShouldConnect: false,
			wantShowHint:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShouldConnect, gotShowHint := resolveImportConnectionMode(tt.importYes, tt.importConnect, tt.importNoConnect)
			if gotShouldConnect != tt.wantShouldConnect {
				t.Fatalf("shouldConnect = %v, want %v", gotShouldConnect, tt.wantShouldConnect)
			}
			if gotShowHint != tt.wantShowHint {
				t.Fatalf("showHint = %v, want %v", gotShowHint, tt.wantShowHint)
			}
		})
	}
}
