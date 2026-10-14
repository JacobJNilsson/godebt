// Package safeguards reads the configuration files that declare a project's process checks, and it never runs any of them.
package safeguards

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
)

// Surfaces holds every configuration surface of spec 002 that the tree contains.
type Surfaces struct {
	Makefile   *Makefile          `json:"makefile"`
	Workflows  []Workflow         `json:"workflows"`
	HookFiles  []HookFile         `json:"hookFiles"`
	HookTools  []HookTool         `json:"hookTools"`
	LintConfig *contract.Location `json:"lintConfig"`
	AgentHooks *AgentHooks        `json:"agentHooks"`
	Errors     []ReadError        `json:"errors"`
}

// ReadError records a surface file that could not be read or parsed.
type ReadError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// reader reads surface files by repo-relative path, and only files that the walk kept exist for it.
type reader struct {
	root  string
	files map[string]bool
	s     *Surfaces
}

// Inspect reads the surfaces of tree and derives the eight safeguards and the broken-reference findings.
func Inspect(tree *discover.Tree) ([]contract.Safeguard, []contract.Finding) {
	files := make(map[string]bool, len(tree.Files))
	for _, f := range tree.Files {
		files[f.Path] = true
	}
	return Evaluate(Read(tree), func(rel string) bool { return files[rel] })
}

// Read parses every surface in the file list of tree, and an unreadable file becomes a ReadError.
func Read(tree *discover.Tree) *Surfaces {
	r := &reader{
		root:  tree.Root,
		files: make(map[string]bool, len(tree.Files)),
		s: &Surfaces{
			Workflows: []Workflow{},
			HookFiles: []HookFile{},
			HookTools: []HookTool{},
			Errors:    []ReadError{},
		},
	}
	for _, f := range tree.Files {
		r.files[f.Path] = true
	}
	r.s.Makefile = r.readMakefile()
	r.readWorkflows(tree.Files)
	r.readHookFiles()
	r.readHookTools()
	r.s.LintConfig = r.lintConfig()
	r.s.AgentHooks = r.readAgentHooks()
	sortSurfaces(r.s)
	return r.s
}

// read returns the text of a kept file, and ok is false when the tree lacks the file or the read fails.
func (r *reader) read(rel string) (text string, ok bool) {
	if !r.files[rel] {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		r.fail(rel, err)
		return "", false
	}
	return string(data), true
}

// fail records an error without the absolute path, which must not reach a report.
func (r *reader) fail(rel string, err error) {
	message := err.Error()
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		message = pathErr.Op + ": " + pathErr.Err.Error()
	}
	r.s.Errors = append(r.s.Errors, ReadError{Path: rel, Message: message})
}

// firstFile returns the first name that the tree holds, or "" when it holds none.
func (r *reader) firstFile(names ...string) string {
	for _, n := range names {
		if r.files[n] {
			return n
		}
	}
	return ""
}

func (r *reader) lintConfig() *contract.Location {
	name := r.firstFile(".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json")
	if name == "" {
		return nil
	}
	return &contract.Location{Path: name, Line: 1}
}

func sortSurfaces(s *Surfaces) {
	sort.Slice(s.Workflows, func(i, j int) bool { return s.Workflows[i].Path < s.Workflows[j].Path })
	sort.Slice(s.HookFiles, func(i, j int) bool { return s.HookFiles[i].Path < s.HookFiles[j].Path })
	sort.Slice(s.HookTools, func(i, j int) bool { return s.HookTools[i].Path < s.HookTools[j].Path })
	sort.Slice(s.Errors, func(i, j int) bool { return s.Errors[i].Path < s.Errors[j].Path })
}
