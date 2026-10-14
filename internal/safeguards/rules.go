package safeguards

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// hook applies the rule of pre-commit-hook and pre-push-hook to every hook file and hook tool that declares the hook.
func (e *evaluator) hook(name string) result {
	r := absent("No hook file or hook tool declares " + name + ".")
	for _, h := range e.s.HookFiles {
		if h.Hook == name {
			r = better(r, e.hookFile(h))
		}
	}
	for _, t := range e.s.HookTools {
		if slices.Contains(t.Hooks, name) {
			r = better(r, e.hookTool(t, name))
		}
	}
	return r
}

func (e *evaluator) hookFile(h HookFile) result {
	file := contract.Location{Path: h.Path}
	if c := firstOther(h.Lines); c != nil {
		return result{contract.EvidenceUnknown, []contract.Location{{Path: h.Path, Line: c.Line}}, fmt.Sprintf("Line %d of %s is in no understood form.", c.Line, h.Path)}
	}
	dir := dirOf(h.Path)
	setter, ok := e.hooksPath[dir]
	if !ok {
		return result{contract.EvidenceConfigured, []contract.Location{file}, fmt.Sprintf("%s exists, and no Makefile recipe sets core.hooksPath to %s.", h.Path, dir)}
	}
	for _, c := range h.Lines {
		if e.broken(c) {
			return result{contract.EvidenceConfigured, []contract.Location{{Path: h.Path, Line: c.Line}}, fmt.Sprintf("%s names %s, which does not exist.", h.Path, c.Ref)}
		}
	}
	return result{contract.EvidenceStructurallyWired, []contract.Location{setter.at, file},
		fmt.Sprintf("Target %s sets core.hooksPath to %s, and the hook runs %s.", setter.target, dir, texts(h.Lines))}
}

func (e *evaluator) hookTool(t HookTool, name string) result {
	file := contract.Location{Path: t.Path}
	at, ok := e.installers[t.Tool]
	if !ok {
		return result{contract.EvidenceConfigured, []contract.Location{file}, fmt.Sprintf("%s declares %s, and nothing installs the %s hooks.", t.Path, name, t.Tool)}
	}
	return result{contract.EvidenceStructurallyWired, []contract.Location{at, file}, fmt.Sprintf("%s declares %s, and %s:%d installs the %s hooks.", t.Path, name, at.Path, at.Line, t.Tool)}
}

func (e *evaluator) lintConfig() result {
	if e.s.LintConfig == nil {
		return absent("No .golangci file exists.")
	}
	base := result{contract.EvidenceConfigured, []contract.Location{*e.s.LintConfig}, e.s.LintConfig.Path + " exists, and no enforcement point runs golangci-lint."}
	return e.wire(base, func(c located) bool { return c.Kind == KindGolangci }, "golangci-lint")
}

// goCheck applies the rule of vet-check and test-check.
func (e *evaluator) goCheck(kind CommandKind, what string) result {
	base := absent("No Makefile recipe runs " + what + ".")
	if m := e.s.Makefile; m != nil {
		if c, ok := firstInRecipes(m, func(c Command) bool { return c.Kind == kind }); ok {
			base = result{contract.EvidenceConfigured, []contract.Location{{Path: m.Path, Line: c.Line}}, "A Makefile recipe runs " + what + ", and no enforcement point reaches it."}
		}
	}
	return e.hiddenByHook(e.wire(base, func(c located) bool { return c.Kind == kind }, what))
}

// hiddenByHook raises a configured Makefile check to unknown when an installed hook holds a line the inspector cannot read, which may run the check.
func (e *evaluator) hiddenByHook(r result) result {
	if r.level != contract.EvidenceConfigured {
		return r
	}
	for _, h := range e.s.HookFiles {
		if _, installed := e.hooksPath[dirOf(h.Path)]; !installed {
			continue
		}
		if c := firstOther(h.Lines); c != nil {
			return result{contract.EvidenceUnknown, append(r.locations, contract.Location{Path: h.Path, Line: c.Line}),
				fmt.Sprintf("Hook %s:%d is not understood, so what it runs is unknown.", h.Path, c.Line)}
		}
	}
	return r
}

func (e *evaluator) coverageBudget() result {
	m := e.s.Makefile
	if m == nil {
		return absent("No Makefile exists.")
	}
	base := absent("No Makefile variable or recipe sets a coverage budget.")
	if i := slices.IndexFunc(m.Vars, func(v Var) bool { return strings.Contains(v.Name, "COVERAGE") }); i >= 0 {
		v := m.Vars[i]
		base = result{contract.EvidenceConfigured, []contract.Location{{Path: m.Path, Line: v.Line}}, "Variable " + v.Name + " sets a coverage budget, and no enforcement point reaches a coverage check."}
	}
	checks := coverageChecks(m)
	for _, t := range m.Targets {
		if line, ok := checks[t.Name]; ok && base.level == contract.EvidenceAbsent {
			base = result{contract.EvidenceConfigured, []contract.Location{{Path: m.Path, Line: line}}, "Target " + t.Name + " checks a coverage profile, and no enforcement point reaches it."}
		}
	}
	return e.hiddenByHook(e.wire(base, func(c located) bool {
		line, ok := checks[c.Target]
		return ok && c.Line == line
	}, "the coverage check"))
}

// coverageChecks maps each target whose recipe writes -coverprofile=<file> and later names <file> to the line that writes the profile.
func coverageChecks(m *Makefile) map[string]int {
	checks := map[string]int{}
	for _, t := range m.Targets {
		for i, c := range t.Recipe {
			profile := coverProfile(c.Text)
			if profile == "" {
				continue
			}
			if slices.ContainsFunc(t.Recipe[i+1:], func(later Command) bool { return strings.Contains(later.Text, profile) }) {
				checks[t.Name] = c.Line
				break
			}
		}
	}
	return checks
}

// coverProfile returns the file of -coverprofile=<file> or -coverprofile <file>, or "" when the text names none.
func coverProfile(text string) string {
	fields := strings.Fields(text)
	for i, f := range fields {
		if profile, ok := strings.CutPrefix(f, "-coverprofile="); ok {
			return profile
		}
		if f == "-coverprofile" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// wire raises base to structurally-wired when a clean enforcement point reaches a matching command, and to unknown when only an unverified one does.
func (e *evaluator) wire(base result, match func(located) bool, what string) result {
	wired, unverified, hit := e.wiring(match)
	switch {
	case wired != nil:
		return result{contract.EvidenceStructurallyWired, append(base.locations, wired.at, hit.location()),
			fmt.Sprintf("%s reaches %s at %s:%d.", describe(wired.at), what, hit.Path, hit.Line)}
	case unverified != nil:
		note := fmt.Sprintf("%s reaches %s, and the inspector cannot verify that step or a line on its path.", describe(unverified.at), what)
		if unverified.verified && !unverified.throughUnknown {
			note = "The Makefile includes another file, so the inspector cannot see every target."
		}
		return better(base, result{contract.EvidenceUnknown, append(base.locations, unverified.at, hit.location()), note})
	}
	return base
}

func (e *evaluator) ciWorkflow() result {
	r := absent("No workflow file has a step.")
	for _, w := range e.s.Workflows {
		if len(w.Steps) > 0 {
			r = better(r, workflowLevel(w))
		}
	}
	return r
}

func workflowLevel(w Workflow) result {
	file := contract.Location{Path: w.Path}
	if len(w.On) == 0 {
		return result{contract.EvidenceUnknown, []contract.Location{file}, w.Path + " has no on key."}
	}
	for _, step := range w.Steps {
		if step.Unverified {
			return result{contract.EvidenceUnknown, []contract.Location{{Path: w.Path, Line: step.Line}}, fmt.Sprintf("The step at %s:%d runs an expression that the inspector never evaluates.", w.Path, step.Line)}
		}
	}
	if !hasTrigger(w) {
		return result{contract.EvidenceConfigured, []contract.Location{file}, w.Path + " runs on neither push nor pull_request."}
	}
	return result{contract.EvidenceStructurallyWired, []contract.Location{file}, w.Path + " runs on " + strings.Join(w.On, " and ") + "."}
}

// agentSettings is the Claude Code settings file that holds the agent hooks.
const agentSettings = ".claude/settings.json"

func (e *evaluator) agentHooks() result {
	h := e.s.AgentHooks
	if h == nil || h.Count == 0 {
		return absent("No agent hook is declared.")
	}
	return result{contract.EvidenceConfigured, []contract.Location{{Path: h.Path, Line: h.Line}},
		fmt.Sprintf("%s declares %d agent hooks, and no enforcement point for them is verifiable.", h.Path, h.Count)}
}

func firstInRecipes(m *Makefile, match func(Command) bool) (Command, bool) {
	for _, t := range m.Targets {
		for _, c := range t.Recipe {
			if match(c) {
				return c, true
			}
		}
	}
	return Command{}, false
}

// describe names an enforcement point, which is a workflow step when it has a line and a hook file when it has none.
func describe(at contract.Location) string {
	if at.Line == 0 {
		return "Hook " + at.Path
	}
	return fmt.Sprintf("The step at %s:%d", at.Path, at.Line)
}

func texts(commands []Command) string {
	out := make([]string, len(commands))
	for i, c := range commands {
		out[i] = c.Text
	}
	return strings.Join(out, ", ")
}
