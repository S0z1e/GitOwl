package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	CodeNoCommitsForWorktree Code = "NoCommitsForWorktree"
	CodeBranchInUse          Code = "BranchInUse"
	CodePathExists           Code = "PathExists"
	CodeInvalidWorktreePath  Code = "InvalidWorktreePath"
)

// NewWorktree is what CreateWorktree makes.
type NewWorktree struct {
	// Branch is the local branch to check out, or the one to create.
	Branch string
	// Create makes Branch at the current worktree's HEAD.
	Create bool
	// Path is the new directory; it must not exist.
	Path string
}

// SuggestWorktreePath proposes a directory beside the main worktree named
// after it and branch, as "app-feature-x" for branch feature/x of app, with
// a number added when that name is taken. It only reads the file system.
func SuggestWorktreePath(main, branch string) string {
	name := filepath.Base(main) + "-" + safeName(branch)
	if branch == "" {
		name = filepath.Base(main) + "-worktree"
	}
	base := filepath.Join(filepath.Dir(main), name)
	p := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
		p = base + "-" + strconv.Itoa(i)
	}
}

// safeName turns a branch name into one directory name: path separators
// and characters awkward in paths become "-".
func safeName(branch string) string {
	var b strings.Builder
	dash := false
	for _, r := range branch {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if !ok {
			if !dash {
				b.WriteByte('-')
			}
			dash = true
			continue
		}
		b.WriteRune(r)
		dash = false
	}
	return strings.Trim(b.String(), "-.")
}

// CreateWorktree adds a linked worktree at spec.Path, checking out an
// existing local branch or a new one made at this worktree's HEAD. Git's
// rules stand: no branch checked out elsewhere, no existing directory, no
// force. It returns the worktrees and branches read afterwards, also after
// a failure, and the new worktree's root. Nothing is removed on failure.
func (r *Repository) CreateWorktree(ctx context.Context, spec NewWorktree) ([]Worktree, Branches, string, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return nil, Branches{}, "", &Error{Code: CodeRepositoryClosed}
	}
	root, opErr := r.createWorktree(ctx, spec)
	list, err := r.Worktrees(ctx)
	if err != nil && opErr == nil {
		opErr = err
	}
	branches, err := r.Branches(ctx)
	if err != nil && opErr == nil {
		opErr = err
	}
	return list, branches, root, opErr
}

func (r *Repository) createWorktree(ctx context.Context, spec NewWorktree) (string, error) {
	snap, err := r.read(ctx)
	if err != nil {
		return "", err
	}
	if snap.Unborn() {
		return "", &Error{Code: CodeNoCommitsForWorktree}
	}
	path := strings.TrimSpace(spec.Path)
	if path == "" || !filepath.IsAbs(path) {
		return "", &Error{Code: CodeInvalidWorktreePath, Detail: path}
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		return "", &Error{Code: CodePathExists, Detail: path}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fail(CodeNoAccess, err)
	}
	branches, err := r.Branches(ctx)
	if err != nil {
		return "", err
	}
	name := spec.Branch
	_, exists := branches.local(name)
	args := []string{"worktree", "add", "--quiet"}
	if spec.Create {
		if strings.TrimSpace(name) != name || name == "" {
			return "", &Error{Code: CodeInvalidBranchName, Detail: name}
		}
		if _, err := r.run(ctx, "check-ref-format", "--branch", name); err != nil {
			return "", fail(CodeInvalidBranchName, err)
		}
		if exists {
			return "", &Error{Code: CodeBranchExists, Detail: name}
		}
		args = append(args, "-b", name, "--", path, "HEAD")
	} else {
		if !exists {
			return "", &Error{Code: CodeBranchNotFound, Detail: name}
		}
		trees, err := r.Worktrees(ctx)
		if err != nil {
			return "", err
		}
		for _, w := range trees {
			if w.Branch == name {
				return "", &Error{Code: CodeBranchInUse, Detail: w.Path}
			}
		}
		args = append(args, "--", path, name)
	}
	if _, err := r.run(ctx, args...); err != nil {
		return "", worktreeFailure(err)
	}
	return canonical(path), nil
}

func worktreeFailure(err error) error {
	var e *Error
	if errors.As(err, &e) {
		msg := e.Detail
		switch {
		case strings.Contains(msg, "already checked out"), strings.Contains(msg, "is already used by worktree"):
			return &Error{Code: CodeBranchInUse, Detail: msg, Err: err}
		case strings.Contains(msg, "already exists"):
			return &Error{Code: CodePathExists, Detail: msg, Err: err}
		case strings.Contains(msg, "Permission denied"), strings.Contains(msg, "Read-only file system"):
			return &Error{Code: CodeNoAccess, Detail: msg, Err: err}
		}
	}
	return err
}
