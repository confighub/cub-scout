// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const recordedMapPageSchema = "map-list-recorded-page.v1"

type RecordedMapPagination struct {
	PageSize      int    `json:"pageSize"`
	Offset        int    `json:"offset"`
	ReturnedCount int    `json:"returnedCount"`
	NextCursor    string `json:"nextCursor,omitempty"`
}

type recordedMapCursor struct {
	Version     int    `json:"version"`
	InputSHA256 string `json:"inputSHA256"`
	ScopeSHA256 string `json:"scopeSHA256"`
	PageSize    int    `json:"pageSize"`
	Offset      int    `json:"offset"`
}

func recordedMapScopeDigest(scope RecordedMapScope) string {
	raw, _ := json.Marshal(scope)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// Cursor is public continuation data, not an authorization token. Canonical
// bytes refuse ambiguous casing, duplicate/unknown fields and trailing data.
func decodeRecordedMapCursor(value string) (recordedMapCursor, error) {
	var cursor recordedMapCursor
	if len(value) > 2048 {
		return cursor, fmt.Errorf("recorded cursor is too large")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return cursor, fmt.Errorf("invalid recorded cursor")
	}
	if err = json.Unmarshal(raw, &cursor); err != nil {
		return cursor, fmt.Errorf("invalid recorded cursor")
	}
	canonical, _ := json.Marshal(cursor)
	if !bytes.Equal(raw, canonical) {
		return cursor, fmt.Errorf("recorded cursor must use canonical fields")
	}
	return cursor, nil
}

func encodeRecordedMapCursor(report RecordedMapReport, size, offset int) string {
	cursor := recordedMapCursor{Version: 1, InputSHA256: report.Provenance.SHA256, ScopeSHA256: recordedMapScopeDigest(report.Scope), PageSize: size, Offset: offset}
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func validateRecordedMapPageOptions(size int, cursor string, summary bool) error {
	if size < 1 || size > 500 {
		return fmt.Errorf("recorded page size must be between 1 and 500")
	}
	if summary {
		return fmt.Errorf("recorded pagination cannot be combined with summary")
	}
	if cursor != "" {
		_, err := decodeRecordedMapCursor(cursor)
		return err
	}
	return nil
}

func buildRecordedMapPage(report RecordedMapReport, size int, value string) (RecordedMapReport, error) {
	if err := validateRecordedMapPageOptions(size, value, false); err != nil {
		return RecordedMapReport{}, err
	}
	if report.Schema != recordedMapSchema || report.Pagination != nil || report.SelectedCount != len(report.Resources) {
		return RecordedMapReport{}, fmt.Errorf("pagination requires a complete recorded report")
	}
	if err := validateRecordedMapScope(report.Scope); err != nil {
		return RecordedMapReport{}, err
	}
	rawSHA, err := hex.DecodeString(report.Provenance.SHA256)
	if err != nil || len(rawSHA) != sha256.Size {
		return RecordedMapReport{}, fmt.Errorf("recorded input digest is unavailable")
	}
	offset := 0
	if value != "" {
		cursor, err := decodeRecordedMapCursor(value)
		if err != nil {
			return RecordedMapReport{}, err
		}
		if cursor.Version != 1 || cursor.InputSHA256 != report.Provenance.SHA256 || cursor.ScopeSHA256 != recordedMapScopeDigest(report.Scope) || cursor.PageSize != size {
			return RecordedMapReport{}, fmt.Errorf("recorded cursor does not match recording, scope, or page size")
		}
		if cursor.Offset <= 0 || cursor.Offset >= len(report.Resources) || cursor.Offset%size != 0 {
			return RecordedMapReport{}, fmt.Errorf("recorded cursor offset is outside page boundaries")
		}
		offset = cursor.Offset
	}
	end := min(offset+size, len(report.Resources))
	page := report
	page.Schema = recordedMapPageSchema
	page.Resources = append([]RecordedMapResource{}, report.Resources[offset:end]...)
	page.Pagination = &RecordedMapPagination{PageSize: size, Offset: offset, ReturnedCount: end - offset}
	if end < len(report.Resources) {
		page.Pagination.NextCursor = encodeRecordedMapCursor(report, size, end)
	}
	return page, nil
}

func recordedMapPageSizeArgument(value interface{}) (int, error) {
	// MCP JSON numbers arrive as float64; internal callers may supply int or
	// json.Number. Never truncate fractional values or convert before bounds.
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case int:
		number = float64(v)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("page_size must be an integer between 1 and 500")
		}
		number = parsed
	default:
		return 0, fmt.Errorf("page_size must be an integer between 1 and 500")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 1 || number > 500 || math.Trunc(number) != number {
		return 0, fmt.Errorf("page_size must be an integer between 1 and 500")
	}
	return int(number), nil
}

type recordedMapPager struct {
	Report RecordedMapReport
	Page   RecordedMapReport
	Size   int
	Cursor string
}

func newRecordedMapPagedViewer(report RecordedMapReport, size int, cursor string) (recordedMapViewer, error) {
	page, err := buildRecordedMapPage(report, size, cursor)
	if err != nil {
		return recordedMapViewer{}, err
	}
	viewer := newRecordedMapViewer(page)
	viewer.pager = &recordedMapPager{Report: report, Page: page, Size: size, Cursor: cursor}
	return viewer, nil
}

func (m recordedMapViewer) updateRecordedPage(key string) recordedMapViewer {
	pager := *m.pager
	var cursor string
	if key == "n" {
		if pager.Page.Pagination.NextCursor == "" {
			return m
		}
		cursor = pager.Page.Pagination.NextCursor
	} else {
		if pager.Page.Pagination.Offset == 0 {
			return m
		}
		previous := pager.Page.Pagination.Offset - pager.Size
		cursor = ""
		if previous > 0 {
			cursor = encodeRecordedMapCursor(pager.Report, pager.Size, previous)
		}
	}
	page, err := buildRecordedMapPage(pager.Report, pager.Size, cursor)
	if err != nil {
		return m
	} // Pure loaded model; constructed cursors always match.
	pager.Cursor, pager.Page = cursor, page
	m.pager = &pager
	m.content = renderRecordedMapReport(page, "ascii")
	m.viewport.SetContent(wrapRecordedExplainText(m.content, m.viewport.Width))
	m.viewport.GotoTop()
	return m
}
