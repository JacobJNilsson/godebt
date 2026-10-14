package safeguards

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
)

// treeExists answers for the files of a tree, as the audit does.
func treeExists(tree *discover.Tree) func(string) bool {
	files := map[string]bool{}
	for _, f := range tree.Files {
		files[f.Path] = true
	}
	return func(rel string) bool { return files[rel] }
}

// evaluate writes each file under a new root and returns the safeguards by id and the findings.
func evaluate(t *testing.T, files map[string]string) (map[string]contract.Safeguard, []contract.Finding) {
	t.Helper()
	tree := walk(t, writeFiles(t, files))
	safeguards, findings := Evaluate(read(t, tree), treeExists(tree))
	byID := map[string]contract.Safeguard{}
	for _, s := range safeguards {
		byID[s.ID] = s
	}
	return byID, findings
}

func TestInspect(t *testing.T) {
	tree := walk(t, filepath.Join(testdata, "fixtures", "safeguards", "odd"))
	gotS, gotF := Inspect(tree)
	wantS, wantF := Evaluate(read(t, tree), treeExists(tree))
	if !reflect.DeepEqual(gotS, wantS) || !reflect.DeepEqual(gotF, wantF) {
		t.Errorf("Inspect differs from Read and Evaluate")
	}
}

func TestEvaluateGolden(t *testing.T) {
	for _, name := range []string{"wired", "loose", "odd", "empty"} {
		t.Run(name, func(t *testing.T) {
			tree := walk(t, filepath.Join(testdata, "fixtures", "safeguards", name))
			safeguards, _ := Evaluate(read(t, tree), treeExists(tree))
			got, err := json.MarshalIndent(safeguards, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join(testdata, "golden", "safeguards-"+name+".json")
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

// TestEvaluateFixtures pins the level of every safeguard of the four fixtures, so a golden update cannot hide a change of level.
func TestEvaluateFixtures(t *testing.T) {
	const (
		absent  = contract.EvidenceAbsent
		conf    = contract.EvidenceConfigured
		wired   = contract.EvidenceStructurallyWired
		unknown = contract.EvidenceUnknown
	)
	ids := IDs
	for name, want := range map[string][]contract.Evidence{
		"wired": {conf, wired, wired, wired, wired, wired, wired, wired},
		"loose": {absent, absent, absent, absent, absent, absent, conf, absent},
		"odd":   {absent, unknown, absent, absent, unknown, conf, unknown, unknown},
		"empty": {absent, absent, absent, absent, absent, absent, absent, absent},
	} {
		tree := walk(t, filepath.Join(testdata, "fixtures", "safeguards", name))
		safeguards, findings := Evaluate(read(t, tree), treeExists(tree))
		var got []contract.Evidence
		for i, s := range safeguards {
			if s.ID != ids[i] {
				t.Fatalf("%s: safeguard %d is %s, want %s", name, i, s.ID, ids[i])
			}
			got = append(got, s.Evidence)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: levels = %v, want %v", name, got, want)
		}
		wantFindings := map[string]int{"odd": 2}[name]
		if len(findings) != wantFindings {
			t.Errorf("%s: %d findings %+v, want %d", name, len(findings), findings, wantFindings)
		}
	}
}

func TestEvaluateOddFindings(t *testing.T) {
	tree := walk(t, filepath.Join(testdata, "fixtures", "safeguards", "odd"))
	_, findings := Evaluate(read(t, tree), treeExists(tree))
	want := []contract.Finding{
		{Kind: contract.KindBrokenReference, Path: ".githooks/pre-commit", StartLine: 3, EndLine: 3, Identity: ".githooks/pre-commit:3:verify",
			Facts: contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: "make verify", Ref: "verify"}}},
		{Kind: contract.KindBrokenReference, Path: ".githooks/pre-commit", StartLine: 4, EndLine: 4, Identity: ".githooks/pre-commit:4:scripts/missing.sh",
			Facts: contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: "sh scripts/missing.sh", Ref: "scripts/missing.sh"}}},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %+v, want %+v", findings, want)
	}
}

const setupHooks = "setup:\n\tgit config core.hooksPath .githooks\n"

func TestEvaluateLevels(t *testing.T) {
	ci := func(on, run string) string {
		return "on: " + on + "\njobs:\n  a:\n    steps:\n      - run: " + run + "\n"
	}
	cases := []struct {
		name  string
		files map[string]string
		id    string
		want  contract.Evidence
	}{
		{"hook tool installed by make", map[string]string{"Makefile": "setup:\n\tlefthook install\n", "lefthook.yml": "pre-push: {}\n"}, IDPrePushHook, contract.EvidenceStructurallyWired},
		{"hook tool installed by an action", map[string]string{".pre-commit-config.yaml": "repos: []\n", ".github/workflows/a.yml": "on: push\njobs:\n  a:\n    steps:\n      - uses: pre-commit/action@v3\n"}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"hook tool installed by a workflow run", map[string]string{".pre-commit-config.yaml": "repos: []\n", ".github/workflows/a.yml": ci("push", "pre-commit install")}, IDPreCommitHook, contract.EvidenceConfigured},
		{"action in an untriggered workflow", map[string]string{".pre-commit-config.yaml": "repos: []\n", ".github/workflows/a.yml": "on: schedule\njobs:\n  a:\n    steps:\n      - uses: pre-commit/action@v3\n"}, IDPreCommitHook, contract.EvidenceConfigured},
		{"pre-commit installed by make", map[string]string{"Makefile": "setup:\n\tpre-commit install\n", ".pre-commit-config.yaml": "repos: []\n"}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"make with several targets", map[string]string{"Makefile": "lint:\n\tgolangci-lint run\ntest:\n\tgo test ./...\n", ".github/workflows/a.yml": ci("push", "make lint test")}, IDTestCheck, contract.EvidenceStructurallyWired},
		{"target defined twice", map[string]string{"Makefile": "check: vet\ncheck: lint\nvet:\nlint:\n\tgolangci-lint run\n", ".golangci.yml": "", ".github/workflows/a.yml": ci("push", "make check")}, IDLintConfig, contract.EvidenceStructurallyWired},
		{"coverage profile with a space", map[string]string{"Makefile": "test:\n\tgo test -coverprofile c.out ./...\n\tsh gate.sh c.out\n", "gate.sh": "", ".github/workflows/a.yml": ci("push", "make test")}, IDCoverageBudget, contract.EvidenceStructurallyWired},
		{"include caps the wiring", map[string]string{"Makefile": "include rules.mk\ntest:\n\tgo test ./...\n", ".github/workflows/a.yml": ci("push", "make test")}, IDTestCheck, contract.EvidenceUnknown},
		{"include spares a direct step", map[string]string{"Makefile": "include rules.mk\ntest:\n\tgo test ./...\n", ".github/workflows/a.yml": ci("push", "go test ./...")}, IDTestCheck, contract.EvidenceStructurallyWired},
		{"include spares a direct step beside make", map[string]string{"Makefile": "include rules.mk\ntest:\n\tgo test ./...\n", ".github/workflows/a.yml": ci("push", "make test") + "      - run: go test ./...\n"}, IDTestCheck, contract.EvidenceStructurallyWired},
		{"include spares an install recipe", map[string]string{"Makefile": "include rules.mk\nsetup:\n\tpre-commit install\n", ".pre-commit-config.yaml": "repos: []\n"}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"include spares a hooks path recipe", map[string]string{"Makefile": "include rules.mk\n" + setupHooks, ".githooks/pre-commit": "make check\n"}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"hook line hides a check", map[string]string{"Makefile": setupHooks + "vet:\n\tgo vet ./...\n", ".githooks/pre-commit": "make vet | tee log\n"}, IDVetCheck, contract.EvidenceUnknown},
		{"workflow wires past a hidden hook", map[string]string{"Makefile": setupHooks + "vet:\n\tgo vet ./...\n", ".githooks/pre-commit": "make vet | tee log\n", ".github/workflows/a.yml": ci("push", "go vet ./...")}, IDVetCheck, contract.EvidenceStructurallyWired},
		{"uninstalled hook hides nothing", map[string]string{"Makefile": "vet:\n\tgo vet ./...\n", ".husky/pre-commit": "make vet | tee log\n"}, IDVetCheck, contract.EvidenceConfigured},
		{"coverage profile inside a flag", map[string]string{"Makefile": "test:\n\tgo test -coverprofile=cover.out ./...\n\tgo tool cover -func=cover.out\n", ".github/workflows/a.yml": ci("push", "make test")}, IDCoverageBudget, contract.EvidenceStructurallyWired},
		{"untriggered workflow never wires", map[string]string{"Makefile": "vet:\n\tgo vet ./...\n", ".github/workflows/a.yml": "jobs:\n  a:\n    steps:\n      - run: go vet ./...\n"}, IDVetCheck, contract.EvidenceUnknown},
		{"ci with an expression", map[string]string{".github/workflows/a.yml": ci("push", "go test ${{ inputs.flags }}")}, IDCIWorkflow, contract.EvidenceUnknown},
		{"hook tool never installed", map[string]string{"lefthook.yml": "pre-commit: {}\n"}, IDPreCommitHook, contract.EvidenceConfigured},
		{"hook with a broken reference", map[string]string{"Makefile": setupHooks, ".githooks/pre-commit": "make gone\n"}, IDPreCommitHook, contract.EvidenceConfigured},
		{"hooks path set by a workflow only", map[string]string{".githooks/pre-push": "go vet ./...\n", ".husky/pre-push": "go vet ./...\n", ".github/workflows/a.yml": ci("push", "git config core.hooksPath .husky")}, IDPrePushHook, contract.EvidenceConfigured},
		{"hooks path at the root", map[string]string{"Makefile": "setup:\n\tgit config core.hooksPath .\n", "pre-commit": "go vet ./...\n"}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"lint configured", map[string]string{".golangci.yml": ""}, IDLintConfig, contract.EvidenceConfigured},
		{"lint wired by a hook", map[string]string{".golangci.yml": "", "Makefile": setupHooks, ".githooks/pre-commit": "golangci-lint run\n"}, IDLintConfig, contract.EvidenceStructurallyWired},
		{"lint behind an expression", map[string]string{".golangci.yml": "", ".github/workflows/a.yml": ci("push", "golangci-lint run ${{ inputs.args }}")}, IDLintConfig, contract.EvidenceUnknown},
		{"lint in a scheduled workflow", map[string]string{".golangci.yml": "", ".github/workflows/a.yml": ci("schedule", "golangci-lint run")}, IDLintConfig, contract.EvidenceConfigured},
		{"vet through an unknown target", map[string]string{"Makefile": "vet:\n\tgo vet $(shell go list ./...)\n", ".github/workflows/a.yml": ci("push", "make vet")}, IDVetCheck, contract.EvidenceUnknown},
		{"vet through a hook with an other line", map[string]string{"Makefile": setupHooks + "vet:\n\tgo vet ./...\n", ".githooks/pre-commit": "make vet\necho done\n"}, IDVetCheck, contract.EvidenceUnknown},
		{"test through a prerequisite cycle", map[string]string{"Makefile": "all: a\na: b\nb: a\n\tgo test ./...\n", ".github/workflows/a.yml": ci("pull_request", "make")}, IDTestCheck, contract.EvidenceStructurallyWired},
		{"test run directly by a step", map[string]string{".github/workflows/a.yml": ci("[push]", "go test ./...")}, IDTestCheck, contract.EvidenceStructurallyWired},
		{"coverage variable only", map[string]string{"Makefile": "MY_COVERAGE = 80\n"}, IDCoverageBudget, contract.EvidenceConfigured},
		{"coverage profile without a check", map[string]string{"Makefile": "test:\n\tgo test -coverprofile=c.out ./...\n"}, IDCoverageBudget, contract.EvidenceAbsent},
		{"coverage profile check not reached", map[string]string{"Makefile": "test:\n\tgo test -coverprofile=c.out ./...\n\tgo tool cover -func=c.out\n\tsh gate.sh c.out\n"}, IDCoverageBudget, contract.EvidenceConfigured},
		{"coverage run by an unrelated target", map[string]string{"Makefile": "test:\n\tgo test -coverprofile=c.out ./...\n\tsh gate.sh c.out\nother:\n\tgo test -coverprofile=c.out ./...\n", "gate.sh": "", ".github/workflows/a.yml": ci("push", "make other")}, IDCoverageBudget, contract.EvidenceConfigured},
		{"workflow on schedule", map[string]string{".github/workflows/a.yml": ci("schedule", "make check")}, IDCIWorkflow, contract.EvidenceConfigured},
		{"workflow without steps", map[string]string{".github/workflows/a.yml": "on: push\njobs: {}\n"}, IDCIWorkflow, contract.EvidenceAbsent},
		{"workflow with an other line", map[string]string{".github/workflows/a.yml": ci("push", "curl -sSfL example.com")}, IDCIWorkflow, contract.EvidenceStructurallyWired},
		{"agent hooks empty", map[string]string{".claude/settings.json": `{"hooks": {}}`}, IDAgentHooks, contract.EvidenceAbsent},
		{"agent settings unreadable", map[string]string{".claude/settings.json": `{`}, IDAgentHooks, contract.EvidenceUnknown},
		{"workflow error makes lint unknown", map[string]string{".golangci.yml": "", ".github/workflows/a.yml": "["}, IDLintConfig, contract.EvidenceUnknown},
		{"hook tool error makes the hook unknown", map[string]string{"lefthook.yml": "["}, IDPrePushHook, contract.EvidenceUnknown},
		{"read error spares a wired safeguard", map[string]string{"Makefile": setupHooks, ".githooks/pre-commit": "make setup\n", ".github/workflows/a.yml": "["}, IDPreCommitHook, contract.EvidenceStructurallyWired},
		{"workflow error makes ci unknown", map[string]string{".github/workflows/a.yml": "["}, IDCIWorkflow, contract.EvidenceUnknown},
		{"hook tool error spares ci", map[string]string{"lefthook.yml": "["}, IDCIWorkflow, contract.EvidenceAbsent},
		{"hook tool error spares agent hooks", map[string]string{"lefthook.yml": "["}, IDAgentHooks, contract.EvidenceAbsent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := evaluate(t, c.files)
			if len(got) != 8 {
				t.Fatalf("%d safeguards, want 8", len(got))
			}
			if s := got[c.id]; s.Evidence != c.want || s.Notes == "" {
				t.Errorf("%s = %s (%q), want %s with a note", c.id, s.Evidence, s.Notes, c.want)
			}
		})
	}
}

// TestReadErrorScope asserts that a bad file changes only the safeguards that read it, as the table of spec 002 says.
func TestReadErrorScope(t *testing.T) {
	base := map[string]string{
		"Makefile":             setupHooks + "check:\n\tgolangci-lint run\n\tgo vet ./...\n\tgo test -coverprofile=c.out ./...\n\tsh gate.sh c.out\n",
		"gate.sh":              "",
		".golangci.yml":        "",
		".githooks/pre-commit": "make check\n",
	}
	want, _ := evaluate(t, base)
	for bad, changed := range map[string][]string{
		".claude/settings.json": {IDAgentHooks},
		"lefthook.yml":          {IDPrePushHook},
	} {
		files := map[string]string{bad: "["}
		for k, v := range base {
			files[k] = v
		}
		got, _ := evaluate(t, files)
		for id, g := range got {
			if slices.Contains(changed, id) {
				if g.Evidence != contract.EvidenceUnknown {
					t.Errorf("bad %s: %s = %s, want unknown", bad, id, g.Evidence)
				}
				continue
			}
			if !reflect.DeepEqual(g, want[id]) {
				t.Errorf("bad %s changed %s: %+v, want %+v", bad, id, g, want[id])
			}
		}
	}
}

func TestHiddenByHookNote(t *testing.T) {
	got, _ := evaluate(t, map[string]string{"Makefile": setupHooks + "test:\n\tgo test ./...\n", ".githooks/pre-push": "make test > log\n"})
	if n := got[IDTestCheck].Notes; n != "Hook .githooks/pre-push:1 is not understood, so what it runs is unknown." {
		t.Errorf("test-check note = %q", n)
	}
}

func TestIncludeNote(t *testing.T) {
	got, findings := evaluate(t, map[string]string{"Makefile": "include rules.mk\ntest:\n\tgo test ./...\n\tmake gen\n", ".github/workflows/a.yml": "on: push\njobs:\n  a:\n    steps:\n      - run: make test\n"})
	if n := got[IDTestCheck].Notes; n != "The Makefile includes another file, so the inspector cannot see every target." {
		t.Errorf("test-check note = %q", n)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none, because the included file may define gen", findings)
	}
}

func TestEvaluateNotesAndLocations(t *testing.T) {
	got, _ := evaluate(t, map[string]string{"Makefile": setupHooks, ".githooks/pre-commit": "make setup\n"})
	want := contract.Safeguard{
		ID: IDPreCommitHook, Evidence: contract.EvidenceStructurallyWired,
		Locations: []contract.Location{{Path: "Makefile", Line: 2}, {Path: ".githooks/pre-commit"}},
		Notes:     "Target setup sets core.hooksPath to .githooks, and the hook runs make setup.",
	}
	if !reflect.DeepEqual(got[IDPreCommitHook], want) {
		t.Errorf("pre-commit-hook = %+v, want %+v", got[IDPreCommitHook], want)
	}
	got, _ = evaluate(t, map[string]string{".claude/settings.json": `{`})
	if n := got[IDAgentHooks].Notes; n != "Reading .claude/settings.json failed: unexpected end of JSON input." {
		t.Errorf("agent-hooks note = %q", n)
	}
}

func TestBrokenReferences(t *testing.T) {
	_, findings := evaluate(t, map[string]string{
		"Makefile":                "a:\n\t$(MAKE) $(NEXT)\n\tsh $(SCRIPT)\n\t./gone.sh\n\tmake a\n",
		".github/workflows/a.yml": "on: push\njobs:\n  a:\n    steps:\n      - run: make missing\n",
	})
	var got []string
	for _, f := range findings {
		got = append(got, f.Identity)
	}
	want := []string{".github/workflows/a.yml:5:missing", "Makefile:4:gone.sh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("broken references = %v, want %v", got, want)
	}
	_, findings = evaluate(t, map[string]string{".github/workflows/a.yml": "on: push\njobs:\n  a:\n    steps:\n      - run: make\n"})
	if len(findings) != 1 || findings[0].Facts.BrokenReference.Ref != "" {
		t.Errorf("make without a Makefile = %+v, want one broken reference", findings)
	}
}

// TestBrokenReferenceIdentity asserts that two equal broken lines are two findings that each match themselves in a comparison.
func TestBrokenReferenceIdentity(t *testing.T) {
	_, findings := evaluate(t, map[string]string{"Makefile": setupHooks, ".githooks/pre-commit": "make verify\nmake verify\n"})
	if len(findings) != 2 || findings[0].Identity == findings[1].Identity {
		t.Fatalf("findings = %+v, want two with distinct identities", findings)
	}
}

// TestBrokenReferencePerLine asserts one finding for a recipe line that several targets share, and for a ref that one line repeats.
func TestBrokenReferencePerLine(t *testing.T) {
	for _, makefile := range []string{"a b:\n\tmake nope\n", "a:\n\tmake gone && make gone\n"} {
		_, findings := evaluate(t, map[string]string{"Makefile": makefile})
		if len(findings) != 1 {
			t.Errorf("%q gives %d findings %+v, want 1", makefile, len(findings), findings)
		}
	}
}

// TestEvaluateOrder asserts that the safeguards and the findings come out sorted however the rules run.
func TestEvaluateOrder(t *testing.T) {
	files := map[string]string{
		"Makefile":             setupHooks + "z:\n\tmake gone\n",
		".githooks/pre-commit": "sh b.sh\nsh a.sh\nmake zz\n",
	}
	for i := 0; i < 10; i++ {
		safeguards, findings := func() ([]contract.Safeguard, []contract.Finding) {
			tree := walk(t, writeFiles(t, files))
			return Evaluate(read(t, tree), treeExists(tree))
		}()
		if !sort.SliceIsSorted(safeguards, func(a, b int) bool { return safeguards[a].ID < safeguards[b].ID }) {
			t.Fatalf("safeguards out of order: %+v", safeguards)
		}
		if i := contract.UnsortedFinding(findings); i >= 0 || len(findings) != 4 {
			t.Fatalf("findings out of order at %d: %+v", i, findings)
		}
	}
}
