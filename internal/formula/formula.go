// Package formula holds the constants that spec 002 names and the score that it defines.
package formula

// ErosionCCThreshold is the CC that a function must exceed to count as eroded.
const ErosionCCThreshold = 10

// The duplication thresholds and budgets of spec 002.
const (
	// DuplicationMinTokens is the fewest symbols of a clone group member.
	DuplicationMinTokens = 100
	// DuplicationMinLines is the fewest lines that a clone group member spans.
	DuplicationMinLines = 3
	// DuplicationMaxTokens is the most symbols of one source set that the detector indexes.
	DuplicationMaxTokens = 2_000_000
	// DuplicationMaxWork is the most work units that indexing and extraction of one source set may spend.
	DuplicationMaxWork = 100_000_000
)

// ScoringVersion changes whenever a weight, term, or constant of the score changes.
const ScoringVersion = "0.3.0"

// Dimension is one weighted part of the index.
type Dimension struct {
	ID     string
	Weight float64
	Terms  []Term
}

// Term is one metric of a dimension, and exactly one of SaturatesAt and CountScale is set.
type Term struct {
	MetricID    string
	Share       float64
	SaturatesAt float64
	CountScale  float64
}

// Dimensions is the score table of spec 002, sorted by dimension id.
var Dimensions = []Dimension{
	{
		ID: "complexity-erosion", Weight: 0.6,
		Terms: []Term{
			{MetricID: "erosion.excess-share.production", Share: 0.5, SaturatesAt: 0.55},
			{MetricID: "erosion.excess-mass.production", Share: 0.5, CountScale: 700},
		},
	},
	{
		ID: "duplication", Weight: 0.4,
		Terms: []Term{
			{MetricID: "duplication.density.production", Share: 0.5, SaturatesAt: 0.3},
			{MetricID: "duplication.groups.production", Share: 0.5, CountScale: 15},
		},
	},
}
