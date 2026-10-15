package report

import (
	"fmt"
	"io"

	"github.com/JacobJNilsson/godebt/internal/complexity"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/duplication"
	"github.com/JacobJNilsson/godebt/internal/formula"
	"github.com/JacobJNilsson/godebt/internal/safeguards"
)

// maxIndex is the highest index that the score of spec 002 can take.
const maxIndex = 100

// Load decodes a report strictly and checks the rules of spec 003 that decoding cannot check.
func Load(r io.Reader) (*Report, error) {
	var out Report
	if err := contract.Strict(r, &out); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	if err := validate(&out); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return &out, nil
}

func validate(r *Report) error {
	if r.SchemaVersion != SchemaVersion {
		return &contract.FieldError{Field: "schemaVersion", Reason: fmt.Sprintf("%q, want %q", r.SchemaVersion, SchemaVersion)}
	}
	if r.Completeness != Complete && r.Completeness != Incomplete {
		return &contract.FieldError{Field: "completeness", Reason: fmt.Sprintf("%q is unknown", r.Completeness)}
	}
	if err := validateScore("score", r.Score); err != nil {
		return err
	}
	if err := validateScore("testScore", r.TestScore); err != nil {
		return err
	}
	if err := checkFindings(r.Findings); err != nil {
		return err
	}
	return checkSafeguards(r.Safeguards)
}

// validateScore checks the direction, the range, and the point sum of one score block.
func validateScore(name string, s contract.Score) error {
	if s.Direction != formula.DirectionLowerIsBetter {
		return &contract.FieldError{Field: name + ".direction", Reason: fmt.Sprintf("%q, want %q", s.Direction, formula.DirectionLowerIsBetter)}
	}
	if s.Index < 0 || s.Index > maxIndex {
		return &contract.FieldError{Field: name + ".index", Reason: fmt.Sprintf("%d is outside 0..%d", s.Index, maxIndex)}
	}
	points := 0
	for _, c := range s.Contributions {
		points += c.Points
	}
	if points != s.Index {
		return &contract.FieldError{Field: name + ".contributions", Reason: fmt.Sprintf("sum to %d, want the index %d", points, s.Index)}
	}
	return nil
}

// checkSafeguards requires the eight ids of spec 002 in order, each with a known evidence level.
func checkSafeguards(guards []contract.Safeguard) error {
	if len(guards) != len(safeguards.IDs) {
		return &contract.FieldError{Field: "safeguards", Reason: fmt.Sprintf("holds %d entries, want %d", len(guards), len(safeguards.IDs))}
	}
	for i, g := range guards {
		if g.ID != safeguards.IDs[i] {
			return &contract.FieldError{Field: fmt.Sprintf("safeguards[%d].id", i), Reason: fmt.Sprintf("%q, want %q", g.ID, safeguards.IDs[i])}
		}
		switch g.Evidence {
		case contract.EvidenceAbsent, contract.EvidenceConfigured, contract.EvidenceStructurallyWired, contract.EvidenceUnknown:
		default:
			return &contract.FieldError{Field: fmt.Sprintf("safeguards[%d].evidence", i), Reason: fmt.Sprintf("%q is unknown", g.Evidence)}
		}
		for j, l := range g.Locations {
			if l.Line < 0 {
				return &contract.FieldError{Field: fmt.Sprintf("safeguards[%d].locations[%d].line", i, j), Reason: fmt.Sprintf("%d is negative", l.Line)}
			}
		}
	}
	return nil
}

func checkFindings(findings []contract.Finding) error {
	for i, f := range findings {
		if err := checkFinding(f); err != nil {
			err.Field = fmt.Sprintf("findings[%d].%s", i, err.Field)
			return err
		}
	}
	if i := contract.UnsortedFinding(findings); i >= 0 {
		return &contract.FieldError{Field: fmt.Sprintf("findings[%d]", i), Reason: "is out of order"}
	}
	return nil
}

// checkFinding rejects an unknown kind, a set that the kind does not allow, and facts of another kind.
func checkFinding(f contract.Finding) *contract.FieldError {
	if f.Identity == "" {
		return &contract.FieldError{Field: "identity", Reason: "is missing"}
	}
	var shapeFits, measured bool
	switch f.Kind {
	case complexity.KindHotspot:
		shapeFits, measured = f.Facts.Hotspot != nil, true
	case duplication.KindCloneGroup:
		shapeFits, measured = f.Facts.Clone != nil, true
	case contract.KindBrokenReference:
		shapeFits = f.Facts.BrokenReference != nil
	default:
		return &contract.FieldError{Field: "kind", Reason: fmt.Sprintf("%q is unknown", f.Kind)}
	}
	// A broken reference sits in a configuration file, which belongs to no measured set.
	if measured != (f.SourceSet == contract.Production || f.SourceSet == contract.Test) || (!measured && f.SourceSet != "") {
		return &contract.FieldError{Field: "sourceSet", Reason: fmt.Sprintf("%q does not fit kind %s", f.SourceSet, f.Kind)}
	}
	if !shapeFits {
		return &contract.FieldError{Field: "facts", Reason: fmt.Sprintf("do not have the shape of %s", f.Kind)}
	}
	return nil
}
