// Package git runs the system Git command line for GitOwl and parses its
// machine-readable output. It does not decide application policy; package
// workflow does.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// ErrNotFound reports that no Git executable could be found.
var ErrNotFound = errors.New("git executable not found")

// candidates are tried when git is not on PATH, as for apps opened from
// Finder, whose PATH omits package manager directories.
var candidates = []string{"/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"}

// Runner runs one Git executable with a fixed, non-interactive environment.
type Runner struct {
	// Path is the Git executable.
	Path string
	// Env lists extra environment entries, applied after GitOwl's own; tests
	// use it to isolate configuration.
	Env []string
}

// Find locates Git on PATH or in the usual install locations.
func Find() (*Runner, error) {
	if p, err := exec.LookPath("git"); err == nil {
		return &Runner{Path: p}, nil
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return &Runner{Path: p}, nil
		}
	}
	return nil, ErrNotFound
}

// Error is a Git command that ran and failed.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// Message returns Git's own error text, without the command.
func (e *Error) Message() string { return strings.TrimSpace(e.Stderr) }

// inherited environment variables that would redirect Git away from the
// repository GitOwl names explicitly.
var dropped = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX",
	"GIT_NAMESPACE", "GIT_EXTERNAL_DIFF", "GIT_DIFF_OPTS", "GIT_PAGER",
	"GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "GIT_TRACE",
}

func (r *Runner) env() []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(dropped, name) || name == "LANG" || strings.HasPrefix(name, "LC_")
	})
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_EDITOR=true",
		"LC_ALL=C",
	)
	return append(env, r.Env...)
}

// global options put before every subcommand.
var global = []string{"-c", "core.quotepath=false", "-c", "color.ui=false", "-c", "core.pager=cat"}

// Run runs git with args in dir and returns its standard output. A nonzero
// exit status is an *Error.
func (r *Runner) Run(ctx context.Context, dir string, stdin io.Reader, args ...string) ([]byte, error) {
	out, _, err := r.RunStatus(ctx, dir, stdin, nil, args...)
	return out, err
}

// RunStatus is Run that also accepts the exit codes in ok, as git diff
// --no-index exits 1 when the files differ.
func (r *Runner) RunStatus(ctx context.Context, dir string, stdin io.Reader, ok []int, args ...string) ([]byte, int, error) {
	if r == nil || r.Path == "" {
		return nil, -1, ErrNotFound
	}
	cmd := exec.CommandContext(ctx, r.Path, append(slices.Clone(global), args...)...)
	cmd.Dir = dir
	cmd.Env = r.env()
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), 0, nil
	}
	if ctx.Err() != nil {
		return nil, -1, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if slices.Contains(ok, code) {
			return stdout.Bytes(), code, nil
		}
		return stdout.Bytes(), code, &Error{Args: args, ExitCode: code, Stderr: stderr.String()}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, -1, ErrNotFound
	}
	if _, serr := os.Stat(r.Path); serr != nil {
		return nil, -1, ErrNotFound
	}
	return nil, -1, err
}

// RunLimit is Run that reads at most limit bytes of standard output. When
// there is more, it stops Git and reports truncated, without an error, so
// a large output is never held in memory whole.
func (r *Runner) RunLimit(ctx context.Context, dir string, limit int, args ...string) (out []byte, truncated bool, err error) {
	if r == nil || r.Path == "" {
		return nil, false, ErrNotFound
	}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Path, append(slices.Clone(global), args...)...)
	cmd.Dir = dir
	cmd.Env = r.env()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, false, ErrNotFound
		}
		if _, serr := os.Stat(r.Path); serr != nil {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	out, rerr := io.ReadAll(io.LimitReader(pipe, int64(limit)+1))
	if len(out) > limit {
		out, truncated = out[:limit], true
		cancel() // the rest is not needed
	}
	werr := cmd.Wait()
	switch {
	case parent.Err() != nil:
		return nil, false, parent.Err()
	case truncated:
		return out, true, nil
	case rerr != nil:
		return nil, false, rerr
	}
	var exit *exec.ExitError
	if errors.As(werr, &exit) {
		return out, false, &Error{Args: args, ExitCode: exit.ExitCode(), Stderr: stderr.String()}
	}
	return out, false, werr
}
