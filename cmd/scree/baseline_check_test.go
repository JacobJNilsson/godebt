package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/scree/internal/formula"
)

// baselinedCopy copies the sets fixture and writes its baseline at the default path.
func baselinedCopy(t *testing.T) (root, file string) {
	t.Helper()
	root = copyFixture(t, "sets")
	if code, _, stderr := runArgs(t, "baseline", root, "--quiet"); code != 0 {
		t.Fatalf("baseline: exit %d, stderr %q", code, stderr)
	}
	return root, filepath.Join(root, baselineFile)
}

// TestBaselineCheckCurrent covers the first write followed by --check, which the self-reference rule must keep equal.
func TestBaselineCheckCurrent(t *testing.T) {
	root, file := baselinedCopy(t)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runArgs(t, "baseline", "--check", root)
	if code != 0 || stdout != "" || stderr != "baseline is current: "+file+"\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, stderr := runArgs(t, "baseline", root, "--check", "--quiet"); code != 0 || stderr != "" {
		t.Errorf("quiet: exit %d, stderr %q", code, stderr)
	}
	if after, err := os.ReadFile(file); err != nil || string(after) != string(before) {
		t.Errorf("check changed the file: %v", err)
	}
}

func TestBaselineCheckCustomOut(t *testing.T) {
	root := copyFixture(t, "sets")
	out := filepath.Join(root, "ci.json")
	if code, _, stderr := runArgs(t, "baseline", root, "--out", out, "--quiet"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runArgs(t, "baseline", root, "--out", out, "--check", "--quiet"); code != 0 || stderr != "" {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, baselineFile)); !os.IsNotExist(err) {
		t.Errorf("default file exists: %v", err)
	}
}

// TestBaselineCheckStaleByCoverage asserts that a new non-Go file makes the file stale, and that the summary shows the unchanged index.
func TestBaselineCheckStaleByCoverage(t *testing.T) {
	root, file := baselinedCopy(t)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runArgs(t, "baseline", root, "--check")
	head := "baseline has not been updated: " + file + "\nregenerate it with: scree baseline " + root + "\n"
	if code != 2 || stdout != "" || !strings.HasPrefix(stderr, head) || stderr[len(head):] != "index 0 to 0  new 0  resolved 0\nchanged: coverage.unsupported.files 2 to 3\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestBaselineCheckStaleByFinding(t *testing.T) {
	root, _ := baselinedCopy(t)
	src := "package extra\n\nfunc F(a int) int {\n" + strings.Repeat("\tif a > 1 {\n\t\ta--\n\t}\n", 14) + "\treturn a\n}\n"
	if err := os.WriteFile(filepath.Join(root, "extra.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runArgs(t, "baseline", root, "--check")
	if code != 2 || !strings.Contains(stderr, "index 0 to 31  new 1  resolved 0\n") || !strings.Contains(stderr, "changed: findings 0 to 1 items\n") || !strings.Contains(stderr, "changed: coverage.production.files 2 to 3\n") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestBaselineCheckStaleByVersion(t *testing.T) {
	root, file := baselinedCopy(t)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), `"scoringVersion": "`+formula.ScoringVersion+`"`, `"scoringVersion": "0.1.0"`, 1)
	if edited == string(data) {
		t.Fatal("the baseline holds no scoring version to edit")
	}
	if err := os.WriteFile(file, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runArgs(t, "baseline", root, "--check")
	head := "baseline has not been updated: " + file + "\nregenerate it with: scree baseline " + root + "\n"
	if code != 2 || !strings.HasPrefix(stderr, head) || !strings.Contains(stderr, "refused: scoringVersion differs") || !strings.HasSuffix(stderr, "changed: scoringVersion \"0.1.0\" to \""+formula.ScoringVersion+"\"\n") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestBaselineCheckRegenerateCommand(t *testing.T) {
	root := copyFixture(t, "sets")
	cfg := filepath.Join(root, "alt.yaml")
	out := filepath.Join(root, "ci.json")
	if err := os.WriteFile(cfg, []byte("exclude: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runArgs(t, "baseline", root, "--out", out, "--quiet"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, stderr := runArgs(t, "baseline", root, "--check", "--out", out, "--config", cfg)
	if !strings.Contains(stderr, "regenerate it with: scree baseline "+root+" --config "+cfg+" --out "+out+"\n") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestBaselineCheckUnusableFile(t *testing.T) {
	root := copyFixture(t, "sets")
	file := filepath.Join(root, baselineFile)
	code, stdout, stderr := runArgs(t, "baseline", root, "--check")
	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "scree: open "+file+": ") {
		t.Errorf("missing: exit %d, stderr %q", code, stderr)
	}
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runArgs(t, "baseline", root, "--check")
	if code != 1 || !strings.Contains(stderr, file) || !strings.Contains(stderr, "regenerate it with: scree baseline "+root+"\n") {
		t.Errorf("invalid: exit %d, stderr %q", code, stderr)
	}
	schema := strings.Replace(string(committedBaseline(t, root)), `"schemaVersion": "1.0.0"`, `"schemaVersion": "9.0.0"`, 1)
	if err := os.WriteFile(file, []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runArgs(t, "baseline", root, "--check"); code != 1 || !strings.Contains(stderr, `schemaVersion: "9.0.0"`) || !strings.Contains(stderr, "regenerate it with:") {
		t.Errorf("schema: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "ci.json")); !os.IsNotExist(err) {
		t.Error("check wrote a file")
	}
}

func TestBaselineCheckAuditError(t *testing.T) {
	_, file := baselinedCopy(t)
	code, _, stderr := runArgs(t, "baseline", "/nonexistent-scree-path", "--check", "--out", file)
	if code != 1 || !strings.HasPrefix(stderr, "scree: stat ") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestBaselineCheckQuotesRegenerateCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my module")
	if err := os.CopyFS(root, os.DirFS(filepath.Join(fixtures, "sets"))); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runArgs(t, "baseline", root, "--quiet"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, stderr := runArgs(t, "baseline", root, "--check")
	if !strings.Contains(stderr, "regenerate it with: scree baseline '"+root+"'\n") {
		t.Errorf("stderr = %q", stderr)
	}
	if got := shellQuote("it's.yaml"); got != `'it'\''s.yaml'` {
		t.Errorf("shellQuote = %s", got)
	}
}

func TestChangedLinesBound(t *testing.T) {
	left, right := map[string]int{"meta": 1}, map[string]int{"meta": 2}
	for i := range 13 {
		left[fmt.Sprintf("k%02d", i)], right[fmt.Sprintf("k%02d", i)] = 1, 2
	}
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	lines := changedLines(a, b)
	if len(lines) != maxChanged+1 || lines[0] != "changed: k00 1 to 2" || lines[maxChanged] != "+3 more" {
		t.Errorf("lines = %q", lines)
	}
	if got := changedLines([]byte(`{"a":[1,2]}`), []byte(`{"a":[1,3]}`)); len(got) != 1 || got[0] != "changed: a" {
		t.Errorf("equal-length arrays: %q", got)
	}
}

// committedBaseline returns the bytes of a freshly written baseline for root.
func committedBaseline(t *testing.T, root string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "b.json")
	if code, _, stderr := runArgs(t, "baseline", root, "--out", out, "--quiet"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestChangedLinesPutScoreAndFindingsFirst(t *testing.T) {
	left, right := map[string]any{"findings": []int{}, "score": map[string]int{"index": 1}}, map[string]any{"findings": []int{1}, "score": map[string]int{"index": 2}}
	for i := range 13 {
		left[fmt.Sprintf("a%02d", i)], right[fmt.Sprintf("a%02d", i)] = 1, 2
	}
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	lines := changedLines(a, b)
	if len(lines) != maxChanged+1 || lines[0] != "changed: score.index 1 to 2" || lines[1] != "changed: findings 0 to 1 items" || lines[2] != "changed: a00 1 to 2" {
		t.Errorf("lines = %q", lines)
	}
}
