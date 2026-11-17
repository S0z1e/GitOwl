package desktop

import (
	"github.com/egoist/mygo/ui"
)

// savedSession is what the window keeps of a worktree while another one is
// open, for this run of the app: the user's words and where they were. It
// holds no workflow handle, background work or Git state; returning reads
// the worktree again and keeps only what still applies.
type savedSession struct {
	message string
	// summary and description are the two fields panel B edits, kept
	// alongside message so a return to the worktree restores exactly what
	// was typed without re-splitting (see composer.syncFields).
	summary       string
	description   string
	applied       string
	showWhenClean bool
	// amend keeps the Amend row's mode and its stashed normal draft, so a
	// return to the worktree finds amend as it was left and, when amend was
	// off, the normal draft intact (commit-b.md: the two drafts are kept
	// apart and survive a worktree switch).
	amend      bool
	amendStash draft
	middleTab  int
	detail     bool
	// file is the selected change; changesTop the first visible row.
	file       *changeKey
	changesTop int
	// commit is the selected commit's hash; historyTop the first visible
	// row of history.
	commit     string
	historyTop int
}

// saveSession remembers the open worktree's session under its root.
func (u *UI) saveSession() {
	if u.repo == nil {
		return
	}
	if u.sessions == nil {
		u.sessions = map[string]savedSession{}
	}
	s := savedSession{
		message: u.cm.message, summary: u.cm.summary, description: u.cm.description,
		applied: u.ai.applied, showWhenClean: u.cm.showWhenClean,
		amend: u.cm.amend, amendStash: u.cm.stash,
		middleTab: u.middleTab, detail: u.lay.detail, commit: u.hist.selected,
	}
	if u.wt.selected != nil {
		key := *u.wt.selected
		s.file = &key
	}
	s.changesTop, _ = u.wt.list.Visible()
	s.historyTop, _ = u.hist.list.Visible()
	u.sessions[u.repo.Root()] = s
}

// restoreSession applies what was saved for root, after the worktree's
// state was read again: a file no longer in the same area is not selected,
// and a commit no longer in history is dropped once history is read.
func (u *UI) restoreSession(root string) {
	s, ok := u.sessions[root]
	if !ok {
		return
	}
	u.cm.message, u.ai.applied, u.cm.showWhenClean = s.message, s.applied, s.showWhenClean
	// Restore the two edited fields as they were and mark them composed, so
	// syncFields leaves them alone unless the message is set from elsewhere.
	u.cm.summary, u.cm.description, u.cm.composed = s.summary, s.description, s.message
	// Restore amend mode and its stashed normal draft; the target reloads
	// from the worktree's HEAD on the next frame (loadAmendTarget).
	u.cm.amend, u.cm.stash = s.amend, s.amendStash
	u.middleTab = s.middleTab
	u.hist.selected, u.hist.restoreTop = s.commit, s.historyTop
	if h := &u.hist; h.loaded && !h.loading {
		// History was read already: apply the selection and scroll now.
		h.selected, h.restoreTop = "", 0
		for i, c := range h.commits {
			if c.Hash == s.commit {
				u.openCommit(i)
			}
		}
		if s.historyTop > 0 && len(h.commits) > 0 {
			h.list.ScrollTo(min(s.historyTop, len(h.commits)-1), ui.Start)
		}
	}
	if s.file != nil {
		if i, ok := u.wt.row(*s.file); ok {
			key := *s.file
			u.wt.selected, u.wt.cursor = &key, i
			u.loadDiff(u.wt.rows[i].change)
		}
	}
	if s.changesTop > 0 && len(u.wt.rows) > 0 {
		u.wt.list.ScrollTo(min(s.changesTop, len(u.wt.rows)-1), ui.Start)
	}
	// A narrow window returns to the detail pane only with something in it.
	u.lay.detail = s.detail && (u.wt.selected != nil && s.middleTab == 0 || s.commit != "" && s.middleTab == 1)
}
