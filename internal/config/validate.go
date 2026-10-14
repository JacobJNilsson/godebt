package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
)

// newFindingKinds are the finding kinds that failOnNew accepts.
var newFindingKinds = []string{"complexity.hotspot", "duplication.clone-group"}

func (f *File) validate() error {
	if err := validPatterns("exclude", f.Exclude); err != nil {
		return err
	}
	if err := validPatterns("classify.test", f.Classify.Test); err != nil {
		return err
	}
	return f.Policy.validate()
}

func validPatterns(at string, patterns []string) error {
	for i, p := range patterns {
		if err := discover.ValidPattern(p); err != nil {
			return &contract.FieldError{Field: fmt.Sprintf("%s[%d]", at, i), Reason: err.Error()}
		}
	}
	return nil
}

func (p Policy) validate() error {
	if p.MaxIndex != nil && (*p.MaxIndex < 0 || *p.MaxIndex > 100) {
		return &contract.FieldError{Field: "policy.maxIndex", Reason: "must be between 0 and 100"}
	}
	if err := p.Regression.validate(); err != nil {
		return err
	}
	if err := validBudgets(p.Budgets); err != nil {
		return err
	}
	for i, kind := range p.FailOnNew {
		if !slices.Contains(newFindingKinds, kind) {
			reason := fmt.Sprintf("%q is not one of %s", kind, strings.Join(newFindingKinds, ", "))
			return &contract.FieldError{Field: fmt.Sprintf("policy.failOnNew[%d]", i), Reason: reason}
		}
	}
	return nil
}

func (r *Regression) validate() error {
	if r == nil {
		return nil
	}
	if r.MaxIncrease != nil && *r.MaxIncrease < 0 {
		return &contract.FieldError{Field: "policy.regression.maxIncrease", Reason: "must not be negative"}
	}
	// The negated comparison also rejects NaN.
	if r.MaxIncreasePercent != nil && !(*r.MaxIncreasePercent >= 0) {
		return &contract.FieldError{Field: "policy.regression.maxIncreasePercent", Reason: "must not be negative"}
	}
	return nil
}

func validBudgets(budgets map[string]Budget) error {
	ids := make([]string, 0, len(budgets))
	for id := range budgets {
		ids = append(ids, id)
	}
	// Sorted keys make the reported error the same on every run.
	slices.Sort(ids)
	for _, id := range ids {
		at := "policy.budgets." + id
		if !slices.Contains(contract.MetricIDs(), id) {
			return &contract.FieldError{Field: at, Reason: "is not a metric id that a report holds"}
		}
		limit := budgets[id].Max
		if limit == nil {
			return &contract.FieldError{Field: at + ".max", Reason: "is missing"}
		}
		if !(*limit >= 0) {
			return &contract.FieldError{Field: at + ".max", Reason: "must not be negative"}
		}
	}
	return nil
}
