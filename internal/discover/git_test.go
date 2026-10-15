package discover

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolateGit points Git at a config that the test owns and removes the variables that a hook sets.
func isolateGit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	excludes := filepath.Join(dir, "global-excludes")
	config := filepath.Join(dir, "gitconfig")
	write(t, dir, map[string]string{
		"global-excludes": "*.global.go\n",
		"gitconfig":       "[core]\n\texcludesFile = " + excludes + "\n[init]\n\tdefaultBranch = main\n",
	})
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newRepo returns a work tree made by git init, outside this repository.
func newRepo(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	root := realTempDir(t)
	git(t, root, "init", "-q")
	return root
}

func TestWalkGitModeFileList(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{
		".gitignore":              "tracked.go\nignored/\n",
		".git/info/exclude":       "local.go\n",
		"a.go":                    "package p\n",
		"gone.go":                 "package p\n",
		"local.go":                "package p\n",
		"tracked.go":              "package p\n",
		"x.global.go":             "package p\n",
		"ignored/i.go":            "package i\n",
		"sub/untracked.go":        "package sub\n",
		"nested/inner.go":         "package n\n",
		"nested/.keep":            "x\n",
		"other/ignored.global.go": "package o\n",
	})
	git(t, filepath.Join(root, "nested"), "init", "-q")
	git(t, root, "add", "-f", "tracked.go", "gone.go")
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{".gitignore", Excluded},
		{"a.go", Production},
		{"sub/untracked.go", Production},
		{"tracked.go", Production},
	}
	if got := entries(walk(t, root, Options{})); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestWalkGitModeSubmoduleFilesAreSkipped(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n", "mod/m.go": "package m\n"})
	git(t, filepath.Join(root, "mod"), "init", "-q")
	git(t, filepath.Join(root, "mod"), "add", "m.go")
	git(t, filepath.Join(root, "mod"), "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "m")
	git(t, root, "add", "mod")
	want := []entry{{"a.go", Production}}
	if got := entries(walk(t, root, Options{})); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestWalkGitModeOmit(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n", "out.json": "{}\n"})
	want := []entry{{"a.go", Production}}
	if got := entries(walk(t, root, Options{Omit: filepath.Join(root, "out.json")})); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestWalkGitModeKeepsNestedModuleDetection(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n", "tools/go.mod": "module tools\n", "tools/t.go": "package t\n"})
	tree := walk(t, root, Options{})
	if want := []string{"tools"}; !reflect.DeepEqual(tree.NestedModules, want) {
		t.Errorf("nested modules = %v, want %v", tree.NestedModules, want)
	}
	if want := []entry{{"a.go", Production}}; !reflect.DeepEqual(entries(tree), want) {
		t.Errorf("files = %v, want %v", entries(tree), want)
	}
}

func TestWalkGitModeReadsNoGitignoreFiles(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{".gitignore": "x\n", "a.go": "package p\n", "sub/b.go": "package sub\n"})
	lock(t, root, ".gitignore")
	tree := walk(t, root, Options{})
	if len(tree.ReadErrors) != 0 {
		t.Errorf("read errors = %v, want none", tree.ReadErrors)
	}
}

func TestWalkGitModeFailureReturnsGitMessage(t *testing.T) {
	root := newRepo(t)
	// A corrupt index makes ls-files fail inside a work tree.
	write(t, root, map[string]string{".git/index": "garbage"})
	_, err := Walk(context.Background(), root, Options{})
	var gitErr *gitError
	if !errors.As(err, &gitErr) || gitErr.Command != "ls-files" || gitErr.Message == "" {
		t.Fatalf("err = %v, want a gitError from ls-files with Git's message", err)
	}
}

func TestWalkWithoutGitFallsBackToGitignore(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{".gitignore": "x.go\n", "x.go": "package p\n", "y.go": "package p\n", "z.global.go": "package p\n"})
	t.Setenv("PATH", t.TempDir())
	want := []entry{{".gitignore", Excluded}, {"y.go", Production}, {"z.global.go", Production}}
	if got := entries(walk(t, root, Options{})); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestWalkGitModeOnTheSetsFixture(t *testing.T) {
	root, err := filepath.Abs(filepath.Join(fixtures, "sets"))
	if err != nil {
		t.Fatal(err)
	}
	isolateGit(t)
	got := map[string]bool{}
	for _, e := range entries(walk(t, root, Options{})) {
		got[e.Path] = true
	}
	// The fixture tracks three files that its .gitignore files match.
	for _, p := range []string{"build/out.go", "ignored.go", "nested/skip.go"} {
		if !got[p] {
			t.Errorf("tracked file %s is missing", p)
		}
	}
}

func TestWalkGitModeRecordsUnopenableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads directories with mode 000")
	}
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n", "testdata/secret/s.go": "package s\n", "open/o.go": "package o\n"})
	lock(t, root, "testdata/secret")
	tree := walk(t, root, Options{})
	want := []ReadError{{Path: "testdata/secret", Set: Testdata, Message: "Permission denied"}}
	if !reflect.DeepEqual(tree.ReadErrors, want) {
		t.Errorf("read errors = %v, want %v", tree.ReadErrors, want)
	}
	wantFiles := []entry{{"a.go", Production}, {"open/o.go", Production}}
	if got := entries(tree); !reflect.DeepEqual(got, wantFiles) {
		t.Errorf("files = %v, want %v", got, wantFiles)
	}
}

// The walk enters the directory and records its own error, so the Git warning must not repeat it.
func TestWalkGitModeReportsUnopenableDirectoryWithTrackedFileOnce(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n", "testdata/secret/deep/s.go": "package s\n"})
	git(t, root, "add", "testdata")
	lock(t, root, "testdata/secret")
	tree := walk(t, root, Options{})
	want := []ReadError{{Path: "testdata/secret", Set: Testdata, Message: "open: permission denied"}}
	if !reflect.DeepEqual(tree.ReadErrors, want) {
		t.Errorf("read errors = %v, want %v", tree.ReadErrors, want)
	}
}

func TestWalkGitModeIgnoresHookVariables(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n"})
	other := realTempDir(t)
	git(t, other, "init", "-q")
	write(t, other, map[string]string{".git/info/exclude": "a.go\n"})
	// A Git hook sets these variables, and they must not point the walk at another repository.
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	want := []entry{{"a.go", Production}}
	if got := entries(walk(t, root, Options{})); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestWalkGitModeFromASubdirectoryRoot(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{
		".gitignore":       "hidden/\n",
		"top.go":           "package p\n",
		"sub/a.go":         "package sub\n",
		"sub/deep/b.go":    "package deep\n",
		"hidden/h.go":      "package h\n",
		"hidden/deep/i.go": "package i\n",
	})
	tree := walk(t, filepath.Join(root, "sub"), Options{})
	want := []entry{{"a.go", Production}, {"deep/b.go", Production}}
	if got := entries(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	// Git ignores the whole root, so the audit follows Git and finds no file.
	tree = walk(t, filepath.Join(root, "hidden"), Options{})
	if len(tree.Files) != 0 || len(tree.ReadErrors) != 0 {
		t.Errorf("files = %v, read errors = %v, want none", entries(tree), tree.ReadErrors)
	}
}

func TestWalkGitModeUnopenableDirectoryFromASubdirectoryRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads directories with mode 000")
	}
	root := newRepo(t)
	write(t, root, map[string]string{
		"a/ok.go":           "package a\n",
		"a/b/b.go":          "package b\n",
		"a/testdata/x/x.go": "package x\n",
		"outside/o.go":      "package o\n",
	})
	lock(t, root, "a/b", "a/testdata/x", "outside")
	tree := walk(t, filepath.Join(root, "a"), Options{})
	want := []ReadError{
		{Path: "b", Message: "Permission denied"},
		{Path: "testdata/x", Set: Testdata, Message: "Permission denied"},
	}
	if !reflect.DeepEqual(tree.ReadErrors, want) {
		t.Errorf("read errors = %v, want %v", tree.ReadErrors, want)
	}
}

func TestWalkGitModeStopsOnACancelledContext(t *testing.T) {
	root := newRepo(t)
	write(t, root, map[string]string{"a.go": "package p\n"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Walk(ctx, root, Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
