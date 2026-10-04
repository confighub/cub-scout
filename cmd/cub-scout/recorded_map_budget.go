// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"math"
)

const recordedMapMaxReportJSONBytes = 4 << 20

// This limit covers canonical report data only, never rendered terminal text,
// duplicated MCP content, protocol envelopes, tokens or input-processing work.
func validateRecordedMapJSONBudget(limit int) error {
	if limit < 1 || limit > recordedMapMaxReportJSONBytes {
		return fmt.Errorf("max report JSON bytes must be between 1 and %d", recordedMapMaxReportJSONBytes)
	}
	return nil
}
func recordedMapJSONBudgetArgument(value interface{}) (int, error) {
	var number float64
	switch v := value.(type) {
	case int:
		number = float64(v)
	case float64:
		number = v
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("max_report_json_bytes must be an integer between 1 and %d", recordedMapMaxReportJSONBytes)
		}
		number = parsed
	default:
		return 0, fmt.Errorf("max_report_json_bytes must be an integer between 1 and %d", recordedMapMaxReportJSONBytes)
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < 1 || number > recordedMapMaxReportJSONBytes {
		return 0, fmt.Errorf("max_report_json_bytes must be an integer between 1 and %d", recordedMapMaxReportJSONBytes)
	}
	return int(number), nil
}
func checkRecordedMapJSONBudgetBytes(raw []byte, limit int) error {
	if limit == 0 {
		return nil
	}
	if err := validateRecordedMapJSONBudget(limit); err != nil {
		return err
	}
	if len(raw) > limit {
		return fmt.Errorf("recorded report JSON requires %d bytes; budget is %d; reduce page size or request summary (report data only, not display or transport bytes)", len(raw), limit)
	}
	return nil
}
func checkRecordedMapJSONBudget(report interface{}, limit int) error {
	if limit == 0 {
		return nil
	} // No additional serialization in the default path.
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return checkRecordedMapJSONBudgetBytes(raw, limit)
}
