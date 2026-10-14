package safeguards

import (
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/JacobJNilsson/godebt/internal/discover"
)

// Workflow is the trigger and step model of one GitHub Actions workflow file.
type Workflow struct {
	Path  string   `json:"path"`
	On    []string `json:"on"`
	Steps []Step   `json:"steps"`
}

// Step is one step of a workflow job.
type Step struct {
	Line int       `json:"line"`
	Run  []Command `json:"run"`
	Uses string    `json:"uses,omitempty"`
	// Unverified is true when a run line holds an expression, which the inspector never evaluates.
	Unverified bool `json:"unverified"`
}

// readWorkflows reads the files that sit directly in .github/workflows, because GitHub reads no subdirectory.
func (r *reader) readWorkflows(files []discover.File) {
	for _, f := range files {
		ext := path.Ext(f.Path)
		if path.Dir(f.Path) != ".github/workflows" || (ext != ".yml" && ext != ".yaml") {
			continue
		}
		if w, ok := r.readWorkflow(f.Path); ok {
			r.s.Workflows = append(r.s.Workflows, w)
		}
	}
}

func (r *reader) readWorkflow(rel string) (Workflow, bool) {
	root, ok := r.readYAML(rel)
	if !ok {
		return Workflow{}, false
	}
	w := Workflow{Path: rel, On: []string{}, Steps: []Step{}}
	if on := mappingValue(root, "on"); on != nil {
		w.On = keys(on)
	}
	jobs := mappingValue(root, "jobs")
	for _, job := range mappingValues(jobs) {
		for _, step := range sequence(mappingValue(job, "steps")) {
			if step.Kind == yaml.MappingNode {
				w.Steps = append(w.Steps, readStep(step))
			}
		}
	}
	return w, true
}

// readYAML parses a kept file and returns its top-level node, which is nil for an empty file.
func (r *reader) readYAML(rel string) (*yaml.Node, bool) {
	text, ok := r.read(rel)
	if !ok {
		return nil, false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		r.fail(rel, err)
		return nil, false
	}
	if len(doc.Content) == 0 {
		return nil, true
	}
	return doc.Content[0], true
}

func readStep(step *yaml.Node) Step {
	s := Step{Line: step.Line, Run: []Command{}}
	if uses := mappingValue(step, "uses"); uses != nil {
		s.Uses = uses.Value
	}
	run := mappingValue(step, "run")
	if run == nil || run.Kind != yaml.ScalarNode {
		return s
	}
	first := run.Line
	// The node of a block scalar starts at its indicator, and the text starts on the next line.
	if run.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		first++
	}
	s.Run = parseScript(run.Value, first)
	s.Unverified = strings.Contains(run.Value, "${{")
	return s
}

// mappingValue returns the value of key in a mapping node, or nil.
func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// mappingValues returns the values of a mapping node in file order.
func mappingValues(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var out []*yaml.Node
	for i := 1; i < len(n.Content); i += 2 {
		out = append(out, n.Content[i])
	}
	return out
}

func sequence(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}

// keys returns the names that a scalar, a sequence of scalars, or the keys of a mapping give.
func keys(n *yaml.Node) []string {
	out := []string{}
	switch n.Kind {
	case yaml.ScalarNode:
		out = append(out, n.Value)
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			out = append(out, n.Content[i].Value)
		}
	}
	return out
}
