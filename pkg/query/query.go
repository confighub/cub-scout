// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package query provides a simple query language for filtering MapEntry resources.
//
// Query Syntax:
//
//	field=value           Exact match
//	field!=value          Not equal
//	field~=pattern        Regex match
//	field=value1,value2   IN list (comma-separated)
//
// Operators:
//
//	AND                   Both conditions must match (default)
//	OR                    Either condition must match
//
// Fields:
//
//	kind                  Resource kind (Deployment, Service, etc.)
//	namespace             Kubernetes namespace
//	name                  Resource name
//	owner                 Owner type (Flux, Argo, Helm, ConfigHub, Native)
//	cluster               Cluster name
//	labels[key]           Label value for given key
//
// Examples:
//
//	kind=Deployment
//	kind=Deployment AND namespace=production
//	owner=Flux OR owner=Argo
//	namespace=prod-* AND owner!=Native
//	labels[app]=nginx
package query

import (
	"fmt"
	"regexp"
	"strings"
)

// Operator represents a logical operator between conditions
type Operator string

const (
	OpAnd Operator = "AND"
	OpOr  Operator = "OR"
)

// Comparator represents how to compare field values
type Comparator string

const (
	CmpEqual    Comparator = "="
	CmpNotEqual Comparator = "!="
	CmpRegex    Comparator = "~="
	CmpIn       Comparator = "IN"
)

// Condition represents a single query condition
type Condition struct {
	Field      string     // Field name (kind, namespace, name, owner, cluster, labels[key])
	Comparator Comparator // How to compare
	Value      string     // Value to compare against
	Values     []string   // For IN comparator
	Regex      *regexp.Regexp // Compiled regex for ~= comparator
}

// Query represents a parsed query with conditions and operators
type Query struct {
	Conditions []Condition
	Operators  []Operator // Operators between conditions (len = len(Conditions) - 1)
}

// Parse parses a query string into a Query struct
func Parse(input string) (*Query, error) {
	if input == "" {
		return &Query{}, nil
	}

	q := &Query{
		Conditions: []Condition{},
		Operators:  []Operator{},
	}

	// Tokenize by AND/OR while preserving them
	tokens := tokenize(input)

	for i, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		upper := strings.ToUpper(token)
		if upper == "AND" || upper == "OR" {
			if len(q.Conditions) == 0 {
				return nil, fmt.Errorf("operator %s without preceding condition", token)
			}
			q.Operators = append(q.Operators, Operator(upper))
			continue
		}

		// Parse condition
		cond, err := parseCondition(token)
		if err != nil {
			return nil, fmt.Errorf("invalid condition at position %d: %w", i, err)
		}
		q.Conditions = append(q.Conditions, cond)
	}

	// Validate: operators should be one less than conditions
	if len(q.Conditions) > 0 && len(q.Operators) != len(q.Conditions)-1 {
		// Fill missing operators with AND (default)
		for len(q.Operators) < len(q.Conditions)-1 {
			q.Operators = append(q.Operators, OpAnd)
		}
	}

	return q, nil
}

// tokenize splits the query string into tokens (conditions and operators)
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder

	words := strings.Fields(input)
	for _, word := range words {
		upper := strings.ToUpper(word)
		if upper == "AND" || upper == "OR" {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, upper)
		} else {
			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(word)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// parseCondition parses a single condition like "field=value"
func parseCondition(s string) (Condition, error) {
	// Check for regex match first (~=)
	if idx := strings.Index(s, "~="); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := strings.TrimSpace(s[idx+2:])
		re, err := regexp.Compile(value)
		if err != nil {
			return Condition{}, fmt.Errorf("invalid regex %q: %w", value, err)
		}
		return Condition{
			Field:      field,
			Comparator: CmpRegex,
			Value:      value,
			Regex:      re,
		}, nil
	}

	// Check for not equal (!=)
	if idx := strings.Index(s, "!="); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := strings.TrimSpace(s[idx+2:])
		return Condition{
			Field:      field,
			Comparator: CmpNotEqual,
			Value:      value,
		}, nil
	}

	// Check for equal (=)
	if idx := strings.Index(s, "="); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := strings.TrimSpace(s[idx+1:])

		// Check if it's a comma-separated list (IN)
		if strings.Contains(value, ",") {
			values := strings.Split(value, ",")
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
			return Condition{
				Field:      field,
				Comparator: CmpIn,
				Values:     values,
			}, nil
		}

		return Condition{
			Field:      field,
			Comparator: CmpEqual,
			Value:      value,
		}, nil
	}

	return Condition{}, fmt.Errorf("invalid condition syntax: %q (expected field=value)", s)
}

// Matchable is the interface that MapEntry must implement for query matching
type Matchable interface {
	GetField(field string) (string, bool)
}

// Matches evaluates the query against a Matchable entry
func (q *Query) Matches(entry Matchable) bool {
	if len(q.Conditions) == 0 {
		return true
	}

	// Evaluate first condition
	result := q.evalCondition(q.Conditions[0], entry)

	// Apply operators with subsequent conditions
	for i, op := range q.Operators {
		nextResult := q.evalCondition(q.Conditions[i+1], entry)
		switch op {
		case OpAnd:
			result = result && nextResult
		case OpOr:
			result = result || nextResult
		}
	}

	return result
}

// evalCondition evaluates a single condition against an entry
func (q *Query) evalCondition(cond Condition, entry Matchable) bool {
	value, exists := entry.GetField(cond.Field)

	switch cond.Comparator {
	case CmpEqual:
		if !exists {
			return false
		}
		// Support wildcard matching with *
		if strings.Contains(cond.Value, "*") {
			pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(cond.Value), `\*`, ".*") + "$"
			re, err := regexp.Compile(pattern)
			if err != nil {
				return value == cond.Value
			}
			return re.MatchString(value)
		}
		return strings.EqualFold(value, cond.Value)

	case CmpNotEqual:
		if !exists {
			return true // Non-existent field is not equal to anything
		}
		return !strings.EqualFold(value, cond.Value)

	case CmpRegex:
		if !exists {
			return false
		}
		return cond.Regex.MatchString(value)

	case CmpIn:
		if !exists {
			return false
		}
		for _, v := range cond.Values {
			if strings.EqualFold(value, v) {
				return true
			}
		}
		return false
	}

	return false
}

// String returns the query as a string representation
func (q *Query) String() string {
	if len(q.Conditions) == 0 {
		return ""
	}

	var parts []string
	for i, cond := range q.Conditions {
		parts = append(parts, cond.String())
		if i < len(q.Operators) {
			parts = append(parts, string(q.Operators[i]))
		}
	}
	return strings.Join(parts, " ")
}

// String returns the condition as a string representation
func (c Condition) String() string {
	switch c.Comparator {
	case CmpIn:
		return fmt.Sprintf("%s=%s", c.Field, strings.Join(c.Values, ","))
	case CmpRegex:
		return fmt.Sprintf("%s~=%s", c.Field, c.Value)
	case CmpNotEqual:
		return fmt.Sprintf("%s!=%s", c.Field, c.Value)
	default:
		return fmt.Sprintf("%s=%s", c.Field, c.Value)
	}
}
