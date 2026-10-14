// Package discover walks an audit root and sorts every file into a source set.
package discover

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	gitignore "github.com/boyter/gocodewalker/go-gitignore"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// SourceSet names the group a file belongs to.
type SourceSet = contract.SourceSet

// The source sets of spec 002, in the order a report lists them.
const (
	Production  = contract.Production
	Test        = contract.Test
	Generated   = contract.Generated
	Vendored    = contract.Vendored
	Testdata    = contract.Testdata
	Excluded    = contract.Excluded
	Unsupported = contract.Unsupported
)

// AllSets returns every source set in report order.
func AllSets() []SourceSet {
	return []SourceSet{Production, Test, Generated, Vendored, Testdata, Excluded, Unsupported}
}

// Options holds the classification patterns from the configuration.
type Options struct {
	Exclude      []string
	TestPatterns []string
	// Omit is the path of one file that the walk leaves out of every set and every count, and "" omits none.
	Omit string
}

// File is one regular file that the walk kept.
type File struct {
	Path string    `json:"path"`
	Set  SourceSet `json:"set"`
	// Syntax, Src, and ParseErr are set for production and test files only.
	Syntax   *ast.File `json:"-"`
	Src      []byte    `json:"-"`
	ParseErr error     `json:"-"`
}

// ReadError records a file or directory that the walk could not read.
type ReadError struct {
	Path string `json:"path"`
	// Set is the set of the path by the path rules, or "" when the path could hold measured files.
	Set     SourceSet `json:"set,omitempty"`
	Message string    `json:"message"`
}

// SetCoverage counts the files of one source set.
type SetCoverage struct {
	Files int `json:"files"`
}

// Tree is the result of a walk.
type Tree struct {
	Root          string                    `json:"root"`
	Module        string                    `json:"module"`
	Files         []File                    `json:"files"`
	NestedModules []string                  `json:"nestedModules"`
	ReadErrors    []ReadError               `json:"readErrors"`
	Coverage      map[SourceSet]SetCoverage `json:"coverage"`
	// Fset holds the positions of every parsed file.
	Fset *token.FileSet `json:"-"`
}

// Walk visits every regular file under root in lexical order and classifies it.
func Walk(ctx context.Context, root string, opts Options) (*Tree, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}
	// WalkDir does not follow a symbolic link at the root, so the walk starts at the target.
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	w := &walker{
		ctx:         ctx,
		root:        abs,
		opts:        opts,
		omit:        resolveOmit(opts.Omit),
		ignoreFiles: gitignore.NewCache(),
		tree: &Tree{
			Root:          abs,
			Files:         []File{},
			NestedModules: []string{},
			ReadErrors:    []ReadError{},
			Coverage:      map[SourceSet]SetCoverage{},
			Fset:          token.NewFileSet(),
		},
	}
	// Any file name other than the exact string ".gitignore" stops the library from reading .git/info/exclude.
	w.ignore = gitignore.NewRepositoryWithCache(abs, "./.gitignore", w.ignoreFiles, w.ignoreError)
	module, err := readModule(abs)
	if err != nil {
		w.readError("go.mod", "", err)
	}
	w.tree.Module = module
	if err := filepath.WalkDir(abs, w.visit); err != nil {
		return nil, err
	}
	finish(w.tree)
	return w.tree, nil
}

// resolveOmit returns the path that the walk sees for the omitted file, with the symbolic links of its directory resolved like the root.
func resolveOmit(omit string) string {
	if omit == "" {
		return ""
	}
	abs, err := filepath.Abs(omit)
	if err != nil {
		return ""
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(dir, filepath.Base(abs))
	}
	return abs
}

// finish sorts every list of a tree and counts the files of each set.
func finish(tree *Tree) {
	sort.Slice(tree.Files, func(i, j int) bool { return tree.Files[i].Path < tree.Files[j].Path })
	sort.Strings(tree.NestedModules)
	sort.Slice(tree.ReadErrors, func(i, j int) bool { return tree.ReadErrors[i].Path < tree.ReadErrors[j].Path })
	for _, set := range AllSets() {
		tree.Coverage[set] = SetCoverage{}
	}
	for _, f := range tree.Files {
		c := tree.Coverage[f.Set]
		c.Files++
		tree.Coverage[f.Set] = c
	}
}

type walker struct {
	ctx  context.Context
	root string
	opts Options
	// omit is the resolved path of Options.Omit.
	omit   string
	ignore gitignore.GitIgnore
	// ignoreFiles is the library's cache of parsed .gitignore files, and the walk adds an empty entry for each unreadable one.
	ignoreFiles gitignore.Cache
	tree        *Tree
}

func (w *walker) visit(path string, d fs.DirEntry, err error) error {
	if ctxErr := w.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	rel, relErr := filepath.Rel(w.root, path)
	if relErr != nil {
		return relErr
	}
	rel = filepath.ToSlash(rel)
	if err != nil {
		// WalkDir reports an unreadable directory after its first visit, and returning nil skips its contents.
		w.readError(rel, w.dirSet(rel), err)
		return nil
	}
	if path == w.root {
		w.checkIgnoreFile(path, rel)
		return nil
	}
	if d.Name() == ".git" {
		return skipEntry(d)
	}
	if d.IsDir() {
		return w.visitDir(path, rel)
	}
	if !w.keeps(path, d) {
		return nil
	}
	file, err := w.classify(path, rel)
	if err != nil {
		// Only a .go file that no path rule places is read, so its set is unknown.
		w.readError(rel, "", err)
		return nil
	}
	w.tree.Files = append(w.tree.Files, file)
	return nil
}

// keeps reports whether a non-directory entry is a regular file that the walk classifies.
func (w *walker) keeps(path string, d fs.DirEntry) bool {
	return d.Type().IsRegular() && path != w.omit && !w.ignored(path, false)
}

// readError records an error without the absolute path, which must not reach a report.
func (w *walker) readError(rel string, set SourceSet, err error) {
	message := err.Error()
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		message = pathErr.Op + ": " + pathErr.Err.Error()
	}
	w.tree.ReadErrors = append(w.tree.ReadErrors, ReadError{Path: rel, Set: set, Message: message})
}

// ignoreError tells the library to skip a pattern line it rejects, and it records every other error as a read error.
func (w *walker) ignoreError(e gitignore.Error) bool {
	cause := e.Underlying()
	// Spec 004 lists the rejected patterns "a/**b", "***", and a lone "!" as a known gap against Git.
	if errors.Is(cause, gitignore.ErrInvalidPatternError) || errors.Is(cause, gitignore.ErrCarriageReturnError) {
		return true
	}
	// A missing .gitignore has no patterns.
	if errors.Is(cause, fs.ErrNotExist) {
		return true
	}
	file := e.Position().File
	var pathErr *fs.PathError
	if file == "" && errors.As(cause, &pathErr) {
		file = pathErr.Path
	}
	// An error without a known file goes to the root, which makes both measured sets incomplete.
	rel := "."
	if r, err := filepath.Rel(w.root, file); err == nil && file != "" {
		rel = filepath.ToSlash(r)
	}
	w.readError(rel, w.dirSet(path.Dir(rel)), cause)
	// An I/O error can carry a line position too, and the parser retries a failed read forever when the handler returns true.
	return false
}

// checkIgnoreFile records a .gitignore that the library could not read, and it gives the library an empty pattern list instead.
func (w *walker) checkIgnoreFile(dir, rel string) {
	file := filepath.Join(dir, ".gitignore")
	// Lstat treats a symbolic link as no regular file, because Git does not follow a link named .gitignore.
	info, err := os.Lstat(file)
	if errors.Is(err, fs.ErrNotExist) || (err != nil && !canOpen(dir)) {
		// WalkDir reports a directory that it cannot open, and it then visits nothing below it.
		return
	}
	if err == nil && !info.Mode().IsRegular() {
		err = &fs.PathError{Op: "read", Path: file, Err: errors.New("not a regular file")}
	}
	if err == nil {
		var f *os.File
		if f, err = os.Open(file); err == nil {
			_ = f.Close()
			return
		}
	}
	w.ignoreFiles.Set(file, gitignore.New(strings.NewReader(""), dir, nil))
	w.readError(path.Join(rel, ".gitignore"), w.dirSet(rel), err)
}

// canOpen reports whether the walk can open a directory to list its entries.
func canOpen(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// dirSet returns the set that the path rules give every file in a directory, or "" when they give none.
func (w *walker) dirSet(rel string) SourceSet {
	if rel == "." {
		return ""
	}
	return w.byPath(rel + "/-")
}

func (w *walker) visitDir(path, rel string) error {
	if w.ignored(path, true) {
		return filepath.SkipDir
	}
	w.checkIgnoreFile(path, rel)
	// A directory in the testdata, vendored, or excluded set is never a nested module.
	info, err := os.Lstat(filepath.Join(path, "go.mod"))
	if err == nil && info.Mode().IsRegular() && w.byPath(rel+"/go.mod") == "" {
		w.tree.NestedModules = append(w.tree.NestedModules, rel)
		return filepath.SkipDir
	}
	return nil
}

// skipEntry skips a directory with its contents, or a single file.
func skipEntry(d fs.DirEntry) error {
	if d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

// ignored calls Absolute because the promoted Ignore and MatchIsDir methods of the repository see no patterns.
func (w *walker) ignored(path string, isDir bool) bool {
	m := w.ignore.Absolute(path, isDir)
	return m != nil && m.Ignore()
}

// byPath applies the source-set rules that look only at the path, and returns "" when none matches.
func (w *walker) byPath(rel string) SourceSet {
	segments := strings.Split(rel, "/")
	dirs := segments[:len(segments)-1]
	switch {
	case hasSegment(dirs, func(s string) bool { return s == "testdata" }):
		return Testdata
	case hasSegment(dirs, func(s string) bool { return s == "vendor" }):
		return Vendored
	case hasSegment(segments, func(s string) bool { return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "_") }),
		matchAny(w.opts.Exclude, rel):
		return Excluded
	}
	return ""
}

func (w *walker) classify(path, rel string) (File, error) {
	if set := w.byPath(rel); set != "" {
		return File{Path: rel, Set: set}, nil
	}
	if !strings.HasSuffix(rel, ".go") {
		return File{Path: rel, Set: Unsupported}, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	syntax, parseErr := parser.ParseFile(w.tree.Fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if ast.IsGenerated(syntax) {
		return File{Path: rel, Set: Generated}, nil
	}
	set := Production
	if strings.HasSuffix(rel, "_test.go") || matchAny(w.opts.TestPatterns, rel) {
		set = Test
	}
	return File{Path: rel, Set: set, Syntax: syntax, Src: src, ParseErr: parseErr}, nil
}

func hasSegment(segments []string, match func(string) bool) bool {
	for _, s := range segments {
		if match(s) {
			return true
		}
	}
	return false
}

func matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchGlob(p, rel) {
			return true
		}
	}
	return false
}

// readModule returns the module path from root/go.mod, or "" when the file does not exist.
func readModule(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		if unquoted, err := strconv.Unquote(fields[1]); err == nil {
			return unquoted, nil
		}
		return fields[1], nil
	}
	return "", nil
}
