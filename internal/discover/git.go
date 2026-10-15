package discover

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
)

// gitError reports a failed git command with the message that Git wrote.
type gitError struct {
	Command, Message string
}

func (e *gitError) Error() string { return "git " + e.Command + ": " + e.Message }

// gitFiles is the file list that Git reports for a root.
type gitFiles struct {
	files map[string]bool
	// dirs holds every directory that contains a listed file.
	dirs map[string]bool
	// unopened lists the directories that Git could not open and that hold no listed file, with the reason from Git.
	unopened []ReadError
}

// has reports whether Git lists the file, or lists a file below the directory.
func (g *gitFiles) has(rel string, isDir bool) bool {
	if isDir {
		return g.dirs[rel]
	}
	return g.files[rel]
}

// listGitFiles returns the files that Git lists for root, or nil when git is missing or root is outside a work tree.
func listGitFiles(ctx context.Context, root string) (*gitFiles, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, nil
	}
	inside, prefix, err := insideWorkTree(ctx, bin, root)
	if err != nil || !inside {
		return nil, err
	}
	out, stderr, err := runGit(ctx, bin, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	list := &gitFiles{files: map[string]bool{}, dirs: map[string]bool{}, unopened: unopenedDirs(stderr, prefix)}
	for _, name := range strings.Split(string(out), "\x00") {
		// --others lists a nested repository as "dir/". A submodule comes through --cached as a plain path whose directory is never in dirs, so the walk skips it.
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		list.files[name] = true
		for dir := path.Dir(name); dir != "." && !list.dirs[dir]; dir = path.Dir(dir) {
			list.dirs[dir] = true
		}
	}
	// The walk enters a directory in dirs and records its read error itself.
	list.unopened = slices.DeleteFunc(list.unopened, func(e ReadError) bool { return list.dirs[e.Path] })
	return list, nil
}

// unopenedDirs parses the warnings of the form "could not open directory 'dir/': reason" and ignores other lines.
// Git writes dir relative to the repository top level, so the function strips prefix and drops a directory outside it.
// The match depends on the English warning text, which LC_ALL=C fixes.
func unopenedDirs(stderr []byte, prefix string) []ReadError {
	var out []ReadError
	for _, line := range strings.Split(string(stderr), "\n") {
		rest, ok := strings.CutPrefix(line, "warning: could not open directory '")
		if !ok {
			continue
		}
		dir, reason, ok := strings.Cut(rest, "': ")
		if rel, inRoot := strings.CutPrefix(dir, prefix); ok && inRoot {
			out = append(out, ReadError{Path: strings.TrimSuffix(rel, "/"), Message: reason})
		}
	}
	return out
}

// insideWorkTree asks Git whether root is in a work tree, and returns the path of root below the top level with a trailing slash.
// Git exits with 128 and "not a git repository" outside one, and any other failure is an error.
func insideWorkTree(ctx context.Context, bin, root string) (bool, string, error) {
	out, _, err := runGit(ctx, bin, root, "rev-parse", "--is-inside-work-tree", "--show-prefix")
	if err != nil {
		var gitErr *gitError
		if errors.As(err, &gitErr) && strings.Contains(gitErr.Message, "not a git repository") {
			return false, "", nil
		}
		return false, "", err
	}
	inside, prefix, _ := strings.Cut(string(out), "\n")
	return inside == "true", strings.TrimSuffix(prefix, "\n"), nil
}

// runGit runs one git command in root and returns its standard output and standard error, or an error that carries the standard error text.
func runGit(ctx context.Context, bin, root string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, bin, append([]string{"-C", root}, args...)...)
	cmd.Env = gitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" || !errors.As(err, new(*exec.ExitError)) {
			message = err.Error()
		}
		return nil, nil, &gitError{Command: args[0], Message: message}
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

// gitEnv drops the variables that a Git hook sets, so Git finds the repository of the root and not the one that runs the hook.
func gitEnv() []string {
	env := []string{"LC_ALL=C"}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "LC_ALL":
			continue
		}
		env = append(env, kv)
	}
	return env
}
