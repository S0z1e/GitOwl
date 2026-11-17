package workflow

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"gitowl/internal/git"
)

const (
	// CodeHunkStale: the file's diff changed since it was shown, so the
	// hunk chosen may no longer be what it was.
	CodeHunkStale Code = "HunkStale"
	// CodeHunkNotFound: the diff has no hunk with that number.
	CodeHunkNotFound Code = "HunkNotFound"
	// CodeHunkFailed: Git could not apply the hunk to the index cleanly;
	// nothing was changed.
	CodeHunkFailed Code = "HunkFailed"

	// Reasons a diff offers no hunk actions, as Diff.NoHunks says.
	HunksUntracked Code = "HunksUntracked"
	HunksAdded     Code = "HunksAdded"
	HunksDeleted   Code = "HunksDeleted"
	HunksRenamed   Code = "HunksRenamed"
	HunksMode      Code = "HunksMode"
	HunksSymlink   Code = "HunksSymlink"
	HunksNoText    Code = "HunksNoText"
)

// hunkSupport says why d, as read, offers no hunk actions, or "" when it
// does: only content changes of tracked regular text files, staged or
// unstaged, can be staged or unstaged by hunk.
func hunkSupport(d Diff) Code {
	ch := d.Change
	switch {
	case ch.Area == Untracked:
		return HunksUntracked
	case ch.Area != Staged && ch.Area != Unstaged:
		return HunksNoText
	case ch.Conflicted || ch.Submodule:
		return HunksNoText
	case ch.Kind == git.Added:
		return HunksAdded
	case ch.Kind == git.Deleted:
		return HunksDeleted
	case ch.Kind == git.Renamed || ch.Kind == git.Copied || ch.OrigPath != "":
		return HunksRenamed
	case ch.Kind == git.TypeChanged:
		return HunksSymlink
	case !d.HasText():
		return HunksNoText
	}
	header, _ := splitPatch(d.Patch)
	for _, l := range header {
		l = strings.TrimRight(l, "\r\n")
		switch {
		case strings.HasPrefix(l, "old mode "), strings.HasPrefix(l, "new mode "):
			return HunksMode
		case strings.HasPrefix(l, "index ") && strings.HasSuffix(l, " 120000"):
			return HunksSymlink
		case strings.HasPrefix(l, "rename "), strings.HasPrefix(l, "copy "), strings.HasPrefix(l, "new file"), strings.HasPrefix(l, "deleted file"):
			return HunksRenamed
		}
	}
	return ""
}

// gitHunk is one @@ section of a patch: its header numbers, the text after
// them and its lines, each with its newline.
type gitHunk struct {
	oldStart, oldLines, newStart, newLines int
	section                                string
	lines                                  []string
}

// splitPatch returns a one-file patch's header lines and its @@ sections.
// Lines keep their line endings, so a patch put together from them is
// byte for byte what Git wrote.
func splitPatch(patch string) ([]string, []gitHunk) {
	var header []string
	var hunks []gitHunk
	for line := range strings.Lines(patch) {
		if strings.HasPrefix(line, "@@ ") {
			if h, ok := parseHunkHeader(line); ok {
				hunks = append(hunks, h)
				continue
			}
		}
		if len(hunks) == 0 {
			header = append(header, line)
			continue
		}
		hunks[len(hunks)-1].lines = append(hunks[len(hunks)-1].lines, line)
	}
	return header, hunks
}

// parseHunkHeader reads "@@ -a,b +c,d @@ section".
func parseHunkHeader(line string) (gitHunk, bool) {
	rest := strings.TrimPrefix(line, "@@ ")
	ranges, section, ok := strings.Cut(rest, " @@")
	if !ok {
		return gitHunk{}, false
	}
	oldR, newR, ok := strings.Cut(ranges, " ")
	if !ok || !strings.HasPrefix(oldR, "-") || !strings.HasPrefix(newR, "+") {
		return gitHunk{}, false
	}
	parse := func(r string) (int, int, bool) {
		a, b, has := strings.Cut(r[1:], ",")
		start, err := strconv.Atoi(a)
		if err != nil {
			return 0, 0, false
		}
		n := 1
		if has {
			if n, err = strconv.Atoi(b); err != nil {
				return 0, 0, false
			}
		}
		return start, n, true
	}
	h := gitHunk{section: section}
	var ok1, ok2 bool
	h.oldStart, h.oldLines, ok1 = parse(oldR)
	h.newStart, h.newLines, ok2 = parse(newR)
	return h, ok1 && ok2
}

// run is a change hunk: a run of - and + lines in one @@ section, as the
// diff viewer marks them.
type run struct {
	section    int
	start, end int // lines of the section, end exclusive
}

// changeRuns finds the change hunks of a patch, in order. A "\ No newline"
// marker belongs to the line before it.
func changeRuns(hunks []gitHunk) []run {
	var runs []run
	for si, h := range hunks {
		open := false
		for li, l := range h.lines {
			switch {
			case l == "" || l[0] == '\\':
				continue
			case l[0] == '-' || l[0] == '+':
				if !open {
					runs = append(runs, run{section: si, start: li})
					open = true
				}
				runs[len(runs)-1].end = li + 1
			default:
				open = false
			}
		}
	}
	return runs
}

// HunkCount returns the number of change hunks in a diff's patch.
func (d Diff) HunkCount() int {
	_, hunks := splitPatch(d.Patch)
	return len(changeRuns(hunks))
}

// hunkPatch builds a patch of patch's change hunk i alone, to apply to the
// index: forward to stage it from an unstaged diff, or reversed to unstage
// it from a staged one. The other changes of its @@ section turn into the
// context the index has: for staging, lines the hunk would delete stay and
// lines it would add go; reversed, the other way round.
func hunkPatch(patch string, i int, reverse bool) (string, bool) {
	header, hunks := splitPatch(patch)
	runs := changeRuns(hunks)
	if i < 0 || i >= len(runs) {
		return "", false
	}
	sel := runs[i]
	h := hunks[sel.section]
	keepAsContext, drop := byte('-'), byte('+')
	if reverse {
		keepAsContext, drop = '+', '-'
	}
	var body []string
	oldN, newN := 0, 0
	dropped := false
	for li, l := range h.lines {
		if l != "" && l[0] == '\\' {
			if !dropped {
				body = append(body, l)
			}
			continue
		}
		dropped = false
		mine := li >= sel.start && li < sel.end
		kind := byte(' ')
		if l != "" {
			kind = l[0]
		}
		switch {
		case mine || kind == ' ':
		case kind == keepAsContext:
			l = " " + l[1:]
			kind = ' '
		case kind == drop:
			dropped = true
			continue
		}
		switch kind {
		case '-':
			oldN++
		case '+':
			newN++
		default:
			oldN++
			newN++
		}
		body = append(body, l)
	}
	oldStart, newStart := h.oldStart, h.newStart
	// An empty side names the line before it.
	if h.oldLines == 0 && oldN > 0 {
		oldStart++
	} else if h.oldLines > 0 && oldN == 0 {
		oldStart--
	}
	if h.newLines == 0 && newN > 0 {
		newStart++
	} else if h.newLines > 0 && newN == 0 {
		newStart--
	}
	var b strings.Builder
	for _, l := range header {
		b.WriteString(l)
	}
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s", oldStart, oldN, newStart, newN, h.section)
	for _, l := range body {
		b.WriteString(l)
	}
	return b.String(), true
}

// StageHunk stages change hunk i of an unstaged diff as it was shown.
func (r *Repository) StageHunk(ctx context.Context, d Diff, i int) (Snapshot, error) {
	return r.applyHunk(ctx, d, i, Unstaged)
}

// UnstageHunk unstages change hunk i of a staged diff as it was shown.
func (r *Repository) UnstageHunk(ctx context.Context, d Diff, i int) (Snapshot, error) {
	return r.applyHunk(ctx, d, i, Staged)
}

// applyHunk changes only the index, by one change hunk, after checking
// that the file's diff is still byte for byte the one shown. Git applies
// all of the hunk or nothing. Files never change. The state read
// afterwards is returned, also after a refusal, so the window shows what
// is there now.
func (r *Repository) applyHunk(ctx context.Context, d Diff, i int, area Area) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	if d.Change.Area != area {
		return Snapshot{}, &Error{Code: CodeHunkNotFound}
	}
	if code := hunkSupport(d); code != "" {
		return Snapshot{}, &Error{Code: code}
	}
	now, err := r.FileDiff(ctx, d.Change)
	if err != nil {
		return Snapshot{}, err
	}
	if now.Patch != d.Patch {
		snap, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, rerr
		}
		return snap, &Error{Code: CodeHunkStale, Detail: d.Change.Path}
	}
	patch, ok := hunkPatch(d.Patch, i, area == Staged)
	if !ok {
		return Snapshot{}, &Error{Code: CodeHunkNotFound}
	}
	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if area == Staged {
		args = append(args, "--reverse")
	}
	_, applyErr := r.app.git.Run(ctx, r.root, bytes.NewReader([]byte(patch)), append(args, "-")...)
	snap, err := r.read(ctx)
	if applyErr != nil {
		if c := CodeOf(gitFailure(applyErr)); c == CodeGitMissing || c == CodeCancelled {
			return snap, gitFailure(applyErr)
		}
		return snap, fail(CodeHunkFailed, applyErr)
	}
	return snap, err
}
