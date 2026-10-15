// Package report holds the audit result and prints it.
package report

import (
	"sort"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/formula"
	"github.com/JacobJNilsson/godebt/internal/inventory"
)

// SchemaVersion is the version of the report shape of spec 003.
const SchemaVersion = "1.1.0"

// Completeness says whether every production metric was measured.
type Completeness string

// The completeness values of spec 003.
const (
	Complete   Completeness = "complete"
	Incomplete Completeness = "incomplete"
)

// Report is the result of one audit, in the shape of spec 003.
type Report struct {
	SchemaVersion   string                     `json:"schemaVersion"`
	AnalyzerVersion string                     `json:"analyzerVersion"`
	ScoringVersion  string                     `json:"scoringVersion"`
	Repo            Repo                       `json:"repo"`
	ConfigDigest    string                     `json:"configDigest"`
	Coverage        Coverage                   `json:"coverage"`
	Completeness    Completeness               `json:"completeness"`
	Metrics         map[string]contract.Metric `json:"metrics"`
	Score           contract.Score             `json:"score"`
	TestScore       contract.Score             `json:"testScore"`
	Findings        []contract.Finding         `json:"findings"`
	Safeguards      []contract.Safeguard       `json:"safeguards"`
	Limits          []contract.Limit           `json:"limits"`
	Meta            Meta                       `json:"meta"`
	// Dir is the directory whose findings the renderers print, and "" means the whole module. It never reaches the JSON.
	Dir string `json:"-"`
}

// Meta holds the facts of one run that two equal audits may not share.
type Meta struct {
	DurationMs int64 `json:"durationMs"`
}

// Run names what produced a report besides the inventory.
type Run struct {
	AnalyzerVersion string
	Config          contract.Config
}

// Repo names the audited root.
type Repo struct {
	Root   string `json:"root"`
	Module string `json:"module"`
}

// Measured counts a set that the audit parses.
type Measured struct {
	Files int `json:"files"`
	SLOC  int `json:"sloc"`
}

// Counted counts a set that the audit does not parse.
type Counted struct {
	Files int `json:"files"`
}

// Coverage has the shape of the coverage block in spec 003.
type Coverage struct {
	Production    Measured `json:"production"`
	Test          Measured `json:"test"`
	Generated     Counted  `json:"generated"`
	Vendored      Counted  `json:"vendored"`
	Testdata      Counted  `json:"testdata"`
	Excluded      Counted  `json:"excluded"`
	Unsupported   Counted  `json:"unsupported"`
	NestedModules []string `json:"nestedModules"`
}

// New builds the report of an inventory, its metrics, findings, limits, and safeguards, sorts the limits by metric id, and leaves Meta zero.
func New(inv *inventory.Inventory, metrics map[string]contract.Metric, findings []contract.Finding, limits []contract.Limit, guards []contract.Safeguard, run Run) *Report {
	tree := inv.Tree
	measured := func(set discover.SourceSet) Measured {
		return Measured{Files: tree.Coverage[set].Files, SLOC: inv.SLOC[set]}
	}
	counted := func(set discover.SourceSet) Counted { return Counted{Files: tree.Coverage[set].Files} }
	sorted := append([]contract.Limit{}, limits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MetricID < sorted[j].MetricID })
	return &Report{
		SchemaVersion:   SchemaVersion,
		AnalyzerVersion: run.AnalyzerVersion,
		ScoringVersion:  formula.ScoringVersion,
		Repo:            Repo{Root: tree.Root, Module: tree.Module},
		ConfigDigest:    contract.Digest(run.Config),
		Coverage: Coverage{
			Production:    measured(discover.Production),
			Test:          measured(discover.Test),
			Generated:     counted(discover.Generated),
			Vendored:      counted(discover.Vendored),
			Testdata:      counted(discover.Testdata),
			Excluded:      counted(discover.Excluded),
			Unsupported:   counted(discover.Unsupported),
			NestedModules: tree.NestedModules,
		},
		Completeness: completeness(metrics),
		Metrics:      metrics,
		Score:        formula.Score(metrics),
		TestScore:    formula.TestScore(metrics),
		Findings:     findings,
		Safeguards:   append([]contract.Safeguard{}, guards...),
		Limits:       sorted,
	}
}

// completeness is incomplete when any production metric is incomplete.
func completeness(metrics map[string]contract.Metric) Completeness {
	for id, m := range metrics {
		if strings.HasSuffix(id, "."+string(contract.Production)) && m.State == contract.Incomplete {
			return Incomplete
		}
	}
	return Complete
}
