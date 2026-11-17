package desktop

import (
	"context"
	"gitowl/internal/workflow"
)

// hunkNote explains a diff staged only as a whole file, and why. A diff
// that can be staged by hunk has the hunk bar instead.
func (u *UI) hunkNote(p *diffPane) string {
	if p.state == nil || p.key.area == workflow.Untracked {
		return ""
	}
	if p.info.NoHunks != "" {
		return u.t("error" + string(p.info.NoHunks))
	}
	return ""
}

// applyHunk stages or unstages hunk i of the shown diff. Only the index
// changes; the list and the diff are read again afterwards, and the
// selection follows the file when no change of it is left in its area.
func (u *UI) applyHunk(i int) {
	p, repo := u.wt.diff, u.repo
	if p == nil || repo == nil || u.indexBusy() {
		return
	}
	info, key := p.info, p.key
	u.wt.busy = &key
	u.background(func() func() {
		var snap workflow.Snapshot
		var err error
		if key.area == workflow.Staged {
			snap, err = repo.UnstageHunk(context.Background(), info, i)
		} else {
			snap, err = repo.StageHunk(context.Background(), info, i)
		}
		return func() {
			if repo != u.repo {
				return
			}
			u.wt.busy = nil
			u.report(err)
			if snap.Root == "" {
				return
			}
			if sel := u.wt.selected; sel != nil && *sel == key && !inArea(snap, key.area, key.path) {
				u.wt.selected = u.followFile(snap, info.Change)
			}
			u.setSnapshot(snap)
		}
	})
}
