package workflow

import (
	"bytes"
	"context"
	"strings"

	"gitowl/internal/git"
)

// MaxPatchBytes is the largest patch GitOwl shows as text; larger ones are
// reported as TooLarge rather than cut short.
const MaxPatchBytes = 4 << 20

// Diff is one file's change, as GitOwl presents it for review.
type Diff struct {
	Change Change
	// Patch is Git's unified diff of the file, empty when no text is shown.
	Patch string
	// Binary marks content Git does not show as text.
	Binary bool
	// TooLarge marks a patch over MaxPatchBytes.
	TooLarge bool
	// Submodule marks a nested repository, whose content is not shown.
	Submodule bool
	// Conflicted marks an unmerged path, whose resolution is out of scope.
	Conflicted bool
	// NoTextChange marks a change without lines, such as a mode change or
	// an empty file added.
	NoTextChange bool
	// NoHunks says why the diff cannot be staged or unstaged by hunk, ""
	// when it can (StageHunk, UnstageHunk).
	NoHunks Code
}

// HasText reports whether Patch holds lines to show.
func (d Diff) HasText() bool {
	return d.Patch != "" && !d.Binary && !d.TooLarge && !d.Submodule && !d.Conflicted && !d.NoTextChange
}

// diffOptions keep Git's output in the shape GitOwl parses, whatever the
// user's configuration says.
var diffOptions = []string{
	"--no-ext-diff", "--no-textconv", "--no-color", "--no-relative",
	"--src-prefix=a/", "--dst-prefix=b/", "--submodule=short", "--find-renames",
}

// FileDiff returns ch's diff in its area: the index against HEAD for staged
// changes (against the empty tree before the first commit), the working tree
// against the index for unstaged ones, and the whole file for untracked ones.
func (r *Repository) FileDiff(ctx context.Context, ch Change) (Diff, error) {
	d, err := r.fileDiff(ctx, ch)
	if err == nil {
		d.NoHunks = hunkSupport(d)
	}
	return d, err
}

func (r *Repository) fileDiff(ctx context.Context, ch Change) (Diff, error) {
	d := Diff{Change: ch}
	switch {
	case ch.Conflicted:
		d.Conflicted = true
		return d, nil
	case ch.Submodule, ch.Area == Untracked && strings.HasSuffix(ch.Path, "/"):
		d.Submodule = true
		return d, nil
	}
	var out []byte
	var err error
	switch ch.Area {
	case Staged:
		args := append([]string{"--literal-pathspecs", "diff", "--cached"}, diffOptions...)
		args = append(args, "--", ch.Path)
		if ch.OrigPath != "" {
			args = append(args, ch.OrigPath)
		}
		out, err = r.run(ctx, args...)
	case Unstaged:
		args := append([]string{"--literal-pathspecs", "diff"}, diffOptions...)
		out, err = r.run(ctx, append(args, "--", ch.Path)...)
	case Untracked:
		args := append([]string{"diff", "--no-index"}, diffOptions...)
		out, _, err = r.app.git.RunStatus(ctx, r.root, nil, []int{1}, append(args, "--", "/dev/null", ch.Path)...)
		err = gitFailure(err)
	}
	if err != nil {
		return Diff{}, err
	}
	return classify(d, out), nil
}

func classify(d Diff, out []byte) Diff {
	if len(out) > MaxPatchBytes {
		d.TooLarge = true
		return d
	}
	for line := range bytes.Lines(out) {
		switch {
		case bytes.HasPrefix(line, []byte("Binary files ")), bytes.HasPrefix(line, []byte("GIT binary patch")):
			d.Binary = true
			return d
		case bytes.HasPrefix(line, []byte("Submodule ")), bytes.HasPrefix(line, []byte("-Subproject commit")), bytes.HasPrefix(line, []byte("+Subproject commit")):
			d.Submodule = true
			return d
		}
	}
	d.Patch = string(out)
	if !bytes.Contains(out, []byte("\n@@ ")) {
		d.NoTextChange = true
	}
	return d
}

// kindLetter is a one-letter summary of a change kind.
func kindLetter(k git.Kind) string {
	return [...]string{"M", "A", "D", "R", "C", "T", "U", "!"}[k]
}

// Letter returns the change's one-letter status, as Git abbreviates it.
func (c Change) Letter() string { return kindLetter(c.Kind) }
