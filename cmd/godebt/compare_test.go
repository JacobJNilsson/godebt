package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compareGolden compares output with a golden file, and it rewrites the file first under -update.
func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(goldens, name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file, run make golden and review the diff:\n%s", path, got)
	}
}

func TestCompareGolden(t *testing.T) {
	before, after := filepath.Join(goldens, "report-sets.json"), filepath.Join(goldens, "report-functions.json")
	for name, flags := range map[string][]string{
		"compare-terminal.txt": nil,
		"compare-markdown.md":  {"--md"},
		"compare-json.json":    {"--json"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runArgs(t, append([]string{"compare", before, after}, flags...)...)
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			compareGolden(t, name, []byte(stdout))
		})
	}
}

func TestCompareRefused(t *testing.T) {
	live := saveBaseline(t)
	for _, flags := range [][]string{nil, {"--json"}} {
		args := append([]string{"compare", filepath.Join(goldens, "report-functions.json"), live}, flags...)
		code, stdout, stderr := runArgs(t, args...)
		if code != 2 || stderr != "" {
			t.Errorf("%v: exit %d, stderr %q, want 2 and nothing", flags, code, stderr)
		}
		if !strings.Contains(stdout, "analyzerVersion differs") {
			t.Errorf("%v: stdout lacks the refusal:\n%s", flags, stdout)
		}
	}
}

func TestCompareErrors(t *testing.T) {
	good := filepath.Join(goldens, "report-functions.json")
	missing := filepath.Join(t.TempDir(), "absent.json")
	cases := map[string]struct {
		args   []string
		stderr string
	}{
		"one report":     {[]string{"compare", good}, usage + "\n"},
		"three reports":  {[]string{"compare", good, good, good}, usage + "\n"},
		"unknown flag":   {[]string{"compare", good, good, "-x"}, "flag provided but not defined: -x\n" + usage + "\n"},
		"missing before": {[]string{"compare", missing, good}, "godebt: open " + missing + ": no such file or directory\n"},
		"missing after":  {[]string{"compare", good, missing}, "godebt: open " + missing + ": no such file or directory\n"},
		"json and md":    {[]string{"compare", good, good, "--json", "--md"}, "godebt: --json and --md are exclusive\n"},
		"not a report":   {[]string{"compare", good, "compare_test.go"}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runArgs(t, tc.args...)
			if code != 1 || stdout != "" {
				t.Errorf("exit %d, stdout %q, want 1 and nothing", code, stdout)
			}
			if tc.stderr != "" && stderr != tc.stderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.stderr)
			}
			if !strings.HasSuffix(stderr, "\n") {
				t.Errorf("stderr = %q, want an error line", stderr)
			}
		})
	}
}

func TestCompareHelp(t *testing.T) {
	code, stdout, _ := runArgs(t, "compare", "--help")
	if code != 0 || stdout != compareHelp {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestCompareWriteFailure(t *testing.T) {
	good := filepath.Join(goldens, "report-functions.json")
	var stderr bytes.Buffer
	if code := run([]string{"compare", good, good}, failingWriter{}, &stderr); code != 1 || stderr.String() != "godebt: write failed\n" {
		t.Errorf("exit %d, stderr %q", code, stderr.String())
	}
}
