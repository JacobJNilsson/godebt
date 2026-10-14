package godebt

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/config"
	"github.com/JacobJNilsson/godebt/internal/contract"
)

// fixtureCopy copies a fixture module into a temporary directory, so a test can add or remove files.
func fixtureCopy(t *testing.T, fixture string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "module")
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata/fixtures", fixture))); err != nil {
		t.Fatal(err)
	}
	return root
}

func audit(t *testing.T, root string) *Report {
	t.Helper()
	r, err := Audit(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuditReadsDefaultConfig(t *testing.T) {
	root := fixtureCopy(t, "functions")
	plain := audit(t, root)
	if err := os.WriteFile(filepath.Join(root, "godebt.yaml"), []byte("exclude: [\"sub/**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := audit(t, root)
	if configured.Coverage.Production.Files != plain.Coverage.Production.Files-1 {
		t.Errorf("production files %d, want one fewer than %d", configured.Coverage.Production.Files, plain.Coverage.Production.Files)
	}
	if configured.Coverage.Excluded.Files != plain.Coverage.Excluded.Files+1 {
		t.Errorf("excluded files %d, want one more than %d", configured.Coverage.Excluded.Files, plain.Coverage.Excluded.Files)
	}
	if want := contract.Digest(contract.Config{Exclude: []string{"sub/**"}}); configured.ConfigDigest != want {
		t.Errorf("digest %s, want %s", configured.ConfigDigest, want)
	}
	if configured.ConfigDigest == plain.ConfigDigest {
		t.Error("the godebt.yaml did not change the digest")
	}
}

func TestAuditRejectsInvalidDefaultConfig(t *testing.T) {
	root := fixtureCopy(t, "functions")
	if err := os.WriteFile(filepath.Join(root, "godebt.yaml"), []byte("scoring: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fe *contract.FieldError
	if _, err := Audit(context.Background(), root, Options{}); !errors.As(err, &fe) || fe.Field != "scoring" {
		t.Errorf("error %v, want a FieldError for scoring", err)
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(path, []byte("policy:\n  maxIndex: 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Policy.MaxIndex == nil || *cfg.Policy.MaxIndex != 7 {
		t.Errorf("LoadConfig = %+v, %v", cfg, err)
	}
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing named config: error %v, want fs.ErrNotExist", err)
	}
	if cfg, err := LoadDefaultConfig(t.TempDir()); cfg != nil || err != nil {
		t.Errorf("LoadDefaultConfig without godebt.yaml = %+v, %v, want nil, nil", cfg, err)
	}
}

func TestLoadReport(t *testing.T) {
	r, err := LoadReport("testdata/golden/report-functions.json")
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo.Module != "example.com/functions" || len(r.Findings) == 0 {
		t.Errorf("module %q with %d findings", r.Repo.Module, len(r.Findings))
	}
	if _, err := LoadReport(filepath.Join(t.TempDir(), "absent.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing report: error %v, want fs.ErrNotExist", err)
	}
	if _, err := LoadReport("godebt.go"); err == nil {
		t.Error("a Go file loaded as a report")
	}
}

func TestEvaluateRefusedBaselineFails(t *testing.T) {
	current := audit(t, "testdata/fixtures/functions")
	baseline := *current
	baseline.AnalyzerVersion = "0.0.1"
	result := Evaluate(Policy{}, current, &baseline)
	if !result.Failed || !strings.HasPrefix(result.Refusal, "analyzerVersion differs") {
		t.Errorf("result = %+v, want a failure that carries the refusal", result)
	}
}

func TestEvaluateWithBaseline(t *testing.T) {
	root := fixtureCopy(t, "functions")
	cc, err := os.ReadFile(filepath.Join(root, "cc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "cc.go")); err != nil {
		t.Fatal(err)
	}
	baseline := audit(t, root)
	if err := os.WriteFile(filepath.Join(root, "cc.go"), cc, 0o600); err != nil {
		t.Fatal(err)
	}
	current := audit(t, root)

	cmp := Compare(baseline, current)
	if !cmp.Comparable || len(cmp.New) == 0 {
		t.Fatalf("comparison = %+v, want new findings from cc.go", cmp)
	}
	p := Policy{FailOnNew: []string{"complexity.hotspot"}}
	result := Evaluate(p, current, baseline)
	if !result.Failed || len(result.Reasons) == 0 || result.Reasons[0].Check != "failOnNew" {
		t.Errorf("result = %+v, want failOnNew reasons", result)
	}
	if result = Evaluate(p, current, nil); result.Failed || len(result.Skipped) != 1 {
		t.Errorf("without a baseline = %+v, want a pass with failOnNew skipped", result)
	}
	zero := 0
	p.Regression = &config.Regression{MaxIncrease: &zero}
	if result = Evaluate(p, current, current); result.Failed || len(result.Skipped) != 0 {
		t.Errorf("against itself = %+v, want a pass", result)
	}
}
