package compare

import (
	"encoding/json"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// matchFindings splits the findings of two reports into new, resolved, persistent, and changed, each sorted.
// A persistent finding is the one from after, so that it shows the current location.
func matchFindings(before, after []contract.Finding) (newFindings, resolved, persistent []contract.Finding, changed []Changed) {
	beforeMatched, afterMatched := make([]bool, len(before)), make([]bool, len(after))
	// The exact rule runs first, so a finding that it matches is no candidate for the sole-finding rule.
	for _, key := range []func([]contract.Finding) []string{exactKeys, soleKeys} {
		pair(key(before), key(after), beforeMatched, afterMatched)
	}
	newFindings, resolved, persistent = []contract.Finding{}, []contract.Finding{}, []contract.Finding{}
	for i, f := range before {
		if !beforeMatched[i] {
			resolved = append(resolved, f)
		}
	}
	for i, f := range after {
		if afterMatched[i] {
			persistent = append(persistent, f)
		} else {
			newFindings = append(newFindings, f)
		}
	}
	contract.SortFindings(newFindings)
	contract.SortFindings(resolved)
	contract.SortFindings(persistent)
	newFindings, resolved, changed = pairChanged(newFindings, resolved)
	return newFindings, resolved, persistent, changed
}

// pair marks as matched each unmatched finding whose key is not "" and occurs once on each side.
func pair(beforeKeys, afterKeys []string, beforeMatched, afterMatched []bool) {
	beforeAt, afterAt := uniqueKeys(beforeKeys, beforeMatched), uniqueKeys(afterKeys, afterMatched)
	for key, i := range beforeAt {
		if j, ok := afterAt[key]; ok {
			beforeMatched[i], afterMatched[j] = true, true
		}
	}
}

// uniqueKeys maps each key that occurs once among the unmatched findings to that finding's index.
func uniqueKeys(keys []string, matched []bool) map[string]int {
	at, count := map[string]int{}, map[string]int{}
	for i, k := range keys {
		if k != "" && !matched[i] {
			at[k] = i
			count[k]++
		}
	}
	for k, n := range count {
		if n > 1 {
			delete(at, k)
		}
	}
	return at
}

// exactKeys keys a named finding on its identity, and an ambiguous one on its identity, path, and facts.
func exactKeys(findings []contract.Finding) []string {
	keys := make([]string, len(findings))
	for i, f := range findings {
		keys[i] = scope(f) + "identity\x00" + f.Identity
		if f.Ambiguous {
			// Marshal cannot fail on facts of a finding that a report holds.
			facts, _ := json.Marshal(f.Facts)
			keys[i] += "\x00" + f.Path + "\x00" + string(facts)
		}
	}
	return keys
}

// soleKeys keys an ambiguous finding on its path when it is the sole finding of its kind on that path, and leaves every other key "".
func soleKeys(findings []contract.Finding) []string {
	onPath := map[string]int{}
	for _, f := range findings {
		onPath[scope(f)+f.Path]++
	}
	keys := make([]string, len(findings))
	for i, f := range findings {
		if f.Ambiguous && onPath[scope(f)+f.Path] == 1 {
			keys[i] = scope(f) + "path\x00" + f.Path
		}
	}
	return keys
}

// scope confines matching to one finding kind in one source set, so a production finding never matches a test finding.
func scope(f contract.Finding) string {
	return f.Kind + "\x00" + string(f.SourceSet) + "\x00"
}
