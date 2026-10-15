package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/compare"
	"github.com/JacobJNilsson/godebt/internal/contract"
)

// errPathOut reports a filtered report that --out would save as a baseline.
var errPathOut = errors.New("--path and --out are exclusive, because a filtered report is not a valid baseline")

// checkDir returns the cleaned --path directory, and "" when the flag is absent.
func checkDir(f auditFlags) (string, error) {
	if !f.dirSet {
		return "", nil
	}
	if f.out != "" {
		return "", errPathOut
	}
	return cleanDir(f.root, f.dir)
}

// printable returns the report and comparison that the renderers print, filtered when dir is set.
func printable(r *godebt.Report, cmp *godebt.Comparison, dir string) (*godebt.Report, *godebt.Comparison) {
	if dir == "" {
		return r, cmp
	}
	return filterReport(r, dir), filterComparison(cmp, dir)
}

// cleanDir returns the slash form of dir, cleaned, and checks that it names a directory under root without a symbolic link.
func cleanDir(root, dir string) (string, error) {
	if dir == "" {
		return "", errors.New("--path must not be empty")
	}
	slash := filepath.ToSlash(dir)
	if path.IsAbs(slash) || filepath.IsAbs(dir) {
		return "", fmt.Errorf("--path %q must be relative to the audited root", dir)
	}
	if slices.Contains(strings.Split(slash, "/"), "..") {
		return "", fmt.Errorf("--path %q must not hold \"..\"", dir)
	}
	clean := path.Clean(slash)
	if err := checkPlainDir(root, clean); err != nil {
		return "", fmt.Errorf("--path %q %w", dir, err)
	}
	return clean, nil
}

// checkPlainDir checks that every element of clean under root is a directory and not a symbolic link.
func checkPlainDir(root, clean string) error {
	cur := root
	for _, el := range strings.Split(clean, "/") {
		if el == "." {
			continue
		}
		cur = filepath.Join(cur, el)
		info, err := os.Lstat(cur)
		if err != nil || !info.IsDir() {
			return errors.New("is not a directory under the audited root")
		}
	}
	return nil
}

// under reports whether p is dir or lies inside it, and the dir "." holds every path.
func under(dir, p string) bool {
	return dir == "." || p == dir || strings.HasPrefix(p, dir+"/")
}

// findingUnder reports whether the path of f, or of any member of a clone group, is under dir.
func findingUnder(dir string, f contract.Finding) bool {
	if under(dir, f.Path) {
		return true
	}
	if f.Facts.Clone == nil {
		return false
	}
	return slices.ContainsFunc(f.Facts.Clone.Members, func(m contract.CloneMember) bool { return under(dir, m.Path) })
}

func findingsUnder(dir string, fs []contract.Finding) []contract.Finding {
	var kept []contract.Finding
	for _, f := range fs {
		if findingUnder(dir, f) {
			kept = append(kept, f)
		}
	}
	return kept
}

// filterReport returns a copy of r whose findings lie under dir, so the metrics and the score stay those of the whole module.
func filterReport(r *godebt.Report, dir string) *godebt.Report {
	c := *r
	c.Findings = findingsUnder(dir, r.Findings)
	c.Dir = dir
	return &c
}

// changedUnder keeps the changed entries whose new group has a member under dir.
func changedUnder(dir string, changed []compare.Changed) []compare.Changed {
	var kept []compare.Changed
	for _, ch := range changed {
		if findingUnder(dir, ch.After) {
			kept = append(kept, ch)
		}
	}
	return kept
}

// filterComparison returns a copy of cmp whose new, resolved, and changed lists lie under dir.
func filterComparison(cmp *godebt.Comparison, dir string) *godebt.Comparison {
	if cmp == nil {
		return nil
	}
	c := *cmp
	c.New = findingsUnder(dir, cmp.New)
	c.Resolved = findingsUnder(dir, cmp.Resolved)
	c.Changed = changedUnder(dir, cmp.Changed)
	return &c
}
