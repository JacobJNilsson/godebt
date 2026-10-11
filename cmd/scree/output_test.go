package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHelpText(t *testing.T) {
	for _, flag := range []string{"--config <file>", "--baseline <report.json>", "--json", "--md", "--out <file>", "--quiet", "--help"} {
		if !strings.Contains(auditHelp, "\n  "+flag) {
			t.Errorf("audit help lacks the flag %s:\n%s", flag, auditHelp)
		}
	}
	_, terms, found := strings.Cut(auditHelp, "\nterms:\n")
	if !found {
		t.Fatalf("audit help lacks the terms:\n%s", auditHelp)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(terms, "\n"), "\n") {
		name, text, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok {
			t.Errorf("term line %q has no name column", line)
			continue
		}
		names = append(names, name)
		if n := len(strings.Fields(text)); n >= 20 {
			t.Errorf("term %s uses %d words, want under 20", name, n)
		}
	}
	want := []string{"index", "production/test", "cc", "p50/p90/max", "eroded", "mass", "count term", "saturating term", "clones", "dup lines", "partial", "baseline", "policy", "safeguards"}
	if !slices.Equal(names, want) {
		t.Errorf("terms = %q, want %q", names, want)
	}
	for _, command := range []string{"\n  audit <path>", "\n  compare <before.json> <after.json>", "\n  version"} {
		if !strings.Contains(help, command) {
			t.Errorf("help lacks %q:\n%s", command, help)
		}
	}
}

func TestAuditMarkdown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"audit", "--md", filepath.Join(fixtures, "functions")}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "# scree report\n") {
		t.Errorf("stdout is not the Markdown report:\n%s", stdout.String())
	}
}

func TestAuditOut(t *testing.T) {
	root := filepath.Join(fixtures, "functions")
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		flags  []string
		file   string
		prefix string
	}{
		{"json by extension", nil, "r.json", "{\n"},
		{"markdown by extension", nil, "r.md", "# scree report\n"},
		{"terminal by extension", nil, "r.txt", "scree 0.3.0-dev  "},
		{"flag over extension", []string{"--md"}, "flag.json", "# scree report\n"},
		{"quiet", []string{"--quiet", "--json"}, "quiet.out", "{\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			args := append(append([]string{"audit", root}, tc.flags...), "--out", path)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code %d, stderr %q", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
			want := "wrote " + path + "\n"
			if slices.Contains(tc.flags, "--quiet") {
				want = ""
			}
			if stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(data), tc.prefix) {
				t.Errorf("%s starts %q, want %q", tc.file, data[:min(len(data), 40)], tc.prefix)
			}
		})
	}
}

func TestAuditOutFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "r.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"audit", filepath.Join(fixtures, "functions"), "--out", path}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr.String(), "scree: open "+path) {
		t.Errorf("stderr = %q, want the open error", stderr.String())
	}
}
