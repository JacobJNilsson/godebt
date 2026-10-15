package compare

import (
	"sort"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// minOverlapping is the number of members of a new group that must overlap a resolved group before the two join.
const minOverlapping = 2

// Changed is a clone group whose group id changed because its extent changed.
// After is the new group, and the before fields come from the resolved group.
type Changed struct {
	After         contract.Finding `json:"after"`
	BeforeGroupID string           `json:"beforeGroupId"`
	BeforeTokens  int              `json:"beforeTokens"`
}

// candidate is a possible pair of one resolved and one new clone group.
type candidate struct {
	resolved, added int
	shared          int
}

// pairChanged moves joined clone groups out of newFindings and resolved, and returns them as changed entries.
func pairChanged(newFindings, resolved []contract.Finding) (restNew, restResolved []contract.Finding, changed []Changed) {
	var cands []candidate
	for r, rf := range resolved {
		for n, nf := range newFindings {
			if shared, ok := overlap(rf, nf); ok {
				cands = append(cands, candidate{r, n, shared})
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.shared != b.shared {
			return a.shared > b.shared
		}
		ra, rb := resolved[a.resolved].Facts.Clone.GroupID, resolved[b.resolved].Facts.Clone.GroupID
		if ra != rb {
			return ra < rb
		}
		return newFindings[a.added].Facts.Clone.GroupID < newFindings[b.added].Facts.Clone.GroupID
	})
	usedResolved, usedNew := make([]bool, len(resolved)), make([]bool, len(newFindings))
	for _, c := range cands {
		if usedResolved[c.resolved] || usedNew[c.added] {
			continue
		}
		usedResolved[c.resolved], usedNew[c.added] = true, true
		before := resolved[c.resolved].Facts.Clone
		changed = append(changed, Changed{After: newFindings[c.added], BeforeGroupID: before.GroupID, BeforeTokens: before.Tokens})
	}
	sort.Slice(changed, func(i, j int) bool {
		a, b := changed[i].After, changed[j].After
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		return a.Identity < b.Identity
	})
	restNew, restResolved = []contract.Finding{}, []contract.Finding{}
	for i, f := range newFindings {
		if !usedNew[i] {
			restNew = append(restNew, f)
		}
	}
	for i, f := range resolved {
		if !usedResolved[i] {
			restResolved = append(restResolved, f)
		}
	}
	if changed == nil {
		changed = []Changed{}
	}
	return restNew, restResolved, changed
}

// overlap returns the lines that the members of two clone groups of one source set share, and whether enough members of added overlap.
func overlap(resolved, added contract.Finding) (shared int, ok bool) {
	if resolved.Facts.Clone == nil || added.Facts.Clone == nil || resolved.SourceSet != added.SourceSet {
		return 0, false
	}
	overlapping := 0
	for _, n := range added.Facts.Clone.Members {
		hit := false
		for _, r := range resolved.Facts.Clone.Members {
			if n.Path != r.Path {
				continue
			}
			if lines := min(n.EndLine, r.EndLine) - max(n.StartLine, r.StartLine) + 1; lines > 0 {
				shared += lines
				hit = true
			}
		}
		if hit {
			overlapping++
		}
	}
	return shared, overlapping >= minOverlapping
}
