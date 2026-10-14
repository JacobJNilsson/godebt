package safeguards

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// The safeguard ids of spec 002.
const (
	IDAgentHooks     = "agent-hooks"
	IDCIWorkflow     = "ci-workflow"
	IDCoverageBudget = "coverage-budget"
	IDLintConfig     = "lint-config"
	IDPreCommitHook  = "pre-commit-hook"
	IDPrePushHook    = "pre-push-hook"
	IDTestCheck      = "test-check"
	IDVetCheck       = "vet-check"
)

// IDs lists the eight safeguard ids in the order that a report lists them.
var IDs = []string{IDAgentHooks, IDCIWorkflow, IDCoverageBudget, IDLintConfig, IDPreCommitHook, IDPrePushHook, IDTestCheck, IDVetCheck}

// result is the level that one rule reaches, with the locations and the note that explain it.
type result struct {
	level     contract.Evidence
	locations []contract.Location
	note      string
}

// rank orders the levels, and unknown sits above configured because an unverified line may hide the wiring.
var rank = map[contract.Evidence]int{
	contract.EvidenceAbsent:            0,
	contract.EvidenceConfigured:        1,
	contract.EvidenceUnknown:           2,
	contract.EvidenceStructurallyWired: 3,
}

// better returns b when it reaches a higher level than a, and a otherwise.
func better(a, b result) result {
	if rank[b.level] > rank[a.level] {
		return b
	}
	return a
}

// hookSetter is a recipe line that sets core.hooksPath.
type hookSetter struct {
	at     contract.Location
	target string
}

type evaluator struct {
	s       *Surfaces
	exists  func(rel string) bool
	targets map[string]Target
	// hooksPath maps each directory that a recipe sets as core.hooksPath to the first recipe line that sets it.
	hooksPath map[string]hookSetter
	// installers maps a hook tool name to the first command or step that installs its hooks.
	installers map[string]contract.Location
	points     []*enforcement
}

// Evaluate derives the eight safeguards of spec 002, and exists reports whether a repo-relative path is a file of the tree.
func Evaluate(s *Surfaces, exists func(rel string) bool) ([]contract.Safeguard, []contract.Finding) {
	e := newEvaluator(s, exists)
	rules := map[string]func() result{
		IDAgentHooks:     e.agentHooks,
		IDCIWorkflow:     e.ciWorkflow,
		IDCoverageBudget: e.coverageBudget,
		IDLintConfig:     e.lintConfig,
		IDPreCommitHook:  func() result { return e.hook("pre-commit") },
		IDPrePushHook:    func() result { return e.hook("pre-push") },
		IDTestCheck:      func() result { return e.goCheck(KindGoTest, "go test") },
		IDVetCheck:       func() result { return e.goCheck(KindGoVet, "go vet") },
	}
	safeguards := make([]contract.Safeguard, 0, len(rules))
	for id, rule := range rules {
		r := better(rule(), e.readErrors(id))
		safeguards = append(safeguards, contract.Safeguard{ID: id, Evidence: r.level, Locations: dedupe(r.locations), Notes: r.note})
	}
	sort.Slice(safeguards, func(i, j int) bool { return safeguards[i].ID < safeguards[j].ID })
	return safeguards, e.brokenReferences()
}

func newEvaluator(s *Surfaces, exists func(rel string) bool) *evaluator {
	e := &evaluator{s: s, exists: exists, targets: map[string]Target{}, hooksPath: map[string]hookSetter{}, installers: map[string]contract.Location{}}
	if m := s.Makefile; m != nil {
		for _, t := range m.Targets {
			e.targets[t.Name] = t
			for _, c := range t.Recipe {
				e.noteSetup(c, contract.Location{Path: m.Path, Line: c.Line}, t.Name)
			}
		}
	}
	for _, w := range s.Workflows {
		for _, step := range w.Steps {
			at := contract.Location{Path: w.Path, Line: step.Line}
			if strings.HasPrefix(step.Uses, "pre-commit/action@") && hasTrigger(w) {
				e.install("pre-commit", at)
			}
		}
	}
	e.points = e.enforcementPoints()
	return e
}

// noteSetup records a recipe line that sets core.hooksPath or installs hooks, and only Makefile recipes reach it because CI runs in a fresh clone.
func (e *evaluator) noteSetup(c Command, at contract.Location, target string) {
	switch c.Kind {
	case KindHooksPath:
		if _, ok := e.hooksPath[c.Ref]; !ok {
			e.hooksPath[c.Ref] = hookSetter{at: at, target: target}
		}
	case KindLefthookInstall:
		e.install("lefthook", at)
	case KindPreCommitInstall:
		e.install("pre-commit", at)
	}
}

func (e *evaluator) install(tool string, at contract.Location) {
	if _, ok := e.installers[tool]; !ok {
		e.installers[tool] = at
	}
}

// broken reports whether a command names a missing make target or script, and a reference that holds a variable is never broken.
func (e *evaluator) broken(c Command) bool {
	if strings.Contains(c.Ref, "$") {
		return false
	}
	switch c.Kind {
	case KindMake:
		// An included file may define the target, so the inspector cannot call it missing.
		if e.s.Makefile != nil && e.s.Makefile.Includes {
			return false
		}
		_, ok := e.targets[e.makeTarget(c.Ref)]
		return !ok
	case KindScript:
		return !e.exists(c.Ref)
	}
	return false
}

func (e *evaluator) brokenReferences() []contract.Finding {
	findings := []contract.Finding{}
	// seen holds each path, line, and ref once, because a recipe line shared by several targets and a repeated ref are one mistake.
	seen := map[string]bool{}
	add := func(path string, commands []Command) {
		for _, c := range commands {
			key := fmt.Sprintf("%s:%d:%s", path, c.Line, c.Ref)
			if e.broken(c) && !seen[key] {
				seen[key] = true
				findings = append(findings, contract.Finding{
					Kind: contract.KindBrokenReference, Path: path, StartLine: c.Line, EndLine: c.Line,
					Identity: key,
					Facts:    contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: c.Text, Ref: c.Ref}},
				})
			}
		}
	}
	if m := e.s.Makefile; m != nil {
		for _, t := range m.Targets {
			add(m.Path, t.Recipe)
		}
	}
	for _, h := range e.s.HookFiles {
		add(h.Path, h.Lines)
	}
	for _, w := range e.s.Workflows {
		for _, step := range w.Steps {
			add(w.Path, step.Run)
		}
	}
	contract.SortFindings(findings)
	return findings
}

// readErrors makes each safeguard whose surface could not be read unknown, and it names the file in the note.
func (e *evaluator) readErrors(id string) result {
	r := absent("")
	for _, re := range e.s.Errors {
		if affects(re.Path, id) {
			r = better(r, result{contract.EvidenceUnknown, []contract.Location{{Path: re.Path}}, fmt.Sprintf("Reading %s failed: %s.", re.Path, re.Message)})
		}
	}
	return r
}

// affects reports whether a safeguard reads the surface at rel, as the table of spec 002 assigns them.
func affects(rel, id string) bool {
	switch {
	case rel == agentSettings:
		return id == IDAgentHooks
	case strings.HasPrefix(rel, ".github/workflows/"):
		return id != IDAgentHooks
	case slices.Contains(hookToolFiles, rel):
		return id == IDPreCommitHook || id == IDPrePushHook
	}
	return id != IDAgentHooks && id != IDCIWorkflow
}

func absent(note string) result {
	return result{level: contract.EvidenceAbsent, locations: []contract.Location{}, note: note}
}

func dedupe(locations []contract.Location) []contract.Location {
	out := []contract.Location{}
	seen := map[contract.Location]bool{}
	for _, l := range locations {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}
