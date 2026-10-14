package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// maxChanged bounds the changed lines that a stale check prints.
const maxChanged = 10

// fields is a decoded JSON object whose values stay raw until the walk needs them.
type fields map[string]json.RawMessage

// changedLines names the leaves that differ between two reports, without meta, and bounds the list at maxChanged.
func changedLines(a, b []byte) []string {
	var left, right fields
	_ = json.Unmarshal(a, &left)
	_ = json.Unmarshal(b, &right)
	delete(left, "meta")
	delete(right, "meta")
	var lines []string
	diffFields(left, right, "", &lines)
	sort.SliceStable(lines, func(i, j int) bool { return lineRank(lines[i]) < lineRank(lines[j]) })
	if len(lines) > maxChanged {
		more := len(lines) - maxChanged
		lines = append(lines[:maxChanged], fmt.Sprintf("+%d more", more))
	}
	return lines
}

// diffValue appends one changed line per differing leaf, and one line for an array that differs.
func diffValue(a, b json.RawMessage, path string, out *[]string) {
	if bytes.Equal(a, b) {
		return
	}
	var left, right fields
	if json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && left != nil && right != nil {
		diffFields(left, right, path, out)
		return
	}
	var leftItems, rightItems []json.RawMessage
	if json.Unmarshal(a, &leftItems) == nil && json.Unmarshal(b, &rightItems) == nil && leftItems != nil && rightItems != nil {
		*out = append(*out, arrayLine(path, len(leftItems), len(rightItems)))
		return
	}
	*out = append(*out, fmt.Sprintf("changed: %s %s to %s", path, compact(a), compact(b)))
}

func arrayLine(path string, before, after int) string {
	if before == after {
		return "changed: " + path
	}
	return fmt.Sprintf("changed: %s %d to %d items", path, before, after)
}

func diffFields(a, b fields, path string, out *[]string) {
	keys := make([]string, 0, len(a)+len(b))
	for key := range a {
		keys = append(keys, key)
	}
	for key := range b {
		if _, ok := a[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := key
		if path != "" {
			child = path + "." + key
		}
		diffValue(a[key], b[key], child, out)
	}
}

// compact prints a raw value on one line, and a missing value as null.
func compact(v json.RawMessage) string {
	if len(v) == 0 {
		return "null"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, v); err != nil {
		return string(v)
	}
	return buf.String()
}

// lineRank puts the score lines first and the findings lines second, so the cap never hides them.
func lineRank(line string) int {
	switch path := strings.Fields(line)[1]; {
	case path == "score" || strings.HasPrefix(path, "score."):
		return 0
	case path == "findings":
		return 1
	}
	return 2
}
