// Package policy evaluates a declared policy against a report, as spec 003 "Policy" defines.
package policy

import (
	"fmt"
	"slices"
	"sort"

	"github.com/JacobJNilsson/godebt/internal/compare"
	"github.com/JacobJNilsson/godebt/internal/config"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// The checks of spec 003, as they appear in Reason.Check and Result.Skipped.
const (
	CheckMaxIndex   = "maxIndex"
	CheckBudget     = "budget"
	CheckRegression = "regression"
	CheckFailOnNew  = "failOnNew"
)

// Reason is one failed check.
type Reason struct {
	Check   string `json:"check"`
	Message string `json:"message"`
	// Kind names the finding kind of a failOnNew reason.
	Kind string `json:"kind,omitempty"`
}

// Result is the outcome of Evaluate.
type Result struct {
	Failed  bool     `json:"failed"`
	Reasons []Reason `json:"reasons"`
	// Skipped names the baseline checks that the policy declares and that did not run for want of a baseline.
	Skipped []string `json:"skipped"`
	// Refusal is why the baseline is not comparable, and a refusal fails the result whatever the policy declares.
	Refusal string `json:"refusal,omitempty"`
}

// Evaluate runs every check of p against r, and each check runs whatever the others found.
// A nil cmp means no baseline.
func Evaluate(p config.Policy, r *report.Report, cmp *compare.Comparison) Result {
	reasons := slices.Concat(maxIndex(p, r), budgets(p, r))
	skipped := []string{}
	if p.Regression != nil {
		if cmp == nil {
			skipped = append(skipped, CheckRegression)
		} else {
			reasons = append(reasons, regression(*p.Regression, cmp)...)
		}
	}
	if len(p.FailOnNew) > 0 {
		if cmp == nil {
			skipped = append(skipped, CheckFailOnNew)
		} else {
			reasons = append(reasons, failOnNew(p.FailOnNew, cmp)...)
		}
	}
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].Check != reasons[j].Check {
			return reasons[i].Check < reasons[j].Check
		}
		return reasons[i].Message < reasons[j].Message
	})
	result := Result{Failed: len(reasons) > 0, Reasons: reasons, Skipped: skipped}
	if cmp != nil && !cmp.Comparable {
		result.Failed, result.Refusal = true, cmp.Refusal
	}
	return result
}

func maxIndex(p config.Policy, r *report.Report) []Reason {
	if p.MaxIndex == nil {
		return nil
	}
	index := r.Score.Index
	if r.Score.Partial {
		// A partial index counts unmeasured dimensions at full weight, but missing analysis never passes a gate.
		return []Reason{{Check: CheckMaxIndex, Message: fmt.Sprintf("index %d is partial because the audit did not measure a metric", index)}}
	}
	if index > *p.MaxIndex {
		return []Reason{{Check: CheckMaxIndex, Message: fmt.Sprintf("index %d is above %d", index, *p.MaxIndex)}}
	}
	return nil
}

func budgets(p config.Policy, r *report.Report) []Reason {
	var out []Reason
	for id, b := range p.Budgets {
		if message := budget(id, *b.Max, r.Metrics); message != "" {
			out = append(out, Reason{Check: CheckBudget, Message: message})
		}
	}
	return out
}

// budget returns why the metric id breaks its budget, or "" when it keeps it.
func budget(id string, limit float64, metrics map[string]contract.Metric) string {
	m, ok := metrics[id]
	switch {
	case !ok:
		return id + " is not in the report"
	case m.State == contract.Incomplete:
		return id + " is incomplete, and missing analysis never passes a budget"
	case m.State == contract.Complete && m.Value > limit:
		return fmt.Sprintf("%s is %g, above %g", id, m.Value, limit)
	}
	return ""
}

func regression(rg config.Regression, cmp *compare.Comparison) []Reason {
	if !cmp.Comparable {
		return []Reason{notComparable(CheckRegression)}
	}
	var out []Reason
	delta := cmp.IndexDelta
	if rg.MaxIncrease != nil && delta > *rg.MaxIncrease {
		out = append(out, Reason{Check: CheckRegression, Message: fmt.Sprintf("index rose %d points, above %d", delta, *rg.MaxIncrease)})
	}
	if pct := rg.MaxIncreasePercent; pct != nil && float64(delta) > float64(cmp.Before.Index)**pct/100 {
		message := fmt.Sprintf("index rose %d points, above %g%% of %d", delta, *pct, cmp.Before.Index)
		out = append(out, Reason{Check: CheckRegression, Message: message})
	}
	return out
}

func failOnNew(kinds []string, cmp *compare.Comparison) []Reason {
	if !cmp.Comparable {
		return []Reason{notComparable(CheckFailOnNew)}
	}
	var out []Reason
	for _, f := range cmp.New {
		if slices.Contains(kinds, f.Kind) {
			message := fmt.Sprintf("new %s %s at %s:%d-%d", f.Kind, f.Identity, f.Path, f.StartLine, f.EndLine)
			out = append(out, Reason{Check: CheckFailOnNew, Kind: f.Kind, Message: message})
		}
	}
	return out
}

// notComparable fails a baseline check without the refusal text, because the caller reports the refusal once.
func notComparable(check string) Reason {
	return Reason{Check: check, Message: "skipped, baseline not comparable"}
}
