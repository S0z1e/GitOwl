package desktop

import (
	"context"
	"strconv"
	"strings"

	"gitowl/internal/workflow"
)

// changeRowHeight is the height of a Changes file row, in DIPs
// (repository-changes.md: `.fr` height 36). It is the Changes list's own
// pitch, apart from the shared rowHeight the branch and commit-file lists
// keep.
const changeRowHeight = 38

// matchesFilter reports whether a file's name or path contains the filter
// text, case-insensitively and taken literally (no globbing). An empty
// filter matches everything.
func matchesFilter(filter, filePath string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(filePath), strings.ToLower(filter))
}

// filtered reports whether the file at the given path is shown under the
// current filter.
func (w *workingTree) filtered(filePath string) bool {
	return matchesFilter(w.filter, filePath)
}

// chromeRows is the Current changes list without empty areas: a header
// whose real count is 0 is omitted, while a header whose files are only
// hidden by Filter files stays, with that real count.
func (w *workingTree) chromeRows() []changeRow {
	var out []changeRow
	for _, r := range w.rows {
		if r.key.header && r.count == 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// visibleCount counts an area's files that the filter shows now.
func (w *workingTree) visibleCount(cs []workflow.Change) int {
	if w.filter == "" {
		return len(cs)
	}
	n := 0
	for _, c := range cs {
		if w.filtered(c.Path) {
			n++
		}
	}
	return n
}

// stat returns a file's line counts, and whether they are known for the
// current snapshot. An area and path identify the file, so a path that is
// both staged and unstaged keeps a count per side.
func (w *workingTree) stat(key changeKey) (workflow.FileStat, bool) {
	if !w.statsReady {
		return workflow.FileStat{}, false
	}
	s, ok := w.stats[key]
	return s, ok
}

// loadNumstat reads the current change list's line counts in the
// background. The read does not block the list; a result that arrives after
// the snapshot moved on is dropped, so stale counts never cover a newer
// snapshot.
func (u *UI) loadNumstat() {
	repo, snap := u.repo, u.snap
	if repo == nil || snap.Clean() {
		u.wt.stats, u.wt.statsReady = nil, false
		return
	}
	u.wt.statSeq++
	seq := u.wt.statSeq
	u.background(func() func() {
		ns, err := repo.Numstat(context.Background(), snap)
		return func() {
			if repo != u.repo || seq != u.wt.statSeq {
				return
			}
			if err != nil || ns.Stale(u.snap) {
				return
			}
			stats := make(map[changeKey]workflow.FileStat, len(ns.Files))
			for _, f := range ns.Files {
				stats[changeKey{area: f.Area, path: f.Path}] = f
			}
			u.wt.stats = stats
			u.wt.stagedAdded, u.wt.stagedRemoved = ns.StagedAdded, ns.StagedRemoved
			u.wt.statsReady = true
		}
	})
}

// lineCounts returns a file's "+N −M" text, or "" when the counts are not
// yet known. A binary or otherwise uncounted change returns the caller's
// binary label instead, through the ok=false, binary=true result, so the
// list can say so rather than show a false zero.
func (u *UI) lineCounts(key changeKey) (text string, binary, ok bool) {
	s, known := u.wt.stat(key)
	if !known {
		return "", false, false
	}
	if s.Binary {
		return "", true, true
	}
	return "+" + strconv.Itoa(s.Added) + " −" + strconv.Itoa(s.Removed), false, true
}
