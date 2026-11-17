package workflow

import (
	"context"
	"errors"
	"strings"

	"gitowl/internal/git"
)

const (
	CodeNothingStaged   Code = "NothingStaged"
	CodeEmptyMessage    Code = "EmptyMessage"
	CodeIdentityMissing Code = "IdentityMissing"
)

// Commit is a commit GitOwl created.
type Commit struct {
	Hash, Subject string
}

// Short returns the abbreviated hash.
func (c Commit) Short() string {
	if len(c.Hash) > 7 {
		return c.Hash[:7]
	}
	return c.Hash
}

// Commit records the staged changes with message, as the user wrote it.
// staged is the StagedFingerprint the user reviewed: when the staged content
// differs, nothing is committed and the error is CodeStateChanged, with the
// fresh state. Git's own configuration supplies the identity; GitOwl does not
// let Git guess one.
func (r *Repository) Commit(ctx context.Context, message, staged string) (Snapshot, Commit, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, Commit{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Commit{}, err
	}
	switch {
	case before.StagedFingerprint != staged:
		return before, Commit{}, &Error{Code: CodeStateChanged}
	case len(before.Staged) == 0:
		return before, Commit{}, &Error{Code: CodeNothingStaged}
	case strings.TrimSpace(message) == "":
		return before, Commit{}, &Error{Code: CodeEmptyMessage}
	}
	_, err = r.app.git.Run(ctx, r.root, strings.NewReader(message),
		"-c", "user.useConfigOnly=true", "commit", "--quiet", "--file=-", "--cleanup=whitespace")
	if err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			after = before
		}
		return after, Commit{}, commitFailure(err)
	}
	after, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Commit{}, err
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	return after, Commit{Hash: after.Head, Subject: strings.TrimSpace(subject)}, nil
}

func commitFailure(err error) error {
	var g *git.Error
	if errors.As(err, &g) {
		msg := g.Message()
		for _, s := range []string{"Please tell me who you are", "Author identity unknown", "Committer identity unknown", "auto-detection is disabled", "unable to auto-detect"} {
			if strings.Contains(msg, s) {
				return fail(CodeIdentityMissing, err)
			}
		}
	}
	return gitFailure(err)
}
