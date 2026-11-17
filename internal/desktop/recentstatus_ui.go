package desktop

import (
	"context"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// recentLine is how one Recent row's status reads in the interface: the
// branch label, the change-count pill text and its color, and a status
// word for a folder that could not be read. reading is true while the
// background read for this row is still in flight.
//
// A row shows a branch and a count only when ok is true (a readable
// repository). When ok is false, status carries the reason; while reading
// both stay empty, so a slow read never looks Clean.
type recentLine struct {
	reading    bool
	ok         bool
	branch     string
	count      string
	countColor ui.Color
	status     string
}

// recentStatusOf returns the last read status for root and whether a read
// is still in flight. A nil status with reading false means no read has
// started yet, which the formatter also treats as reading.
func (u *UI) recentStatusOf(root string) (*workflow.RecentStatus, bool) {
	if u.recent.reading[root] {
		return nil, true
	}
	if st, ok := u.recent.status[root]; ok {
		return &st, false
	}
	return nil, true
}

// recentStatusLine turns a read status into the row's labels and colors,
// using the interface language through t and the design colors in d. It is
// pure: the same status always gives the same line, so tests read it
// directly. A nil status (not yet read) reads as still reading.
func (u *UI) recentStatusLine(st *workflow.RecentStatus, reading bool, d kitTokens) recentLine {
	if st == nil || reading {
		return recentLine{reading: true, status: u.t("recentStatusReading")}
	}
	switch st.Kind {
	case workflow.RecentOK:
		return u.recentOKLine(*st, d)
	case workflow.RecentMissing:
		return recentLine{status: u.t("recentStatusMissing")}
	case workflow.RecentNotRepository:
		return recentLine{status: u.t("recentStatusNotRepository")}
	case workflow.RecentPermissionDenied:
		return recentLine{status: u.t("recentStatusPermission")}
	default:
		return recentLine{status: u.t("recentStatusFailed")}
	}
}

// recentOKLine builds the line for a readable repository: its branch label
// and the deduplicated change count colored by the design (clean green,
// changes amber).
func (u *UI) recentOKLine(st workflow.RecentStatus, d kitTokens) recentLine {
	line := recentLine{ok: true, branch: u.recentBranch(st)}
	if st.Modified == 0 {
		line.count, line.countColor = u.t("recentStatusClean"), d.add
	} else {
		line.count = u.t("recentStatusChanges", "count", st.Modified)
		line.countColor = d.warn
	}
	return line
}

// recentBranch is the branch label for a readable repository: the short
// commit hash for a detached HEAD, the "no commits yet" word for an unborn
// repository, else the branch name.
func (u *UI) recentBranch(st workflow.RecentStatus) string {
	switch {
	case st.Unborn:
		return u.t("recentStatusUnborn")
	case st.Detached:
		return u.t("recentStatusDetached", "short", st.Short)
	default:
		return st.Branch
	}
}

// refreshRecentStatus reads every recent repository's status in the
// background, one goroutine each so a slow one does not hold up the rest,
// and applies each reply on the UI thread only while it is still current.
// A generation number (seq) rises whenever the list changes or the Landing
// is entered again, so a reply for an old generation is dropped and can
// neither revive a removed row nor overwrite the current screen. It never
// blocks opening a repository: the reads run off the UI thread.
func (u *UI) refreshRecentStatus() {
	u.recent.seq++
	seq := u.recent.seq
	u.recent.status = map[string]workflow.RecentStatus{}
	u.recent.reading = map[string]bool{}
	if u.wf == nil {
		return
	}
	list := append([]string(nil), u.prefs.Recent...)
	for _, root := range list {
		u.recent.reading[root] = true
	}
	for _, root := range list {
		root := root
		u.background(func() func() {
			st := u.wf.RecentStatus(context.Background(), root)
			return func() { u.applyRecentStatus(seq, root, st) }
		})
	}
}

// applyRecentStatus records one reply if its generation is still current
// and the row is still awaited; otherwise it is dropped, so a stale reply
// cannot revive a removed row or overwrite the current screen.
func (u *UI) applyRecentStatus(seq uint64, root string, st workflow.RecentStatus) {
	if seq != u.recent.seq {
		return
	}
	if !u.recent.reading[root] {
		return
	}
	u.recent.status[root] = st
	delete(u.recent.reading, root)
}

// recentMissing reports whether root was last read as a folder that is
// gone, for the Switch Repository picker's dimmed, "Not found" item. A row
// still reading, or read as a non-repository, is not treated as missing.
func (u *UI) recentMissing(root string) bool {
	if u.recent.reading[root] {
		return false
	}
	st, ok := u.recent.status[root]
	return ok && st.Kind == workflow.RecentMissing
}
