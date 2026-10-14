package formula

import (
	"math"
	"sort"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// DirectionLowerIsBetter is the only direction of the index.
const DirectionLowerIsBetter = "lower-is-better"

// fullScore is the score of a dimension or term whose metric was not measured.
const fullScore = 100

// Score computes the index of spec 002 from the production metrics, and it reads no configuration.
func Score(metrics map[string]contract.Metric) contract.Score {
	out := contract.Score{Direction: DirectionLowerIsBetter, Contributions: make([]contract.Contribution, len(Dimensions))}
	weighted := make([]float64, len(Dimensions))
	for i, d := range Dimensions {
		terms, dimension, partial := scoreDimension(d, metrics)
		out.Partial = out.Partial || partial
		weighted[i] = d.Weight * dimension
		out.Contributions[i] = contract.Contribution{Dimension: d.ID, Weight: d.Weight, Terms: terms}
	}
	total := sum(append([]float64(nil), weighted...))
	// A count term stays below 100, so the rounded index stays within 0..100 without a clamp.
	out.Index = int(math.Floor(total + 0.5))
	for i, points := range apportion(weighted, out.Index) {
		out.Contributions[i].Points = points
	}
	return out
}

// scoreDimension returns the scored terms of one dimension, its score, and whether a required metric was not measured.
func scoreDimension(d Dimension, metrics map[string]contract.Metric) ([]contract.Term, float64, bool) {
	zeroCount := false
	for _, t := range d.Terms {
		m, ok := metrics[t.MetricID]
		if ok && t.CountScale > 0 && m.State == contract.Complete && m.Value == 0 {
			zeroCount = true
		}
	}
	terms := make([]contract.Term, len(d.Terms))
	parts := make([]float64, len(d.Terms))
	partial := false
	for i, t := range d.Terms {
		m, ok := metrics[t.MetricID]
		term := contract.Term{MetricID: t.MetricID, State: m.State, SaturatesAt: t.SaturatesAt, CountScale: t.CountScale}
		switch {
		case !ok:
			term.State, term.Score, partial = contract.Incomplete, fullScore, true
		case m.State == contract.Complete:
			term.Value, term.Score = m.Value, termScore(t, m.Value)
		case m.State == contract.NotApplicable && zeroCount:
			term.Score = 0
		default:
			term.Score, partial = fullScore, true
		}
		terms[i] = term
		parts[i] = t.Share * term.Score
	}
	if partial {
		return terms, fullScore, true
	}
	return terms, sum(parts), false
}

// termScore scores a measured value by the saturating rule or the count rule of spec 002.
func termScore(t Term, value float64) float64 {
	if t.SaturatesAt > 0 {
		return fullScore * min(1, value/t.SaturatesAt)
	}
	b := math.Log(1 + value/t.CountScale)
	return fullScore * b / (1 + b)
}

// apportion splits total into integer points by largest remainder, and a tie goes to the lower index, which is the lower dimension id.
func apportion(weighted []float64, total int) []int {
	points := make([]int, len(weighted))
	order := make([]int, len(weighted))
	left := total
	for i, w := range weighted {
		points[i] = int(math.Floor(w))
		left -= points[i]
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra := weighted[order[a]] - math.Floor(weighted[order[a]])
		rb := weighted[order[b]] - math.Floor(weighted[order[b]])
		return ra > rb
	})
	for k := 0; k < left && k < len(order); k++ {
		points[order[k]]++
	}
	return points
}

// sum adds the values in ascending order, as spec 002 demands, so that the result depends only on the values.
func sum(values []float64) float64 {
	sort.Float64s(values)
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}
