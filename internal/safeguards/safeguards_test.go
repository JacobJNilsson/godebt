package safeguards

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const testdata = "../../testdata"

func walk(t *testing.T, root string) *discover.Tree {
	t.Helper()
	tree, err := discover.Walk(context.Background(), root, discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func read(t *testing.T, tree *discover.Tree) *Surfaces {
	t.Helper()
	return Read(tree)
}

// readFiles writes each file under a new root and reads the surfaces of that root.
func readFiles(t *testing.T, files map[string]string) *Surfaces {
	t.Helper()
	return read(t, walk(t, writeFiles(t, files)))
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReadGolden(t *testing.T) {
	for _, name := range []string{"wired", "loose", "odd", "empty"} {
		t.Run(name, func(t *testing.T) {
			s := read(t, walk(t, filepath.Join(testdata, "fixtures", "safeguards", name)))
			got, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join(testdata, "golden", "surfaces-"+name+".json")
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
		})
	}
}

// TestReadOrder shuffles the file list of the tree, because the sorted lists must not depend on walk order.
func TestReadOrder(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"Makefile":                   "setup:\n\tgit config core.hooksPath .githooks\n",
		".githooks/pre-commit":       "make check\n",
		".githooks/pre-push":         "make check\n",
		".husky/pre-commit":          "make lint\n",
		".husky/pre-push":            "make test\n",
		".github/workflows/a.yml":    "on: push\njobs: {}\n",
		".github/workflows/b.yaml":   "on: [push]\njobs: {}\n",
		".github/workflows/c.yml":    "on: pull_request\njobs: {}\n",
		".github/workflows/z.yml":    "on: [push\n",
		"lefthook.yml":               "pre-push:\n  commands: {}\n",
		".lefthook.yaml":             "pre-commit: {}\n",
		".lefthook.yml":              "pre-commit: [\n",
		".pre-commit-config.yaml":    "repos: []\n",
		".claude/settings.json":      "{",
		".github/workflows/nested/x": "ignored\n",
	})
	tree := walk(t, root)
	want := read(t, tree)
	if len(want.Workflows) != 3 || len(want.HookFiles) != 4 || len(want.HookTools) != 3 || len(want.Errors) != 3 {
		t.Fatalf("fixture yields %d workflows, %d hook files, %d hook tools, %d errors", len(want.Workflows), len(want.HookFiles), len(want.HookTools), len(want.Errors))
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(tree.Files), func(a, b int) { tree.Files[a], tree.Files[b] = tree.Files[b], tree.Files[a] })
		if got := read(t, tree); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffle %d changed the order:\ngot  %+v\nwant %+v", i, got, want)
		}
	}
}

func TestNoExec(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range matches {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); path == "os/exec" {
				t.Errorf("%s imports os/exec, and safeguards never run anything", name)
			}
		}
	}
}

func TestMakefile(t *testing.T) {
	s := readFiles(t, map[string]string{
		"Makefile": strings.Join([]string{
			"A = 1",
			"B := two words",
			"export C ?= 3",
			"-include other.mk",
			"all build: dep",
			"\t# A recipe comment.",
			"\tgo build ./...",
			"",
			"gen:",
			"\tinclude_it=`date`",
			"%.o: %.c",
			"\tcc -c $<",
			".PHONY: all",
			"\tmake hidden",
			"  not a recipe",
			"A += more",
			"build: gen",
			"\tgo vet ./...",
		}, "\n"),
	})
	want := &Makefile{
		Path:     "Makefile",
		Includes: true,
		Vars:     []Var{{Name: "A", Value: "1", Line: 1}, {Name: "B", Value: "two words", Line: 2}, {Name: "C", Value: "3", Line: 3}, {Name: "A", Value: "more", Line: 16}},
		Targets: []Target{
			{Name: "all", Line: 5, Prereqs: []string{"dep"}, Recipe: []Command{{Line: 7, Text: "go build ./...", Kind: KindGoBuild}}},
			{Name: "build", Line: 5, Prereqs: []string{"dep", "gen"}, Recipe: []Command{{Line: 7, Text: "go build ./...", Kind: KindGoBuild}, {Line: 18, Text: "go vet ./...", Kind: KindGoVet}}},
			{Name: "gen", Line: 9, Prereqs: []string{}, Recipe: []Command{{Line: 10, Text: "include_it=`date`", Kind: KindOther}}, Unknown: true},
			{Name: "%.o", Line: 11, Prereqs: []string{"%.c"}, Recipe: []Command{{Line: 12, Text: "cc -c $<", Kind: KindOther}}},
		},
	}
	if !reflect.DeepEqual(s.Makefile, want) {
		t.Errorf("Makefile = %+v, want %+v", s.Makefile, want)
	}
}

// TestInclude asserts that the word include inside a recipe is no include directive.
func TestInclude(t *testing.T) {
	s := readFiles(t, map[string]string{"Makefile": "test:\n\tgo test -run TestInclude ./...\n"})
	if s.Makefile.Includes || s.Makefile.Targets[0].Unknown {
		t.Errorf("Makefile = %+v, want no include and a known target", s.Makefile)
	}
	s = readFiles(t, map[string]string{"Makefile": "ifdef CI\n  include ci.mk\nendif\n"})
	if !s.Makefile.Includes {
		t.Error("an indented include inside a conditional does not set Includes")
	}
	for _, directive := range []string{"include a.mk", "sinclude a.mk"} {
		s := readFiles(t, map[string]string{"Makefile": directive + "\n"})
		if !s.Makefile.Includes {
			t.Errorf("%q does not set Includes", directive)
		}
	}
}

// TestConditionals asserts that a conditional directive keeps the recipe of the open target.
func TestConditionals(t *testing.T) {
	s := readFiles(t, map[string]string{"Makefile": "check:\nifeq ($(CI),true)\n\tgo vet ./...\nelse\n\tgo test ./...\nendif\n"})
	want := []Command{{Line: 3, Text: "go vet ./...", Kind: KindGoVet}, {Line: 5, Text: "go test ./...", Kind: KindGoTest}}
	if !reflect.DeepEqual(s.Makefile.Targets[0].Recipe, want) {
		t.Errorf("recipe = %+v, want %+v", s.Makefile.Targets[0].Recipe, want)
	}
}

func TestMakefilePreference(t *testing.T) {
	s := readFiles(t, map[string]string{"GNUmakefile": "a:\n", "Makefile": "b:\n"})
	if s.Makefile == nil || s.Makefile.Path != "GNUmakefile" {
		t.Errorf("Makefile = %+v, want GNUmakefile, which make reads first", s.Makefile)
	}
}

func TestWorkflowOn(t *testing.T) {
	cases := map[string][]string{
		"on: push\n":                       {"push"},
		"on: [push, pull_request]\n":       {"push", "pull_request"},
		"on:\n  push:\n  schedule: []\n":   {"push", "schedule"},
		"name: x\n":                        {},
		"on:\n  - push\n  - {a: b}\n":      {"push"},
		"":                                 {},
		"- a list at the top\n":            {},
		"on: {push: {}, pull_request: {}}": {"push", "pull_request"},
	}
	for src, want := range cases {
		s := readFiles(t, map[string]string{".github/workflows/w.yml": src})
		if len(s.Workflows) != 1 || !reflect.DeepEqual(s.Workflows[0].On, want) {
			t.Errorf("on of %q = %+v, want %v", src, s.Workflows, want)
		}
	}
}

func TestWorkflowSteps(t *testing.T) {
	src := strings.Join([]string{
		"on: push",
		"jobs:",
		"  a:",
		"    steps:",
		"      - uses: actions/checkout@v4",
		"      - run: make check",
		"      - run: >",
		"          go vet ./...",
		"      - run: |",
		"          # A comment.",
		"          go test \\",
		"            ./...",
		"          echo ${{ github.sha }}",
		"      - not a step",
		"  b: not a job",
		"  c:",
		"    uses: ./.github/workflows/reuse.yml",
	}, "\n")
	s := readFiles(t, map[string]string{".github/workflows/w.yaml": src})
	want := []Step{
		{Line: 5, Run: []Command{}, Uses: "actions/checkout@v4"},
		{Line: 6, Run: []Command{{Line: 6, Text: "make check", Kind: KindMake, Ref: "check"}}},
		{Line: 7, Run: []Command{{Line: 8, Text: "go vet ./...", Kind: KindGoVet}}},
		{Line: 9, Run: []Command{
			{Line: 11, Text: "go test ./...", Kind: KindGoTest},
			{Line: 13, Text: "echo ${{ github.sha }}", Kind: KindOther},
		}, Unverified: true},
	}
	if len(s.Workflows) != 1 || !reflect.DeepEqual(s.Workflows[0].Steps, want) {
		t.Errorf("steps = %+v, want %+v", s.Workflows, want)
	}
}

func TestHookTools(t *testing.T) {
	s := readFiles(t, map[string]string{
		"lefthook.yaml":           "pre-push: {}\npre-commit: {}\ncommit-msg: {}\n",
		".pre-commit-config.yaml": "default_install_hook_types: [pre-commit, pre-push]\n",
	})
	want := []HookTool{
		{Path: ".pre-commit-config.yaml", Tool: "pre-commit", Hooks: []string{"pre-commit", "pre-push"}},
		{Path: "lefthook.yaml", Tool: "lefthook", Hooks: []string{"pre-commit", "pre-push"}},
	}
	if !reflect.DeepEqual(s.HookTools, want) {
		t.Errorf("hook tools = %+v, want %+v", s.HookTools, want)
	}
}

func TestLintConfig(t *testing.T) {
	s := readFiles(t, map[string]string{".golangci.toml": "", ".golangci.json": "{}"})
	want := &contract.Location{Path: ".golangci.toml", Line: 1}
	if !reflect.DeepEqual(s.LintConfig, want) {
		t.Errorf("lint config = %+v, want %+v", s.LintConfig, want)
	}
}

func TestAgentHooks(t *testing.T) {
	cases := []struct {
		src  string
		want *AgentHooks
		errs int
	}{
		{`{"permissions": {}}`, nil, 0},
		{"{\n  \"hooks\": {}\n}", &AgentHooks{Path: ".claude/settings.json", Line: 2, Count: 0}, 0},
		{`{"hooks": {"Stop": [{"hooks": [{}, {}]}, {}], "PreToolUse": [{"matcher": "Edit", "hooks": [{}]}]}}`, &AgentHooks{Path: ".claude/settings.json", Line: 1, Count: 3}, 0},
		{`{"hooks": []}`, nil, 1},
		{`not json`, nil, 1},
	}
	for _, c := range cases {
		s := readFiles(t, map[string]string{".claude/settings.json": c.src})
		if !reflect.DeepEqual(s.AgentHooks, c.want) || len(s.Errors) != c.errs {
			t.Errorf("agent hooks of %q = %+v with errors %+v, want %+v with %d errors", c.src, s.AgentHooks, s.Errors, c.want, c.errs)
		}
	}
}

func TestHookFileSources(t *testing.T) {
	s := readFiles(t, map[string]string{
		"Makefile":              "setup:\n\tgit config core.hooksPath hooks && git config core.hooksPath .husky\n",
		"hooks/pre-commit":      "make check\n",
		"hooks/commit-msg":      "make check\n",
		".husky/pre-push":       "make check\n",
		".git/hooks/pre-commit": "make check\n",
	})
	var got []string
	for _, h := range s.HookFiles {
		got = append(got, h.Path+" "+h.Hook)
	}
	want := []string{".husky/pre-push pre-push", "hooks/pre-commit pre-commit"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hook files = %v, want %v", got, want)
	}
}

func TestUnreadableFile(t *testing.T) {
	root := writeFiles(t, map[string]string{"Makefile": "a:\n", ".golangci.yml": ""})
	tree := walk(t, root)
	if err := os.Chmod(filepath.Join(root, "Makefile"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "Makefile"), 0o644) })
	if _, err := os.ReadFile(filepath.Join(root, "Makefile")); err == nil {
		t.Skip("the test runs with permission to read any file")
	}
	s := read(t, tree)
	want := []ReadError{{Path: "Makefile", Message: "open: permission denied"}}
	if s.Makefile != nil || !reflect.DeepEqual(s.Errors, want) || s.LintConfig == nil {
		t.Errorf("surfaces = %+v, want no Makefile, errors %+v, and a lint config", s, want)
	}
	safeguards, _ := Evaluate(s, treeExists(tree))
	for _, g := range safeguards {
		wantUnknown := g.ID != IDAgentHooks && g.ID != IDCIWorkflow
		if (g.Evidence == contract.EvidenceUnknown) != wantUnknown {
			t.Errorf("%s = %s after a Makefile read error", g.ID, g.Evidence)
		}
	}
}
