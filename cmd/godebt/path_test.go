package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/compare"
	"github.com/JacobJNilsson/godebt/internal/contract"
)

// movedTree copies the named fixture and moves the listed files into the directory dir, next to an empty directory named pk.
func movedTree(t *testing.T, fixture, dir string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join(fixtures, fixture))); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{dir, "pk"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		if err := os.Rename(filepath.Join(root, f), filepath.Join(root, dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type pathReport struct {
	godebt.Report
	Comparison *godebt.Comparison `json:"comparison"`
}

func auditJSON(t *testing.T, args ...string) pathReport {
	t.Helper()
	code, stdout, stderr := runArgs(t, append([]string{"audit", "--json"}, args...)...)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var r pathReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuditPathFiltersFindings(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	all := auditJSON(t, root)
	if len(all.Findings) == 0 {
		t.Fatal("the tree has no findings")
	}
	for _, tc := range []struct {
		dir     string
		prefix  string // the directory with a trailing slash; empty for "."
		wantAny bool
	}{{"pkg", "pkg/", true}, {"./pkg/", "pkg/", true}, {"pk", "pk/", false}, {"sub", "sub/", false}, {".", "", true}} {
		got := auditJSON(t, root, "--path", tc.dir)
		if got.Score.Index != all.Score.Index {
			t.Errorf("--path %s: index %d, want %d", tc.dir, got.Score.Index, all.Score.Index)
		}
		if tc.dir == "." {
			if len(got.Findings) != len(all.Findings) {
				t.Errorf("--path . keeps %d findings, want %d", len(got.Findings), len(all.Findings))
			}
		} else {
			for _, f := range got.Findings {
				if !strings.HasPrefix(f.Path, tc.prefix) {
					t.Errorf("--path %s keeps %s", tc.dir, f.Path)
				}
			}
		}
		if (len(got.Findings) > 0) != tc.wantAny {
			t.Errorf("--path %s keeps %d findings, want any %v", tc.dir, len(got.Findings), tc.wantAny)
		}
	}
}

func TestAuditPathExcludesSiblingWithSharedPrefix(t *testing.T) {
	root := movedTree(t, "functions", "pkgx", "cc.go")
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if all := auditJSON(t, root); len(all.Findings) == 0 {
		t.Fatal("the tree has no findings")
	}
	if got := auditJSON(t, root, "--path", "pkg"); len(got.Findings) != 0 {
		t.Errorf("--path pkg keeps %d findings from pkgx, want 0", len(got.Findings))
	}
}

func TestAuditPathKeepsPartlyInsideCloneGroup(t *testing.T) {
	root := movedTree(t, "clones/exact", "sub", "b.go", "b_test.go")
	for dir, want := range map[string]int{"sub": 2, "pk": 0} {
		got := auditJSON(t, root, "--path", dir)
		if len(got.Findings) != want {
			t.Errorf("--path %s keeps %d findings, want %d", dir, len(got.Findings), want)
		}
	}
	text := runText(t, root, "--path", "sub")
	if !strings.Contains(text, "  a.go:") || !strings.Contains(text, "  sub/b.go:") {
		t.Errorf("the group lost a member:\n%s", text)
	}
}

func runText(t *testing.T, args ...string) string {
	t.Helper()
	code, stdout, stderr := runArgs(t, append([]string{"audit"}, args...)...)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	return stdout
}

func TestAuditPathFiltersComparison(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	base := saveBaseline(t)
	all := auditJSON(t, root, "--baseline", base)
	got := auditJSON(t, root, "--baseline", base, "--path", "pkg")
	if len(all.Comparison.New) == 0 || len(all.Comparison.Resolved) == 0 {
		t.Fatalf("the unfiltered comparison has %d new and %d resolved findings", len(all.Comparison.New), len(all.Comparison.Resolved))
	}
	if len(got.Comparison.New) != len(all.Comparison.New) || len(got.Comparison.Resolved) != 0 {
		t.Errorf("filtered comparison has %d new and %d resolved findings, want %d and 0", len(got.Comparison.New), len(got.Comparison.Resolved), len(all.Comparison.New))
	}
	if got.Comparison.IndexDelta != all.Comparison.IndexDelta {
		t.Errorf("index delta %d, want %d", got.Comparison.IndexDelta, all.Comparison.IndexDelta)
	}
	text := runText(t, root, "--baseline", base, "--path", "pkg")
	want := fmt.Sprintf("new %d  resolved 0  changed 0\n", len(all.Comparison.New))
	if !strings.Contains(text, want) {
		t.Errorf("text lacks %q:\n%s", want, text)
	}
}

func TestAuditPathPolicyStaysWhole(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	cfg := writeFile(t, "c.yaml", "policy:\n  maxIndex: 1\n")
	if code, _, _ := runArgs(t, "audit", root, "--config", cfg, "--path", "pk"); code != exitPolicy {
		t.Errorf("exit %d, want %d, because the policy covers the whole module", code, exitPolicy)
	}
}

func TestAuditPathHeader(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	if text := runText(t, root, "--path", "./pkg/"); !strings.Contains(text, "\nfindings under pkg\n") {
		t.Errorf("text header lacks the directory:\n%s", text)
	}
	code, md, _ := runArgs(t, "audit", root, "--md", "--path", "pkg")
	if code != 0 || !strings.Contains(md, "\nPrinted findings are under `pkg`.\n") {
		t.Errorf("Markdown header lacks the directory:\n%s", md)
	}
	if text := runText(t, root); strings.Contains(text, "findings under") {
		t.Errorf("an unfiltered run names a directory:\n%s", text)
	}
}

func TestAuditPathUsageErrors(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	out := filepath.Join(t.TempDir(), "r.json")
	for name, args := range map[string][]string{
		"with out":       {"--path", "pkg", "--out", out},
		"absolute":       {"--path", "/pkg"},
		"dotdot":         {"--path", "pkg/../pk"},
		"leading dotdot": {"--path", "../x"},
		"missing":        {"--path", "nope"},
		"file":           {"--path", "pkg/cc.go"},
		"empty":          {"--path", ""},
		"empty equals":   {"--path="},
	} {
		code, stdout, stderr := runArgs(t, append([]string{"audit", root}, args...)...)
		if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "godebt: ") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1 and a godebt error", name, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("--path with --out wrote the file")
	}
}

func TestAuditPathRejectsSymlink(t *testing.T) {
	root := movedTree(t, "functions", "pkg", "cc.go")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "lnk")); err != nil {
		t.Skipf("the OS cannot make symbolic links: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "pkg"), filepath.Join(root, "inner")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"lnk", "inner", "lnk/sub", "./inner/"} {
		code, stdout, stderr := runArgs(t, "audit", root, "--path", dir)
		if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "godebt: ") {
			t.Errorf("--path %s: exit %d, stdout %q, stderr %q, want exit 1 and a godebt error", dir, code, stdout, stderr)
		}
	}
}

func TestHelpListsPath(t *testing.T) {
	if !strings.Contains(auditHelp, "\n  --path <dir>") || !strings.Contains(usage, "[--path <dir>]") {
		t.Error("help or usage lacks --path <dir>")
	}
}

func TestFilterComparisonKeepsChangedUnderDir(t *testing.T) {
	changed := func(id string, paths ...string) compare.Changed {
		facts := &contract.CloneFacts{GroupID: id}
		for _, p := range paths {
			facts.Members = append(facts.Members, contract.CloneMember{Path: p, StartLine: 1, EndLine: 5})
		}
		return compare.Changed{After: contract.Finding{Kind: "duplication.clone-group", Path: paths[0], Identity: id, Facts: contract.Facts{Clone: facts}}}
	}
	cmp := &godebt.Comparison{Changed: []compare.Changed{changed("in", "pkg/a.go", "x/b.go"), changed("member", "x/b.go", "pkg/c.go"), changed("out", "x/b.go", "y/c.go")}}
	got := filterComparison(cmp, "pkg").Changed
	if len(got) != 2 || got[0].After.Identity != "in" || got[1].After.Identity != "member" {
		t.Errorf("filtered changed = %+v, want in and member", got)
	}
}
