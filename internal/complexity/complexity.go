// Package complexity measures the complexity distributions and the erosion of the measured source sets.
package complexity

import (
	"math"
	"sort"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/formula"
	"github.com/JacobJNilsson/godebt/internal/inventory"
)

// KindHotspot is the finding kind of an eroded function.
const KindHotspot = "complexity.hotspot"

const (
	unitCount = "count"
	unitMass  = "mass"
	unitRatio = "ratio"
)

// Measure returns the complexity and erosion metrics of both measured sets, and one hotspot finding per eroded function.
func Measure(inv *inventory.Inventory) (map[string]contract.Metric, []contract.Finding) {
	metrics := map[string]contract.Metric{}
	findings := []contract.Finding{}
	for _, set := range []contract.SourceSet{contract.Production, contract.Test} {
		var fns []inventory.Function
		for _, f := range inv.Functions {
			if f.Set == set {
				fns = append(fns, f)
			}
		}
		setMetrics := measureSet(fns)
		if inv.Incomplete(set) {
			detail := contract.Detail{Errors: inv.ErrorPaths(set)}
			for name, m := range setMetrics {
				setMetrics[name] = contract.Metric{State: contract.Incomplete, Unit: m.Unit, Detail: detail}
			}
		}
		for name, m := range setMetrics {
			metrics[name+"."+string(set)] = m
		}
		findings = append(findings, hotspots(fns)...)
	}
	contract.SortFindings(findings)
	return metrics, findings
}

// measureSet returns the metrics of the functions of one set, keyed by id without the set suffix.
func measureSet(fns []inventory.Function) map[string]contract.Metric {
	ccs := make([]int, 0, len(fns))
	var masses, erodedMasses []float64
	for _, f := range fns {
		ccs = append(ccs, f.CC)
		m := functionMass(f)
		masses = append(masses, m)
		if f.CC > formula.ErosionCCThreshold {
			erodedMasses = append(erodedMasses, m)
		}
	}
	sort.Ints(ccs)
	mass, erodedMass, eroded := sum(masses), sum(erodedMasses), len(erodedMasses)
	metrics := map[string]contract.Metric{
		"complexity.functions": measured(float64(len(ccs)), unitCount),
		"complexity.cc.p50":    notApplicable(unitCount),
		"complexity.cc.p90":    notApplicable(unitCount),
		"complexity.cc.max":    notApplicable(unitCount),
		"erosion.mass":         measured(mass, unitMass),
		"erosion.eroded-count": measured(float64(eroded), unitCount),
		"erosion.eroded-share": notApplicable(unitRatio),
	}
	if len(ccs) > 0 {
		metrics["complexity.cc.p50"] = measured(float64(percentile(ccs, 50)), unitCount)
		metrics["complexity.cc.p90"] = measured(float64(percentile(ccs, 90)), unitCount)
		metrics["complexity.cc.max"] = measured(float64(ccs[len(ccs)-1]), unitCount)
	}
	if mass > 0 {
		metrics["erosion.eroded-share"] = contract.Metric{
			State: contract.Complete, Value: erodedMass / mass, Unit: unitRatio, Numerator: erodedMass, Denominator: mass,
		}
	}
	return metrics
}

// sum adds the values in ascending order, as spec 002 demands, so that the result depends only on the values and never on file order.
func sum(values []float64) float64 {
	sort.Float64s(values)
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func functionMass(f inventory.Function) float64 {
	return float64(f.CC) * math.Sqrt(float64(f.SLOC))
}

func measured(value float64, unit string) contract.Metric {
	return contract.Metric{State: contract.Complete, Value: value, Unit: unit}
}

func notApplicable(unit string) contract.Metric {
	return contract.Metric{State: contract.NotApplicable, Unit: unit}
}

// percentile returns the nearest-rank percentile p of a non-empty ascending list, at the 1-based rank ceil(p/100 × n).
func percentile(sorted []int, p int) int {
	rank := (p*len(sorted) + 99) / 100
	return sorted[rank-1]
}

func hotspots(fns []inventory.Function) []contract.Finding {
	var out []contract.Finding
	for _, f := range fns {
		if f.CC <= formula.ErosionCCThreshold {
			continue
		}
		out = append(out, contract.Finding{
			Kind: KindHotspot, Path: f.Path, StartLine: f.StartLine, EndLine: f.EndLine,
			Identity: f.Identity, Ambiguous: f.Ambiguous, SourceSet: f.Set,
			Facts: contract.Facts{Hotspot: &contract.HotspotFacts{CC: f.CC, Nesting: f.Nesting, SLOC: f.SLOC, Mass: functionMass(f)}},
		})
	}
	return out
}
