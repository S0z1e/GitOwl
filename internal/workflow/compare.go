package workflow

import (
	"context"

	"gitowl/internal/git"
)

// InCompare is the area of a file that differs between two compared refs.
const InCompare Area = 4

// CompareResult is the file list between two sides. A single file's patch
// is a Diff from CompareFileDiff.
type CompareResult struct {
	A, B         string
	AHash, BHash string
	Files        []Change
}

// CompareRefs lists the files that differ from a to b. An empty b is the
// working tree, so a commit can be compared to the working directory.
func (r *Repository) CompareRefs(ctx context.Context, a, b string) (CompareResult, error) {
	cmp := CompareResult{A: a, B: b}
	if a == "" {
		return cmp, &Error{Code: CodeRefNotFound, Detail: a}
	}
	hash, err := r.verifyRef(ctx, a)
	if err != nil {
		return cmp, err
	}
	if hash == "" {
		return cmp, &Error{Code: CodeRefNotFound, Detail: a}
	}
	cmp.AHash = hash
	args := append([]string{"diff", "--raw", "-z", "--no-abbrev", "--find-renames", "--no-ext-diff"}, hash)
	if b != "" {
		other, err := r.verifyRef(ctx, b)
		if err != nil {
			return cmp, err
		}
		if other == "" {
			return cmp, &Error{Code: CodeRefNotFound, Detail: b}
		}
		cmp.BHash = other
		args = append(args, other)
	}
	args = append(args, "--")
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, args...)
	if err != nil {
		return cmp, gitFailure(err)
	}
	changes, err := git.ParseRaw(out)
	if err != nil {
		return cmp, fail(CodeGitFailed, err)
	}
	for _, rc := range changes {
		cmp.Files = append(cmp.Files, Change{Path: rc.Path, OrigPath: rc.OrigPath, Kind: rawKind(rc.Status), Area: InCompare, Submodule: rc.Submodule()})
	}
	return cmp, nil
}

// CompareFileDiff returns f's Diff between the two sides of cmp.
func (r *Repository) CompareFileDiff(ctx context.Context, cmp CompareResult, f Change) (Diff, error) {
	d := Diff{Change: f}
	if f.Submodule {
		d.Submodule = true
		return d, nil
	}
	args := append([]string{"--literal-pathspecs", "diff"}, diffOptions...)
	args = append(args, cmp.AHash)
	if cmp.BHash != "" {
		args = append(args, cmp.BHash)
	}
	args = append(args, "--", f.Path)
	if f.OrigPath != "" {
		args = append(args, f.OrigPath)
	}
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, args...)
	if err != nil {
		return Diff{}, gitFailure(err)
	}
	return classify(d, out), nil
}
