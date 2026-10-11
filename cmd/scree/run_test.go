package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/scree"
	"github.com/JacobJNilsson/scree/internal/report"
)

const fixtures = "../../testdata/fixtures"

func TestRun(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
		wantCode   int
	}{
		{name: "version", args: []string{"version"}, wantStdout: "scree 0.3.0\n"},
		{name: "missing subcommand", wantStderr: usage + "\n", wantCode: 1},
		{name: "unknown subcommand", args: []string{"inspect"}, wantStderr: usage + "\n", wantCode: 1},
		{name: "audit without path", args: []string{"audit"}, wantStderr: usage + "\n", wantCode: 1},
		{name: "audit with two paths", args: []string{"audit", "a", "b"}, wantStderr: usage + "\n", wantCode: 1},
		{
			name:       "audit unknown flag",
			args:       []string{"audit", "-x"},
			wantStderr: "flag provided but not defined: -x\n" + usage + "\n",
			wantCode:   1,
		},
		{
			name:       "audit unknown flag after path",
			args:       []string{"audit", ".", "-x"},
			wantStderr: "flag provided but not defined: -x\n" + usage + "\n",
			wantCode:   1,
		},
		{
			name:       "audit missing path",
			args:       []string{"audit", "/nonexistent-scree-path"},
			wantStderr: "scree: stat /nonexistent-scree-path: no such file or directory\n",
			wantCode:   1,
		},
		{name: "extra argument", args: []string{"version", "x"}, wantStderr: usage + "\n", wantCode: 1},
		{name: "help flag", args: []string{"-h"}, wantStdout: help},
		{name: "long help flag", args: []string{"--help"}, wantStdout: help},
		{name: "audit help flag", args: []string{"audit", "-h"}, wantStdout: auditHelp},
		{name: "audit long help flag", args: []string{"audit", ".", "--help"}, wantStdout: auditHelp},
		{
			name:       "json and md",
			args:       []string{"audit", "--json", "--md", "."},
			wantStderr: "scree: --json and --md are exclusive\n",
			wantCode:   1,
		},
		{
			name:       "unknown flag",
			args:       []string{"-x"},
			wantStderr: "flag provided but not defined: -x\n" + usage + "\n",
			wantCode:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if got := stdout.String(); got != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", got, tc.wantStdout)
			}
			if got := stderr.String(); got != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", got, tc.wantStderr)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRunVersionWriteFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"version"}, failingWriter{}, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

// TestAuditJSONParity proves that the CLI prints the marshalled report of scree.Audit.
func TestAuditJSONParity(t *testing.T) {
	root := filepath.Join(fixtures, "functions")
	r, err := scree.Audit(context.Background(), root, scree.Options{})
	if err != nil {
		t.Fatal(err)
	}
	r.Meta = report.Meta{}
	want, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"audit", "--json", root}, {"audit", root, "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v: exit code %d, stderr %q", args, code, stderr.String())
		}
		printed, err := report.Load(&stdout)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		printed.Meta = report.Meta{}
		got, err := json.MarshalIndent(printed, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%v: stdout differs from json.MarshalIndent of scree.Audit:\n%s", args, stdout.String())
		}
	}
}

func TestAuditTerminal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"audit", filepath.Join(fixtures, "broken")}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\nincomplete: bad.go, bad_test.go\n") {
		t.Errorf("stdout lacks the error paths:\n%s", stdout.String())
	}
}

func TestAuditWriteFailure(t *testing.T) {
	for _, args := range [][]string{{"audit", fixtures}, {"audit", "--json", fixtures}} {
		var stderr bytes.Buffer
		if code := run(args, failingWriter{}, &stderr); code != 1 {
			t.Errorf("%v: exit code = %d, want 1", args, code)
		}
		if want := "scree: write failed\n"; stderr.String() != want {
			t.Errorf("%v: stderr = %q, want %q", args, stderr.String(), want)
		}
	}
}
