// Package compare compares two saved reports, as spec 003 "Comparison" defines.
package compare

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// Comparison is the result of Compare.
// A refused comparison has summaries and a refusal, and no deltas or finding lists.
type Comparison struct {
	Comparable bool               `json:"comparable"`
	Refusal    string             `json:"refusal,omitempty"`
	Before     Summary            `json:"before"`
	After      Summary            `json:"after"`
	IndexDelta int                `json:"indexDelta"`
	Metrics    []MetricDelta      `json:"metrics"`
	New        []contract.Finding `json:"new"`
	Resolved   []contract.Finding `json:"resolved"`
	Persistent []contract.Finding `json:"persistent"`
	Changed    []Changed          `json:"changed"`
}

// MarshalJSON omits the index delta of a refused comparison, because a refused comparison computes no delta.
func (c Comparison) MarshalJSON() ([]byte, error) {
	out := struct {
		Comparable bool               `json:"comparable"`
		Refusal    string             `json:"refusal,omitempty"`
		Before     Summary            `json:"before"`
		After      Summary            `json:"after"`
		IndexDelta *int               `json:"indexDelta,omitempty"`
		Metrics    []MetricDelta      `json:"metrics"`
		New        []contract.Finding `json:"new"`
		Resolved   []contract.Finding `json:"resolved"`
		Persistent []contract.Finding `json:"persistent"`
		Changed    []Changed          `json:"changed"`
	}{c.Comparable, c.Refusal, c.Before, c.After, nil, c.Metrics, c.New, c.Resolved, c.Persistent, c.Changed}
	if c.Comparable {
		out.IndexDelta = &c.IndexDelta
	}
	return json.Marshal(out)
}

// Summary names one report of a comparison by its index and the versions that decide comparability.
type Summary struct {
	Index           int    `json:"index"`
	Partial         bool   `json:"partial"`
	AnalyzerVersion string `json:"analyzerVersion"`
	ScoringVersion  string `json:"scoringVersion"`
	SchemaVersion   string `json:"schemaVersion"`
	ConfigDigest    string `json:"configDigest"`
}

// MetricDelta is one metric in both reports.
// A metric absent from one report is nil on that side.
type MetricDelta struct {
	ID     string           `json:"id"`
	Before *contract.Metric `json:"before,omitempty"`
	After  *contract.Metric `json:"after,omitempty"`
	// Delta is nil unless the metric is complete in both reports.
	Delta *float64 `json:"delta"`
}

// Compare compares before with after, and refuses when the versions or configuration digests differ.
func Compare(before, after *report.Report) *Comparison {
	c := &Comparison{Before: summarize(before), After: summarize(after)}
	if refusal := refuse(c.Before, c.After); refusal != "" {
		c.Refusal = refusal
		return c
	}
	c.Comparable = true
	c.IndexDelta = c.After.Index - c.Before.Index
	c.Metrics = metricDeltas(before.Metrics, after.Metrics)
	c.New, c.Resolved, c.Persistent, c.Changed = matchFindings(before.Findings, after.Findings)
	return c
}

func summarize(r *report.Report) Summary {
	return Summary{
		Index: r.Score.Index, Partial: r.Score.Partial,
		AnalyzerVersion: r.AnalyzerVersion, ScoringVersion: r.ScoringVersion,
		SchemaVersion: r.SchemaVersion, ConfigDigest: r.ConfigDigest,
	}
}

// refuse returns a reason that names the first differing field, or "" when the reports are comparable.
func refuse(before, after Summary) string {
	fields := []struct{ name, before, after string }{
		{"schemaVersion", before.SchemaVersion, after.SchemaVersion},
		{"analyzerVersion", before.AnalyzerVersion, after.AnalyzerVersion},
		{"scoringVersion", before.ScoringVersion, after.ScoringVersion},
		{"configDigest", before.ConfigDigest, after.ConfigDigest},
	}
	for _, f := range fields {
		if f.before != f.after {
			return fmt.Sprintf("%s differs: before %q, after %q", f.name, f.before, f.after)
		}
	}
	return ""
}

func metricDeltas(before, after map[string]contract.Metric) []MetricDelta {
	ids := map[string]bool{}
	for id := range before {
		ids[id] = true
	}
	for id := range after {
		ids[id] = true
	}
	out := make([]MetricDelta, 0, len(ids))
	for id := range ids {
		d := MetricDelta{ID: id, Before: lookup(before, id), After: lookup(after, id)}
		if complete(d.Before) && complete(d.After) {
			delta := d.After.Value - d.Before.Value
			d.Delta = &delta
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func lookup(metrics map[string]contract.Metric, id string) *contract.Metric {
	m, ok := metrics[id]
	if !ok {
		return nil
	}
	return &m
}

func complete(m *contract.Metric) bool {
	return m != nil && m.State == contract.Complete
}
