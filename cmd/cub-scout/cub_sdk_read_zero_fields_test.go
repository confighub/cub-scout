// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// zeroFieldsNoLongerPrinted are fields the cub that made the recordings
// (v0.8.x) printed when they were zero, and SDK core v0.10.11 leaves out. The
// same table is in internal/hubread's tests.
var zeroFieldsNoLongerPrinted = map[string]interface{}{
	"BoundLinkID":             "00000000-0000-0000-0000-000000000000",
	"AdoptedEndTagID":         "00000000-0000-0000-0000-000000000000",
	"RestoreTagID":            "00000000-0000-0000-0000-000000000000",
	"ExpiresAt":               "0001-01-01T00:00:00Z",
	"TriggerCountByEventType": map[string]interface{}{},
}

// sameByEitherRoute compares what the SDK route returned with what the fake
// cub printed, which is a recording made by cub v0.8.x: byte for byte where
// that still holds, and otherwise value for value, allowing only that a named
// field which is zero in the recording is absent by the SDK route. Nothing may
// be added and nothing else may differ. Bytes are compared against a cub of the
// SDK's own version in the Connected lane.
func sameByEitherRoute(t *testing.T, viaCub, viaSDK []byte) {
	t.Helper()
	if string(viaCub) == string(viaSDK) {
		return
	}
	var want, have interface{}
	require.NoError(t, json.Unmarshal(viaCub, &want))
	require.NoError(t, json.Unmarshal(viaSDK, &have))
	dropped := 0
	var strip func(value interface{}) interface{}
	strip = func(value interface{}) interface{} {
		switch typed := value.(type) {
		case map[string]interface{}:
			for field, zero := range zeroFieldsNoLongerPrinted {
				if found, ok := typed[field]; ok && reflect.DeepEqual(found, zero) {
					delete(typed, field)
					dropped++
				}
			}
			for key, inner := range typed {
				typed[key] = strip(inner)
			}
		case []interface{}:
			for i, inner := range typed {
				typed[i] = strip(inner)
			}
		}
		return value
	}
	want = strip(want)
	require.Positive(t, dropped, "the routes differ and no zero field accounts for it:\nby cub: %s\nby the SDK: %s", viaCub, viaSDK)
	require.Equal(t, want, have)
}
