package safeguards

import (
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// located is a command with the path of the file that holds it, and the Makefile target whose recipe holds it.
type located struct {
	Path   string
	Target string
	Command
}

func (l located) location() contract.Location {
	return contract.Location{Path: l.Path, Line: l.Line}
}

// enforcement is a point that runs commands without a person choosing to, such as a CI step or an installed hook.
type enforcement struct {
	at contract.Location
	// verified is false when the point itself holds a line that the inspector cannot follow.
	verified bool
	// reached lists the commands of the point and every command that make reaches from them.
	reached []located
	// targets lists the make targets that the walk visited.
	targets map[string]bool
	// throughUnknown is true when the walk visited a target whose recipe the inspector cannot follow.
	throughUnknown bool
}

// reach follows make commands through prerequisites and recipes, and the visited set ends a cycle.
func (e *evaluator) reach(p *enforcement, path, target string, commands []Command) {
	for _, c := range commands {
		p.reached = append(p.reached, located{Path: path, Target: target, Command: c})
		if c.Kind == KindMake {
			e.visit(p, e.makeTarget(c.Ref))
		}
	}
}

func (e *evaluator) visit(p *enforcement, name string) {
	t, ok := e.targets[name]
	if !ok || p.targets[name] {
		return
	}
	p.targets[name] = true
	p.throughUnknown = p.throughUnknown || t.Unknown
	for _, pre := range t.Prereqs {
		e.visit(p, pre)
	}
	e.reach(p, e.s.Makefile.Path, name, t.Recipe)
}

// makeTarget returns the target that make runs for ref, which is the first target when ref is empty.
func (e *evaluator) makeTarget(ref string) string {
	if ref == "" && e.s.Makefile != nil && len(e.s.Makefile.Targets) > 0 {
		return e.s.Makefile.Targets[0].Name
	}
	return ref
}

// enforcementPoints lists the steps of triggered workflows and of workflows without an on key, and the installed hook files.
func (e *evaluator) enforcementPoints() []*enforcement {
	var points []*enforcement
	for _, w := range e.s.Workflows {
		triggered := hasTrigger(w)
		if !triggered && len(w.On) > 0 {
			continue
		}
		for _, step := range w.Steps {
			p := &enforcement{at: contract.Location{Path: w.Path, Line: step.Line}, verified: triggered && !step.Unverified, targets: map[string]bool{}}
			e.reach(p, w.Path, "", step.Run)
			points = append(points, p)
		}
	}
	for _, h := range e.s.HookFiles {
		if _, ok := e.hooksPath[dirOf(h.Path)]; !ok {
			continue
		}
		p := &enforcement{at: contract.Location{Path: h.Path}, verified: understood(h.Lines), targets: map[string]bool{}}
		e.reach(p, h.Path, "", h.Lines)
		points = append(points, p)
	}
	return points
}

func hasTrigger(w Workflow) bool {
	for _, on := range w.On {
		if on == "push" || on == "pull_request" {
			return true
		}
	}
	return false
}

// understood reports whether every command has a form that spec 002 lists.
func understood(commands []Command) bool {
	return firstOther(commands) == nil
}

func firstOther(commands []Command) *Command {
	for i := range commands {
		if commands[i].Kind == KindOther {
			return &commands[i]
		}
	}
	return nil
}

func dirOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return "."
}

// hidden reports whether a reached command sits in a Makefile target of a Makefile that includes another file, which may redefine the target.
func (e *evaluator) hidden(c located) bool {
	return c.Target != "" && e.s.Makefile.Includes
}

// wiring finds a clean enforcement point that reaches a matching command, and else an unverified one.
func (e *evaluator) wiring(match func(located) bool) (wired, unverified *enforcement, hit located) {
	for _, p := range e.points {
		for _, c := range p.reached {
			if !match(c) {
				continue
			}
			if p.verified && !p.throughUnknown && !e.hidden(c) {
				return p, nil, c
			}
			if unverified == nil {
				unverified, hit = p, c
			}
		}
	}
	return nil, unverified, hit
}
