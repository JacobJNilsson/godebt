// Package inventory lists the functions of the measured files with their size and complexity.
package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"path"
	"sort"
	"strconv"

	"github.com/JacobJNilsson/godebt/internal/discover"
)

// Function is one function of a measured file.
type Function struct {
	Identity  string             `json:"identity"`
	Ambiguous bool               `json:"ambiguous"`
	Path      string             `json:"path"`
	Set       discover.SourceSet `json:"set"`
	StartLine int                `json:"startLine"`
	EndLine   int                `json:"endLine"`
	CC        int                `json:"cc"`
	Nesting   int                `json:"nesting"`
	SLOC      int                `json:"sloc"`
}

// The kinds of an Error.
const (
	KindParse = "parse"
	KindRead  = "read"
)

// Error records a file that did not parse, or a file or directory that could not be read.
type Error struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	// Set is empty for a read error whose path could hold files of any set.
	Set     discover.SourceSet `json:"set,omitempty"`
	Message string             `json:"message"`
}

// Inventory holds every function of the production and test sets.
type Inventory struct {
	Tree      *discover.Tree `json:"tree"`
	Functions []Function     `json:"functions"`
	// SLOC counts the code lines of the parsed files per measured set.
	SLOC map[discover.SourceSet]int `json:"sloc"`
	// CodeLines marks the code lines of each parsed file by path, where CodeLines[path][l] is true for a code line l.
	CodeLines map[string][]bool `json:"-"`
	Errors    []Error           `json:"errors"`
}

// Incomplete reports whether an error touches the set.
func (inv *Inventory) Incomplete(set discover.SourceSet) bool {
	return len(inv.ErrorPaths(set)) > 0
}

// ErrorPaths lists the paths of the errors that touch the set, in the order of Errors.
// A read error without a set could hide files of every set.
func (inv *Inventory) ErrorPaths(set discover.SourceSet) []string {
	var paths []string
	for _, e := range inv.Errors {
		if e.Set == set || (e.Kind == KindRead && e.Set == "") {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

// declaration is a function declaration or a package-level var spec, with its closures.
type declaration struct {
	file     *discover.File
	lines    []bool
	identity string
	// decl is nil for a var spec, which is not a function of its own.
	decl *ast.FuncDecl
	lits []*ast.FuncLit
}

// Build measures the functions of the parsed production and test files of a tree.
func Build(tree *discover.Tree) *Inventory {
	inv := &Inventory{
		Tree:      tree,
		Functions: []Function{},
		SLOC:      map[discover.SourceSet]int{discover.Production: 0, discover.Test: 0},
		CodeLines: map[string][]bool{},
		Errors:    []Error{},
	}
	for _, e := range tree.ReadErrors {
		inv.Errors = append(inv.Errors, Error{Kind: KindRead, Path: e.Path, Set: e.Set, Message: e.Message})
	}
	var decls []*declaration
	for i := range tree.Files {
		f := &tree.Files[i]
		if f.Set != discover.Production && f.Set != discover.Test {
			continue
		}
		if f.ParseErr != nil {
			// The report keeps one message per file, because the count of later errors differs between Go releases.
			inv.Errors = append(inv.Errors, Error{Kind: KindParse, Path: f.Path, Set: f.Set, Message: firstError(f)})
			continue
		}
		lines := codeLines(tree.Fset.File(f.Syntax.Pos()), f.Src)
		inv.CodeLines[f.Path] = lines
		for _, isCode := range lines {
			if isCode {
				inv.SLOC[f.Set]++
			}
		}
		decls = append(decls, declarations(f, lines)...)
	}
	resolveCollisions(decls)
	for _, d := range decls {
		inv.Functions = append(inv.Functions, measure(tree.Fset, d)...)
	}
	sortFunctions(inv.Functions)
	sortErrors(inv.Errors)
	return inv
}

// firstError returns the error nearest the start of the file, placed by byte offset so that a //line comment has no effect.
func firstError(f *discover.File) string {
	var list scanner.ErrorList
	if !errors.As(f.ParseErr, &list) || len(list) == 0 {
		return f.ParseErr.Error()
	}
	first := list[0]
	for _, e := range list[1:] {
		if e.Pos.Offset < first.Pos.Offset {
			first = e
		}
	}
	before := f.Src[:min(first.Pos.Offset, len(f.Src))]
	line := bytes.Count(before, []byte("\n")) + 1
	column := len(before) - bytes.LastIndexByte(before, '\n')
	return fmt.Sprintf("%s:%d:%d: %s", f.Path, line, column, first.Msg)
}

// declarations lists the top-level declarations of a file that hold functions.
func declarations(f *discover.File, lines []bool) []*declaration {
	dir := path.Dir(f.Path)
	var out []*declaration
	for _, decl := range f.Syntax.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Body == nil {
				continue
			}
			out = append(out, &declaration{file: f, lines: lines, identity: dir + ":" + funcName(decl), decl: decl, lits: funcLits(decl.Body)})
		case *ast.GenDecl:
			// A const spec cannot hold a function literal, so only var specs add closures here.
			for _, spec := range decl.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if lits := funcLits(value); len(lits) > 0 {
					out = append(out, &declaration{file: f, lines: lines, identity: dir + ":" + value.Names[0].Name, lits: lits})
				}
			}
		}
	}
	return out
}

// funcName returns the name of a function, with the receiver type name before a method.
func funcName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return decl.Name.Name
	}
	typ := decl.Recv.List[0].Type
	for {
		switch t := typ.(type) {
		case *ast.Ident:
			return t.Name + "." + decl.Name.Name
		case *ast.StarExpr:
			typ = t.X
		case *ast.ParenExpr:
			typ = t.X
		case *ast.IndexExpr:
			typ = t.X
		case *ast.IndexListExpr:
			typ = t.X
		default:
			return decl.Name.Name
		}
	}
}

// funcLits returns every function literal under a node in source order.
func funcLits(node ast.Node) []*ast.FuncLit {
	var lits []*ast.FuncLit
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			lits = append(lits, lit)
		}
		return true
	})
	return lits
}

// resolveCollisions appends the file base name, then a source-order ordinal, until the identities in each source set are unique.
func resolveCollisions(decls []*declaration) {
	disambiguate(decls, func(d *declaration, _ int) string { return "@" + path.Base(d.file.Path) })
	disambiguate(decls, func(_ *declaration, ordinal int) string { return "@" + strconv.Itoa(ordinal) })
}

// disambiguate appends a suffix to every declaration whose identity another one of its set shares.
// The ordinal counts the members of one collision in the order of decls, which is source order within a file.
func disambiguate(decls []*declaration, suffix func(d *declaration, ordinal int) string) {
	type key struct {
		set      discover.SourceSet
		identity string
	}
	groups := map[key][]*declaration{}
	for _, d := range decls {
		k := key{d.file.Set, d.identity}
		groups[k] = append(groups[k], d)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		for i, d := range group {
			d.identity += suffix(d, i+1)
		}
	}
}

// measure returns the function of a declaration, when it has one, and one function per closure.
func measure(fset *token.FileSet, d *declaration) []Function {
	newFunction := func(identity string, node, body ast.Node) Function {
		// PositionFor without adjustment keeps a //line comment from moving the real lines.
		start, end := fset.PositionFor(node.Pos(), false).Line, fset.PositionFor(node.End(), false).Line
		sloc := 0
		for line := start; line <= end; line++ {
			if d.lines[line] {
				sloc++
			}
		}
		return Function{
			Identity: identity, Ambiguous: node != ast.Node(d.decl),
			Path: d.file.Path, Set: d.file.Set, StartLine: start, EndLine: end,
			CC: complexity(body), Nesting: nesting(body, 0), SLOC: sloc,
		}
	}
	var out []Function
	if d.decl != nil {
		out = append(out, newFunction(d.identity, d.decl, d.decl.Body))
	}
	for i, lit := range d.lits {
		out = append(out, newFunction(d.identity+"#"+strconv.Itoa(i+1), lit, lit.Body))
	}
	return out
}

func sortFunctions(fns []Function) {
	sort.Slice(fns, func(i, j int) bool {
		a, b := fns[i], fns[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		return a.Identity < b.Identity
	})
}

// sortErrors orders errors by path, then kind, because one path can hold only one error of each kind.
func sortErrors(errs []Error) {
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Path != errs[j].Path {
			return errs[i].Path < errs[j].Path
		}
		return errs[i].Kind < errs[j].Kind
	})
}
