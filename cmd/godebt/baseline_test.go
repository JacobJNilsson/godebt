package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt"
)

// copyFixture copies a fixture module into a temporary directory, so a command can write beside its files.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "module")
	if err := os.CopyFS(root, os.DirFS(filepath.Join(fixtures, name))); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBaselineWritesGolden(t *testing.T) {
	for _, name := range []string{"functions", "sets"} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "b.json")
			code, stdout, stderr := runArgs(t, "baseline", filepath.Join(fixtures, name), "--out", out)
			if code != 0 || stdout != "" || stderr != "wrote "+out+"\n" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, "baseline-"+name+".json", data)
		})
	}
}

// TestBaselineDefaultOut asserts the default file name at the audited root, and that the file is a valid baseline for audit and compare.
func TestBaselineDefaultOut(t *testing.T) {
	root := copyFixture(t, "functions")
	code, _, stderr := runArgs(t, "baseline", "--quiet", root)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	file := filepath.Join(root, "godebt-baseline.json")
	if _, err := godebt.LoadReport(file); err != nil {
		t.Fatalf("default file: %v", err)
	}
	if code, _, stderr := runArgs(t, "audit", root, "--baseline", file, "--quiet"); code != 0 || strings.Contains(stderr, "baseline:") {
		t.Errorf("audit: exit %d, stderr %q", code, stderr)
	}
	if code, stdout, stderr := runArgs(t, "compare", file, file); code != 0 || stderr != "" || !strings.Contains(stdout, "godebt") {
		t.Errorf("compare: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// TestBaselineDefaultPath asserts that the audited path defaults to the working directory.
func TestBaselineDefaultPath(t *testing.T) {
	root := copyFixture(t, "functions")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if code, _, stderr := runArgs(t, "baseline"); code != 0 || stderr != "wrote godebt-baseline.json\n" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, err := godebt.LoadReport("godebt-baseline.json"); err != nil {
		t.Error(err)
	}
}

func TestBaselineUsageErrors(t *testing.T) {
	cases := map[string]struct {
		args       []string
		wantStderr string
	}{
		"two paths":    {[]string{"baseline", "a", "b"}, usage + "\n"},
		"unknown flag": {[]string{"baseline", "-x"}, "flag provided but not defined: -x\n" + usage + "\n"},
		"missing root": {[]string{"baseline", "/nonexistent-godebt-path"}, "godebt: stat /nonexistent-godebt-path: no such file or directory\n"},
		"bad config":   {[]string{"baseline", fixtures + "/empty", "--config", "/nonexistent-godebt.yaml"}, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runArgs(t, c.args...)
			if code != 1 || stdout != "" || (c.wantStderr != "" && stderr != c.wantStderr) || stderr == "" {
				t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
}

func TestBaselineWriteError(t *testing.T) {
	code, _, stderr := runArgs(t, "baseline", filepath.Join(fixtures, "empty"), "--out", filepath.Join(t.TempDir(), "no", "b.json"))
	if code != 1 || !strings.HasPrefix(stderr, "godebt: ") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestBaselineHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		if code, stdout, _ := runArgs(t, "baseline", flag); code != 0 || stdout != baselineHelp {
			t.Errorf("%s: exit %d, stdout %q", flag, code, stdout)
		}
	}
}
