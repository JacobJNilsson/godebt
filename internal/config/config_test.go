package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// specExample is the configuration example of spec 003.
const specExample = `
exclude:
  - "internal/gen/**"
classify:
  test:
    - "internal/testutil/**"
policy:
  maxIndex: 40
  regression:
    maxIncrease: 2
    maxIncreasePercent: 10
  budgets:
    duplication.density.production: { max: 0.05 }
  failOnNew:
    - complexity.hotspot
`

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "godebt.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, content string) (*File, error) {
	t.Helper()
	return Load(write(t, content))
}

func TestLoadSpecExample(t *testing.T) {
	f, err := load(t, specExample)
	if err != nil {
		t.Fatal(err)
	}
	maxIndex, maxIncrease, maxIncreasePercent, maxDensity := 40, 2, 10.0, 0.05
	want := &File{
		Exclude:  []string{"internal/gen/**"},
		Classify: Classify{Test: []string{"internal/testutil/**"}},
		Policy: Policy{
			MaxIndex:   &maxIndex,
			Regression: &Regression{MaxIncrease: &maxIncrease, MaxIncreasePercent: &maxIncreasePercent},
			Budgets:    map[string]Budget{"duplication.density.production": {Max: &maxDensity}},
			FailOnNew:  []string{"complexity.hotspot"},
		},
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("Load = %+v, want %+v", f, want)
	}
}

func TestLoadAbsentKeysStayNil(t *testing.T) {
	f, err := load(t, "policy:\n  maxIndex: 0\n")
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	if want := (Policy{MaxIndex: &zero}); !reflect.DeepEqual(f.Policy, want) {
		t.Errorf("policy = %+v, want a set maxIndex 0 and nothing else", f.Policy)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	f, err := load(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Policy.MaxIndex != nil || len(f.Exclude) != 0 {
		t.Errorf("empty file = %+v", f)
	}
}

func TestLoadMissingNamedFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}

func TestLoadDefault(t *testing.T) {
	root := t.TempDir()
	if got := Default(root); got != filepath.Join(root, "godebt.yaml") {
		t.Errorf("Default = %q", got)
	}
	f, err := LoadDefault(root)
	if f != nil || err != nil {
		t.Errorf("missing default = %v, %v, want nil, nil", f, err)
	}
	if err := os.WriteFile(Default(root), []byte("policy:\n  maxIndex: 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err = LoadDefault(root)
	if err != nil || f == nil || *f.Policy.MaxIndex != 3 {
		t.Errorf("present default = %+v, %v", f, err)
	}
}

func TestLoadUnknownKeys(t *testing.T) {
	cases := map[string]string{
		"colour":                        "colour: red\n",
		"classify.prod":                 "classify:\n  prod: [a]\n",
		"policy.weights":                "policy:\n  weights: {a: 1}\n",
		"scoring":                       "scoring:\n  saturatesAt: 3\n",
		"policy.regression.maxDecrease": "policy:\n  regression:\n    maxDecrease: 1\n",
		"policy.budgets.duplication.density.production.min": "policy:\n  budgets:\n    duplication.density.production: {min: 1}\n",
	}
	for want, content := range cases {
		t.Run(want, func(t *testing.T) {
			_, err := load(t, content)
			assertField(t, err, want)
		})
	}
}

func TestLoadTypeError(t *testing.T) {
	_, err := load(t, "policy:\n  maxIndex: high\n")
	if err == nil {
		t.Fatal("want an error for a string maxIndex")
	}
}

func TestLoadRejectsSecondDocument(t *testing.T) {
	_, err := load(t, "exclude: [a]\n---\nexclude: [b]\n")
	if err == nil {
		t.Fatal("want an error for a second YAML document")
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]string{
		"policy.maxIndex":                              "policy:\n  maxIndex: 101\n",
		"policy.regression.maxIncrease":                "policy:\n  regression:\n    maxIncrease: -1\n",
		"policy.regression.maxIncreasePercent":         "policy:\n  regression:\n    maxIncreasePercent: -0.5\n",
		"policy.budgets.duplication.density":           "policy:\n  budgets:\n    duplication.density: {max: 1}\n",
		"policy.budgets.a.b.vendored":                  "policy:\n  budgets:\n    a.b.vendored: {max: 1}\n",
		"policy.budgets.a..test":                       "policy:\n  budgets:\n    a..test: {max: 1}\n",
		"policy.budgets.complexity.functions.test.max": "policy:\n  budgets:\n    complexity.functions.test: {max: -1}\n",
		"policy.budgets.foo.bar.production":            "policy:\n  budgets:\n    foo.bar.production: {max: 1}\n",
		"policy.budgets.erosion.mass.test.max":         "policy:\n  budgets:\n    erosion.mass.test: {max: .nan}\n",
		"policy.failOnNew[1]":                          "policy:\n  failOnNew: [complexity.hotspot, complexity.file]\n",
		"exclude[0]":                                   "exclude: [\"internal/[gen\"]\n",
		"classify.test[1]":                             "classify:\n  test: [\"a/**\", \"\"]\n",
	}
	for want, content := range cases {
		t.Run(want, func(t *testing.T) {
			_, err := load(t, content)
			assertField(t, err, want)
		})
	}
	_, err := load(t, "policy:\n  maxIndex: -1\n")
	assertField(t, err, "policy.maxIndex")
	_, err = load(t, "policy:\n  regression:\n    maxIncreasePercent: .nan\n")
	assertField(t, err, "policy.regression.maxIncreasePercent")
	for _, budget := range []string{"{}", "{max: null}", ""} {
		_, err = load(t, "policy:\n  budgets:\n    erosion.mass.production: "+budget+"\n")
		assertField(t, err, "policy.budgets.erosion.mass.production.max")
	}
}

func TestIntegerKnobsRejectFractions(t *testing.T) {
	cases := map[string]string{
		"policy.maxIndex":               "policy:\n  maxIndex: 2.5\n",
		"policy.regression.maxIncrease": "policy:\n  regression:\n    maxIncrease: 0.5\n",
		"policy.maxIndex ":              "exclude: [&a 2.5]\npolicy:\n  maxIndex: *a\n",
		"policy.maxIndex  ":             "policy:\n  maxIndex: !!int 2.5\n",
	}
	for field, content := range cases {
		field = strings.TrimSpace(field)
		_, err := load(t, content)
		assertField(t, err, field)
		var fe *contract.FieldError
		if errors.As(err, &fe) && fe.Reason != "must be a whole number" {
			t.Errorf("%s reason = %q", field, fe.Reason)
		}
	}
}

func TestEmptyRegressionIsDeclared(t *testing.T) {
	for _, content := range []string{"policy:\n  regression:\n", "policy:\n  regression: {}\n", "policy:\n  regression: null\n"} {
		f, err := load(t, content)
		if err != nil {
			t.Fatal(err)
		}
		if f.Policy.Regression == nil || f.Policy.Regression.MaxIncrease != nil || f.Policy.Regression.MaxIncreasePercent != nil {
			t.Errorf("%q: regression = %+v, want declared and empty", content, f.Policy.Regression)
		}
	}
	f, err := load(t, "policy:\n  maxIndex: 3\n")
	if err != nil || f.Policy.Regression != nil {
		t.Errorf("absent regression = %+v, %v, want nil", f.Policy.Regression, err)
	}
}

func TestValidationAcceptsBounds(t *testing.T) {
	content := "exclude: [\"a/*.go\", \"**/gen/**\"]\npolicy:\n  maxIndex: 100\n  regression: {maxIncrease: 0, maxIncreasePercent: 0}\n" +
		"  budgets:\n    complexity.functions.test: {max: 0}\n  failOnNew: [duplication.clone-group]\n"
	if _, err := load(t, content); err != nil {
		t.Fatal(err)
	}
}

func TestDigestIgnoresPolicy(t *testing.T) {
	a, err := load(t, "exclude: [b, a]\nclassify:\n  test: [t/**]\npolicy:\n  maxIndex: 10\n")
	if err != nil {
		t.Fatal(err)
	}
	b, err := load(t, "exclude: [a, b]\nclassify:\n  test: [t/**]\npolicy:\n  maxIndex: 90\n  failOnNew: [complexity.hotspot]\n")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Digest(a.Contract()) != contract.Digest(b.Contract()) {
		t.Error("files that differ only in policy must have the same digest")
	}
	c, err := load(t, "exclude: [a]\nclassify:\n  test: [t/**]\n")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Digest(a.Contract()) == contract.Digest(c.Contract()) {
		t.Error("files that differ in exclude must differ in digest")
	}
}

func TestContractOfNilFile(t *testing.T) {
	var f *File
	if got := f.Contract(); len(got.Exclude) != 0 || len(got.TestPatterns) != 0 {
		t.Errorf("nil file contract = %+v", got)
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	var fe *contract.FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want a FieldError for %s", err, field)
	}
	if fe.Field != field {
		t.Errorf("field = %q, want %q (%v)", fe.Field, field, fe)
	}
}
